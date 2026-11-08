package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// feedFixture is shaped like Calendar's own all.ics: CRLF lines, a folded
// SUMMARY, a weekly series with one moved and one cancelled occurrence, a
// cancelled single event, and an all-day series.
var feedFixture = strings.Join([]string{
	"BEGIN:VCALENDAR",
	"PRODID:-//BioTron Design Team//BioTron Calendar//EN",
	"VERSION:2.0",
	"BEGIN:VTIMEZONE",
	"TZID:America/Toronto",
	"END:VTIMEZONE",
	"BEGIN:VEVENT",
	"UID:weekly@biotron",
	"SEQUENCE:6",
	"DTSTART;TZID=America/Toronto:20260903T180000",
	"DTEND;TZID=America/Toronto:20260903T190000",
	// The last start is 2026-10-01 18:00 Toronto, 22:00 UTC.
	"RRULE:FREQ=WEEKLY;UNTIL=20261001T220000Z",
	"SUMMARY:Weekly build night with a title long enough that Calendar folds",
	"  it onto a second line",
	"STATUS:CONFIRMED",
	"END:VEVENT",
	"BEGIN:VEVENT",
	"UID:weekly@biotron",
	"RECURRENCE-ID;TZID=America/Toronto:20260910T180000",
	"DTSTART;TZID=America/Toronto:20260910T190000",
	"STATUS:CONFIRMED",
	"END:VEVENT",
	"BEGIN:VEVENT",
	"UID:weekly@biotron",
	"RECURRENCE-ID;TZID=America/Toronto:20260917T180000",
	"DTSTART;TZID=America/Toronto:20260917T180000",
	"STATUS:CANCELLED",
	"END:VEVENT",
	"BEGIN:VEVENT",
	"UID:single@biotron",
	"DTSTART;TZID=America/Toronto:20260912T100000",
	"STATUS:CANCELLED",
	"END:VEVENT",
	"BEGIN:VEVENT",
	"UID:allday@biotron",
	"DTSTART;VALUE=DATE:20260905",
	"RRULE:FREQ=WEEKLY;UNTIL=20260919",
	"STATUS:CONFIRMED",
	"END:VEVENT",
	"END:VCALENDAR",
	"",
}, "\r\n")

func TestFeedStatus(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	feed, err := ParseFeed([]byte(feedFixture), toronto)
	if err != nil {
		t.Fatalf("ParseFeed: %v", err)
	}
	cases := []struct {
		name, uid, recurrence string
		want                  Status
	}{
		{"a plain weekly occurrence", "weekly@biotron", "2026-09-24T18:00:00", StatusExists},
		{"the first occurrence", "weekly@biotron", "2026-09-03T18:00:00", StatusExists},
		{"the last occurrence, by its UTC UNTIL", "weekly@biotron", "2026-10-01T18:00:00", StatusExists},
		{"an occurrence moved by an override", "weekly@biotron", "2026-09-10T18:00:00", StatusExists},
		{"an occurrence cancelled by an override", "weekly@biotron", "2026-09-17T18:00:00", StatusCancelled},
		{"past the series' end", "weekly@biotron", "2026-10-08T18:00:00", StatusDeleted},
		{"a time the series no longer starts at", "weekly@biotron", "2026-09-24T17:00:00", StatusDeleted},
		{"a day the series does not repeat on", "weekly@biotron", "2026-09-25T18:00:00", StatusDeleted},
		{"before the series starts", "weekly@biotron", "2026-08-27T18:00:00", StatusDeleted},
		{"a cancelled single event", "single@biotron", "2026-09-12T10:00:00", StatusCancelled},
		{"an all-day occurrence", "allday@biotron", "2026-09-19T00:00:00", StatusExists},
		{"past an all-day series' end", "allday@biotron", "2026-09-26T00:00:00", StatusDeleted},
		{"a deleted series", "gone@biotron", "2026-09-10T18:00:00", StatusDeleted},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := feed.Status(testCase.uid, testCase.recurrence); got != testCase.want {
				t.Fatalf("Status = %s, want %s", got, testCase.want)
			}
		})
	}
}

// An error page or a cut-off body must never parse as an empty calendar,
// which would read as every series deleted.
func TestParseFeedRefusesWhatIsNotAWholeCalendar(t *testing.T) {
	for name, body := range map[string]string{
		"empty":       "",
		"html":        "<html>bad gateway</html>",
		"cut off":     strings.SplitAfter(feedFixture, "END:VEVENT\r\n")[0],
		"no ending":   "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n",
		"no uid":      "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20260905\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
		"bad dtstart": "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:soon\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseFeed([]byte(body), time.UTC); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestClientFeedReadsAllEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/feeds/all.ics" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(feedFixture))
	}))
	defer server.Close()

	feed, err := NewClient(server.URL, time.UTC).Feed(context.Background())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if got := feed.Status("single@biotron", "2026-09-12T10:00:00"); got != StatusCancelled {
		t.Fatalf("Status = %s", got)
	}
}

func TestClientFeedNon200IsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if _, err := NewClient(server.URL, time.UTC).Feed(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}
