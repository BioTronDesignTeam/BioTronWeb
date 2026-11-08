// Package authclienttest is a stand-in Auth for the tests of a service that
// gates its routes with authclient. It answers GET /v1/session the way Auth
// does, so a test drives the real client over HTTP instead of a mock.
package authclienttest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
)

// Auth answers /v1/session. A new one has no one signed in: every cookie is
// refused with 401, as Auth refuses an unknown one.
type Auth struct {
	server *httptest.Server

	mu       sync.Mutex
	status   int
	operator *authclient.Operator
	granted  map[string]bool
	calls    int
	last     url.Values
}

// New starts a stand-in Auth, stopped when the test ends.
func New(t testing.TB) *Auth {
	t.Helper()
	a := &Auth{status: http.StatusUnauthorized}
	a.server = httptest.NewServer(http.HandlerFunc(a.serve))
	t.Cleanup(a.server.Close)
	return a
}

// URL is the base URL to give authclient.New.
func (a *Auth) URL() string { return a.server.URL }

// Client returns a client for appID that asks this stand-in.
func (a *Auth) Client(appID string, options ...authclient.Option) *authclient.Client {
	return authclient.New(a.server.URL, appID, options...)
}

// SignIn makes every request with a cookie belong to operator, holding the
// permissions granted and no others.
func (a *Auth) SignIn(operator authclient.Operator, granted ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = http.StatusOK
	a.operator = &operator
	a.granted = map[string]bool{}
	for _, permission := range granted {
		a.granted[permission] = true
	}
}

// Answer makes every request with a cookie get status and no body: 401 for a
// refused session, 403 for a guest of another tool, 500 for a broken Auth.
func (a *Auth) Answer(status int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = status
	a.operator = nil
}

// Calls is how many requests reached the stand-in.
func (a *Auth) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

// LastQuery is the query of the last request.
func (a *Auth) LastQuery() url.Values {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.last
}

func (a *Auth) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	a.last = r.URL.Query()
	if r.Method != http.MethodGet || r.URL.Path != "/v1/session" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Header.Get("Cookie") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if a.status != http.StatusOK {
		w.WriteHeader(a.status)
		return
	}
	permissions := map[string]bool{}
	for _, permission := range r.URL.Query()["permission"] {
		permissions[permission] = a.granted[permission]
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(sessionBody{Operator: a.operator, Permissions: permissions}); err != nil {
		panic(err)
	}
}

type sessionBody struct {
	Operator    *authclient.Operator `json:"operator"`
	Permissions map[string]bool      `json:"permissions"`
}
