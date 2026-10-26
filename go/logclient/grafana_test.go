package logclient

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

var testBatchOptions = batchOptions{queue: 64, maxBatch: 100, interval: time.Hour, timeout: time.Second}

// otlpServer records every OTLP request body it receives.
type otlpServer struct {
	*httptest.Server
	mu       sync.Mutex
	bodies   []otlpLogsRequest
	paths    []string
	failWith int
}

func newOTLPServer(t *testing.T) *otlpServer {
	s := &otlpServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := r.BasicAuth()
		if !ok || user != "123456" || token != "glc_secret" {
			t.Error("missing basic auth")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		var body otlpLogsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		s.paths = append(s.paths, r.URL.Path)
		failWith := s.failWith
		s.mu.Unlock()
		if failWith != 0 {
			w.WriteHeader(failWith)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *otlpServer) records() []otlpRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []otlpRecord
	for _, body := range s.bodies {
		for _, resource := range body.ResourceLogs {
			for _, scope := range resource.ScopeLogs {
				records = append(records, scope.LogRecords...)
			}
		}
	}
	return records
}

func attribute(record otlpRecord, key string) (otlpValue, bool) {
	for _, a := range record.Attributes {
		if a.Key == key {
			return a.Value, true
		}
	}
	return otlpValue{}, false
}

func TestGrafanaReceivesOTLPLogs(t *testing.T) {
	server := newOTLPServer(t)
	client := &Client{
		minimum: 1,
		grafana: newGrafana(server.URL+"/otlp/", "123456", "glc_secret", "exogui-api", "staging", testBatchOptions),
	}
	if !client.Enabled() {
		t.Fatal("Grafana alone must enable the client")
	}
	client.LogAsync(Debug, "below LOG_LEVEL", nil)
	client.LogAsync(Warning, "Component down", map[string]any{
		"component": "postgres", "status": 503, "ratio": 0.5, "up": false, "nested": map[string]int{"a": 1}, "empty": nil,
	})
	client.LogAsync(Error, "Question failed", struct {
		Thread string `json:"thread"`
	}{"t-1"})
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(server.paths) != 1 || server.paths[0] != "/otlp/v1/logs" {
		t.Fatalf("paths = %v, want one POST to /otlp/v1/logs", server.paths)
	}
	resource := server.bodies[0].ResourceLogs[0].Resource.Attributes
	if len(resource) != 2 || *resource[0].Value.StringValue != "exogui-api" || *resource[1].Value.StringValue != "staging" {
		t.Fatalf("resource = %+v", resource)
	}
	records := server.records()
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2 (debug is below LOG_LEVEL)", len(records))
	}

	down := records[0]
	if down.SeverityNumber != 13 || down.SeverityText != "WARN" || *down.Body.StringValue != "Component down" || down.TimeUnixNano == "" {
		t.Fatalf("record = %+v", down)
	}
	if v, _ := attribute(down, "component"); v.StringValue == nil || *v.StringValue != "postgres" {
		t.Errorf("component = %+v", v)
	}
	if v, _ := attribute(down, "status"); v.IntValue == nil || *v.IntValue != "503" {
		t.Errorf("status = %+v", v)
	}
	if v, _ := attribute(down, "ratio"); v.DoubleValue == nil || *v.DoubleValue != 0.5 {
		t.Errorf("ratio = %+v", v)
	}
	if v, _ := attribute(down, "up"); v.BoolValue == nil || *v.BoolValue {
		t.Errorf("up = %+v", v)
	}
	if v, _ := attribute(down, "nested"); v.StringValue == nil || *v.StringValue != `{"a":1}` {
		t.Errorf("nested = %+v", v)
	}
	if _, ok := attribute(down, "empty"); ok {
		t.Error("a null field must be left out")
	}

	failed := records[1]
	if failed.SeverityNumber != 17 || failed.SeverityText != "ERROR" {
		t.Fatalf("record = %+v", failed)
	}
	if v, _ := attribute(failed, "thread"); v.StringValue == nil || *v.StringValue != "t-1" {
		t.Errorf("a struct payload must become attributes; got %+v", failed.Attributes)
	}
}

func TestGrafanaFailureNeverReachesTheCaller(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	server := newOTLPServer(t)
	server.failWith = http.StatusUnauthorized
	client := &Client{minimum: 1, grafana: newGrafana(server.URL, "123456", "glc_secret", "exogui-api", "", testBatchOptions)}
	client.LogAsync(Info, "sent anyway", nil)
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(server.records()) != 1 {
		t.Fatal("the event was never attempted")
	}
}

func TestGrafanaFromEnvNeedsAllThreeVariables(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	t.Setenv("GRAFANA_OTLP_ENDPOINT", "")
	t.Setenv("GRAFANA_OTLP_USER", "")
	t.Setenv("GRAFANA_OTLP_TOKEN", "")
	if newGrafanaFromEnv("exogui-api") != nil {
		t.Fatal("unset variables must disable Grafana")
	}
	t.Setenv("GRAFANA_OTLP_ENDPOINT", "https://otlp-gateway.example/otlp")
	t.Setenv("GRAFANA_OTLP_TOKEN", "glc_secret")
	if newGrafanaFromEnv("exogui-api") != nil {
		t.Fatal("a missing user must disable Grafana")
	}
	t.Setenv("GRAFANA_OTLP_USER", "123456")
	g := newGrafanaFromEnv("exogui-api")
	if g == nil || g.endpoint != "https://otlp-gateway.example/otlp/v1/logs" {
		t.Fatalf("grafana = %+v", g)
	}
	_ = g.batch.close(context.Background())
}
