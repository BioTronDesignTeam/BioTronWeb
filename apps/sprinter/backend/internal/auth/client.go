// Package auth asks Auth who is behind a session cookie and whether they may
// administer Sprinter.
//
// Auth owns identity and per-product permissions. Sprinter asks two questions
// per admin request — who are you, and may you — because the log has to name
// the operator behind a guard or automation change, not just report that one
// happened.
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

// AppID is Sprinter's application id in Auth. PermissionAdmin is the one
// permission it declares: Sprinter has no public routes and no reader tier.
const (
	AppID           = "sprinter"
	PermissionAdmin = "admin"
)

var (
	ErrUnauthenticated = errors.New("not authenticated")
	ErrForbidden       = errors.New("admin permission required")
)

// Operator is the signed-in person, as Auth describes them.
type Operator struct {
	GitHubID    int64  `json:"github_id"`
	Login       string `json:"login"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	IsSuperuser bool   `json:"is_superuser"`
	IsManager   bool   `json:"is_manager"`
	IsStaff     bool   `json:"is_staff"`
}

// Status is what a browser needs to draw the admin UI: who is signed in, and
// whether they may change anything.
type Status struct {
	Operator *Operator `json:"operator,omitempty"`
	CanAdmin bool      `json:"can_admin"`
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

// Status resolves the session. An absent or rejected cookie is an empty status
// and no error, which the caller turns into 401. Anything Sprinter cannot
// interpret — a transport failure, an unexpected status — is an error, and the
// caller must answer 503. It must never be read as a pass.
func (c *Client) Status(ctx context.Context, cookie string) (Status, error) {
	if strings.TrimSpace(cookie) == "" {
		return Status{}, nil
	}
	var operator Operator
	status, err := c.getJSON(ctx, "/auth/me?app="+AppID, cookie, &operator)
	if err != nil {
		return Status{}, err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return Status{}, nil
	}
	if status != http.StatusOK {
		return Status{}, fmt.Errorf("Auth /auth/me returned %d", status)
	}
	allowed, err := c.CanAdmin(ctx, cookie)
	// A signed-in operator without the permission is a valid answer, not a
	// failure: the UI shows them the sign-in state they already have.
	if errors.Is(err, ErrForbidden) {
		return Status{Operator: &operator}, nil
	}
	if err != nil {
		return Status{}, err
	}
	return Status{Operator: &operator, CanAdmin: allowed}, nil
}

func (c *Client) CanAdmin(ctx context.Context, cookie string) (bool, error) {
	if strings.TrimSpace(cookie) == "" {
		return false, ErrUnauthenticated
	}
	query := url.Values{"app": {AppID}, "permission": {PermissionAdmin}}
	var response struct {
		Allowed bool `json:"allowed"`
	}
	status, err := c.getJSON(ctx, "/v1/check?"+query.Encode(), cookie, &response)
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusOK:
		if !response.Allowed {
			return false, ErrForbidden
		}
		return true, nil
	case http.StatusUnauthorized:
		return false, ErrUnauthenticated
	case http.StatusForbidden:
		return false, ErrForbidden
	default:
		return false, fmt.Errorf("Auth /v1/check returned %d", status)
	}
}

func (c *Client) getJSON(ctx context.Context, path, cookie string, destination any) (int, error) {
	if c.baseURL == "" {
		return 0, errors.New("OAUTH_MANAGER_URL is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
			return response.StatusCode, err
		}
	}
	return response.StatusCode, nil
}
