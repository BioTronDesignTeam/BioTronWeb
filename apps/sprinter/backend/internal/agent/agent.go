// Package agent is the loop between a question and an answer.
//
// It asks the model, runs whatever tools the model asked for, feeds the
// results back, and stops when the model answers in words. Three things bound
// it, because a model can loop forever and a Discord interaction cannot: a
// turn cap, a tool-call cap, and a deadline. When a cap stops the loop the
// answer says so, rather than presenting a half-finished search as the whole
// story.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/tools"
)

// The defaults. Eight turns is enough for "look, look again, answer" twice
// over; twelve tool calls is more than any question here has needed. Two
// minutes is well inside Discord's fifteen-minute interaction window and short
// enough that a stuck question frees its concurrency slot.
const (
	DefaultMaxTurns     = 8
	DefaultMaxToolCalls = 12
	DefaultTimeout      = 2 * time.Minute
)

// The sentences the loop adds for itself. They are answers, not errors: the
// person asked a fair question and deserves to know the search stopped early.
const (
	turnCapNotice     = "I stopped after %d turns without finishing. Ask a narrower question, or ask about one service at a time."
	toolCapNotice     = "I stopped after %d tool calls, so this answer may be incomplete."
	rateLimitedNotice = "The model is rate limited. Try again in a minute."
	noAnswerNotice    = "The model returned nothing. Try asking again."
)

type Options struct {
	MaxTurns     int
	MaxToolCalls int
	Timeout      time.Duration
	// System overrides SystemPrompt. It exists for tests; production uses the
	// package's own prompt.
	System string
}

// Result is one answered question. Messages is the transcript the loop built —
// the question, every assistant turn with its tool calls, and every tool
// result — so a thread can be replayed later from the database alone.
type Result struct {
	Text     string
	Messages []model.Message
	// Tools names each tool that ran, in the order first called. It goes into
	// the event payload, so an operator can see what a question cost to answer.
	Tools        []string
	InputTokens  int
	OutputTokens int
	Turns        int
	// RateLimited says the model refused for quota. The caller answers with
	// Text and stores nothing: no question was actually answered.
	RateLimited bool
}

type Agent struct {
	model   model.Model
	tools   map[string]tools.Tool
	specs   []model.ToolSpec
	options Options
}

func New(m model.Model, list []tools.Tool, options Options) *Agent {
	if options.MaxTurns <= 0 {
		options.MaxTurns = DefaultMaxTurns
	}
	if options.MaxToolCalls <= 0 {
		options.MaxToolCalls = DefaultMaxToolCalls
	}
	if options.Timeout <= 0 {
		options.Timeout = DefaultTimeout
	}
	if options.System == "" {
		options.System = SystemPrompt
	}
	byName := make(map[string]tools.Tool, len(list))
	specs := make([]model.ToolSpec, 0, len(list))
	for _, tool := range list {
		spec := tool.Spec()
		byName[spec.Name] = tool
		specs = append(specs, spec)
	}
	return &Agent{model: m, tools: byName, specs: specs, options: options}
}

// Model is the model's name, for the agent_threads row and the logs.
func (a *Agent) Model() string { return a.model.Name() }

// Answer is one question with no history: the `/agent` command, and the first
// turn of a thread.
func (a *Agent) Answer(ctx context.Context, question string) (Result, error) {
	return a.Continue(ctx, nil, question)
}

