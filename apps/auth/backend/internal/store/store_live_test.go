package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// These run every query against a real PostgreSQL, because the SQL is the
// part no compiler checks. Set AUTH_TEST_DATABASE_URL to a throwaway database
// with the Prisma migrations applied; without it the suite skips. The address
// looks like this:
//
//	postgresql://user:pass@host:5432/db?schema=oauth (a placeholder; trufflehog:ignore)
//
// The tests use operator ids far above any real GitHub id and remove them
// again, so the seeded apps and permissions are the only shared rows.

const (
	liveMember  int64 = 9_000_000_001
	liveManager int64 = 9_000_000_002
)

func liveStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	st, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	cleanup := func() {
		for _, id := range []int64{liveMember, liveManager} {
			_, _ = st.pool.Exec(ctx, `DELETE FROM operators WHERE github_id = $1`, id)
		}
		_, _ = st.pool.Exec(ctx, `DELETE FROM apps WHERE id = 'live-test'`)
	}
	cleanup()
	t.Cleanup(func() {
		cleanup()
		st.Close()
	})
	return st, ctx
}

func TestLiveOperatorsAndOrgMembers(t *testing.T) {
	st, ctx := liveStore(t)

	if _, err := st.GetOperator(ctx, liveMember); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing operator: err = %v, want ErrNotFound", err)
	}
	if err := st.UpsertOperator(ctx, Operator{GitHubID: liveMember, Login: "live-member"}, true); err != nil {
		t.Fatal(err)
	}
	// A second sign-in refreshes the profile and clears a superuser flag that
	// SUPERUSER_GITHUB_IDS no longer lists.
	if err := st.UpsertOperator(ctx, Operator{
		GitHubID: liveMember, Login: "live-member-2", Name: "Live", AvatarURL: "https://a.example/x.png",
	}, false); err != nil {
		t.Fatal(err)
	}
	m, err := st.GetOperator(ctx, liveMember)
	if err != nil {
		t.Fatal(err)
	}
	if m.Login != "live-member-2" || m.Name != "Live" || m.AvatarURL != "https://a.example/x.png" ||
		m.IsSuperuser || m.IsManager || m.IsBanned || m.LastLoginAt.IsZero() {
		t.Fatalf("operator = %+v", m)
	}

	// An empty name is stored as NULL and read back as "".
	if err := st.UpsertOperator(ctx, Operator{GitHubID: liveManager, Login: "live-manager"}, false); err != nil {
		t.Fatal(err)
	}
	var name *string
	if err := st.pool.QueryRow(ctx, `SELECT name FROM operators WHERE github_id = $1`, liveManager).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != nil {
		t.Fatalf("empty name stored as %q, want NULL", *name)
	}

	if err := st.SetManager(ctx, liveManager, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBanned(ctx, liveMember, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBanned(ctx, 9_000_000_999, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ban of a missing operator: err = %v, want ErrNotFound", err)
	}
	if err := st.SetManager(ctx, 9_000_000_999, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("manager flag of a missing operator: err = %v, want ErrNotFound", err)
	}

	members, err := st.ListOrgMembers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]OrgMember{}
	for i, member := range members {
		if member.GitHubID == GuestGitHubID {
			t.Fatal("the guest operator must not be listed")
		}
		if i > 0 && members[i-1].Login > member.Login {
			t.Fatal("members must be ordered by login")
		}
		seen[member.GitHubID] = member
	}
	if !seen[liveMember].IsBanned || !seen[liveManager].IsManager {
		t.Fatalf("flags not read back: %+v", seen)
	}
}

func TestLiveCatalogAndGrants(t *testing.T) {
	st, ctx := liveStore(t)
	if err := st.UpsertOperator(ctx, Operator{GitHubID: liveMember, Login: "live-member"}, false); err != nil {
		t.Fatal(err)
	}

	apps, err := st.ListApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]App{}
	for _, app := range apps {
		byID[app.ID] = app
	}
	if !byID["exo-gui"].DailyKeyEnabled || byID["logger"].DailyKeyEnabled {
		t.Fatalf("seeded apps = %+v", byID)
	}
	if err := st.CreateApp(ctx, "live-test", "Live test", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateApp(ctx, "live-test", "Live test", ""); err == nil {
		t.Fatal("a duplicate app must be refused")
	}

	perms, err := st.ListPermissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(perms); i++ {
		a, b := perms[i-1], perms[i]
		if a.AppID > b.AppID || (a.AppID == b.AppID && a.Key > b.Key) {
			t.Fatal("permissions must be ordered by app, then key")
		}
	}
	if p, err := st.GetPermission(ctx, "logger", "view"); err != nil || p.Label != "View" {
		t.Fatalf("logger/view = %+v, %v", p, err)
	}
	if _, err := st.GetPermission(ctx, "logger", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing permission: err = %v, want ErrNotFound", err)
	}

	if err := st.CreateGrant(ctx, liveMember, "logger", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grant of a missing permission: err = %v, want ErrNotFound", err)
	}
	for _, key := range []string{"live", "historical", "live"} {
		if err := st.CreateGrant(ctx, liveMember, "exo-gui", key); err != nil {
			t.Fatalf("grant %s: %v", key, err)
		}
	}
	grants, err := st.ListGrantsForOperator(ctx, liveMember)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 2 || grants[0].PermissionKey != "historical" || grants[1].PermissionKey != "live" ||
		grants[0].CreatedAt.IsZero() {
		t.Fatalf("grants = %+v", grants)
	}

	if err := st.DeleteGrant(ctx, liveMember, "exo-gui", "live"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteGrant(ctx, liveMember, "exo-gui", "live"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke: err = %v, want ErrNotFound", err)
	}
	if err := st.DeleteAllGrants(ctx, liveMember); err != nil {
		t.Fatal(err)
	}
	if grants, err := st.ListGrantsForOperator(ctx, liveMember); err != nil || len(grants) != 0 {
		t.Fatalf("grants after clearing = %+v, %v", grants, err)
	}
}

func TestLiveProductDailyKeys(t *testing.T) {
	st, ctx := liveStore(t)

	apps, err := st.ListDailyKeyApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].ID != "exo-gui" {
		t.Fatalf("daily-key apps = %+v", apps)
	}

	day := time.Now().Format("2006-01-02")
	first, err := st.EnsureProductDailyKey(ctx, "exo-gui", day, "LIVETESTKEY1")
	if err != nil {
		t.Fatal(err)
	}
	// The first key of the day wins; a later candidate never replaces it.
	second, err := st.EnsureProductDailyKey(ctx, "exo-gui", day, "LIVETESTKEY2")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.AppID != "exo-gui" || first.Day != day {
		t.Fatalf("keys = %+v then %+v", first, second)
	}
	if _, err := st.EnsureProductDailyKey(ctx, "logger", day, "LIVETESTKEY3"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("app without daily keys: err = %v, want ErrNotFound", err)
	}

	if _, err := st.EnsureProductDailyKey(ctx, "exo-gui", "2000-01-01", "LIVETESTKEY4"); err != nil {
		t.Fatal(err)
	}
	if n, err := st.DeleteOldProductDailyKeys(ctx); err != nil || n < 1 {
		t.Fatalf("prune: n = %d, err = %v", n, err)
	}
	var left int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM guest_keys WHERE day = $1::date`, day).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("today's key was pruned: %d left", left)
	}
}
