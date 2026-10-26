package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/cache"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// /v1/session is /auth/me and /v1/check in one call. Calendar, Sprinter and
// Exo read it through go/authclient, which tells "signed out" (401, or 403
// for a guest of another tool) from "signed in" (200) by its status alone.

// grantStore holds one member, 9, with calendar/write.
type grantStore struct{}

func (grantStore) GetOperator(context.Context, int64) (*store.OrgMember, error) {
	return nil, store.ErrNotFound
}

func (grantStore) ListGrantsForOperator(_ context.Context, operatorID int64) ([]store.Grant, error) {
	if operatorID != 9 {
		return nil, nil
	}
	return []store.Grant{{OperatorID: 9, AppID: "calendar", PermissionKey: "write"}}, nil
}

func (grantStore) GetGuestKey(context.Context, string, string) (string, error) {
	return "", store.ErrNotFound
}

func TestSessionAnswersUnauthorizedWithoutSession(t *testing.T) {
	h := &Handler{}
	response := httptest.NewRecorder()
	h.RequireSession(http.HandlerFunc(h.Session)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/session?app=calendar&permission=write", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestSessionAnswersOperatorAndPermissions(t *testing.T) {
	h := &Handler{Cache: cache.New(grantStore{})}
	guest := &store.SessionOperator{GitHubID: store.GuestGitHubID, Login: store.GuestLogin, GuestAppID: "exo-gui"}

	for _, tc := range []struct {
		name       string
		operator   *store.SessionOperator
		query      string
		wantStatus int
		want       map[string]bool
	}{
		{
			name:       "a member holds what they were granted and nothing else",
			operator:   &store.SessionOperator{GitHubID: 9, Login: "ada"},
			query:      "app=calendar&permission=write&permission=admin",
			wantStatus: http.StatusOK,
			want:       map[string]bool{"write": true, "admin": false},
		},
		{
			name:       "managers hold every permission",
			operator:   &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true},
			query:      "app=sprinter&permission=admin",
			wantStatus: http.StatusOK,
			want:       map[string]bool{"admin": true},
		},
		{
			name:       "a guest holds live and historical of its own tool, never commands",
			operator:   guest,
			query:      "app=exo-gui&permission=live&permission=historical&permission=commands",
			wantStatus: http.StatusOK,
			want:       map[string]bool{"live": true, "historical": true, "commands": false},
		},
		{
			name:       "a guest is refused for another tool, as /auth/me refuses it",
			operator:   guest,
			query:      "app=calendar&permission=write",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "no permission asked returns the operator alone",
			operator:   &store.SessionOperator{GitHubID: 9, Login: "ada"},
			query:      "app=calendar",
			wantStatus: http.StatusOK,
			want:       map[string]bool{},
		},
		{
			name:       "a missing app is a bad request",
			operator:   &store.SessionOperator{GitHubID: 9, Login: "ada"},
			query:      "permission=write",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "an empty permission is a bad request",
			operator:   &store.SessionOperator{GitHubID: 9, Login: "ada"},
			query:      "app=calendar&permission=",
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := withOperator(httptest.NewRequest(http.MethodGet, "/v1/session?"+tc.query, nil), tc.operator)
			response := httptest.NewRecorder()
			h.Session(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", response.Code, tc.wantStatus, response.Body)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			var body struct {
				Operator    map[string]any  `json:"operator"`
				Permissions map[string]bool `json:"permissions"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Operator["login"] != tc.operator.Login || body.Operator["is_guest"] != tc.operator.IsGuest() {
				t.Fatalf("operator = %v", body.Operator)
			}
			if len(body.Permissions) != len(tc.want) {
				t.Fatalf("permissions = %v, want %v", body.Permissions, tc.want)
			}
			for key, want := range tc.want {
				if got, ok := body.Permissions[key]; !ok || got != want {
					t.Fatalf("permissions = %v, want %v", body.Permissions, tc.want)
				}
			}
		})
	}
}
