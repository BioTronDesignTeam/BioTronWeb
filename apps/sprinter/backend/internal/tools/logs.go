package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/readstore"
)

const (
	recentLogsCap = 6000
	logHistoryCap = 8000
	logPayloadCap = 500
)

// levels are the four values the log_level enum holds. Passing anything else
// to the query would be a cast error, so it is refused here with a sentence
// the model can act on.
var levels = []string{"debug", "info", "warning", "error"}

// logArgs is what both log tools accept. The two differ only in the window and
// the paging, so the filters are declared once.
type logArgs struct {
	Service string `json:"service"`
	Level   string `json:"level"`
	Search  string `json:"search"`
	Limit   *int   `json:"limit"`
	From    string `json:"from"`
	To      string `json:"to"`
	Cursor  string `json:"cursor"`
}

func (a logArgs) query(limit int) (readstore.LogQuery, error) {
	query := readstore.LogQuery{
		Service: strings.TrimSpace(a.Service),
		Level:   strings.TrimSpace(a.Level),
		Search:  strings.TrimSpace(a.Search),
		Cursor:  strings.TrimSpace(a.Cursor),
		Limit:   limit,
	}
	if err := oneOf(query.Level, levels...); err != nil {
		return readstore.LogQuery{}, fmt.Errorf("level %w", err)
	}
	from, err := parseTime("from", a.From)
	if err != nil {
		return readstore.LogQuery{}, err
	}
	to, err := parseTime("to", a.To)
	if err != nil {
		return readstore.LogQuery{}, err
	}
	if from != nil && to != nil && to.Before(*from) {
		return readstore.LogQuery{}, errors.New("to must not be before from")
	}
	query.From, query.To = from, to
	return query, nil
}

func parseTime(name, raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC3339 timestamp such as 2026-09-05T14:00:00Z, got %q", name, raw)
	}
	return &parsed, nil
}

const logFilterSchema = `
	"service": {"type": "string", "description": "One Logger service id, such as oauth-manager or exo-api. Omit for every service."},
	"level": {"type": "string", "enum": ["debug", "info", "warning", "error"], "description": "Keep only this level. Omit for every level."},
	"search": {"type": "string", "description": "Keep only rows whose message contains this text, case-insensitively. It is a substring, not a pattern."}`

// --- recent_logs ------------------------------------------------------------

// RecentLogs is the "what just happened" tool: the newest rows, no paging.
type RecentLogs struct {
	Reader Reader
}

func (t RecentLogs) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name: "recent_logs",
		Description: "The newest structured log rows, newest first. Use it to " +
			"find out what a service has been doing or why something failed. " +
			"Each row is a timestamp, a level, a service, a message and a " +
			"payload; the payload is cut at " + fmt.Sprint(logPayloadCap) +
			" characters. Returns at most 50 rows and " + fmt.Sprint(recentLogsCap) +
			" characters. For a window in the past, or for more rows than one " +
			"call returns, use log_history.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {` + logFilterSchema + `,
				"limit": {"type": "integer", "description": "How many rows, 1 to 50. Defaults to 20."}
			}
		}`),
	}
}

func (t RecentLogs) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed logArgs
	if err := decode(args, &parsed); err != nil {
		return "", err
	}
	limit, err := clampInt(parsed.Limit, 20, 1, 50)
	if err != nil {
		return "", fmt.Errorf("limit %w", err)
	}
	// The window and the cursor belong to log_history. Silently ignoring them
	// would let the model believe it filtered when it did not.
	if parsed.From != "" || parsed.To != "" || parsed.Cursor != "" {
		return "", errors.New("recent_logs takes no from, to or cursor; use log_history for a time window or a second page")
	}
	filter, err := parsed.query(limit)
	if err != nil {
		return "", err
	}

	queryCtx, cancel := dbContext(ctx)
	defer cancel()
	rows, err := t.Reader.RecentLogs(queryCtx, filter)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "No log rows match that filter.", nil
	}
	return capResult(formatLogs(rows), recentLogsCap,
		"Lower limit, or narrow by service, level or search."), nil
}

// --- log_history ------------------------------------------------------------

// LogHistory is the same filters over a window, with a cursor. It exists so a
// question about last Tuesday does not have to be answered from the newest
// fifty rows.
type LogHistory struct {
	Reader Reader
}

func (t LogHistory) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name: "log_history",
		Description: "Structured log rows in a time window, newest first, one " +
			"page at a time. Same filters as recent_logs plus from, to and " +
			"cursor. When the answer ends with a next_cursor line there are " +
			"more rows: call again with that cursor and the same filters. " +
			"Returns at most 50 rows and " + fmt.Sprint(logHistoryCap) + " characters.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {` + logFilterSchema + `,
				"limit": {"type": "integer", "description": "How many rows per page, 1 to 50. Defaults to 20."},
				"from": {"type": "string", "description": "Oldest timestamp to include, RFC3339, such as 2026-09-01T00:00:00Z."},
				"to": {"type": "string", "description": "Newest timestamp to include, RFC3339."},
				"cursor": {"type": "string", "description": "The next_cursor from the previous page. Send the same filters with it."}
			}
		}`),
	}
}

func (t LogHistory) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed logArgs
	if err := decode(args, &parsed); err != nil {
		return "", err
	}
	limit, err := clampInt(parsed.Limit, 20, 1, 50)
	if err != nil {
		return "", fmt.Errorf("limit %w", err)
	}
	filter, err := parsed.query(limit)
	if err != nil {
		return "", err
	}

	queryCtx, cancel := dbContext(ctx)
	defer cancel()
	rows, next, err := t.Reader.LogHistory(queryCtx, filter)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "No log rows match that filter in that window.", nil
	}
	text := formatLogs(rows)
	if next != "" {
		text += "\nnext_cursor: " + next
	} else {
		text += "\nnext_cursor: none. This is the last page."
	}
	return capResult(text, logHistoryCap,
		"Lower limit, narrow the window, or filter by service or level."), nil
}

// formatLogs writes one row per line. Plain lines rather than JSON: the model
// answers in prose, and a line costs fewer tokens than an object.
func formatLogs(rows []readstore.LogRow) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%d rows, newest first.\n", len(rows))
	for _, row := range rows {
		fmt.Fprintf(&out, "%s %s %s | %s", stamp(row.CreatedAt), row.Level, row.Service, row.Message)
		if payload := strings.TrimSpace(string(row.Payload)); payload != "" && payload != "null" {
			fmt.Fprintf(&out, " | %s", truncate(payload, logPayloadCap))
		}
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
}
