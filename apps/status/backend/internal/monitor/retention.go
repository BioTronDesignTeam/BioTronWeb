package monitor

import (
	"context"
	"log"
	"time"
)

// Pruner deletes the health history recorded before a cutoff.
type Pruner interface {
	DeleteHealthBefore(context.Context, time.Time) (int64, error)
}

// Retain deletes the history older than keep, once at start and then every
// interval, until ctx ends. Without it health_checks grows for ever, although
// the status page never reads past its longest window.
func Retain(ctx context.Context, store Pruner, keep, interval time.Duration) {
	prune := func() {
		deleted, err := store.DeleteHealthBefore(ctx, time.Now().Add(-keep))
		switch {
		case err != nil && ctx.Err() == nil:
			log.Printf("prune health history: %v", err)
		case deleted > 0:
			log.Printf("pruned %d health checks older than %s", deleted, keep)
		}
	}
	prune()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}
