package auth

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// Logger (and every other product) tells "signed out" apart from "signed in
// without the permission" purely from the status of /v1/check: 401 means there
// is no usable session, 200 carries the boolean verdict. These tests pin that
// contract so a future change cannot quietly turn a permission refusal into a
// sign-in loop on the other side of the wire.

func TestCheckAnswersUnauthorizedWithoutSession(t *testing.T) {
	app := fiber.New()
	app.Get("/v1/check", (&Handler{}).RequireSession, (&Handler{}).Check)

	response, err := app.Test(httptest.NewRequest("GET", "/v1/check?app=logger&permission=view", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 so callers can tell signed-out from unpermitted", response.StatusCode)
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
			wantStatus: fiber.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "exo guests are authenticated but never hold logger view",
			operator:   &store.SessionOperator{GitHubID: store.GuestGitHubID, Login: store.GuestLogin, GuestAppID: "exo-gui"},
			query:      "app=logger&permission=view",
			wantStatus: fiber.StatusOK,
			wantAllow:  false,
		},
		{
			name:       "exo guests keep the telemetry permissions their key grants",
			operator:   &store.SessionOperator{GitHubID: store.GuestGitHubID, Login: store.GuestLogin, GuestAppID: "exo-gui"},
			query:      "app=exo-gui&permission=historical",
			wantStatus: fiber.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "missing query parameters are a bad request, never a silent refusal",
			operator:   &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true},
			query:      "app=logger",
			wantStatus: fiber.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// RequireSession is covered above; here the operator it would have
			// loaded is injected directly so the verdict needs no database.
			app := fiber.New()
			app.Get("/v1/check", func(c fiber.Ctx) error {
				c.Locals(OperatorLocal, tc.operator)
				return h.Check(c)
			})

			response, err := app.Test(httptest.NewRequest("GET", "/v1/check?"+tc.query, nil))
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.wantStatus)
			}
			if tc.wantStatus != fiber.StatusOK {
				return
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
