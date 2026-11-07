package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
)

// unfolded undoes ICS line folding, so a long line can be matched whole.
func unfolded(body string) string {
	return strings.ReplaceAll(body, "\r\n ", "")
}

func (api *liveAPI) feed(target string, headers map[string]string) (reply, string) {
	api.t.Helper()
	return api.send(http.MethodGet, target, "", headers)
}

func TestLiveFeedRoutes(t *testing.T) {
	api := newLiveAPI(t)
	subteam := api.createSubteam("Software")
	event := api.publish(api.createEvent(subteam.ID, map[string]any{"recurrence_until": ""}))
	draft := api.createEvent(subteam.ID, map[string]any{"title": "Still a draft"})
	target := "/v1/feeds/scopes/" + subteam.ID + ".ics"

	response, body := api.feed(target, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d: %s", target, response.StatusCode, body)
	}
	wantHeaders := map[string]string{
		"Content-Type":        "text/calendar; charset=utf-8",
		"Content-Disposition": `inline; filename="` + subteam.SlugPath + `.ics"`,
		"Cache-Control":       "public, max-age=60, stale-while-revalidate=300",
	}
	for name, want := range wantHeaders {
		if got := response.Header.Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	body = unfolded(body)
	for _, want := range []string{
		"X-WR-CALNAME:BioTron — " + api.project.Name + " · Software",
		"URL:http://localhost:8083/v1/feeds/scopes/" + subteam.ID + ".ics",
		"UID:" + event.UID,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("the feed is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, draft.UID) {
		t.Fatal("a draft reached a public feed")
	}
	// A scope feed is exact: the project's own feed has none of its
	// subteam's events.
	if _, projectFeed := api.feed("/v1/feeds/scopes/"+api.projectID+".ics", nil); strings.Contains(projectFeed, event.UID) {
		t.Fatal("a project feed inherited its subteam's event")
	}

	// The ETag revalidates, in the weak form a client may send back too.
	etag := response.Header.Get("ETag")
	for _, header := range []string{etag, "W/" + etag} {
		revalidated, empty := api.feed(target, map[string]string{"If-None-Match": header})
		if revalidated.StatusCode != http.StatusNotModified || empty != "" || revalidated.Header.Get("ETag") != etag {
			t.Fatalf("If-None-Match %s returned %d with %q", header, revalidated.StatusCode, empty)
		}
	}

	// A GET route answers HEAD too, with the same validators. net/http's
	// server drops the body; a recorder keeps it, so it is not checked here.
	head, _ := api.send(http.MethodHead, target, "", nil)
	if head.StatusCode != http.StatusOK || head.Header.Get("ETag") != etag || head.Header.Get("Content-Type") != wantHeaders["Content-Type"] {
		t.Fatalf("HEAD returned %d with ETag %q", head.StatusCode, head.Header.Get("ETag"))
	}

	// Another spelling of the same UUID is the same feed.
	if alias, _ := api.feed("/v1/feeds/scopes/"+strings.ToUpper(subteam.ID)+".ics", nil); alias.StatusCode != http.StatusOK {
		t.Fatalf("an upper-case feed id returned %d", alias.StatusCode)
	}
	for _, missing := range []string{
		"/v1/feeds/scopes/" + uuid.NewString() + ".ics",
		"/v1/feeds/scopes/" + subteam.ID,
		"/v1/feeds/scopes/not-a-uuid.ics",
	} {
		if unknown, _ := api.feed(missing, nil); unknown.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s returned %d, want 404", missing, unknown.StatusCode)
		}
	}

	// An archived scope keeps its feed, so existing subscriptions keep
	// reconciling instead of losing every event at once.
	api.call(http.MethodPost, "/v1/admin/scopes/"+api.projectID+"/archive", "", http.StatusOK, nil)
	archived, archivedBody := api.feed(target, nil)
	if archived.StatusCode != http.StatusOK || !strings.Contains(archivedBody, "UID:"+event.UID) {
		t.Fatalf("the feed of an archived scope returned %d:\n%s", archived.StatusCode, archivedBody)
	}
	if _, all := api.feed("/v1/feeds/all.ics", nil); !strings.Contains(all, "UID:"+event.UID) {
		t.Fatal("all.ics dropped an event of an archived scope")
	}
}

// The repro of the review, through the real routes: a client that
// revalidates by date must see a delete.
func TestLiveFeedIsNotNotModifiedAfterAnEventIsDeleted(t *testing.T) {
	api := newLiveAPI(t)
	kept := api.publish(api.createEvent(api.projectID, map[string]any{"title": "Kept"}))
	deleted := api.publish(api.createEvent(api.projectID, map[string]any{"title": "Deleted"}))

	targets := []string{"/v1/feeds/all.ics", "/v1/feeds/scopes/" + api.projectID + ".ics"}
	lastModified := map[string]string{}
	for _, target := range targets {
		first, body := api.feed(target, nil)
		if !strings.Contains(body, "UID:"+deleted.UID) {
			t.Fatalf("%s is missing the event before the delete", target)
		}
		lastModified[target] = first.Header.Get("Last-Modified")
	}
	api.call(http.MethodDelete, "/v1/admin/events/"+deleted.ID, "", http.StatusNoContent, nil)
	for _, target := range targets {
		again, after := api.feed(target, map[string]string{"If-Modified-Since": lastModified[target]})
		if again.StatusCode != http.StatusOK {
			t.Fatalf("%s answered %d to If-Modified-Since after a delete, want 200", target, again.StatusCode)
		}
		if strings.Contains(after, "UID:"+deleted.UID) || !strings.Contains(after, "UID:"+kept.UID) {
			t.Fatalf("%s after the delete:\n%s", target, after)
		}
	}
}

// The occurrence list honours the archive and the scope filter.
func TestLiveOccurrencesFollowTheScopeTree(t *testing.T) {
	api := newLiveAPI(t)
	subteam := api.createSubteam("Software")
	event := api.publish(api.createEvent(subteam.ID, nil))
	listed := func(target string) []model.Occurrence {
		t.Helper()
		var occurrences []model.Occurrence
		api.call(http.MethodGet, target, "", http.StatusOK, &occurrences)
		return occurrences
	}
	window := "/v1/events?from=2026-12-01&to=2027-01-01&scope_id="
	if got := listed(window + subteam.ID); len(got) != 5 || got[0].SeriesID != event.ID {
		t.Fatalf("a weekly series over December listed %d occurrences: %+v", len(got), got)
	}
	if got := listed(window + api.projectID); len(got) != 0 {
		t.Fatalf("a project's filter listed its subteam's occurrences: %+v", got)
	}
	api.call(http.MethodPost, "/v1/admin/scopes/"+api.projectID+"/archive", "", http.StatusOK, nil)
	if got := listed(window + subteam.ID); len(got) != 0 {
		t.Fatalf("an archived project's subteam still lists occurrences: %+v", got)
	}
}
