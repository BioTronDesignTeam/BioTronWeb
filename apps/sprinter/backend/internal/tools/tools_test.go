package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/readstore"
)

// stubReader answers with fixed rows and records the query it was given, so a
// test can check that an argument reached the SQL layer as the right filter.
type stubReader struct {
	logs    []readstore.LogRow
	next    string
	health  []readstore.HealthRow
	access  readstore.AccessReport
	lastLog readstore.LogQuery
	err     error
}

func (s *stubReader) RecentLogs(_ context.Context, query readstore.LogQuery) ([]readstore.LogRow, error) {
	s.lastLog = query
	return s.logs, s.err
}

func (s *stubReader) LogHistory(_ context.Context, query readstore.LogQuery) ([]readstore.LogRow, string, error) {
	s.lastLog = query
	return s.logs, s.next, s.err
}

func (s *stubReader) HealthHistory(context.Context, string, time.Time, int) ([]readstore.HealthRow, error) {
	return s.health, s.err
}

func (s *stubReader) Access(context.Context, string) (readstore.AccessReport, error) {
	return s.access, s.err
}

func run(t *testing.T, tool Tool, args string) (string, error) {
	t.Helper()
	return tool.Run(t.Context(), json.RawMessage(args))
}

func TestCapResultSaysItCut(t *testing.T) {
	short := "one line"
	if got := capResult(short, 100, "narrow it"); got != short {
		t.Fatalf("a short result must pass through unchanged, got %q", got)
	}

	long := strings.Repeat("row of text\n", 100)
	got := capResult(long, 120, "Ask for fewer rows.")
	if len(got) <= 120 {
		// The notice is added after the cut, so the result is a little longer
		// than the limit. What matters is that the content was cut.
		t.Log("capped result is shorter than the limit plus its notice")
	}
	if strings.Count(got, "row of text") >= 100 {
		t.Fatal("nothing was cut")
	}
	if !strings.Contains(got, "Cut here") || !strings.Contains(got, "Ask for fewer rows.") {
		t.Fatalf("a capped result must say it was cut and how to narrow: %q", got)
	}
	// The cut lands on a line boundary, so the model never reads half a row.
	body := got[:strings.Index(got, "\n[Cut here")]
	if strings.HasSuffix(body, "row of tex") {
		t.Fatal("the cut split a line")
	}
}

func TestTruncateMarksWhatItDropped(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("got %q", got)
	}
	if got := truncate("0123456789abc", 10); got != "0123456789…" {
		t.Fatalf("got %q", got)
	}
}

