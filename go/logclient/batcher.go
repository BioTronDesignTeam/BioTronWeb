package logclient

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// batcher delivers items to one destination in the background. add never
// blocks: past the queue's capacity it drops the item, so a slow or dead
// destination can never stall the service. One goroutine sends a batch when
// it is full or when the interval passes, each send under its own timeout,
// and a failed batch is dropped rather than retried.
type batcher[T any] struct {
	name     string
	queue    chan T
	send     func(ctx context.Context, batch []T) error
	maxBatch int
	interval time.Duration
	timeout  time.Duration

	closeOnce sync.Once
	closing   chan struct{}
	done      chan struct{}
	dropped   atomic.Int64
	// failing is touched only by the run goroutine. It keeps a destination
	// that is down from writing one line per batch to the standard log.
	failing bool
}

type batchOptions struct {
	queue    int
	maxBatch int
	interval time.Duration
	timeout  time.Duration
}

var defaultBatchOptions = batchOptions{queue: 1024, maxBatch: 100, interval: time.Second, timeout: 5 * time.Second}

func newBatcher[T any](name string, options batchOptions, send func(ctx context.Context, batch []T) error) *batcher[T] {
	b := &batcher[T]{
		name:     name,
		queue:    make(chan T, options.queue),
		send:     send,
		maxBatch: options.maxBatch,
		interval: options.interval,
		timeout:  options.timeout,
		closing:  make(chan struct{}),
		done:     make(chan struct{}),
	}
	go b.run()
	return b
}

func (b *batcher[T]) add(item T) {
	select {
	case <-b.closing:
		return
	default:
	}
	select {
	case b.queue <- item:
	default:
		// Say so on the first drop and every thousandth after it, so a
		// flood of events cannot become a flood of warnings.
		if n := b.dropped.Add(1); n%1000 == 1 {
			log.Printf("%s delivery busy; %d events dropped so far", b.name, n)
		}
	}
}

// close sends what is queued and stops. It waits until that is done or ctx
// expires, whichever is first.
func (b *batcher[T]) close(ctx context.Context) error {
	b.closeOnce.Do(func() { close(b.closing) })
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *batcher[T]) run() {
	defer close(b.done)
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	batch := make([]T, 0, b.maxBatch)
	for {
		select {
		case item := <-b.queue:
			batch = append(batch, item)
			if len(batch) >= b.maxBatch {
				batch = b.flush(batch)
			}
		case <-ticker.C:
			batch = b.flush(batch)
		case <-b.closing:
			for {
				select {
				case item := <-b.queue:
					batch = append(batch, item)
					if len(batch) >= b.maxBatch {
						batch = b.flush(batch)
					}
				default:
					b.flush(batch)
					return
				}
			}
		}
	}
}

func (b *batcher[T]) flush(batch []T) []T {
	if len(batch) == 0 {
		return batch
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	err := b.send(ctx, batch)
	cancel()
	if err != nil && !b.failing {
		log.Printf("%s delivery failed; dropping events until it recovers: %v", b.name, err)
	} else if err == nil && b.failing {
		log.Printf("%s delivery recovered", b.name)
	}
	b.failing = err != nil
	return batch[:0]
}
