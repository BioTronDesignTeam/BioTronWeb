package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/readstore"
)

// stubReader answers with fixed rows, so a test can drive a tool without a
// database.
type stubReader struct {
	health []readstore.HealthRow
	access readstore.AccessReport
	err    error
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
	cut := strings.Index(got, "\n[Cut here")
	if cut < 0 {
		t.Fatalf("the cut marker must start a line of its own: %q", got)
	}
	body := got[:cut]
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
	all := All(reader, "http://status", "http://calendar")
	if len(all) != 4 {
		t.Fatalf("tools = %d, want 4", len(all))
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
	if got := len(All(nil, "http://status", "http://calendar")); got != 2 {
		t.Fatalf("tools without a reader = %d, want 2", got)
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
			{Login: "alex", Name: "Alex", PermissionKey: "admin", GrantedAt: time.Unix(1757000000, 0)},
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
		"alex (Alex): admin", "BANNED", "Managers", "lead", "Superusers", "root",
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
		t.Fatal("an unconfigured Status URL must be an error")
	}
}
