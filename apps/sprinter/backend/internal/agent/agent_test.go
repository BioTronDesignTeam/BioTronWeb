package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model/fake"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/tools"
)

// countingTool answers with a fixed string, or fails, and counts how often it
// ran. What the loop does with a tool matters more here than what the tool is.
type countingTool struct {
	name string
	out  string
	err  error
	runs int
}

func (t *countingTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:        t.name,
		Description: "a tool for tests",
		Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

func (t *countingTool) Run(context.Context, json.RawMessage) (string, error) {
	t.runs++
	return t.out, t.err
}

func callTurn(name string, calls ...string) fake.Turn {
	turn := fake.Turn{Response: model.Response{InputTokens: 100, OutputTokens: 20}}
	for i, args := range calls {
		turn.Response.ToolCalls = append(turn.Response.ToolCalls, model.ToolCall{
			ID: fmt.Sprintf("call-%d", i), Name: name, Args: json.RawMessage(args),
		})
	}
	return turn
}

func textTurn(text string) fake.Turn {
	return fake.Turn{Response: model.Response{Text: text, InputTokens: 50, OutputTokens: 10}}
}

func TestAnswerWithNoTools(t *testing.T) {
	answering := fake.Text("Nothing is down right now.")
	loop := New(answering, nil, Options{})

	result, err := loop.Answer(t.Context(), "  is anything down?  ")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if result.Text != "Nothing is down right now." {
		t.Fatalf("text = %q", result.Text)
	}
	if result.Turns != 1 || len(result.Tools) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if result.InputTokens != 10 || result.OutputTokens != 5 {
		t.Fatalf("tokens = %d in, %d out", result.InputTokens, result.OutputTokens)
	}
	// The caller stores exactly these two turns: the question as asked, and
	// the answer.
	if len(result.Messages) != 2 {
		t.Fatalf("messages = %+v", result.Messages)
	}
	if result.Messages[0].Role != model.RoleUser || result.Messages[0].Text != "is anything down?" {
		t.Fatalf("first message = %+v", result.Messages[0])
	}
	if result.Messages[1].Role != model.RoleAssistant {
		t.Fatalf("second message = %+v", result.Messages[1])
	}

	calls := answering.Calls()
	if len(calls) != 1 || calls[0].System != SystemPrompt {
		t.Fatalf("the loop must send its own system prompt: %+v", calls)
	}
}

func TestAnswerRunsAToolAndFeedsTheResultBack(t *testing.T) {
	tool := &countingTool{name: "platform_status", out: "everything operational"}
	answering := fake.New(
		callTurn("platform_status", `{}`),
		textTurn("Everything is operational."),
	)
	loop := New(answering, []tools.Tool{tool}, Options{})

	result, err := loop.Answer(t.Context(), "how is the platform?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if result.Text != "Everything is operational." {
		t.Fatalf("text = %q", result.Text)
	}
	if tool.runs != 1 {
		t.Fatalf("the tool ran %d times, want 1", tool.runs)
	}
	if result.Turns != 2 {
		t.Fatalf("turns = %d, want 2", result.Turns)
	}
	if len(result.Tools) != 1 || result.Tools[0] != "platform_status" {
		t.Fatalf("tools = %v", result.Tools)
	}
	// Tokens are summed across turns, because the question cost both.
	if result.InputTokens != 150 || result.OutputTokens != 30 {
		t.Fatalf("tokens = %d in, %d out", result.InputTokens, result.OutputTokens)
	}

	// Four turns are stored: the question, the call, the result, the answer.
	if len(result.Messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(result.Messages))
	}
	if len(result.Messages[1].ToolCalls) != 1 {
		t.Fatalf("the call turn is missing its call: %+v", result.Messages[1])
	}
	answered := result.Messages[2]
	if answered.Role != model.RoleUser || len(answered.ToolResults) != 1 {
		t.Fatalf("the result turn is wrong: %+v", answered)
	}
	if answered.ToolResults[0].Content != "everything operational" || answered.ToolResults[0].IsError {
		t.Fatalf("result = %+v", answered.ToolResults[0])
	}

	// The second call to the model must see the whole conversation so far.
	second := answering.Calls()[1]
	if len(second.History) != 3 {
		t.Fatalf("second call saw %d turns, want 3", len(second.History))
	}
}

func TestAToolFailureIsAResultNotAnError(t *testing.T) {
	tool := &countingTool{name: "recent_logs", err: errors.New("limit must be between 1 and 50")}
	answering := fake.New(
		callTurn("recent_logs", `{"limit": 500}`),
		textTurn("I could not read the logs with those arguments."),
	)
	loop := New(answering, []tools.Tool{tool}, Options{})

	result, err := loop.Answer(t.Context(), "show me 500 log rows")
	if err != nil {
		t.Fatalf("a failing tool must not fail the question: %v", err)
	}
	if result.Text == "" {
		t.Fatal("the loop must still answer")
	}
	failed := result.Messages[2].ToolResults[0]
	if !failed.IsError || !strings.Contains(failed.Content, "limit must be between 1 and 50") {
		t.Fatalf("the model must be told what went wrong: %+v", failed)
	}
}

func TestAnUnknownToolIsReportedToTheModel(t *testing.T) {
	answering := fake.New(
		callTurn("read_secrets", `{}`),
		textTurn("There is no such tool."),
	)
	loop := New(answering, nil, Options{})

	result, err := loop.Answer(t.Context(), "read the secrets")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	refused := result.Messages[2].ToolResults[0]
	if !refused.IsError || !strings.Contains(refused.Content, "no tool called read_secrets") {
		t.Fatalf("result = %+v", refused)
	}
}

