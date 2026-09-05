// Package fake is a scripted model for tests.
//
// The agent loop's behaviour — how many turns it takes, when it stops, what it
// does with a tool error — is decided by what the model returns. A script
// makes that an input to the test rather than a vendor's mood.
package fake

import (
	"context"
	"errors"
	"sync"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
)

// Turn is one scripted answer. Err, when set, is returned instead of Response,
// which is how a test drives the rate-limited path.
type Turn struct {
	Response model.Response
	Err      error
}

// Call records one Generate the loop made, so a test can assert on the history
// the loop built as well as on the answer it produced.
type Call struct {
	System  string
	History []model.Message
	Tools   []model.ToolSpec
}

// Model returns its script in order. Running past the end is a test bug, so it
// returns an error rather than looping or blocking.
type Model struct {
	mu    sync.Mutex
	turns []Turn
	calls []Call
	name  string
}

func New(turns ...Turn) *Model {
	return &Model{turns: turns, name: "fake/scripted"}
}

// Text is the common script: one answer, no tool calls.
func Text(answer string) *Model {
	return New(Turn{Response: model.Response{Text: answer, InputTokens: 10, OutputTokens: 5}})
}

func (m *Model) Name() string { return m.name }

func (m *Model) Generate(_ context.Context, system string, history []model.Message, tools []model.ToolSpec) (model.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// The history is the loop's own slice and it keeps appending to it, so the
	// record has to be a copy or every recorded call ends up identical.
	m.calls = append(m.calls, Call{
		System:  system,
		History: append([]model.Message(nil), history...),
		Tools:   tools,
	})
	if len(m.turns) == 0 {
		return model.Response{}, errors.New("fake model: the script ran out")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	return turn.Response, turn.Err
}

// Calls returns what the loop asked for, in order.
func (m *Model) Calls() []Call {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Call(nil), m.calls...)
}
