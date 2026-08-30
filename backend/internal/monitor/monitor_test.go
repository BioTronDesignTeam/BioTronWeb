package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

type memoryStore struct {
	latest    []model.Health
	persisted []model.Health
}

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