func TestTheTurnCapStopsTheLoopAndSaysSo(t *testing.T) {
	tool := &countingTool{name: "platform_status", out: "still operational"}
	// A model that only ever calls tools would run forever without the cap.
	answering := fake.New(
		callTurn("platform_status", `{}`),
		callTurn("platform_status", `{}`),
		fake.Turn{Response: model.Response{
			Text:      "I have looked twice so far.",
			ToolCalls: []model.ToolCall{{ID: "call-x", Name: "platform_status", Args: json.RawMessage(`{}`)}},
		}},
	)
	loop := New(answering, []tools.Tool{tool}, Options{MaxTurns: 3})

	result, err := loop.Answer(t.Context(), "keep checking")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if result.Turns != 3 || tool.runs != 3 {
		t.Fatalf("turns = %d, tool runs = %d", result.Turns, tool.runs)
	}
	if !strings.Contains(result.Text, "stopped after 3 turns") {
		t.Fatalf("an unfinished answer must say so: %q", result.Text)
	}
	// Whatever the model did manage to say is kept, because a partial finding
	// beats nothing at all.
	if !strings.Contains(result.Text, "I have looked twice so far.") {
		t.Fatalf("the last thing the model said must survive: %q", result.Text)
	}
}

func TestTheToolCapTakesTheToolsAwayAndSaysSo(t *testing.T) {
	tool := &countingTool{name: "platform_status", out: "operational"}
	answering := fake.New(
		callTurn("platform_status", `{}`, `{}`),
		callTurn("platform_status", `{}`),
		textTurn("Here is what I found."),
	)
	loop := New(answering, []tools.Tool{tool}, Options{MaxToolCalls: 2})

	result, err := loop.Answer(t.Context(), "check everything")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if tool.runs != 2 {
		t.Fatalf("the tool ran %d times, want the cap of 2", tool.runs)
	}
	if !strings.Contains(result.Text, "stopped after 2 tool calls") {
		t.Fatalf("a capped answer must say so: %q", result.Text)
	}
	// Once the budget is spent the model is offered no tools, which is what
	// makes the loop end rather than spin against a refusal.
	last := answering.Calls()[2]
	if last.Tools != nil {
		t.Fatalf("the last turn was still offered tools: %+v", last.Tools)
	}
}

func TestARateLimitIsAnAnswerNotAFailure(t *testing.T) {
	answering := fake.New(fake.Turn{Err: fmt.Errorf("gemini: %w: quota", model.ErrRateLimited)})
	loop := New(answering, nil, Options{})

	result, err := loop.Answer(t.Context(), "how is the platform?")
	if err != nil {
		t.Fatalf("a rate limit must not be an error: %v", err)
	}
	if !result.RateLimited || !strings.Contains(result.Text, "rate limited") {
		t.Fatalf("result = %+v", result)
	}
	// Nothing was answered, so there is nothing to store in a transcript.
	if len(result.Messages) != 0 {
		t.Fatalf("messages = %+v", result.Messages)
	}
}

func TestAnyOtherModelFailureIsAnError(t *testing.T) {
	answering := fake.New(fake.Turn{Err: errors.New("dial failed")})
	loop := New(answering, nil, Options{})
	if _, err := loop.Answer(t.Context(), "hello"); err == nil {
		t.Fatal("a broken model must be an error the bot can report")
	}
}

func TestContinueSeesTheHistoryButDoesNotReturnIt(t *testing.T) {
	history := []model.Message{
		{Role: model.RoleUser, Text: "how is the platform?"},
		{Role: model.RoleAssistant, Text: "Everything is operational."},
	}
	answering := fake.Text("Still operational.")
	loop := New(answering, nil, Options{})

	result, err := loop.Continue(t.Context(), history, "and now?")
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	// Only the new turns come back, so the caller appends rather than
	// rewriting the transcript it already stored.
	if len(result.Messages) != 2 || result.Messages[0].Text != "and now?" {
		t.Fatalf("messages = %+v", result.Messages)
	}
	seen := answering.Calls()[0].History
	if len(seen) != 3 || seen[0].Text != "how is the platform?" {
		t.Fatalf("the model must see the whole thread: %+v", seen)
	}
}

func TestAnEmptyQuestionIsRefused(t *testing.T) {
	loop := New(fake.Text("unused"), nil, Options{})
	if _, err := loop.Answer(t.Context(), "   "); err == nil {
		t.Fatal("an empty question must be refused before it costs a model call")
	}
}

func TestAnEmptyAnswerStillSaysSomething(t *testing.T) {
	loop := New(fake.New(textTurn("")), nil, Options{})
	result, err := loop.Answer(t.Context(), "hello")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if result.Text == "" {
		t.Fatal("the bot must never post an empty message")
	}
}

func TestDefaultsAreApplied(t *testing.T) {
	loop := New(fake.Text("hi"), nil, Options{})
	if loop.options.MaxTurns != DefaultMaxTurns ||
		loop.options.MaxToolCalls != DefaultMaxToolCalls ||
		loop.options.Timeout != DefaultTimeout ||
		loop.options.System != SystemPrompt {
		t.Fatalf("options = %+v", loop.options)
	}
	if loop.Model() != "fake/scripted" {
		t.Fatalf("model = %q", loop.Model())
	}
}
