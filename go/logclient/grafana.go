package logclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// grafana ships log events to Grafana Cloud Loki through its OTLP gateway,
// using OTLP/HTTP with the JSON encoding and basic auth (instance id and an
// access policy token with logs:write).
//
// The OTLP JSON body is written by hand rather than with the OpenTelemetry Go
// SDK. Every backend's go.mod replaces this module with the local copy, so
// whatever logclient imports lands in every service image; the SDK and its
// OTLP exporter would add protobuf, gRPC status types, and several otel
// modules for what is one small POST. The wire format is stable (OTLP 1.x)
// and the test below pins the fields Loki reads.
type grafana struct {
	endpoint   string
	user       string
	token      string
	resource   []otlpAttribute
	httpClient *http.Client
	batch      *batcher[otlpRecord]
}

// newGrafanaFromEnv reads GRAFANA_OTLP_ENDPOINT, GRAFANA_OTLP_USER, and
// GRAFANA_OTLP_TOKEN. All three unset means disabled, quietly; some unset
// means disabled with a warning naming the missing ones, never their values.
func newGrafanaFromEnv(service string) *grafana {
	endpoint := strings.TrimSpace(os.Getenv("GRAFANA_OTLP_ENDPOINT"))
	user := strings.TrimSpace(os.Getenv("GRAFANA_OTLP_USER"))
	token := strings.TrimSpace(os.Getenv("GRAFANA_OTLP_TOKEN"))
	if endpoint == "" && user == "" && token == "" {
		return nil
	}
	// Only the names of the unset variables are logged, never a value.
	var missing []string
	if endpoint == "" {
		missing = append(missing, "GRAFANA_OTLP_ENDPOINT")
	}
	if user == "" {
		missing = append(missing, "GRAFANA_OTLP_USER")
	}
	if token == "" {
		missing = append(missing, "GRAFANA_OTLP_TOKEN")
	}
	if len(missing) > 0 {
		log.Printf("warning: Grafana log export disabled; unset: %s", strings.Join(missing, ", "))
		return nil
	}
	return newGrafana(endpoint, user, token, service, os.Getenv("BIOTRON_ENV"), defaultBatchOptions)
}

// newGrafana takes the gateway's base URL, such as
// https://otlp-gateway-prod-us-east-0.grafana.net/otlp, and posts to its
// /v1/logs. A URL that already ends in /v1/logs is used as given.
func newGrafana(endpoint, user, token, service, environment string, options batchOptions) *grafana {
	endpoint = strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(endpoint, "/v1/logs") {
		endpoint += "/v1/logs"
	}
	resource := []otlpAttribute{stringAttribute("service.name", service)}
	if environment != "" {
		resource = append(resource, stringAttribute("deployment.environment.name", environment))
	}
	g := &grafana{
		endpoint:   endpoint,
		user:       user,
		token:      token,
		resource:   resource,
		httpClient: &http.Client{Timeout: options.timeout},
	}
	g.batch = newBatcher("Grafana log", options, g.send)
	return g
}

// add converts the event now, on the caller's goroutine, so a payload the
// caller changes afterwards cannot change what is sent.
func (g *grafana) add(level Level, message string, payload any) {
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	number, text := otlpSeverity(level)
	g.batch.add(otlpRecord{
		TimeUnixNano:         now,
		ObservedTimeUnixNano: now,
		SeverityNumber:       number,
		SeverityText:         text,
		Body:                 otlpValue{StringValue: &message},
		Attributes:           otlpAttributes(payload),
	})
}

func (g *grafana) send(ctx context.Context, records []otlpRecord) error {
	body := otlpLogsRequest{ResourceLogs: []otlpResourceLogs{{
		Resource: otlpResource{Attributes: g.resource},
		ScopeLogs: []otlpScopeLogs{{
			Scope:      otlpScope{Name: "github.com/BioTronDesignTeam/BioTronWeb/go/logclient"},
			LogRecords: records,
		}},
	}}}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal OTLP logs: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.SetBasicAuth(g.user, g.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := g.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send OTLP logs: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("grafana returned HTTP %d", response.StatusCode)
	}
	return nil
}

// The types below are the subset of the OTLP logs JSON encoding this client
// writes. 64-bit integers are strings, as the encoding requires.
type otlpLogsRequest struct {
	ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
}

type otlpResourceLogs struct {
	Resource  otlpResource    `json:"resource"`
	ScopeLogs []otlpScopeLogs `json:"scopeLogs"`
}

type otlpResource struct {
	Attributes []otlpAttribute `json:"attributes"`
}

type otlpScopeLogs struct {
	Scope      otlpScope    `json:"scope"`
	LogRecords []otlpRecord `json:"logRecords"`
}

type otlpScope struct {
	Name string `json:"name"`
}

type otlpRecord struct {
	TimeUnixNano         string          `json:"timeUnixNano"`
	ObservedTimeUnixNano string          `json:"observedTimeUnixNano"`
	SeverityNumber       int             `json:"severityNumber"`
	SeverityText         string          `json:"severityText"`
	Body                 otlpValue       `json:"body"`
	Attributes           []otlpAttribute `json:"attributes,omitempty"`
}

type otlpAttribute struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
	IntValue    *string  `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
}

func stringAttribute(key, value string) otlpAttribute {
	return otlpAttribute{Key: key, Value: otlpValue{StringValue: &value}}
}

// otlpSeverity maps a level to the first number of its OTLP severity range.
func otlpSeverity(level Level) (int, string) {
	switch level {
	case Debug:
		return 5, "DEBUG"
	case Warning:
		return 13, "WARN"
	case Error:
		return 17, "ERROR"
	default:
		return 9, "INFO"
	}
}

// otlpAttributes turns a payload into one attribute per top-level field, the
// way it reads after a JSON round trip, so a struct and a map with the same
// JSON give the same attributes. Nested objects and arrays are kept as JSON
// text. A payload that is not a JSON object becomes a single "payload".
func otlpAttributes(payload any) []otlpAttribute {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return []otlpAttribute{stringAttribute("payload_error", err.Error())}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return []otlpAttribute{stringAttribute("payload", string(raw))}
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	attributes := make([]otlpAttribute, 0, len(fields))
	for _, key := range keys {
		if value, ok := otlpScalar(fields[key]); ok {
			attributes = append(attributes, otlpAttribute{Key: key, Value: value})
		}
	}
	return attributes
}

func otlpScalar(field json.RawMessage) (otlpValue, bool) {
	decoder := json.NewDecoder(bytes.NewReader(field))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return otlpValue{}, false
	}
	switch v := value.(type) {
	case nil:
		return otlpValue{}, false
	case string:
		return otlpValue{StringValue: &v}, true
	case bool:
		return otlpValue{BoolValue: &v}, true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			text := strconv.FormatInt(i, 10)
			return otlpValue{IntValue: &text}, true
		}
		if f, err := v.Float64(); err == nil {
			return otlpValue{DoubleValue: &f}, true
		}
		text := v.String()
		return otlpValue{StringValue: &text}, true
	default:
		text := string(field)
		return otlpValue{StringValue: &text}, true
	}
}
