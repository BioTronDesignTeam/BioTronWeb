package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	calendarlogic "github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/store/queries"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	schemaName  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// Store turns the sqlc queries in package queries into the model the handlers
// use. The SQL itself lives in queries/queries.sql.
type Store struct {
	pool    *pgxpool.Pool
	queries *queries.Queries
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
	return &Store{pool: pool, queries: queries.New(pool)}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) ListScopes(ctx context.Context, includeArchived bool) ([]model.Scope, error) {
	rows, err := s.queries.ListScopes(ctx, includeArchived)
	if err != nil {
		return nil, err
	}
	var scopes []model.Scope
	for _, row := range rows {
		scopes = append(scopes, scopeFromRow(row))
	}
	return scopes, nil
}

func (s *Store) GetScope(ctx context.Context, id string) (model.Scope, error) {
	row, err := s.queries.GetScope(ctx, id)
	if err != nil {
		return model.Scope{}, mapNotFound(err)
	}
	return scopeFromRow(queries.ListScopesRow(row)), nil
}

func (s *Store) CreateScope(ctx context.Context, scope model.Scope) (model.Scope, error) {
	if scope.ID == "" {
		scope.ID = uuid.NewString()
	}
	err := s.queries.CreateScope(ctx, queries.CreateScopeParams{
		ID: scope.ID, Kind: queries.CalendarScopeKind(scope.Kind), Name: scope.Name,
		Slug: scope.Slug, ParentID: value(scope.ParentID),
	})
	if err != nil {
		return model.Scope{}, mapConstraintError(err)
	}
	return s.GetScope(ctx, scope.ID)
}

func (s *Store) RenameScope(ctx context.Context, id, name, slug string) (model.Scope, error) {
	affected, err := s.queries.RenameScope(ctx, queries.RenameScopeParams{ID: id, Name: name, Slug: slug})
	if err != nil {
		return model.Scope{}, mapConstraintError(err)
	}
	if affected == 0 {
		return model.Scope{}, ErrNotFound
	}
	return s.GetScope(ctx, id)
}

