// Package store is Sprinter's Postgres access. Plain SQL through pgx: the
// schema is Prisma's, but the bot never runs a Node process to read it.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	schemaName  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := databaseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
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

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// --- Guards -----------------------------------------------------------------

func (s *Store) ListGuards(ctx context.Context) ([]Guard, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT subject, guild_id, role_ids, channel_ids, updated_at
		FROM guards ORDER BY subject
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	guards := []Guard{}
	for rows.Next() {
		guard, err := scanGuard(rows)
		if err != nil {
			return nil, err
		}
		guards = append(guards, guard)
	}
	return guards, rows.Err()
}

func (s *Store) GetGuard(ctx context.Context, subject string) (Guard, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT subject, guild_id, role_ids, channel_ids, updated_at
		FROM guards WHERE subject = $1
	`, subject)
	return scanGuard(row)
}

// UpsertGuard writes the whole guard. A guard is a short list an operator
// edits as a unit, so there is no partial update to merge.
func (s *Store) UpsertGuard(ctx context.Context, guard Guard) (Guard, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO guards (subject, guild_id, role_ids, channel_ids, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (subject) DO UPDATE SET
			guild_id = EXCLUDED.guild_id,
			role_ids = EXCLUDED.role_ids,
			channel_ids = EXCLUDED.channel_ids,
			updated_at = now()
		RETURNING subject, guild_id, role_ids, channel_ids, updated_at
	`, guard.Subject, guard.GuildID, nonNil(guard.RoleIDs), nonNil(guard.ChannelIDs))
	return scanGuard(row)
}

func (s *Store) DeleteGuard(ctx context.Context, subject string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM guards WHERE subject = $1`, subject)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Automations ------------------------------------------------------------

const automationColumns = `
	id::text, kind::text, name, scope_id::text, channel_id, lead_user_id,
	lead_hours, lookback_hours, any_author, post_hour, deliver::text, enabled,
	created_at, updated_at
`

func (s *Store) ListAutomations(ctx context.Context) ([]Automation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+automationColumns+`
		FROM automations ORDER BY kind, lower(name)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	automations := []Automation{}
	for rows.Next() {
		automation, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		automations = append(automations, automation)
	}
	return automations, rows.Err()
}

func (s *Store) GetAutomation(ctx context.Context, id string) (Automation, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+automationColumns+` FROM automations WHERE id = $1::uuid
	`, id)
	return scanAutomation(row)
}

func (s *Store) CreateAutomation(ctx context.Context, automation Automation) (Automation, error) {
	if automation.ID == "" {
		automation.ID = uuid.NewString()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO automations (
			id, kind, name, scope_id, channel_id, lead_user_id, lead_hours,
			lookback_hours, any_author, post_hour, deliver, enabled, updated_at
		) VALUES (
			$1::uuid, $2::"AutomationKind", $3, $4::uuid, $5, $6, $7,
			$8, $9, $10, $11::"NudgeDelivery", $12, now()
		)
	`, automation.ID, automation.Kind, automation.Name, automation.ScopeID,
		automation.ChannelID, automation.LeadUserID, automation.LeadHours,
		automation.LookbackHours, automation.AnyAuthor, automation.PostHour,
		automation.Deliver, automation.Enabled)
	if err != nil {
		return Automation{}, mapConstraintError(err)
	}
	return s.GetAutomation(ctx, automation.ID)
}

// UpdateAutomation writes every field. The admin UI sends the whole row back,
// and a PATCH that merged nulls could not tell "clear the lead" from "leave
// the lead alone".
func (s *Store) UpdateAutomation(ctx context.Context, automation Automation) (Automation, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE automations SET
			kind = $2::"AutomationKind", name = $3, scope_id = $4::uuid,
			channel_id = $5, lead_user_id = $6, lead_hours = $7,
			lookback_hours = $8, any_author = $9, post_hour = $10,
			deliver = $11::"NudgeDelivery", enabled = $12, updated_at = now()
		WHERE id = $1::uuid
	`, automation.ID, automation.Kind, automation.Name, automation.ScopeID,
		automation.ChannelID, automation.LeadUserID, automation.LeadHours,
		automation.LookbackHours, automation.AnyAuthor, automation.PostHour,
		automation.Deliver, automation.Enabled)
	if err != nil {
		return Automation{}, mapConstraintError(err)
	}
	if tag.RowsAffected() == 0 {
		return Automation{}, ErrNotFound
	}
	return s.GetAutomation(ctx, automation.ID)
}

func (s *Store) DeleteAutomation(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM automations WHERE id = $1::uuid`, id)
	if err != nil {
		return mapConstraintError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Threads and transcripts ------------------------------------------------

func (s *Store) CreateThread(ctx context.Context, thread Thread) (Thread, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO agent_threads (
			thread_id, guild_id, channel_id, opener_id, model, turn_count, last_active_at
		) VALUES ($1, $2, $3, $4, $5, $6, now())
		RETURNING thread_id, guild_id, channel_id, opener_id, model, turn_count,
		          created_at, last_active_at
	`, thread.ThreadID, thread.GuildID, thread.ChannelID, thread.OpenerID,
		thread.Model, thread.TurnCount)
	created, err := scanThread(row)
	if err != nil {
		return Thread{}, mapConstraintError(err)
	}
	return created, nil
}

