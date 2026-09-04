package api

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	// The runtime image is debian-slim and carries no zoneinfo database, so the
	// America/Toronto rules used for daily buckets travel inside the binary.
	_ "time/tzdata"

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
	"github.com/BioTronDesignTeam/Logger/backend/internal/uptime"
)

// Status states shared by components, applications, and the overall roll-up.
const (
	stateOperational = "operational"
	stateDegraded    = "degraded"
	stateDown        = "down"
	stateUnknown     = "unknown"
)

const (
	historyDays    = 90
	bucketTimeZone = "America/Toronto"
)

type overallStatus struct {
	State     string    `json:"state"`
	Uptime24h *float64  `json:"uptime_24h"`
	Uptime7d  *float64  `json:"uptime_7d"`
	Uptime90d *float64  `json:"uptime_90d"`
	UpdatedAt time.Time `json:"updated_at"`
}

type publicComponent struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	Uptime24h *float64   `json:"uptime_24h"`
	Uptime7d  *float64   `json:"uptime_7d"`
	Uptime90d *float64   `json:"uptime_90d"`
	CheckedAt *time.Time `json:"checked_at"`
}

type publicApplication struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	State       string            `json:"state"`
	Components  []publicComponent `json:"components"`
}

type statusResponse struct {
	Overall      overallStatus       `json:"overall"`
	Applications []publicApplication `json:"applications"`
}

type historyBucket struct {
	Date   string   `json:"date"`
	Uptime *float64 `json:"uptime"`
	State  string   `json:"state"`
}

type componentHistory struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	ApplicationID string          `json:"application_id"`
	Buckets       []historyBucket `json:"buckets"`
}

type historyResponse struct {
	Days       int                `json:"days"`
	Timezone   string             `json:"timezone"`
	Components []componentHistory `json:"components"`
}

// status serves the public status page. It contains no health detail string, no
// health URL, and no log data of any kind: everything here is world-readable.
func (s *Server) status(c *fiber.Ctx) error {
	payload, err := cached(c.UserContext(), s.statusCache, "status", func(ctx context.Context) (statusResponse, error) {
		return s.buildStatus(ctx)
	})
	if err != nil {
		return err
	}
	return c.JSON(payload)
}

// statusHistory serves the fixed-width daily bar. days is clamped rather than
// echoed back, so a hostile value never reaches the response.
func (s *Server) statusHistory(c *fiber.Ctx) error {
	days := clampDays(c.Query("days"))
	payload, err := cached(c.UserContext(), s.statusCache, "history:"+strconv.Itoa(days), func(ctx context.Context) (historyResponse, error) {
		return s.buildHistory(ctx, days)
	})
	if err != nil {
		return err
	}
	return c.JSON(payload)
}

func clampDays(raw string) int {
	days, err := strconv.Atoi(raw)
	if err != nil {
		return historyDays
	}
	if days < 1 {
		return 1
	}
	if days > historyDays {
		return historyDays
	}
	return days
}

func (s *Server) buildStatus(ctx context.Context) (statusResponse, error) {
	now := s.now()
	applications := s.catalog.Applications()
	services := allServices(applications)

	latest, err := s.store.LatestHealth(ctx, services)
	if err != nil {
		return statusResponse{}, err
	}
	samples, err := s.samples(ctx, now, services)
	if err != nil {
		return statusResponse{}, err
	}

	windows := []uptime.Window{
		{Start: now.Add(-24 * time.Hour), End: now},
		{Start: now.AddDate(0, 0, -7), End: now},
		{Start: now.AddDate(0, 0, -historyDays), End: now},
	}
	totals := make([]uptime.Result, len(windows))

	response := statusResponse{
		Applications: make([]publicApplication, 0, len(applications)),
		Overall:      overallStatus{UpdatedAt: now.UTC().Truncate(time.Second)},
	}
	applicationStates := make([]string, 0, len(applications))

	for _, application := range applications {
		public := publicApplication{
			ID:          application.ID,
			Name:        application.Name,
			Description: application.Description,
			Components:  make([]publicComponent, 0, len(application.Components)),
		}
		componentStates := make([]string, 0, len(application.Components))

		for _, component := range application.Components {
			results := computeWindows(samples[component.ID], windows, s.maxGap)
			for i, result := range results {
				totals[i] = uptime.Combine(totals[i], result)
			}

			state, checkedAt := s.componentState(latest, component.ID, now)
			componentStates = append(componentStates, state)
			public.Components = append(public.Components, publicComponent{
				ID:        component.ID,
				Name:      component.Name,
				State:     state,
				Uptime24h: results[0].Percent(),
				Uptime7d:  results[1].Percent(),
				Uptime90d: results[2].Percent(),
				CheckedAt: checkedAt,
			})
		}

		public.State = rollUp(componentStates)
		applicationStates = append(applicationStates, public.State)
		response.Applications = append(response.Applications, public)
	}

	response.Overall.State = rollUp(applicationStates)
	response.Overall.Uptime24h = totals[0].Percent()
	response.Overall.Uptime7d = totals[1].Percent()
	response.Overall.Uptime90d = totals[2].Percent()
	return response, nil
}

