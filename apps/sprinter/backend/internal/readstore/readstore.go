// Package readstore is the second Postgres pool: the one the agent's tools
// read through.
//
// It connects as a role with SELECT on the logger schema and on four oauth
// tables and nothing else. That is the containment. A model can be talked into
// asking for anything, so the answer to "what stops it writing" is the role's
// grants, not the wording of a prompt. Every query here names its schema,
// because this pool has no search_path of its own.
package readstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Timeout bounds one query. A tool call sits inside a Discord interaction, so
// a slow scan has to give up long before the interaction does.
const Timeout = 10 * time.Second

type Store struct {
	pool *pgxpool.Pool
}

// New opens the pool and proves the role can reach the database. A read URL
// that names a role without the grants still connects; the tools report the
// permission error when they run, which is the right place to see it.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse read database config: %w", err)
	}
	config.ConnConfig.RuntimeParams["timezone"] = "America/Toronto"
	// The tools qualify every table, so the pool needs no search_path. Saying
	// so out loud keeps a later `?schema=` in the URL from looking meaningful.
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// --- Logs -------------------------------------------------------------------

// LogRow is one row of logger.logs. Payload stays raw so a tool can truncate
// it without first understanding it.
type LogRow struct {
	ID        int64
	Service   string
	Level     string
	Message   string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// LogQuery is what both log tools filter by. An empty field is no filter.
type LogQuery struct {
	Service string
	Level   string
	Search  string
	From    *time.Time
	To      *time.Time
	Cursor  string
	Limit   int
}

// RecentLogs answers the newest rows that match, with no paging.
func (s *Store) RecentLogs(ctx context.Context, query LogQuery) ([]LogRow, error) {
	rows, _, err := s.queryLogs(ctx, query, false)
	return rows, err
}

// LogHistory answers a page and the cursor for the next one. It paginates on
// (created_at, id) exactly as Logger's own history does, so a cursor means the
// same thing in both places and a row inserted mid-walk cannot shift the page.
func (s *Store) LogHistory(ctx context.Context, query LogQuery) ([]LogRow, string, error) {
	return s.queryLogs(ctx, query, true)
}

func (s *Store) queryLogs(ctx context.Context, query LogQuery, paged bool) ([]LogRow, string, error) {
	var args []any
	var clauses []string
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if query.Service != "" {
		add("service = $%d", query.Service)
	}
	if query.Level != "" {
		add("level::text = $%d", query.Level)
	}
	if query.Search != "" {
		add("message ILIKE $%d", "%"+escapeLike(query.Search)+"%")
	}
	if query.From != nil {
		add("created_at >= $%d", *query.From)
	}
	if query.To != nil {
		add("created_at <= $%d", *query.To)
	}
	if query.Cursor != "" {
		cursor, err := DecodeCursor(query.Cursor)
		if err != nil {
			return nil, "", err
		}
		args = append(args, cursor.CreatedAt, cursor.ID)
		clauses = append(clauses, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	where := "TRUE"
	if len(clauses) > 0 {
		where = strings.Join(clauses, " AND ")
	}

	// One row past the limit is how the walk learns there is another page
	// without a second count query.
	fetch := query.Limit
	if paged {
		fetch++
	}
	args = append(args, fetch)
	statement := fmt.Sprintf(`
		SELECT id, service, level::text, message, payload, created_at
		FROM logger.logs
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d
	`, where, len(args))

	found, err := s.pool.Query(ctx, statement, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query logs: %w", err)
	}
	defer found.Close()
	logs := make([]LogRow, 0, query.Limit)
	for found.Next() {
		var row LogRow
		var payload []byte
		if err := found.Scan(&row.ID, &row.Service, &row.Level, &row.Message,
			&payload, &row.CreatedAt); err != nil {
			return nil, "", err
		}
		row.Payload = payload
		logs = append(logs, row)
	}
	if err := found.Err(); err != nil {
		return nil, "", err
	}
	if paged && len(logs) > query.Limit {
		last := logs[query.Limit-1]
		return logs[:query.Limit], EncodeCursor(Cursor{CreatedAt: last.CreatedAt, ID: last.ID}), nil
	}
	return logs, "", nil
}

// --- Health -----------------------------------------------------------------

// HealthRow is one recorded check. Detail carries the dial error, which is the
// point of asking Sprinter rather than reading the public status page.
type HealthRow struct {
	OK        bool
	Detail    string
	CheckedAt time.Time
}

func (s *Store) HealthHistory(ctx context.Context, service string, since time.Time, limit int) ([]HealthRow, error) {
	found, err := s.pool.Query(ctx, `
		SELECT ok, COALESCE(detail, ''), checked_at
		FROM logger.health_checks
		WHERE service = $1 AND checked_at >= $2
		ORDER BY checked_at DESC, id DESC
		LIMIT $3
	`, service, since, limit)
	if err != nil {
		return nil, fmt.Errorf("query health checks: %w", err)
	}
	defer found.Close()
	checks := make([]HealthRow, 0, limit)
	for found.Next() {
		var row HealthRow
		if err := found.Scan(&row.OK, &row.Detail, &row.CheckedAt); err != nil {
			return nil, err
		}
		checks = append(checks, row)
	}
	return checks, found.Err()
}

// --- Access -----------------------------------------------------------------

// GrantRow is one operator's one permission on one app.
type GrantRow struct {
	Login         string
	Name          string
	PermissionKey string
	Label         string
	IsManager     bool
	IsSuperuser   bool
	IsBanned      bool
	GrantedAt     time.Time
}

// OperatorRow is an operator named for a reason other than a grant: a manager
// or a superuser, who is admitted everywhere without one.
type OperatorRow struct {
	Login       string
	Name        string
	IsManager   bool
	IsSuperuser bool
	IsBanned    bool
}

// AccessReport is who may use one app. Grants are the explicit permissions;
// Managers and Superusers are admitted by their flag alone, so an answer that
// listed only the grants would be wrong.
type AccessReport struct {
	AppID       string
	AppName     string
	AppFound    bool
	Permissions []PermissionRow
	Grants      []GrantRow
	Managers    []OperatorRow
	Superusers  []OperatorRow
}

// PermissionRow is one key an app defines, whether or not anybody holds it.
type PermissionRow struct {
	Key         string
	Label       string
	Description string
}

func (s *Store) Access(ctx context.Context, appID string) (AccessReport, error) {
	report := AccessReport{AppID: appID}
	err := s.pool.QueryRow(ctx,
		`SELECT name FROM oauth.apps WHERE id = $1`, appID).Scan(&report.AppName)
	switch {
	case err == nil:
		report.AppFound = true
	case errors.Is(err, pgx.ErrNoRows):
		// A missing app is an answer, not a failure: the model asked about
		// something that is not registered, and should be told so.
	default:
		return AccessReport{}, fmt.Errorf("read app: %w", err)
	}

	if report.Permissions, err = s.permissions(ctx, appID); err != nil {
		return AccessReport{}, err
	}
	if report.Grants, err = s.grants(ctx, appID); err != nil {
		return AccessReport{}, err
	}
	managers, superusers, err := s.privilegedOperators(ctx)
	if err != nil {
		return AccessReport{}, err
	}
	report.Managers, report.Superusers = managers, superusers
	return report, nil
}

func (s *Store) permissions(ctx context.Context, appID string) ([]PermissionRow, error) {
	found, err := s.pool.Query(ctx, `
		SELECT key, label, description
		FROM oauth.permissions WHERE app_id = $1 ORDER BY key
	`, appID)
	if err != nil {
		return nil, fmt.Errorf("query permissions: %w", err)
	}
	defer found.Close()
	permissions := []PermissionRow{}
	for found.Next() {
		var row PermissionRow
		if err := found.Scan(&row.Key, &row.Label, &row.Description); err != nil {
			return nil, err
		}
		permissions = append(permissions, row)
	}
	return permissions, found.Err()
}

func (s *Store) grants(ctx context.Context, appID string) ([]GrantRow, error) {
	// The permission row is joined outward: a grant whose catalog entry was
	// deleted still admits the operator, so hiding it would understate access.
	found, err := s.pool.Query(ctx, `
		SELECT o.login, COALESCE(o.name, ''), g.permission_key,
		       COALESCE(p.label, ''), o.is_manager, o.is_superuser, o.is_banned,
		       g.created_at
		FROM oauth.grants g
		JOIN oauth.operators o ON o.github_id = g.operator_id
		LEFT JOIN oauth.permissions p
		       ON p.app_id = g.app_id AND p.key = g.permission_key
		WHERE g.app_id = $1
		ORDER BY lower(o.login), g.permission_key
	`, appID)
	if err != nil {
		return nil, fmt.Errorf("query grants: %w", err)
	}
	defer found.Close()
	grants := []GrantRow{}
	for found.Next() {
		var row GrantRow
		if err := found.Scan(&row.Login, &row.Name, &row.PermissionKey, &row.Label,
			&row.IsManager, &row.IsSuperuser, &row.IsBanned, &row.GrantedAt); err != nil {
			return nil, err
		}
		grants = append(grants, row)
	}
	return grants, found.Err()
}

func (s *Store) privilegedOperators(ctx context.Context) ([]OperatorRow, []OperatorRow, error) {
	found, err := s.pool.Query(ctx, `
		SELECT login, COALESCE(name, ''), is_manager, is_superuser, is_banned
		FROM oauth.operators
		WHERE is_manager OR is_superuser
		ORDER BY lower(login)
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("query privileged operators: %w", err)
	}
	defer found.Close()
	managers, superusers := []OperatorRow{}, []OperatorRow{}
	for found.Next() {
		var row OperatorRow
		if err := found.Scan(&row.Login, &row.Name, &row.IsManager,
			&row.IsSuperuser, &row.IsBanned); err != nil {
			return nil, nil, err
		}
		if row.IsSuperuser {
			superusers = append(superusers, row)
		}
		if row.IsManager {
			managers = append(managers, row)
		}
	}
	return managers, superusers, found.Err()
}

// --- Cursors ----------------------------------------------------------------

// Cursor is the keyset position: the last row a page returned.
type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

func EncodeCursor(cursor Cursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeCursor rejects anything it did not write. The cursor reaches this
// package through a model, so it is untrusted input like any other argument.
func DecodeCursor(encoded string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Cursor{}, errors.New("invalid cursor")
	}
	var cursor Cursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.ID < 1 || cursor.CreatedAt.IsZero() {
		return Cursor{}, errors.New("invalid cursor")
	}
	return cursor, nil
}

// escapeLike keeps a search string from being read as a pattern. Without it a
// question containing a percent sign quietly matches everything.
func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
