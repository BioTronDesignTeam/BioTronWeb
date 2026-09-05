package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
)

const calendarCap = 4000

// CalendarUpcoming reads Calendar's public "what is on next" endpoint. Which
// occurrences count as upcoming is Calendar's rule, decided there once; this
// tool asks the question rather than reimplementing the answer.
type CalendarUpcoming struct {
	CalendarURL string
}

type calendarArgs struct {
	Limit *int `json:"limit"`
	Days  *int `json:"days"`
}

// occurrence is the part of Calendar's event object this tool reports. The
// endpoint returns more; a description would fill the cap on its own.
type occurrence struct {
	Title     string    `json:"title"`
	ScopePath string    `json:"scope_path"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Location  string    `json:"location"`
	URL       string    `json:"url"`
	AllDay    bool      `json:"all_day"`
}

func (t CalendarUpcoming) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name: "calendar_upcoming",
		Description: "Events that are still to come or still running, soonest " +
			"first, from the BioTron team calendar. Each event has a title, the " +
			"calendar it belongs to, a start, an end, a location and a link. " +
			"Returns at most 20 events and " + fmt.Sprint(calendarCap) + " characters.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"limit": {"type": "integer", "description": "How many events, 1 to 20. Defaults to 5."},
				"days": {"type": "integer", "description": "How far ahead to look, 1 to 90 days. Defaults to 42."}
			}
		}`),
	}
}

func (t CalendarUpcoming) Run(ctx context.Context, args json.RawMessage) (string, error) {
	if t.CalendarURL == "" {
		return "", errors.New("no Calendar URL is configured")
	}
	var parsed calendarArgs
	if err := decode(args, &parsed); err != nil {
		return "", err
	}
	limit, err := clampInt(parsed.Limit, 5, 1, 20)
	if err != nil {
		return "", fmt.Errorf("limit %w", err)
	}
	days, err := clampInt(parsed.Days, 42, 1, 90)
	if err != nil {
		return "", fmt.Errorf("days %w", err)
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	query.Set("days", strconv.Itoa(days))
	body, err := fetchJSON(ctx, t.CalendarURL+"/v1/events/upcoming?"+query.Encode())
	if err != nil {
		return "", err
	}
	var events []occurrence
	if err := json.Unmarshal(body, &events); err != nil {
		return "", fmt.Errorf("calendar answered something this tool cannot read: %w", err)
	}
	if len(events) == 0 {
		return fmt.Sprintf("No events in the next %d days.", days), nil
	}

	var out strings.Builder
	fmt.Fprintf(&out, "%d events in the next %d days, soonest first.\n", len(events), days)
	for _, event := range events {
		fmt.Fprintf(&out, "- %s | calendar: %s | starts: %s | ends: %s",
			event.Title, fallback(event.ScopePath, "unknown"),
			stamp(event.StartsAt), stamp(event.EndsAt))
		if event.AllDay {
			out.WriteString(" | all day")
		}
		if event.Location != "" {
			fmt.Fprintf(&out, " | location: %s", event.Location)
		}
		if event.URL != "" {
			fmt.Fprintf(&out, " | url: %s", event.URL)
		}
		out.WriteString("\n")
	}
	return capResult(strings.TrimRight(out.String(), "\n"), calendarCap,
		"Ask for fewer events or a shorter window."), nil
}

func fallback(value, whenEmpty string) string {
	if strings.TrimSpace(value) == "" {
		return whenEmpty
	}
	return value
}
