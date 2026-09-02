package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	calendarlogic "github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
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

func (s *Store) ListScopes(ctx context.Context, includeArchived bool) ([]model.Scope, error) {
	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE scope_tree AS (
			SELECT id, parent_id, status = 'ACTIVE' AS effectively_active
			FROM calendar_scopes WHERE parent_id IS NULL
			UNION ALL
			SELECT child.id, child.parent_id,
			       parent.effectively_active AND child.status = 'ACTIVE'
			FROM calendar_scopes child
			JOIN scope_tree parent ON child.parent_id = parent.id
		)
		SELECT s.id::text, s.kind::text, s.name, s.slug, s.status::text,
		       COALESCE(s.parent_id::text, ''), s.archived_at, s.created_at, s.updated_at,
		       (SELECT count(*) FROM calendar_scopes c WHERE c.parent_id = s.id),
		       (SELECT count(*) FROM event_series e WHERE e.scope_id = s.id)
		FROM calendar_scopes s
		JOIN scope_tree visibility ON visibility.id = s.id
		WHERE $1 OR visibility.effectively_active
		ORDER BY CASE s.kind WHEN 'TEAM' THEN 0 WHEN 'PROJECT' THEN 1 ELSE 2 END, lower(s.name)
	`, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scopes []model.Scope
	for rows.Next() {
		scope, err := scanScope(rows)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, rows.Err()
}

func (s *Store) GetScope(ctx context.Context, id string) (model.Scope, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT s.id::text, s.kind::text, s.name, s.slug, s.status::text,
		       COALESCE(s.parent_id::text, ''), s.archived_at, s.created_at, s.updated_at,
		       (SELECT count(*) FROM calendar_scopes c WHERE c.parent_id = s.id),
		       (SELECT count(*) FROM event_series e WHERE e.scope_id = s.id)
		FROM calendar_scopes s WHERE s.id = $1
	`, id)
	return scanScope(row)
}

func (s *Store) CreateScope(ctx context.Context, scope model.Scope) (model.Scope, error) {
	if scope.ID == "" {
		scope.ID = uuid.NewString()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO calendar_scopes (id, kind, name, slug, parent_id)
		VALUES ($1, $2::"CalendarScopeKind", $3, $4, NULLIF($5, '')::uuid)
	`, scope.ID, scope.Kind, scope.Name, scope.Slug, value(scope.ParentID))
	if err != nil {
		return model.Scope{}, mapConstraintError(err)
	}
	return s.GetScope(ctx, scope.ID)
}

func (s *Store) RenameScope(ctx context.Context, id, name, slug string) (model.Scope, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE calendar_scopes SET name = $2, slug = $3, updated_at = now() WHERE id = $1
	`, id, name, slug)
	if err != nil {
		return model.Scope{}, mapConstraintError(err)
	}
	if tag.RowsAffected() == 0 {
		return model.Scope{}, ErrNotFound
	}
	return s.GetScope(ctx, id)
}

