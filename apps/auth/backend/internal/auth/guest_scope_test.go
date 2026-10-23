package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

func TestMeRequiresMatchingProductForGuest(t *testing.T) {
	h := &Handler{}
	guest := &store.SessionOperator{
		GitHubID:   store.GuestGitHubID,
		Login:      store.GuestLogin,
		GuestAppID: "exo-gui",
	}

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/me?app=exo-gui", want: http.StatusOK},
		{path: "/me?app=calendar", want: http.StatusForbidden},
		{path: "/me", want: http.StatusForbidden},
	} {
		response := httptest.NewRecorder()
		h.Me(response, withOperator(httptest.NewRequest("GET", tc.path, nil), guest))
		if response.Code != tc.want {
			t.Errorf("GET %s status = %d, want %d", tc.path, response.Code, tc.want)
		}
	}
}
