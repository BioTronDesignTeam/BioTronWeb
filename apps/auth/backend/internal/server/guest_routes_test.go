package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/auth"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/cache"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

const guestKey = "GUESTKEY0001"

// guestStore knows only the guest account and Exo's daily key.
type guestStore struct{}

func (guestStore) GetOperator(_ context.Context, githubID int64) (*store.OrgMember, error) {
	if githubID != store.GuestGitHubID {
		return nil, store.ErrNotFound
	}
	return &store.OrgMember{GitHubID: githubID, Login: store.GuestLogin}, nil
}

func (guestStore) ListGrantsForOperator(context.Context, int64) ([]store.Grant, error) {
	return nil, nil
}

func (guestStore) GetGuestKey(context.Context, string, string) (string, error) {
	return guestKey, nil
}

// A guest session reaches /auth/me and /v1/check for its own tool and
// nothing of the catalog or the grants, which only the Auth UI reads.
func TestGuestsAreRefusedTheCatalogAndGrants(t *testing.T) {
	signer := auth.NewSigner([]byte("test-signing-key-that-is-at-least-32-bytes"))
	h := &auth.Handler{Cache: cache.New(guestStore{}), Signer: signer}
	app := New(h, []string{allowedOrigin}, nil, nil)
	const day = "2026-10-23"
	value, err := signer.Session(auth.Session{
		GitHubID:   store.GuestGitHubID,
		GuestAppID: "exo-gui",
		GuestDay:   day,
		GuestKeyID: signer.GuestKeyID("exo-gui", day, guestKey),
	}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]int{
		"/apps":                                 http.StatusForbidden,
		"/permissions":                          http.StatusForbidden,
		"/me/grants":                            http.StatusForbidden,
		"/auth/me?app=exo-gui":                  http.StatusOK,
		"/v1/check?app=exo-gui&permission=live": http.StatusOK,
		"/v1/session?app=exo-gui":               http.StatusOK,
		"/v1/session?app=calendar":              http.StatusForbidden,
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: value})
		if got := serve(app, request).Code; got != want {
			t.Errorf("guest GET %s = %d, want %d", target, got, want)
		}
	}
}
