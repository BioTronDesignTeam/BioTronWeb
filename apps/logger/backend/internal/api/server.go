package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"

	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
	"github.com/BioTronDesignTeam/Logger/backend/internal/selflog"
)

// bodyLimit bounds a request body, an ingest event included.
const bodyLimit = 256 * 1024

type Store interface {
	Ping(context.Context) error
	InsertLog(context.Context, model.NewLog) (model.Log, error)
	RecentLogs(context.Context, []string, []model.LogLevel, string, int) ([]model.Log, error)
	QueryLogs(context.Context, model.HistoryQuery) (model.LogPage, error)
	LatestHealth(context.Context, []string) (map[string]model.Health, error)
	HealthHistory(context.Context, []string, time.Time, time.Time, time.Duration) (map[string][]model.HealthPoint, error)
}

// Options carries the settings the public status layer needs. Health durations
// come from the same configuration the monitor uses, so the "is this reading
// stale?" threshold always tracks the real polling cadence.
type Options struct {
	IngestToken           string
	HealthInterval        time.Duration
	HealthHistoryInterval time.Duration
	StatusCacheTTL        time.Duration
	StatusRateLimit       int
	IngestRateLimit       int
	// TrustedProxies are the peers whose Cf-Connecting-Ip header the API may
	// believe. Every rate-limit bucket is keyed off that value, so an empty
	// list means all of them collapse into one bucket holding the edge proxy's
	// own bridge address.
	TrustedProxies []string
	// Events receives the request log and the events the API raises itself.
	// Logger records its own events by writing into its store, not by posting
	// to its own ingest route, so this is *selflog.Recorder in the service and
	// a collector in the tests.
	Events httplog.Sink
	// Now is injectable so tests can pin a window without sleeping.
	Now func() time.Time
}

func (o Options) withDefaults() Options {
	if o.HealthInterval <= 0 {
		o.HealthInterval = 15 * time.Second
	}
	if o.HealthHistoryInterval <= 0 {
		o.HealthHistoryInterval = 5 * time.Minute
	}
	if o.StatusCacheTTL <= 0 {
		o.StatusCacheTTL = 30 * time.Second
	}
	if o.StatusRateLimit <= 0 {
		o.StatusRateLimit = 60
	}
	if o.IngestRateLimit <= 0 {
		o.IngestRateLimit = 600
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Events == nil {
		o.Events = selflog.Discard{}
	}
	return o
}

type Server struct {
	store          Store
	catalog        *catalog.Catalog
	authorizer     auth.Authorizer
	events         httplog.Sink
	ingestToken    string
	healthInterval time.Duration
	maxGap         time.Duration
	location       *time.Location
	statusCache    *ttlCache
	now            func() time.Time
}

func New(store Store, serviceCatalog *catalog.Catalog, authorizer auth.Authorizer, options Options) http.Handler {
	options = options.withDefaults()
	location, err := time.LoadLocation(bucketTimeZone)
	if err != nil {
		log.Printf("load %s, falling back to UTC: %v", bucketTimeZone, err)
		location = time.UTC
	}
	server := &Server{
		store:          store,
		catalog:        serviceCatalog,
		authorizer:     authorizer,
		events:         options.Events,
		ingestToken:    options.IngestToken,
		healthInterval: options.HealthInterval,
		// A stretch longer than three heartbeats means nobody was watching.
		maxGap:      3 * options.HealthHistoryInterval,
		location:    location,
		statusCache: newTTLCache(options.StatusCacheTTL, options.Now),
		now:         options.Now,
	}

	// Logger sits behind the shared edge proxy, which rewrites
	// Cf-Connecting-Ip to the real client address on every hop. Without
	// trusting it, every request seems to come from the proxy's container
	// address and the limiters below throttle the whole internet as one
	// client. The header is only believed when the peer is on this list.
	proxies := newProxyTrust(options.TrustedProxies)

	mux := http.NewServeMux()
	mux.Handle("GET /health", handle(server.health))

	// Ingestion is reachable from the internet and gated by one static token
	// shared by every sender, so a leak anywhere buys an unbounded write into
	// the platform's only audit trail — forged entries to bury a real
	// intrusion, or 256 KiB a request until the shared Postgres fills and takes
	// authentication down with Logger.
	//
	// The throttle sits ahead of the token check so a flood costs an attacker a
	// bucket whether or not they hold the token. It is per client address,
	// which is only true because the app trusts the edge for that address; see
	// proxies above.
	ingestLimit := newLimiter(options.IngestRateLimit, time.Minute, proxies.clientIP)
	mux.Handle("POST /v1/logs", ingestLimit.wrap(handle(server.requireIngestToken(server.ingestLog))))

	// The public layer answers with no session at all. It runs time-series
	// queries for anyone who asks, so it is rate limited per client address.
	// The Fiber app mounted this limiter on all of /v1 after the ingest route,
	// so the log explorer routes and unknown /v1 paths share the bucket too.
	publicLimit := newLimiter(options.StatusRateLimit, time.Minute, proxies.clientIP)
	public := func(h handlerFunc) http.Handler { return publicLimit.wrap(handle(h)) }
	mux.Handle("GET /v1/status", public(cacheFor(30*time.Second, server.status)))
	mux.Handle("GET /v1/status/history", public(cacheFor(30*time.Second, server.statusHistory)))
	mux.Handle("GET /v1/session", public(server.session))

	mux.Handle("GET /v1/apps", public(server.requireRead(server.apps)))
	mux.Handle("GET /v1/apps/{app}/logs/recent", public(server.requireRead(server.recentLogs)))
	mux.Handle("GET /v1/apps/{app}/logs/history", public(server.requireRead(server.historyLogs)))

	notFound := unmatched(mux, "/", "/v1", "/v1/")
	mux.Handle("/v1", public(notFound))
	mux.Handle("/v1/", public(notFound))
	mux.Handle("/", handle(notFound))

	handler := lenientPaths(mux)
	handler = recoverPanics(handler)
	// Fiber never cancelled a request's context when the client went away, and
	// the handlers count on that: a cancelled call to Auth would otherwise
	// read as an Auth outage and write a false error into the warehouse.
	handler = keepContext(handler)

	// Outermost, so the request log records the status the client got.
	//
	// Quiet covers the three routes the status page polls on every load and the
	// ingest route. Ingest is quiet for a reason of its own: an event for every
	// accepted event would double the warehouse, one row for the event and one
	// for the request that carried it. A refused ingest still speaks, because a
	// 400 says one of our own services is sending what this one cannot store.
	return httplog.New(options.Events, httplog.Options{
		Quiet: []string{"/v1/status", "/v1/status/history", "/v1/session", "/v1/logs"},
		Skip: func(r *http.Request, status int) bool {
			// These requests come from the internet. One warning row per refused
			// attempt would hand an attacker a write into the audit trail at the
			// rate limit itself: 600 rows a minute per address on ingest alone,
			// enough to bury a real intrusion in noise. A throttled request is
			// therefore dropped, and so is an ingest turned away for a bad
			// token. The limiter already bounds both, and neither tells an
			// operator anything the limiter has not counted.
			if status == http.StatusTooManyRequests {
				return true
			}
			return status == http.StatusUnauthorized && r.URL.Path == "/v1/logs"
		},
	})(handler)
}

func keepContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithoutCancel(r.Context())))
	})
}

