package logclient

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

func TestBatcherSendsFullBatchesAndFlushesOnClose(t *testing.T) {
	var mu sync.Mutex
	var batches [][]int
	b := newBatcher("test", batchOptions{queue: 16, maxBatch: 3, interval: time.Hour, timeout: time.Second},
		func(ctx context.Context, batch []int) error {
			mu.Lock()
			defer mu.Unlock()
			batches = append(batches, append([]int(nil), batch...))
			return nil
		})
	for i := range 5 {
		b.add(i)
	}
	if err := b.close(context.Background()); err != nil {
		t.Fatal(err)
	}
	b.add(99) // after close: dropped, never panics
	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 2 || len(batches[0]) != 3 || len(batches[1]) != 2 || batches[1][1] != 4 {
		t.Fatalf("batches = %v, want [[0 1 2] [3 4]]", batches)
	}
}

func TestBatcherSendsOnInterval(t *testing.T) {
	sent := make(chan []int, 1)
	b := newBatcher("test", batchOptions{queue: 16, maxBatch: 100, interval: 10 * time.Millisecond, timeout: time.Second},
		func(ctx context.Context, batch []int) error {
			sent <- append([]int(nil), batch...)
			return nil
		})
	defer func() { _ = b.close(context.Background()) }()
	b.add(7)
	select {
	case batch := <-sent:
		if len(batch) != 1 || batch[0] != 7 {
			t.Fatalf("batch = %v", batch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interval flush never happened")
	}
}

func TestBatcherDropsWhenFullAndSurvivesFailures(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	release := make(chan struct{})
	b := newBatcher("test", batchOptions{queue: 2, maxBatch: 1, interval: time.Hour, timeout: time.Second},
		func(ctx context.Context, batch []int) error {
			<-release
			return errors.New("destination down")
		})
	// One item is held by the blocked send, two fill the queue, the rest drop.
	for i := range 10 {
		b.add(i)
	}
	if b.dropped.Load() == 0 {
		t.Fatal("a full queue must drop instead of blocking")
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBatcherCloseGivesUpAtTheDeadline(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	b := newBatcher("test", batchOptions{queue: 2, maxBatch: 1, interval: time.Hour, timeout: time.Second},
		func(ctx context.Context, batch []int) error {
			<-release
			return nil
		})
	b.add(1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close = %v, want the deadline", err)
	}
}
