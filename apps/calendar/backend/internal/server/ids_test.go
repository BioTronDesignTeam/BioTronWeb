package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An id that is not a UUID used to reach Postgres, fail its uuid cast, and
// come back as a 500. routingApp has no store, so each of these also proves
// the refusal comes before any query.
func TestIDsThatAreNotUUIDsAreRefusedBeforeAQuery(t *testing.T) {
	app := routingApp(t)
	jsonRequest := func(method, target, body string) *http.Request {
		request := httptest.NewRequest(method, target, strings.NewReader(body))
		request.Header.Set("X-Requested-With", "XMLHttpRequest")
		request.Header.Set("Cookie", "session=abc")
		request.Header.Set("Content-Type", "application/json")
		return request
	}
	notFound := answer{status: http.StatusNotFound, body: `{"error":"not found"}`}
	for _, request := range []*http.Request{
		jsonRequest(http.MethodPatch, "/v1/admin/scopes/nope", `{"name":"Drivetrain"}`),
		jsonRequest(http.MethodPost, "/v1/admin/scopes/nope/archive", ""),
		jsonRequest(http.MethodPost, "/v1/admin/scopes/nope/restore", ""),
		jsonRequest(http.MethodDelete, "/v1/admin/scopes/nope", ""),
		jsonRequest(http.MethodPatch, "/v1/admin/events/nope", `{}`),
		jsonRequest(http.MethodPost, "/v1/admin/events/nope/publish", `{"expected_sequence":1}`),
		jsonRequest(http.MethodPost, "/v1/admin/events/nope/cancel", `{"expected_sequence":1}`),
		jsonRequest(http.MethodDelete, "/v1/admin/events/1234", ""),
		jsonRequest(http.MethodPut, "/v1/admin/events/nope/occurrences", `{}`),
		jsonRequest(http.MethodDelete, "/v1/admin/events/nope/occurrences?recurrence_id_local=2026-12-01T18:00:00&expected_sequence=1", ""),
		httptest.NewRequest(http.MethodGet, "/v1/feeds/scopes/nope.ics", nil),
	} {
		checkAnswer(t, app, request, notFound)
	}

	badScope := answer{status: http.StatusBadRequest, body: `{"error":"scope_id must be a UUID"}`}
	checkAnswer(t, app, jsonRequest(http.MethodGet, "/v1/admin/events?scope_id=nope", ""), badScope)
	checkAnswer(t, app, jsonRequest(http.MethodPost, "/v1/admin/events",
		`{"scope_id":"nope","title":"Build night","starts_at_local":"2026-12-01T18:00","ends_at_local":"2026-12-01T19:00"}`),
		badScope)
	checkAnswer(t, app, jsonRequest(http.MethodPost, "/v1/admin/scopes",
		`{"kind":"PROJECT","name":"Drivetrain","parent_id":"nope"}`),
		answer{status: http.StatusBadRequest, body: `{"error":"parent_id must be a UUID"}`})
}

func TestCanonicalIDAcceptsOnlyWhatPostgresStores(t *testing.T) {
	const id = "1d6f7a4c-6c1c-4f43-9c39-52a3b1b2a0f1"
	for _, value := range []string{id, strings.ToUpper(id), "urn:uuid:" + id, "{" + id + "}", " " + id + " "} {
		if got, ok := canonicalID(value); !ok || got != id {
			t.Fatalf("canonicalID(%q) = %q, %v; want %q", value, got, ok, id)
		}
	}
	for _, value := range []string{"", "nope", id + "0", "1234"} {
		if _, ok := canonicalID(value); ok {
			t.Fatalf("canonicalID(%q) accepted a value that is not a UUID", value)
		}
	}
}
