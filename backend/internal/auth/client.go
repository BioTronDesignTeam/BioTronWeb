// Package auth is Exo's server-side authorization gate.
//
// OAuthManager owns identity and per-product permissions; Exo asks it a single
// question per request: may the operator behind this session cookie use this
// permission of this app? The answer comes from
// GET {OAUTH_MANAGER_URL}/v1/check?app=exo-gui&permission=<key>, the same
// contract Logger and BiotronCalendar use.
//
// Exo is the only product with a guest tier, so the invariant this package
// exists to hold is: a daily guest may use Live and Historical, never Commands.
// It is already enforced inside OAuthManager (store.Allowed refuses `commands`
// for a guest), but that check only runs if something calls it — which, until
// this package existed, nothing in Exo did.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AppID is Exo's application id in OAuthManager. It must match the row in
// OAuthManager's `apps` table.
const AppID = "exo-gui"

// The permission keys Exo declares. A daily guest key grants Live and
// Historical only.
const (
	PermissionLive       = "live"
	PermissionHistorical = "historical"
	PermissionCommands   = "commands"
)

// ErrNotConfigured is returned when OAUTH_MANAGER_URL is unset. Callers must
// treat it as a denial, never as a pass.
var ErrNotConfigured = errors.New("OAUTH_MANAGER_URL is not configured")

// Decision separates "who are you" from "may you". They must not be collapsed:
// telling an operator who is signed in but lacks a permission that they are
// unauthenticated sends them back to a sign-in button that already worked.
type Decision struct {
	Authenticated bool
	Allowed       bool
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Configured reports whether the client can actually reach OAuthManager.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != ""
}

// Check asks OAuthManager whether the session in cookieHeader may use
// permission on Exo.
//
// The /v1/check route sits behind OAuthManager's RequireSession, so today it
// answers 401 for a missing, expired or banned session and 200 {"allowed":bool}
// otherwise — it does not currently emit 403. 403 is handled anyway, as
// authenticated-but-unpermitted, so Exo keeps working if OAuthManager ever
// grows stricter middleware in front of that route.
func (c *Client) Check(ctx context.Context, cookieHeader, permission string) (Decision, error) {
	if !c.Configured() {
		return Decision{}, ErrNotConfigured
	}
	if strings.TrimSpace(cookieHeader) == "" {
		// No session material at all: no point spending a network call, and no
		// way this is anything but unauthenticated.
		return Decision{}, nil
	}

	query := url.Values{"app": {AppID}, "permission": {permission}}
	endpoint := c.baseURL + "/v1/check?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Decision{}, err
	}
	request.Header.Set("Cookie", cookieHeader)
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return Decision{}, fmt.Errorf("check Exo permission %q: %w", permission, err)
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
		var result struct {
			Allowed bool `json:"allowed"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			return Decision{}, fmt.Errorf("decode Exo permission check: %w", err)
		}
		return Decision{Authenticated: true, Allowed: result.Allowed}, nil
	case http.StatusUnauthorized:
		return Decision{Authenticated: false, Allowed: false}, nil
	case http.StatusForbidden:
		return Decision{Authenticated: true, Allowed: false}, nil
	default:
		return Decision{}, fmt.Errorf("OAuthManager /v1/check returned HTTP %d", response.StatusCode)
	}
}
