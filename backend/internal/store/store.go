package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrBanned = errors.New("banned")

// GuestGitHubID is the sentinel operator for daily-key guest sessions.
// Real GitHub numeric ids are >= 1.
const GuestGitHubID int64 = 0
const GuestLogin = "guest"

type Store struct {
	pool *pgxpool.Pool
}

var schemaPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func New(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := databaseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
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

type Operator struct {
	GitHubID    int64
	Login       string
	Name        string
	AvatarURL   string
	IsSuperuser bool
	IsManager   bool
	IsBanned    bool
}

type SessionOperator struct {
	GitHubID    int64  `json:"github_id"`
	Login       string `json:"login"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	IsSuperuser bool   `json:"is_superuser"`
	IsManager   bool   `json:"is_manager"`
	IsBanned    bool   `json:"is_banned"`
	GuestAppID  string `json:"guest_app_id,omitempty"`
}

type App struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	DailyKeyEnabled bool   `json:"daily_key_enabled"`
}

type Permission struct {
	AppID       string `json:"app_id"`
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Grant struct {
	OperatorID    int64     `json:"operator_id"`
	AppID         string    `json:"app_id"`
	PermissionKey string    `json:"permission_key"`
	CreatedAt     time.Time `json:"created_at"`
}

type AccessRequest struct {
	ID              string     `json:"id"`
	RequesterID     int64      `json:"requester_id"`
	RequesterLogin  string     `json:"requester_login"`
	AppID           string     `json:"app_id"`
	AppName         string     `json:"app_name"`
	PermissionKey   string     `json:"permission_key"`
	PermissionLabel string     `json:"permission_label"`
	Status          string     `json:"status"`
	ReviewedBy      *int64     `json:"reviewed_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
}

type OrgMember struct {
	GitHubID    int64     `json:"github_id"`
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	AvatarURL   string    `json:"avatar_url"`
	IsSuperuser bool      `json:"is_superuser"`
	IsManager   bool      `json:"is_manager"`
	IsBanned    bool      `json:"is_banned"`
	LastLoginAt time.Time `json:"last_login_at"`
}

func (op *SessionOperator) IsStaff() bool {
	return op.IsSuperuser || op.IsManager
}

func (op *SessionOperator) IsGuest() bool {
	return op.GitHubID == GuestGitHubID
}

func (s *Store) UpsertOperator(ctx context.Context, op Operator, forceSuperuser bool) error {
	// SUPERUSER_GITHUB_IDS is authoritative on every login: listed → superuser,
	// omitted → cleared. Managers are independent (org UI / is_manager).
	_, err := s.pool.Exec(ctx, `
		INSERT INTO operators (github_id, login, name, avatar_url, is_superuser, last_login_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (github_id) DO UPDATE
		SET login = EXCLUDED.login,
		    name = EXCLUDED.name,
		    avatar_url = EXCLUDED.avatar_url,
		    last_login_at = now(),
		    is_superuser = EXCLUDED.is_superuser
	`, op.GitHubID, op.Login, nullable(op.Name), nullable(op.AvatarURL), forceSuperuser)
	return err
}

func (s *Store) CreateSession(ctx context.Context, idHash string, githubID int64, guestAppID string, expiresAt time.Time, userAgent string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, operator_id, guest_app_id, expires_at, user_agent)
		VALUES ($1, $2, $3, $4, $5)
	`, idHash, githubID, nullable(guestAppID), expiresAt, nullable(userAgent))
	return err
}

func (s *Store) GetSessionOperator(ctx context.Context, idHash string) (*SessionOperator, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT o.github_id, o.login, COALESCE(o.name, ''), COALESCE(o.avatar_url, ''),
		       o.is_superuser, o.is_manager, o.is_banned, COALESCE(s.guest_app_id, '')
		FROM sessions s
		JOIN operators o ON o.github_id = s.operator_id
		WHERE s.id = $1 AND s.expires_at > now()
	`, idHash)
	var so SessionOperator
	if err := row.Scan(&so.GitHubID, &so.Login, &so.Name, &so.AvatarURL,
		&so.IsSuperuser, &so.IsManager, &so.IsBanned, &so.GuestAppID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if so.IsBanned {
		return nil, ErrBanned
	}
	return &so, nil
}

func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, idHash)
	return err
}

