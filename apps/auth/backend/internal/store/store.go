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

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store/queries"
)

var ErrNotFound = errors.New("not found")

// GuestGitHubID is the sentinel operator for daily-key guest sessions.
// Real GitHub numeric ids are >= 1.
const GuestGitHubID int64 = 0
const GuestLogin = "guest"

// Store runs the queries that sqlc generates from queries/queries.sql and
// turns their rows into the types the handlers use.
type Store struct {
	pool *pgxpool.Pool
	q    *queries.Queries
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
	return &Store{pool: pool, q: queries.New(pool)}, nil
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
	// The flag mirrors SUPERUSER_GITHUB_IDS, which RequireSession reads on every
	// request: listed → superuser, omitted → cleared. Managers are independent
	// (org UI / is_manager).
	return s.q.UpsertOperator(ctx, queries.UpsertOperatorParams{
		GitHubID:    op.GitHubID,
		Login:       op.Login,
		Name:        nullable(op.Name),
		AvatarURL:   nullable(op.AvatarURL),
		IsSuperuser: forceSuperuser,
	})
}

// ReconcileSuperusers sets the superuser flag of every operator to whether ids
// lists them, and returns how many changed.
func (s *Store) ReconcileSuperusers(ctx context.Context, ids []int64) (int64, error) {
	if ids == nil {
		// A nil slice is NULL to Postgres, and NULL is in no list.
		ids = []int64{}
	}
	return s.q.ReconcileSuperusers(ctx, ids)
}

func (s *Store) GetOperator(ctx context.Context, githubID int64) (*OrgMember, error) {
	row, err := s.q.GetOperator(ctx, githubID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m := OrgMember(row)
	return &m, nil
}

func (s *Store) ListOrgMembers(ctx context.Context) ([]OrgMember, error) {
	rows, err := s.q.ListOrgMembers(ctx, GuestGitHubID)
	if err != nil {
		return nil, err
	}
	var out []OrgMember
	for _, row := range rows {
		out = append(out, OrgMember(row))
	}
	return out, nil
}

func (s *Store) SetBanned(ctx context.Context, githubID int64, banned bool) error {
	n, err := s.q.SetBanned(ctx, queries.SetBannedParams{GitHubID: githubID, IsBanned: banned})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetManager(ctx context.Context, githubID int64, manager bool) error {
	n, err := s.q.SetManager(ctx, queries.SetManagerParams{GitHubID: githubID, IsManager: manager})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListApps(ctx context.Context) ([]App, error) {
	rows, err := s.q.ListApps(ctx)
	if err != nil {
		return nil, err
	}
	var out []App
	for _, row := range rows {
		out = append(out, App(row))
	}
	return out, nil
}

func (s *Store) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := s.q.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	var out []Permission
	for _, row := range rows {
		out = append(out, Permission(row))
	}
	return out, nil
}

func (s *Store) GetPermission(ctx context.Context, appID, key string) (*Permission, error) {
	row, err := s.q.GetPermission(ctx, queries.GetPermissionParams{AppID: appID, Key: key})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p := Permission(row)
	return &p, nil
}

func (s *Store) CreateApp(ctx context.Context, id, name, description string) error {
	return s.q.CreateApp(ctx, queries.CreateAppParams{ID: id, Name: name, Description: description})
}

func (s *Store) ListGrantsForOperator(ctx context.Context, operatorID int64) ([]Grant, error) {
	rows, err := s.q.ListGrantsForOperator(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	var out []Grant
	for _, row := range rows {
		out = append(out, Grant(row))
	}
	return out, nil
}

func (s *Store) CreateGrant(ctx context.Context, operatorID int64, appID, permissionKey string) error {
	if _, err := s.GetPermission(ctx, appID, permissionKey); err != nil {
		return err
	}
	return s.q.CreateGrant(ctx, queries.CreateGrantParams{
		OperatorID: operatorID, AppID: appID, PermissionKey: permissionKey,
	})
}

func (s *Store) DeleteGrant(ctx context.Context, operatorID int64, appID, permissionKey string) error {
	n, err := s.q.DeleteGrant(ctx, queries.DeleteGrantParams{
		OperatorID: operatorID, AppID: appID, PermissionKey: permissionKey,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAllGrants(ctx context.Context, operatorID int64) error {
	return s.q.DeleteAllGrants(ctx, operatorID)
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
	row, err := s.q.EnsureProductDailyKey(ctx, queries.EnsureProductDailyKeyParams{
		AppID: appID, Day: day, Key: candidate,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductDailyKey{}, ErrNotFound
		}
		return ProductDailyKey{}, err
	}
	return ProductDailyKey{AppID: row.AppID, Day: row.Day, Key: row.Key}, nil
}

// RotateProductDailyKey replaces the key of one enabled product for one Eastern
// day with candidate, creating it if there was none.
func (s *Store) RotateProductDailyKey(ctx context.Context, appID, day, candidate string) (ProductDailyKey, error) {
	row, err := s.q.RotateProductDailyKey(ctx, queries.RotateProductDailyKeyParams{
		AppID: appID, Day: day, Key: candidate,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductDailyKey{}, ErrNotFound
		}
		return ProductDailyKey{}, err
	}
	return ProductDailyKey{AppID: row.AppID, Day: row.Day, Key: row.Key}, nil
}

// GetGuestKey returns the key of one product and Eastern day, or ErrNotFound
// when there is none or the product no longer has daily keys.
func (s *Store) GetGuestKey(ctx context.Context, appID, day string) (string, error) {
	key, err := s.q.GetGuestKey(ctx, queries.GetGuestKeyParams{AppID: appID, Day: day})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return key, err
}

func (s *Store) ListDailyKeyApps(ctx context.Context) ([]App, error) {
	rows, err := s.q.ListDailyKeyApps(ctx)
	if err != nil {
		return nil, err
	}
	var out []App
	for _, row := range rows {
		out = append(out, App(row))
	}
	return out, nil
}

// DeleteOldProductDailyKeys removes product keys for past Eastern days.
func (s *Store) DeleteOldProductDailyKeys(ctx context.Context) (int64, error) {
	return s.q.DeleteOldProductDailyKeys(ctx)
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
