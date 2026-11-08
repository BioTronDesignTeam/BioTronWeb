// Package authclient asks Auth who is behind a session cookie and whether they
// hold a permission of one tool. Calendar, Sprinter and Exo all gate their
// routes with it, so the 401/403/503 mapping, the CSRF header check, and the
// fail-closed rule are written once.
//
// Auth owns identity, roles, and grants. A backend never decides on its own:
// every gated request asks Auth, with the caller's cookie, in one call to
// GET /v1/session. There is no cache. Auth promises that a ban or a revoke
// refuses the very next request, and a cache here would break that promise.
package authclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

// Timeout bounds each call to Auth.
const Timeout = 5 * time.Second

// maxBody is the most of an Auth answer that is read.
const maxBody = 1 << 20

var (
	// ErrNotConfigured is the outage of a client with no Auth URL. It is a
	// denial, never a pass.
	ErrNotConfigured = errors.New("OAUTH_MANAGER_URL is not configured")
	// ErrUnauthenticated is the cause Require records for a 401.
	ErrUnauthenticated = errors.New("authentication required")
	// ErrForbidden is the cause Require records for a 403 over a permission.
	ErrForbidden = errors.New("permission required")
	// ErrMissingXHR is the cause Require records for a mutating request
	// without the X-Requested-With header.
	ErrMissingXHR = errors.New("X-Requested-With header required")
)

// Operator is the signed-in person, as Auth describes them. A daily guest is
// an operator too, with IsGuest set and the tool its key opened.
type Operator struct {
	GitHubID    int64  `json:"github_id"`
	Login       string `json:"login"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	IsSuperuser bool   `json:"is_superuser"`
	IsManager   bool   `json:"is_manager"`
	IsStaff     bool   `json:"is_staff"`
	IsGuest     bool   `json:"is_guest"`
	GuestAppID  string `json:"guest_app_id"`
}

// Session is Auth's answer for one cookie. The zero value is anonymous: no
// cookie, or one Auth does not accept for this tool (expired, banned, no
// longer in the organization, or a guest of another tool).
type Session struct {
	Operator *Operator
	// Permissions holds a verdict for each permission asked about.
	Permissions map[string]bool
}

// Authenticated reports whether someone is signed in.
func (s Session) Authenticated() bool { return s.Operator != nil }

// Can reports whether the session holds permission. Only permissions passed
// to Lookup have a verdict; any other is false.
func (s Session) Can(permission string) bool {
	return s.Operator != nil && s.Permissions[permission]
}

// Client asks Auth about sessions for one tool.
type Client struct {
	baseURL string
	appID   string
	http    *http.Client
	events  *logclient.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default client, which times out after Timeout.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) { c.http = client }
}

// WithEvents sends an Error event whenever Auth cannot be reached, the one
// refusal an operator cannot diagnose from the response.
func WithEvents(events *logclient.Client) Option {
	return func(c *Client) { c.events = events }
}

// New returns a client for the tool appID, which must match its row in Auth's
// apps table. An empty baseURL is allowed: every gated request then fails
// closed with 503, and Configured reports false so the service can warn.
func New(baseURL, appID string, options ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		appID:   appID,
		http:    &http.Client{Timeout: Timeout},
	}
	for _, option := range options {
		option(c)
	}
	return c
}

// With returns a copy of the client with more options applied, so a server
// package can add its events to a client main built.
func (c *Client) With(options ...Option) *Client {
	copied := *c
	for _, option := range options {
		option(&copied)
	}
	return &copied
}

// AppID is the tool this client asks about.
func (c *Client) AppID() string { return c.appID }

// Configured reports whether the client has an Auth URL to ask.
func (c *Client) Configured() bool { return c != nil && c.baseURL != "" }

// Lookup asks Auth who is behind cookieHeader and which of permissions they
// hold on this tool. The whole Cookie header is forwarded, as the browser sent
// it.
//
// An absent or refused session is an anonymous Session and no error. An error
// means Auth could not answer: it is unreachable, not configured, or said
// something this client cannot read. The caller must answer 503 and never
// treat it as a pass.
func (c *Client) Lookup(ctx context.Context, cookieHeader string, permissions ...string) (Session, error) {
	if !c.Configured() {
		return Session{}, ErrNotConfigured
	}
	if strings.TrimSpace(cookieHeader) == "" {
		// No session material at all: nothing to ask Auth about.
		return Session{}, nil
	}
	query := url.Values{"app": {c.appID}}
	for _, permission := range permissions {
		query.Add("permission", permission)
	}
	var body struct {
		Operator    *Operator       `json:"operator"`
		Permissions map[string]bool `json:"permissions"`
	}
	status, err := c.getJSON(ctx, "/v1/session?"+query.Encode(), cookieHeader, &body)
	if err != nil {
		return Session{}, err
	}
	switch status {
	case http.StatusOK:
		if body.Operator == nil {
			return Session{}, errors.New("auth /v1/session answered without an operator")
		}
		return Session{Operator: body.Operator, Permissions: verdicts(permissions, body.Permissions)}, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		// 403 is a guest of another tool: signed in, but not here.
		return Session{}, nil
	case http.StatusNotFound:
		return c.lookupSeparately(ctx, cookieHeader, permissions)
	default:
		return Session{}, fmt.Errorf("auth /v1/session returned HTTP %d", status)
	}
}

// lookupSeparately answers Lookup from /auth/me and one /v1/check per
// permission, for an Auth older than /v1/session. It exists only so a tool
// deployed ahead of Auth keeps working, and can go once every Auth has the
// endpoint. A session that ends between the calls is anonymous, not an
// outage.
func (c *Client) lookupSeparately(ctx context.Context, cookieHeader string, permissions []string) (Session, error) {
	var operator Operator
	status, err := c.getJSON(ctx, "/auth/me?"+url.Values{"app": {c.appID}}.Encode(), cookieHeader, &operator)
	if err != nil {
		return Session{}, err
	}
	switch status {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return Session{}, nil
	default:
		return Session{}, fmt.Errorf("auth /auth/me returned HTTP %d", status)
	}
	held := make(map[string]bool, len(permissions))
	for _, permission := range permissions {
		var answer struct {
			Allowed bool `json:"allowed"`
		}
		query := url.Values{"app": {c.appID}, "permission": {permission}}
		status, err := c.getJSON(ctx, "/v1/check?"+query.Encode(), cookieHeader, &answer)
		if err != nil {
			return Session{}, err
		}
		switch status {
		case http.StatusOK:
			held[permission] = answer.Allowed
		case http.StatusUnauthorized:
			return Session{}, nil
		case http.StatusForbidden:
			held[permission] = false
		default:
			return Session{}, fmt.Errorf("auth /v1/check returned HTTP %d", status)
		}
	}
	return Session{Operator: &operator, Permissions: held}, nil
}

// verdicts keeps exactly the permissions asked about, each false unless Auth
// said true.
func verdicts(asked []string, answered map[string]bool) map[string]bool {
	held := make(map[string]bool, len(asked))
	for _, permission := range asked {
		held[permission] = answered[permission]
	}
	return held
}

// getJSON GETs path with the caller's cookie and decodes a 2xx body into
// destination. It returns the status; only a failure to ask or to read a 2xx
// answer is an error.
func (c *Client) getJSON(ctx context.Context, path, cookieHeader string, destination any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Cookie", cookieHeader)
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if err := json.NewDecoder(io.LimitReader(response.Body, maxBody)).Decode(destination); err != nil {
			return response.StatusCode, fmt.Errorf("decode auth %s: %w", strings.SplitN(path, "?", 2)[0], err)
		}
	}
	return response.StatusCode, nil
}
