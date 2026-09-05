package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/ical"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

func (h *Handler) allFeed(c fiber.Ctx) error {
	series, err := h.store.ListFeedSeries(c.Context(), "")
	if err != nil {
		return err
	}
	return h.sendFeed(c, "All BioTron events", "all-biotron-events.ics", h.config.PublicBaseURL+"/v1/feeds/all.ics", time.Time{}, series)
}

func (h *Handler) scopeFeed(c fiber.Ctx) error {
	if uuid.Validate(c.Params("id")) != nil {
		return fiber.ErrNotFound
	}
	scope, err := h.store.GetScope(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	series, err := h.store.ListFeedSeries(c.Context(), scope.ID)
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
	return h.sendFeed(c, name, filename, source, scope.UpdatedAt, series)
}

func (h *Handler) sendFeed(c fiber.Ctx, name, filename, source string, metadataModified time.Time, series []model.EventSeries) error {
	feed, err := ical.Build(name, source, series, h.location)
	if err != nil {
		return err
	}
	lastModified := feed.LastModified
	if metadataModified.After(lastModified) {
		lastModified = metadataModified
	}
	c.Set(fiber.HeaderContentType, "text/calendar; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`inline; filename="%s"`, safeFilename(filename)))
	return sendCacheable(c, feed.Content, lastModified, true)
}

// sendCacheable gives every public read the same validators: a strong
// content-hash ETag, a Last-Modified derived from the data, and a short shared
// cache window. honourModifiedSince is false for resources whose content also
// changes with the clock, where an unchanged Last-Modified would hand a client
// a stale 304 long after the body should have changed.
func sendCacheable(c fiber.Ctx, body []byte, lastModified time.Time, honourModifiedSince bool) error {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(body))
	lastModified = lastModified.UTC().Truncate(time.Second)
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderLastModified, lastModified.Format(http.TimeFormat))
	c.Set(fiber.HeaderCacheControl, "public, max-age=60, stale-while-revalidate=300")
	if ifNoneMatch := c.Get(fiber.HeaderIfNoneMatch); ifNoneMatch != "" {
		if etagMatches(ifNoneMatch, etag) {
			return notModified(c)
		}
	} else if modifiedSince := c.Get(fiber.HeaderIfModifiedSince); honourModifiedSince && modifiedSince != "" {
		if parsed, err := http.ParseTime(modifiedSince); err == nil && !lastModified.After(parsed) {
			return notModified(c)
		}
	}
	return c.Send(body)
}

// A 304 must not carry a body. Fiber v2's SendStatus wrote the status text as
// one whenever the body was empty, which is why this sends explicitly instead.
// Fiber v3 resets the body for statuses that disallow one and fasthttp drops it
// again on the way out, so the framework now enforces this too — the explicit
// send stays because it says what the response is meant to be rather than
// relying on two layers below it to agree.
func notModified(c fiber.Ctx) error {
	return c.Status(fiber.StatusNotModified).Send(nil)
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
