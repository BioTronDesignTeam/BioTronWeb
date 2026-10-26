package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/auth"
)

// The move from Fiber to net/http rewrote every line of the wiring below.
// None of that is allowed to change what the service actually enforces, so
// these tests exercise the real router that New() builds rather than a
// stand-in.

// allowedOrigin is a sibling tool's frontend, ownOrigin Auth's own.
const (
	allowedOrigin = "http://localhost:5174"
	ownOrigin     = "http://localhost:5173"
)

// newTestApp builds the production router. The handler has no store, which is
// fine: every assertion here is about a gate that answers before any handler
// would reach the database.
func newTestApp(trustedProxies []string) http.Handler {
	return New(&auth.Handler{Cfg: auth.Config{FrontendURL: ownOrigin}}, []string{ownOrigin, allowedOrigin}, trustedProxies, nil)
}

func serve(app http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

func TestHealthStaysPublic(t *testing.T) {
	response := serve(newTestApp(nil), httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /health = %d, want 200", response.Code)
	}
	if got := response.Body.String(); got != `{"status":"ok"}` {
		t.Fatalf("GET /health body = %q", got)
	}
}

// Calendar, Sprinter and exo-gui all read /v1/check, and they tell "signed out"
// from "signed in without the permission" by its status. RequireSession has to
// answer before the handler sees the query, so a missing parameter on an
// anonymous request is still 401 and never 400.
func TestCheckAnswersUnauthorizedBeforeItValidatesParameters(t *testing.T) {
	app := newTestApp(nil)
	for _, target := range []string{
		"/v1/check?app=calendar&permission=write",
		"/v1/check",
		"/v1/session?app=calendar&permission=write",
		"/v1/session",
	} {
		response := serve(app, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("GET %s = %d, want 401", target, response.Code)
		}
		if got := response.Body.String(); got != "Unauthorized" {
			t.Errorf("GET %s body = %q, want the bare status text Fiber sent", target, got)
		}
	}
}

// Every mutating route carries RequireXHR, and it runs ahead of the session
// lookup so a cross-site form post is refused before it can touch state.
func TestMutatingRoutesRefuseRequestsWithoutTheXHRHeader(t *testing.T) {
	app := newTestApp(nil)
	for _, route := range []struct {
		method string
		target string
	}{
		{http.MethodPost, "/auth/logout"},
		{http.MethodPost, "/auth/guest-keys/exo-gui/rotate"},
		{http.MethodPost, "/apps"},
		{http.MethodPost, "/grants"},
		{http.MethodDelete, "/grants"},
	} {
		bare := serve(app, httptest.NewRequest(route.method, route.target, nil))
		if bare.Code != http.StatusForbidden {
			t.Errorf("%s %s without X-Requested-With = %d, want 403",
				route.method, route.target, bare.Code)
		}

		// With the header the request gets past CSRF and lands on the session
		// check, which is the next gate and still refuses an anonymous caller.
		request := httptest.NewRequest(route.method, route.target, nil)
		request.Header.Set("X-Requested-With", "XMLHttpRequest")
		withHeader := serve(app, request)
		if withHeader.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with X-Requested-With = %d, want 401",
				route.method, route.target, withHeader.Code)
		}
	}
}

// The /org routes also carry RequireXHR, but RequireSession and RequireStaff
// run ahead of every route there, so an anonymous caller is turned away as
// unauthenticated before the CSRF guard is reached. That ordering must not
// silently invert into a state-changing route answering an unauthenticated
// request.
func TestOrgRoutesAnswerUnauthenticatedBeforeAnyRouteHandler(t *testing.T) {
	app := newTestApp(nil)
	for _, route := range []struct {
		method string
		target string
	}{
		{http.MethodGet, "/org/members"},
		{http.MethodGet, "/org/members/1/grants"},
		{http.MethodPost, "/org/members/1/ban"},
		{http.MethodPost, "/org/members/1/unban"},
		{http.MethodPatch, "/org/members/1/manager"},
	} {
		for _, header := range []string{"", "XMLHttpRequest"} {
			request := httptest.NewRequest(route.method, route.target, nil)
			if header != "" {
				request.Header.Set("X-Requested-With", header)
			}
			response := serve(app, request)
			if response.Code != http.StatusUnauthorized {
				t.Errorf("%s %s (X-Requested-With=%q) = %d, want 401",
					route.method, route.target, header, response.Code)
			}
		}
	}
}