// Continue is a question with the thread's transcript in front of it. History
// is not copied into the result; only the new turns are, so the caller appends
// rather than rewrites.
func (a *Agent) Continue(ctx context.Context, history []model.Message, question string) (Result, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return Result{}, errors.New("the question is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, a.options.Timeout)
	defer cancel()

	asked := model.Message{Role: model.RoleUser, Text: question}
	// working is what the model sees; result.Messages is what the caller
	// stores. They share the new turns and differ by the history.
	working := make([]model.Message, 0, len(history)+4)
	working = append(working, history...)
	working = append(working, asked)

	result := Result{Messages: []model.Message{asked}}
	// One nonce for this whole answer, so the model sees the same tag on
	// every result and can tell where each one ends.
	nonce := newNonce()
	spent := 0
	toolCapHit := false

	for turn := 1; turn <= a.options.MaxTurns; turn++ {
		result.Turns = turn
		// Once the tool budget is spent the model is offered no tools at all,
		// which is what guarantees the loop ends: it has nothing left to ask
		// for except an answer.
		offered := a.specs
		if toolCapHit {
			offered = nil
		}

		response, err := a.model.Generate(ctx, a.options.System, working, offered)
		if err != nil {
			if errors.Is(err, model.ErrRateLimited) {
				return Result{Text: rateLimitedNotice, RateLimited: true}, nil
			}
			return Result{}, err
		}
		result.InputTokens += response.InputTokens
		result.OutputTokens += response.OutputTokens

		assistant := model.Message{
			Role:      model.RoleAssistant,
			Text:      response.Text,
			ToolCalls: response.ToolCalls,
		}
		working = append(working, assistant)
		result.Messages = append(result.Messages, assistant)

		if len(response.ToolCalls) == 0 {
			result.Text = strings.TrimSpace(response.Text)
			if result.Text == "" {
				result.Text = noAnswerNotice
			}
			if toolCapHit {
				result.Text = join(result.Text, fmt.Sprintf(toolCapNotice, a.options.MaxToolCalls))
			}
			return result, nil
		}

		results := make([]model.ToolResult, 0, len(response.ToolCalls))
		for _, call := range response.ToolCalls {
			if spent >= a.options.MaxToolCalls {
				toolCapHit = true
				results = append(results, model.ToolResult{
					ID: call.ID, Name: call.Name, IsError: true,
					Content: "The tool budget for this question is spent. Answer from what you already have.",
				})
				continue
			}
			spent++
			result.Tools = appendOnce(result.Tools, call.Name)
			results = append(results, a.runTool(ctx, call))
		}
		for i := range results {
			results[i].Content = fence(nonce, results[i].Content)
		}
		// Every result goes back as one user turn, because that is how both
		// vendors model it: results are user-side content answering the calls.
		answered := model.Message{Role: model.RoleUser, ToolResults: results}
		working = append(working, answered)
		result.Messages = append(result.Messages, answered)
	}

	// The loop ran out of turns. Whatever the model last said is kept, because
	// a partial finding beats nothing, but it is labelled as unfinished.
	result.Text = join(lastText(result.Messages), fmt.Sprintf(turnCapNotice, a.options.MaxTurns))
	return result, nil
}

// runTool runs one call. A tool failure is a result the model can read and
// work around, not a reason to abandon the question: a mistyped service id
// should cost one turn, not the answer.
func (a *Agent) runTool(ctx context.Context, call model.ToolCall) model.ToolResult {
	tool, ok := a.tools[call.Name]
	if !ok {
		return model.ToolResult{
			ID: call.ID, Name: call.Name, IsError: true,
			Content: "There is no tool called " + call.Name + ". Use one of the tools you were given.",
		}
	}
	output, err := tool.Run(ctx, call.Args)
	if err != nil {
		// The error text goes to the model, which may retry with better
		// arguments. It also goes to the process log, because a tool that
		// fails every time is an operator's problem, not the model's.
		log.Printf("tool %s: %v", call.Name, err)
		return model.ToolResult{
			ID: call.ID, Name: call.Name, IsError: true,
			Content: call.Name + " failed: " + err.Error(),
		}
	}
	return model.ToolResult{ID: call.ID, Name: call.Name, Content: output}
}

// fence wraps one tool result so the model can see where the data starts and
// where it stops. Everything a tool returns was written by somebody else — a
// log message, an event title, an operator's name — and some of it reads like
// an order. The system prompt says text inside this fence is data; the fence
// is what makes that sentence enforceable, because the model can tell which
// text it covers.
//
// The nonce is fresh per answer, so a log line cannot close the fence and
// speak as the loop: it would have to guess 128 random bits first.
func fence(nonce, content string) string {
	opening := `<tool_result nonce="` + nonce + `">`
	closing := `</tool_result nonce="` + nonce + `">`
	return opening + "\n" + content + "\n" + closing
}

// newNonce is 16 random bytes as hex. crypto/rand, not math/rand: a
// predictable tag is no tag at all.
func newNonce() string {
	var raw [16]byte
	// Since Go 1.24 crypto/rand.Read cannot fail; it panics rather than
	// return a short read, so there is no error to handle here.
	rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// lastText is the newest thing the model actually said, which may be several
// turns back if it spent the last ones calling tools.
func lastText(messages []model.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == model.RoleAssistant {
			if text := strings.TrimSpace(messages[i].Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func join(text, notice string) string {
	if text == "" {
		return notice
	}
	return text + "\n\n" + notice
}

func appendOnce(names []string, name string) []string {
	for _, existing := range names {
		if existing == name {
			return names
		}
	}
	return append(names, name)
}
