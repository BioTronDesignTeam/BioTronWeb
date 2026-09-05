package store

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLiveSeenOccurrences(t *testing.T) {
	store, ctx := liveStore(t)
	automation, err := store.CreateAutomation(ctx, Automation{
		Kind: KindAnnounce, Name: "seen live test", ScopeID: uuid.NewString(),
		ChannelID: "200000000000000001", LeadHours: 24, LookbackHours: 24,
		Deliver: DeliverChannel, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create automation: %v", err)
	}
	t.Cleanup(func() {
		if err := store.DeleteAutomation(ctx, automation.ID); err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("clean up automation: %v", err)
		}
	})

	starts := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	uid := "uid-" + uuid.NewString()
	saved, err := store.UpsertSeen(ctx, automation.ID, SeenOccurrence{
		UID: uid, RecurrenceIDLocal: "2026-09-10T09:00:00", Sequence: 1,
		StartsAt: starts, EndsAt: starts.Add(time.Hour), Title: "Kickoff",
		ScopeID: automation.ScopeID,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if saved.FirstSeenAt.IsZero() || saved.LastSeenAt.IsZero() {
		t.Fatalf("saved = %+v", saved)
	}
	firstSeen := saved.FirstSeenAt

	// The second call is the update path: first_seen_at must not move, but
	// last_seen_at, the sequence, and the title must.
	time.Sleep(10 * time.Millisecond)
	updated, err := store.UpsertSeen(ctx, automation.ID, SeenOccurrence{
		UID: uid, RecurrenceIDLocal: "2026-09-10T09:00:00", Sequence: 2,
		StartsAt: starts, EndsAt: starts.Add(time.Hour), Title: "Kickoff (moved)",
		ScopeID: automation.ScopeID,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !updated.FirstSeenAt.Equal(firstSeen) {
		t.Fatalf("first_seen_at moved: %v -> %v", firstSeen, updated.FirstSeenAt)
	}
	if !updated.LastSeenAt.After(saved.LastSeenAt) || updated.Sequence != 2 || updated.Title != "Kickoff (moved)" {
		t.Fatalf("updated = %+v", updated)
	}

	future, err := store.ListSeenFuture(ctx, automation.ID, time.Now())
	if err != nil {
		t.Fatalf("list future: %v", err)
	}
	found := false
	for _, occ := range future {
		if occ.UID == uid {
			found = true
		}
	}
	if !found {
		t.Fatalf("future = %+v, want %q", future, uid)
	}

	past, err := store.ListSeenFuture(ctx, automation.ID, starts.Add(time.Hour))
	if err != nil {
		t.Fatalf("list future past starts_at: %v", err)
	}
	for _, occ := range past {
		if occ.UID == uid {
			t.Fatalf("occurrence past its start still listed as future: %+v", occ)
		}
	}

	if err := store.DeleteSeen(ctx, automation.ID, uid, "2026-09-10T09:00:00"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.DeleteSeen(ctx, automation.ID, uid, "2026-09-10T09:00:00"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete again = %v, want ErrNotFound", err)
	}
}

func TestLivePostedOccurrences(t *testing.T) {
	store, ctx := liveStore(t)
	automation, err := store.CreateAutomation(ctx, Automation{
		Kind: KindAnnounce, Name: "posted live test", ScopeID: uuid.NewString(),
		ChannelID: "200000000000000001", LeadHours: 24, LookbackHours: 24,
		Deliver: DeliverChannel, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create automation: %v", err)
	}
	t.Cleanup(func() {
		if err := store.DeleteAutomation(ctx, automation.ID); err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("clean up automation: %v", err)
		}
	})

	uid := "uid-" + uuid.NewString()
	if _, err := store.GetPosted(ctx, automation.ID, uid, "2026-09-10T09:00:00"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing = %v, want ErrNotFound", err)
	}

	created, err := store.UpsertPosted(ctx, automation.ID, uid, "2026-09-10T09:00:00", 1,
		"200000000000000001", "300000000000000001")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if created.Sequence != 1 || created.MessageID != "300000000000000001" {
		t.Fatalf("created = %+v", created)
	}

	// A later edit at a higher sequence updates the same row rather than
	// inserting a second one.
	updated, err := store.UpsertPosted(ctx, automation.ID, uid, "2026-09-10T09:00:00", 2,
		"200000000000000001", "300000000000000001")
	if err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	if updated.Sequence != 2 {
		t.Fatalf("updated = %+v", updated)
	}
	got, err := store.GetPosted(ctx, automation.ID, uid, "2026-09-10T09:00:00")
	if err != nil || got.Sequence != 2 {
		t.Fatalf("get = %+v, %v", got, err)
	}
}

func TestLiveNudges(t *testing.T) {
	store, ctx := liveStore(t)
	lead := "400000000000000001"
	automation, err := store.CreateAutomation(ctx, Automation{
		Kind: KindNudge, Name: "nudge live test", ScopeID: uuid.NewString(),
		ChannelID: "200000000000000001", LeadUserID: &lead, LeadHours: 24,
		LookbackHours: 24, Deliver: DeliverDM, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create automation: %v", err)
	}
	t.Cleanup(func() {
		if err := store.DeleteAutomation(ctx, automation.ID); err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("clean up automation: %v", err)
		}
	})

	uid := "uid-" + uuid.NewString()
	has, err := store.HasNudge(ctx, automation.ID, uid, "2026-09-10T09:00:00", 1, TriggerUpcoming)
	if err != nil {
		t.Fatalf("has nudge before insert: %v", err)
	}
	if has {
		t.Fatal("has nudge before insert = true")
	}

	messageID := "300000000000000002"
	created, err := store.InsertNudge(ctx, automation.ID, uid, "2026-09-10T09:00:00", 1,
		TriggerUpcoming, NudgeSent, &messageID)
	if err != nil {
		t.Fatalf("insert nudge: %v", err)
	}
	if created.Trigger != TriggerUpcoming || created.Status != NudgeSent ||
		created.MessageID == nil || *created.MessageID != messageID {
		t.Fatalf("created = %+v", created)
	}

	has, err = store.HasNudge(ctx, automation.ID, uid, "2026-09-10T09:00:00", 1, TriggerUpcoming)
	if err != nil {
		t.Fatalf("has nudge after insert: %v", err)
	}
	if !has {
		t.Fatal("has nudge after insert = false")
	}

	// The uniqueness the scheduler leans on: the same occurrence, sequence,
	// and trigger cannot be nudged twice.
	if _, err := store.InsertNudge(ctx, automation.ID, uid, "2026-09-10T09:00:00", 1,
		TriggerUpcoming, NudgeSatisfied, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate nudge = %v, want ErrConflict", err)
	}

	// A different trigger for the same occurrence and sequence is a distinct
	// row.
	if _, err := store.InsertNudge(ctx, automation.ID, uid, "2026-09-10T09:00:00", 1,
		TriggerCancelled, NudgeSatisfied, nil); err != nil {
		t.Fatalf("insert cancelled trigger: %v", err)
	}
}