func (s *Server) buildHistory(ctx context.Context, days int) (historyResponse, error) {
	now := s.now()
	applications := s.catalog.Applications()
	samples, err := s.samples(ctx, now, allServices(applications))
	if err != nil {
		return historyResponse{}, err
	}

	windows := uptime.DailyWindows(now, days, s.location)
	response := historyResponse{Days: days, Timezone: s.location.String(), Components: make([]componentHistory, 0)}
	for _, application := range applications {
		for _, component := range application.Components {
			results := uptime.Bucketed(samples[component.ID], windows, s.maxGap)
			buckets := make([]historyBucket, 0, len(windows))
			for i, window := range windows {
				percentage := results[i].Percent()
				buckets = append(buckets, historyBucket{
					Date:   window.Start.Format("2006-01-02"),
					Uptime: percentage,
					State:  bucketState(percentage),
				})
			}
			response.Components = append(response.Components, componentHistory{
				ID:            component.ID,
				Name:          component.Name,
				ApplicationID: application.ID,
				Buckets:       buckets,
			})
		}
	}
	return response, nil
}

// samples fetches, once per cache period, the full ninety days both public
// routes need. The query is the expensive part of this feature and it changes
// slowly, so one fetch feeds the rolling uptimes and the daily bars alike.
func (s *Server) samples(ctx context.Context, now time.Time, services []string) (map[string][]model.HealthPoint, error) {
	return cached(ctx, s.statusCache, "samples", func(ctx context.Context) (map[string][]model.HealthPoint, error) {
		return s.store.HealthHistory(ctx, services, now.AddDate(0, 0, -historyDays), now)
	})
}

func computeWindows(samples []model.HealthPoint, windows []uptime.Window, maxGap time.Duration) []uptime.Result {
	results := make([]uptime.Result, len(windows))
	for i, window := range windows {
		results[i] = uptime.Compute(samples, window, maxGap)
	}
	return results
}

// componentState maps the latest observation onto the coarse public enum. The
// health detail string never leaves this function: it embeds internal
// hostnames, ports, and raw dial errors, which together are a network map.
func (s *Server) componentState(latest map[string]model.Health, id string, now time.Time) (string, *time.Time) {
	health, ok := latest[id]
	if !ok || health.CheckedAt.IsZero() {
		return stateUnknown, nil
	}
	checkedAt := health.CheckedAt.UTC()
	if now.Sub(health.CheckedAt) > 3*s.healthInterval {
		return stateUnknown, &checkedAt
	}
	if health.OK {
		return stateOperational, &checkedAt
	}
	return stateDown, &checkedAt
}

// rollUp aggregates child states into a parent state. A parent is operational
// only when every child is; partial visibility is degraded rather than a
// reassuring green, because "all systems operational" must mean it.
func rollUp(states []string) string {
	if len(states) == 0 {
		return stateUnknown
	}
	var operational, degraded, down, unknown int
	for _, state := range states {
		switch state {
		case stateOperational:
			operational++
		case stateDegraded:
			degraded++
		case stateUnknown:
			unknown++
		default:
			down++
		}
	}
	switch {
	case down == len(states):
		return stateDown
	case down > 0 || degraded > 0:
		return stateDegraded
	case unknown == len(states):
		return stateUnknown
	case operational == len(states):
		return stateOperational
	default:
		// A mix of operational and unknown. Something is unobserved, so this is
		// not the moment to promise that all systems are operational.
		return stateDegraded
	}
}

func bucketState(percentage *float64) string {
	switch {
	case percentage == nil:
		return stateUnknown
	case *percentage >= 100:
		return stateOperational
	case *percentage > 99:
		return stateDegraded
	default:
		return stateDown
	}
}

func allServices(applications []catalog.Application) []string {
	services := make([]string, 0, len(applications)*2)
	for _, application := range applications {
		services = append(services, catalog.ServiceIDs(application)...)
	}
	return services
}

// ttlCache is a small in-process cache for the public routes. The status
// queries are unauthenticated and non-trivial, so an uncached hit is a cheap
// denial-of-service lever; a Redis round trip would be more machinery than this
// needs, since a stale-by-thirty-seconds status page is still a true one.
type ttlCache struct {
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	values  map[string]cacheEntry
	loaders map[string]*sync.Mutex
}

type cacheEntry struct {
	value    any
	storedAt time.Time
}

func newTTLCache(ttl time.Duration, now func() time.Time) *ttlCache {
	return &ttlCache{
		ttl:     ttl,
		now:     now,
		values:  make(map[string]cacheEntry),
		loaders: make(map[string]*sync.Mutex),
	}
}

func (c *ttlCache) lookup(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.values[key]
	if !ok || c.now().Sub(entry.storedAt) > c.ttl {
		return nil, false
	}
	return entry.value, true
}

// loader returns the lock guarding one key. Locks are per key because building
// a response reads another cached key underneath it, and a single shared lock
// would deadlock on that nesting.
func (c *ttlCache) loader(key string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, ok := c.loaders[key]
	if !ok {
		lock = &sync.Mutex{}
		c.loaders[key] = lock
	}
	return lock
}

func (c *ttlCache) store(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = cacheEntry{value: value, storedAt: c.now()}
}

// cached returns the memoised value for key, computing it at most once at a
// time. Loads are serialised so a burst of requests against a cold cache turns
// into one database query rather than sixty.
func cached[T any](ctx context.Context, cache *ttlCache, key string, load func(context.Context) (T, error)) (T, error) {
	if value, ok := cache.lookup(key); ok {
		if typed, ok := value.(T); ok {
			return typed, nil
		}
	}
	lock := cache.loader(key)
	lock.Lock()
	defer lock.Unlock()
	if value, ok := cache.lookup(key); ok {
		if typed, ok := value.(T); ok {
			return typed, nil
		}
	}
	value, err := load(ctx)
	if err != nil {
		var zero T
		log.Printf("build %s: %v", key, err)
		return zero, fiber.NewError(fiber.StatusServiceUnavailable, "status is temporarily unavailable")
	}
	cache.store(key, value)
	return value, nil
}
