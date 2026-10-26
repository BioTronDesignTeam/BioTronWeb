package api

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/catalog"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/model"
)

type Store interface {
	Ping(context.Context) error
	LatestHealth(context.Context, []string) (map[string]model.Health, error)
	HealthHistory(context.Context, []string, time.Time, time.Time, time.Duration) (map[string][]model.HealthPoint, error)
}

// Options carries the settings the public status layer needs. Health durations
// come from the same configuration the monitor uses, so the "is this reading
// stale?" threshold always tracks the real polling cadence.
type Options struct {
	HealthInterval        time.Duration
	HealthHistoryInterval time.Duration
	StatusCacheTTL        time.Duration
	StatusRateLimit       int
	// TrustedProxies are the peers whose Cf-Connecting-Ip header the API may
	// believe. Every rate-limit bucket is keyed off that value, so an empty
	// list means all of them collapse into one bucket holding the edge proxy's
	// own bridge address.
	TrustedProxies []string
	// Events receives the request log: *logclient.Client in the service and a
	// collector in the tests. Nil drops every event.
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
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Events == nil {
		// A nil client drops every event.
		o.Events = (*logclient.Client)(nil)
	}
	return o
}

type Server struct {
	store          Store
	catalog        *catalog.Catalog
	healthInterval time.Duration
	maxGap         time.Duration
	location       *time.Location
	statusCache    *ttlCache
	now            func() time.Time
}

func New(store Store, serviceCatalog *catalog.Catalog, options Options) http.Handler {
	options = options.withDefaults()
	location, err := time.LoadLocation(bucketTimeZone)
	if err != nil {
		log.Printf("load %s, falling back to UTC: %v", bucketTimeZone, err)
		location = time.UTC
	}
	server := &Server{
		store:          store,
		catalog:        serviceCatalog,
		healthInterval: options.HealthInterval,
		// A stretch longer than three heartbeats means nobody was watching.
		maxGap:      3 * options.HealthHistoryInterval,
		location:    location,
		statusCache: newTTLCache(options.StatusCacheTTL, options.Now),
		now:         options.Now,
	}

	// Status sits behind the shared edge proxy, which rewrites
	// Cf-Connecting-Ip to the real client address on every hop. Without
	// trusting it, every request seems to come from the proxy's container
	// address and the limiters below throttle the whole internet as one
	// client. The header is only believed when the peer is on this list.
	proxies := newProxyTrust(options.TrustedProxies)

	mux := http.NewServeMux()
	mux.Handle("GET /health", handle(server.health))

	// The public layer answers with no session at all. It runs time-series
	// queries for anyone who asks, so it is rate limited per client address.
	publicLimit := newLimiter(options.StatusRateLimit, time.Minute, proxies.clientIP)
	public := func(h handlerFunc) http.Handler { return publicLimit.wrap(handle(h)) }
	mux.Handle("GET /v1/status", public(cacheFor(30*time.Second, server.status)))
	mux.Handle("GET /v1/status/history", public(cacheFor(30*time.Second, server.statusHistory)))

	// httplog is outermost, so the request log records the status the client
	// got and a panic is logged as the 500 it became.
	//
	// Quiet covers the two routes the status page polls on every load.
	return httplog.New(options.Events, httplog.Options{
		Quiet: []string{"/v1/status", "/v1/status/history"},
		Skip: func(_ *http.Request, status int) bool {
			// These requests come from the internet. One warning per throttled
			// request would hand an attacker a write into the log stream at the
			// rate limit itself; the limiter already counts them.
			return status == http.StatusTooManyRequests
		},
	})(mux)
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
			"service": "status-api",
			"status":  "unavailable",
		})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"service": "status-api", "status": "ok"})
	return nil
}
