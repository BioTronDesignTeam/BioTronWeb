package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/cache"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// guestKeys is a store whose daily keys the test changes, as a rotation or
// turning daily keys off would.
type guestKeys struct {
	fakeOperators
	mu  sync.Mutex
	key string
	err error
}

func (g *guestKeys) GetGuestKey(context.Context, string, string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return "", g.err
	}
	if g.key == "" {
		return "", store.ErrNotFound
	}
	return g.key, nil
}

func (g *guestKeys) set(key string, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.key, g.err = key, err
}

// A guest session names the key it signed in with, and every request checks
// it is still the tool's key: a rotation, or the tool no longer using daily
// keys, ends the session at its next request instead of at midnight.
func TestGuestSessionEndsWhenItsKeyIsRotatedOrWithdrawn(t *testing.T) {
	src := &guestKeys{key: testGuestKey}
	h := &Handler{Cache: cache.New(src), Signer: NewSigner(testSigningKey)}
	status := func(request *http.Request) int {
		t.Helper()
		response := httptest.NewRecorder()
		h.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(response, request)
		return response.Code
	}

	if got := status(guestSession(t, h)); got != http.StatusOK {
		t.Fatalf("a guest with today's key = %d, want 200", got)
	}

	src.set("ROTATEDKEY01", nil)
	h.Cache.InvalidateGuestKeys("exo-gui")
	if got := status(guestSession(t, h)); got != http.StatusUnauthorized {
		t.Fatalf("a guest after a rotation = %d, want 401", got)
	}

	src.set("", nil)
	h.Cache.InvalidateGuestKeys("exo-gui")
	if got := status(guestSession(t, h)); got != http.StatusUnauthorized {
		t.Fatalf("a guest after daily keys were turned off = %d, want 401", got)
	}

	src.set("", errors.New("database down"))
	h.Cache.InvalidateGuestKeys("exo-gui")
	if got := status(guestSession(t, h)); got != http.StatusServiceUnavailable {
		t.Fatalf("a guest when the store cannot answer = %d, want 503", got)
	}

	// A guest cookie from before sessions named their key is refused, so its
	// holder signs in again with today's key.
	src.set(testGuestKey, nil)
	h.Cache.InvalidateGuestKeys("exo-gui")
	legacy, err := h.Signer.sign(&sessionClaims{
		GuestApp:         "exo-gui",
		RegisteredClaims: h.Signer.registered("0", sessionAudience, time.Now().Add(time.Hour)),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/auth/me?app=exo-gui", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: legacy})
	if got := status(request); got != http.StatusUnauthorized {
		t.Fatalf("a guest cookie without its key = %d, want 401", got)
	}
}

func TestGuestKeyIDDependsOnEveryPart(t *testing.T) {
	s := NewSigner(testSigningKey)
	base := s.GuestKeyID("exo-gui", "2026-10-23", "ABCDEFGHIJKL")
	for _, other := range []string{
		s.GuestKeyID("calendar", "2026-10-23", "ABCDEFGHIJKL"),
		s.GuestKeyID("exo-gui", "2026-10-24", "ABCDEFGHIJKL"),
		s.GuestKeyID("exo-gui", "2026-10-23", "ABCDEFGHIJKM"),
		NewSigner([]byte("another-signing-key-of-at-least-32-bytes")).GuestKeyID("exo-gui", "2026-10-23", "ABCDEFGHIJKL"),
	} {
		if other == base {
			t.Fatal("two different keys share one GuestKeyID")
		}
	}
}
