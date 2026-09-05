// Package calendar reads occurrences from Calendar's public API. Sprinter
// never writes to Calendar and never sees a draft or a cancelled occurrence's
// note — /v1/events drops a cancelled occurrence outright, which is why the
// scheduler package treats "an occurrence Sprinter used to see is now absent"
// as the cancellation signal.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// localDateLayout matches Calendar's own from/to query format: a wall-clock
// date, not a timestamp. /v1/events resolves that date in its own configured
// timezone (America/Toronto), so the client resolves it the same way rather
// than sending an instant Calendar would have to reinterpret.
const localDateLayout = "2006-01-02"

// Occurrence mirrors the fields of Calendar's public JSON that the scheduler
// needs. Calendar's full shape carries more (series_id, scope_name, ...); this
// is deliberately the subset Sprinter reads.
type Occurrence struct {
	UID              string    `json:"uid"`
	RecurrenceID     string    `json:"recurrence_id_local"`
	SeriesSequence   int       `json:"series_sequence"`
	OverrideSequence int       `json:"override_sequence,omitempty"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Location         string    `json:"location"`
	URL              string    `json:"url"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	AllDay           bool      `json:"all_day"`
	Timezone         string    `json:"timezone"`
	ScopeID          string    `json:"scope_id"`
	ScopePath        string    `json:"scope_path"`
	Modified         bool      `json:"modified"`
}

// Sequence is the occurrence's edit counter: the override's when it has one,
// otherwise the series'. Calendar bumps it on every edit, which is what lets
// the scheduler tell an already-posted occurrence from a changed one.
func (o Occurrence) Sequence() int {
	if o.OverrideSequence != 0 {
		return o.OverrideSequence
	}
	return o.SeriesSequence
}

// timeout bounds one call to Calendar. A stalled Calendar must never hold a
// scheduler tick open.
const timeout = 5 * time.Second

type Client struct {
	baseURL  string
	location *time.Location
	http     *http.Client
}

// NewClient builds a client against Calendar's base URL (CALENDAR_URL).
// location is the timezone Calendar itself runs in; the from/to instants
// passed to Occurrences are resolved to wall-clock dates in it before they go
// on the wire, because that is the only format /v1/events accepts.
func NewClient(baseURL string, location *time.Location) *Client {
	if location == nil {
		location = time.UTC
	}
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		location: location,
		http:     &http.Client{Timeout: timeout},
	}
}

// Occurrences fetches every occurrence in scopeID (every scope when scopeID
// is empty) whose window overlaps [from, to]. A non-200 response, a transport
// failure, or a body Calendar's own shape cannot decode is always an error:
// the scheduler must never mistake "Calendar could not be reached" for "there
// is nothing on the calendar right now".
func (c *Client) Occurrences(ctx context.Context, scopeID string, from, to time.Time) ([]Occurrence, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fromDate := from.In(c.location)
	toDate := to.In(c.location)
	// /v1/events requires strict to > from; two instants inside the same
	// local day would otherwise resolve to the same date and be refused.
	if !dateAfter(toDate, fromDate) {
		toDate = toDate.AddDate(0, 0, 1)
	}

	query := url.Values{
		"from": {fromDate.Format(localDateLayout)},
		"to":   {toDate.Format(localDateLayout)},
	}
	if scopeID != "" {
		query.Set("scope_id", scopeID)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/v1/events?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch occurrences: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read occurrences response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Calendar returned HTTP %d: %s", response.StatusCode, truncate(body, 200))
	}
	var occurrences []Occurrence
	if err := json.Unmarshal(body, &occurrences); err != nil {
		return nil, fmt.Errorf("decode occurrences: %w", err)
	}
	return occurrences, nil
}

func dateAfter(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	if ay != by {
		return ay > by
	}
	if am != bm {
		return am > bm
	}
	return ad > bd
}

func truncate(body []byte, n int) string {
	text := strings.TrimSpace(string(body))
	if len(text) > n {
		return text[:n] + "..."
	}
	return text
}
