package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Authorizer interface {
	Authorize(ctx context.Context, cookieHeader string) (Decision, error)
}

type Decision struct {
	Authenticated bool
	Allowed       bool
}

type OAuthAuthorizer struct {
	endpoint string
	client   *http.Client
}

func NewOAuthAuthorizer(baseURL string) *OAuthAuthorizer {
	query := url.Values{"app": {"logger"}, "permission": {"view"}}
	return &OAuthAuthorizer{
		endpoint: baseURL + "/v1/check?" + query.Encode(),
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (a *OAuthAuthorizer) Authorize(ctx context.Context, cookieHeader string) (Decision, error) {
	if cookieHeader == "" {
		return Decision{}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint, nil)
	if err != nil {
		return Decision{}, err
	}
	req.Header.Set("Cookie", cookieHeader)

	response, err := a.client.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("check OAuth permission: %w", err)
	}
	defer response.Body.Close()
	// 401 and 403 mean different things and must not be collapsed. A signed-in
	// user who simply lacks logger/view is authenticated, and telling the portal
	// otherwise sends them back to the sign-in button that just worked, forever.
	if response.StatusCode == http.StatusUnauthorized {
		return Decision{Authenticated: false, Allowed: false}, nil
	}
	if response.StatusCode == http.StatusForbidden {
		return Decision{Authenticated: true, Allowed: false}, nil
	}
	if response.StatusCode != http.StatusOK {
		return Decision{}, fmt.Errorf("OAuth permission check returned HTTP %d", response.StatusCode)
	}

	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return Decision{}, fmt.Errorf("decode OAuth permission check: %w", err)
	}
	return Decision{Authenticated: true, Allowed: result.Allowed}, nil
}

type AllowAll struct{}

func (AllowAll) Authorize(context.Context, string) (Decision, error) {
	return Decision{Authenticated: true, Allowed: true}, nil
}
