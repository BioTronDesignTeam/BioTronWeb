package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/auth"
)

// The Fiber v3 migration rewrote every line of the wiring below: the trusted
// proxy fields were renamed, CORS moved from comma-joined strings to slices,
// and handlers changed shape. None of that is allowed to change what the
// service actually enforces, so these tests exercise the real router that
// New() builds rather than a stand-in.

const allowedOrigin = "http://localhost:5174"

// newTestApp builds the production router. The handler has no store, which is
// fine: every assertion here is about a gate that answers before any handler
// would reach the database.
func newTestApp(trustedProxies []string) *fiber.App {
	return New(&auth.Handler{}, []string{allowedOrigin}, trustedProxies, nil)
}

func TestHealthStaysPublic(t *testing.T) {
	response, err := newTestApp(nil).Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("GET /health = %d, want 200", response.StatusCode)
	}
}

// Logger, Calendar and exo-gui all read /v1/check, and they tell "signed out"
// from "signed in without the permission" by its status. RequireSession has to
// answer before the handler sees the query, so a missing parameter on an
// anonymous request is still 401 and never 400.
func TestCheckAnswersUnauthorizedBeforeItValidatesParameters(t *testing.T) {
	app := newTestApp(nil)
	for _, target := range []string{
		"/v1/check?app=logger&permission=view",
		"/v1/check",
	} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, target, nil))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("GET %s = %d, want 401", target, response.StatusCode)
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
		{http.MethodPost, "/apps"},
		{http.MethodPost, "/grants"},
		{http.MethodDelete, "/grants"},
	} {
		bare, err := app.Test(httptest.NewRequest(route.method, route.target, nil))
		if err != nil {
			t.Fatal(err)
		}
		if bare.StatusCode != fiber.StatusForbidden {
			t.Errorf("%s %s without X-Requested-With = %d, want 403",
				route.method, route.target, bare.StatusCode)
		}

		// With the header the request gets past CSRF and lands on the session
		// check, which is the next gate and still refuses an anonymous caller.
		request := httptest.NewRequest(route.method, route.target, nil)
		request.Header.Set("X-Requested-With", "XMLHttpRequest")
		withHeader, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		if withHeader.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("%s %s with X-Requested-With = %d, want 401",
				route.method, route.target, withHeader.StatusCode)
		}
	}
}

// The /org routes also carry RequireXHR, but the group already installs
// RequireSession and RequireStaff ahead of every route in it, so an anonymous
// caller is turned away as unauthenticated before the CSRF guard is reached.
// That ordering is what the group is for, and it must not silently invert into
// a state-changing route answering an unauthenticated request.
func TestOrgGroupAnswersUnauthenticatedBeforeAnyRouteHandler(t *testing.T) {
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
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("%s %s (X-Requested-With=%q) = %d, want 401",
					route.method, route.target, header, response.StatusCode)
			}
		}
	}
}

func TestCORSAnswersOnlyListedOriginsAndKeepsCredentials(t *testing.T) {
	app := newTestApp(nil)

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("Origin", allowedOrigin)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want the exact origin %q", got, allowedOrigin)
	}
	if got := response.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	if got := response.Header.Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("Vary = %q, want it to include Origin so caches cannot cross origins", got)
	}

	denied := httptest.NewRequest(http.MethodGet, "/health", nil)
	denied.Header.Set("Origin", "https://evil.example")
	deniedResponse, err := app.Test(denied)
	if err != nil {
		t.Fatal(err)
	}
	if got := deniedResponse.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q for an unlisted origin, want it absent", got)
	}
}

// guestLoginStatus posts one guest login carrying the given forwarded client
// address. The body is deliberately empty, so a request the limiter admits
// stops at the 400 for a missing app_id and never reaches the database.
func guestLoginStatus(t *testing.T, app *fiber.App, clientIP string) int {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/auth/guest", nil)
	request.Header.Set("Cf-Connecting-Ip", clientIP)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode
}

// The limiter on guest login must key on the individual caller. That only
// works while Fiber trusts the edge to set Cf-Connecting-Ip: if the trusted
// proxy configuration stops taking effect, every client in the world shares a
// single bucket and one visitor can lock everyone out. Fiber's test transport
// dials from 0.0.0.0, so trusting that address stands in for the edge.
func TestGuestLoginLimiterKeysOnTheForwardedClientBehindATrustedProxy(t *testing.T) {
	app := newTestApp([]string{"0.0.0.0"})

	for i := range 20 {
		if status := guestLoginStatus(t, app, "203.0.113.10"); status != fiber.StatusBadRequest {
			t.Fatalf("guest login %d = %d, want 400 while under the limit", i+1, status)
		}
	}
	if status := guestLoginStatus(t, app, "203.0.113.10"); status != fiber.StatusTooManyRequests {
		t.Fatalf("guest login 21 = %d, want 429", status)
	}
	if status := guestLoginStatus(t, app, "203.0.113.11"); status != fiber.StatusBadRequest {
		t.Fatalf("a second client = %d, want 400: the limiter collapsed into one global bucket", status)
	}
}

// The mirror image: an untrusted peer cannot spend someone else's bucket or
// escape its own by inventing a Cf-Connecting-Ip header.
func TestGuestLoginLimiterIgnoresTheForwardedHeaderFromAnUntrustedPeer(t *testing.T) {
	app := newTestApp([]string{"127.0.0.1", "::1"})

	for i := range 20 {
		if status := guestLoginStatus(t, app, "203.0.113.10"); status != fiber.StatusBadRequest {
			t.Fatalf("guest login %d = %d, want 400 while under the limit", i+1, status)
		}
	}
	if status := guestLoginStatus(t, app, "198.51.100.7"); status != fiber.StatusTooManyRequests {
		t.Fatalf("a spoofed client address = %d, want 429: the header was trusted from an untrusted peer", status)
	}
}
