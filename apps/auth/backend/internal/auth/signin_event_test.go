package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

// event is one delivery to Logger's ingest route, decoded as Logger receives it.
type event struct {
	Service string         `json:"service"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Payload map[string]any `json:"payload"`
}

// fakeLogger stands in for Logger and hands each delivery to the test.
func fakeLogger(t *testing.T) chan event {
	t.Helper()
	received := make(chan event, 4)
	logger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e event
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			t.Errorf("decode event: %v", err)
		}
		received <- e
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(logger.Close)
	t.Setenv("LOGGER_URL", logger.URL)
	t.Setenv("LOGGER_INGEST_TOKEN", "test-token")
	return received
}

// A refused sign-in leaves no other trace: the operator is never stored, and
// the browser only sees ?auth=denied. This event is the whole record, so its
// message and its reason are pinned here.
func TestSignInRefusedIsReportedWhenTheCallerIsNotAnOrgMember(t *testing.T) {
	received := fakeLogger(t)

	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test","token_type":"bearer"}`))
		default:
			// GitHub answers 404 for the membership of a caller who is not in
			// the org, which the client reports as "not a member" and not as a
			// failure of the handshake.
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer github.Close()

	h := &Handler{
		GitHub: NewGitHubClient(GitHubOptions{
			ClientID:     "id",
			ClientSecret: "secret",
			Org:          "BioTronDesignTeam",
			APIBase:      github.URL,
			TokenURL:     github.URL + "/token",
		}),
		Events: logclient.NewFromEnv("oauth-manager"),
		Cfg:    Config{FrontendURL: "http://localhost:5173"},
	}
	app := fiber.New()
	app.Get("/auth/github/callback", h.Callback)

	request := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=s&code=c", nil)
	request.AddCookie(&http.Cookie{Name: stateCookie, Value: "s"})
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusFound {
		t.Fatalf("callback status = %d, want 302 back to the frontend", response.StatusCode)
	}

	select {
	case e := <-received:
		if e.Message != "Sign-in refused" {
			t.Errorf("message = %q, want %q", e.Message, "Sign-in refused")
		}
		if e.Level != string(logclient.Warning) {
			t.Errorf("level = %q, want warning: a refusal is not a failure of the service", e.Level)
		}
		if e.Service != "oauth-manager" {
			t.Errorf("service = %q, want oauth-manager", e.Service)
		}
		if e.Payload["reason"] != "not a member" {
			t.Errorf("payload reason = %v, want %q", e.Payload["reason"], "not a member")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event reached Logger; a refused sign-in went unrecorded")
	}
}
