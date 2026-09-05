package calendar

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

const (
	LocalDateTimeLayout = "2006-01-02T15:04:05"
	LocalDateLayout     = "2006-01-02"
	maxOccurrences      = 1000
)

var allowedPatchKeys = map[string]bool{
	"title":           true,
	"description":     true,
	"location":        true,
	"url":             true,
	"starts_at_local": true,
	"ends_at_local":   true,
}

type occurrencePatch struct {
	Title         *string
	Description   *string
	Location      *string
	URL           *string
	StartsAtLocal *time.Time
	EndsAtLocal   *time.Time
}

func ParseLocalDateTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{LocalDateTimeLayout, "2006-01-02T15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("local date-time must use YYYY-MM-DDTHH:MM[:SS]")
}

func ParseLocalDate(value string) (time.Time, error) {
	parsed, err := time.ParseInLocation(LocalDateLayout, strings.TrimSpace(value), time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must use YYYY-MM-DD")
	}
	return parsed, nil
}

func FormatLocal(value time.Time) string {
	return value.Format(LocalDateTimeLayout)
}

// InLocation projects a stored Toronto wall-clock value onto the instant it
// names. Series are stored and stepped as wall clock, so this is the only place
// daylight saving is resolved, and the policy is RFC 5545 section 3.3.5 —
// the same rule every subscriber's calendar client applies to the TZID values
// in our feeds, so the app and the feed always agree:
//
//   - Ambiguous wall clock (the hour repeated at fall-back, e.g. 01:30 on
//     2026-11-01) resolves to the FIRST of the two instants, the offset still
//     in force before the transition. Go's time.Date already does this.
//   - Nonexistent wall clock (the hour skipped at spring-forward, e.g. 02:30 on
//     2026-03-08) is interpreted with the offset in force BEFORE the gap, which
//     lands on the first instant after it, 03:30 EDT. Go's time.Date instead
//     normalises backwards to 01:30 EST, an hour EARLIER than the meeting a
//     subscriber's client would show, so that case is corrected here.
//
// A meeting is only ever moved when its local time genuinely does not exist;
// every other wall clock is preserved exactly, which is what keeps a weekly
// series at the same local hour across a DST change.
func InLocation(value time.Time, location *time.Location) time.Time {
	projected := time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), location)
	if sameWallClock(projected, value) {
		return projected
	}
	// Go landed before the gap, so its zone is the pre-gap offset. Re-apply the
	// requested wall clock with that offset to move forward across the gap.
	_, offsetBeforeGap := projected.Zone()
	frame := time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), time.UTC)
	return frame.Add(-time.Duration(offsetBeforeGap) * time.Second).In(location)
}

// WallClock is the inverse of InLocation: the location-local wall clock of an
// instant, expressed in the UTC frame the schema stores its TIMESTAMP columns
// in. Query bounds have to be compared in that frame, not as instants.
func WallClock(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), 0, time.UTC)
}

func sameWallClock(projected, wall time.Time) bool {
	return projected.Year() == wall.Year() && projected.Month() == wall.Month() &&
		projected.Day() == wall.Day() && projected.Hour() == wall.Hour() &&
		projected.Minute() == wall.Minute() && projected.Second() == wall.Second()
}

func ValidatePatch(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, errors.New("patch must be a JSON object")
	}
	for key, value := range values {
		if !allowedPatchKeys[key] {
			return nil, fmt.Errorf("patch field %q is not allowed", key)
		}
		if key == "starts_at_local" || key == "ends_at_local" {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return nil, fmt.Errorf("%s cannot be cleared", key)
			}
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return nil, fmt.Errorf("%s must be a local date-time string", key)
			}
			if _, err := ParseLocalDateTime(text); err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			continue
		}
		if !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return nil, fmt.Errorf("%s must be a string or null", key)
			}
			switch key {
			case "title":
				if len(strings.TrimSpace(text)) < 2 || len(text) > 160 {
					return nil, errors.New("title must be between 2 and 160 characters")
				}
			case "description":
				if len(text) > 5000 {
					return nil, errors.New("description must not exceed 5000 characters")
				}
			case "location":
				if len(text) > 300 {
					return nil, errors.New("location must not exceed 300 characters")
				}
			case "url":
				if len(text) > 1000 {
					return nil, errors.New("url must not exceed 1000 characters")
				}
			}
			if key == "url" && text != "" {
				parsed, err := url.ParseRequestURI(text)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
					return nil, errors.New("url must be an http or https URL")
				}
			}
		}
	}
	return json.Marshal(values)
}

func PatchIsEmpty(raw json.RawMessage) bool {
	var values map[string]json.RawMessage
	return json.Unmarshal(raw, &values) == nil && len(values) == 0
}

// IsNoOpOverride reports whether an override row carries no occurrence change.
// Resetting an occurrence deletes its row, but releases before that fix wrote
// an empty MODIFIED patch as a tombstone instead, and those rows survive in
// existing databases. They must behave exactly like a plain generated
// occurrence: no feed exception, and no block on series-level edits.
func IsNoOpOverride(override model.EventOverride) bool {
	return override.State == model.OverrideModified && PatchIsEmpty(override.Patch)
}

// OccurrenceChanges drops no-op override rows, leaving only the overrides that
// genuinely change or cancel an occurrence.
func OccurrenceChanges(overrides []model.EventOverride) []model.EventOverride {
	changes := make([]model.EventOverride, 0, len(overrides))
	for _, override := range overrides {
		if IsNoOpOverride(override) {
			continue
		}
		changes = append(changes, override)
	}
	return changes
}

