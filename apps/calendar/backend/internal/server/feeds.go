package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/ical"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
)

func (h *Handler) allFeed(w http.ResponseWriter, r *http.Request) error {
	series, err := h.store.ListFeedSeries(r.Context(), "")
	if err != nil {
		return err
	}
	return h.sendFeed(w, r, "All BioTron events", "all-biotron-events.ics", h.config.PublicBaseURL+"/v1/feeds/all.ics", time.Time{}, series)
}

func (h *Handler) scopeFeed(w http.ResponseWriter, r *http.Request) error {
	file, ics := strings.CutSuffix(r.PathValue("file"), ".ics")
	id, ok := canonicalID(file)
	if !ics || !ok {
		return errNotFound
	}
	scope, err := h.store.GetScope(r.Context(), id)
	if err != nil {
		return err
	}
	series, err := h.store.ListFeedSeries(r.Context(), scope.ID)
	if err != nil {
		return err
	}
	// The path, not the leaf name: two subteams may both be called "Software",
	// and a subscriber ends up with both feeds side by side.
	name := scope.Name
	if scope.Kind != "TEAM" {
		name = "BioTron — " + scope.Path
	}
	filename := scope.SlugPath + ".ics"
	source := h.config.PublicBaseURL + "/v1/feeds/scopes/" + scope.ID + ".ics"
	return h.sendFeed(w, r, name, filename, source, scope.UpdatedAt, series)
}

func (h *Handler) sendFeed(w http.ResponseWriter, r *http.Request, name, filename, source string, metadataModified time.Time, series []model.EventSeries) error {
	feed, err := ical.Build(name, source, series, h.location)
	if err != nil {
		return err
	}
	lastModified := feed.LastModified
	if metadataModified.After(lastModified) {
		lastModified = metadataModified
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safeFilename(filename)))
	return sendCacheable(w, r, feed.Content, lastModified)
}

// sendCacheable gives every public read the same validators: a strong
// content-hash ETag, an informational Last-Modified, and a short shared cache
// window.
//
// Only If-None-Match can earn a 304; If-Modified-Since is never honoured.
// Last-Modified is the newest updated_at among the rows a response still
// holds, and that misses changes a client must see:
//   - a deleted series leaves no row behind, so the date stays put or goes
//     back, and a feed would answer 304 while still listing the deleted event;
//   - renaming a parent project renames a subteam's feed without touching the
//     subteam's row;
//   - the header has one-second resolution, so a second edit in the same
//     second as a fetch is invisible;
//   - the upcoming list slides with the clock while its rows stay the same.
//
// The ETag hashes the exact bytes sent, so it changes whenever anything a
// client would see changes. A client that revalidates by date alone gets a
// full 200 every time. That costs bandwidth, never correctness, and the
// server builds the body either way to compute the ETag.
func sendCacheable(w http.ResponseWriter, r *http.Request, body []byte, lastModified time.Time) error {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(body))
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastModified.UTC().Truncate(time.Second).Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" && etagMatches(ifNoneMatch, etag) {
		return notModified(w)
	}
	_, _ = w.Write(body)
	return nil
}

// A 304 carries no body; the validators and Cache-Control stay.
func notModified(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNotModified)
	return nil
}

// noContent answers a delete that left nothing to return.
func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// queryOr is a query parameter, or fallback when it is absent or empty.
func queryOr(r *http.Request, name, fallback string) string {
	if value := r.URL.Query().Get(name); value != "" {
		return value
	}
	return fallback
}

// etagMatches implements RFC 9110 weak comparison for If-None-Match: the header
// is a comma-separated list, "*" matches anything, and a client may return a
// validator we sent as strong with a W/ prefix. Comparing the raw header string
// for equality made every conditional request from such a client a full 200.
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}

func safeFilename(value string) string {
	var output strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			output.WriteRune(r)
		}
	}
	if output.Len() == 0 {
		return "calendar.ics"
	}
	return output.String()
}