func TestEveryRouteIsMounted(t *testing.T) {
	app := newTestApp(nil)
	for _, route := range []struct {
		method string
		target string
		want   int
	}{
		{http.MethodGet, "/auth/me", http.StatusUnauthorized},
		{http.MethodGet, "/auth/guest-keys", http.StatusUnauthorized},
		{http.MethodGet, "/apps", http.StatusUnauthorized},
		{http.MethodGet, "/permissions", http.StatusUnauthorized},
		{http.MethodGet, "/me/grants", http.StatusUnauthorized},
		{http.MethodGet, "/nowhere", http.StatusNotFound},
	} {
		if response := serve(app, httptest.NewRequest(route.method, route.target, nil)); response.Code != route.want {
			t.Errorf("%s %s = %d, want %d", route.method, route.target, response.Code, route.want)
		}
	}
}

func TestCORSAnswersOnlyListedOriginsAndKeepsCredentials(t *testing.T) {
	app := newTestApp(nil)

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("Origin", ownOrigin)
	response := serve(app, request)
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != ownOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want the exact origin %q", got, ownOrigin)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	if got := response.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("Vary = %q, want it to include Origin so caches cannot cross origins", got)
	}

	denied := httptest.NewRequest(http.MethodGet, "/health", nil)
	denied.Header.Set("Origin", "https://evil.example")
	if got := serve(app, denied).Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q for an unlisted origin, want it absent", got)
	}
}

// Another tool's frontend gets credentialed CORS only for what the tools
// call: /auth/me, /v1/check, /v1/session, sign-out, and the guest sign-in. Script on a
// sibling hostname cannot read the roster or send a staff action with a staff
// member's cookie; only Auth's own frontend can.
func TestCORSLimitsSiblingOriginsToTheRoutesTheyCall(t *testing.T) {
	app := newTestApp(nil)
	for _, tc := range []struct {
		origin, method, path string
		allowed              bool
	}{
		{allowedOrigin, http.MethodGet, "/auth/me", true},
		{allowedOrigin, http.MethodGet, "/v1/check", true},
		{allowedOrigin, http.MethodGet, "/v1/session", true},
		{allowedOrigin, http.MethodPost, "/auth/logout", true},
		{allowedOrigin, http.MethodPost, "/auth/guest", true},
		{allowedOrigin, http.MethodGet, "/org/members", false},
		{allowedOrigin, http.MethodGet, "/auth/guest-keys", false},
		{allowedOrigin, http.MethodPost, "/grants", false},
		{allowedOrigin, http.MethodDelete, "/grants", false},
		{allowedOrigin, http.MethodPost, "/org/members/1/ban", false},
		{allowedOrigin, http.MethodPatch, "/org/members/1/manager", false},
		{allowedOrigin, http.MethodPost, "/auth/guest-keys/exo-gui/rotate", false},
		{allowedOrigin, http.MethodPost, "/apps", false},
		{ownOrigin, http.MethodGet, "/org/members", true},
		{ownOrigin, http.MethodDelete, "/grants", true},
		{ownOrigin, http.MethodPost, "/org/members/1/ban", true},
	} {
		preflight := httptest.NewRequest(http.MethodOptions, tc.path, nil)
		preflight.Header.Set("Origin", tc.origin)
		preflight.Header.Set("Access-Control-Request-Method", tc.method)
		preflight.Header.Set("Access-Control-Request-Headers", "content-type, x-requested-with")
		if got := serve(app, preflight).Header().Get("Access-Control-Allow-Origin") != ""; got != tc.allowed {
			t.Errorf("preflight %s %s from %s allowed = %v, want %v", tc.method, tc.path, tc.origin, got, tc.allowed)
		}
		request := httptest.NewRequest(tc.method, tc.path, nil)
		request.Header.Set("Origin", tc.origin)
		if got := serve(app, request).Header().Get("Access-Control-Allow-Origin") != ""; got != tc.allowed {
			t.Errorf("%s %s from %s allowed = %v, want %v", tc.method, tc.path, tc.origin, got, tc.allowed)
		}
	}
}

