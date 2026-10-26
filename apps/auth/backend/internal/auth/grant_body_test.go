package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A grant names a GitHub id, which is always positive; the guest account (0)
// and negative ids are refused before the store is asked.
func TestGrantBodyRefusesTheGuestAndNegativeIDs(t *testing.T) {
	h := &Handler{}
	for _, body := range []string{
		`{"operator_id":0,"app_id":"exo-gui","permission_key":"commands"}`,
		`{"operator_id":-1,"app_id":"exo-gui","permission_key":"commands"}`,
		`{"app_id":"exo-gui","permission_key":"commands"}`,
	} {
		for _, handler := range []http.HandlerFunc{h.CreateGrant, h.DeleteGrant} {
			request := httptest.NewRequest(http.MethodPost, "/grants", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != http.StatusBadRequest {
				t.Errorf("%s = %d, want 400", body, response.Code)
			}
		}
	}
}
