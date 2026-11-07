package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model"
)

// stalledModel never answers on its own. Only its context ends the call.
type stalledModel struct{}

func (stalledModel) Name() string { return "stalled" }

func (stalledModel) Generate(ctx context.Context, _ string, _ []model.Message, _ []model.ToolSpec) (model.Response, error) {
	<-ctx.Done()
	return model.Response{}, ctx.Err()
}

// A stalled draft must not hold the tick. It times out, and the nudge goes
// out without a draft, on a scheduler context that has no deadline at all.
func TestAStalledDraftTimesOutAndTheNudgeStillGoes(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := nudgeAutomation()
	occ := calendar.Occurrence{
		UID: "uid-stall", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	fd := newFakeDiscord()
	s := New(Deps{
		Store: newFakeStore(automation), Calendar: &fakeCalendar{occurrences: []calendar.Occurrence{occ}},
		Discord: fd, Model: stalledModel{}, Location: toronto, Now: fixedNow(now),
		ModelTimeout: 50 * time.Millisecond,
	})

	done := make(chan error, 1)
	go func() { done <- s.Tick(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Tick: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the tick was still waiting on the model")
	}
	if len(fd.dms) != 1 || !strings.Contains(fd.dms[0].content, "could not be generated") {
		t.Fatalf("dms = %+v, want the nudge without a draft", fd.dms)
	}
}

func TestNewDefaultsTheModelTimeout(t *testing.T) {
	if got := New(Deps{}).modelTimeout; got != DefaultModelTimeout {
		t.Fatalf("modelTimeout = %v, want %v", got, DefaultModelTimeout)
	}
}
