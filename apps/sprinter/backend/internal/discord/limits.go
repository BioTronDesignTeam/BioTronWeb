package discord

import "sync"

// maxConcurrent is how many questions the bot answers at once. Each one holds
// a model call, a database pool connection, and up to two minutes; three is
// enough for a team of this size and small enough that a burst cannot starve
// the admin API sharing the process.
const maxConcurrent = 3

// The two refusals. They differ because the fix differs: one person waits for
// their own answer, everybody else waits for a slot.
const (
	busyUserMessage   = "You already have a question running. Wait for that answer, then ask again."
	busyGlobalMessage = "Sprinter is answering as many questions as it can right now. Try again in a moment."
)

// limiter caps how many questions run at once, per person and overall.
//
// One question per person is the important half: a model call costs money and
// a person who spams the command would otherwise hold every slot. Both checks
// refuse rather than queue, because a queued question answers minutes later
// into a conversation that has moved on.
type limiter struct {
	mu    sync.Mutex
	busy  map[string]bool
	slots chan struct{}
}

func newLimiter(concurrent int) *limiter {
	return &limiter{busy: map[string]bool{}, slots: make(chan struct{}, concurrent)}
}

// acquire takes a slot for one person. It returns the release function and an
// empty string on success, or the sentence to show them on refusal.
func (l *limiter) acquire(userID string) (func(), string) {
	l.mu.Lock()
	if l.busy[userID] {
		l.mu.Unlock()
		return nil, busyUserMessage
	}
	l.busy[userID] = true
	l.mu.Unlock()

	select {
	case l.slots <- struct{}{}:
	default:
		// The global slot was not free, so the personal one has to go back or
		// this person could never ask again.
		l.clear(userID)
		return nil, busyGlobalMessage
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			<-l.slots
			l.clear(userID)
		})
	}, ""
}

func (l *limiter) clear(userID string) {
	l.mu.Lock()
	delete(l.busy, userID)
	l.mu.Unlock()
}
