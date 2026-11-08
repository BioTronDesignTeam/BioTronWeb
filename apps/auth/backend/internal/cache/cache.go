// Package cache keeps each operator's standing and grants, and which sessions
// were signed out, in memory for a short while, so the permission check that
// every tool makes on every page does not reach Postgres each time.
//
// One process serves Auth, so the cache is a map, not Redis. Every change
// Auth makes to an operator (a grant, a revoke, a ban, the manager flag)
// invalidates that operator at once, and rotating a daily guest key
// invalidates that app's keys, and a sign-out marks its session revoked, so
// the change applies to the very next request. The TTL only bounds how long
// a change made outside this process, such as by hand in psql, can go unseen.
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

// TTL is the longest an operator's standing or grants are kept.
const TTL = 30 * time.Second

// Source is where the cache loads from: the store.
type Source interface {
	GetOperator(ctx context.Context, githubID int64) (*store.OrgMember, error)
	ListGrantsForOperator(ctx context.Context, operatorID int64) ([]store.Grant, error)
	// GetGuestKey returns the daily key of an app for one Eastern day, or
	// store.ErrNotFound when there is none or the app no longer has daily keys.
	GetGuestKey(ctx context.Context, appID, day string) (string, error)
	// IsSessionRevoked reports whether a session was signed out, by its id.
	IsSessionRevoked(ctx context.Context, sessionID string) (bool, error)
}

type kept[T any] struct {
	value    T
	loadedAt time.Time
}

// Cache is safe for use by many goroutines.
type Cache struct {
	src Source
	ttl time.Duration
	now func() time.Time

	mu        sync.Mutex
	operators map[int64]kept[store.OrgMember]
	grants    map[int64]kept[[]store.Grant]
	// generation counts the invalidations of each operator. A load that
	// raced an invalidation is not kept, so a revoke can never be undone by
	// a read that started before it.
	generation map[int64]uint64
	// guestKeys holds each app's daily key by app and day, so every request
	// of a guest session can check that its key still stands, and
	// guestKeyGeneration counts the invalidations of each app's keys.
	guestKeys          map[guestKeyID]kept[string]
	guestKeyGeneration map[string]uint64
	// revoked holds whether each session id was signed out. A revocation by
	// this process is kept at once, so it applies to the very next request.
	revoked map[string]kept[bool]
}

type guestKeyID struct{ app, day string }

func New(src Source) *Cache {
	return &Cache{
		src:                src,
		ttl:                TTL,
		now:                time.Now,
		operators:          map[int64]kept[store.OrgMember]{},
		grants:             map[int64]kept[[]store.Grant]{},
		generation:         map[int64]uint64{},
		guestKeys:          map[guestKeyID]kept[string]{},
		guestKeyGeneration: map[string]uint64{},
		revoked:            map[string]kept[bool]{},
	}
}

// Operator returns an operator's row, from the last TTL or from the store.
// A missing operator is store.ErrNotFound, and is not kept.
func (c *Cache) Operator(ctx context.Context, githubID int64) (*store.OrgMember, error) {
	op, gen, ok := lookup(c, c.operators, githubID)
	if ok {
		return &op, nil
	}
	loaded, err := c.src.GetOperator(ctx, githubID)
	if err != nil {
		return nil, err
	}
	keep(c, c.operators, githubID, *loaded, gen)
	return loaded, nil
}

