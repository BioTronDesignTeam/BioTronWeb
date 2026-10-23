package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		s.installationTokens++
		s.mu.Unlock()
		expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		_, _ = w.Write([]byte(`{"token":"installation-token","expires_at":"` + expires + `"}`))
	case strings.HasPrefix(r.URL.Path, "/orgs/BioTronDesignTeam/members/"):
		s.mu.Lock()
		s.memberAuth = r.Header.Get("Authorization")
		s.mu.Unlock()
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
	u, err := url.Parse(g.AuthCodeURL("the-state"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "github.com" || u.Path != "/login/oauth/authorize" || q.Get("client_id") != "Iv1.abc" ||
		q.Get("redirect_uri") != "http://localhost:8080/auth/github/callback" || q.Get("state") != "the-state" ||
		q.Get("allow_signup") != "false" || q.Has("scope") {
		t.Fatalf("authorize URL = %s", u)
	}
}

func TestSignInStepsAgainstGitHub(t *testing.T) {
	stub := newStubGitHub(t, http.StatusNoContent)
	g := stub.client()
	ctx := context.Background()

	if _, err := g.Exchange(ctx, "used"); err == nil || !strings.Contains(err.Error(), "bad_verification_code") {
		t.Fatalf("a refused code: err = %v", err)
	}
	token, err := g.Exchange(ctx, "good")
	if err != nil {
		t.Fatal(err)
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
