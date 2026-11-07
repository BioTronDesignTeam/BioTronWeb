package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/auth"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/cache"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

const (
	memberID    int64 = 11
	managerID   int64 = 12
	superuserID int64 = 13
)

// rolesStore knows a Member, a Manager, and an operator whom only
// SUPERUSER_GITHUB_IDS makes a Superuser, plus the guest account.
type rolesStore struct{ guestStore }

func (rolesStore) GetOperator(ctx context.Context, githubID int64) (*store.OrgMember, error) {
	switch githubID {
	case memberID:
		return &store.OrgMember{GitHubID: githubID, Login: "member"}, nil
	case managerID:
		return &store.OrgMember{GitHubID: githubID, Login: "manager", IsManager: true}, nil
	case superuserID:
		return &store.OrgMember{GitHubID: githubID, Login: "superuser"}, nil
	default:
		return guestStore{}.GetOperator(ctx, githubID)
	}
}

// Every role gate of the real router, with a real signed-in session of each
// role: dropping or reordering one middleware in chain() fails here.
func TestSignedInRolesHitTheRightGates(t *testing.T) {
	signer := auth.NewSigner([]byte("test-signing-key-that-is-at-least-32-bytes"))
	h := &auth.Handler{
		Cache:   cache.New(rolesStore{}),
		Members: auth.NewMembership(func(context.Context, int64) (bool, error) { return true, nil }),
		Signer:  signer,
		Cfg: auth.Config{
			FrontendURL:   ownOrigin,
			IsSuperuserID: func(id int64) bool { return id == superuserID },
		},
	}
	app := New(h, []string{ownOrigin}, nil, nil)
	cookie := func(session auth.Session) *http.Cookie {
		t.Helper()
		value, err := signer.Session(session, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: auth.SessionCookie, Value: value}
	}
	const day = "2026-10-23"
	guest := cookie(auth.Session{
		GitHubID: store.GuestGitHubID, GuestAppID: "exo-gui", GuestDay: day,
		GuestKeyID: signer.GuestKeyID("exo-gui", day, guestKey),
	})
	call := func(c *http.Cookie, method, target, body string) int {
		t.Helper()
		request := httptest.NewRequest(method, target, strings.NewReader(body))
		request.AddCookie(c)
		request.Header.Set("X-Requested-With", "XMLHttpRequest")
		request.Header.Set("Content-Type", "application/json")
		return serve(app, request).Code
	}

	staffOnly := []struct{ method, target string }{
		{http.MethodGet, "/org/members"},
		{http.MethodGet, "/org/members/1/grants"},
		{http.MethodPost, "/org/members/1/ban"},
		{http.MethodPost, "/org/members/1/unban"},
		{http.MethodPost, "/org/members/1/sign-out"},
		{http.MethodPost, "/grants"},
		{http.MethodDelete, "/grants"},
		{http.MethodGet, "/auth/guest-keys"},
		{http.MethodPost, "/auth/guest-keys/exo-gui/rotate"},
	}
	superuserOnly := []struct{ method, target string }{
		{http.MethodPost, "/apps"},
		{http.MethodPatch, "/org/members/1/manager"},
	}

	member := cookie(auth.Session{GitHubID: memberID})
	for _, route := range append(staffOnly, superuserOnly...) {
		if got := call(member, route.method, route.target, ""); got != http.StatusForbidden {
			t.Errorf("Member %s %s = %d, want 403", route.method, route.target, got)
		}
	}
	for _, route := range append(append(staffOnly, superuserOnly...),
		struct{ method, target string }{http.MethodPost, "/auth/logout/everywhere"},
		struct{ method, target string }{http.MethodGet, "/me/grants"},
	) {
		if got := call(guest, route.method, route.target, ""); got != http.StatusForbidden {
			t.Errorf("Guest %s %s = %d, want 403", route.method, route.target, got)
		}
	}

	manager := cookie(auth.Session{GitHubID: managerID})
	for _, route := range superuserOnly {
		if got := call(manager, route.method, route.target, ""); got != http.StatusForbidden {
			t.Errorf("Manager %s %s = %d, want 403", route.method, route.target, got)
		}
	}
	// Past the gates, a malformed body stops at 400 before any store call.
	if got := call(manager, http.MethodPost, "/grants", "{"); got != http.StatusBadRequest {
		t.Errorf("Manager POST /grants = %d, want the 400 past the gate", got)
	}

	superuser := cookie(auth.Session{GitHubID: superuserID})
	if got := call(superuser, http.MethodPatch, "/org/members/1/manager", "{"); got != http.StatusBadRequest {
		t.Errorf("Superuser PATCH /org/members/1/manager = %d, want the 400 past the gate", got)
	}
	if got := call(superuser, http.MethodPost, "/apps", "{"); got != http.StatusBadRequest {
		t.Errorf("Superuser POST /apps = %d, want the 400 past the gate", got)
	}
}
