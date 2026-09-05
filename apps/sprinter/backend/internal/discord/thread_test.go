package discord

import (
	"encoding/json"
	"testing"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

func TestAllowedInChecksTheThreadsParentChannel(t *testing.T) {
	guard := store.Guard{
		Subject: store.SubjectAgentThread, GuildID: guild,
		RoleIDs: []string{leads}, ChannelIDs: []string{general},
	}
	// A thread's own id is never in a guard's channel list, so the parent is
	// offered with it. Offering the thread id alone would refuse everybody.
	if ok, why := allowedIn(guard, guild, []string{thread, general}, []string{leads}, true); !ok {
		t.Fatalf("a lead in the guarded channel must pass, got %q", why)
	}
	if ok, why := allowedIn(guard, guild, []string{thread, offtop}, []string{leads}, true); ok || why != reasonWrongChanel {
		t.Fatalf("a thread under an unguarded channel must be refused, got (%t, %q)", ok, why)
	}
	// A role taken away between the first question and the tenth must close
	// the thread to that person.
	if ok, why := allowedIn(guard, guild, []string{thread, general}, []string{guests}, true); ok || why != reasonWrongRole {
		t.Fatalf("a member without the role must be refused, got (%t, %q)", ok, why)
	}
	if ok, why := allowedIn(guard, guild, []string{thread, general}, nil, false); ok || why != reasonNoMember {
		t.Fatalf("no member means no roles to read, got (%t, %q)", ok, why)
	}
}

func TestDecodeTranscriptSkipsWhatItCannotRead(t *testing.T) {
	good, err := json.Marshal(model.Message{Role: model.RoleUser, Text: "how is the platform?"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	rows := []store.Message{
		{Sequence: 1, Content: good},
		{Sequence: 2, Content: json.RawMessage(`"not a turn"`)},
		{Sequence: 3, Content: good},
	}
	history, next := decodeTranscript(rows)
	// One bad row must not close the conversation, and the next sequence has
	// to come from the stored numbers rather than from the count, or the write
	// would collide with the row that failed to decode.
	if len(history) != 2 {
		t.Fatalf("history = %+v", history)
	}
	if next != 4 {
		t.Fatalf("next sequence = %d, want 4", next)
	}

	if _, next := decodeTranscript(nil); next != 1 {
		t.Fatalf("an empty thread must start at sequence 1, got %d", next)
	}
}

func TestLimiterAllowsOneQuestionPerPerson(t *testing.T) {
	limits := newLimiter(3)

	release, busy := limits.acquire("levon")
	if release == nil {
		t.Fatalf("the first question must be admitted, got %q", busy)
	}
	if _, busy := limits.acquire("levon"); busy != busyUserMessage {
		t.Fatalf("a second question from the same person must be refused, got %q", busy)
	}
	// Somebody else is unaffected.
	other, busy := limits.acquire("ada")
	if other == nil {
		t.Fatalf("another person must be admitted, got %q", busy)
	}

	release()
	again, busy := limits.acquire("levon")
	if again == nil {
		t.Fatalf("the slot must come back when the answer finishes, got %q", busy)
	}
	again()
	other()
}

func TestLimiterCapsTheWholeBot(t *testing.T) {
	limits := newLimiter(2)
	first, _ := limits.acquire("one")
	second, _ := limits.acquire("two")
	if first == nil || second == nil {
		t.Fatal("both slots must be free at the start")
	}

	release, busy := limits.acquire("three")
	if release != nil || busy != busyGlobalMessage {
		t.Fatalf("a third question must be refused, got %q", busy)
	}
	// The refused person must not be left marked busy, or they could never ask
	// again once a slot frees.
	first()
	third, busy := limits.acquire("three")
	if third == nil {
		t.Fatalf("the refused person must be able to ask again, got %q", busy)
	}
	third()
	second()
}

func TestEchoRunnerReturnsAStorableTranscript(t *testing.T) {
	reply, err := EchoRunner{}.Answer(t.Context(), Request{Question: "hello"})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if reply.Text != "Echo: hello" {
		t.Fatalf("text = %q", reply.Text)
	}
	// The bot stores whatever the runner returns, so even the echo has to
	// return a transcript a follow-up could replay.
	if len(reply.Messages) != 2 ||
		reply.Messages[0].Role != model.RoleUser ||
		reply.Messages[1].Role != model.RoleAssistant {
		t.Fatalf("messages = %+v", reply.Messages)
	}
	if _, err := (EchoRunner{}).Continue(t.Context(), Request{Question: "again"}, reply.Messages); err != nil {
		t.Fatalf("continue: %v", err)
	}
}
