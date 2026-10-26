package monitor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"

	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

type recordedEvent struct {
	level   logclient.Level
	message string
	payload map[string]any
}

// collector keeps the events in memory. poll records them on its own
// goroutine, one after another, so no lock is needed.
type collector struct {
	events []recordedEvent
}

func (c *collector) LogAsync(level logclient.Level, message string, payload any) {
	fields, _ := payload.(map[string]any)
	c.events = append(c.events, recordedEvent{level: level, message: message, payload: fields})
}

type memoryStore struct {
	latest      []model.Health
	persisted   []model.Health
	postgresErr error
}

func (s *memoryStore) PingPostgres(context.Context) error { return s.postgresErr }

func (s *memoryStore) SetLatestHealth(health model.Health) {
	s.latest = append(s.latest, health)
}

func (s *memoryStore) InsertHealth(_ context.Context, health model.Health) error {
	s.persisted = append(s.persisted, health)
	return nil
}

func TestPollCachesAndPersistsHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	serviceCatalog, err := catalog.Load(`[
      {"id":"test","name":"Test","components":[{"id":"test-api","name":"API","health_url":"` + server.URL + `"}]}
    ]`)
	if err != nil {
		t.Fatal(err)
	}
	dataStore := &memoryStore{}
	monitor := New(dataStore, serviceCatalog, nil, time.Second, time.Hour, time.Second)
	monitor.poll(context.Background())

	if len(dataStore.latest) != 1 || !dataStore.latest[0].OK {
		t.Fatalf("latest health = %#v", dataStore.latest)
	}
	if len(dataStore.persisted) != 1 {
		t.Fatalf("persisted health = %#v", dataStore.persisted)
	}
}

// The shared database is probed through the store's own pool, not over HTTP,
// and a failed ping carries the error as its detail.
func TestPollPingsTheDatabaseThroughTheStore(t *testing.T) {
	serviceCatalog, err := catalog.Load(`[
      {"id":"infrastructure","name":"Infrastructure","components":[
        {"id":"postgres","name":"Database","check":"postgres"}
      ]}
    ]`)
	if err != nil {
		t.Fatal(err)
	}

	dataStore := &memoryStore{}
	New(dataStore, serviceCatalog, nil, time.Second, time.Hour, time.Second).poll(context.Background())
	if len(dataStore.latest) != 1 || !dataStore.latest[0].OK || dataStore.latest[0].Detail == "" {
		t.Fatalf("postgres health = %#v, want ok with a timing detail", dataStore.latest)
	}

	dataStore = &memoryStore{postgresErr: errors.New("dial tcp postgres:5432: connection refused")}
	New(dataStore, serviceCatalog, nil, time.Second, time.Hour, time.Second).poll(context.Background())
	if len(dataStore.latest) != 1 || dataStore.latest[0].OK || dataStore.latest[0].Detail != "dial tcp postgres:5432: connection refused" {
		t.Fatalf("postgres health = %#v, want down with the dial error as detail", dataStore.latest)
	}
}

// History takes a row on a state change or on a heartbeat, so it can say what
// happened only to someone already reading it. These two events are what tells
// an operator, and a poll that changes nothing must stay silent.
func TestPollReportsComponentDownAndRecovered(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if healthy.Load() {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		response.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	serviceCatalog, err := catalog.Load(`[
      {"id":"test","name":"Test","components":[{"id":"test-api","name":"API","health_url":"` + server.URL + `"}]}
    ]`)
	if err != nil {
		t.Fatal(err)
	}
	events := &collector{}
	monitor := New(&memoryStore{}, serviceCatalog, events, time.Second, time.Hour, time.Second)

	monitor.poll(context.Background())
	if len(events.events) != 0 {
		t.Fatalf("a first look at a healthy component recorded %#v, want nothing", events.events)
	}

	healthy.Store(false)
	monitor.poll(context.Background())
	healthy.Store(true)
	monitor.poll(context.Background())
	monitor.poll(context.Background())

	if len(events.events) != 2 {
		t.Fatalf("events = %#v, want one down and one recovered", events.events)
	}
	down := events.events[0]
	if down.message != "Component down" || down.level != logclient.Warning {
		t.Fatalf("down event = %#v, want Component down at warning", down)
	}
	if down.payload["component"] != "test-api" || down.payload["detail"] == "" {
		t.Fatalf("down payload = %#v, want the component and the probe detail", down.payload)
	}
	recovered := events.events[1]
	if recovered.message != "Component recovered" || recovered.level != logclient.Info {
		t.Fatalf("recovered event = %#v, want Component recovered at info", recovered)
	}
	if recovered.payload["component"] != "test-api" {
		t.Fatalf("recovered payload = %#v, want the component", recovered.payload)
	}
	if _, ok := recovered.payload["down_for_s"]; !ok {
		t.Fatalf("recovered payload = %#v, want how long it was down", recovered.payload)
	}
}
