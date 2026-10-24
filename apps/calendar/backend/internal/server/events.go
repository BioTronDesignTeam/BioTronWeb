package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	calendarlogic "github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/store"
)

type eventInput struct {
	ScopeID          string `json:"scope_id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Location         string `json:"location"`
	URL              string `json:"url"`
	StartsAtLocal    string `json:"starts_at_local"`
	EndsAtLocal      string `json:"ends_at_local"`
	AllDay           bool   `json:"all_day"`
	RecurrenceUntil  string `json:"recurrence_until"`
	ExpectedSequence int    `json:"expected_sequence"`
}

func (h *Handler) listOccurrences(w http.ResponseWriter, r *http.Request) error {
	from, to, err := h.parseRange(r)
	if err != nil {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	scopeID := strings.TrimSpace(r.URL.Query().Get("scope_id"))
	if scopeID != "" && uuid.Validate(scopeID) != nil {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "scope_id must be a UUID"})
	}
	series, err := h.store.ListSeries(r.Context(), scopeID, false, h.window(from, to))
	if err != nil {
		return err
	}
	occurrences, err := calendarlogic.Expand(series, from, to, h.location)
	if err != nil {
		return err
	}
	if occurrences == nil {
		occurrences = []model.Occurrence{}
	}
	return writeJSON(w, http.StatusOK, occurrences)
}

const (
	upcomingDefaultLimit = 5
	upcomingMaxLimit     = 20
	upcomingDefaultDays  = 42
	upcomingMaxDays      = 90
)

// listUpcoming answers "what is on next" for every lightweight consumer — the
// BioTron site's calendar section, the Sprinter bot, anything after them — so
// that "which events are upcoming" is decided once, here, instead of being
// reimplemented per client. It returns the same occurrence objects as
// GET /v1/events, already filtered to occurrences still running or still to
// come, ordered by start, and truncated to limit.
func (h *Handler) listUpcoming(w http.ResponseWriter, r *http.Request) error {
	limit := clampQuery(r.URL.Query().Get("limit"), upcomingDefaultLimit, upcomingMaxLimit)
	days := clampQuery(r.URL.Query().Get("days"), upcomingDefaultDays, upcomingMaxDays)
	now := h.now().In(h.location)
	until := now.AddDate(0, 0, days)
	series, err := h.store.ListSeries(r.Context(), "", false, h.window(now, until))
	if err != nil {
		return err
	}
	occurrences, err := upcomingOccurrences(series, now, until, limit, h.location)
	if err != nil {
		return err
	}
	body, err := json.Marshal(occurrences)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	// The window slides with the clock, so an unchanged Last-Modified must never
	// be allowed to serve a stale 304; the content-hash ETag still can.
	return sendCacheable(w, r, body, latestChange(series), false)
}

// upcomingOccurrences is the rule this endpoint exists to own, kept out of the
// handler so it can be pinned against a fixture clock.
//
// An occurrence is upcoming while it is still to come OR still running, and
// stops being upcoming the moment it ends. The lower bound therefore applies to
// the occurrence's END, not its start, which is what keeps a long or all-day
// event that began days ago and finished yesterday out of the list while
// keeping tonight's in-progress build in it. And the bound is the instant of
// the request, not the start of today, which is the difference between "what is
// on next" and "what is on today".
//
// Both halves come from Expand's EndsAt.After(from) && StartsAt.Before(to), and
// it runs after expansion, so a weekly series contributes only the instances
// that fall in the window rather than being kept or dropped as a whole.
func upcomingOccurrences(series []model.EventSeries, now, until time.Time, limit int, location *time.Location) ([]model.Occurrence, error) {
	occurrences, err := calendarlogic.Expand(series, now, until, location)
	if err != nil {
		return nil, err
	}
	if len(occurrences) > limit {
		occurrences = occurrences[:limit]
	}
	if occurrences == nil {
		occurrences = []model.Occurrence{}
	}
	return occurrences, nil
}

// latestChange is the most recent edit across the series and overrides that
// could appear in a response, used as its Last-Modified.
func latestChange(series []model.EventSeries) time.Time {
	latest := time.Time{}
	for _, event := range series {
		if event.UpdatedAt.After(latest) {
			latest = event.UpdatedAt
		}
		for _, override := range event.Overrides {
			if override.UpdatedAt.After(latest) {
				latest = override.UpdatedAt
			}
		}
	}
	if latest.IsZero() {
		latest = time.Now()
	}
	return latest
}

// clampQuery clamps rather than rejects, so a consumer asking for more than the
// API will give gets the maximum instead of an error it has to handle. Every
// count it reads is at least 1.
func clampQuery(raw string, fallback, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	if value < 1 {
		return 1
	}
	if value > maximum {
		return maximum
	}
	return value
}

// window converts an instant range into the wall-clock range the schema stores,
// so a read only loads the series and overrides it can possibly render.
func (h *Handler) window(from, to time.Time) *store.Window {
	return &store.Window{
		From: calendarlogic.WallClock(from, h.location),
		To:   calendarlogic.WallClock(to, h.location),
	}
}

// The editor lists the whole calendar, past and future, drafts included, so
// this read is deliberately not windowed.
func (h *Handler) listAdminEvents(w http.ResponseWriter, r *http.Request) error {
	series, err := h.store.ListSeries(r.Context(), strings.TrimSpace(r.URL.Query().Get("scope_id")), true, nil)
	if err != nil {
		return err
	}
	if series == nil {
		series = []model.EventSeries{}
	}
	return writeJSON(w, http.StatusOK, series)
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) error {
	var body eventInput
	if err := decodeBody(r, &body); err != nil {
		return err
	}
	event, err := h.eventFromInput(body)
	if err != nil {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	active, err := h.store.ScopeIsActive(r.Context(), event.ScopeID)
	if err != nil {
		return err
	}
	if !active {
		return writeJSON(w, http.StatusConflict, map[string]any{"error": "events can only be created in an active scope"})
	}
	event, err = h.store.CreateSeries(r.Context(), event)
	if err != nil {
		return err
	}
	h.adminEvent(r, "Event created", map[string]any{
		"series_id": event.ID, "scope_id": event.ScopeID, "title": event.Title,
	})
	return writeJSON(w, http.StatusCreated, event)
}

func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) error {
	current, err := h.store.GetSeries(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	var body eventInput
	if err := decodeBody(r, &body); err != nil {
		return err
	}
	updated, err := h.eventFromInput(body)
	if err != nil {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	updated.ID = current.ID
	if current.State != model.EventDraft && updated.ScopeID != current.ScopeID {
		return writeJSON(w, http.StatusConflict, map[string]any{
			"error": "a published event cannot move feeds; cancel it and create a replacement",
		})
	}
	if updated.ScopeID != current.ScopeID {
		active, err := h.store.ScopeIsActive(r.Context(), updated.ScopeID)
		if err != nil {
			return err
		}
		if !active {
			return writeJSON(w, http.StatusConflict, map[string]any{"error": "the selected scope is archived"})
		}
	}
	if occurrenceChanges := calendarlogic.OccurrenceChanges(current.Overrides); len(occurrenceChanges) > 0 {
		structuralChange := !updated.StartsAtLocal.Equal(current.StartsAtLocal) ||
			updated.AllDay != current.AllDay || updated.Timezone != current.Timezone
		if structuralChange {
			return writeJSON(w, http.StatusConflict, map[string]any{
				"error": "remove occurrence changes before changing the series start, timezone, or all-day mode",
			})
		}
		if updated.RecurrenceUntil == nil && current.RecurrenceUntil != nil {
			return writeJSON(w, http.StatusConflict, map[string]any{
				"error": "remove occurrence changes before turning a recurring series into a one-off event",
			})
		}
		if updated.RecurrenceUntil != nil {
			for _, override := range occurrenceChanges {
				if localDateAfter(override.RecurrenceIDLocal, *updated.RecurrenceUntil) {
					return writeJSON(w, http.StatusConflict, map[string]any{
						"error": "the new series end date would discard an occurrence change",
					})
				}
			}
		}
	}
	updated, err = h.store.UpdateSeries(r.Context(), updated, body.ExpectedSequence)
	if err != nil {
		return err
	}
	h.adminEvent(r, "Event updated", map[string]any{
		"series_id": updated.ID, "sequence": updated.Sequence,
	})
	return writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) publishEvent(w http.ResponseWriter, r *http.Request) error {
	current, err := h.store.GetSeries(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	active, err := h.store.ScopeIsActive(r.Context(), current.ScopeID)
	if err != nil {
		return err
	}
	if !active {
		return writeJSON(w, http.StatusConflict, map[string]any{"error": "an event cannot be published in an archived scope"})
	}
	sequence, err := expectedSequence(r)
	if err != nil {
		return err
	}
	event, err := h.store.PublishSeries(r.Context(), current.ID, sequence)
	if err != nil {
		return err
	}
	h.adminEvent(r, "Event published", map[string]any{
		"series_id": event.ID, "scope_id": event.ScopeID, "title": event.Title,
	})
	return writeJSON(w, http.StatusOK, event)
}

func (h *Handler) cancelEvent(w http.ResponseWriter, r *http.Request) error {
	sequence, err := expectedSequence(r)
	if err != nil {
		return err
	}
	event, err := h.store.CancelSeries(r.Context(), r.PathValue("id"), sequence)
	if err != nil {
		return err
	}
	h.adminEvent(r, "Event cancelled", map[string]any{"series_id": event.ID})
	return writeJSON(w, http.StatusOK, event)
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) error {
	// The row is gone once the delete returns, so its id has to be read from
	// the path.
	seriesID := r.PathValue("id")
	deleted, err := h.store.DeleteSeries(r.Context(), seriesID)
	if err != nil {
		return err
	}
	// The row is gone, so this log line is the only record of what it was.
	h.adminEvent(r, "Event deleted", map[string]any{"series_id": seriesID, "title": deleted.Title, "state": deleted.State})
	return noContent(w)
}

func (h *Handler) upsertOccurrence(w http.ResponseWriter, r *http.Request) error {
	event, err := h.store.GetSeries(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if event.State != model.EventPublished {
		return writeJSON(w, http.StatusConflict, map[string]any{"error": "only a published series can have occurrence changes"})
	}
	if event.RecurrenceUntil == nil {
		return writeJSON(w, http.StatusConflict, map[string]any{"error": "one-off events must be edited as a whole event"})
	}
	var body struct {
		RecurrenceIDLocal string          `json:"recurrence_id_local"`
		State             string          `json:"state"`
		Patch             json.RawMessage `json:"patch"`
		ExpectedSequence  int             `json:"expected_sequence"`
	}
	if err := decodeBody(r, &body); err != nil {
		return err
	}
	if body.ExpectedSequence != event.Sequence {
		return store.ErrConflict
	}
	recurrenceID, err := calendarlogic.ParseLocalDateTime(body.RecurrenceIDLocal)
	if err != nil || !calendarlogic.IsGeneratedStart(event, recurrenceID) {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "recurrence_id_local is not an occurrence in this series"})
	}
	body.State = strings.ToUpper(strings.TrimSpace(body.State))
	if body.State != model.OverrideModified && body.State != model.OverrideCancelled {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "state must be MODIFIED or CANCELLED"})
	}
	patch := json.RawMessage(`{}`)
	if body.State == model.OverrideModified {
		patch, err = calendarlogic.ValidatePatch(body.Patch)
		if err != nil {
			return writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		decoded, err := calendarlogic.DecodePatch(patch)
		if err != nil {
			return writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		start := recurrenceID
		end := recurrenceID.Add(event.EndsAtLocal.Sub(event.StartsAtLocal))
		if decoded.StartsAtLocal != nil {
			start = *decoded.StartsAtLocal
		}
		if decoded.EndsAtLocal != nil {
			end = *decoded.EndsAtLocal
		} else if decoded.StartsAtLocal != nil {
			end = start.Add(event.EndsAtLocal.Sub(event.StartsAtLocal))
		}
		if !end.After(start) {
			return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "the occurrence must end after it starts"})
		}
		if event.AllDay && (start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 || end.Hour() != 0 || end.Minute() != 0 || end.Second() != 0) {
			return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "all-day occurrence boundaries must be at midnight"})
		}
		if decoded.Title != nil && strings.TrimSpace(*decoded.Title) == "" {
			return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "the occurrence title cannot be empty"})
		}
	}
	override, err := h.store.UpsertOverride(r.Context(), model.EventOverride{
		SeriesID: event.ID, RecurrenceIDLocal: recurrenceID, State: body.State, Patch: patch,
	}, body.ExpectedSequence)
	if err != nil {
		return err
	}
	h.adminEvent(r, "Occurrence changed", map[string]any{
		"series_id":           override.SeriesID,
		"recurrence_id_local": calendarlogic.FormatLocal(override.RecurrenceIDLocal),
		"cancelled":           override.State == model.OverrideCancelled,
	})
	return writeJSON(w, http.StatusOK, override)
}

func (h *Handler) deleteOccurrenceOverride(w http.ResponseWriter, r *http.Request) error {
	recurrenceID, err := calendarlogic.ParseLocalDateTime(r.URL.Query().Get("recurrence_id_local"))
	if err != nil {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	expected, err := strconv.Atoi(r.URL.Query().Get("expected_sequence"))
	if err != nil || expected < 1 {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expected_sequence is required"})
	}
	// The override is gone once the reset returns, so the series id has to be
	// read from the path.
	seriesID := r.PathValue("id")
	if err := h.store.ResetOverride(r.Context(), seriesID, recurrenceID, expected); err != nil {
		return err
	}
	h.adminEvent(r, "Occurrence reset", map[string]any{
		"series_id":           seriesID,
		"recurrence_id_local": calendarlogic.FormatLocal(recurrenceID),
	})
	return noContent(w)
}

func (h *Handler) eventFromInput(body eventInput) (model.EventSeries, error) {
	body.ScopeID = strings.TrimSpace(body.ScopeID)
	body.Title = strings.TrimSpace(body.Title)
	body.Description = strings.TrimSpace(body.Description)
	body.Location = strings.TrimSpace(body.Location)
	body.URL = strings.TrimSpace(body.URL)
	if body.ScopeID == "" || len(body.Title) < 2 || len(body.Title) > 160 {
		return model.EventSeries{}, errors.New("scope_id and a title between 2 and 160 characters are required")
	}
	if len(body.Description) > 5000 || len(body.Location) > 300 || len(body.URL) > 1000 {
		return model.EventSeries{}, errors.New("one or more event fields are too long")
	}
	if body.URL != "" {
		parsed, err := url.ParseRequestURI(body.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return model.EventSeries{}, errors.New("url must be an http or https URL")
		}
	}
	startsAt, err := calendarlogic.ParseLocalDateTime(body.StartsAtLocal)
	if err != nil {
		return model.EventSeries{}, err
	}
	endsAt, err := calendarlogic.ParseLocalDateTime(body.EndsAtLocal)
	if err != nil {
		return model.EventSeries{}, err
	}
	if !endsAt.After(startsAt) {
		return model.EventSeries{}, errors.New("the event must end after it starts")
	}
	if body.AllDay && (startsAt.Hour() != 0 || startsAt.Minute() != 0 || endsAt.Hour() != 0 || endsAt.Minute() != 0) {
		return model.EventSeries{}, errors.New("all-day event boundaries must be at midnight")
	}
	var recurrenceUntil *time.Time
	if strings.TrimSpace(body.RecurrenceUntil) != "" {
		parsed, err := calendarlogic.ParseLocalDate(body.RecurrenceUntil)
		if err != nil {
			return model.EventSeries{}, err
		}
		recurrenceUntil = &parsed
	}
	event := model.EventSeries{
		ScopeID: body.ScopeID, Title: body.Title, Description: body.Description,
		Location: body.Location, URL: body.URL, StartsAtLocal: startsAt, EndsAtLocal: endsAt,
		Timezone: h.config.DefaultTimezone, AllDay: body.AllDay, RecurrenceUntil: recurrenceUntil,
	}
	if _, err := calendarlogic.GeneratedStarts(event); err != nil {
		return model.EventSeries{}, err
	}
	return event, nil
}

func (h *Handler) parseRange(r *http.Request) (time.Time, time.Time, error) {
	today := time.Now().In(h.location)
	fromDate := queryOr(r, "from", today.Format(calendarlogic.LocalDateLayout))
	toDate := queryOr(r, "to", today.AddDate(0, 3, 0).Format(calendarlogic.LocalDateLayout))
	fromLocal, err := calendarlogic.ParseLocalDate(fromDate)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	toLocal, err := calendarlogic.ParseLocalDate(toDate)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	from := calendarlogic.InLocation(fromLocal, h.location)
	to := calendarlogic.InLocation(toLocal, h.location)
	if !to.After(from) {
		return time.Time{}, time.Time{}, errors.New("to must be after from")
	}
	if to.Sub(from) > time.Duration(h.config.MaxRangeDays)*24*time.Hour {
		return time.Time{}, time.Time{}, errors.New("requested calendar range is too large")
	}
	return from, to, nil
}

func expectedSequence(r *http.Request) (int, error) {
	var body struct {
		ExpectedSequence *int `json:"expected_sequence"`
	}
	if err := decodeBody(r, &body); err != nil {
		return 0, err
	}
	if body.ExpectedSequence == nil {
		return 0, errBadRequest
	}
	return *body.ExpectedSequence, nil
}

func localDateAfter(value, date time.Time) bool {
	valueDate := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	dateOnly := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	return valueDate.After(dateOnly)
}
