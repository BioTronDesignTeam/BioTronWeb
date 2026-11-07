package calendar

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/common"
)

// Status is what Calendar says about one occurrence that a windowed fetch no
// longer returned.
type Status int

const (
	// StatusExists means the occurrence is still on the calendar. It left the
	// fetch for another reason: it moved past the window's end, its scope was
	// archived, or it moved to another scope.
	StatusExists Status = iota
	// StatusCancelled means Calendar marks the occurrence, or its whole
	// series, as cancelled.
	StatusCancelled
	// StatusDeleted means the occurrence is gone: its series was deleted, or
	// the series no longer generates that start, because its end date or its
	// time moved.
	StatusDeleted
)

func (s Status) String() string {
	switch s {
	case StatusCancelled:
		return "cancelled"
	case StatusDeleted:
		return "deleted"
	default:
		return "exists"
	}
}

// feedLimit bounds the whole-calendar feed. It holds every published series
// once, not every occurrence, so this is generous.
const feedLimit = 16 << 20

// Feed is Calendar's All Events feed, indexed by uid. Unlike /v1/events, the
// feed keeps cancelled series and occurrences, marked STATUS:CANCELLED, and
// keeps events in archived scopes. A series that is not in it at all was
// deleted. That makes it the one public source that can tell "cancelled"
// from "no longer in this view".
type Feed struct {
	series   map[string]*feedSeries
	location *time.Location
}

type feedSeries struct {
	cancelled bool
	// start is the first occurrence's wall clock, as a zone-less time.
	start time.Time
	// last is the date of the last generated occurrence, or the zero time
	// for a single event.
	last time.Time
	// overrides maps an occurrence's original local start, in the
	// recurrence_id_local format, to whether it is cancelled.
	overrides map[string]bool
}

// Status reports what the feed says about the occurrence uid generates at
// recurrenceIDLocal, the "2006-01-02T15:04:05" wall clock /v1/events sends.
func (f *Feed) Status(uid, recurrenceIDLocal string) Status {
	series, ok := f.series[uid]
	if !ok {
		return StatusDeleted
	}
	if series.cancelled {
		return StatusCancelled
	}
	if cancelled, ok := series.overrides[recurrenceIDLocal]; ok {
		if cancelled {
			return StatusCancelled
		}
		return StatusExists
	}
	if !series.generates(recurrenceIDLocal) {
		return StatusDeleted
	}
	return StatusExists
}

// generates reports whether the series still has an occurrence starting at
// recurrenceIDLocal. Calendar's series repeat weekly on the wall clock, from
// the first start to an inclusive last date, and nothing else.
func (s *feedSeries) generates(recurrenceIDLocal string) bool {
	candidate, err := time.Parse(recurrenceLayout, recurrenceIDLocal)
	if err != nil {
		return false
	}
	if candidate.Hour() != s.start.Hour() || candidate.Minute() != s.start.Minute() || candidate.Second() != s.start.Second() {
		return false
	}
	days := civilDays(candidate) - civilDays(s.start)
	if days == 0 {
		return true
	}
	if s.last.IsZero() || days < 0 || days%7 != 0 {
		return false
	}
	return civilDays(candidate) <= civilDays(s.last)
}

// recurrenceLayout is the recurrence_id_local format.
const recurrenceLayout = "2006-01-02T15:04:05"

func civilDays(t time.Time) int64 {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// Feed fetches and indexes Calendar's All Events feed.
func (c *Client) Feed(ctx context.Context) (*Feed, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/feeds/all.ics", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/calendar")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, feedLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read feed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar feed returned HTTP %d: %s", response.StatusCode, common.Truncate(strings.TrimSpace(string(body)), 200))
	}
	if len(body) > feedLimit {
		// A cut feed would be missing series, and a missing series reads as
		// deleted. Refuse it rather than cancel what was cut off.
		return nil, fmt.Errorf("calendar feed is larger than %d bytes", feedLimit)
	}
	return ParseFeed(body, c.location)
}