// Both limits count bytes. A log line or a payload holding an accent or an
// emoji puts a character across the limit, and cutting through one leaves the
// model a replacement glyph instead of the text.
func TestCutsNeverSplitACharacter(t *testing.T) {
	// "é" is two bytes, so a character straddles every odd byte index.
	accents := strings.Repeat("é", 40)
	for limit := 9; limit <= 12; limit++ {
		got := truncate(accents, limit)
		if !utf8.ValidString(got) {
			t.Fatalf("truncate at %d is not valid UTF-8: %q", limit, got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("truncate at %d dropped its marker: %q", limit, got)
		}
	}

	// A four-byte emoji, and a body with no line to cut on, so capResult
	// falls back to cutting at the limit itself.
	rockets := strings.Repeat("🚀", 40)
	for limit := 9; limit <= 12; limit++ {
		got := capResult(rockets, limit, "Ask for fewer rows.")
		if !utf8.ValidString(got) {
			t.Fatalf("capResult at %d is not valid UTF-8: %q", limit, got)
		}
		if !strings.Contains(got, "Cut here") {
			t.Fatalf("capResult at %d did not say it cut: %q", limit, got)
		}
	}

	// The line-boundary path has to stay valid too.
	lines := strings.Repeat("🚀🚀🚀\n", 20)
	got := capResult(lines, 50, "Ask for fewer rows.")
	if !utf8.ValidString(got) {
		t.Fatalf("capResult on lines is not valid UTF-8: %q", got)
	}
}

func TestEverySpecIsUsableJSONSchema(t *testing.T) {
	reader := &stubReader{}
	all := All(reader, "http://logger", "http://calendar")
	if len(all) != 6 {
		t.Fatalf("tools = %d, want 6", len(all))
	}
	names := map[string]bool{}
	for _, tool := range all {
		spec := tool.Spec()
		if spec.Name == "" || spec.Description == "" {
			t.Fatalf("tool %+v is missing a name or a description", spec)
		}
		if names[spec.Name] {
			t.Fatalf("two tools are called %q", spec.Name)
		}
		names[spec.Name] = true
		var schema struct {
			Type       string                     `json:"type"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(spec.Schema, &schema); err != nil {
			t.Fatalf("tool %q has an unparseable schema: %v", spec.Name, err)
		}
		if schema.Type != "object" {
			t.Fatalf("tool %q takes %q arguments, want object", spec.Name, schema.Type)
		}
	}
	// A missing reader must leave out the SQL tools rather than build tools that
	// would panic on their first call.
	if got := len(All(nil, "http://logger", "http://calendar")); got != 2 {
		t.Fatalf("tools without a reader = %d, want 2", got)
	}
}

func TestRecentLogsValidatesItsArguments(t *testing.T) {
	tool := RecentLogs{Reader: &stubReader{}}
	cases := map[string]string{
		"a limit above the cap":       `{"limit": 51}`,
		"a limit below one":           `{"limit": 0}`,
		"an unknown level":            `{"level": "fatal"}`,
		"a window it cannot page":     `{"from": "2026-09-01T00:00:00Z"}`,
		"a cursor it cannot use":      `{"cursor": "abc"}`,
		"arguments that are not JSON": `{"limit":`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, tool, args); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestRecentLogsPassesItsFiltersDown(t *testing.T) {
	reader := &stubReader{logs: []readstore.LogRow{{
		ID: 4, Service: "oauth-manager", Level: "error", Message: "Authorization failed",
		Payload: json.RawMessage(`{"stage":"check"}`), CreatedAt: time.Unix(1757000000, 0),
	}}}
	out, err := run(t, RecentLogs{Reader: reader},
		`{"service":"oauth-manager","level":"error","search":"Auth","limit":5}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if reader.lastLog.Service != "oauth-manager" || reader.lastLog.Level != "error" ||
		reader.lastLog.Search != "Auth" || reader.lastLog.Limit != 5 {
		t.Fatalf("query = %+v", reader.lastLog)
	}
	for _, want := range []string{"1 rows", "oauth-manager", "Authorization failed", `{"stage":"check"}`} {
		if !strings.Contains(out, want) {
			t.Fatalf("result is missing %q:\n%s", want, out)
		}
	}

	// The default limit is applied rather than left at zero, which would ask
	// Postgres for no rows at all.
	if _, err := run(t, RecentLogs{Reader: reader}, `{}`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if reader.lastLog.Limit != 20 {
		t.Fatalf("default limit = %d, want 20", reader.lastLog.Limit)
	}
}

func TestRecentLogsSaysWhenNothingMatched(t *testing.T) {
	out, err := run(t, RecentLogs{Reader: &stubReader{}}, `{"service":"nobody"}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "No log rows") {
		t.Fatalf("result = %q", out)
	}
}

func TestLogHistoryReportsItsCursor(t *testing.T) {
	reader := &stubReader{
		logs: []readstore.LogRow{{ID: 9, Service: "exo-api", Level: "info", Message: "Telemetry read"}},
		next: "cursor-token",
	}
	out, err := run(t, LogHistory{Reader: reader},
		`{"from":"2026-09-01T00:00:00Z","to":"2026-09-05T00:00:00Z","limit":1}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "next_cursor: cursor-token") {
		t.Fatalf("result must carry the cursor:\n%s", out)
	}
	if reader.lastLog.From == nil || reader.lastLog.To == nil {
		t.Fatalf("query = %+v", reader.lastLog)
	}

	// The last page has to say so, or the model keeps asking for another one.
	reader.next = ""
	out, err = run(t, LogHistory{Reader: reader}, `{}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "last page") {
		t.Fatalf("result = %q", out)
	}
}

func TestLogHistoryRejectsABadWindow(t *testing.T) {
	tool := LogHistory{Reader: &stubReader{}}
	cases := map[string]string{
		"a from that is not a timestamp": `{"from": "yesterday"}`,
		"a to that is not a timestamp":   `{"to": "soon"}`,
		"a window that runs backwards":   `{"from":"2026-09-05T00:00:00Z","to":"2026-09-01T00:00:00Z"}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, tool, args); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestHealthHistoryNeedsAServiceAndASaneWindow(t *testing.T) {
	tool := HealthHistory{Reader: &stubReader{}}
	for name, args := range map[string]string{
		"no service":      `{}`,
		"a blank service": `{"service": "  "}`,
		"too many hours":  `{"service": "logger-api", "hours": 169}`,
		"no hours at all": `{"service": "logger-api", "hours": 0}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, tool, args); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestHealthHistoryCountsFailuresAndKeepsTheDetail(t *testing.T) {
	reader := &stubReader{health: []readstore.HealthRow{
		{OK: false, Detail: "dial tcp 10.0.0.2:8080: connect: refused", CheckedAt: time.Unix(1757000060, 0)},
		{OK: true, CheckedAt: time.Unix(1757000000, 0)},
	}}
	out, err := run(t, HealthHistory{Reader: reader}, `{"service":"exo-api","hours":6}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"exo-api", "2 checks", "1 failed", "down", "connect: refused", "up"} {
		if !strings.Contains(out, want) {
			t.Fatalf("result is missing %q:\n%s", want, out)
		}
	}

	empty, err := run(t, HealthHistory{Reader: &stubReader{}}, `{"service":"typo"}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(empty, "No health checks") {
		t.Fatalf("result = %q", empty)
	}
}

func TestWhoHasAccessNamesManagersAndSuperusersSeparately(t *testing.T) {
	reader := &stubReader{access: readstore.AccessReport{
		AppID: "sprinter", AppName: "Sprinter", AppFound: true,
		Permissions: []readstore.PermissionRow{{Key: "admin", Label: "Administer Sprinter"}},
		Grants: []readstore.GrantRow{
			{Login: "levon", Name: "Levon", PermissionKey: "admin", GrantedAt: time.Unix(1757000000, 0)},
			{Login: "banned-one", PermissionKey: "admin", IsBanned: true, GrantedAt: time.Unix(1757000000, 0)},
		},
		Managers:   []readstore.OperatorRow{{Login: "lead", IsManager: true}},
		Superusers: []readstore.OperatorRow{{Login: "root", IsSuperuser: true}},
	}}
	out, err := run(t, WhoHasAccess{Reader: reader}, `{"app":"sprinter"}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"App sprinter (Sprinter)", "admin (Administer Sprinter)",
		"levon (Levon): admin", "BANNED", "Managers", "lead", "Superusers", "root",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("result is missing %q:\n%s", want, out)
		}
	}
}

func TestWhoHasAccessNeedsAnAppAndSaysWhenItIsUnknown(t *testing.T) {
	if _, err := run(t, WhoHasAccess{Reader: &stubReader{}}, `{}`); err == nil {
		t.Fatal("app is required")
	}
	out, err := run(t, WhoHasAccess{Reader: &stubReader{}}, `{"app":"ghost"}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, `No app is registered with the id "ghost"`) {
		t.Fatalf("result = %q", out)
	}
}

func TestCalendarUpcomingValidatesItsArguments(t *testing.T) {
	tool := CalendarUpcoming{CalendarURL: "http://calendar"}
	for name, args := range map[string]string{
		"too many events": `{"limit": 21}`,
		"no events":       `{"limit": 0}`,
		"too many days":   `{"days": 91}`,
		"no days":         `{"days": 0}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, tool, args); err == nil {
				t.Fatal("want an error")
			}
		})
	}
	// A tool with nowhere to call must say so rather than dial an empty URL.
	if _, err := run(t, CalendarUpcoming{}, `{}`); err == nil {
		t.Fatal("an unconfigured Calendar URL must be an error")
	}
	if _, err := run(t, PlatformStatus{}, `{}`); err == nil {
		t.Fatal("an unconfigured Logger URL must be an error")
	}
}

// A page big enough to be cut is exactly the page whose next_cursor the
// model needs, so the cursor line must survive the cap.
func TestLogHistoryKeepsItsCursorWhenCut(t *testing.T) {
	var rows []readstore.LogRow
	for i := 0; i < 50; i++ {
		rows = append(rows, readstore.LogRow{
			ID: int64(i), Service: "exo-api", Level: "info",
			Message: strings.Repeat("x", 400),
		})
	}
	reader := &stubReader{logs: rows, next: "cursor-token"}
	out, err := run(t, LogHistory{Reader: reader}, `{}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "[Cut here.") {
		t.Fatalf("this page was meant to be cut:\n%d chars", len(out))
	}
	if !strings.HasSuffix(out, "next_cursor: cursor-token") {
		t.Fatalf("the cursor must end a cut page:\n%s", out[len(out)-200:])
	}
	if len(out) > logHistoryCap+len("[Cut here. This result was longer than 8000 characters. Lower limit, narrow the window, or filter by service or level.]")+64 {
		t.Fatalf("result is %d chars, far over the cap", len(out))
	}
}
