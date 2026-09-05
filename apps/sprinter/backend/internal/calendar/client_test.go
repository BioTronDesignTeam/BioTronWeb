package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOccurrencesRequestsExpectedWindow(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"uid": "abc", "recurrence_id_local": "2026-09-10T09:00:00",
			"series_sequence": 2, "override_sequence": 3,
			"title": "Kickoff", "description": "", "location": "Lab",
			"url": "https://example.com", "starts_at": "2026-09-10T13:00:00Z",
			"ends_at": "2026-09-10T14:00:00Z", "all_day": false,
			"timezone": "America/Toronto", "scope_id": "s1",
			"scope_path": "Exo", "modified": true
		}]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, toronto)
	from := time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	occurrences, err := client.Occurrences(context.Background(), "s1", from, to)
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("len = %d", len(occurrences))
	}
	occ := occurrences[0]
	if occ.UID != "abc" || occ.Sequence() != 3 || !occ.Modified {
		t.Fatalf("occurrence = %+v", occ)
	}

	parsed, err := http.NewRequest(http.MethodGet, "http://x/?"+gotQuery, nil)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if got := parsed.URL.Query().Get("scope_id"); got != "s1" {
		t.Fatalf("scope_id = %q", got)
	}
	if got := parsed.URL.Query().Get("from"); got != "2026-09-10" {
		t.Fatalf("from = %q", got)
	}
	if got := parsed.URL.Query().Get("to"); got != "2026-09-11" {
		t.Fatalf("to = %q, want the next day since from and to fall in the same local day", got)
	}
}

func TestOccurrencesSequenceFallsBackToSeries(t *testing.T) {
	occ := Occurrence{SeriesSequence: 4}
	if occ.Sequence() != 4 {
		t.Fatalf("Sequence() = %d, want 4", occ.Sequence())
	}
	occ.OverrideSequence = 7
	if occ.Sequence() != 7 {
		t.Fatalf("Sequence() = %d, want 7", occ.Sequence())
	}
}

func TestOccurrencesNon200IsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	client := NewClient(server.URL, time.UTC)
	_, err := client.Occurrences(context.Background(), "", time.Now(), time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestOccurrencesTransportErrorIsError(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", time.UTC)
	_, err := client.Occurrences(context.Background(), "", time.Now(), time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected a transport error, got nil")
	}
}

// jsonRoundTrip guards against the Occurrence struct silently losing a field
// Calendar sends: every field here must survive an encode/decode.
func TestOccurrenceJSONRoundTrip(t *testing.T) {
	starts := time.Date(2026, 9, 10, 13, 0, 0, 0, time.UTC)
	occ := Occurrence{
		UID: "u", RecurrenceID: "2026-09-10T09:00:00", SeriesSequence: 1,
		OverrideSequence: 2, Title: "T", Description: "D", Location: "L",
		URL: "https://x", StartsAt: starts, EndsAt: starts.Add(time.Hour),
		AllDay: false, Timezone: "America/Toronto", ScopeID: "s",
		ScopePath: "Exo", Modified: true,
	}
	raw, err := json.Marshal(occ)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Occurrence
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != occ {
		t.Fatalf("decoded = %+v, want %+v", decoded, occ)
	}
}
