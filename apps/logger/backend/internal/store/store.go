package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
	"github.com/BioTronDesignTeam/Logger/backend/internal/store/queries"
)

var schemaPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Store keeps health history in Postgres and the latest reading of each
// component in memory. The monitor and the API run in this one process, so the
// monitor's last reading is all the status page needs; Postgres answers only
// for a component not yet probed since this process started.
type Store struct {
	db      *pgxpool.Pool
	queries *queries.Queries

	mu     sync.RWMutex
	latest map[string]model.Health
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	databaseConfig, err := databaseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.NewWithConfig(ctx, databaseConfig)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{db: db, queries: queries.New(db), latest: make(map[string]model.Health)}, nil
}

func (s *Store) Close() { s.db.Close() }

// Ping answers /health: Logger is up when its database answers.
func (s *Store) Ping(ctx context.Context) error { return s.PingPostgres(ctx) }

// PingPostgres lets the health monitor report the shared database as its own
// component on the status page.
func (s *Store) PingPostgres(ctx context.Context) error { return s.db.Ping(ctx) }

// SetLatestHealth remembers the monitor's newest reading of a component.
func (s *Store) SetLatestHealth(health model.Health) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest[health.Service] = health
}

func (s *Store) InsertHealth(ctx context.Context, health model.Health) error {
	err := s.queries.InsertHealth(ctx, queries.InsertHealthParams{
		Service:   health.Service,
		Ok:        health.OK,
		Detail:    nullableString(health.Detail),
		CheckedAt: health.CheckedAt,
	})
	if err != nil {
		return fmt.Errorf("insert health check: %w", err)
	}
	return nil
}

// DeleteHealthBefore deletes the history rows checked before cutoff and
// returns how many went.
func (s *Store) DeleteHealthBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	deleted, err := s.queries.DeleteHealthBefore(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old health checks: %w", err)
	}
	return deleted, nil
}

// LatestHealth answers from memory, and from Postgres for any component the
// monitor has not probed since this process started.
func (s *Store) LatestHealth(ctx context.Context, services []string) (map[string]model.Health, error) {
	result := make(map[string]model.Health, len(services))
	missing := make([]string, 0, len(services))
	s.mu.RLock()
	for _, service := range services {
		if health, ok := s.latest[service]; ok {
			result[service] = health
		} else {
			missing = append(missing, service)
		}
	}
	s.mu.RUnlock()
	if len(missing) == 0 {
		return result, nil
	}

	rows, err := s.queries.LatestHealth(ctx, missing)
	if err != nil {
		return nil, fmt.Errorf("query latest health: %w", err)
	}
	for _, row := range rows {
		result[row.Service] = model.Health{Service: row.Service, OK: row.Ok, Detail: row.Detail, CheckedAt: row.CheckedAt}
	}
	return result, nil
}

// HealthHistory returns the health observations needed to walk uptime across
// [from, to], collapsed server-side.
//
// Ninety days of five-minute heartbeats is roughly 26,000 rows per component.
// Streaming all of them back sorted a quarter of a million rows to disk on every
// cache miss, so the in-range branch keeps only the rows that carry information:
// a state transition, the row that starts a silence, the row that ends one, and
// the last row in the window. Everything between two kept rows was, by
// construction, an unbroken run of one state.
//
// That collapse would be lossy on its own. The walker judges a silence by how
// long a segment lasts, so a run of 26,000 identical heartbeats reduced to its
// two endpoints would read as a ninety-day silence and score as unknown rather
// than as fully up. The `continuous` column carries the missing evidence: it is
// true when observation ran on from that row to the next kept row without a
// break wider than the tolerance.
//
// gapTolerance MUST be the same value the caller later passes to the uptime
// walk as its maximum gap. The SQL decides which rows may be dropped and sets
// `continuous` using this tolerance, and the walk trusts that flag; if the two
// drift apart, a stretch the walk would have called unknown arrives already
// marked as observed and the silence disappears without trace. The API layer
// passes its single maxGap field to both, which is what keeps them in step.
//
// The carry-in row is unchanged: for each service, the single most recent
// observation at or before `from` is the state the component was already in when
// the window opened. It is never marked continuous, because the distance to the
// first in-window row is a real observation gap and the walk should judge it by
// its length.
//
// Detail is deliberately not selected. It carries raw dial errors and internal
// hostnames, and its only consumer here is a public endpoint.
func (s *Store) HealthHistory(ctx context.Context, services []string, from, to time.Time, gapTolerance time.Duration) (map[string][]model.HealthPoint, error) {
	rows, err := s.queries.HealthHistory(ctx, queries.HealthHistoryParams{
		Services:     services,
		WindowStart:  from,
		WindowEnd:    to,
		GapTolerance: interval(gapTolerance),
	})
	if err != nil {
		return nil, fmt.Errorf("query health history: %w", err)
	}

	history := make(map[string][]model.HealthPoint, len(services))
	for _, row := range rows {
		history[row.Service] = append(history[row.Service], model.HealthPoint{
			OK:         row.Ok,
			CheckedAt:  row.CheckedAt,
			Continuous: row.Continuous,
		})
	}
	return history, nil
}

func databaseConfig(raw string) (*pgxpool.Config, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	query := u.Query()
	schema := query.Get("schema")
	if schema == "" {
		schema = "logger"
	}
	if !schemaPattern.MatchString(schema) {
		return nil, errors.New("database schema contains invalid characters")
	}
	query.Del("schema")
	u.RawQuery = query.Encode()

	config, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	config.ConnConfig.RuntimeParams["timezone"] = "America/Toronto"
	return config, nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// interval encodes a Go duration as a Postgres interval. Comparing intervals
// directly is markedly cheaper than EXTRACT(EPOCH ...), which yields numeric and
// then pays numeric comparison on every one of a quarter of a million rows.
func interval(duration time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: duration.Microseconds(), Valid: true}
}
