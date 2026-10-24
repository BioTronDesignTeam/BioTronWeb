package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// stubGitHub plays GitHub for one test: the token endpoint, /user, the App's
// installation and its token, and the membership check, which answers
// membership for every call.
type stubGitHub struct {
	*httptest.Server
	key        *rsa.PrivateKey
	membership int

	mu                 sync.Mutex
	installationTokens int
	memberAuth         string
	// verifier is the code_verifier of the last token exchange.
	verifier string
	// rejected is an installation token the membership check answers 401
	// for, as GitHub does for a revoked one.
	rejected string
	// rejectAll answers 401 for every installation token.
	rejectAll bool
	// tokenGate, when set, holds every installation token answer until it
	// is closed.
	tokenGate chan struct{}
}

func newStubGitHub(t *testing.T, membership int) *stubGitHub {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &stubGitHub{key: key, membership: membership}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *stubGitHub) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/login/oauth/access_token":
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.verifier = body["code_verifier"]
		s.mu.Unlock()
		if body["code"] != "good" || body["client_secret"] != "secret" {
			_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"user-token","token_type":"bearer"}`))
	case r.URL.Path == "/user":
		if r.Header.Get("Authorization") != "Bearer user-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"login":"octo","name":"Octo","avatar_url":"https://a.example/o.png"}`))
	case r.URL.Path == "/orgs/BioTronDesignTeam/installation":
		if !s.validAppJWT(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":7}`))
	case r.URL.Path == "/app/installations/7/access_tokens" && r.Method == http.MethodPost:
		if !s.validAppJWT(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		s.mu.Lock()
		gate := s.tokenGate
		s.mu.Unlock()
		if gate != nil {
			<-gate
		}
		s.mu.Lock()
		s.installationTokens++
		token := "installation-token"
		if s.installationTokens > 1 {
			token += "-" + strconv.Itoa(s.installationTokens)
		}
		s.mu.Unlock()
		expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		_, _ = w.Write([]byte(`{"token":"` + token + `","expires_at":"` + expires + `"}`))
	case strings.HasPrefix(r.URL.Path, "/orgs/BioTronDesignTeam/members/"):
		s.mu.Lock()
		s.memberAuth = r.Header.Get("Authorization")
		rejected := s.rejectAll || (s.rejected != "" && s.memberAuth == "Bearer "+s.rejected)
		s.mu.Unlock()
		if rejected {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if s.membership == http.StatusFound {
			w.Header().Set("Location", "/orgs/BioTronDesignTeam/public_members/octo")
		}
		w.WriteHeader(s.membership)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *stubGitHub) validAppJWT(r *http.Request) bool {
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	_, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return &s.key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer("12345"), jwt.WithExpirationRequired())
	return err == nil
}

func (s *stubGitHub) client() *GitHubClient {
	return NewGitHubClient(GitHubOptions{
		AppID:        "12345",
		PrivateKey:   s.key,
		ClientID:     "client-id",
		ClientSecret: "secret",
		CallbackURL:  "http://localhost:8080/auth/github/callback",
		Org:          "BioTronDesignTeam",
		APIBase:      s.URL,
		OAuthBase:    s.URL,
	})
}

func TestAuthCodeURLNamesTheAppAndTheCallback(t *testing.T) {
	g := NewGitHubClient(GitHubOptions{ClientID: "Iv1.abc", CallbackURL: "http://localhost:8080/auth/github/callback"})
	u, err := url.Parse(g.AuthCodeURL("the-state", "the-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "github.com" || u.Path != "/login/oauth/authorize" || q.Get("client_id") != "Iv1.abc" ||
		q.Get("redirect_uri") != "http://localhost:8080/auth/github/callback" || q.Get("state") != "the-state" ||
		q.Get("allow_signup") != "false" || q.Has("scope") ||
		q.Get("code_challenge") != pkceChallenge("the-verifier") || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL = %s", u)
	}
}

func TestSignInStepsAgainstGitHub(t *testing.T) {
	stub := newStubGitHub(t, http.StatusNoContent)
	g := stub.client()
	ctx := context.Background()

	if _, err := g.Exchange(ctx, "used", "the-verifier"); err == nil || !strings.Contains(err.Error(), "bad_verification_code") {
		t.Fatalf("a refused code: err = %v", err)
	}
	token, err := g.Exchange(ctx, "good", "the-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if stub.verifier != "the-verifier" {
		t.Fatalf("code_verifier = %q, want the sign-in's verifier", stub.verifier)
	}
	user, err := g.FetchUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != 42 || user.Login != "octo" || user.Name != "Octo" {
		t.Fatalf("user = %+v", user)
	}
	for range 2 {
		member, err := g.IsOrgMember(ctx, "octo")
		if err != nil || !member {
			t.Fatalf("member = %v, err = %v", member, err)
		}
	}
	if stub.installationTokens != 1 {
		t.Fatalf("installation tokens = %d, want one kept for the second check", stub.installationTokens)
	}
	// The membership is asked with the App's token, never the user's.
	if stub.memberAuth != "Bearer installation-token" {
		t.Fatalf("membership Authorization = %q", stub.memberAuth)
	}
}

func TestMembershipAnswers(t *testing.T) {
	for _, tc := range []struct {
		status  int
		member  bool
		wantErr bool
	}{
		{http.StatusNoContent, true, false},
		{http.StatusNotFound, false, false},
		// 302: the token may not read the members. Following it would read
		// the public members, so it is an error, never a member.
		{http.StatusFound, false, true},
		{http.StatusInternalServerError, false, true},
	} {
		member, err := newStubGitHub(t, tc.status).client().IsOrgMember(context.Background(), "octo")
		if member != tc.member || (err != nil) != tc.wantErr {
			t.Errorf("status %d: member = %v, err = %v", tc.status, member, err)
		}
	}
}

// GitHub refuses an installation token that was revoked before it expired.
// The kept token is dropped and the check asked once more, so a revoked
// token costs one request its retry rather than an hour of failed checks.
func TestMembershipRetriesOnceWithANewTokenAfterA401(t *testing.T) {
	stub := newStubGitHub(t, http.StatusNoContent)
	g := stub.client()
	ctx := context.Background()
	if member, err := g.IsOrgMember(ctx, "octo"); err != nil || !member {
		t.Fatalf("first check: member = %v, err = %v", member, err)
	}

	stub.mu.Lock()
	stub.rejected = "installation-token"
	stub.mu.Unlock()
	if member, err := g.IsOrgMember(ctx, "octo"); err != nil || !member {
		t.Fatalf("check with a revoked token: member = %v, err = %v", member, err)
	}
	if stub.installationTokens != 2 || stub.memberAuth != "Bearer installation-token-2" {
		t.Fatalf("tokens = %d, last Authorization = %q, want a second token used", stub.installationTokens, stub.memberAuth)
	}

	// A new token GitHub refuses too is an error, not an endless retry.
	stub.mu.Lock()
	stub.rejectAll = true
	stub.mu.Unlock()
	if _, err := g.IsOrgMember(ctx, "octo"); err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("a token refused twice: err = %v, want status 401", err)
	}
	if stub.installationTokens != 3 {
		t.Fatalf("tokens = %d, want exactly one more fetch", stub.installationTokens)
	}
}

// Requests that find no usable token share one fetch, and none of them holds
// the lock while GitHub answers: a request that gives up returns at once.
func TestInstallationTokenIsFetchedOnceForConcurrentChecks(t *testing.T) {
	stub := newStubGitHub(t, http.StatusNoContent)
	gate := make(chan struct{})
	stub.tokenGate = gate
	g := stub.client()

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := g.IsOrgMember(context.Background(), "octo"); err != nil {
				errs <- err
			}
		}()
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := g.installationToken(cancelled); err == nil {
		t.Fatal("a cancelled request got a token while the fetch was held")
	}
	if waited := time.Since(start); waited > time.Second {
		t.Fatalf("a cancelled request waited %s for the lock", waited)
	}

	close(gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if stub.installationTokens != 1 {
		t.Fatalf("installation tokens = %d, want one shared fetch", stub.installationTokens)
	}
}

// RFC 7636, appendix B: the S256 challenge of its example verifier.
func TestPKCEChallengeIsS256(t *testing.T) {
	if got := pkceChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge = %q", got)
	}
}
