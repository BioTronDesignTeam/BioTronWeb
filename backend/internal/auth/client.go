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

	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

var (
	ErrUnauthenticated = errors.New("not authenticated")
	ErrForbidden       = errors.New("write permission required")
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Status struct {
	Operator *model.Operator `json:"operator,omitempty"`
	CanWrite bool            `json:"can_write"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) Status(ctx context.Context, cookie string) (Status, error) {
	if strings.TrimSpace(cookie) == "" {
		return Status{}, nil
	}
	var operator model.Operator
	status, err := c.getJSON(ctx, "/auth/me?app=calendar", cookie, &operator)
	if err != nil {
		return Status{}, err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return Status{}, nil
	}
	if status != http.StatusOK {
		return Status{}, fmt.Errorf("OAuthManager /auth/me returned %d", status)
	}
	allowed, err := c.CanWrite(ctx, cookie)
	if errors.Is(err, ErrForbidden) {
		return Status{Operator: &operator}, nil
	}
	if err != nil {
		return Status{}, err
	}
	return Status{Operator: &operator, CanWrite: allowed}, nil
}

func (c *Client) CanWrite(ctx context.Context, cookie string) (bool, error) {
	if strings.TrimSpace(cookie) == "" {
		return false, ErrUnauthenticated
	}
	query := url.Values{"app": {"calendar"}, "permission": {"write"}}
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
		return false, fmt.Errorf("OAuthManager /v1/check returned %d", status)
	}
}

func (c *Client) getJSON(ctx context.Context, path, cookie string, destination any) (int, error) {
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
