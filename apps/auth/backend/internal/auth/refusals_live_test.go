package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
)

const liveSuperuser int64 = 9_000_000_003

// A Manager can never ban, unban, or sign out a Superuser: a banned Superuser
// could only be let back in by another Superuser or by hand in the database.
func TestLiveOnlyASuperuserCanBanASuperuser(t *testing.T) {
	h, st := liveHandler(t)
	ctx := context.Background()
	if err := st.UpsertOperator(ctx, store.Operator{GitHubID: liveSuperuser, Login: "live-superuser"}, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.SetBanned(ctx, liveSuperuser, false) })
	h.Cfg.IsSuperuserID = func(id int64) bool { return id == liveSuperuser }
	act := func(handler http.HandlerFunc, actor *store.SessionOperator) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/org/members/x", nil)
		request.SetPathValue("id", "9000000003")
		response := httptest.NewRecorder()
		handler(response, withOperator(request, actor))
		return response
	}
	manager := &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true}
	for name, tc := range map[string]struct {
		handler http.HandlerFunc
		body    string
	}{
		"ban":      {h.BanMember, `{"error":"Only a superuser can ban or unban a superuser."}`},
		"unban":    {h.UnbanMember, `{"error":"Only a superuser can ban or unban a superuser."}`},
		"sign out": {h.SignOutMember, `{"error":"Only a superuser can sign out a superuser."}`},
	} {
		if response := act(tc.handler, manager); response.Code != http.StatusForbidden || response.Body.String() != tc.body {
			t.Errorf("a Manager's %s of a Superuser = %d %s, want 403 %s", name, response.Code, response.Body, tc.body)
		}
	}
	if target, err := st.GetOperator(ctx, liveSuperuser); err != nil || target.IsBanned {
		t.Fatalf("after a Manager's ban: %+v, %v; want not banned", target, err)
	}

	superuser := &store.SessionOperator{GitHubID: 8, Login: "other-superuser", IsSuperuser: true}
	if response := act(h.BanMember, superuser); response.Code != http.StatusNoContent {
		t.Fatalf("a Superuser's ban of a Superuser = %d %s, want 204", response.Code, response.Body)
	}
	if target, err := st.GetOperator(ctx, liveSuperuser); err != nil || !target.IsBanned {
		t.Fatalf("after a Superuser's ban: %+v, %v; want banned", target, err)
	}
}

// The callback is the one place that refuses to mint a session for a banned
// operator.
func TestLiveBannedOperatorCannotSignIn(t *testing.T) {
	h, st := liveHandler(t)
	events, received := logclienttest.New(t, "oauth-manager")
	h.Events = events
	ctx := context.Background()
	if err := st.UpsertOperator(ctx, store.Operator{GitHubID: liveOperator, Login: "octo"}, false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBanned(ctx, liveOperator, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.SetBanned(ctx, liveOperator, false) })

	login := httptest.NewRecorder()
	h.LoginRedirect(login, httptest.NewRequest(http.MethodGet, "/auth/github/login?redirect=http://localhost:5174/x", nil))
	authorize, err := url.Parse(login.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRequest(http.MethodGet,
		"/auth/github/callback?code=good&state="+url.QueryEscape(authorize.Query().Get("state")), nil)
	callback.AddCookie(cookieFrom(t, login, stateCookie))
	response := httptest.NewRecorder()
	h.Callback(response, callback)

	if response.Code != http.StatusFound || response.Header().Get("Location") != "http://localhost:5174/?auth=banned" {
		t.Fatalf("callback = %d to %q, want 302 to ?auth=banned", response.Code, response.Header().Get("Location"))
	}
	stateCleared := false
	for _, c := range response.Result().Cookies() {
		if c.Name == SessionCookie && c.Value != "" {
			t.Fatal("a banned operator got a session cookie")
		}
		if c.Name == stateCookie && c.Value == "" && c.Expires.Before(time.Now()) {
			stateCleared = true
		}
	}
	if !stateCleared {
		t.Fatal("the state cookie was not cleared")
	}
	timeout := time.After(3 * time.Second)
	for {
		select {
		case e := <-received:
			if e.Message == "Sign-in refused" {
				if e.Payload["reason"] != "banned" || e.Payload["login"] != "octo" {
					t.Fatalf("payload = %v, want reason banned and the login", e.Payload)
				}
				return
			}
		case <-timeout:
			t.Fatal("no Sign-in refused event")
		}
	}
}

// A wrong key, a key of another tool, and an empty key never make a guest
// session; the right key in any case and spacing does.
func TestLiveGuestLoginRefusesWrongKeyAndOtherTools(t *testing.T) {
	h, st := liveHandler(t)
	key, err := st.EnsureProductDailyKey(context.Background(), "exo-gui", easternDay(time.Now()), "LIVEGUESTKEY")
	if err != nil {
		t.Fatal(err)
	}
	right := " " + strings.ToLower(formatGuestKey(key.Key)) + " "
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"a wrong key", `{"app_id":"exo-gui","key":"AAAA-BBBB-CCCC"}`, http.StatusUnauthorized},
		{"Exo's key for a tool without daily keys", `{"app_id":"calendar","key":"` + formatGuestKey(key.Key) + `"}`, http.StatusUnauthorized},
		{"an empty key", `{"app_id":"exo-gui","key":""}`, http.StatusBadRequest},
		{"the right key in lower case with spaces", `{"app_id":"exo-gui","key":"` + right + `"}`, http.StatusNoContent},
	} {
		request := httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		h.GuestLogin(response, request)
		if response.Code != tc.want {
			t.Errorf("%s = %d %s, want %d", tc.name, response.Code, response.Body, tc.want)
		}
		session := ""
		for _, c := range response.Result().Cookies() {
			if c.Name == SessionCookie {
				session = c.Value
			}
		}
		if (session != "") != (tc.want == http.StatusNoContent) {
			t.Errorf("%s: session cookie %q", tc.name, session)
		}
		if tc.want == http.StatusUnauthorized && !strings.Contains(response.Body.String(), "invalid or expired key") {
			t.Errorf("%s: body %s", tc.name, response.Body)
		}
	}
}