func (s *Store) SetScopeArchived(ctx context.Context, id string, archived bool) (model.Scope, error) {
	status := model.ScopeActive
	if archived {
		status = model.ScopeArchived
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE calendar_scopes
		SET status = $2::"CalendarScopeStatus",
		    archived_at = CASE WHEN $2 = 'ARCHIVED' THEN now() ELSE NULL END,
		    updated_at = now()
		WHERE id = $1 AND kind <> 'TEAM'
	`, id, status)
	if err != nil {
		return model.Scope{}, err
	}
	if tag.RowsAffected() == 0 {
		return model.Scope{}, ErrNotFound
	}
	return s.GetScope(ctx, id)
}

func (s *Store) DeleteScope(ctx context.Context, id string) error {
	scope, err := s.GetScope(ctx, id)
	if err != nil {
		return err
	}
	if scope.Kind == model.ScopeTeam || scope.ChildCount > 0 || scope.EventCount > 0 {
		return ErrConflict
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM calendar_scopes WHERE id = $1`, id)
	if err != nil {
		return mapConstraintError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ScopeIsActive(ctx context.Context, id string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id, status FROM calendar_scopes WHERE id = $1
			UNION ALL
			SELECT parent.id, parent.parent_id, parent.status
			FROM calendar_scopes parent JOIN ancestors child ON child.parent_id = parent.id
		)
		SELECT COALESCE(bool_and(status = 'ACTIVE'), false) FROM ancestors
	`, id).Scan(&active)
	return active, err
}

func (s *Store) ListSeries(ctx context.Context, scopeID string, includeDrafts bool) ([]model.EventSeries, error) {
	return s.listSeries(ctx, scopeID, includeDrafts, includeDrafts)
}

// ListFeedSeries retains published history for stable subscription URLs even
// when a scope is archived. Public JSON views use ListSeries and hide archived
// scope trees; feeds continue to reconcile existing subscriber calendars.
func (s *Store) ListFeedSeries(ctx context.Context, scopeID string) ([]model.EventSeries, error) {
	return s.listSeries(ctx, scopeID, false, true)
}

func (s *Store) listSeries(ctx context.Context, scopeID string, includeDrafts, includeHiddenScopes bool) ([]model.EventSeries, error) {
	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE scope_tree AS (
			SELECT id, parent_id, status = 'ACTIVE' AS effectively_active
			FROM calendar_scopes WHERE parent_id IS NULL
			UNION ALL
			SELECT child.id, child.parent_id,
			       parent.effectively_active AND child.status = 'ACTIVE'
			FROM calendar_scopes child
			JOIN scope_tree parent ON child.parent_id = parent.id
		)
		SELECT e.id::text, e.uid, e.scope_id::text, s.name, s.kind::text, e.state::text,
		       e.title, e.description, e.location, e.url, e.starts_at_local, e.ends_at_local,
		       e.timezone, e.all_day, COALESCE(e.recurrence_until::text, ''), e.sequence,
		       e.published_at, e.cancelled_at, e.created_at, e.updated_at
		FROM event_series e
		JOIN calendar_scopes s ON s.id = e.scope_id
		JOIN scope_tree visibility ON visibility.id = s.id
		WHERE ($1 = '' OR e.scope_id = $1::uuid)
		  AND ($2 OR e.state IN ('PUBLISHED', 'CANCELLED'))
		  AND ($3 OR visibility.effectively_active)
		ORDER BY e.starts_at_local, lower(e.title)
	`, scopeID, includeDrafts, includeHiddenScopes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var series []model.EventSeries
	for rows.Next() {
		event, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		series = append(series, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachOverrides(ctx, series); err != nil {
		return nil, err
	}
	return series, nil
}

func (s *Store) GetSeries(ctx context.Context, id string) (model.EventSeries, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT e.id::text, e.uid, e.scope_id::text, s.name, s.kind::text, e.state::text,
		       e.title, e.description, e.location, e.url, e.starts_at_local, e.ends_at_local,
		       e.timezone, e.all_day, COALESCE(e.recurrence_until::text, ''), e.sequence,
		       e.published_at, e.cancelled_at, e.created_at, e.updated_at
		FROM event_series e JOIN calendar_scopes s ON s.id = e.scope_id
		WHERE e.id = $1
	`, id)
	event, err := scanSeries(row)
	if err != nil {
		return model.EventSeries{}, err
	}
	series := []model.EventSeries{event}
	if err := s.attachOverrides(ctx, series); err != nil {
		return model.EventSeries{}, err
	}
	return series[0], nil
}

func (s *Store) CreateSeries(ctx context.Context, event model.EventSeries) (model.EventSeries, error) {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.UID == "" {
		event.UID = uuid.NewString() + "@biotron.ca"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO event_series (
			id, uid, scope_id, title, description, location, url, starts_at_local,
			ends_at_local, timezone, all_day, recurrence_until
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, event.ID, event.UID, event.ScopeID, event.Title, event.Description, event.Location, event.URL,
		event.StartsAtLocal, event.EndsAtLocal, event.Timezone, event.AllDay, event.RecurrenceUntil)
	if err != nil {
		return model.EventSeries{}, mapConstraintError(err)
	}
	return s.GetSeries(ctx, event.ID)
}

func (s *Store) UpdateSeries(ctx context.Context, event model.EventSeries, expectedSequence int) (model.EventSeries, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE event_series SET
			scope_id = $2, title = $3, description = $4, location = $5, url = $6,
			starts_at_local = $7, ends_at_local = $8, timezone = $9, all_day = $10,
			recurrence_until = $11, sequence = sequence + 1, updated_at = now()
		WHERE id = $1 AND sequence = $12
	`, event.ID, event.ScopeID, event.Title, event.Description, event.Location, event.URL,
		event.StartsAtLocal, event.EndsAtLocal, event.Timezone, event.AllDay,
		event.RecurrenceUntil, expectedSequence)
	if err != nil {
		return model.EventSeries{}, mapConstraintError(err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.GetSeries(ctx, event.ID); errors.Is(err, ErrNotFound) {
			return model.EventSeries{}, ErrNotFound
		}
		return model.EventSeries{}, ErrConflict
	}
	return s.GetSeries(ctx, event.ID)
}

func (s *Store) PublishSeries(ctx context.Context, id string, expectedSequence int) (model.EventSeries, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE event_series SET state = 'PUBLISHED', published_at = COALESCE(published_at, now()),
			cancelled_at = NULL, sequence = sequence + 1, updated_at = now()
		WHERE id = $1 AND sequence = $2 AND state IN ('DRAFT', 'CANCELLED')
	`, id, expectedSequence)
	if err != nil {
		return model.EventSeries{}, err
	}
	if tag.RowsAffected() == 0 {
		return model.EventSeries{}, ErrConflict
	}
	return s.GetSeries(ctx, id)
}

func (s *Store) CancelSeries(ctx context.Context, id string, expectedSequence int) (model.EventSeries, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE event_series SET state = 'CANCELLED', cancelled_at = now(),
			sequence = sequence + 1, updated_at = now()
		WHERE id = $1 AND sequence = $2 AND state = 'PUBLISHED'
	`, id, expectedSequence)
	if err != nil {
		return model.EventSeries{}, err
	}
	if tag.RowsAffected() == 0 {
		return model.EventSeries{}, ErrConflict
	}
	return s.GetSeries(ctx, id)
}

func (s *Store) DeleteDraftSeries(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM event_series WHERE id = $1 AND state = 'DRAFT'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) UpsertOverride(ctx context.Context, override model.EventOverride, expectedSeriesSequence int) (model.EventOverride, error) {
	if override.ID == "" {
		override.ID = uuid.NewString()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.EventOverride{}, err
	}
	defer tx.Rollback(ctx)
	if err := bumpPublishedSeries(ctx, tx, override.SeriesID, expectedSeriesSequence); err != nil {
		return model.EventOverride{}, err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO event_overrides (id, series_id, recurrence_id_local, state, patch, sequence)
		VALUES ($1, $2, $3, $4::"EventOverrideState", $5, 1)
		ON CONFLICT (series_id, recurrence_id_local) DO UPDATE SET
			state = EXCLUDED.state, patch = EXCLUDED.patch,
			sequence = event_overrides.sequence + 1, updated_at = now()
		RETURNING id::text, series_id::text, recurrence_id_local, state::text,
		          patch, sequence, created_at, updated_at
	`, override.ID, override.SeriesID, override.RecurrenceIDLocal, override.State, override.Patch)
	saved, err := scanOverride(row)
	if err != nil {
		return model.EventOverride{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.EventOverride{}, err
	}
	return saved, nil
}

func (s *Store) ResetOverride(ctx context.Context, seriesID string, recurrenceID time.Time, expectedSeriesSequence int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := bumpPublishedSeries(ctx, tx, seriesID, expectedSeriesSequence); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE event_overrides
		SET state = 'MODIFIED', patch = '{}', sequence = sequence + 1, updated_at = now()
		WHERE series_id = $1 AND recurrence_id_local = $2
	`, seriesID, recurrenceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func bumpPublishedSeries(ctx context.Context, tx pgx.Tx, seriesID string, expectedSequence int) error {
	tag, err := tx.Exec(ctx, `
		UPDATE event_series SET sequence = sequence + 1, updated_at = now()
		WHERE id = $1 AND sequence = $2 AND state = 'PUBLISHED'
	`, seriesID, expectedSequence)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) attachOverrides(ctx context.Context, series []model.EventSeries) error {
	if len(series) == 0 {
		return nil
	}
	byID := make(map[string]*model.EventSeries, len(series))
	for i := range series {
		byID[series[i].ID] = &series[i]
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, series_id::text, recurrence_id_local, state::text,
		       patch, sequence, created_at, updated_at
		FROM event_overrides ORDER BY recurrence_id_local
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		override, err := scanOverride(rows)
		if err != nil {
			return err
		}
		if event := byID[override.SeriesID]; event != nil {
			event.Overrides = append(event.Overrides, override)
		}
	}
	return rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanScope(row scanner) (model.Scope, error) {
	var scope model.Scope
	var parentID string
	if err := row.Scan(&scope.ID, &scope.Kind, &scope.Name, &scope.Slug, &scope.Status,
		&parentID, &scope.ArchivedAt, &scope.CreatedAt, &scope.UpdatedAt,
		&scope.ChildCount, &scope.EventCount); err != nil {
		return model.Scope{}, mapNotFound(err)
	}
	if parentID != "" {
		scope.ParentID = &parentID
	}
	return scope, nil
}

func scanSeries(row scanner) (model.EventSeries, error) {
	var event model.EventSeries
	var recurrenceUntil string
	if err := row.Scan(&event.ID, &event.UID, &event.ScopeID, &event.ScopeName, &event.ScopeKind,
		&event.State, &event.Title, &event.Description, &event.Location, &event.URL,
		&event.StartsAtLocal, &event.EndsAtLocal, &event.Timezone, &event.AllDay,
		&recurrenceUntil, &event.Sequence, &event.PublishedAt, &event.CancelledAt,
		&event.CreatedAt, &event.UpdatedAt); err != nil {
		return model.EventSeries{}, mapNotFound(err)
	}
	if recurrenceUntil != "" {
		parsed, err := calendarlogic.ParseLocalDate(recurrenceUntil)
		if err != nil {
			return model.EventSeries{}, err
		}
		event.RecurrenceUntil = &parsed
	}
	return event, nil
}

func scanOverride(row scanner) (model.EventOverride, error) {
	var override model.EventOverride
	var patch []byte
	if err := row.Scan(&override.ID, &override.SeriesID, &override.RecurrenceIDLocal,
		&override.State, &patch, &override.Sequence, &override.CreatedAt, &override.UpdatedAt); err != nil {
		return model.EventOverride{}, mapNotFound(err)
	}
	if !json.Valid(patch) {
		return model.EventOverride{}, errors.New("stored override patch is invalid JSON")
	}
	override.Patch = json.RawMessage(patch)
	return override, nil
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

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}

func databaseConfig(raw string) (*pgxpool.Config, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	query := parsed.Query()
	schema := query.Get("schema")
	if schema == "" {
		schema = "calendar"
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

func Slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var output strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			output.WriteRune(r)
			lastDash = false
		} else if !lastDash && output.Len() > 0 {
			output.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(output.String(), "-")
}