func (s *Store) GetThread(ctx context.Context, threadID string) (Thread, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT thread_id, guild_id, channel_id, opener_id, model, turn_count,
		       created_at, last_active_at
		FROM agent_threads WHERE thread_id = $1
	`, threadID)
	return scanThread(row)
}

// TouchThread records one more answered turn. The count and the timestamp move
// together because a later sweep archives threads by both.
func (s *Store) TouchThread(ctx context.Context, threadID string) (Thread, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE agent_threads
		SET turn_count = turn_count + 1, last_active_at = now()
		WHERE thread_id = $1
		RETURNING thread_id, guild_id, channel_id, opener_id, model, turn_count,
		          created_at, last_active_at
	`, threadID)
	return scanThread(row)
}

// AppendMessage stores one transcript turn. The caller owns the sequence, and
// the unique index on (thread_id, sequence) turns a replayed Discord event
// into a conflict rather than a duplicated turn.
func (s *Store) AppendMessage(ctx context.Context, threadID string, sequence int, role string, content json.RawMessage) (Message, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO agent_messages (thread_id, sequence, role, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, thread_id, sequence, role, content, created_at
	`, threadID, sequence, role, []byte(content))
	saved, err := scanMessage(row)
	if err != nil {
		// The unique index on (thread_id, sequence) is what turns a replayed
		// Discord event into a conflict, so the caller has to be able to tell
		// that apart from a database that is simply down.
		return Message{}, mapConstraintError(err)
	}
	return saved, nil
}

func (s *Store) ListMessages(ctx context.Context, threadID string) ([]Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, thread_id, sequence, role, content, created_at
		FROM agent_messages WHERE thread_id = $1 ORDER BY sequence
	`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []Message{}
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// --- Scanning ---------------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

func scanGuard(row scanner) (Guard, error) {
	var guard Guard
	if err := row.Scan(&guard.Subject, &guard.GuildID, &guard.RoleIDs,
		&guard.ChannelIDs, &guard.UpdatedAt); err != nil {
		return Guard{}, mapNotFound(err)
	}
	guard.RoleIDs = nonNil(guard.RoleIDs)
	guard.ChannelIDs = nonNil(guard.ChannelIDs)
	return guard, nil
}

func scanAutomation(row scanner) (Automation, error) {
	var automation Automation
	if err := row.Scan(&automation.ID, &automation.Kind, &automation.Name,
		&automation.ScopeID, &automation.ChannelID, &automation.LeadUserID,
		&automation.LeadHours, &automation.LookbackHours, &automation.AnyAuthor,
		&automation.PostHour, &automation.Deliver, &automation.Enabled,
		&automation.CreatedAt, &automation.UpdatedAt); err != nil {
		return Automation{}, mapNotFound(err)
	}
	return automation, nil
}

func scanThread(row scanner) (Thread, error) {
	var thread Thread
	if err := row.Scan(&thread.ThreadID, &thread.GuildID, &thread.ChannelID,
		&thread.OpenerID, &thread.Model, &thread.TurnCount,
		&thread.CreatedAt, &thread.LastActiveAt); err != nil {
		return Thread{}, mapNotFound(err)
	}
	return thread, nil
}

func scanMessage(row scanner) (Message, error) {
	var message Message
	var content []byte
	if err := row.Scan(&message.ID, &message.ThreadID, &message.Sequence,
		&message.Role, &content, &message.CreatedAt); err != nil {
		return Message{}, mapNotFound(err)
	}
	if !json.Valid(content) {
		return Message{}, errors.New("stored transcript content is invalid JSON")
	}
	message.Content = json.RawMessage(content)
	return message, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func mapConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23505", "23514":
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
		}
	}
	return err
}

// nonNil keeps a Postgres text[] out of JSON as `[]` rather than `null`, so a
// guard with no channels reads the same in the API as an empty list.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// databaseConfig turns the `?schema=` Prisma writes into a search_path, which
// is what pgx understands. Both pools point at the same database, so the
// schema has to come from the URL rather than a qualified name in every query.
func databaseConfig(raw string) (*pgxpool.Config, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	query := parsed.Query()
	schema := query.Get("schema")
	if schema == "" {
		schema = "sprinter"
	}
	if !schemaName.MatchString(schema) {
		return nil, errors.New("database schema contains invalid characters")
	}
	query.Del("schema")
	parsed.RawQuery = query.Encode()
	config, err := pgxpool.ParseConfig(parsed.String())
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	config.ConnConfig.RuntimeParams["timezone"] = "America/Toronto"
	return config, nil
}

// Timeout is the deadline every background caller uses for one store call, so
// a stalled database cannot hold a Discord interaction open past its window.
const Timeout = 5 * time.Second
