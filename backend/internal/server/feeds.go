package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/ical"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

func (h *Handler) allFeed(c *fiber.Ctx) error {
	series, err := h.store.ListFeedSeries(c.UserContext(), "")
	if err != nil {
		return err
	}
	return h.sendFeed(c, "All BioTron events", "all-biotron-events.ics", h.config.PublicBaseURL+"/v1/feeds/all.ics", time.Time{}, series)
}

func (h *Handler) scopeFeed(c *fiber.Ctx) error {
	if uuid.Validate(c.Params("id")) != nil {
		return fiber.ErrNotFound
	}
	scope, err := h.store.GetScope(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	series, err := h.store.ListFeedSeries(c.UserContext(), scope.ID)
	if err != nil {
		return err
	}
	name := scope.Name
	if scope.Kind != "TEAM" {
		name = "BioTron — " + scope.Name
	}
	filename := scope.Slug + ".ics"
	source := h.config.PublicBaseURL + "/v1/feeds/scopes/" + scope.ID + ".ics"
	return h.sendFeed(c, name, filename, source, scope.UpdatedAt, series)
}

func (h *Handler) sendFeed(c *fiber.Ctx, name, filename, source string, metadataModified time.Time, series []model.EventSeries) error {
	feed, err := ical.Build(name, source, series, h.location)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(feed.Content)
	etag := fmt.Sprintf(`"%x"`, hash)
	lastModified := feed.LastModified.UTC().Truncate(time.Second)
	if metadataModified.After(lastModified) {
		lastModified = metadataModified.UTC().Truncate(time.Second)
	}
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderLastModified, lastModified.Format(http.TimeFormat))
	c.Set(fiber.HeaderCacheControl, "public, max-age=60, stale-while-revalidate=300")
	c.Set(fiber.HeaderContentType, "text/calendar; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`inline; filename="%s"`, safeFilename(filename)))
	if ifNoneMatch := c.Get(fiber.HeaderIfNoneMatch); ifNoneMatch != "" {
		if ifNoneMatch == etag {
			return c.SendStatus(fiber.StatusNotModified)
		}
	} else if modifiedSince := c.Get(fiber.HeaderIfModifiedSince); modifiedSince != "" {
		if parsed, err := http.ParseTime(modifiedSince); err == nil && !lastModified.After(parsed) {
			return c.SendStatus(fiber.StatusNotModified)
		}
	}
	return c.Send(feed.Content)
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
