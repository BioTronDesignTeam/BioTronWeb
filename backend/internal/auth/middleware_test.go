package auth

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
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
func gatedApp(client *Client, permission string) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/v1/probe", client.Require(permission), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})
	return app
}

func request(t *testing.T, app *fiber.App, cookie string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/probe", nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
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

	if resp := request(t, app, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
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

	if resp := request(t, app, "oauth_session=stale"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// The case the guest tier exists for: a valid session that simply does not hold
// the permission. This is 403, not 401 — the operator is signed in.
func TestRequireRejectsSessionWithoutPermission(t *testing.T) {
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		_, _ = io.WriteString(w, `{"allowed": false}`)
	})
	app := gatedApp(NewClient(upstream.URL), PermissionCommands)

	if resp := request(t, app, "oauth_session=guest"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

// A session that holds the permission reaches the handler.
func TestRequireAllowsSessionWithPermission(t *testing.T) {
	upstream := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		_, _ = io.WriteString(w, `{"allowed": true}`)
	})
	app := gatedApp(NewClient(upstream.URL), PermissionHistorical)

	if resp := request(t, app, "oauth_session=valid"); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
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
		if resp.StatusCode != want {
			t.Errorf("%s: status = %d, want %d", permission, resp.StatusCode, want)
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

	if resp := request(t, app, "oauth_session=valid"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
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
	if resp := request(t, app, "oauth_session=valid"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}

	// And an unexpected status, which must not be read as an allow.
	broken := stubOAuthManager(t, func(w http.ResponseWriter, _ string) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	app = gatedApp(NewClient(broken.URL), PermissionLive)
	if resp := request(t, app, "oauth_session=valid"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

// An unset OAUTH_MANAGER_URL must never behave like an allow-all.
func TestRequireFailsClosedWhenUnconfigured(t *testing.T) {
	silenceLog(t)

	app := gatedApp(NewClient(""), PermissionLive)
	if resp := request(t, app, "oauth_session=valid"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
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
