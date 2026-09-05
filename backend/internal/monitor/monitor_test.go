package monitor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

type memoryStore struct {
	latest      []model.Health
	persisted   []model.Health
	postgresErr error
	redisErr    error
}

func (s *memoryStore) PingPostgres(context.Context) error { return s.postgresErr }
func (s *memoryStore) PingRedis(context.Context) error    { return s.redisErr }

func (s *memoryStore) SetLatestHealth(_ context.Context, health model.Health) error {
	s.latest = append(s.latest, health)
	return nil
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
	monitor := New(dataStore, serviceCatalog, time.Second, time.Hour, time.Second)
	monitor.poll(context.Background())

	if len(dataStore.latest) != 1 || !dataStore.latest[0].OK {
		t.Fatalf("latest health = %#v", dataStore.latest)
	}
	if len(dataStore.persisted) != 1 {
		t.Fatalf("persisted health = %#v", dataStore.persisted)
	}
}

// The shared database and cache are probed through the store's own
// connections, not over HTTP, and each one is judged on its own so the page can
// say which of the two is missing.
func TestPollPingsSharedInfrastructureThroughTheStore(t *testing.T) {
	serviceCatalog, err := catalog.Load(`[
      {"id":"infrastructure","name":"Infrastructure","components":[
        {"id":"postgres","name":"Database","check":"postgres"},
        {"id":"redis","name":"Cache","check":"redis"}
      ]}
    ]`)
	if err != nil {
		t.Fatal(err)
	}
	dataStore := &memoryStore{redisErr: errors.New("dial tcp redis:6379: connection refused")}
	monitor := New(dataStore, serviceCatalog, time.Second, time.Hour, time.Second)
	monitor.poll(context.Background())

	byService := make(map[string]model.Health)
	for _, health := range dataStore.latest {
		byService[health.Service] = health
	}
	if postgres := byService["postgres"]; !postgres.OK || postgres.Detail == "" {
		t.Fatalf("postgres health = %#v, want ok with a timing detail", postgres)
	}
	if redis := byService["redis"]; redis.OK || redis.Detail != "dial tcp redis:6379: connection refused" {
		t.Fatalf("redis health = %#v, want down with the dial error as detail", redis)
	}
}