func (s *Store) DeleteSessionsForOperator(ctx context.Context, operatorID int64) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE operator_id = $1`, operatorID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) GetOperator(ctx context.Context, githubID int64) (*OrgMember, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT github_id, login, COALESCE(name, ''), COALESCE(avatar_url, ''),
		       is_superuser, is_manager, is_banned, last_login_at
		FROM operators WHERE github_id = $1
	`, githubID)
	var m OrgMember
	if err := row.Scan(&m.GitHubID, &m.Login, &m.Name, &m.AvatarURL,
		&m.IsSuperuser, &m.IsManager, &m.IsBanned, &m.LastLoginAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (s *Store) ListOrgMembers(ctx context.Context) ([]OrgMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT github_id, login, COALESCE(name, ''), COALESCE(avatar_url, ''),
		       is_superuser, is_manager, is_banned, last_login_at
		FROM operators
		WHERE github_id <> $1
		ORDER BY login
	`, GuestGitHubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgMember
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.GitHubID, &m.Login, &m.Name, &m.AvatarURL,
			&m.IsSuperuser, &m.IsManager, &m.IsBanned, &m.LastLoginAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SetBanned(ctx context.Context, githubID int64, banned bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE operators SET is_banned = $2 WHERE github_id = $1`, githubID, banned)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetManager(ctx context.Context, githubID int64, manager bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE operators SET is_manager = $2 WHERE github_id = $1`, githubID, manager)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListApps(ctx context.Context) ([]App, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, description, daily_key_enabled FROM apps ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []App
	for rows.Next() {
		var a App
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.DailyKeyEnabled); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT app_id, key, label, description FROM permissions ORDER BY app_id, key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.AppID, &p.Key, &p.Label, &p.Description); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPermission(ctx context.Context, appID, key string) (*Permission, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT app_id, key, label, description FROM permissions WHERE app_id = $1 AND key = $2
	`, appID, key)
	var p Permission
	if err := row.Scan(&p.AppID, &p.Key, &p.Label, &p.Description); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateApp(ctx context.Context, id, name, description string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO apps (id, name, description) VALUES ($1, $2, $3)`, id, name, description)
	return err
}

func (s *Store) ListGrantsForOperator(ctx context.Context, operatorID int64) ([]Grant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT operator_id, app_id, permission_key, created_at
		FROM grants WHERE operator_id = $1
		ORDER BY app_id, permission_key
	`, operatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.OperatorID, &g.AppID, &g.PermissionKey, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) HasGrant(ctx context.Context, operatorID int64, appID, permissionKey string) (bool, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT 1 FROM grants WHERE operator_id = $1 AND app_id = $2 AND permission_key = $3
	`, operatorID, appID, permissionKey)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Allowed reports whether op may use permissionKey on appID.
// Superusers and managers are allowed everywhere; members need a grant.
func (s *Store) Allowed(ctx context.Context, op *SessionOperator, appID, permissionKey string) (bool, error) {
	if op.IsSuperuser || op.IsManager {
		return true, nil
	}
	if op.IsGuest() {
		return op.GuestAppID == appID && permissionKey == "view", nil
	}
	return s.HasGrant(ctx, op.GitHubID, appID, permissionKey)
}

func (s *Store) CreateGrant(ctx context.Context, operatorID int64, appID, permissionKey string) error {
	if _, err := s.GetPermission(ctx, appID, permissionKey); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO grants (operator_id, app_id, permission_key)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, operatorID, appID, permissionKey)
	return err
}

func (s *Store) DeleteGrant(ctx context.Context, operatorID int64, appID, permissionKey string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM grants WHERE operator_id = $1 AND app_id = $2 AND permission_key = $3
	`, operatorID, appID, permissionKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAllGrants(ctx context.Context, operatorID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM grants WHERE operator_id = $1`, operatorID)
	return err
}

func (s *Store) CreateAccessRequest(ctx context.Context, id string, requesterID int64, appID, permissionKey string) error {
	if _, err := s.GetPermission(ctx, appID, permissionKey); err != nil {
		return err
	}
	has, err := s.HasGrant(ctx, requesterID, appID, permissionKey)
	if err != nil {
		return err
	}
	if has {
		return fmt.Errorf("%w: already granted", ErrConflict)
	}
	row := s.pool.QueryRow(ctx, `
		SELECT 1 FROM access_requests
		WHERE requester_id = $1 AND app_id = $2 AND permission_key = $3 AND status = 'pending'
	`, requesterID, appID, permissionKey)
	var one int
	if err := row.Scan(&one); err == nil {
		return fmt.Errorf("%w: pending request exists", ErrConflict)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO access_requests (id, requester_id, app_id, permission_key, status)
		VALUES ($1, $2, $3, $4, 'pending')
	`, id, requesterID, appID, permissionKey)
	return err
}

func (s *Store) ListRequestsForOperator(ctx context.Context, operatorID int64) ([]AccessRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.requester_id, o.login, r.app_id, a.name, r.permission_key, p.label,
		       r.status, r.reviewed_by, r.created_at, r.reviewed_at
		FROM access_requests r
		JOIN operators o ON o.github_id = r.requester_id
		JOIN apps a ON a.id = r.app_id
		JOIN permissions p ON p.app_id = r.app_id AND p.key = r.permission_key
		WHERE r.requester_id = $1
		ORDER BY r.created_at DESC
	`, operatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

func (s *Store) ListPendingRequests(ctx context.Context) ([]AccessRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.requester_id, o.login, r.app_id, a.name, r.permission_key, p.label,
		       r.status, r.reviewed_by, r.created_at, r.reviewed_at
		FROM access_requests r
		JOIN operators o ON o.github_id = r.requester_id
		JOIN apps a ON a.id = r.app_id
		JOIN permissions p ON p.app_id = r.app_id AND p.key = r.permission_key
		WHERE r.status = 'pending'
		ORDER BY r.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

func scanRequests(rows pgx.Rows) ([]AccessRequest, error) {
	var out []AccessRequest
	for rows.Next() {
		var r AccessRequest
		var reviewedBy *int64
		var reviewedAt *time.Time
		if err := rows.Scan(
			&r.ID, &r.RequesterID, &r.RequesterLogin, &r.AppID, &r.AppName,
			&r.PermissionKey, &r.PermissionLabel, &r.Status, &reviewedBy, &r.CreatedAt, &reviewedAt,
		); err != nil {
			return nil, err
		}
		r.ReviewedBy = reviewedBy
		r.ReviewedAt = reviewedAt
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetAccessRequest(ctx context.Context, id string) (*AccessRequest, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT r.id, r.requester_id, o.login, r.app_id, a.name, r.permission_key, p.label,
		       r.status, r.reviewed_by, r.created_at, r.reviewed_at
		FROM access_requests r
		JOIN operators o ON o.github_id = r.requester_id
		JOIN apps a ON a.id = r.app_id
		JOIN permissions p ON p.app_id = r.app_id AND p.key = r.permission_key
		WHERE r.id = $1
	`, id)
	var r AccessRequest
	var reviewedBy *int64
	var reviewedAt *time.Time
	if err := row.Scan(
		&r.ID, &r.RequesterID, &r.RequesterLogin, &r.AppID, &r.AppName,
		&r.PermissionKey, &r.PermissionLabel, &r.Status, &reviewedBy, &r.CreatedAt, &reviewedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	r.ReviewedBy = reviewedBy
	r.ReviewedAt = reviewedAt
	return &r, nil
}

func (s *Store) ApproveRequest(ctx context.Context, requestID string, reviewerID int64) (*AccessRequest, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT id, requester_id, app_id, permission_key, status
		FROM access_requests WHERE id = $1 FOR UPDATE
	`, requestID)
	var id string
	var requesterID int64
	var appID, permissionKey, status string
	if err := row.Scan(&id, &requesterID, &appID, &permissionKey, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if status != "pending" {
		return nil, fmt.Errorf("%w: not pending", ErrConflict)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE access_requests
		SET status = 'approved', reviewed_by = $2, reviewed_at = now()
		WHERE id = $1
	`, requestID, reviewerID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO grants (operator_id, app_id, permission_key)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, requesterID, appID, permissionKey); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetAccessRequest(ctx, requestID)
}

