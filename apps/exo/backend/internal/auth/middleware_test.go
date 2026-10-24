package auth

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// silenceLog keeps the deliberate fail-closed log lines out of test output.
func silenceLog(t *testing.T) {
	t.Helper()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
}

// stubOAuthManager stands in for OAuthManager's GET /v1/check. It asserts the
// contract Exo depends on — app and permission arrive as query parameters and
// the caller's cookie is forwarded — and answers the way OAuthManager does.
func stubOAuthManager(t *testing.T, respond func(w http.ResponseWriter, permission string)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/check" {
			t.Errorf("path = %q, want /v1/check", r.URL.Path)
		}
		if got := r.URL.Query().Get("app"); got != AppID {
			t.Errorf("app = %q, want %q", got, AppID)
		}
		if r.Header.Get("Cookie") == "" {
			t.Error("the caller's session cookie was not forwarded")
		}
		respond(w, r.URL.Query().Get("permission"))
	}))
	t.Cleanup(server.Close)
	return server
}

// gatedApp mounts a single route behind Require, the way a real Exo route will.
func gatedApp(client *Client, permission string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/probe", client.Require(permission)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})))
	return mux
}

func request(t *testing.T, app http.Handler, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/probe", nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, req)
	return recorder
}

// No session at all. Exo must not spend a call on OAuthManager, and must say
// 401 so the frontend sends the operator to sign in.
func TestRequireRejectsMissingSession(t *testing.T) {
	called := false
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	app := gatedApp(NewClient(upstream.URL), PermissionCommands)

	if resp := request(t, app, ""); resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.Code)
	}
	if called {
		t.Fatal("OAuthManager was called for a request carrying no cookie")
	}
}

// A session OAuthManager does not recognise: expired, forged, or belonging to a
// banned operator. /v1/check sits behind RequireSession, so all of those are
// 401.
func TestRequireRejectsUnknownSession(t *testing.T) {
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	app := gatedApp(NewClient(upstream.URL), PermissionLive)

	if resp := request(t, app, "oauth_session=stale"); resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.Code)
	}
}

// The case the guest tier exists for: a valid session that simply does not hold
// the permission. This is 403, not 401 — the operator is signed in.
func TestRequireRejectsSessionWithoutPermission(t *testing.T) {
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		_, _ = io.WriteString(w, `{"allowed": false}`)
	})
	app := gatedApp(NewClient(upstream.URL), PermissionCommands)

	if resp := request(t, app, "oauth_session=guest"); resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.Code)
	}
}

// A session that holds the permission reaches the handler.
func TestRequireAllowsSessionWithPermission(t *testing.T) {
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		_, _ = io.WriteString(w, `{"allowed": true}`)
	})
	app := gatedApp(NewClient(upstream.URL), PermissionHistorical)

	if resp := request(t, app, "oauth_session=valid"); resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}
}

// AGENTS.md's invariant, expressed against the wire contract: the same guest
// key that opens Live and Historical must not open Commands.
func TestGuestKeyOpensLiveAndHistoricalButNotCommands(t *testing.T) {
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, permission string) {
		allowed := permission == PermissionLive || permission == PermissionHistorical
		if allowed {
			_, _ = io.WriteString(w, `{"allowed": true}`)
			return
		}
		_, _ = io.WriteString(w, `{"allowed": false}`)
	})
	client := NewClient(upstream.URL)

	for permission, want := range map[string]int{
		PermissionLive:       http.StatusOK,
		PermissionHistorical: http.StatusOK,
		PermissionCommands:   http.StatusForbidden,
	} {
		resp := request(t, gatedApp(client, permission), "oauth_session=daily-guest")
		if resp.Code != want {
			t.Errorf("%s: status = %d, want %d", permission, resp.Code, want)
		}
	}
}

// OAuthManager may one day put stricter middleware in front of /v1/check. A 403
// from it means authenticated-but-unpermitted, not "sign in again".
func TestRequireTreatsForbiddenAsUnpermitted(t *testing.T) {
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		w.WriteHeader(http.StatusForbidden)
	})
	app := gatedApp(NewClient(upstream.URL), PermissionCommands)

	if resp := request(t, app, "oauth_session=valid"); resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.Code)
	}
}

