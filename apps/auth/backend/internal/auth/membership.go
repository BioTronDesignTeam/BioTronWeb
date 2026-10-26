package auth

import (
	"context"
	"sync"
	"time"
)

// MembershipTTL is how long the answer of one membership check is kept. A
// person removed from the organization loses access within this long, however
// long their session cookie has left.
const MembershipTTL = 15 * time.Minute

// Membership keeps the answer of the last organization membership check of
// each operator, keyed by GitHub id, and asks GitHub again once it is older
// than MembershipTTL. Requests of one operator that find no fresh answer share
// one check. Its methods are safe for use by many goroutines.
//
// It fails closed: when GitHub cannot answer and no answer of the last
// MembershipTTL is kept, Member returns the error and the request is refused
// with 503. A kept answer is never used past its TTL, so an outage of GitHub
// cannot stretch a removed member's access.
type Membership struct {
	check func(ctx context.Context, login string) (bool, error)
	ttl   time.Duration
	now   func() time.Time

	mu       sync.Mutex
	kept     map[int64]membershipAnswer
	checking map[int64]*membershipCheck
}

type membershipAnswer struct {
	member    bool
	checkedAt time.Time
}

type membershipCheck struct {
	done   chan struct{}
	member bool
	err    error
}

// NewMembership returns an empty cache that asks check, such as
// GitHubClient.IsOrgMember.
func NewMembership(check func(ctx context.Context, login string) (bool, error)) *Membership {
	return &Membership{
		check:    check,
		ttl:      MembershipTTL,
		now:      time.Now,
		kept:     map[int64]membershipAnswer{},
		checking: map[int64]*membershipCheck{},
	}
}

// Member reports whether the operator with githubID and login is a member,
// from an answer of the last MembershipTTL or from a new check.
func (m *Membership) Member(ctx context.Context, githubID int64, login string) (bool, error) {
	m.mu.Lock()
	if answer, ok := m.kept[githubID]; ok && m.fresh(answer, m.now()) {
		m.mu.Unlock()
		return answer.member, nil
	}
	check := m.checking[githubID]
	if check == nil {
		check = &membershipCheck{done: make(chan struct{})}
		m.checking[githubID] = check
		// The check serves every waiting request of the operator, so the
		// first one's cancellation must not fail the others.
		go m.run(context.WithoutCancel(ctx), githubID, login, check)
	}
	m.mu.Unlock()
	select {
	case <-check.done:
		return check.member, check.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (m *Membership) run(ctx context.Context, githubID int64, login string, check *membershipCheck) {
	member, err := m.check(ctx, login)
	m.mu.Lock()
	delete(m.checking, githubID)
	if err == nil {
		m.keepLocked(githubID, member)
	}
	m.mu.Unlock()
	check.member, check.err = member, err
	close(check.done)
}

// Keep records an answer that was just asked elsewhere, such as at sign-in.
func (m *Membership) Keep(githubID int64, member bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keepLocked(githubID, member)
}

func (m *Membership) keepLocked(githubID int64, member bool) {
	now := m.now()
	// Drop the old answers, so the map holds only the operators seen within
	// the last TTL.
	for id, answer := range m.kept {
		if !m.fresh(answer, now) {
			delete(m.kept, id)
		}
	}
	m.kept[githubID] = membershipAnswer{member: member, checkedAt: now}
}

func (m *Membership) fresh(answer membershipAnswer, now time.Time) bool {
	age := now.Sub(answer.checkedAt)
	return age >= 0 && age < m.ttl
}
