package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

var schemaPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Store struct {
	db       *pgxpool.Pool
	cache    *redis.Client
	tailSize int64
}

func New(ctx context.Context, databaseURL, redisURL string, tailSize int) (*Store, error) {
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

	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	cache := redis.NewClient(redisOptions)
	if err := cache.Ping(ctx).Err(); err != nil {
		db.Close()
		_ = cache.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Store{db: db, cache: cache, tailSize: int64(tailSize)}, nil
}

func (s *Store) Close() {
	s.db.Close()
	_ = s.cache.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.PingPostgres(ctx); err != nil {
		return err
	}
	return s.PingRedis(ctx)
}

// PingPostgres and PingRedis are split out so the health monitor can report
// the shared database and cache as separate components on the status page.
func (s *Store) PingPostgres(ctx context.Context) error { return s.db.Ping(ctx) }
func (s *Store) PingRedis(ctx context.Context) error    { return s.cache.Ping(ctx).Err() }

func (s *Store) InsertLog(ctx context.Context, input model.NewLog) (model.Log, error) {
	var entry model.Log
	var id int64
	var level string
	var payload []byte
	err := s.db.QueryRow(ctx, `
		INSERT INTO logs (service, level, message, payload)
		VALUES ($1, $2::log_level, $3, $4::jsonb)
		RETURNING id, service, level::text, message, payload, created_at
	`, input.Service, string(input.Level), input.Message, nullableJSON(input.Payload)).Scan(
		&id, &entry.Service, &level, &entry.Message, &payload, &entry.CreatedAt,
	)
	if err != nil {
		return model.Log{}, fmt.Errorf("insert log: %w", err)
	}
	entry.ID = strconv.FormatInt(id, 10)
	entry.Level = model.LogLevel(level)
	entry.Payload = payload

	if err := s.cacheLog(ctx, entry); err != nil {
		// Postgres is the warehouse and remains authoritative. A cache failure must
		// not encourage a client retry that would duplicate the durable row.
		log.Printf("logger cache write for %s: %v", entry.Service, err)
	}
	return entry, nil
}

func (s *Store) RecentLogs(ctx context.Context, services []string, levels []model.LogLevel, search string, limit int) ([]model.Log, error) {
	entries := make([]model.Log, 0, limit)
	cacheComplete := true
	for _, service := range services {
		rows, err := s.cache.LRange(ctx, logKey(service), 0, s.tailSize-1).Result()
		if err != nil {
			cacheComplete = false
			break
		}
		for _, raw := range rows {
			var entry model.Log
			if err := json.Unmarshal([]byte(raw), &entry); err != nil {
				cacheComplete = false
				break
			}
			if matches(entry, levels, search) {
				entries = append(entries, entry)
			}
		}
		if !cacheComplete {
			break
		}
	}

	if !cacheComplete || len(entries) == 0 {
		page, err := s.QueryLogs(ctx, model.HistoryQuery{
			Services: services,
			Levels:   levels,
			Search:   search,
			Limit:    limit,
		})
		if err != nil {
			return nil, err
		}
		return page.Logs, nil
	}

	sortLogs(entries)
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (s *Store) QueryLogs(ctx context.Context, query model.HistoryQuery) (model.LogPage, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}

	args := []any{query.Services}
	clauses := []string{"service = ANY($1)"}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	if len(query.Levels) > 0 {
		levels := make([]string, 0, len(query.Levels))
		for _, level := range query.Levels {
			levels = append(levels, string(level))
		}
		add("level::text = ANY($%d)", levels)
	}
	if query.From != nil {
		add("created_at >= $%d", *query.From)
	}
	if query.To != nil {
		add("created_at <= $%d", *query.To)
	}
	if query.Search != "" {
		args = append(args, "%"+query.Search+"%")
		position := len(args)
		clauses = append(clauses, fmt.Sprintf("(message ILIKE $%d OR COALESCE(payload::text, '') ILIKE $%d)", position, position))
	}
	if query.Cursor != "" {
		cursor, err := decodeCursor(query.Cursor)
		if err != nil {
			return model.LogPage{}, err
		}
		args = append(args, cursor.CreatedAt, cursor.ID)
		clauses = append(clauses, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}

	args = append(args, query.Limit+1)
	statement := fmt.Sprintf(`
		SELECT id, service, level::text, message, payload, created_at
		FROM logs
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d
	`, strings.Join(clauses, " AND "), len(args))

	rows, err := s.db.Query(ctx, statement, args...)
	if err != nil {
		return model.LogPage{}, fmt.Errorf("query logs: %w", err)
	}
	defer rows.Close()

	entries := make([]model.Log, 0, query.Limit+1)
	ids := make([]int64, 0, query.Limit+1)
	for rows.Next() {
		var entry model.Log
		var id int64
		var level string
		var payload []byte
		if err := rows.Scan(&id, &entry.Service, &level, &entry.Message, &payload, &entry.CreatedAt); err != nil {
			return model.LogPage{}, fmt.Errorf("scan log: %w", err)
		}
		entry.ID = strconv.FormatInt(id, 10)
		entry.Level = model.LogLevel(level)
		entry.Payload = payload
		entries = append(entries, entry)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return model.LogPage{}, fmt.Errorf("iterate logs: %w", err)
	}

	page := model.LogPage{Logs: entries}
	if len(entries) > query.Limit {
		last := entries[query.Limit-1]
		page.NextCursor = encodeCursor(logCursor{CreatedAt: last.CreatedAt, ID: ids[query.Limit-1]})
		page.Logs = entries[:query.Limit]
	}
	return page, nil
}

func (s *Store) SetLatestHealth(ctx context.Context, health model.Health) error {
	raw, err := json.Marshal(health)
	if err != nil {
		return err
	}
	return s.cache.Set(ctx, healthKey(health.Service), raw, 0).Err()
}

func (s *Store) InsertHealth(ctx context.Context, health model.Health) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO health_checks (service, ok, detail, checked_at)
		VALUES ($1, $2, $3, $4)
	`, health.Service, health.OK, nullableString(health.Detail), health.CheckedAt)
	if err != nil {
		return fmt.Errorf("insert health check: %w", err)
	}
	return nil
}

func (s *Store) LatestHealth(ctx context.Context, services []string) (map[string]model.Health, error) {
	result := make(map[string]model.Health, len(services))
	keys := make([]string, 0, len(services))
	for _, service := range services {
		keys = append(keys, healthKey(service))
	}

	values, cacheErr := s.cache.MGet(ctx, keys...).Result()
	if cacheErr == nil {
		for i, value := range values {
			raw, ok := value.(string)
			if !ok {
				continue
			}
			var health model.Health
			if json.Unmarshal([]byte(raw), &health) == nil {
				result[services[i]] = health
			}
		}
	}

	missing := make([]string, 0, len(services))
	for _, service := range services {
		if _, ok := result[service]; !ok {
			missing = append(missing, service)
		}
	}
	if len(missing) == 0 {
		return result, nil
	}

	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (service) service, ok, COALESCE(detail, ''), checked_at
		FROM health_checks
		WHERE service = ANY($1)
		ORDER BY service, checked_at DESC
	`, missing)
	if err != nil {
		return nil, fmt.Errorf("query latest health: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var health model.Health
		if err := rows.Scan(&health.Service, &health.OK, &health.Detail, &health.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan latest health: %w", err)
		}
		result[health.Service] = health
		_ = s.SetLatestHealth(ctx, health)
	}
	return result, rows.Err()
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
	rows, err := s.db.Query(ctx, `
		WITH window_rows AS (
			SELECT service, ok, checked_at,
				lag(ok) OVER w AS previous_ok,
				lag(checked_at) OVER w AS previous_at,
				lead(checked_at) OVER w AS next_at
			FROM health_checks
			WHERE service = ANY($1) AND checked_at > $2 AND checked_at <= $3
			WINDOW w AS (PARTITION BY service ORDER BY checked_at)
		)
		(
			SELECT DISTINCT ON (service) service, ok, checked_at, false AS continuous
			FROM health_checks
			WHERE service = ANY($1) AND checked_at <= $2
			ORDER BY service, checked_at DESC
		)
		UNION ALL
		(
			SELECT service, ok, checked_at,
				next_at IS NOT NULL AND next_at - checked_at <= $4 AS continuous
			FROM window_rows
			WHERE previous_ok IS DISTINCT FROM ok
				OR checked_at - previous_at > $4
				OR next_at - checked_at > $4
				OR next_at IS NULL
		)
		ORDER BY service, checked_at
	`, services, from, to, interval(gapTolerance))
	if err != nil {
		return nil, fmt.Errorf("query health history: %w", err)
	}
	defer rows.Close()

	history := make(map[string][]model.HealthPoint, len(services))
	for rows.Next() {
		var service string
		var point model.HealthPoint
		if err := rows.Scan(&service, &point.OK, &point.CheckedAt, &point.Continuous); err != nil {
			return nil, fmt.Errorf("scan health history: %w", err)
		}
		history[service] = append(history[service], point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate health history: %w", err)
	}
	return history, nil
}

func (s *Store) cacheLog(ctx context.Context, entry model.Log) error {
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = s.cache.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, logKey(entry.Service), raw)
		pipe.LTrim(ctx, logKey(entry.Service), 0, s.tailSize-1)
		return nil
	})
	return err
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

type logCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

func encodeCursor(cursor logCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(encoded string) (logCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return logCursor{}, errors.New("invalid cursor")
	}
	var cursor logCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.ID < 1 || cursor.CreatedAt.IsZero() {
		return logCursor{}, errors.New("invalid cursor")
	}
	return cursor, nil
}

func matches(entry model.Log, levels []model.LogLevel, search string) bool {
	if len(levels) > 0 {
		matched := false
		for _, level := range levels {
			if entry.Level == level {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if search == "" {
		return true
	}
	needle := strings.ToLower(search)
	return strings.Contains(strings.ToLower(entry.Message), needle) || strings.Contains(strings.ToLower(string(entry.Payload)), needle)
}

func sortLogs(entries []model.Log) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].CreatedAt.After(entries[j-1].CreatedAt); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// interval encodes a Go duration as a Postgres interval. Comparing intervals
// directly is markedly cheaper than EXTRACT(EPOCH ...), which yields numeric and
// then pays numeric comparison on every one of a quarter of a million rows.
func interval(duration time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: duration.Microseconds(), Valid: true}
}

func logKey(service string) string    { return "logger:logs:" + service }
func healthKey(service string) string { return "logger:health:" + service }
