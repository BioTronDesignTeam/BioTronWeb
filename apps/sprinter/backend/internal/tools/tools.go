// Package tools is what the agent may look at.
//
// Six tools, each one read-only: two HTTP endpoints that are already public,
// and four SQL queries through the read-only pool. A tool validates its own
// arguments and caps its own output, because both come from a model. The
// argument may be nonsense and the result may be a thousand rows, and neither
// is allowed to reach the loop as such.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/readstore"
)

// Tool is one thing the agent can ask for. Run returns text for the model:
// an error means the call failed, and the loop reports it as a tool error
// rather than abandoning the question.
type Tool interface {
	Spec() model.ToolSpec
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

// Reader is the part of readstore the SQL tools use. It is an interface so a
// unit test can drive argument validation without a database.
type Reader interface {
	RecentLogs(ctx context.Context, query readstore.LogQuery) ([]readstore.LogRow, error)
	LogHistory(ctx context.Context, query readstore.LogQuery) ([]readstore.LogRow, string, error)
	HealthHistory(ctx context.Context, service string, since time.Time, limit int) ([]readstore.HealthRow, error)
	Access(ctx context.Context, appID string) (readstore.AccessReport, error)
}

// All builds every tool. A nil reader leaves out the four SQL tools, which is
// what happens when READ_DATABASE_URL is unset: the bot still answers from the
// two public endpoints rather than refusing to start.
func All(reader Reader, loggerURL, calendarURL string) []Tool {
	all := []Tool{PlatformStatus{LoggerURL: loggerURL}}
	if calendarURL != "" {
		all = append(all, CalendarUpcoming{CalendarURL: calendarURL})
	}
	if reader != nil {
		all = append(all,
			RecentLogs{Reader: reader},
			LogHistory{Reader: reader},
			HealthHistory{Reader: reader},
			WhoHasAccess{Reader: reader},
		)
	}
	return all
}

// capResult is the one place a tool result is shortened. Every tool goes
// through it, so no tool can quietly return a megabyte, and a cut result says
// it was cut: a model that reads a truncated list as the whole list gives a
// confidently wrong answer.
func capResult(text string, limit int, narrow string) string {
	if len(text) <= limit {
		return text
	}
	cut := strings.LastIndex(text[:limit], "\n")
	if cut <= 0 {
		cut = runeSafeCut(text, limit)
	}
	return strings.TrimRight(text[:cut], "\n") +
		fmt.Sprintf("\n[Cut here. This result was longer than %d characters. %s]", limit, narrow)
}

// truncate shortens one field rather than the whole result. A log payload is
// the usual case: useful at a glance, unbounded in principle.
func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:runeSafeCut(text, limit)] + "…"
}

// runeSafeCut moves a byte index back to the start of a character. Every
// limit here counts bytes, and a log line holding an accent or an emoji puts
// a character across the limit; cutting mid-character leaves the model a
// replacement glyph, and Discord a message that is not valid UTF-8.
func runeSafeCut(text string, cut int) int {
	for cut > 0 && cut < len(text) && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return cut
}

// decode reads a tool's arguments. Unknown fields are ignored on purpose: a
// model that adds a stray key has still asked a clear question, and spending a
// turn on the spelling of an argument nobody needed helps nobody.
func decode(args json.RawMessage, into any) error {
	if len(args) == 0 {
		return nil
	}
	if err := json.Unmarshal(args, into); err != nil {
		return fmt.Errorf("arguments are not valid JSON for this tool: %w", err)
	}
	return nil
}

// clampInt applies a default and a range. It reports the value it used, so a
// tool never has to decide whether zero meant "none" or "unset".
func clampInt(value *int, fallback, low, high int) (int, error) {
	if value == nil {
		return fallback, nil
	}
	if *value < low || *value > high {
		return 0, fmt.Errorf("must be between %d and %d, got %d", low, high, *value)
	}
	return *value, nil
}

// oneOf checks a closed set. The schema already lists the values, but a model
// is free to ignore a schema, so the check has to exist here too.
func oneOf(value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("must be one of %s, got %q", strings.Join(allowed, ", "), value)
}

// dbContext bounds one database call. The agent's own deadline covers a whole
// question, which is minutes; one scan that takes minutes is a stuck query, and
// the tool should say so rather than spend the question's whole budget on it.
func dbContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, readstore.Timeout)
}

// stamp is how every time is written back to the model: one format, UTC, to
// the second. A mixture of formats invites the model to invent a third.
func stamp(at time.Time) string {
	return at.UTC().Format(time.RFC3339)
}
