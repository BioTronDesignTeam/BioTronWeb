package ical

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	calendarlogic "github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

const productID = "-//BioTron Design Team//BioTron Calendar//EN"

type Feed struct {
	Content      []byte
	LastModified time.Time
}

func Build(name, sourceURL string, series []model.EventSeries, location *time.Location) (Feed, error) {
	lines := []string{
		"BEGIN:VCALENDAR",
		"PRODID:" + productID,
		"VERSION:2.0",
		"CALSCALE:GREGORIAN",
		"X-WR-CALNAME:" + escapeText(name),
		"X-WR-TIMEZONE:America/Toronto",
		"REFRESH-INTERVAL;VALUE=DURATION:PT1H",
		"X-PUBLISHED-TTL:PT1H",
	}
	if sourceURL != "" {
		lines = append(lines, "URL:"+safeValue(sourceURL))
	}
	lines = append(lines, torontoTimezoneLines()...)

	lastModified := time.Time{}
	for _, event := range series {
		if event.State != model.EventPublished && event.State != model.EventCancelled {
			continue
		}
		master, err := masterEventLines(event, location)
		if err != nil {
			return Feed{}, err
		}
		lines = append(lines, master...)
		if event.UpdatedAt.After(lastModified) {
			lastModified = event.UpdatedAt
		}
		for _, override := range event.Overrides {
			exception, err := overrideEventLines(event, override, location)
			if err != nil {
				return Feed{}, err
			}
			lines = append(lines, exception...)
			if override.UpdatedAt.After(lastModified) {
				lastModified = override.UpdatedAt
			}
		}
	}
	lines = append(lines, "END:VCALENDAR")

	if lastModified.IsZero() {
		lastModified = time.Now().UTC().Truncate(time.Second)
	}
	var output bytes.Buffer
	for _, line := range lines {
		output.WriteString(foldLine(line))
		output.WriteString("\r\n")
	}
	return Feed{Content: output.Bytes(), LastModified: lastModified}, nil
}

func masterEventLines(event model.EventSeries, location *time.Location) ([]string, error) {
	updated := event.UpdatedAt
	if updated.IsZero() {
		updated = time.Now()
	}
	lines := []string{
		"BEGIN:VEVENT",
		"UID:" + safeValue(event.UID),
		"DTSTAMP:" + utcDateTime(updated),
		"LAST-MODIFIED:" + utcDateTime(updated),
		fmt.Sprintf("SEQUENCE:%d", event.Sequence),
	}
	lines = append(lines, dateProperty("DTSTART", event.StartsAtLocal, event.AllDay, location))
	lines = append(lines, dateProperty("DTEND", event.EndsAtLocal, event.AllDay, location))
	if event.RecurrenceUntil != nil {
		starts, err := calendarlogic.GeneratedStarts(event)
		if err != nil {
			return nil, err
		}
		last := starts[len(starts)-1]
		if event.AllDay {
			lines = append(lines, "RRULE:FREQ=WEEKLY;UNTIL="+last.Format("20060102"))
		} else {
			lines = append(lines, "RRULE:FREQ=WEEKLY;UNTIL="+calendarlogic.InLocation(last, location).UTC().Format("20060102T150405Z"))
		}
	}
	lines = appendDetails(lines, event.Title, event.Description, event.Location, event.URL)
	if event.State == model.EventCancelled {
		lines = append(lines, "STATUS:CANCELLED")
	} else {
		lines = append(lines, "STATUS:CONFIRMED")
	}
	return append(lines, "END:VEVENT"), nil
}

