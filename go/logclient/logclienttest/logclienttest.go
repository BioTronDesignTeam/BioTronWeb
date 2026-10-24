// Package logclienttest gives a test a real *logclient.Client whose events it
// can read back. The client exports to a stand-in Grafana OTLP gateway that
// decodes each record into an Event, so a test checks what Grafana would
// receive without reaching into the client.
package logclienttest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

// Event is one exported record, read back the way the service sent it.
// Payload numbers are float64, as encoding/json decodes them.
type Event struct {
	Service string
	Level   logclient.Level
	Message string
	Payload map[string]any
}

// New returns a client for service and the channel its events arrive on. It
// sets the GRAFANA_OTLP_* variables with t.Setenv and clears PostHog, so the
// test cannot run in parallel. Events arrive in batches, within about a
// second of being logged. The client is closed when the test ends.
func New(t testing.TB, service string) (*logclient.Client, <-chan Event) {
	t.Helper()
	events := make(chan Event, 256)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body otlpRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode OTLP logs: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, resource := range body.ResourceLogs {
			name := ""
			for _, attribute := range resource.Resource.Attributes {
				if attribute.Key == "service.name" {
					name, _ = attribute.Value.decode().(string)
				}
			}
			for _, scope := range resource.ScopeLogs {
				for _, record := range scope.LogRecords {
					event := Event{Service: name, Level: level(record.SeverityText), Payload: map[string]any{}}
					event.Message, _ = record.Body.decode().(string)
					for _, attribute := range record.Attributes {
						event.Payload[attribute.Key] = attribute.Value.decode()
					}
					select {
					case events <- event:
					default:
						t.Errorf("more than %d events were never read", cap(events))
					}
				}
			}
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GRAFANA_OTLP_ENDPOINT", server.URL)
	t.Setenv("GRAFANA_OTLP_USER", "1")
	t.Setenv("GRAFANA_OTLP_TOKEN", "test")
	t.Setenv("POSTHOG_API_KEY", "")
	t.Setenv("LOG_LEVEL", "debug")
	client := logclient.NewFromEnv(service)
	// Registered after the server's cleanup, so it runs first and the last
	// batch still has somewhere to go.
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	return client, events
}

func level(severity string) logclient.Level {
	switch strings.ToUpper(severity) {
	case "DEBUG":
		return logclient.Debug
	case "WARN":
		return logclient.Warning
	case "ERROR":
		return logclient.Error
	default:
		return logclient.Info
	}
}

type otlpRequest struct {
	ResourceLogs []struct {
		Resource struct {
			Attributes []otlpAttribute `json:"attributes"`
		} `json:"resource"`
		ScopeLogs []struct {
			LogRecords []struct {
				SeverityText string          `json:"severityText"`
				Body         otlpValue       `json:"body"`
				Attributes   []otlpAttribute `json:"attributes"`
			} `json:"logRecords"`
		} `json:"scopeLogs"`
	} `json:"resourceLogs"`
}

type otlpAttribute struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue *string  `json:"stringValue"`
	BoolValue   *bool    `json:"boolValue"`
	IntValue    *string  `json:"intValue"`
	DoubleValue *float64 `json:"doubleValue"`
}

func (v otlpValue) decode() any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case v.IntValue != nil:
		number, _ := strconv.ParseFloat(*v.IntValue, 64)
		return number
	case v.DoubleValue != nil:
		return *v.DoubleValue
	default:
		return nil
	}
}
