package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

// These drive the whole API, as server.New builds it, against a real database
// with an editor signed in. Set CALENDAR_TEST_DATABASE_URL as for the other
// live tests; without it they skip. Each test works under a project of its
// own, named at random, and removes it when it ends, so the database can be
// shared.

// liveAPI is the real handler and the project a test owns.
type liveAPI struct {
	t         *testing.T
	app       http.Handler
	pool      *pgxpool.Pool
	ctx       context.Context
	project   model.Scope
	projectID string
}

func newLiveAPI(t *testing.T) *liveAPI {
	t.Helper()
	databaseURL := os.Getenv("CALENDAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CALENDAR_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	calendarStore, err := store.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(calendarStore.Close)
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		FrontendURL: "http://localhost:5176", PublicBaseURL: "http://localhost:8083",
		MaxRangeDays: 370, DefaultTimezone: "America/Toronto",
	}
	api := &liveAPI{
		t:    t,
		app:  New(cfg, calendarStore, editorAuth(t).Client(AppID), logclient.NewFromEnv("calendar-api"), location),
		pool: fixturePool(t, ctx, databaseURL),
		ctx:  ctx,
	}
	name := "Live " + uuid.NewString()[:8]
	api.call(http.MethodPost, "/v1/admin/scopes",
		fmt.Sprintf(`{"kind":"PROJECT","name":%q,"parent_id":%q}`, name, liveTeamScopeID),
		http.StatusCreated, &api.project)
	api.projectID = api.project.ID
	t.Cleanup(api.clean)
	return api
}

// clean removes the test's project, its subteams and their events. The
// parent foreign key is ON DELETE RESTRICT, so events go first, then
// children.
func (api *liveAPI) clean() {
	subtree := `SELECT id FROM calendar_scopes WHERE id = $1::uuid OR parent_id = $1::uuid`
	for _, statement := range []string{
		`DELETE FROM event_series WHERE scope_id IN (` + subtree + `)`,
		`DELETE FROM calendar_scopes WHERE parent_id = $1::uuid`,
		`DELETE FROM calendar_scopes WHERE id = $1::uuid`,
	} {
		if _, err := api.pool.Exec(api.ctx, statement, api.projectID); err != nil {
			api.t.Errorf("clean: %v", err)
		}
	}
}

// reply is the status and headers of a response whose body has been read.
type reply struct {
	StatusCode int
	Header     http.Header
}

// send makes one request as the signed-in editor and returns the response
// with its body read.
func (api *liveAPI) send(method, target, body string, headers map[string]string) (reply, string) {
	api.t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	request.Header.Set("Cookie", "session=abc")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := serveTest(api.app, request)
	defer response.Body.Close()
	read, err := io.ReadAll(response.Body)
	if err != nil {
		api.t.Fatal(err)
	}
	return reply{StatusCode: response.StatusCode, Header: response.Header}, string(read)
}

// call makes one request, fails unless it got want, and decodes a JSON
// answer into out when out is not nil. It returns the body.
func (api *liveAPI) call(method, target, body string, want int, out any) string {
	api.t.Helper()
	response, read := api.send(method, target, body, nil)
	if response.StatusCode != want {
		api.t.Fatalf("%s %s returned %d, want %d: %s", method, target, response.StatusCode, want, read)
	}
	if out != nil {
		if err := json.Unmarshal([]byte(read), out); err != nil {
			api.t.Fatalf("decode %s %s: %v: %s", method, target, err, read)
		}
	}
	return read
}

// refused makes one request and fails unless it got want with an error that
// mentions reason.
func (api *liveAPI) refused(method, target, body string, want int, reason string) {
	api.t.Helper()
	read := api.call(method, target, body, want, nil)
	if !strings.Contains(read, reason) {
		api.t.Fatalf("%s %s answered %s, want an error about %q", method, target, read, reason)
	}
}

