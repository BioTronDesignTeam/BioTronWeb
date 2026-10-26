package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/cache"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

// storedSuperuser is a store whose every operator row says is_superuser, as
// one would after its id was removed from SUPERUSER_GITHUB_IDS but before it
// signed in again.
type storedSuperuser struct{ fakeOperators }

func (storedSuperuser) GetOperator(_ context.Context, githubID int64) (*store.OrgMember, error) {
	return &store.OrgMember{GitHubID: githubID, Login: "octo", IsSuperuser: true}, nil
}

// SUPERUSER_GITHUB_IDS decides on every request, whatever the stored flag
// says, so removing an id and redeploying applies to the next request.
func TestSuperuserComesFromTheConfiguredList(t *testing.T) {
	github := &fakeGitHubMembers{members: map[string]bool{"octo": true}}
	for _, tc := range []struct {
		name   string
		listed map[int64]bool
		want   bool
	}{
		{"listed", map[int64]bool{42: true}, true},
		{"removed from the list", map[int64]bool{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{
				Cache:   cache.New(storedSuperuser{}),
				Members: NewMembership(github.IsOrgMember),
				Signer:  NewSigner(testSigningKey),
				Cfg:     Config{IsSuperuserID: func(id int64) bool { return tc.listed[id] }},
			}
			value, err := h.Signer.Session(Session{GitHubID: 42}, time.Now().Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
			request.AddCookie(&http.Cookie{Name: SessionCookie, Value: value})
			var got *store.SessionOperator
			h.RequireSession(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = OperatorFrom(r)
			})).ServeHTTP(httptest.NewRecorder(), request)
			if got == nil {
				t.Fatal("the session was refused")
			}
			if got.IsSuperuser != tc.want {
				t.Fatalf("IsSuperuser = %v, want %v", got.IsSuperuser, tc.want)
			}
		})
	}
}