func overrideEventLines(event model.EventSeries, override model.EventOverride, location *time.Location) ([]string, error) {
	start := override.RecurrenceIDLocal
	end := start.Add(event.EndsAtLocal.Sub(event.StartsAtLocal))
	title, description, eventLocation, eventURL := event.Title, event.Description, event.Location, event.URL
	if override.State == model.OverrideModified {
		patch, err := calendarlogic.DecodePatch(override.Patch)
		if err != nil {
			return nil, err
		}
		if patch.Title != nil {
			title = *patch.Title
		}
		if patch.Description != nil {
			description = *patch.Description
		}
		if patch.Location != nil {
			eventLocation = *patch.Location
		}
		if patch.URL != nil {
			eventURL = *patch.URL
		}
		if patch.StartsAtLocal != nil {
			start = *patch.StartsAtLocal
		}
		if patch.EndsAtLocal != nil {
			end = *patch.EndsAtLocal
		} else if patch.StartsAtLocal != nil {
			end = start.Add(event.EndsAtLocal.Sub(event.StartsAtLocal))
		}
	}
	updated := override.UpdatedAt
	if event.UpdatedAt.After(updated) {
		updated = event.UpdatedAt
	}
	if updated.IsZero() {
		updated = time.Now()
	}
	lines := []string{
		"BEGIN:VEVENT",
		"UID:" + safeValue(event.UID),
		"RECURRENCE-ID" + datePropertySuffix(override.RecurrenceIDLocal, event.AllDay, location),
		"DTSTAMP:" + utcDateTime(updated),
		"LAST-MODIFIED:" + utcDateTime(updated),
		fmt.Sprintf("SEQUENCE:%d", event.Sequence+override.Sequence),
		dateProperty("DTSTART", start, event.AllDay, location),
		dateProperty("DTEND", end, event.AllDay, location),
	}
	lines = appendDetails(lines, title, description, eventLocation, eventURL)
	// A cancelled series cancels every one of its instances, including the ones
	// an editor had previously changed. Deriving the status from the override
	// alone left edited occurrences of a cancelled series sitting on every
	// subscriber's calendar permanently.
	if event.State == model.EventCancelled || override.State == model.OverrideCancelled {
		lines = append(lines, "STATUS:CANCELLED")
	} else {
		lines = append(lines, "STATUS:CONFIRMED")
	}
	return append(lines, "END:VEVENT"), nil
}

func appendDetails(lines []string, title, description, location, eventURL string) []string {
	lines = append(lines, "SUMMARY:"+escapeText(title))
	if description != "" {
		lines = append(lines, "DESCRIPTION:"+escapeText(description))
	}
	if location != "" {
		lines = append(lines, "LOCATION:"+escapeText(location))
	}
	if eventURL != "" {
		lines = append(lines, "URL:"+safeValue(eventURL))
	}
	return lines
}

func dateProperty(name string, value time.Time, allDay bool, location *time.Location) string {
	return name + datePropertySuffix(value, allDay, location)
}

func datePropertySuffix(value time.Time, allDay bool, location *time.Location) string {
	if allDay {
		return ";VALUE=DATE:" + value.Format("20060102")
	}
	return ";TZID=America/Toronto:" + calendarlogic.InLocation(value, location).Format("20060102T150405")
}

func utcDateTime(value time.Time) string {
	return value.UTC().Format("20060102T150405Z")
}

func escapeText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r\n", "\\n")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\n")
	value = strings.ReplaceAll(value, ";", "\\;")
	return strings.ReplaceAll(value, ",", "\\,")
}

func safeValue(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

func foldLine(line string) string {
	if len(line) <= 75 {
		return line
	}
	var output strings.Builder
	remaining := line
	limit := 75
	for len(remaining) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(remaining[cut]) {
			cut--
		}
		if cut == 0 {
			cut = limit
		}
		output.WriteString(remaining[:cut])
		output.WriteString("\r\n ")
		remaining = remaining[cut:]
		limit = 74
	}
	output.WriteString(remaining)
	return output.String()
}

func torontoTimezoneLines() []string {
	return []string{
		"BEGIN:VTIMEZONE",
		"TZID:America/Toronto",
		"X-LIC-LOCATION:America/Toronto",
		"BEGIN:DAYLIGHT",
		"TZOFFSETFROM:-0500",
		"TZOFFSETTO:-0400",
		"TZNAME:EDT",
		"DTSTART:19700308T020000",
		"RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU",
		"END:DAYLIGHT",
		"BEGIN:STANDARD",
		"TZOFFSETFROM:-0400",
		"TZOFFSETTO:-0500",
		"TZNAME:EST",
		"DTSTART:19701101T020000",
		"RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU",
		"END:STANDARD",
		"END:VTIMEZONE",
	}
}
