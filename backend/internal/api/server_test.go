package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

type fakeStore struct {
	inserted     model.NewLog
	health       map[string]model.Health
	history      map[string][]model.HealthPoint
	historyCalls int
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
func (f *fakeStore) HealthHistory(context.Context, []string, time.Time, time.Time) (map[string][]model.HealthPoint, error) {
	f.historyCalls++
	return f.history, nil
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
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", response.StatusCode)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer secret")
	response, err = app.Test(request, -1)
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

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/apps", nil), -1)
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
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/apps/exo/logs/history?levels=critical", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
