// Package tools is what the agent may look at.
//
// Two tools, each one read-only: Calendar's public upcoming-events endpoint,
// and one SQL query through the read-only pool. A tool validates its own
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

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/common"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/readstore"
)

// Tool is one thing the agent can ask for. Run returns text for the model:
// an error means the call failed, and the loop reports it as a tool error
// rather than abandoning the question.
type Tool interface {
	Spec() model.ToolSpec
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

// Privileged is implemented by a tool whose output Auth shows only to
// managers and superusers. The agent offers such a tool only to a question
// from someone the who-has-access guard admits, and the bot sends that
// answer to the person alone, never to the channel.
type Privileged interface {
	Privileged() bool
}

// IsPrivileged reports whether tool is one only the who-has-access guard
// admits.
func IsPrivileged(tool Tool) bool {
	privileged, ok := tool.(Privileged)
	return ok && privileged.Privileged()
}

// Reader is the part of readstore the SQL tool uses. It is an interface so a
// unit test can drive argument validation without a database.
type Reader interface {
	Access(ctx context.Context, appID string) (readstore.AccessReport, error)
}

// All builds every tool. A nil reader leaves out the SQL tool, which is what
// happens when READ_DATABASE_URL is unset, and an empty Calendar URL leaves out
// the Calendar tool: the bot still starts rather than refusing to.
func All(reader Reader, calendarURL string) []Tool {
	var all []Tool
	if calendarURL != "" {
		all = append(all, CalendarUpcoming{CalendarURL: calendarURL})
	}
	if reader != nil {
		all = append(all, WhoHasAccess{Reader: reader})
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
		cut = common.RuneStart(text, limit)
	}
	return strings.TrimRight(text[:cut], "\n") +
		fmt.Sprintf("\n[Cut here. This result was longer than %d characters. %s]", limit, narrow)
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

// clampInt applies a default and a range from 1 to high. It reports the value
// it used, so a tool never has to decide whether zero meant "none" or "unset".
func clampInt(value *int, fallback, high int) (int, error) {
	if value == nil {
		return fallback, nil
	}
	if *value < 1 || *value > high {
		return 0, fmt.Errorf("must be between 1 and %d, got %d", high, *value)
	}
	return *value, nil
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