// eventBody is a whole event as the editor sends it: a weekly meeting from
// 2026-12-01 18:00 to 19:00 until 2026-12-29, with changes laid over it.
func eventBody(scopeID string, changes map[string]any) string {
	body := map[string]any{
		"scope_id": scopeID, "title": "Build night",
		"starts_at_local": "2026-12-01T18:00:00", "ends_at_local": "2026-12-01T19:00:00",
		"recurrence_until": "2026-12-29",
	}
	for key, value := range changes {
		body[key] = value
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func (api *liveAPI) createEvent(scopeID string, changes map[string]any) model.EventSeries {
	api.t.Helper()
	var event model.EventSeries
	api.call(http.MethodPost, "/v1/admin/events", eventBody(scopeID, changes), http.StatusCreated, &event)
	return event
}

func (api *liveAPI) publish(event model.EventSeries) model.EventSeries {
	api.t.Helper()
	api.call(http.MethodPost, "/v1/admin/events/"+event.ID+"/publish",
		fmt.Sprintf(`{"expected_sequence":%d}`, event.Sequence), http.StatusOK, &event)
	return event
}

func (api *liveAPI) createSubteam(name string) model.Scope {
	api.t.Helper()
	var scope model.Scope
	api.call(http.MethodPost, "/v1/admin/scopes",
		fmt.Sprintf(`{"kind":"SUBTEAM","name":%q,"parent_id":%q}`, name, api.projectID),
		http.StatusCreated, &scope)
	return scope
}

func occurrenceBody(recurrenceID, state, patch string, sequence int) string {
	return fmt.Sprintf(`{"recurrence_id_local":%q,"state":%q,"patch":%s,"expected_sequence":%d}`,
		recurrenceID, state, patch, sequence)
}

// An edit of a series that has a changed occurrence must not silently throw
// the change away: moving the series, ending it before the change, or making
// it a one-off are refused until the change is reset. A plain field edit
// still goes through.
func TestLiveSeriesEditsThatWouldDiscardOccurrenceChangesAre409(t *testing.T) {
	api := newLiveAPI(t)
	event := api.publish(api.createEvent(api.projectID, nil))
	occurrences := "/v1/admin/events/" + event.ID + "/occurrences"
	api.call(http.MethodPut, occurrences,
		occurrenceBody("2026-12-15T18:00:00", "MODIFIED", `{"title":"Moved"}`, event.Sequence), http.StatusOK, nil)
	sequence := event.Sequence + 1
	target := "/v1/admin/events/" + event.ID

	api.refused(http.MethodPatch, target, eventBody(api.projectID, map[string]any{
		"starts_at_local": "2026-12-01T17:00:00", "ends_at_local": "2026-12-01T18:00:00", "expected_sequence": sequence,
	}), http.StatusConflict, "remove occurrence changes before")
	api.refused(http.MethodPatch, target, eventBody(api.projectID, map[string]any{
		"all_day": true, "starts_at_local": "2026-12-01T00:00:00", "ends_at_local": "2026-12-02T00:00:00", "expected_sequence": sequence,
	}), http.StatusConflict, "remove occurrence changes before")
	api.refused(http.MethodPatch, target, eventBody(api.projectID, map[string]any{
		"recurrence_until": "2026-12-08", "expected_sequence": sequence,
	}), http.StatusConflict, "would discard an occurrence change")
	api.refused(http.MethodPatch, target, eventBody(api.projectID, map[string]any{
		"recurrence_until": "", "expected_sequence": sequence,
	}), http.StatusConflict, "one-off")

	var updated model.EventSeries
	api.call(http.MethodPatch, target, eventBody(api.projectID, map[string]any{
		"title": "Build night (room change)", "recurrence_until": "2026-12-15", "expected_sequence": sequence,
	}), http.StatusOK, &updated)
	if updated.Title != "Build night (room change)" || updated.Sequence != sequence+1 || len(updated.Overrides) != 1 {
		t.Fatalf("a title edit did not keep the occurrence change: %+v", updated)
	}
	// A stale sequence is someone else's edit, not this one.
	api.refused(http.MethodPatch, target, eventBody(api.projectID, map[string]any{"expected_sequence": sequence}),
		http.StatusConflict, "conflicts")

	// Once the change is reset, the structural edit goes through.
	api.call(http.MethodDelete, fmt.Sprintf("%s?recurrence_id_local=2026-12-15T18:00:00&expected_sequence=%d", occurrences, updated.Sequence),
		"", http.StatusNoContent, nil)
	api.call(http.MethodPatch, target, eventBody(api.projectID, map[string]any{
		"starts_at_local": "2026-12-01T17:00:00", "ends_at_local": "2026-12-01T18:00:00", "expected_sequence": updated.Sequence + 1,
	}), http.StatusOK, nil)

	// A published event cannot move feeds.
	other := api.createSubteam("Elsewhere")
	api.refused(http.MethodPatch, target, eventBody(other.ID, map[string]any{"expected_sequence": updated.Sequence + 2}),
		http.StatusConflict, "cannot move feeds")
}

// Every refusal upsertOccurrence makes, through the route.
func TestLiveOccurrenceChangeValidation(t *testing.T) {
	api := newLiveAPI(t)
	event := api.publish(api.createEvent(api.projectID, nil))
	occurrences := "/v1/admin/events/" + event.ID + "/occurrences"
	put := func(body string, want int, reason string) {
		t.Helper()
		api.refused(http.MethodPut, occurrences, body, want, reason)
	}
	moved := `{"title":"Moved"}`

	put(occurrenceBody("2026-12-16T18:00:00", "MODIFIED", moved, event.Sequence), http.StatusBadRequest, "not an occurrence")
	put(occurrenceBody("2027-01-05T18:00:00", "MODIFIED", moved, event.Sequence), http.StatusBadRequest, "not an occurrence")
	put(occurrenceBody("2026-12-15T18:00:00", "PAUSED", moved, event.Sequence), http.StatusBadRequest, "MODIFIED or CANCELLED")
	put(occurrenceBody("2026-12-15T18:00:00", "MODIFIED", `{"ends_at_local":"2026-12-15T17:00:00"}`, event.Sequence),
		http.StatusBadRequest, "end after it starts")
	put(occurrenceBody("2026-12-15T18:00:00", "MODIFIED", `{"colour":"red"}`, event.Sequence), http.StatusBadRequest, "not allowed")
	put(occurrenceBody("2026-12-15T18:00:00", "MODIFIED", `{"title":"   "}`, event.Sequence), http.StatusBadRequest, "title")
	put(occurrenceBody("2026-12-15T18:00:00", "MODIFIED", moved, event.Sequence-1), http.StatusConflict, "conflicts")

	allDay := api.publish(api.createEvent(api.projectID, map[string]any{
		"all_day": true, "starts_at_local": "2026-12-01T00:00:00", "ends_at_local": "2026-12-02T00:00:00",
	}))
	api.refused(http.MethodPut, "/v1/admin/events/"+allDay.ID+"/occurrences",
		occurrenceBody("2026-12-15T00:00:00", "MODIFIED", `{"starts_at_local":"2026-12-15T09:00:00"}`, allDay.Sequence),
		http.StatusBadRequest, "midnight")

	draft := api.createEvent(api.projectID, nil)
	api.refused(http.MethodPut, "/v1/admin/events/"+draft.ID+"/occurrences",
		occurrenceBody("2026-12-15T18:00:00", "MODIFIED", moved, draft.Sequence), http.StatusConflict, "only a published series")

	oneOff := api.publish(api.createEvent(api.projectID, map[string]any{"recurrence_until": ""}))
	api.refused(http.MethodPut, "/v1/admin/events/"+oneOff.ID+"/occurrences",
		occurrenceBody("2026-12-01T18:00:00", "MODIFIED", moved, oneOff.Sequence), http.StatusConflict, "one-off")

	// A valid cancellation goes through and bumps the series; resetting it
	// twice finds nothing the second time.
	var cancelled model.EventOverride
	api.call(http.MethodPut, occurrences, occurrenceBody("2026-12-15T18:00:00", "cancelled", moved, event.Sequence),
		http.StatusOK, &cancelled)
	if cancelled.State != model.OverrideCancelled || string(cancelled.Patch) != "{}" {
		t.Fatalf("a cancellation kept a patch or lost its state: %+v", cancelled)
	}
	reset := occurrences + "?recurrence_id_local=2026-12-15T18:00:00&expected_sequence="
	api.call(http.MethodDelete, reset+fmt.Sprint(event.Sequence+1), "", http.StatusNoContent, nil)
	api.call(http.MethodDelete, reset+fmt.Sprint(event.Sequence+2), "", http.StatusNotFound, nil)
	api.refused(http.MethodDelete, occurrences+"?recurrence_id_local=2026-12-15T18:00:00", "", http.StatusBadRequest, "expected_sequence")
}

// The scope tree's rules, through the routes: the kinds that can nest, what
// an archived ancestor blocks, and what may be deleted.
func TestLiveScopeArchiveAndDeleteRules(t *testing.T) {
	api := newLiveAPI(t)
	project := "/v1/admin/scopes/" + api.projectID
	subteam := api.createSubteam("Software")
	draft := api.createEvent(subteam.ID, nil)
	published := api.publish(api.createEvent(subteam.ID, map[string]any{"title": "Published"}))

	api.refused(http.MethodPost, "/v1/admin/scopes",
		fmt.Sprintf(`{"kind":"PROJECT","name":"Nested","parent_id":%q}`, api.projectID), http.StatusConflict, "compatible")
	api.refused(http.MethodPost, "/v1/admin/scopes",
		fmt.Sprintf(`{"kind":"SUBTEAM","name":"Loose","parent_id":%q}`, liveTeamScopeID), http.StatusConflict, "compatible")
	api.refused(http.MethodPost, "/v1/admin/scopes",
		fmt.Sprintf(`{"kind":"TEAM","name":"Second root","parent_id":%q}`, liveTeamScopeID), http.StatusBadRequest, "PROJECT or SUBTEAM")
	api.call(http.MethodPost, "/v1/admin/scopes",
		fmt.Sprintf(`{"kind":"SUBTEAM","name":"Software","parent_id":%q}`, api.projectID), http.StatusConflict, nil)

	// Only an empty leaf can be deleted, and never the team root.
	api.call(http.MethodDelete, project, "", http.StatusConflict, nil)
	api.call(http.MethodDelete, "/v1/admin/scopes/"+subteam.ID, "", http.StatusConflict, nil)
	api.call(http.MethodDelete, "/v1/admin/scopes/"+liveTeamScopeID, "", http.StatusConflict, nil)
	api.call(http.MethodPost, "/v1/admin/scopes/"+liveTeamScopeID+"/archive", "", http.StatusNotFound, nil)

	// An archived project blocks new work anywhere below it, and hides its
	// subtree from the public list while the editor still sees it.
	var archived model.Scope
	api.call(http.MethodPost, project+"/archive", "", http.StatusOK, &archived)
	if archived.Status != model.ScopeArchived || archived.ArchivedAt == nil {
		t.Fatalf("archive returned %+v", archived)
	}
	api.refused(http.MethodPost, "/v1/admin/scopes",
		fmt.Sprintf(`{"kind":"SUBTEAM","name":"Late","parent_id":%q}`, api.projectID), http.StatusConflict, "not an active")
	api.refused(http.MethodPost, "/v1/admin/events", eventBody(subteam.ID, nil), http.StatusConflict, "active scope")
	api.refused(http.MethodPost, "/v1/admin/events/"+draft.ID+"/publish",
		fmt.Sprintf(`{"expected_sequence":%d}`, draft.Sequence), http.StatusConflict, "archived scope")
	if public := api.call(http.MethodGet, "/v1/scopes", "", http.StatusOK, nil); strings.Contains(public, subteam.ID) {
		t.Fatal("the public scope list still shows a subteam of an archived project")
	}
	if admin := api.call(http.MethodGet, "/v1/admin/scopes", "", http.StatusOK, nil); !strings.Contains(admin, subteam.ID) {
		t.Fatal("the editor's scope list lost a subteam of an archived project")
	}

	// A child cannot be restored under an archived parent.
	api.call(http.MethodPost, "/v1/admin/scopes/"+subteam.ID+"/archive", "", http.StatusOK, nil)
	api.refused(http.MethodPost, "/v1/admin/scopes/"+subteam.ID+"/restore", "", http.StatusConflict, "restore the parent project first")
	api.call(http.MethodPost, project+"/restore", "", http.StatusOK, nil)
	api.call(http.MethodPost, "/v1/admin/scopes/"+subteam.ID+"/restore", "", http.StatusOK, nil)

	// Events go in any state, then the empty subteam can.
	api.call(http.MethodDelete, "/v1/admin/events/"+draft.ID, "", http.StatusNoContent, nil)
	api.call(http.MethodDelete, "/v1/admin/events/"+published.ID, "", http.StatusNoContent, nil)
	api.call(http.MethodDelete, "/v1/admin/events/"+published.ID, "", http.StatusNotFound, nil)
	api.call(http.MethodDelete, "/v1/admin/scopes/"+subteam.ID, "", http.StatusNoContent, nil)
	api.call(http.MethodDelete, "/v1/admin/scopes/"+subteam.ID, "", http.StatusNotFound, nil)
	var events []model.EventSeries
	api.call(http.MethodGet, "/v1/admin/events?scope_id="+subteam.ID, "", http.StatusOK, &events)
	if len(events) != 0 {
		t.Fatalf("deleted events are still listed: %+v", events)
	}
}