// Grants returns an operator's grants, from the last TTL or from the store.
// The slice is shared: callers must not change it.
func (c *Cache) Grants(ctx context.Context, operatorID int64) ([]store.Grant, error) {
	grants, gen, ok := lookup(c, c.grants, operatorID)
	if ok {
		return grants, nil
	}
	grants, err := c.src.ListGrantsForOperator(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	if grants == nil {
		grants = []store.Grant{}
	}
	keep(c, c.grants, operatorID, grants, gen)
	return grants, nil
}

// HasGrant reports whether the operator holds one permission.
func (c *Cache) HasGrant(ctx context.Context, operatorID int64, appID, permissionKey string) (bool, error) {
	grants, err := c.Grants(ctx, operatorID)
	if err != nil {
		return false, err
	}
	for _, g := range grants {
		if g.AppID == appID && g.PermissionKey == permissionKey {
			return true, nil
		}
	}
	return false, nil
}

// GuestKey returns an app's daily key for one Eastern day, from the last TTL
// or from the store. A missing key is store.ErrNotFound, and is not kept.
func (c *Cache) GuestKey(ctx context.Context, appID, day string) (string, error) {
	id := guestKeyID{app: appID, day: day}
	c.mu.Lock()
	entry, ok := c.guestKeys[id]
	if ok {
		if age := c.now().Sub(entry.loadedAt); age >= 0 && age < c.ttl {
			c.mu.Unlock()
			return entry.value, nil
		}
		delete(c.guestKeys, id)
	}
	gen := c.guestKeyGeneration[appID]
	c.mu.Unlock()

	key, err := c.src.GetGuestKey(ctx, appID, day)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.guestKeyGeneration[appID] == gen {
		now := c.now()
		for k, entry := range c.guestKeys {
			if now.Sub(entry.loadedAt) >= c.ttl {
				delete(c.guestKeys, k)
			}
		}
		c.guestKeys[id] = kept[string]{value: key, loadedAt: now}
	}
	return key, nil
}

// InvalidateGuestKeys forgets every daily key of one app, so the next guest
// request reads the store. A load that raced it is not kept.
func (c *Cache) InvalidateGuestKeys(appID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id := range c.guestKeys {
		if id.app == appID {
			delete(c.guestKeys, id)
		}
	}
	c.guestKeyGeneration[appID]++
}

// Invalidate forgets one operator, so the next request reads the store.
func (c *Cache) Invalidate(githubID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.operators, githubID)
	delete(c.grants, githubID)
	c.generation[githubID]++
}

// lookup returns a fresh entry, or the operator's generation to hand to keep
// after loading.
func lookup[T any](c *Cache, m map[int64]kept[T], id int64) (T, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := m[id]
	if ok {
		if age := c.now().Sub(entry.loadedAt); age >= 0 && age < c.ttl {
			return entry.value, 0, true
		}
		delete(m, id)
	}
	var zero T
	return zero, c.generation[id], false
}

func keep[T any](c *Cache, m map[int64]kept[T], id int64, value T, gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation[id] != gen {
		return
	}
	now := c.now()
	// Drop stale entries as new ones arrive, so the maps hold only the
	// operators seen within the last TTL.
	for k, entry := range m {
		if now.Sub(entry.loadedAt) >= c.ttl {
			delete(m, k)
		}
	}
	m[id] = kept[T]{value: value, loadedAt: now}
}

// Revoked reports whether the session with this id was signed out, from the
// last TTL or from the store. A revocation made through MarkRevoked is seen
// at once; one made by hand in psql within the TTL.
func (c *Cache) Revoked(ctx context.Context, sessionID string) (bool, error) {
	c.mu.Lock()
	entry, ok := c.revoked[sessionID]
	if ok && (entry.value || c.now().Sub(entry.loadedAt) < c.ttl) {
		c.mu.Unlock()
		return entry.value, nil
	}
	c.mu.Unlock()

	revoked, err := c.src.IsSessionRevoked(ctx, sessionID)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// A MarkRevoked that raced this load wins: a revocation is never undone.
	if current, ok := c.revoked[sessionID]; ok && current.value {
		return true, nil
	}
	c.keepRevokedLocked(sessionID, revoked)
	return revoked, nil
}

// MarkRevoked records that this process signed the session out, so the next
// request with it is refused without waiting for the TTL.
func (c *Cache) MarkRevoked(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keepRevokedLocked(sessionID, true)
}

func (c *Cache) keepRevokedLocked(sessionID string, revoked bool) {
	now := c.now()
	// Drop stale answers as new ones arrive. A revoked one is dropped too: the
	// store still holds it, so the next request reads it from there.
	for id, entry := range c.revoked {
		if now.Sub(entry.loadedAt) >= c.ttl {
			delete(c.revoked, id)
		}
	}
	c.revoked[sessionID] = kept[bool]{value: revoked, loadedAt: now}
}
