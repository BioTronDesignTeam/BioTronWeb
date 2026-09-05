package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
)

// These run every query in this package against a real database, because the
// enum casts and the text[] columns are exactly what a compiler cannot check.
// Set SPRINTER_TEST_DATABASE_URL to a database with the sprinter schema
// migrated; without it the tests skip.
//
//	SPRINTER_TEST_DATABASE_URL='postgresql://biotron:change-me@127.0.0.1:15433/biotron?schema=sprinter' \
//	  go test ./internal/store/ -run Live -v
//
// Every row they write carries a generated id and is removed at the end, so
// they can run against the development database without disturbing it.
func liveStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("SPRINTER_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SPRINTER_TEST_DATABASE_URL is unset")
	}
	ctx := context.Background()
	store, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(store.Close)
	return store, ctx
}

func TestLiveGuards(t *testing.T) {
	store, ctx := liveStore(t)
	// The subject is a primary key with four legal values, so this test takes
	// one of them and puts back whatever it found.
	const subject = SubjectNudge
	before, err := store.GetGuard(ctx, subject)
	restore := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatalf("read existing guard: %v", err)
	}
	t.Cleanup(func() {
		if restore {
			if _, err := store.UpsertGuard(ctx, before); err != nil {
				t.Errorf("restore guard: %v", err)
			}
			return
		}
		if err := store.DeleteGuard(ctx, subject); err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("clean up guard: %v", err)
		}
	})

	saved, err := store.UpsertGuard(ctx, Guard{
		Subject: subject, GuildID: "100000000000000001",
		RoleIDs: []string{"300000000000000001"}, ChannelIDs: []string{},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(saved.RoleIDs) != 1 || len(saved.ChannelIDs) != 0 || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved = %+v", saved)
	}
	// The second write must update rather than fail on the primary key.
	if _, err := store.UpsertGuard(ctx, Guard{
		Subject: subject, GuildID: "100000000000000002",
		RoleIDs: []string{"300000000000000002"}, ChannelIDs: []string{"200000000000000001"},
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got, err := store.GetGuard(ctx, subject)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GuildID != "100000000000000002" || len(got.ChannelIDs) != 1 {
		t.Fatalf("got = %+v", got)
	}
	guards, err := store.ListGuards(ctx)
	if err != nil || len(guards) == 0 {
		t.Fatalf("list = %v, %v", guards, err)
	}
	if _, err := store.GetGuard(ctx, "no-such-subject"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing guard = %v, want ErrNotFound", err)
	}
}

func TestLiveAutomations(t *testing.T) {
	store, ctx := liveStore(t)
	automation := Automation{
		Kind: KindAnnounce, Name: "live test", ScopeID: uuid.NewString(),
		ChannelID: "200000000000000001", LeadHours: 24, LookbackHours: 24,
		Deliver: DeliverChannel, Enabled: true,
	}
	created, err := store.CreateAutomation(ctx, automation)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		if err := store.DeleteAutomation(ctx, created.ID); err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("clean up automation: %v", err)
		}
	})
	if created.Kind != KindAnnounce || created.Deliver != DeliverChannel || created.PostHour != nil {
		t.Fatalf("created = %+v", created)
	}

	hour := 9
	lead := "400000000000000001"
	created.Kind = KindNudge
	created.Deliver = DeliverDM
	created.PostHour = &hour
	created.LeadUserID = &lead
	created.Enabled = false
	updated, err := store.UpdateAutomation(ctx, created)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Kind != KindNudge || updated.PostHour == nil || *updated.PostHour != 9 ||
		updated.LeadUserID == nil || *updated.LeadUserID != lead || updated.Enabled {
		t.Fatalf("updated = %+v", updated)
	}
	if _, err := store.GetAutomation(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing automation = %v, want ErrNotFound", err)
	}
	if _, err := store.ListAutomations(ctx); err != nil {
		t.Fatalf("list: %v", err)
	}
}

func TestLiveThreadsAndMessages(t *testing.T) {
	store, ctx := liveStore(t)
	threadID := "999" + uuid.NewString()[:15]
	thread, err := store.CreateThread(ctx, Thread{
		ThreadID: threadID, GuildID: "100000000000000001",
		ChannelID: "200000000000000001", OpenerID: "400000000000000001",
		Model: "echo", TurnCount: 1,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	t.Cleanup(func() {
		// The messages go with it: the foreign key cascades on delete.
		if _, err := store.pool.Exec(ctx, `DELETE FROM agent_threads WHERE thread_id = $1`, threadID); err != nil {
			t.Errorf("clean up thread: %v", err)
		}
	})
	if thread.TurnCount != 1 || thread.CreatedAt.IsZero() {
		t.Fatalf("thread = %+v", thread)
	}

	if _, err := store.AppendMessage(ctx, threadID, 1, "user", json.RawMessage(`{"role":"user","text":"hello"}`)); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if _, err := store.AppendMessage(ctx, threadID, 2, "assistant", json.RawMessage(`{"role":"assistant","text":"Echo: hello"}`)); err != nil {
		t.Fatalf("append second: %v", err)
	}
	// A replayed Discord event must not duplicate a turn.
	if _, err := store.AppendMessage(ctx, threadID, 1, "user", json.RawMessage(`{}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate sequence = %v, want ErrConflict", err)
	}
	messages, err := store.ListMessages(ctx, threadID)
	if err != nil || len(messages) != 2 || messages[0].Sequence != 1 || messages[1].Role != "assistant" {
		t.Fatalf("messages = %+v, %v", messages, err)
	}

	touched, err := store.TouchThread(ctx, threadID)
	if err != nil {
		t.Fatalf("touch: %v", err)
	}
	if touched.TurnCount != 2 || !touched.LastActiveAt.After(thread.LastActiveAt) {
		t.Fatalf("touched = %+v, was %+v", touched, thread)
	}
	if _, err := store.GetThread(ctx, "no-such-thread"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing thread = %v, want ErrNotFound", err)
	}
}