// An authorization service Exo cannot reach must fail closed.
func TestRequireFailsClosedWhenOAuthManagerIsUnreachable(t *testing.T) {
	silenceLog(t)

	// A port nothing is listening on: the transport error path.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	address := dead.URL
	dead.Close()

	app := gatedApp(NewClient(address), PermissionLive)
	if resp := request(t, app, "oauth_session=valid"); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.Code)
	}

	// And an unexpected status, which must not be read as an allow.
	broken := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	app = gatedApp(NewClient(broken.URL), PermissionLive)
	if resp := request(t, app, "oauth_session=valid"); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.Code)
	}
}

// An unset OAUTH_MANAGER_URL must never behave like an allow-all.
func TestRequireFailsClosedWhenUnconfigured(t *testing.T) {
	silenceLog(t)

	app := gatedApp(NewClient(""), PermissionLive)
	if resp := request(t, app, "oauth_session=valid"); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.Code)
	}
}

// The permission key must reach OAuthManager verbatim; a gate that always asked
// about `live` would silently open Commands to guests.
func TestCheckSendsTheRequestedPermission(t *testing.T) {
	var seen []string
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, permission string) {
		seen = append(seen, permission)
		_, _ = io.WriteString(w, `{"allowed": true}`)
	})
	client := NewClient(upstream.URL + "/")

	for _, permission := range []string{PermissionLive, PermissionHistorical, PermissionCommands} {
		if _, err := client.Check(context.Background(), "oauth_session=valid", permission); err != nil {
			t.Fatalf("check %q: %v", permission, err)
		}
	}
	want := []string{PermissionLive, PermissionHistorical, PermissionCommands}
	for i, permission := range want {
		if i >= len(seen) || seen[i] != permission {
			t.Fatalf("permissions sent = %v, want %v", seen, want)
		}
	}
}

// The frontend reads these bodies, so the move off Fiber must not change a
// byte of them: compact JSON, no trailing newline, charset in the type.
func TestRequireRefusalBodies(t *testing.T) {
	silenceLog(t)
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		_, _ = io.WriteString(w, `{"allowed": false}`)
	})

	for _, tc := range []struct {
		name   string
		client *Client
		cookie string
		status int
		body   string
	}{
		{"no session", NewClient(upstream.URL), "", http.StatusUnauthorized, `{"error":"authentication required"}`},
		{"no permission", NewClient(upstream.URL), "oauth_session=guest", http.StatusForbidden, `{"error":"permission required","permission":"commands"}`},
		{"unconfigured", NewClient(""), "oauth_session=valid", http.StatusServiceUnavailable, `{"error":"authorization service unavailable"}`},
	} {
		resp := request(t, gatedApp(tc.client, PermissionCommands), tc.cookie)
		body, _ := io.ReadAll(resp.Body)
		if resp.Code != tc.status || string(body) != tc.body {
			t.Errorf("%s: %d %s, want %d %s", tc.name, resp.Code, body, tc.status, tc.body)
		}
		if got := resp.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", tc.name, got)
		}
	}
}

// A handler behind the gate reads the decision Require made instead of asking
// OAuthManager again.
func TestDecisionFromReachesTheHandler(t *testing.T) {
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		_, _ = io.WriteString(w, `{"allowed": true}`)
	})
	var got Decision
	var ok bool
	handler := NewClient(upstream.URL).Require(PermissionLive)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, ok = DecisionFrom(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cookie", "oauth_session=valid")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !ok || got != (Decision{Authenticated: true, Allowed: true}) {
		t.Fatalf("DecisionFrom = %+v, %v", got, ok)
	}
	if _, ok := DecisionFrom(httptest.NewRequest(http.MethodGet, "/", nil)); ok {
		t.Fatal("DecisionFrom found a decision on a request that never passed the gate")
	}
}
