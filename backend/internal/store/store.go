package store

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a lookup matches no (live) row.
var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(sanitizeDSN(databaseURL))
	if err != nil {
		return nil, err
	}
	// Every connection runs in Eastern (DST-aware) so timestamptz values render
	// and day boundaries compute in local Waterloo time. See CLAUDE.md.
	cfg.ConnConfig.RuntimeParams["timezone"] = "America/Toronto"
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
	GitHubID  int64
	Login     string
	Name      string
	AvatarURL string
}

// SessionOperator is the operator behind a valid (unexpired) session.
type SessionOperator struct {
	GitHubID  int64
	Login     string
	Name      string
	AvatarURL string
}

// UpsertOperator inserts or refreshes the operator row keyed by github_id and
// bumps last_login_at.
func (s *Store) UpsertOperator(ctx context.Context, op Operator) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO operators (github_id, login, name, avatar_url, last_login_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (github_id) DO UPDATE
		SET login = EXCLUDED.login,
		    name = EXCLUDED.name,
		    avatar_url = EXCLUDED.avatar_url,
		    last_login_at = now()
	`, op.GitHubID, op.Login, nullable(op.Name), nullable(op.AvatarURL))
	return err
}

func (s *Store) CreateSession(ctx context.Context, idHash string, githubID int64, expiresAt time.Time, userAgent string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, operator_id, expires_at, user_agent)
		VALUES ($1, $2, $3, $4)
	`, idHash, githubID, expiresAt, nullable(userAgent))
	return err
}

// GetSessionOperator returns the operator for a live session, or ErrNotFound if
// the session is missing or expired.
func (s *Store) GetSessionOperator(ctx context.Context, idHash string) (*SessionOperator, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT o.github_id, o.login, COALESCE(o.name, ''), COALESCE(o.avatar_url, '')
		FROM sessions s
		JOIN operators o ON o.github_id = s.operator_id
		WHERE s.id = $1 AND s.expires_at > now()
	`, idHash)
	var so SessionOperator
	if err := row.Scan(&so.GitHubID, &so.Login, &so.Name, &so.AvatarURL); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &so, nil
}

func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, idHash)
	return err
}

// DeleteExpiredSessions removes sessions past their expiry and returns how many
// were pruned. GetSessionOperator already ignores expired rows; this reclaims the
// disk they would otherwise hold forever.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DeleteOldGuestKeys removes guest keys for past Eastern days and returns how many
// were pruned, so the table doesn't accumulate one stale key per day forever.
func (s *Store) DeleteOldGuestKeys(ctx context.Context) (int64, error) {
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

// sanitizeDSN drops the Prisma-only `schema` query param, which pgx/libpq don't
// understand (public is the default search_path anyway).
func sanitizeDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Del("schema")
	u.RawQuery = q.Encode()
	return u.String()
}

type GuestKey struct {
	Day string
	Key string
}

// EnsureGuestKey returns the guest key for the given Eastern day (YYYY-MM-DD),
// creating it from candidate if none exists yet. The caller passes the day so the
// key's validity and the guest session's expiry share one clock read. A single
// statement so the row is always returned — the no-op DO UPDATE fires RETURNING
// on conflict too.
func (s *Store) EnsureGuestKey(ctx context.Context, day, candidate string) (GuestKey, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO guest_keys (day, key)
		VALUES ($1::date, $2)
		ON CONFLICT (day) DO UPDATE SET key = guest_keys.key
		RETURNING day::text, key
	`, day, candidate)
	var gk GuestKey
	if err := row.Scan(&gk.Day, &gk.Key); err != nil {
		return GuestKey{}, err
	}
	return gk, nil
}
