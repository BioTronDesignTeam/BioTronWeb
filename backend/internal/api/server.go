package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

type Store interface {
	Ping(context.Context) error
	InsertLog(context.Context, model.NewLog) (model.Log, error)
	RecentLogs(context.Context, []string, []model.LogLevel, string, int) ([]model.Log, error)
	QueryLogs(context.Context, model.HistoryQuery) (model.LogPage, error)
	LatestHealth(context.Context, []string) (map[string]model.Health, error)
	HealthHistory(context.Context, []string, time.Time, time.Time) (map[string][]model.HealthPoint, error)
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
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

type Server struct {
	store          Store
	catalog        *catalog.Catalog
	authorizer     auth.Authorizer
	ingestToken    string
	healthInterval time.Duration
	maxGap         time.Duration
	location       *time.Location
	statusCache    *ttlCache
	now            func() time.Time
}

func New(store Store, serviceCatalog *catalog.Catalog, authorizer auth.Authorizer, options Options) *fiber.App {
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
		ingestToken:    options.IngestToken,
		healthInterval: options.HealthInterval,
		// A stretch longer than three heartbeats means nobody was watching.
		maxGap:      3 * options.HealthHistoryInterval,
		location:    location,
		statusCache: newTTLCache(options.StatusCacheTTL, options.Now),
		now:         options.Now,
	}
	app := fiber.New(fiber.Config{
		BodyLimit: 256 * 1024,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			message := "internal server error"
			var fiberError *fiber.Error
			if errors.As(err, &fiberError) {
				code = fiberError.Code
				message = fiberError.Message
			} else {
				log.Printf("request %s %s: %v", c.Method(), c.Path(), err)
			}
			return c.Status(code).JSON(fiber.Map{"error": message})
		},
	})
	app.Use(recover.New())

	app.Get("/health", server.health)
	v1 := app.Group("/v1")
	v1.Post("/logs", server.requireIngestToken, server.ingestLog)

	// The public layer answers with no session at all. It runs time-series
	// queries for anyone who asks, so it is rate limited per client address.
	public := v1.Group("", limiter.New(limiter.Config{
		Max:        options.StatusRateLimit,
		Expiration: time.Minute,
	}))
	public.Get("/status", cacheFor(30*time.Second), server.status)
	public.Get("/status/history", cacheFor(30*time.Second), server.statusHistory)
	public.Get("/session", server.session)

	v1.Get("/apps", server.requireRead, server.apps)
	v1.Get("/apps/:app/logs/recent", server.requireRead, server.recentLogs)
	v1.Get("/apps/:app/logs/history", server.requireRead, server.historyLogs)
	return app
}

// cacheFor lets shared caches hold the public status responses briefly. They are
// world-readable by design and change no faster than the health poll.
func cacheFor(ttl time.Duration) fiber.Handler {
	value := "public, max-age=" + strconv.Itoa(int(ttl.Seconds()))
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, value)
		return c.Next()
	}
}

func (s *Server) health(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"service": "logger-api",
			"status":  "unavailable",
		})
	}
	return c.JSON(fiber.Map{"service": "logger-api", "status": "ok"})
}

func (s *Server) requireIngestToken(c *fiber.Ctx) error {
	provided := strings.TrimPrefix(c.Get(fiber.HeaderAuthorization), "Bearer ")
	if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(s.ingestToken)) != 1 {
		return fiber.ErrUnauthorized
	}
	return c.Next()
}

func (s *Server) requireRead(c *fiber.Ctx) error {
	decision, err := s.authorizer.Authorize(c.UserContext(), c.Get(fiber.HeaderCookie))
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "authorization service unavailable")
	}
	if !decision.Authenticated {
		return fiber.ErrUnauthorized
	}
	if !decision.Allowed {
		return fiber.ErrForbidden
	}
	return c.Next()
}

// session always answers HTTP 200. The portal needs to tell "signed out" from
// "signed in without logger/view", and a 401 or 403 collapses those two into
// one, which is what sent permission-less users round the sign-in loop.
func (s *Server) session(c *fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	decision, err := s.authorizer.Authorize(c.UserContext(), c.Get(fiber.HeaderCookie))
	if err != nil {
		// The permission service is unreachable. Reporting a signed-out visitor
		// is the safe answer: it grants nothing and offers a sign-in button that
		// will work again once OAuthManager is back.
		log.Printf("session check: %v", err)
		decision = auth.Decision{}
	}
	return c.JSON(fiber.Map{"authenticated": decision.Authenticated, "allowed": decision.Allowed})
}

func (s *Server) ingestLog(c *fiber.Ctx) error {
	var input model.NewLog
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	input.Service = strings.TrimSpace(input.Service)
	input.Message = strings.TrimSpace(input.Message)
	if !s.catalog.HasService(input.Service) {
		return fiber.NewError(fiber.StatusBadRequest, "unknown service")
	}
	if !input.Level.Valid() {
		return fiber.NewError(fiber.StatusBadRequest, "level must be debug, info, warning, or error")
	}
	if input.Message == "" || len(input.Message) > 16*1024 {
		return fiber.NewError(fiber.StatusBadRequest, "message must be between 1 and 16384 characters")
	}
	if len(input.Payload) > 128*1024 || (len(input.Payload) > 0 && !json.Valid(input.Payload)) {
		return fiber.NewError(fiber.StatusBadRequest, "payload must be valid JSON no larger than 128 KiB")
	}

	entry, err := s.store.InsertLog(c.UserContext(), input)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(entry)
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

func (s *Server) apps(c *fiber.Ctx) error {
	applications := s.catalog.Applications()
	services := make([]string, 0)
	for _, app := range applications {
		services = append(services, catalog.ServiceIDs(app)...)
	}
	health, err := s.store.LatestHealth(c.UserContext(), services)
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
	return c.JSON(fiber.Map{"applications": statuses, "generated_at": time.Now()})
}

func (s *Server) recentLogs(c *fiber.Ctx) error {
	app, ok := s.catalog.Application(c.Params("app"))
	if !ok {
		return fiber.ErrNotFound
	}
	levels, err := parseLevels(c.Query("levels"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	limit := parseLimit(c.Query("limit"))
	logs, err := s.store.RecentLogs(c.UserContext(), catalog.ServiceIDs(app), levels, strings.TrimSpace(c.Query("q")), limit)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"logs": logs})
}

func (s *Server) historyLogs(c *fiber.Ctx) error {
	app, ok := s.catalog.Application(c.Params("app"))
	if !ok {
		return fiber.ErrNotFound
	}
	levels, err := parseLevels(c.Query("levels"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	from, err := parseTime(c.Query("from"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "from must be an RFC3339 timestamp")
	}
	to, err := parseTime(c.Query("to"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "to must be an RFC3339 timestamp")
	}
	page, err := s.store.QueryLogs(c.UserContext(), model.HistoryQuery{
		Services: catalog.ServiceIDs(app),
		Levels:   levels,
		From:     from,
		To:       to,
		Search:   strings.TrimSpace(c.Query("q")),
		Cursor:   c.Query("cursor"),
		Limit:    parseLimit(c.Query("limit")),
	})
	if err != nil {
		if err.Error() == "invalid cursor" {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return err
	}
	return c.JSON(page)
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