func DecodePatch(raw json.RawMessage) (occurrencePatch, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return occurrencePatch{}, err
	}
	var patch occurrencePatch
	decodeString := func(key string, destination **string) error {
		value, present := values[key]
		if !present {
			return nil
		}
		text := ""
		if !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			if err := json.Unmarshal(value, &text); err != nil {
				return err
			}
		}
		*destination = &text
		return nil
	}
	if err := decodeString("title", &patch.Title); err != nil {
		return occurrencePatch{}, err
	}
	if err := decodeString("description", &patch.Description); err != nil {
		return occurrencePatch{}, err
	}
	if err := decodeString("location", &patch.Location); err != nil {
		return occurrencePatch{}, err
	}
	if err := decodeString("url", &patch.URL); err != nil {
		return occurrencePatch{}, err
	}
	for key, destination := range map[string]**time.Time{
		"starts_at_local": &patch.StartsAtLocal,
		"ends_at_local":   &patch.EndsAtLocal,
	} {
		value, present := values[key]
		if !present {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return occurrencePatch{}, err
		}
		parsed, err := ParseLocalDateTime(text)
		if err != nil {
			return occurrencePatch{}, err
		}
		*destination = &parsed
	}
	return patch, nil
}

func GeneratedStarts(series model.EventSeries) ([]time.Time, error) {
	starts := []time.Time{series.StartsAtLocal}
	if series.RecurrenceUntil == nil {
		return starts, nil
	}
	until := *series.RecurrenceUntil
	if dateAfter(series.StartsAtLocal, until) {
		return nil, errors.New("recurrence end is before the first occurrence")
	}
	for next := series.StartsAtLocal.AddDate(0, 0, 7); !dateAfter(next, until); next = next.AddDate(0, 0, 7) {
		if len(starts) >= maxOccurrences {
			return nil, errors.New("recurrence exceeds 1000 occurrences")
		}
		starts = append(starts, next)
	}
	return starts, nil
}

func IsGeneratedStart(series model.EventSeries, candidate time.Time) bool {
	starts, err := GeneratedStarts(series)
	if err != nil {
		return false
	}
	for _, start := range starts {
		if start.Equal(candidate) {
			return true
		}
	}
	return false
}

func Expand(series []model.EventSeries, from, to time.Time, location *time.Location) ([]model.Occurrence, error) {
	var occurrences []model.Occurrence
	for _, event := range series {
		if event.State != model.EventPublished {
			continue
		}
		starts, err := GeneratedStarts(event)
		if err != nil {
			return nil, fmt.Errorf("expand %s: %w", event.ID, err)
		}
		overrides := make(map[string]model.EventOverride, len(event.Overrides))
		for _, override := range OccurrenceChanges(event.Overrides) {
			overrides[FormatLocal(override.RecurrenceIDLocal)] = override
		}
		duration := event.EndsAtLocal.Sub(event.StartsAtLocal)
		for _, generatedStart := range starts {
			generatedEnd := generatedStart.Add(duration)
			occurrence := model.Occurrence{
				SeriesID:       event.ID,
				UID:            event.UID,
				ScopeID:        event.ScopeID,
				ScopeName:      event.ScopeName,
				ScopePath:      event.ScopePath,
				ScopeKind:      event.ScopeKind,
				Title:          event.Title,
				Description:    event.Description,
				Location:       event.Location,
				URL:            event.URL,
				AllDay:         event.AllDay,
				Timezone:       event.Timezone,
				RecurrenceID:   FormatLocal(generatedStart),
				Recurring:      event.RecurrenceUntil != nil,
				SeriesSequence: event.Sequence,
			}
			if override, ok := overrides[occurrence.RecurrenceID]; ok {
				if override.State == model.OverrideCancelled {
					continue
				}
				patch, err := DecodePatch(override.Patch)
				if err != nil {
					return nil, fmt.Errorf("decode override %s: %w", override.ID, err)
				}
				if patch.Title != nil {
					occurrence.Title = *patch.Title
				}
				if patch.Description != nil {
					occurrence.Description = *patch.Description
				}
				if patch.Location != nil {
					occurrence.Location = *patch.Location
				}
				if patch.URL != nil {
					occurrence.URL = *patch.URL
				}
				if patch.StartsAtLocal != nil {
					generatedStart = *patch.StartsAtLocal
				}
				if patch.EndsAtLocal != nil {
					generatedEnd = *patch.EndsAtLocal
				} else if patch.StartsAtLocal != nil {
					generatedEnd = generatedStart.Add(duration)
				}
				occurrence.Modified = !PatchIsEmpty(override.Patch)
				occurrence.OverrideSequence = override.Sequence
			}
			if !generatedEnd.After(generatedStart) {
				return nil, fmt.Errorf("event %s occurrence %s ends before it starts", event.ID, occurrence.RecurrenceID)
			}
			occurrence.StartsAt = InLocation(generatedStart, location)
			occurrence.EndsAt = InLocation(generatedEnd, location)
			if occurrence.EndsAt.After(from) && occurrence.StartsAt.Before(to) {
				occurrences = append(occurrences, occurrence)
			}
		}
	}
	sort.Slice(occurrences, func(i, j int) bool {
		if occurrences[i].StartsAt.Equal(occurrences[j].StartsAt) {
			return occurrences[i].Title < occurrences[j].Title
		}
		return occurrences[i].StartsAt.Before(occurrences[j].StartsAt)
	})
	return occurrences, nil
}

func dateAfter(value, date time.Time) bool {
	valueDate := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	dateOnly := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	return valueDate.After(dateOnly)
}
