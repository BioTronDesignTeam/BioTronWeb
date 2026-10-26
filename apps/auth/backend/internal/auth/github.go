package auth

// Sign-in with GitHub is the user authorization of the BioTron GitHub App:
//
//  1. AuthCodeURL sends the browser to GitHub with the App's client id, the
//     callback URL, a random state, and the S256 challenge of a PKCE verifier.
//  2. GitHub sends the browser back with a code. Exchange trades it for a user
//     token, with the client secret and the verifier, so a code stolen on its
//     way back is worth nothing without this browser's state cookie.
//  3. FetchUser reads the user with that token. The token then goes out of
//     scope: it is never stored, logged, or returned.
//  4. IsOrgMember asks whether the user is a member of the organization with
//     the App's own installation token, which holds Members: read and nothing
//     else. The user is never asked for an organization scope.
//
// Never log or return the client secret, the code, the state, the verifier, a
// token, or the App's key. Errors name the step and GitHub's status, never
// GitHub's text.

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	githubTimeout    = 10 * time.Second
	githubAPIVersion = "2022-11-28"
	// GitHub accepts an App JWT of at most ten minutes and advises an iat a
	// minute in the past against clock drift.
	appJWTLifetime = 9 * time.Minute
	appJWTBackdate = time.Minute
	// An installation token lasts an hour; a new one is fetched five minutes
	// before it ends.
	installationTokenMargin = 5 * time.Minute
)

type GitHubClient struct {
	appID        string
	key          *rsa.PrivateKey
	clientID     string
	clientSecret string
	callbackURL  string
	org          string
	apiBase      string
	oauthBase    string
	http         *http.Client

	// mu guards the installation token and the one fetch of it in flight;
	// it is never held across a call to GitHub.
	mu        sync.Mutex
	orgToken  string
	expiresAt time.Time
	fetching  *tokenFetch
}

// tokenFetch is one fetch of the installation token, which every caller that
// finds no usable token waits on, so a slow GitHub costs one fetch, not one
// per request.
type tokenFetch struct {
	done  chan struct{}
	token string
	err   error
}

type GitHubOptions struct {
	// AppID and PrivateKey sign the App's JWT, which gets the installation
	// token of the organization.
	AppID      string
	PrivateKey *rsa.PrivateKey
	// ClientID and ClientSecret are the App's, for the user authorization.
	ClientID     string
	ClientSecret string
	CallbackURL  string
	Org          string
	// APIBase and OAuthBase default to GitHub's; tests point them at a stub.
	APIBase   string
	OAuthBase string
}

