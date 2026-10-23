// Package cache keeps each operator's standing and grants in memory for a
// short while, so the permission check that every tool makes on every page
// does not reach Postgres each time.
//
// One process serves Auth, so the cache is a map, not Redis. Every change
// Auth makes to an operator (a grant, a revoke, a ban, the manager flag)
// invalidates that operator at once, so the change applies to the very next
// request. The TTL only bounds how long a change made outside this process,
// such as by hand in psql, can go unseen.
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// TTL is the longest an operator's standing or grants are kept.
const TTL = 30 * time.Second

// Source is where the cache loads from: the store.
type Source interface {
	GetOperator(ctx context.Context, githubID int64) (*store.OrgMember, error)
	ListGrantsForOperator(ctx context.Context, operatorID int64) ([]store.Grant, error)
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
}

func New(src Source) *Cache {
	return &Cache{
		src:        src,
		ttl:        TTL,
		now:        time.Now,
		operators:  map[int64]kept[store.OrgMember]{},
		grants:     map[int64]kept[[]store.Grant]{},
		generation: map[int64]uint64{},
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
