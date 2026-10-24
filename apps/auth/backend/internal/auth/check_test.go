package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// Logger (and every other product) tells "signed out" apart from "signed in
// without the permission" purely from the status of /v1/check: 401 means there
// is no usable session, 200 carries the boolean verdict. These tests pin that
// contract so a future change cannot quietly turn a permission refusal into a
// sign-in loop on the other side of the wire.

func TestCheckAnswersUnauthorizedWithoutSession(t *testing.T) {
	h := &Handler{}
	handler := h.RequireSession(http.HandlerFunc(h.Check))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/check?app=logger&permission=view", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 so callers can tell signed-out from unpermitted", response.Code)
	}
}

func TestCheckAnswersOKWithVerdictForValidSessions(t *testing.T) {
	h := &Handler{Store: &store.Store{}}

	for _, tc := range []struct {
		name       string
		operator   *store.SessionOperator
		query      string
		wantStatus int
		wantAllow  bool
	}{
		{
			name:       "staff hold every permission",
			operator:   &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true},
			query:      "app=logger&permission=view",
			wantStatus: http.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "exo guests are authenticated but never hold logger view",
			operator:   &store.SessionOperator{GitHubID: store.GuestGitHubID, Login: store.GuestLogin, GuestAppID: "exo-gui"},
			query:      "app=logger&permission=view",
			wantStatus: http.StatusOK,
			wantAllow:  false,
		},
		{
			name:       "exo guests keep the telemetry permissions their key grants",
			operator:   &store.SessionOperator{GitHubID: store.GuestGitHubID, Login: store.GuestLogin, GuestAppID: "exo-gui"},
			query:      "app=exo-gui&permission=historical",
			wantStatus: http.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "missing query parameters are a bad request, never a silent refusal",
			operator:   &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true},
			query:      "app=logger",
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// RequireSession is covered above; here the operator it would have
			// loaded is injected directly so the verdict needs no database.
			request := withOperator(httptest.NewRequest(http.MethodGet, "/v1/check?"+tc.query, nil), tc.operator)
			response := httptest.NewRecorder()
			h.Check(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			var body struct {
				Allowed bool `json:"allowed"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Allowed != tc.wantAllow {
				t.Fatalf("allowed = %v, want %v", body.Allowed, tc.wantAllow)
			}
		})
	}
}
