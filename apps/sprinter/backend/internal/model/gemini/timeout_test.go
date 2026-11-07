package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model"
)

// A Gemini request that never answers must still end, even when the caller's
// context has no deadline: the scheduler drafts nudges on a context that runs
// until shutdown. The HTTP client's own timeout is what ends it.
func TestAStalledRequestTimesOutWithoutAContextDeadline(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	m, err := newModel(context.Background(), "key", "gemini-2.5-flash",
		&http.Client{Timeout: 100 * time.Millisecond}, server.URL)
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := m.Generate(context.Background(), "", []model.Message{{Role: model.RoleUser, Text: "hi"}}, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a stalled request returned no error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Generate was still waiting on a stalled request")
	}
}
