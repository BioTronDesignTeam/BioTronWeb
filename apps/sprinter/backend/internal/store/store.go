// Package store is Sprinter's Postgres access. The SQL lives in
// queries/queries.sql and sqlc makes it into Go (sqlc.yaml): the schema is
// Prisma's, but the bot never runs a Node process to read it.
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

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store/queries"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	schemaName  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

type Store struct {
	pool *pgxpool.Pool
	q    *queries.Queries
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
	return &Store{pool: pool, q: queries.New(pool)}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// --- Guards -----------------------------------------------------------------

func (s *Store) ListGuards(ctx context.Context) ([]Guard, error) {
	rows, err := s.q.ListGuards(ctx)
	if err != nil {
		return nil, err
	}
	guards := make([]Guard, 0, len(rows))
	for _, row := range rows {
		guards = append(guards, guardFrom(row))
	}
	return guards, nil
}

func (s *Store) GetGuard(ctx context.Context, subject string) (Guard, error) {
	row, err := s.q.GetGuard(ctx, subject)
	if err != nil {
		return Guard{}, mapNotFound(err)
	}
	return guardFrom(row), nil
}

// UpsertGuard writes the whole guard. A guard is a short list an operator
// edits as a unit, so there is no partial update to merge.
func (s *Store) UpsertGuard(ctx context.Context, guard Guard) (Guard, error) {
	row, err := s.q.UpsertGuard(ctx, queries.UpsertGuardParams{
		Subject:    guard.Subject,
		GuildID:    guard.GuildID,
		RoleIds:    nonNil(guard.RoleIDs),
		ChannelIds: nonNil(guard.ChannelIDs),
	})
	if err != nil {
		return Guard{}, mapNotFound(err)
	}
	return guardFrom(row), nil
}

func (s *Store) DeleteGuard(ctx context.Context, subject string) error {
	deleted, err := s.q.DeleteGuard(ctx, subject)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Automations ------------------------------------------------------------

func (s *Store) ListAutomations(ctx context.Context) ([]Automation, error) {
	rows, err := s.q.ListAutomations(ctx)
	if err != nil {
		return nil, err
	}
	automations := make([]Automation, 0, len(rows))
	for _, row := range rows {
		automations = append(automations, automationFrom(queries.GetAutomationRow(row)))
	}
	return automations, nil
}

func (s *Store) GetAutomation(ctx context.Context, id string) (Automation, error) {
	row, err := s.q.GetAutomation(ctx, id)
	if err != nil {
		return Automation{}, mapNotFound(err)
	}
	return automationFrom(row), nil
}

func (s *Store) CreateAutomation(ctx context.Context, automation Automation) (Automation, error) {
	if automation.ID == "" {
		automation.ID = uuid.NewString()
	}
	err := s.q.CreateAutomation(ctx, queries.CreateAutomationParams{
		ID:            automation.ID,
		Kind:          automation.Kind,
		Name:          automation.Name,
		ScopeID:       automation.ScopeID,
		ChannelID:     automation.ChannelID,
		LeadUserID:    automation.LeadUserID,
		LeadHours:     int32(automation.LeadHours),
		LookbackHours: int32(automation.LookbackHours),
		AnyAuthor:     automation.AnyAuthor,
		PostHour:      int32Pointer(automation.PostHour),
		Deliver:       automation.Deliver,
		Enabled:       automation.Enabled,
	})
	if err != nil {
		return Automation{}, mapConstraintError(err)
	}
	return s.GetAutomation(ctx, automation.ID)
}

// UpdateAutomation writes every field. The admin UI sends the whole row back,
// and a PATCH that merged nulls could not tell "clear the lead" from "leave
// the lead alone".
func (s *Store) UpdateAutomation(ctx context.Context, automation Automation) (Automation, error) {
	updated, err := s.q.UpdateAutomation(ctx, queries.UpdateAutomationParams{
		ID:            automation.ID,
		Kind:          automation.Kind,
		Name:          automation.Name,
		ScopeID:       automation.ScopeID,
		ChannelID:     automation.ChannelID,
		LeadUserID:    automation.LeadUserID,
		LeadHours:     int32(automation.LeadHours),
		LookbackHours: int32(automation.LookbackHours),
		AnyAuthor:     automation.AnyAuthor,
		PostHour:      int32Pointer(automation.PostHour),
		Deliver:       automation.Deliver,
		Enabled:       automation.Enabled,
	})
	if err != nil {
		return Automation{}, mapConstraintError(err)
	}
	if updated == 0 {
		return Automation{}, ErrNotFound
	}
	return s.GetAutomation(ctx, automation.ID)
}

func (s *Store) DeleteAutomation(ctx context.Context, id string) error {
	deleted, err := s.q.DeleteAutomation(ctx, id)
	if err != nil {
		return mapConstraintError(err)
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Threads and transcripts ------------------------------------------------

func (s *Store) CreateThread(ctx context.Context, thread Thread) (Thread, error) {
	row, err := s.q.CreateThread(ctx, queries.CreateThreadParams{
		ThreadID:  thread.ThreadID,
		GuildID:   thread.GuildID,
		ChannelID: thread.ChannelID,
		OpenerID:  thread.OpenerID,
		Model:     thread.Model,
		TurnCount: int32(thread.TurnCount),
	})
	if err != nil {
		return Thread{}, mapConstraintError(mapNotFound(err))
	}
	return threadFrom(row), nil
}

func (s *Store) GetThread(ctx context.Context, threadID string) (Thread, error) {
	row, err := s.q.GetThread(ctx, threadID)
	if err != nil {
		return Thread{}, mapNotFound(err)
	}
	return threadFrom(row), nil
}

// TouchThread records one more answered turn. The count and the timestamp move
// together because a later sweep archives threads by both.
func (s *Store) TouchThread(ctx context.Context, threadID string) (Thread, error) {
	row, err := s.q.TouchThread(ctx, threadID)
	if err != nil {
		return Thread{}, mapNotFound(err)
	}
	return threadFrom(row), nil
}

// AppendMessage stores one transcript turn. The caller owns the sequence, and
// the unique index on (thread_id, sequence) turns a replayed Discord event
// into a conflict rather than a duplicated turn.
func (s *Store) AppendMessage(ctx context.Context, threadID string, sequence int, role string, content json.RawMessage) (Message, error) {
	row, err := s.q.AppendMessage(ctx, queries.AppendMessageParams{
		ThreadID: threadID,
		Sequence: int32(sequence),
		Role:     role,
		Content:  []byte(content),
	})
	if err != nil {
		// The unique index on (thread_id, sequence) is what turns a replayed
		// Discord event into a conflict, so the caller has to be able to tell
		// that apart from a database that is simply down.
		return Message{}, mapConstraintError(mapNotFound(err))
	}
	return messageFrom(row)
}

func (s *Store) ListMessages(ctx context.Context, threadID string) ([]Message, error) {
	rows, err := s.q.ListMessages(ctx, threadID)
	if err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		message, err := messageFrom(row)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

// --- Rows -------------------------------------------------------------------

func guardFrom(row queries.Guard) Guard {
	return Guard{
		Subject:    row.Subject,
		GuildID:    row.GuildID,
		RoleIDs:    nonNil(row.RoleIds),
		ChannelIDs: nonNil(row.ChannelIds),
		UpdatedAt:  row.UpdatedAt,
	}
}

// automationFrom takes GetAutomation's row. The list queries return the same
// columns, so their rows convert to it.
func automationFrom(row queries.GetAutomationRow) Automation {
	return Automation{
		ID:            row.ID,
		Kind:          row.Kind,
		Name:          row.Name,
		ScopeID:       row.ScopeID,
		ChannelID:     row.ChannelID,
		LeadUserID:    row.LeadUserID,
		LeadHours:     int(row.LeadHours),
		LookbackHours: int(row.LookbackHours),
		AnyAuthor:     row.AnyAuthor,
		PostHour:      intPointer(row.PostHour),
		Deliver:       row.Deliver,
		Enabled:       row.Enabled,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func threadFrom(row queries.AgentThread) Thread {
	return Thread{
		ThreadID:     row.ThreadID,
		GuildID:      row.GuildID,
		ChannelID:    row.ChannelID,
		OpenerID:     row.OpenerID,
		Model:        row.Model,
		TurnCount:    int(row.TurnCount),
		CreatedAt:    row.CreatedAt,
		LastActiveAt: row.LastActiveAt,
	}
}

func messageFrom(row queries.AgentMessage) (Message, error) {
	if !json.Valid(row.Content) {
		return Message{}, errors.New("stored transcript content is invalid JSON")
	}
	return Message{
		ID:        row.ID,
		ThreadID:  row.ThreadID,
		Sequence:  int(row.Sequence),
		Role:      row.Role,
		Content:   json.RawMessage(row.Content),
		CreatedAt: row.CreatedAt,
	}, nil
}

func intPointer(value *int32) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}

func int32Pointer(value *int) *int32 {
	if value == nil {
		return nil
	}
	converted := int32(*value)
	return &converted
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
