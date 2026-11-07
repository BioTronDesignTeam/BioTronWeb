package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve answers every request with status and body.
func serve(t *testing.T, status int, body string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("a tool sent a credential: %v", r.Header)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// A broken sibling service is a failed tool call the model can read, never a
// panic and never a result past the cap.
func TestHTTPToolsTurnBadAnswersIntoErrors(t *testing.T) {
	huge := `[` + strings.Repeat(`{"title":"x"},`, maxBody/10) + `{"title":"x"}]`
	cases := []struct {
		name, body string
		status     int
	}{
		{"a server error", `{"error":"boom"}`, http.StatusInternalServerError},
		{"a body that is not JSON", `<html>bad gateway</html>`, http.StatusOK},
		{"a body larger than the read limit", huge, http.StatusOK},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			base := serve(t, testCase.status, testCase.body)
			for _, tool := range []Tool{PlatformStatus{StatusURL: base}, CalendarUpcoming{CalendarURL: base}} {
				out, err := tool.Run(t.Context(), nil)
				if err == nil {
					t.Fatalf("%s answered %d characters, want an error", tool.Spec().Name, len(out))
				}
			}
		})
	}
}

func TestPlatformStatusIsCapped(t *testing.T) {
	long := `{"components":[` + strings.Repeat(`{"id":"component","state":"operational"},`, 400) + `{"id":"last"}]}`
	out, err := PlatformStatus{StatusURL: serve(t, http.StatusOK, long)}.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) > statusCap+200 || !strings.Contains(out, "[Cut here.") {
		t.Fatalf("result of %d characters, want it cut at %d with a notice", len(out), statusCap)
	}
}

func TestCalendarUpcomingReportsEvents(t *testing.T) {
	body := `[{"title":"Build night","scope_path":"Exo","starts_at":"2026-09-10T22:00:00Z",` +
		`"ends_at":"2026-09-11T00:00:00Z","location":"Lab"}]`
	out, err := CalendarUpcoming{CalendarURL: serve(t, http.StatusOK, body)}.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "- Build night | calendar: Exo | starts: 2026-09-10T22:00:00Z | ends: 2026-09-11T00:00:00Z | location: Lab"
	if !strings.Contains(out, want) {
		t.Fatalf("output = %q, want a line %q", out, want)
	}
	empty, err := CalendarUpcoming{CalendarURL: serve(t, http.StatusOK, `[]`)}.Run(t.Context(), nil)
	if err != nil || empty != "No events in the next 42 days." {
		t.Fatalf("empty = %q, %v", empty, err)
	}
}