func (s *Store) SetScopeArchived(ctx context.Context, id string, archived bool) (model.Scope, error) {
	status := model.ScopeActive
	if archived {
		status = model.ScopeArchived
	}
	affected, err := s.queries.SetScopeStatus(ctx, queries.SetScopeStatusParams{
		ID: id, Status: queries.CalendarScopeStatus(status),
	})
	if err != nil {
		return model.Scope{}, err
	}
	if affected == 0 {
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
	affected, err := s.queries.DeleteScope(ctx, id)
	if err != nil {
		return mapConstraintError(err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ScopeIsActive(ctx context.Context, id string) (bool, error) {
	return s.queries.ScopeIsActive(ctx, id)
}

// Window is a wall-clock range, in the UTC frame the schema stores, used to
// load only the rows a read can possibly need. A nil Window loads everything.
type Window struct {
	From time.Time
	To   time.Time
}

// widened pads the window by a day on each side. The stored wall clock maps to
// an instant through a daylight-saving resolution that can move a value forward
// by an hour, and an all-day boundary sits at midnight, so the SQL bound is kept
// deliberately loose. Expand still applies the exact rule; this only decides
// which rows are worth fetching.
func (w *Window) widened() (from, to *time.Time) {
	if w == nil {
		return nil, nil
	}
	widenedFrom, widenedTo := w.From.AddDate(0, 0, -1), w.To.AddDate(0, 0, 1)
	return &widenedFrom, &widenedTo
}

// ListSeries loads every series in scope. A non-nil window restricts the load to
// series that could produce an occurrence inside it, which is what the public
// JSON reads want; the editor passes nil because it lists the whole calendar.
func (s *Store) ListSeries(ctx context.Context, scopeID string, includeDrafts bool, window *Window) ([]model.EventSeries, error) {
	return s.listSeries(ctx, scopeID, includeDrafts, includeDrafts, window)
}

// ListFeedSeries retains published history for stable subscription URLs even
// when a scope is archived. Public JSON views use ListSeries and hide archived
// scope trees; feeds continue to reconcile existing subscriber calendars.
//
// Feeds are deliberately never windowed. A subscription is reconciled against
// the whole document, so dropping a past or expired VEVENT would tell a client
// to delete a meeting that actually happened.
func (s *Store) ListFeedSeries(ctx context.Context, scopeID string) ([]model.EventSeries, error) {
	return s.listSeries(ctx, scopeID, false, true, nil)
}

func (s *Store) listSeries(ctx context.Context, scopeID string, includeDrafts, includeHiddenScopes bool, window *Window) ([]model.EventSeries, error) {
	windowFrom, windowTo := window.widened()
	rows, err := s.queries.ListSeries(ctx, queries.ListSeriesParams{
		ScopeID: scopeID, IncludeDrafts: includeDrafts, IncludeHiddenScopes: includeHiddenScopes,
		WindowFrom: windowFrom, WindowTo: windowTo,
	})
	if err != nil {
		return nil, err
	}
	var series []model.EventSeries
	for _, row := range rows {
		event, err := seriesFromRow(row)
		if err != nil {
			return nil, err
		}
		series = append(series, event)
	}
	if err := s.attachOverrides(ctx, series, window); err != nil {
		return nil, err
	}
	return series, nil
}

func (s *Store) GetSeries(ctx context.Context, id string) (model.EventSeries, error) {
	row, err := s.queries.GetSeries(ctx, id)
	if err != nil {
		return model.EventSeries{}, mapNotFound(err)
	}
	event, err := seriesFromRow(queries.ListSeriesRow(row))
	if err != nil {
		return model.EventSeries{}, err
	}
	series := []model.EventSeries{event}
	// The editor and the update guard both need every override on the series,
	// so this path is never windowed.
	if err := s.attachOverrides(ctx, series, nil); err != nil {
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
	err := s.queries.CreateSeries(ctx, queries.CreateSeriesParams{
		ID: event.ID, Uid: event.UID, ScopeID: event.ScopeID, Title: event.Title,
		Description: event.Description, Location: event.Location, Url: event.URL,
		StartsAtLocal: event.StartsAtLocal, EndsAtLocal: event.EndsAtLocal, Timezone: event.Timezone,
		AllDay: event.AllDay, RecurrenceUntil: event.RecurrenceUntil,
	})
	if err != nil {
		return model.EventSeries{}, mapConstraintError(err)
	}
	return s.GetSeries(ctx, event.ID)
}

func (s *Store) UpdateSeries(ctx context.Context, event model.EventSeries, expectedSequence int) (model.EventSeries, error) {
	var affected int64
	if sequence, ok := sequenceParam(expectedSequence); ok {
		var err error
		affected, err = s.queries.UpdateSeries(ctx, queries.UpdateSeriesParams{
			ID: event.ID, ScopeID: event.ScopeID, Title: event.Title, Description: event.Description,
			Location: event.Location, Url: event.URL, StartsAtLocal: event.StartsAtLocal,
			EndsAtLocal: event.EndsAtLocal, Timezone: event.Timezone, AllDay: event.AllDay,
			RecurrenceUntil: event.RecurrenceUntil, ExpectedSequence: sequence,
		})
		if err != nil {
			return model.EventSeries{}, mapConstraintError(err)
		}
	}
	if affected == 0 {
		if _, err := s.GetSeries(ctx, event.ID); errors.Is(err, ErrNotFound) {
			return model.EventSeries{}, ErrNotFound
		}
		return model.EventSeries{}, ErrConflict
	}
	return s.GetSeries(ctx, event.ID)
}

func (s *Store) PublishSeries(ctx context.Context, id string, expectedSequence int) (model.EventSeries, error) {
	sequence, ok := sequenceParam(expectedSequence)
	if !ok {
		return model.EventSeries{}, ErrConflict
	}
	affected, err := s.queries.PublishSeries(ctx, queries.PublishSeriesParams{ID: id, ExpectedSequence: sequence})
	if err != nil {
		return model.EventSeries{}, err
	}
	if affected == 0 {
		return model.EventSeries{}, ErrConflict
	}
	return s.GetSeries(ctx, id)
}

func (s *Store) CancelSeries(ctx context.Context, id string, expectedSequence int) (model.EventSeries, error) {
	sequence, ok := sequenceParam(expectedSequence)
	if !ok {
		return model.EventSeries{}, ErrConflict
	}
	affected, err := s.queries.CancelSeries(ctx, queries.CancelSeriesParams{ID: id, ExpectedSequence: sequence})
	if err != nil {
		return model.EventSeries{}, err
	}
	if affected == 0 {
		return model.EventSeries{}, ErrConflict
	}
	return s.GetSeries(ctx, id)
}

// DeletedSeries is what is left to say about a series once its row is gone.
type DeletedSeries struct {
	Title string
	State string
}

// DeleteSeries removes a series in any state, and its overrides with it (the
// foreign key cascades). It used to refuse anything but a draft, which left no
// way to take a test event or a mistake off a live calendar: cancelling keeps
// the event in every feed, marked cancelled, for good.
//
// A deleted event simply stops appearing in the feeds. A subscribed calendar
// mirrors its feed, so it drops the event on its next refresh. Nobody is told
// it was called off; an editor who wants that cancels the event instead.
func (s *Store) DeleteSeries(ctx context.Context, id string) (DeletedSeries, error) {
	row, err := s.queries.DeleteSeries(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedSeries{}, ErrNotFound
	}
	return DeletedSeries{Title: row.Title, State: row.State}, err
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
	inTx := s.queries.WithTx(tx)
	if err := bumpPublishedSeries(ctx, inTx, override.SeriesID, expectedSeriesSequence); err != nil {
		return model.EventOverride{}, err
	}
	row, err := inTx.UpsertOverride(ctx, queries.UpsertOverrideParams{
		ID: override.ID, SeriesID: override.SeriesID, RecurrenceIDLocal: override.RecurrenceIDLocal,
		State: queries.EventOverrideState(override.State), Patch: override.Patch,
	})
	if err != nil {
		return model.EventOverride{}, mapNotFound(err)
	}
	saved, err := overrideFromRow(queries.ListOverridesRow(row))
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
	inTx := s.queries.WithTx(tx)
	if err := bumpPublishedSeries(ctx, inTx, seriesID, expectedSeriesSequence); err != nil {
		return err
	}
	// Reset returns the occurrence to the series schedule, so the override row
	// is removed rather than tombstoned. A retained empty-patch row would keep
	// tripping the "remove occurrence changes first" guard on series edits with
	// nothing left in the UI to remove. The series sequence bump above is what
	// tells subscribers to re-read the feed.
	affected, err := inTx.DeleteOverride(ctx, queries.DeleteOverrideParams{
		SeriesID: seriesID, RecurrenceIDLocal: recurrenceID,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func bumpPublishedSeries(ctx context.Context, inTx *queries.Queries, seriesID string, expectedSequence int) error {
	sequence, ok := sequenceParam(expectedSequence)
	if !ok {
		return ErrConflict
	}
	affected, err := inTx.BumpPublishedSeries(ctx, queries.BumpPublishedSeriesParams{
		ID: seriesID, ExpectedSequence: sequence,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) attachOverrides(ctx context.Context, series []model.EventSeries, window *Window) error {
	if len(series) == 0 {
		return nil
	}
	byID := make(map[string]*model.EventSeries, len(series))
	ids := make([]string, 0, len(series))
	for i := range series {
		byID[series[i].ID] = &series[i]
		ids = append(ids, series[i].ID)
	}
	windowFrom, windowTo := window.widened()
	// Reading the whole override table to answer one request was survivable
	// while this only ran for the editor. It now runs on every load of the
	// public site, so it is scoped to the series actually loaded and, when the
	// caller has a window, to the overrides that can affect it.
	//
	// An override only reaches outside its own recurrence slot when it moves the
	// occurrence in time, so a patch carrying either boundary is always loaded:
	// that is how a meeting dragged across months still shows up. Everything
	// else stays where its recurrence id puts it.
	rows, err := s.queries.ListOverrides(ctx, queries.ListOverridesParams{
		SeriesIds: ids, WindowFrom: windowFrom, WindowTo: windowTo,
	})
	if err != nil {
		return err
	}
	for _, row := range rows {
		override, err := overrideFromRow(row)
		if err != nil {
			return err
		}
		if event := byID[override.SeriesID]; event != nil {
			event.Overrides = append(event.Overrides, override)
		}
	}
	return nil
}

func scopeFromRow(row queries.ListScopesRow) model.Scope {
	scope := model.Scope{
		ID: row.ID, Kind: row.Kind, Name: row.Name, Slug: row.Slug,
		Path: row.Path, SlugPath: row.SlugPath, Status: row.Status,
		ArchivedAt: row.ArchivedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		ChildCount: int(row.ChildCount), EventCount: int(row.EventCount),
	}
	if row.ParentID != "" {
		parentID := row.ParentID
		scope.ParentID = &parentID
	}
	return scope
}

func seriesFromRow(row queries.ListSeriesRow) (model.EventSeries, error) {
	event := model.EventSeries{
		ID: row.ID, UID: row.Uid, ScopeID: row.ScopeID, ScopeName: row.ScopeName,
		ScopePath: row.ScopePath, ScopeKind: row.ScopeKind, State: row.State,
		Title: row.Title, Description: row.Description, Location: row.Location, URL: row.Url,
		StartsAtLocal: row.StartsAtLocal, EndsAtLocal: row.EndsAtLocal,
		Timezone: row.Timezone, AllDay: row.AllDay, Sequence: int(row.Sequence),
		PublishedAt: row.PublishedAt, CancelledAt: row.CancelledAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.RecurrenceUntil != "" {
		parsed, err := calendarlogic.ParseLocalDate(row.RecurrenceUntil)
		if err != nil {
			return model.EventSeries{}, err
		}
		event.RecurrenceUntil = &parsed
	}
	return event, nil
}

func overrideFromRow(row queries.ListOverridesRow) (model.EventOverride, error) {
	if !json.Valid(row.Patch) {
		return model.EventOverride{}, errors.New("stored override patch is invalid JSON")
	}
	return model.EventOverride{
		ID: row.ID, SeriesID: row.SeriesID, RecurrenceIDLocal: row.RecurrenceIDLocal,
		State: row.State, Patch: json.RawMessage(row.Patch), Sequence: int(row.Sequence),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

// sequenceParam narrows an expected sequence to the column's int4. A value
// outside that range can never match a stored sequence, so it is the stale
// write that the caller answers with a conflict.
func sequenceParam(sequence int) (int32, bool) {
	if sequence < math.MinInt32 || sequence > math.MaxInt32 {
		return 0, false
	}
	return int32(sequence), true
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
