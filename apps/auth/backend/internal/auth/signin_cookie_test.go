package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestStateCookiePathFollowsTheCallback(t *testing.T) {
	for callback, want := range map[string]string{
		"http://localhost:8080/auth/github/callback":             "/auth/github",
		"https://auth.uwbiotron.dev/api/auth/github/callback":    "/api/auth/github",
		"https://auth.uwbiotron.ca/api/auth/github/callback?x=1": "/api/auth/github",
		"https://auth.example/elsewhere":                         "/",
		"https://auth.example/callback":                          "/",
		"::not a url":                                            "/",
	} {
		if got := StateCookiePath(callback); got != want {
			t.Errorf("StateCookiePath(%q) = %q, want %q", callback, got, want)
		}
	}
}

// The state cookie goes only to the sign-in routes as the browser sees them,
// and the challenge sent to GitHub is the S256 of the verifier it carries.
func TestLoginScopesTheStateCookieAndSendsAPKCEChallenge(t *testing.T) {
	stub := newStubGitHub(t, http.StatusNoContent)
	h := &Handler{
		GitHub: stub.client(),
		Signer: NewSigner(testSigningKey),
		Cfg: Config{
			FrontendURL:     "http://localhost:5173",
			StateCookiePath: StateCookiePath("https://auth.uwbiotron.dev/api/auth/github/callback"),
		},
	}
	login := httptest.NewRecorder()
	h.LoginRedirect(login, httptest.NewRequest(http.MethodGet, "/auth/github/login", nil))
	cookie := cookieFrom(t, login, stateCookie)
	if cookie.Path != "/api/auth/github" || !cookie.HttpOnly {
		t.Fatalf("state cookie = %+v, want HttpOnly at /api/auth/github", cookie)
	}
	signIn, ok := h.Signer.ReadState(cookie.Value)
	if !ok || len(signIn.Verifier) < 43 {
		t.Fatalf("state cookie holds %+v, %v; want a verifier of at least 43 characters", signIn, ok)
	}
	authorize, err := url.Parse(login.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if q := authorize.Query(); q.Get("code_challenge") != pkceChallenge(signIn.Verifier) || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL = %s", authorize)
	}

	// The callback clears the cookie at the same path, or the browser would
	// keep it.
	callback := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=good&state=wrong", nil)
	callback.AddCookie(cookie)
	cleared := httptest.NewRecorder()
	h.Callback(cleared, callback)
	for _, c := range cleared.Result().Cookies() {
		if c.Name == stateCookie && (c.Path != "/api/auth/github" || c.Expires.After(time.Now())) {
			t.Fatalf("cleared state cookie = %+v", c)
		}
	}
}
