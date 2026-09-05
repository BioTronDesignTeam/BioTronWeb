package discord

import (
	"context"
	"strings"
)

// messageLimit is Discord's per-message character cap.
const messageLimit = 2000

// Request is one question, with everything the gate already established about
// where it came from. The runner needs the ids to scope what it may read.
type Request struct {
	Subject   string
	GuildID   string
	ChannelID string
	UserID    string
	Question  string
}

// Runner answers a question. The agent loop will implement it; until then
// EchoRunner does, so the command path can be finished and tested without a
// model key.
type Runner interface {
	Answer(ctx context.Context, req Request) (string, error)
}

// EchoRunner repeats the question back. It exists so that every step around
// the model — the gate, the deferred reply, the follow-up, the thread, the
// transcript row — is exercised before the model lands.
type EchoRunner struct{}

func (EchoRunner) Answer(_ context.Context, req Request) (string, error) {
	return "Echo: " + req.Question, nil
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
