package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
)

// A deleted series leaves no row behind, so the feed's Last-Modified stays put
// or goes back. A client that revalidated with If-Modified-Since used to get a
// 304 and keep the deleted meeting for good. Only the content-hash ETag may
// answer 304 now.
func TestFeedIsNotNotModifiedAfterASeriesIsDeleted(t *testing.T) {
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{location: location}
	earlier := newSeries(t, "a", "Stays", "2026-11-03T10:00:00", "2026-11-03T11:00:00")
	earlier.UpdatedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	later := newSeries(t, "b", "Deleted", "2026-11-04T10:00:00", "2026-11-04T11:00:00")
	later.UpdatedAt = earlier.UpdatedAt.Add(time.Hour)

	send := func(request *http.Request, series ...model.EventSeries) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		if err := handler.sendFeed(recorder, request, "All", "all.ics", "", time.Time{}, series); err != nil {
			t.Fatal(err)
		}
		return recorder
	}

	first := send(httptest.NewRequest(http.MethodGet, "/v1/feeds/all.ics", nil), earlier, later)
	lastModified, etag := first.Header().Get("Last-Modified"), first.Header().Get("ETag")

	// Series b is deleted, and the client revalidates by date.
	byDate := httptest.NewRequest(http.MethodGet, "/v1/feeds/all.ics", nil)
	byDate.Header.Set("If-Modified-Since", lastModified)
	after := send(byDate, earlier)
	if after.Code != http.StatusOK {
		t.Fatalf("If-Modified-Since after a delete returned %d, want 200", after.Code)
	}
	if strings.Contains(after.Body.String(), "UID:b@biotron.ca") {
		t.Fatal("the deleted series is still in the feed")
	}

	// Revalidating by date alone is never a 304, even with nothing changed.
	unchanged := httptest.NewRequest(http.MethodGet, "/v1/feeds/all.ics", nil)
	unchanged.Header.Set("If-Modified-Since", time.Now().UTC().Format(http.TimeFormat))
	if code := send(unchanged, earlier).Code; code != http.StatusOK {
		t.Fatalf("If-Modified-Since alone returned %d, want 200", code)
	}

	// The old ETag no longer matches, and the new one does.
	byTag := httptest.NewRequest(http.MethodGet, "/v1/feeds/all.ics", nil)
	byTag.Header.Set("If-None-Match", etag)
	if code := send(byTag, earlier).Code; code != http.StatusOK {
		t.Fatalf("the ETag from before the delete returned %d, want 200", code)
	}
	byTag.Header.Set("If-None-Match", after.Header().Get("ETag"))
	if code := send(byTag, earlier).Code; code != http.StatusNotModified {
		t.Fatalf("the current ETag returned %d, want 304", code)
	}
}
