package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

// A grant for someone who has never signed in is 404 with a reason, not the
// foreign key's 500.
func TestLiveGrantForAnUnknownOperatorIs404(t *testing.T) {
	h, _ := liveHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/grants",
		strings.NewReader(`{"operator_id":9000000999,"app_id":"calendar","permission_key":"write"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.CreateGrant(response, withOperator(request, &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true}))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "never signed in") {
		t.Fatalf("grant for an unknown operator = %d %s, want 404", response.Code, response.Body)
	}
}
