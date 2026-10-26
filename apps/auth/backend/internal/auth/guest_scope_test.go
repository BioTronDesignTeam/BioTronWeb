package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
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
		h.Me(response, withOperator(httptest.NewRequest(http.MethodGet, tc.path, nil), guest))
		if response.Code != tc.want {
			t.Errorf("GET %s status = %d, want %d", tc.path, response.Code, tc.want)
		}
	}
}

func TestGuestAccessIsScopedToProductTelemetry(t *testing.T) {
	h := &Handler{}
	guest := &store.SessionOperator{GitHubID: store.GuestGitHubID, GuestAppID: "exo-gui"}

	for _, permission := range []string{"live", "historical"} {
		allowed, err := h.allowed(context.Background(), guest, "exo-gui", permission)
		if err != nil || !allowed {
			t.Fatalf("expected Exo guest %s to be allowed, allowed=%v err=%v", permission, allowed, err)
		}
	}

	for _, tc := range []struct {
		appID      string
		permission string
	}{
		{appID: "calendar", permission: "live"},
		{appID: "calendar", permission: "write"},
		{appID: "exo-gui", permission: "commands"},
		{appID: "sprinter", permission: "admin"},
	} {
		allowed, err := h.allowed(context.Background(), guest, tc.appID, tc.permission)
		if err != nil {
			t.Fatalf("allowed(%q, %q): %v", tc.appID, tc.permission, err)
		}
		if allowed {
			t.Errorf("guest unexpectedly allowed %s/%s", tc.appID, tc.permission)
		}
	}
}