// ParseFeed indexes an iCalendar body as Calendar writes it. It fails on a
// body that is not a whole calendar, so an error page or a cut-off response
// can never read as "every series was deleted".
func ParseFeed(body []byte, location *time.Location) (*Feed, error) {
	if location == nil {
		location = time.UTC
	}
	lines, err := unfold(body)
	if err != nil {
		return nil, fmt.Errorf("read calendar feed: %w", err)
	}
	if len(lines) == 0 || lines[0] != "BEGIN:VCALENDAR" || lines[len(lines)-1] != "END:VCALENDAR" {
		return nil, errors.New("calendar feed is not one whole VCALENDAR")
	}
	feed := &Feed{series: map[string]*feedSeries{}, location: location}
	var event map[string]property
	for _, line := range lines {
		switch {
		case line == "BEGIN:VEVENT":
			event = map[string]property{}
		case line == "END:VEVENT":
			if event != nil {
				if err := feed.add(event); err != nil {
					return nil, err
				}
			}
			event = nil
		case event != nil:
			prop := parseProperty(line)
			event[prop.name] = prop
		}
	}
	return feed, nil
}

func (f *Feed) add(event map[string]property) error {
	uid := event["UID"].value
	if uid == "" {
		return fmt.Errorf("calendar feed has an event with no UID")
	}
	series := f.series[uid]
	if series == nil {
		series = &feedSeries{overrides: map[string]bool{}}
		f.series[uid] = series
	}
	cancelled := event["STATUS"].value == "CANCELLED"
	if recurrence, ok := event["RECURRENCE-ID"]; ok {
		local, err := wallClock(recurrence)
		if err != nil {
			return fmt.Errorf("event %s: RECURRENCE-ID: %w", uid, err)
		}
		series.overrides[local.Format(recurrenceLayout)] = cancelled
		return nil
	}
	start, err := wallClock(event["DTSTART"])
	if err != nil {
		return fmt.Errorf("event %s: DTSTART: %w", uid, err)
	}
	series.cancelled = cancelled
	series.start = start
	if rule, ok := event["RRULE"]; ok {
		last, err := f.until(rule.value)
		if err != nil {
			return fmt.Errorf("event %s: RRULE: %w", uid, err)
		}
		series.last = last
	}
	return nil
}

// until reads the last occurrence's date from FREQ=WEEKLY;UNTIL=..., which
// Calendar writes as a DATE for an all-day series and as a UTC instant
// otherwise. The instant is the last start, so its local date is the last
// date.
func (f *Feed) until(rule string) (time.Time, error) {
	for _, part := range strings.Split(rule, ";") {
		value, ok := strings.CutPrefix(part, "UNTIL=")
		if !ok {
			continue
		}
		if instant, err := time.Parse("20060102T150405Z", value); err == nil {
			local := instant.In(f.location)
			return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC), nil
		}
		return time.Parse("20060102", value)
	}
	return time.Time{}, fmt.Errorf("no UNTIL in %q", rule)
}

type property struct {
	name   string
	params string
	value  string
}

// parseProperty splits "NAME;PARAM=x:value". Calendar writes no quoted
// parameter, so the first colon ends the name and its parameters.
func parseProperty(line string) property {
	head, value, _ := strings.Cut(line, ":")
	name, params, _ := strings.Cut(head, ";")
	return property{name: strings.ToUpper(name), params: params, value: value}
}

// wallClock reads a DTSTART or RECURRENCE-ID as the zone-less wall clock
// Calendar stores: ";VALUE=DATE:20260910" or ";TZID=America/Toronto:20260910T180000".
func wallClock(prop property) (time.Time, error) {
	if strings.Contains(prop.params, "VALUE=DATE") && !strings.Contains(prop.params, "VALUE=DATE-TIME") {
		return time.Parse("20060102", prop.value)
	}
	return time.Parse("20060102T150405", strings.TrimSuffix(prop.value, "Z"))
}

// unfold joins RFC 5545 continuation lines, which start with a space or a
// tab, back onto the line they continue.
func unfold(body []byte) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), feedLimit)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}
