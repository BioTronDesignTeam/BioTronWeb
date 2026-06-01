package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"
)

// githubTimeout bounds every outbound GitHub call (token exchange + API) so a
// slow/hung GitHub can't tie up an OAuth callback request indefinitely.
const githubTimeout = 10 * time.Second

// GitHubClient wraps the OAuth flow plus the two GitHub API calls we need:
// fetching the user and checking org membership.
type GitHubClient struct {
	oauth   *oauth2.Config
	apiBase string
	org     string
	http    *http.Client
}

// GitHubOptions configures NewGitHubClient. APIBase/AuthURL/TokenURL default to
// real GitHub; tests override them to point at a fake server.
type GitHubOptions struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
	Org          string
	APIBase      string
	AuthURL      string
	TokenURL     string
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
	endpoint := githuboauth.Endpoint
	if opts.AuthURL != "" {
		endpoint.AuthURL = opts.AuthURL
	}
	if opts.TokenURL != "" {
		endpoint.TokenURL = opts.TokenURL
	}
	return &GitHubClient{
		oauth: &oauth2.Config{
			ClientID:     opts.ClientID,
			ClientSecret: opts.ClientSecret,
			RedirectURL:  opts.CallbackURL,
			Scopes:       []string{"read:org"},
			Endpoint:     endpoint,
		},
		apiBase: apiBase,
		org:     opts.Org,
		http:    &http.Client{Timeout: githubTimeout},
	}
}

func (g *GitHubClient) AuthCodeURL(state string) string {
	return g.oauth.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *GitHubClient) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, g.http)
	return g.oauth.Exchange(ctx, code)
}

func (g *GitHubClient) FetchUser(ctx context.Context, tok *oauth2.Token) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase+"/user", nil)
	if err != nil {
		return nil, err
	}
	g.authHeaders(req, tok)
	resp, err := g.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github GET /user: status %d", resp.StatusCode)
	}
	var u GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// IsOrgMember reports whether the authenticated user is an active member of the
// configured org (requires the read:org scope).
func (g *GitHubClient) IsOrgMember(ctx context.Context, tok *oauth2.Token) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase+"/user/memberships/orgs/"+g.org, nil)
	if err != nil {
		return false, err
	}
	g.authHeaders(req, tok)
	resp, err := g.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var m struct {
			State string `json:"state"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			return false, err
		}
		return m.State == "active", nil
	case http.StatusNotFound, http.StatusForbidden:
		return false, nil
	default:
		return false, fmt.Errorf("github membership check: status %d", resp.StatusCode)
	}
}

func (g *GitHubClient) authHeaders(req *http.Request, tok *oauth2.Token) {
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
}
