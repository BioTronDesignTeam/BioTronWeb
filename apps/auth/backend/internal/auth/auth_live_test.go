package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/cache"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// This walks one operator through a whole sign-in against a real PostgreSQL
// and a stub GitHub, then through a grant, a revoke, and a ban, each of which
// must apply to the very next request. Set AUTH_TEST_DATABASE_URL to a
// throwaway database with the Prisma migrations applied; without it the test
// skips. The stub GitHub's user is id 42, which the test removes again.

const liveOperator int64 = 42

func liveHandler(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not set")
	}
	st, err := store.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(st.Close)
	github := newStubGitHub(t, http.StatusNoContent).client()
	return &Handler{
		Store:   st,
		Cache:   cache.New(st),
		GitHub:  github,
		Members: NewMembership(github.IsOrgMember),
		Signer:  NewSigner(testSigningKey),
		Cfg: Config{
			FrontendURL:    "http://localhost:5173",
			AllowedOrigins: []string{"http://localhost:5173", "http://localhost:5174"},
			CookieSameSite: "Lax",
			SessionTTL:     time.Hour,
		},
	}, st
}

func cookieFrom(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range response.Result().Cookies() {
		if c.Name == name && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", name, response.Header()["Set-Cookie"])
	return nil
}

func TestLiveSignInThenGrantRevokeAndBan(t *testing.T) {
	h, st := liveHandler(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = st.DeleteAllGrants(ctx, liveOperator) })

	// 1. Login: a signed state cookie and a redirect to GitHub.
	login := httptest.NewRecorder()
	h.LoginRedirect(login, httptest.NewRequest(http.MethodGet, "/auth/github/login?redirect=http://localhost:5174/x", nil))
	if login.Code != http.StatusFound {
		t.Fatalf("login = %d", login.Code)
	}
	authorize, err := url.Parse(login.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	stateCookieValue := cookieFrom(t, login, stateCookie)

	// 2. Callback: the session cookie, and back to the origin asked for.
	callback := httptest.NewRequest(http.MethodGet,
		"/auth/github/callback?code=good&state="+url.QueryEscape(authorize.Query().Get("state")), nil)
	callback.AddCookie(stateCookieValue)
	signedIn := httptest.NewRecorder()
	h.Callback(signedIn, callback)
	if signedIn.Code != http.StatusFound || signedIn.Header().Get("Location") != "http://localhost:5174" {
		t.Fatalf("callback = %d to %q: %s", signedIn.Code, signedIn.Header().Get("Location"), signedIn.Body)
	}
	session := cookieFrom(t, signedIn, SessionCookie)
	if !session.HttpOnly || session.Path != "/" || session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie attributes = %+v", session)
	}

	get := func(handler http.HandlerFunc, target string, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		h.RequireSession(handler).ServeHTTP(response, request)
		return response
	}
	allowed := func() bool {
		t.Helper()
		response := get(h.Check, "/v1/check?app=calendar&permission=write", session)
		if response.Code != http.StatusOK {
			t.Fatalf("check = %d", response.Code)
		}
		var body struct {
			Allowed bool `json:"allowed"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.Allowed
	}

	me := get(h.Me, "/auth/me", session)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"login":"octo"`) {
		t.Fatalf("me = %d %s", me.Code, me.Body)
	}
	if allowed() {
		t.Fatal("a new operator holds calendar/write")
	}

	// 3. A grant and a revoke apply to the next check, with the cache warm.
	staff := &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true}
	change := func(handler http.HandlerFunc, method, target, body string) {
		t.Helper()
		request := withOperator(httptest.NewRequest(method, target, strings.NewReader(body)), staff)
		request.Header.Set("Content-Type", "application/json")
		if strings.Contains(target, "/members/") {
			request.SetPathValue("id", "42")
		}
		response := httptest.NewRecorder()
		handler(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s %s = %d %s", method, target, response.Code, response.Body)
		}
	}
	grant := `{"operator_id":42,"app_id":"calendar","permission_key":"write"}`
	change(h.CreateGrant, http.MethodPost, "/grants", grant)
	if !allowed() {
		t.Fatal("the grant did not apply to the next check")
	}
	change(h.DeleteGrant, http.MethodDelete, "/grants", grant)
	if allowed() {
		t.Fatal("the revoke did not apply to the next check")
	}

	// 4. A ban refuses the very next request; an unban lets the cookie in again.
	change(h.BanMember, http.MethodPost, "/org/members/42/ban", "")
	if response := get(h.Me, "/auth/me", session); response.Code != http.StatusUnauthorized {
		t.Fatalf("me after a ban = %d, want 401", response.Code)
	}
	change(h.UnbanMember, http.MethodPost, "/org/members/42/unban", "")
	if response := get(h.Me, "/auth/me", session); response.Code != http.StatusOK {
		t.Fatalf("me after an unban = %d, want 200", response.Code)
	}

	// 5. A session cookie from before the move to signed cookies is refused.
	if response := get(h.Me, "/auth/me", &http.Cookie{Name: SessionCookie, Value: "opaque-old-session-token"}); response.Code != http.StatusUnauthorized {
		t.Fatalf("old session cookie = %d, want 401", response.Code)
	}
}

func TestLiveGuestSessionIsScopedToItsTool(t *testing.T) {
	h, st := liveHandler(t)
	ctx := context.Background()
	key, err := st.EnsureProductDailyKey(ctx, "exo-gui", easternDay(time.Now()), "LIVEGUESTKEY")
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/auth/guest",
		strings.NewReader(`{"app_id":"exo-gui","key":"`+strings.ToLower(formatGuestKey(key.Key))+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.GuestLogin(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("guest login = %d %s", response.Code, response.Body)
	}
	session := cookieFrom(t, response, SessionCookie)
	if until := time.Until(session.Expires); until <= 0 || until > 25*time.Hour {
		t.Fatalf("guest session expires in %s, want by the next Eastern midnight", until)
	}

	for target, want := range map[string]int{
		"/auth/me?app=exo-gui":  http.StatusOK,
		"/auth/me?app=calendar": http.StatusForbidden,
	} {
		me := httptest.NewRequest(http.MethodGet, target, nil)
		me.AddCookie(session)
		got := httptest.NewRecorder()
		h.RequireSession(http.HandlerFunc(h.Me)).ServeHTTP(got, me)
		if got.Code != want {
			t.Errorf("GET %s = %d, want %d", target, got.Code, want)
		}
	}

	// Rotating today's key ends the session at its next request.
	rotate := httptest.NewRequest(http.MethodPost, "/auth/guest-keys/exo-gui/rotate", nil)
	rotate.SetPathValue("app", "exo-gui")
	rotated := httptest.NewRecorder()
	h.RotateProductDailyKey(rotated, withOperator(rotate, &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true}))
	if rotated.Code != http.StatusOK || strings.Contains(rotated.Body.String(), formatGuestKey(key.Key)) {
		t.Fatalf("rotate = %d %s", rotated.Code, rotated.Body)
	}
	me := httptest.NewRequest(http.MethodGet, "/auth/me?app=exo-gui", nil)
	me.AddCookie(session)
	after := httptest.NewRecorder()
	h.RequireSession(http.HandlerFunc(h.Me)).ServeHTTP(after, me)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("a guest after a rotation = %d, want 401", after.Code)
	}
}
