// Package readstore is the second Postgres pool: the one the agent's tools
// read through.
//
// It connects as a role with SELECT on the logger schema and on four oauth
// tables and nothing else. That is the containment. A model can be talked into
// asking for anything, so the answer to "what stops it writing" is the role's
// grants, not the wording of a prompt. Every query names its schema, because
// this pool has no search_path of its own. The SQL lives in
// queries/queries.sql and sqlc makes it into Go (sqlc.yaml).
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

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/readstore/queries"
)

// Timeout bounds one query. A tool call sits inside a Discord interaction, so
// a slow scan has to give up long before the interaction does.
const Timeout = 10 * time.Second

type Store struct {
	pool *pgxpool.Pool
	q    *queries.Queries
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
	return &Store{pool: pool, q: queries.New(pool)}, nil
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
	// An empty filter is a null argument, which the query reads as no filter.
	params := queries.LogsParams{
		Service:  optional(query.Service),
		Level:    optional(query.Level),
		FromTime: query.From,
		ToTime:   query.To,
	}
	if query.Search != "" {
		pattern := "%" + escapeLike(query.Search) + "%"
		params.Search = &pattern
	}
	if query.Cursor != "" {
		cursor, err := DecodeCursor(query.Cursor)
		if err != nil {
			return nil, "", err
		}
		params.CursorCreatedAt, params.CursorID = &cursor.CreatedAt, &cursor.ID
	}

	// One row past the limit is how the walk learns there is another page
	// without a second count query.
	fetch := query.Limit
	if paged {
		fetch++
	}
	params.RowLimit = int32(fetch)

	found, err := s.q.Logs(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("query logs: %w", err)
	}
	logs := make([]LogRow, 0, len(found))
	for _, row := range found {
		logs = append(logs, LogRow{
			ID: row.ID, Service: row.Service, Level: row.Level,
			Message: row.Message, Payload: row.Payload, CreatedAt: row.CreatedAt,
		})
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
	found, err := s.q.HealthHistory(ctx, queries.HealthHistoryParams{
		Service: service, Since: since, RowLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("query health checks: %w", err)
	}
	checks := make([]HealthRow, 0, len(found))
	for _, row := range found {
		checks = append(checks, HealthRow{OK: row.Ok, Detail: row.Detail, CheckedAt: row.CheckedAt})
	}
	return checks, nil
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
	name, err := s.q.AppName(ctx, appID)
	switch {
	case err == nil:
		report.AppName, report.AppFound = name, true
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

// accessRowLimit bounds each of the three access queries. An app with more
// than five hundred grants, or a guild with more than five hundred managers,
// is not a report anybody reads; without the cap one such row set would be
// pulled into memory in full and then cut in the tool's formatter anyway.
const accessRowLimit = 500

func (s *Store) permissions(ctx context.Context, appID string) ([]PermissionRow, error) {
	found, err := s.q.Permissions(ctx, queries.PermissionsParams{AppID: appID, RowLimit: accessRowLimit})
	if err != nil {
		return nil, fmt.Errorf("query permissions: %w", err)
	}
	permissions := make([]PermissionRow, 0, len(found))
	for _, row := range found {
		permissions = append(permissions, PermissionRow(row))
	}
	return permissions, nil
}

func (s *Store) grants(ctx context.Context, appID string) ([]GrantRow, error) {
	found, err := s.q.Grants(ctx, queries.GrantsParams{AppID: appID, RowLimit: accessRowLimit})
	if err != nil {
		return nil, fmt.Errorf("query grants: %w", err)
	}
	grants := make([]GrantRow, 0, len(found))
	for _, row := range found {
		grants = append(grants, GrantRow{
			Login: row.Login, Name: row.Name, PermissionKey: row.PermissionKey,
			Label: row.Label, IsManager: row.IsManager, IsSuperuser: row.IsSuperuser,
			IsBanned: row.IsBanned, GrantedAt: row.CreatedAt,
		})
	}
	return grants, nil
}

func (s *Store) privilegedOperators(ctx context.Context) ([]OperatorRow, []OperatorRow, error) {
	found, err := s.q.PrivilegedOperators(ctx, accessRowLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("query privileged operators: %w", err)
	}
	managers, superusers := []OperatorRow{}, []OperatorRow{}
	for _, row := range found {
		operator := OperatorRow(row)
		if operator.IsSuperuser {
			superusers = append(superusers, operator)
		}
		if operator.IsManager {
			managers = append(managers, operator)
		}
	}
	return managers, superusers, nil
}

// optional is the null that means "no filter" for an empty string.
func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
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
