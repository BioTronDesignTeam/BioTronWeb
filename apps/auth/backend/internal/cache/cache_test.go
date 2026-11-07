package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

// fakeSource counts loads and serves whatever the test last set.
type fakeSource struct {
	operator    store.OrgMember
	grants      []store.Grant
	loads       int
	guestKey    string
	beforeGrant func()
	revoked     map[string]bool
}

func (f *fakeSource) GetOperator(_ context.Context, id int64) (*store.OrgMember, error) {
	f.loads++
	if id != f.operator.GitHubID {
		return nil, store.ErrNotFound
	}
	op := f.operator
	return &op, nil
}

func (f *fakeSource) ListGrantsForOperator(context.Context, int64) ([]store.Grant, error) {
	f.loads++
	if f.beforeGrant != nil {
		f.beforeGrant()
	}
	return f.grants, nil
}

func (f *fakeSource) GetGuestKey(context.Context, string, string) (string, error) {
	f.loads++
	if f.guestKey == "" {
		return "", store.ErrNotFound
	}
	return f.guestKey, nil
}

func (f *fakeSource) IsSessionRevoked(_ context.Context, sessionID string) (bool, error) {
	f.loads++
	return f.revoked[sessionID], nil
}

func newTestCache(src Source) (*Cache, *time.Time) {
	now := time.Unix(1_000_000, 0)
	c := New(src)
	c.now = func() time.Time { return now }
	return c, &now
}

func TestOperatorIsKeptForTheTTLAndReloadedAfter(t *testing.T) {
	src := &fakeSource{operator: store.OrgMember{GitHubID: 7, Login: "op"}}
	c, now := newTestCache(src)
	ctx := context.Background()

	for range 3 {
		if _, err := c.Operator(ctx, 7); err != nil {
			t.Fatal(err)
		}
	}
	if src.loads != 1 {
		t.Fatalf("loads = %d, want 1 within the TTL", src.loads)
	}
	*now = now.Add(TTL)
	src.operator.IsBanned = true
	op, err := c.Operator(ctx, 7)
	if err != nil || !op.IsBanned || src.loads != 2 {
		t.Fatalf("after the TTL: op = %+v, err = %v, loads = %d", op, err, src.loads)
	}
}

func TestMissingOperatorIsNotKept(t *testing.T) {
	src := &fakeSource{}
	c, _ := newTestCache(src)
	for range 2 {
		if _, err := c.Operator(context.Background(), 9); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	}
	if src.loads != 2 {
		t.Fatalf("loads = %d, want every lookup of a missing operator to reach the store", src.loads)
	}
}

// A revoke must apply to the very next check, not after the TTL.
func TestInvalidateAppliesAtOnce(t *testing.T) {
	src := &fakeSource{grants: []store.Grant{{OperatorID: 7, AppID: "calendar", PermissionKey: "write"}}}
	c, _ := newTestCache(src)
	ctx := context.Background()

	if ok, _ := c.HasGrant(ctx, 7, "calendar", "write"); !ok {
		t.Fatal("expected the grant")
	}
	src.grants = nil
	c.Invalidate(7)
	if ok, _ := c.HasGrant(ctx, 7, "calendar", "write"); ok {
		t.Fatal("the revoked grant survived the invalidation")
	}
}

// A load that started before a revoke returns the old grants to its own
// request, but must not keep them for later ones.
func TestALoadThatRacedAnInvalidationIsNotKept(t *testing.T) {
	src := &fakeSource{grants: []store.Grant{{OperatorID: 7, AppID: "calendar", PermissionKey: "write"}}}
	c, _ := newTestCache(src)
	src.beforeGrant = func() {
		src.beforeGrant = nil
		c.Invalidate(7)
	}
	ctx := context.Background()
	if _, err := c.Grants(ctx, 7); err != nil {
		t.Fatal(err)
	}
	src.grants = nil
	if ok, _ := c.HasGrant(ctx, 7, "calendar", "write"); ok {
		t.Fatal("a stale load was kept after the invalidation it raced")
	}
}

// A guest key is kept for the TTL, and rotating it applies to the next guest
// request: InvalidateGuestKeys drops it and refuses a load that raced it.
func TestGuestKeyIsKeptUntilInvalidated(t *testing.T) {
	src := &fakeSource{guestKey: "OLD"}
	c, now := newTestCache(src)
	ctx := context.Background()

	for range 3 {
		if key, err := c.GuestKey(ctx, "exo-gui", "2026-10-23"); err != nil || key != "OLD" {
			t.Fatalf("key = %q, err = %v", key, err)
		}
	}
	if src.loads != 1 {
		t.Fatalf("loads = %d, want one within the TTL", src.loads)
	}

	src.guestKey = "NEW"
	c.InvalidateGuestKeys("exo-gui")
	if key, _ := c.GuestKey(ctx, "exo-gui", "2026-10-23"); key != "NEW" {
		t.Fatalf("after a rotation key = %q, want NEW", key)
	}

	// Turned off by hand in the database: seen once the TTL ends.
	src.guestKey = ""
	*now = now.Add(TTL)
	if _, err := c.GuestKey(ctx, "exo-gui", "2026-10-23"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after the TTL err = %v, want ErrNotFound", err)
	}
}

// A sign-out through this process applies to the very next request; one made
// in the store alone, such as in psql, within the TTL.
func TestRevokedSessionsApplyAtOnceAndStay(t *testing.T) {
	src := &fakeSource{revoked: map[string]bool{}}
	c, now := newTestCache(src)
	ctx := context.Background()

	if revoked, err := c.Revoked(ctx, "a"); err != nil || revoked {
		t.Fatalf("a fresh session: revoked = %v, err = %v", revoked, err)
	}
	c.MarkRevoked("a")
	if revoked, _ := c.Revoked(ctx, "a"); !revoked {
		t.Fatal("MarkRevoked did not apply to the next request")
	}

	// Revoked in the store only: seen once the kept answer is older than the TTL.
	if revoked, _ := c.Revoked(ctx, "b"); revoked {
		t.Fatal("b revoked before anyone revoked it")
	}
	src.revoked["b"] = true
	*now = now.Add(TTL)
	if revoked, _ := c.Revoked(ctx, "b"); !revoked {
		t.Fatal("a revocation in the store was not seen after the TTL")
	}

	// The kept revocation of a is swept, and the store still answers for it.
	src.revoked["a"] = true
	*now = now.Add(TTL)
	c.MarkRevoked("c")
	if revoked, _ := c.Revoked(ctx, "a"); !revoked {
		t.Fatal("a revocation was forgotten")
	}
}
