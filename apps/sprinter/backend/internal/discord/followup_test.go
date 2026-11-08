package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store"
)

// threadStore keeps one thread and its transcript the way Postgres does:
// a turn's sequence is unique, and TouchThread counts the turn.
type threadStore struct {
	mu     sync.Mutex
	thread store.Thread
	rows   []store.Message
	// conflicts counts appends that hit an existing sequence.
	conflicts int
}

func (s *threadStore) GetGuard(context.Context, string) (store.Guard, error) {
	return openGuard(commandAgentThread), nil
}

func (s *threadStore) CreateThread(_ context.Context, thread store.Thread) (store.Thread, error) {
	return thread, nil
}

func (s *threadStore) GetThread(_ context.Context, threadID string) (store.Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if threadID != s.thread.ThreadID {
		return store.Thread{}, store.ErrNotFound
	}
	return s.thread, nil
}

func (s *threadStore) TouchThread(_ context.Context, _ string) (store.Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.thread.TurnCount++
	return s.thread, nil
}

func (s *threadStore) AppendMessage(_ context.Context, threadID string, sequence int, role string, content json.RawMessage) (store.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.rows {
		if row.Sequence == sequence {
			s.conflicts++
			return store.Message{}, fmt.Errorf("sequence %d: %w", sequence, store.ErrConflict)
		}
	}
	row := store.Message{ThreadID: threadID, Sequence: sequence, Role: role, Content: content}
	s.rows = append(s.rows, row)
	return row, nil
}

func (s *threadStore) ListMessages(context.Context, string) ([]store.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.Message(nil), s.rows...), nil
}

// slowRunner takes a moment to answer, long enough for a second follow-up
// to arrive while the first is still running. It records how much history
// each call saw.
type slowRunner struct {
	mu   sync.Mutex
	seen []int
}

func (r *slowRunner) Answer(context.Context, Request) (Reply, error) { return Reply{}, nil }

func (r *slowRunner) Continue(_ context.Context, req Request, history []model.Message) (Reply, error) {
	r.mu.Lock()
	r.seen = append(r.seen, len(history))
	r.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	return Reply{Text: "answer", Messages: []model.Message{
		{Role: model.RoleUser, Text: req.Question},
		{Role: model.RoleAssistant, Text: "answer"},
	}}, nil
}

func followUp(author, content string) *discordgo.MessageCreate {
	return &discordgo.MessageCreate{Message: &discordgo.Message{
		ChannelID: thread, GuildID: guild, Content: content, Type: discordgo.MessageTypeDefault,
		Author: &discordgo.User{ID: author},
		Member: &discordgo.Member{Roles: []string{leads}},
	}}
}

// Two people posting in one thread at once used to read the same transcript
// and write their turns at the same sequence numbers. They now take turns.
func TestConcurrentFollowUpsInOneThreadTakeTurns(t *testing.T) {
	sprinterStore := &threadStore{thread: store.Thread{
		ThreadID: thread, GuildID: guild, ChannelID: general, TurnCount: 1,
	}}
	runner := &slowRunner{}
	bot := testBot(newFakeAPI(), sprinterStore, runner)

	var wg sync.WaitGroup
	for _, author := range []string{"400000000000000001", "400000000000000002"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bot.handleThreadMessage(followUp(author, "and the next one?"))
		}()
	}
	wg.Wait()

	if sprinterStore.conflicts != 0 {
		t.Fatalf("%d turns collided on a sequence number", sprinterStore.conflicts)
	}
	if len(sprinterStore.rows) != 4 || sprinterStore.thread.TurnCount != 3 {
		t.Fatalf("rows = %d, turns = %d, want 4 rows and 3 turns", len(sprinterStore.rows), sprinterStore.thread.TurnCount)
	}
	if len(runner.seen) != 2 || runner.seen[0] == runner.seen[1] {
		t.Fatalf("history lengths = %v, want the second answer to see the first", runner.seen)
	}
}

// The turn cap is checked once it is this follow-up's turn, so two people
// racing for a thread's last turn cannot both have it.
func TestTheLastTurnGoesToOneFollowUp(t *testing.T) {
	sprinterStore := &threadStore{thread: store.Thread{
		ThreadID: thread, GuildID: guild, ChannelID: general, TurnCount: DefaultMaxThreadTurns - 1,
	}}
	runner := &slowRunner{}
	api := newFakeAPI()
	bot := testBot(api, sprinterStore, runner)

	var wg sync.WaitGroup
	for _, author := range []string{"400000000000000001", "400000000000000002"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bot.handleThreadMessage(followUp(author, "one more?"))
		}()
	}
	wg.Wait()

	if len(runner.seen) != 1 || sprinterStore.thread.TurnCount != DefaultMaxThreadTurns {
		t.Fatalf("answers = %d, turns = %d, want one answer to take the last turn", len(runner.seen), sprinterStore.thread.TurnCount)
	}
}

func TestThreadLocksForgetIdleThreads(t *testing.T) {
	locks := newThreadLocks()
	unlock := locks.lock("a")
	unlock()
	if len(locks.locks) != 0 {
		t.Fatalf("locks = %v, want none held", locks.locks)
	}
}
