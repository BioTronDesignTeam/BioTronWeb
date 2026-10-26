package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

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
	id := strings.TrimSuffix(r.PathValue("file"), ".ics")
	if uuid.Validate(id) != nil {
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
	return sendCacheable(w, r, feed.Content, lastModified, true)
}

// sendCacheable gives every public read the same validators: a strong
// content-hash ETag, a Last-Modified derived from the data, and a short shared
// cache window. honourModifiedSince is false for resources whose content also
// changes with the clock, where an unchanged Last-Modified would hand a client
// a stale 304 long after the body should have changed.
func sendCacheable(w http.ResponseWriter, r *http.Request, body []byte, lastModified time.Time, honourModifiedSince bool) error {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(body))
	lastModified = lastModified.UTC().Truncate(time.Second)
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastModified.Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" {
		if etagMatches(ifNoneMatch, etag) {
			return notModified(w)
		}
	} else if modifiedSince := r.Header.Get("If-Modified-Since"); honourModifiedSince && modifiedSince != "" {
		if parsed, err := http.ParseTime(modifiedSince); err == nil && !lastModified.After(parsed) {
			return notModified(w)
		}
	}
	writeBody(w, http.StatusOK, body)
	return nil
}

// A 304 must not carry a body. net/http drops one for this status, and also
// the Content-Type and Content-Length, which Fiber used to send with it; the
// validators and Cache-Control stay.
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
