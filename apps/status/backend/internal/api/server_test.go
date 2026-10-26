package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/catalog"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/model"
)

// serve runs one request through the API in process and returns what the
// client would receive.
func serve(app http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, request)
	return recorder
}

type recordedEvent struct {
	level   logclient.Level
	message string
	payload map[string]any
}

// eventCollector stands in for the event client. httplog calls it before the
// request returns, so the test reads the slice without waiting.
type eventCollector struct {
	events []recordedEvent
}

func (c *eventCollector) LogAsync(level logclient.Level, message string, payload any) {
	fields, _ := payload.(map[string]any)
	c.events = append(c.events, recordedEvent{level: level, message: message, payload: fields})
}

type fakeStore struct {
	health           map[string]model.Health
	history          map[string][]model.HealthPoint
	historyCalls     int
	historyTolerance time.Duration
}

func (f *fakeStore) Ping(context.Context) error { return nil }
func (f *fakeStore) LatestHealth(context.Context, []string) (map[string]model.Health, error) {
	return f.health, nil
}
func (f *fakeStore) HealthHistory(_ context.Context, _ []string, _, _ time.Time, gapTolerance time.Duration) (map[string][]model.HealthPoint, error) {
	f.historyCalls++
	f.historyTolerance = gapTolerance
	return f.history, nil
}

// The public routes face the internet. A refused request is logged, because
// it may be a broken link of ours, but a throttled one is not: one event per
// throttled request would let anyone write into the log stream at the rate
// limit itself.
func TestRequestLogSkipsThrottledRequests(t *testing.T) {
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	events := &eventCollector{}
	app := New(&fakeStore{}, serviceCatalog, Options{
		StatusRateLimit: 1,
		// httptest requests come from 192.0.2.1, standing in for the edge proxy.
		TrustedProxies: []string{"192.0.2.1"},
		Events:         events,
	})

	get := func(path string) int {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Cf-Connecting-Ip", "203.0.113.7")
		return serve(app, request).Code
	}
	if got := get("/v1/status"); got != http.StatusOK {
		t.Fatalf("first request = %d, want 200", got)
	}
	if got := get("/v1/status"); got != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429", got)
	}
	if got := get("/v1/nope"); got != http.StatusNotFound {
		t.Fatalf("unknown path = %d, want 404", got)
	}

	if len(events.events) != 1 {
		t.Fatalf("events = %#v, want only the 404", events.events)
	}
	event := events.events[0]
	if event.message != "HTTP request completed" || event.level != logclient.Warning {
		t.Fatalf("event = %#v, want the request log at warning", event)
	}
	if event.payload["path"] != "/v1/nope" || event.payload["status"] != http.StatusNotFound {
		t.Fatalf("payload = %#v, want the path and 404", event.payload)
	}
}

// Without an event client the API still serves; the nil client drops events.
func TestAPIServesWithoutAnEventClient(t *testing.T) {
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	app := New(&fakeStore{}, serviceCatalog, Options{})
	if got := serve(app, httptest.NewRequest(http.MethodGet, "/health", nil)).Code; got != http.StatusOK {
		t.Fatalf("health = %d, want 200", got)
	}
}
