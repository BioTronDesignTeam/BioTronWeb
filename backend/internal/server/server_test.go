package server

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

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/store"
)

const testOrg = "BioTronDesignTeam"

// fakeGitHub stands in for GitHub's OAuth + API endpoints so the full login
// flow can be exercised without real credentials.
func fakeGitHub(t *testing.T, member bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"faketoken","token_type":"bearer","scope":"read:org"}`))
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer faketoken" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":4242,"login":"octo-eng","name":"Octo Eng","avatar_url":"https://example.com/a.png"}`))
	})
	mux.HandleFunc("/user/memberships/orgs/"+testOrg, func(w http.ResponseWriter, _ *http.Request) {
		if !member {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"active","role":"member"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testApp(t *testing.T, member bool) *fiber.App {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping DB-backed auth test")
	}
	st, err := store.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(st.Close)

	gh := fakeGitHub(t, member)
	client := auth.NewGitHubClient(auth.GitHubOptions{
		ClientID:     "cid",
		ClientSecret: "secret",
		CallbackURL:  "http://localhost:8080/auth/github/callback",
		Org:          testOrg,
		APIBase:      gh.URL,
		AuthURL:      gh.URL + "/login/oauth/authorize",
		TokenURL:     gh.URL + "/login/oauth/access_token",
	})
	h := &auth.Handler{
		Store:  st,
		GitHub: client,
		Cfg: auth.Config{
			FrontendURL:    "http://localhost:5173",
			CookieSecure:   false,
			CookieSameSite: "Lax",
			SessionTTL:     time.Hour,
		},
	}
	return New(h, "http://localhost:5173")
}

func cookie(resp *http.Response, name string) *http.Cookie {
	for _, ck := range resp.Cookies() {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}

func TestOperatorLoginFlow(t *testing.T) {
	app := testApp(t, true)

	// 1) login -> 302 + state cookie + authorize URL with our params
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/auth/github/login", nil), -1)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login status = %d, want 302", resp.StatusCode)
	}
	state := cookie(resp, "exo_oauth_state")
	if state == nil || state.Value == "" {
		t.Fatal("login did not set a state cookie")
	}
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "client_id=cid") ||
		!strings.Contains(loc, url.QueryEscape("read:org")) {
		t.Fatalf("authorize URL missing params: %s", loc)
	}

	// 2) callback -> 302 to the SPA + session cookie
	cb := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=abc&state="+url.QueryEscape(state.Value), nil)
	cb.AddCookie(state)
	cbResp, err := app.Test(cb, -1)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if cbResp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", cbResp.StatusCode)
	}
	if loc := cbResp.Header.Get("Location"); loc != "http://localhost:5173" {
		t.Fatalf("callback redirect = %q, want the SPA", loc)
	}
	sess := cookie(cbResp, "exo_session")
	if sess == nil || sess.Value == "" {
		t.Fatal("callback did not set a session cookie")
	}

	// 3) /auth/me with the session -> 200 + operator
	me := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	me.AddCookie(sess)
	meResp, err := app.Test(me, -1)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want 200", meResp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(meResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if body["login"] != "octo-eng" {
		t.Fatalf("me login = %v, want octo-eng", body["login"])
	}

	// 4) /auth/me without a session -> 401
	naResp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/auth/me", nil), -1)
	if naResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me without cookie = %d, want 401", naResp.StatusCode)
	}

	// 5) logout -> 204, and the session is no longer valid
	lo := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	lo.AddCookie(sess)
	loResp, _ := app.Test(lo, -1)
	if loResp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", loResp.StatusCode)
	}
	me2 := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	me2.AddCookie(sess)
	me2Resp, _ := app.Test(me2, -1)
	if me2Resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d, want 401", me2Resp.StatusCode)
	}
}

func TestNonMemberDenied(t *testing.T) {
	app := testApp(t, false)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/auth/github/login", nil), -1)
	state := cookie(resp, "exo_oauth_state")
	if state == nil {
		t.Fatal("no state cookie")
	}

	cb := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=abc&state="+url.QueryEscape(state.Value), nil)
	cb.AddCookie(state)
	cbResp, _ := app.Test(cb, -1)
	if cbResp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", cbResp.StatusCode)
	}
	if loc := cbResp.Header.Get("Location"); !strings.Contains(loc, "auth=denied") {
		t.Fatalf("non-member redirect = %q, want auth=denied", loc)
	}
	if sess := cookie(cbResp, "exo_session"); sess != nil && sess.Value != "" {
		t.Fatal("non-member should not get a session cookie")
	}
}