func (s *Store) DenyRequest(ctx context.Context, requestID string, reviewerID int64) (*AccessRequest, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE access_requests
		SET status = 'denied', reviewed_by = $2, reviewed_at = now()
		WHERE id = $1 AND status = 'pending'
	`, requestID, reviewerID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		_, gerr := s.GetAccessRequest(ctx, requestID)
		if errors.Is(gerr, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: not pending", ErrConflict)
	}
	return s.GetAccessRequest(ctx, requestID)
}

type ProductDailyKey struct {
	AppID   string `json:"app_id"`
	AppName string `json:"app_name"`
	Day     string `json:"day"`
	Key     string `json:"key"`
}

// EnsureProductDailyKey returns the key for one product and Eastern day,
// creating it from candidate if necessary. Only explicitly enabled apps can
// have daily keys.
func (s *Store) EnsureProductDailyKey(ctx context.Context, appID, day, candidate string) (ProductDailyKey, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO guest_keys (app_id, day, key)
		SELECT id, $2::date, $3 FROM apps
		WHERE id = $1 AND daily_key_enabled = true
		ON CONFLICT (app_id, day) DO UPDATE SET key = guest_keys.key
		RETURNING app_id, day::text, key
	`, appID, day, candidate)
	var key ProductDailyKey
	if err := row.Scan(&key.AppID, &key.Day, &key.Key); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductDailyKey{}, ErrNotFound
		}
		return ProductDailyKey{}, err
	}
	return key, nil
}

func (s *Store) ListDailyKeyApps(ctx context.Context) ([]App, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, daily_key_enabled
		FROM apps WHERE daily_key_enabled = true ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []App
	for rows.Next() {
		var app App
		if err := rows.Scan(&app.ID, &app.Name, &app.Description, &app.DailyKeyEnabled); err != nil {
			return nil, err
		}
		out = append(out, app)
	}
	return out, rows.Err()
}

// DeleteOldProductDailyKeys removes product keys for past Eastern days.
func (s *Store) DeleteOldProductDailyKeys(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM guest_keys WHERE day < (now() AT TIME ZONE 'America/Toronto')::date`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func databaseConfig(raw string) (*pgxpool.Config, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	q := u.Query()
	schema := q.Get("schema")
	if schema == "" {
		schema = "oauth"
	}
	if !schemaPattern.MatchString(schema) {
		return nil, errors.New("database schema contains invalid characters")
	}
	q.Del("schema")
	u.RawQuery = q.Encode()

	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.ConnConfig.RuntimeParams["timezone"] = "America/Toronto"
	return cfg, nil
}
