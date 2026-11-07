package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/cache"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
)

// failingOperators answers every operator lookup with err.
type failingOperators struct {
	fakeOperators
	err error
}

func (f failingOperators) GetOperator(context.Context, int64) (*store.OrgMember, error) {
	return nil, f.err
}

// A valid cookie of an operator whose row is gone is signed out (401); a
// store that cannot answer is an outage (503), never a pass.
func TestRequireSessionWhenTheOperatorCannotBeRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"operator row gone", store.ErrNotFound, http.StatusUnauthorized},
		{"store down", errors.New("database down"), http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{Cache: cache.New(failingOperators{err: tc.err}), Signer: NewSigner(testSigningKey)}
			value, err := h.Signer.Session(Session{GitHubID: 42}, time.Now().Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
			request.AddCookie(&http.Cookie{Name: SessionCookie, Value: value})
			response := httptest.NewRecorder()
			h.RequireSession(http.HandlerFunc(h.Me)).ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
		})
	}
}
