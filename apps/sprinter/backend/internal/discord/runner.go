package discord

import (
	"context"
	"strings"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
)

// messageLimit is Discord's per-message character cap.
const messageLimit = 2000

// Request is one question, with everything the gate already established about
// where it came from. The runner needs the ids to scope what it may read.
type Request struct {
	Subject   string
	GuildID   string
	ChannelID string
	// ThreadID is set on a follow-up inside a thread and empty on a command.
	ThreadID string
	UserID   string
	Question string
}

// Reply is one answered question. Messages is the transcript the answer built,
// which the bot stores as the thread's turns: the loop knows how many turns it
// took and what each tool returned, and the bot must not guess at that.
type Reply struct {
	Text         string
	Messages     []model.Message
	Tools        []string
	InputTokens  int
	OutputTokens int
}

// Runner answers a question. The agent loop implements it; EchoRunner does too,
// so the command path, the thread, and the transcript can be tested without a
// model key.
type Runner interface {
	Answer(ctx context.Context, req Request) (Reply, error)
	Continue(ctx context.Context, req Request, history []model.Message) (Reply, error)
}

// EchoRunner repeats the question back. It is what runs when GEMINI_API_KEY is
// unset, so a missing key costs the answers and nothing else.
type EchoRunner struct{}

func (EchoRunner) Answer(_ context.Context, req Request) (Reply, error) {
	return echo(req), nil
}

func (EchoRunner) Continue(_ context.Context, req Request, _ []model.Message) (Reply, error) {
	return echo(req), nil
}

func echo(req Request) Reply {
	text := "Echo: " + req.Question
	return Reply{
		Text: text,
		Messages: []model.Message{
			{Role: model.RoleUser, Text: req.Question},
			{Role: model.RoleAssistant, Text: text},
		},
	}
}

// splitMessage cuts an answer into messages Discord will accept, preferring a
// line boundary. A single line longer than the limit is cut at the limit,
// because there is nothing better to cut at.
func splitMessage(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var parts []string
	for len(text) > limit {
		cut := strings.LastIndex(text[:limit+1], "\n")
		if cut <= 0 {
			cut = limit
		}
		parts = append(parts, strings.TrimRight(text[:cut], "\n"))
		text = strings.TrimLeft(text[cut:], "\n")
	}
	if text != "" {
		parts = append(parts, text)
	}
	return parts
}

// threadName is the title Discord shows on the thread. Discord caps it at 100
// characters; 80 leaves the name readable in the channel list.
func threadName(question string) string {
	name := strings.TrimSpace(strings.ReplaceAll(question, "\n", " "))
	runes := []rune(name)
	if len(runes) > 80 {
		return string(runes[:80])
	}
	if len(runes) == 0 {
		return "Agent thread"
	}
	return name
}
