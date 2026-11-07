package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/cache"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

// Sign-out ends the session on the server, not only in the browser: a copy of
// the cookie is refused from the very next request. Sign out everywhere, and
// a manager's sign-out of someone, end every session signed before it.
func TestLiveSignOutRevokesSessions(t *testing.T) {
	h, st := liveHandler(t)
	ctx := context.Background()
	if err := st.UpsertOperator(ctx, store.Operator{GitHubID: liveOperator, Login: "octo"}, false); err != nil {
		t.Fatal(err)
	}
	h.invalidate(liveOperator)
	cookie := func() *http.Cookie {
		t.Helper()
		value, err := h.Signer.Session(Session{GitHubID: liveOperator}, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: SessionCookie, Value: value}
	}
	serve := func(handler http.HandlerFunc, method string, c *http.Cookie) int {
		t.Helper()
		request := httptest.NewRequest(method, "/x", nil)
		request.AddCookie(c)
		response := httptest.NewRecorder()
		h.RequireSession(handler).ServeHTTP(response, request)
		return response.Code
	}
	me := func(c *http.Cookie) int { return serve(h.Me, http.MethodGet, c) }

	// Two devices. Signing out on one leaves the other signed in, and the
	// copy of the first cookie is refused at once.
	laptop, phone := cookie(), cookie()
	if got := serve(h.Logout, http.MethodPost, laptop); got != http.StatusNoContent {
		t.Fatalf("sign out = %d", got)
	}
	if got := me(laptop); got != http.StatusUnauthorized {
		t.Fatalf("a signed-out cookie = %d, want 401", got)
	}
	if got := me(phone); got != http.StatusOK {
		t.Fatalf("the other device after a sign-out = %d, want 200", got)
	}
	// The revocation is in the store, not only in this process's cache.
	h.Cache = cache.New(st)
	if got := me(laptop); got != http.StatusUnauthorized {
		t.Fatalf("a signed-out cookie after a restart = %d, want 401", got)
	}

	// Sign out everywhere ends the phone too, and a later sign-in works.
	tablet := cookie()
	if got := serve(h.LogoutEverywhere, http.MethodPost, phone); got != http.StatusNoContent {
		t.Fatalf("sign out everywhere = %d", got)
	}
	for name, c := range map[string]*http.Cookie{"phone": phone, "tablet": tablet} {
		if got := me(c); got != http.StatusUnauthorized {
			t.Errorf("%s after sign out everywhere = %d, want 401", name, got)
		}
	}
	later := cookie()
	if got := me(later); got != http.StatusOK {
		t.Fatalf("a sign-in after sign out everywhere = %d, want 200", got)
	}

	// A manager signs the operator out everywhere.
	request := httptest.NewRequest(http.MethodPost, "/org/members/42/sign-out", nil)
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()
	h.SignOutMember(response, withOperator(request, &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true}))
	if response.Code != http.StatusNoContent {
		t.Fatalf("a manager's sign-out = %d %s", response.Code, response.Body)
	}
	if got := me(later); got != http.StatusUnauthorized {
		t.Fatalf("after a manager's sign-out = %d, want 401", got)
	}
}