// cacheFor lets shared caches hold the public status responses briefly. They are
// world-readable by design and change no faster than the health poll.
func cacheFor(ttl time.Duration, next handlerFunc) handlerFunc {
	value := "public, max-age=" + strconv.Itoa(int(ttl.Seconds()))
	return func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Cache-Control", value)
		return next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"service": "logger-api",
			"status":  "unavailable",
		})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"service": "logger-api", "status": "ok"})
	return nil
}

func (s *Server) requireIngestToken(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(s.ingestToken)) != 1 {
			return errUnauthorized
		}
		return next(w, r)
	}
}

func (s *Server) requireRead(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		decision, err := s.authorizer.Authorize(r.Context(), cookieHeader(r))
		if err != nil {
			// Auth is unreachable, so every operator is locked out of the log
			// explorer. The 503 the caller gets says nothing about why.
			log.Printf("authorize read: %v", err)
			s.events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{"error": err.Error()})
			return newError(http.StatusServiceUnavailable, "authorization service unavailable")
		}
		if !decision.Authenticated {
			return errUnauthorized
		}
		if !decision.Allowed {
			return errForbidden
		}
		return next(w, r)
	}
}

// cookieHeader is the browser's Cookie header as Auth expects it. HTTP/2 may
// split cookies across several fields; they rejoin with "; ".
func cookieHeader(r *http.Request) string {
	return strings.Join(r.Header.Values("Cookie"), "; ")
}

// session always answers HTTP 200. The portal needs to tell "signed out" from
// "signed in without logger/view", and a 401 or 403 collapses those two into
// one, which is what sent permission-less users round the sign-in loop.
func (s *Server) session(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	decision, err := s.authorizer.Authorize(r.Context(), cookieHeader(r))
	if err != nil {
		// The permission service is unreachable. Reporting a signed-out visitor
		// is the safe answer: it grants nothing and offers a sign-in button that
		// will work again once OAuthManager is back.
		log.Printf("session check: %v", err)
		// The visitor sees a sign-in button that cannot work. Only a request
		// carrying a cookie reaches this line -- Authorize answers an empty
		// cookie header without calling out -- so an anonymous flood cannot
		// turn this into a write into the warehouse.
		s.events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{"error": err.Error()})
		decision = auth.Decision{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": decision.Authenticated, "allowed": decision.Allowed})
	return nil
}

func (s *Server) ingestLog(w http.ResponseWriter, r *http.Request) error {
	var input model.NewLog
	if err := decodeJSONBody(w, r, &input); err != nil {
		return err
	}
	input.Service = strings.TrimSpace(input.Service)
	input.Message = strings.TrimSpace(input.Message)
	if !s.catalog.HasService(input.Service) {
		return newError(http.StatusBadRequest, "unknown service")
	}
	if !input.Level.Valid() {
		return newError(http.StatusBadRequest, "level must be debug, info, warning, or error")
	}
	if input.Message == "" || len(input.Message) > 16*1024 {
		return newError(http.StatusBadRequest, "message must be between 1 and 16384 characters")
	}
	if len(input.Payload) > 128*1024 || (len(input.Payload) > 0 && !json.Valid(input.Payload)) {
		return newError(http.StatusBadRequest, "payload must be valid JSON no larger than 128 KiB")
	}

	entry, err := s.store.InsertLog(r.Context(), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, entry)
	return nil
}

// decodeJSONBody reads a JSON request body the way Fiber's body binder did:
// the Content-Type must name JSON, and the whole body must be one JSON value.
// A body past bodyLimit is a 413, as Fiber's BodyLimit made it.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, into any) error {
	mediaType, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";")
	if strings.ToLower(strings.TrimSpace(mediaType)) != "application/json" {
		return newError(http.StatusBadRequest, "invalid JSON body")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyLimit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return newError(http.StatusRequestEntityTooLarge)
		}
		return newError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := json.Unmarshal(body, into); err != nil {
		return newError(http.StatusBadRequest, "invalid JSON body")
	}
	return nil
}

