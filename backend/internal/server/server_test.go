package server

import (
	"net/http/httptest"
	"testing"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
)

func TestHealth(t *testing.T) {
	app := New("http://localhost:5173", []string{"127.0.0.1"}, nil, auth.NewClient("http://oauth-manager:8080"))
	resp, err := app.Test(httptest.NewRequest("GET", "/health", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("health = %d, want 200", resp.StatusCode)
	}
}