type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func NewGitHubClient(opts GitHubOptions) *GitHubClient {
	apiBase := opts.APIBase
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	oauthBase := opts.OAuthBase
	if oauthBase == "" {
		oauthBase = "https://github.com"
	}
	return &GitHubClient{
		appID:        opts.AppID,
		key:          opts.PrivateKey,
		clientID:     opts.ClientID,
		clientSecret: opts.ClientSecret,
		callbackURL:  opts.CallbackURL,
		org:          opts.Org,
		apiBase:      strings.TrimRight(apiBase, "/"),
		oauthBase:    strings.TrimRight(oauthBase, "/"),
		http: &http.Client{
			Timeout: githubTimeout,
			// GitHub answers a membership check with 302 when the token may
			// not read the members; followed, it would read the public
			// members instead. No call here expects a redirect.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// AuthCodeURL returns GitHub's sign-in page for one sign-in with state and
// the PKCE verifier, of which only the S256 challenge is sent.
func (g *GitHubClient) AuthCodeURL(state, verifier string) string {
	query := url.Values{
		"client_id":             {g.clientID},
		"redirect_uri":          {g.callbackURL},
		"state":                 {state},
		"allow_signup":          {"false"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	return g.oauthBase + "/login/oauth/authorize?" + query.Encode()
}

// Exchange trades the code of the callback, and the PKCE verifier of its
// sign-in, for a user token.
func (g *GitHubClient) Exchange(ctx context.Context, code, verifier string) (string, error) {
	var answer struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	err := g.call(ctx, http.MethodPost, g.oauthBase+"/login/oauth/access_token", "", map[string]string{
		"client_id":     g.clientID,
		"client_secret": g.clientSecret,
		"code":          code,
		"redirect_uri":  g.callbackURL,
		"code_verifier": verifier,
	}, &answer)
	if err != nil {
		return "", err
	}
	if answer.AccessToken == "" {
		// GitHub answers 200 with an error code, such as
		// bad_verification_code for a used or expired code. Only a code
		// of GitHub's known shape is repeated.
		reason := "no token"
		if isErrorCode(answer.Error) {
			reason = answer.Error
		}
		return "", fmt.Errorf("github token exchange: %s", reason)
	}
	return answer.AccessToken, nil
}

// FetchUser reads the user of a user token.
func (g *GitHubClient) FetchUser(ctx context.Context, token string) (*GitHubUser, error) {
	var u GitHubUser
	if err := g.call(ctx, http.MethodGet, g.apiBase+"/user", token, nil, &u); err != nil {
		return nil, err
	}
	if u.ID <= 0 || u.Login == "" {
		return nil, errors.New("github GET /user: answer without id or login")
	}
	return &u, nil
}

// IsOrgMember reports whether login is a member of the organization:
// GET /orgs/{org}/members/{login} answers 204 for a member and 404 for anyone
// else. A 302 means the installation token may not read the members, which is
// an error of the App's settings, never a "no". A 401 means GitHub no longer
// takes the kept installation token, such as after the App's key was revoked:
// it is dropped and the check asked once more with a new one.
func (g *GitHubClient) IsOrgMember(ctx context.Context, login string) (bool, error) {
	status, err := g.membershipStatus(ctx, login)
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusNoContent:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	case http.StatusFound:
		return false, fmt.Errorf("github membership check: the App may not read the members of %s (grant it Members: read)", g.org)
	default:
		return false, fmt.Errorf("github membership check: status %d", status)
	}
}

// membershipStatus asks GitHub once, and once more with a new installation
// token if GitHub refused the kept one.
func (g *GitHubClient) membershipStatus(ctx context.Context, login string) (int, error) {
	path := "/orgs/" + url.PathEscape(g.org) + "/members/" + url.PathEscape(login)
	for attempt := 1; ; attempt++ {
		token, err := g.installationToken(ctx)
		if err != nil {
			return 0, err
		}
		req, err := g.request(ctx, http.MethodGet, g.apiBase+path, token, nil)
		if err != nil {
			return 0, err
		}
		resp, err := g.http.Do(req)
		if err != nil {
			return 0, fmt.Errorf("github membership check: %w", redact(err))
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && attempt == 1 {
			g.dropToken(token)
			continue
		}
		return resp.StatusCode, nil
	}
}

// installationToken returns the token of the App's installation on the
// organization, limited to Members: read, kept until shortly before it ends.
// Callers that find no usable token share one fetch.
func (g *GitHubClient) installationToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	if g.orgToken != "" && time.Now().Before(g.expiresAt.Add(-installationTokenMargin)) {
		token := g.orgToken
		g.mu.Unlock()
		return token, nil
	}
	fetch := g.fetching
	if fetch == nil {
		fetch = &tokenFetch{done: make(chan struct{})}
		g.fetching = fetch
		// The fetch serves every waiting request, so the first one's
		// cancellation must not fail the others; the client's timeout still
		// bounds it.
		go g.fetchInstallationToken(context.WithoutCancel(ctx), fetch)
	}
	g.mu.Unlock()
	select {
	case <-fetch.done:
		return fetch.token, fetch.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// fetchInstallationToken runs one fetch and keeps its token.
func (g *GitHubClient) fetchInstallationToken(ctx context.Context, fetch *tokenFetch) {
	token, expiresAt, err := g.newInstallationToken(ctx)
	g.mu.Lock()
	if err == nil {
		g.orgToken, g.expiresAt = token, expiresAt
	}
	g.fetching = nil
	g.mu.Unlock()
	fetch.token, fetch.err = token, err
	close(fetch.done)
}

// dropToken forgets the kept installation token if it is still token, so the
// next caller fetches a new one.
func (g *GitHubClient) dropToken(token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.orgToken == token {
		g.orgToken, g.expiresAt = "", time.Time{}
	}
}

// newInstallationToken asks GitHub for a new installation token.
func (g *GitHubClient) newInstallationToken(ctx context.Context) (string, time.Time, error) {
	appJWT, err := g.appJWT()
	if err != nil {
		return "", time.Time{}, err
	}
	var installation struct {
		ID int64 `json:"id"`
	}
	if err := g.call(ctx, http.MethodGet, g.apiBase+"/orgs/"+url.PathEscape(g.org)+"/installation",
		appJWT, nil, &installation); err != nil {
		return "", time.Time{}, fmt.Errorf("find the App's installation on %s: %w", g.org, err)
	}
	var access struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := g.call(ctx, http.MethodPost,
		g.apiBase+"/app/installations/"+strconv.FormatInt(installation.ID, 10)+"/access_tokens",
		appJWT, map[string]any{"permissions": map[string]string{"members": "read"}}, &access); err != nil {
		return "", time.Time{}, fmt.Errorf("get the installation token of %s: %w", g.org, err)
	}
	if access.Token == "" || access.ExpiresAt.IsZero() {
		return "", time.Time{}, errors.New("github installation token: answer without token or expiry")
	}
	return access.Token, access.ExpiresAt, nil
}

// appJWT signs the App's own JWT, which only finds the installation and gets
// its token.
func (g *GitHubClient) appJWT() (string, error) {
	if g.key == nil || g.appID == "" {
		return "", errors.New("GitHub App id or private key not configured")
	}
	now := time.Now()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Issuer:    g.appID,
		IssuedAt:  jwt.NewNumericDate(now.Add(-appJWTBackdate)),
		ExpiresAt: jwt.NewNumericDate(now.Add(appJWTLifetime)),
	}).SignedString(g.key)
	if err != nil {
		return "", errors.New("cannot sign the GitHub App JWT")
	}
	return signed, nil
}

func (g *GitHubClient) request(ctx context.Context, method, target, token string, body any) (*http.Request, error) {
	var reader io.Reader = http.NoBody
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if strings.HasSuffix(target, "/login/oauth/access_token") {
		req.Header.Set("Accept", "application/json")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", "biotron-auth")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	return req, nil
}

// call sends one request and decodes its JSON answer into out. The error
// names the path and the status: the query, the body, and GitHub's text can
// hold a code or a secret.
func (g *GitHubClient) call(ctx context.Context, method, target, token string, body, out any) error {
	req, err := g.request(ctx, method, target, token, body)
	if err != nil {
		return err
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("github %s %s: %w", method, req.URL.Path, redact(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github %s %s: status %d", method, req.URL.Path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("github %s %s: unreadable answer", method, req.URL.Path)
	}
	return nil
}

// pkceChallenge returns the S256 challenge of a PKCE verifier (RFC 7636).
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// redact drops the URL from a transport error, keeping only its cause.
func redact(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// isErrorCode reports whether s looks like one of GitHub's error codes, such
// as bad_verification_code, and so is safe to log.
func isErrorCode(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && r != '_' {
			return false
		}
	}
	return true
}