// A preflight is answered by the CORS layer with 204 and the allowed methods
// and headers; it never reaches the session check of the route behind it.
func TestCORSPreflightIsAnsweredBeforeTheRoute(t *testing.T) {
	app := newTestApp(nil)
	request := httptest.NewRequest(http.MethodOptions, "/grants", nil)
	request.Header.Set("Origin", ownOrigin)
	request.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	request.Header.Set("Access-Control-Request-Headers", "content-type, x-requested-with")
	response := serve(app, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight = %d, want 204", response.Code)
	}
	header := response.Header()
	if got := header.Get("Access-Control-Allow-Origin"); got != ownOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
	if got := header.Get("Access-Control-Allow-Methods"); got != "GET, POST, DELETE, PATCH, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q", got)
	}
	if got := header.Get("Access-Control-Allow-Headers"); got != "Content-Type, X-Requested-With" {
		t.Errorf("Access-Control-Allow-Headers = %q", got)
	}
}

// guestLoginStatus posts one guest login from peer carrying the given
// forwarded client address. The body is deliberately empty, so a request the
// limiter admits stops at the 400 for a missing app_id and never reaches the
// database.
func guestLoginStatus(t *testing.T, app http.Handler, peer, clientIP string) int {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/auth/guest", nil)
	request.RemoteAddr = peer + ":40000"
	request.Header.Set("Cf-Connecting-Ip", clientIP)
	return serve(app, request).Code
}

// The limiter on guest login must key on the individual caller. That only
// works while the edge is trusted to set Cf-Connecting-Ip: if the trusted
// proxy configuration stops taking effect, every client in the world shares a
// single bucket and one visitor can lock everyone out.
func TestGuestLoginLimiterKeysOnTheForwardedClientBehindATrustedProxy(t *testing.T) {
	app := newTestApp([]string{"172.16.0.0/12"})
	const edge = "172.18.0.5"

	for i := range 20 {
		if status := guestLoginStatus(t, app, edge, "203.0.113.10"); status != http.StatusBadRequest {
			t.Fatalf("guest login %d = %d, want 400 while under the limit", i+1, status)
		}
	}
	if status := guestLoginStatus(t, app, edge, "203.0.113.10"); status != http.StatusTooManyRequests {
		t.Fatalf("guest login 21 = %d, want 429", status)
	}
	if status := guestLoginStatus(t, app, edge, "203.0.113.11"); status != http.StatusBadRequest {
		t.Fatalf("a second client = %d, want 400: the limiter collapsed into one global bucket", status)
	}
}

// The mirror image: an untrusted peer cannot spend someone else's bucket or
// escape its own by inventing a Cf-Connecting-Ip header.
func TestGuestLoginLimiterIgnoresTheForwardedHeaderFromAnUntrustedPeer(t *testing.T) {
	app := newTestApp([]string{"127.0.0.1", "::1"})
	const peer = "198.51.100.7"

	for i := range 20 {
		if status := guestLoginStatus(t, app, peer, "203.0.113.10"); status != http.StatusBadRequest {
			t.Fatalf("guest login %d = %d, want 400 while under the limit", i+1, status)
		}
	}
	if status := guestLoginStatus(t, app, peer, "203.0.113.99"); status != http.StatusTooManyRequests {
		t.Fatalf("a spoofed client address = %d, want 429: the header was trusted from an untrusted peer", status)
	}
}

func TestLimiterHeadersAndWindowReset(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := newLimiter(2, time.Minute, func(*http.Request) string { return "one" })
	l.now = func() time.Time { return now }
	app := l.wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	first := serve(app, httptest.NewRequest(http.MethodPost, "/", nil))
	if first.Header().Get("X-RateLimit-Limit") != "2" || first.Header().Get("X-RateLimit-Remaining") != "1" ||
		first.Header().Get("X-RateLimit-Reset") != "60" {
		t.Fatalf("first request headers = %v", first.Header())
	}
	serve(app, httptest.NewRequest(http.MethodPost, "/", nil))
	now = now.Add(15 * time.Second)
	refused := serve(app, httptest.NewRequest(http.MethodPost, "/", nil))
	if refused.Code != http.StatusTooManyRequests || refused.Header().Get("Retry-After") != "45" ||
		refused.Body.String() != "Too Many Requests" {
		t.Fatalf("third request = %d %v %q", refused.Code, refused.Header(), refused.Body.String())
	}
	now = now.Add(45 * time.Second)
	if again := serve(app, httptest.NewRequest(http.MethodPost, "/", nil)); again.Code != http.StatusNoContent {
		t.Fatalf("after the window = %d, want the request admitted", again.Code)
	}
}
