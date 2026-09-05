package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"

	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

// noTestTimeout is v3's spelling of v2's app.Test(request, -1): let the handler
// run to completion instead of failing it after Fiber's one-second default.
var noTestTimeout = fiber.TestConfig{Timeout: 0}

type recordedEvent struct {
	level   logclient.Level
	message string
	payload map[string]any
}

// eventCollector stands in for the recorder that writes Logger's own events
// into its store. fiberlog calls it before the request returns, so the test
// reads the slice without waiting.
type eventCollector struct {
	events []recordedEvent
}

func (c *eventCollector) LogAsync(level logclient.Level, message string, payload any) {
	fields, _ := payload.(map[string]any)
	c.events = append(c.events, recordedEvent{level: level, message: message, payload: fields})
}

type fakeStore struct {
	inserted         model.NewLog
	health           map[string]model.Health
	history          map[string][]model.HealthPoint
	historyCalls     int
	historyTolerance time.Duration
}

func (f *fakeStore) Ping(context.Context) error { return nil }
func (f *fakeStore) InsertLog(_ context.Context, input model.NewLog) (model.Log, error) {
	f.inserted = input
	return model.Log{ID: "1", Service: input.Service, Level: input.Level, Message: input.Message, Payload: input.Payload, CreatedAt: time.Now()}, nil
}
func (f *fakeStore) RecentLogs(context.Context, []string, []model.LogLevel, string, int) ([]model.Log, error) {
	return []model.Log{}, nil
}
func (f *fakeStore) QueryLogs(context.Context, model.HistoryQuery) (model.LogPage, error) {
	return model.LogPage{Logs: []model.Log{}}, nil
}
func (f *fakeStore) LatestHealth(context.Context, []string) (map[string]model.Health, error) {
	return f.health, nil
}
func (f *fakeStore) HealthHistory(_ context.Context, _ []string, _, _ time.Time, gapTolerance time.Duration) (map[string][]model.HealthPoint, error) {
	f.historyCalls++
	f.historyTolerance = gapTolerance
	return f.history, nil
}

// The ingest token is static, shared by every sender and internet-reachable, so
// the route needs a ceiling of its own: without one a single leaked token can
// forge unbounded audit entries or fill the shared Postgres. The limit is per
// sender address, so one noisy service cannot silence the others.
func TestIngestIsRateLimitedPerSender(t *testing.T) {
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	app := New(&fakeStore{}, serviceCatalog, auth.AllowAll{}, Options{
		IngestToken:     "secret",
		IngestRateLimit: 2,
		// app.Test dials from 0.0.0.0, standing in for the edge proxy.
		TrustedProxies: []string{"0.0.0.0"},
	})
	body := []byte(`{"service":"exo-api","level":"info","message":"telemetry batch stored"}`)

	post := func(senderIP, token string) int {
		request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Cf-Connecting-Ip", senderIP)
		response, err := app.Test(request, noTestTimeout)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}

	if got := post("203.0.113.7", "secret"); got != http.StatusCreated {
		t.Fatalf("first ingest = %d, want 201", got)
	}
	// The throttle sits ahead of the token check, so wrong-token floods are
	// charged to the same bucket rather than being free.
	if got := post("203.0.113.7", "wrong"); got != http.StatusUnauthorized {
		t.Fatalf("second ingest with a bad token = %d, want 401", got)
	}
	if got := post("203.0.113.7", "secret"); got != http.StatusTooManyRequests {
		t.Fatalf("third ingest = %d, want 429", got)
	}
	if got := post("198.51.100.4", "secret"); got != http.StatusCreated {
		t.Fatalf("ingest from a second sender = %d, want 201; the limit is one global bucket", got)
	}
}

func TestIngestRequiresTokenAndAcceptsStructuredLog(t *testing.T) {
	dataStore := &fakeStore{}
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	app := New(dataStore, serviceCatalog, auth.AllowAll{}, Options{IngestToken: "secret"})
	body := []byte(`{"service":"exo-api","level":"warning","message":"temperature high","payload":{"celsius":73}}`)

	request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, noTestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", response.StatusCode)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer secret")
	response, err = app.Test(request, noTestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("created status = %d", response.StatusCode)
	}
	if dataStore.inserted.Service != "exo-api" || dataStore.inserted.Level != model.LevelWarning {
		t.Fatalf("unexpected inserted log: %#v", dataStore.inserted)
	}
}

func TestApplicationsAggregateComponentHealth(t *testing.T) {
	now := time.Now()
	dataStore := &fakeStore{health: map[string]model.Health{
		"site-web": {Service: "site-web", OK: true, CheckedAt: now},
	}}
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	app := New(dataStore, serviceCatalog, auth.AllowAll{}, Options{IngestToken: "secret"})

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/apps", nil), noTestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var payload struct {
		Applications []applicationStatus `json:"applications"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Applications) == 0 || payload.Applications[0].State != "healthy" {
		t.Fatalf("unexpected application status: %#v", payload.Applications)
	}
}

func TestHistoryRejectsUnknownLevel(t *testing.T) {
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	app := New(&fakeStore{}, serviceCatalog, auth.AllowAll{}, Options{IngestToken: "secret"})
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/apps/exo/logs/history?levels=critical", nil), noTestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

// The request log must not become a way to write into the audit trail. Ingest
// faces the internet under one shared token, so a request refused for a bad
// token or turned away by the limiter records nothing: an attacker would
// otherwise fill the warehouse at the rate limit. An accepted event is quiet
// too, or every stored event would cost a second row describing the request
// that carried it. A 400 does record, because it says one of our own services
// is sending what this one cannot store.
func TestIngestRequestLogRecordsOnlyOurOwnMistakes(t *testing.T) {
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	events := &eventCollector{}
	app := New(&fakeStore{}, serviceCatalog, auth.AllowAll{}, Options{
		IngestToken:     "secret",
		IngestRateLimit: 2,
		// app.Test dials from 0.0.0.0, standing in for the edge proxy.
		TrustedProxies: []string{"0.0.0.0"},
		Events:         events,
	})

	post := func(senderIP, token, body string) int {
		request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader([]byte(body)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Cf-Connecting-Ip", senderIP)
		response, err := app.Test(request, noTestTimeout)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}

	accepted := `{"service":"exo-api","level":"info","message":"telemetry batch stored"}`
	if got := post("198.51.100.4", "secret", accepted); got != http.StatusCreated {
		t.Fatalf("accepted ingest = %d, want 201", got)
	}
	if got := post("203.0.113.7", "secret", `{"service":"nope","level":"info","message":"x"}`); got != http.StatusBadRequest {
		t.Fatalf("unknown service = %d, want 400", got)
	}
	if got := post("203.0.113.7", "wrong", accepted); got != http.StatusUnauthorized {
		t.Fatalf("bad token = %d, want 401", got)
	}
	if got := post("203.0.113.7", "secret", accepted); got != http.StatusTooManyRequests {
		t.Fatalf("third request from one address = %d, want 429", got)
	}

	if len(events.events) != 1 {
		t.Fatalf("events = %#v, want only the 400", events.events)
	}
	event := events.events[0]
	if event.message != "HTTP request completed" || event.level != logclient.Warning {
		t.Fatalf("event = %#v, want the request log at warning", event)
	}
	if event.payload["path"] != "/v1/logs" || event.payload["status"] != http.StatusBadRequest {
		t.Fatalf("payload = %#v, want the ingest path and 400", event.payload)
	}
	if event.payload["error"] != "unknown service" {
		t.Fatalf("payload = %#v, want the cause the sender was given", event.payload)
	}
}
