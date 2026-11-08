package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

// The Auth page shows these refusals as they are, so they read as sentences
// in the glossary's words, not as log lines.
func TestRefusalsTheAuthPageShows(t *testing.T) {
	h := &Handler{}
	manager := &store.SessionOperator{GitHubID: 7, Login: "manager", IsManager: true}
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		method  string
		id      string
		body    string
		want    string
	}{
		{"ban yourself", h.BanMember, http.MethodPost, "7", "", `{"error":"You can't ban yourself."}`},
		{"ban the guest", h.BanMember, http.MethodPost, "0", "", `{"error":"Guests can't be banned. Rotate the Daily guest key to sign them out."}`},
		{"change your own role", h.SetManager, http.MethodPatch, "7", `{"manager":false}`, `{"error":"You can't change your own Manager role."}`},
		{"make the guest a manager", h.SetManager, http.MethodPatch, "0", `{"manager":true}`, `{"error":"Guests can't be made managers."}`},
	} {
		request := httptest.NewRequest(tc.method, "/org/members/"+tc.id, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		request.SetPathValue("id", tc.id)
		response := httptest.NewRecorder()
		tc.handler(response, withOperator(request, manager))
		if response.Code != http.StatusBadRequest || response.Body.String() != tc.want {
			t.Errorf("%s: %d %s, want 400 %s", tc.name, response.Code, response.Body, tc.want)
		}
	}
}