type applicationStatus struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	State       string            `json:"state"`
	Components  []componentStatus `json:"components"`
}

type componentStatus struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	Detail    string     `json:"detail,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

func (s *Server) apps(w http.ResponseWriter, r *http.Request) error {
	applications := s.catalog.Applications()
	services := make([]string, 0)
	for _, app := range applications {
		services = append(services, catalog.ServiceIDs(app)...)
	}
	health, err := s.store.LatestHealth(r.Context(), services)
	if err != nil {
		return err
	}

	statuses := make([]applicationStatus, 0, len(applications))
	for _, app := range applications {
		status := applicationStatus{
			ID: app.ID, Name: app.Name, Description: app.Description, State: "healthy",
			Components: make([]componentStatus, 0, len(app.Components)),
		}
		for _, component := range app.Components {
			componentState := componentStatus{ID: component.ID, Name: component.Name, State: "unknown"}
			if latest, ok := health[component.ID]; ok {
				componentState.Detail = latest.Detail
				componentState.CheckedAt = &latest.CheckedAt
				if latest.OK {
					componentState.State = "healthy"
				} else {
					componentState.State = "unhealthy"
					status.State = "unhealthy"
				}
			} else if status.State == "healthy" {
				status.State = "unknown"
			}
			status.Components = append(status.Components, componentState)
		}
		statuses = append(statuses, status)
	}
	writeJSON(w, http.StatusOK, map[string]any{"applications": statuses, "generated_at": time.Now()})
	return nil
}

func (s *Server) recentLogs(w http.ResponseWriter, r *http.Request) error {
	app, ok := s.catalog.Application(r.PathValue("app"))
	if !ok {
		return errNotFound
	}
	query := r.URL.Query()
	levels, err := parseLevels(query.Get("levels"))
	if err != nil {
		return newError(http.StatusBadRequest, err.Error())
	}
	limit := parseLimit(query.Get("limit"))
	logs, err := s.store.RecentLogs(r.Context(), catalog.ServiceIDs(app), levels, strings.TrimSpace(query.Get("q")), limit)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
	return nil
}

func (s *Server) historyLogs(w http.ResponseWriter, r *http.Request) error {
	app, ok := s.catalog.Application(r.PathValue("app"))
	if !ok {
		return errNotFound
	}
	query := r.URL.Query()
	levels, err := parseLevels(query.Get("levels"))
	if err != nil {
		return newError(http.StatusBadRequest, err.Error())
	}
	from, err := parseTime(query.Get("from"))
	if err != nil {
		return newError(http.StatusBadRequest, "from must be an RFC3339 timestamp")
	}
	to, err := parseTime(query.Get("to"))
	if err != nil {
		return newError(http.StatusBadRequest, "to must be an RFC3339 timestamp")
	}
	page, err := s.store.QueryLogs(r.Context(), model.HistoryQuery{
		Services: catalog.ServiceIDs(app),
		Levels:   levels,
		From:     from,
		To:       to,
		Search:   strings.TrimSpace(query.Get("q")),
		Cursor:   query.Get("cursor"),
		Limit:    parseLimit(query.Get("limit")),
	})
	if err != nil {
		if err.Error() == "invalid cursor" {
			return newError(http.StatusBadRequest, err.Error())
		}
		return err
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

func parseLevels(raw string) ([]model.LogLevel, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := make(map[model.LogLevel]bool)
	levels := make([]model.LogLevel, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		level := model.LogLevel(strings.ToLower(strings.TrimSpace(part)))
		if !level.Valid() {
			return nil, errors.New("levels must contain only debug, info, warning, or error")
		}
		if !seen[level] {
			levels = append(levels, level)
			seen[level] = true
		}
	}
	return levels, nil
}

func parseLimit(raw string) int {
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 100
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func parseTime(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
