package monitor

import (
	"context"
	"testing"
	"time"
)

type pruner struct {
	cutoffs chan time.Time
}

func (p pruner) DeleteHealthBefore(_ context.Context, cutoff time.Time) (int64, error) {
	p.cutoffs <- cutoff
	return 0, nil
}

func TestRetainPrunesAtStartAndOnEveryTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := pruner{cutoffs: make(chan time.Time, 8)}
	keep := 91 * 24 * time.Hour
	done := make(chan struct{})
	go func() {
		Retain(ctx, store, keep, 10*time.Millisecond)
		close(done)
	}()

	for i := range 2 {
		select {
		case cutoff := <-store.cutoffs:
			if age := time.Since(cutoff); age < keep || age > keep+time.Minute {
				t.Fatalf("prune %d cut at %s ago, want %s", i, age, keep)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("prune %d never ran", i)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Retain did not stop with its context")
	}
}
