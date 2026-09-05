package server

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// --- Guards -----------------------------------------------------------------

type guardBody struct {
	GuildID    string   `json:"guild_id"`
	RoleIDs    []string `json:"role_ids"`
	ChannelIDs []string `json:"channel_ids"`
}

func (h *Handler) listGuards(c fiber.Ctx) error {
	guards, err := h.store.ListGuards(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(guards)
}

func (h *Handler) getGuard(c fiber.Ctx) error {
	subject, err := guardSubject(c)
	if err != nil {
		return err
	}
	guard, err := h.store.GetGuard(c.Context(), subject)
	if err != nil {
		return err
	}
	return c.JSON(guard)
}

func (h *Handler) putGuard(c fiber.Ctx) error {
	subject, err := guardSubject(c)
	if err != nil {
		return err
	}
	var body guardBody
	if err := bind(c, &body); err != nil {
		return err
	}
	if !isDiscordID(body.GuildID) {
		return badRequest("guild_id must be a Discord id")
	}
	if err := requireDiscordIDs("role_ids", body.RoleIDs); err != nil {
		return err
	}
	if err := requireDiscordIDs("channel_ids", body.ChannelIDs); err != nil {
		return err
	}
	// An empty role list would let nobody use the feature, which is a guard
	// nobody meant to write. Deleting the guard is how a feature is turned off.
	if len(body.RoleIDs) == 0 {
		return badRequest("role_ids must name at least one role")
	}
	guard, err := h.store.UpsertGuard(c.Context(), store.Guard{
		Subject:    subject,
		GuildID:    body.GuildID,
		RoleIDs:    body.RoleIDs,
		ChannelIDs: body.ChannelIDs,
	})
	if err != nil {
		return err
	}
	h.adminEvent(c, "Guard saved", map[string]any{
		"subject":  guard.Subject,
		"guild_id": guard.GuildID,
		"roles":    len(guard.RoleIDs),
		"channels": len(guard.ChannelIDs),
	})
	return c.JSON(guard)
}

func (h *Handler) deleteGuard(c fiber.Ctx) error {
	subject, err := guardSubject(c)
	if err != nil {
		return err
	}
	if err := h.store.DeleteGuard(c.Context(), subject); err != nil {
		return err
	}
	h.adminEvent(c, "Guard deleted", map[string]any{"subject": subject})
	return c.SendStatus(fiber.StatusNoContent)
}

func guardSubject(c fiber.Ctx) (string, error) {
	subject := c.Params("subject")
	switch subject {
	case store.SubjectAgent, store.SubjectAgentThread, store.SubjectAnnounce, store.SubjectNudge:
		return subject, nil
	default:
		return "", badRequest("unknown guard subject")
	}
}

// --- Automations ------------------------------------------------------------

// automationBody is one shape for create and update. Every field is a pointer
// so PATCH can tell "leave this alone" from "set this to zero", and so create
// can fill the defaults the operator did not send.
type automationBody struct {
	Kind          *string `json:"kind"`
	Name          *string `json:"name"`
	ScopeID       *string `json:"scope_id"`
	ChannelID     *string `json:"channel_id"`
	LeadUserID    *string `json:"lead_user_id"`
	LeadHours     *int    `json:"lead_hours"`
	LookbackHours *int    `json:"lookback_hours"`
	AnyAuthor     *bool   `json:"any_author"`
	PostHour      *int    `json:"post_hour"`
	Deliver       *string `json:"deliver"`
	Enabled       *bool   `json:"enabled"`
}

func (h *Handler) listAutomations(c fiber.Ctx) error {
	automations, err := h.store.ListAutomations(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(automations)
}

func (h *Handler) getAutomation(c fiber.Ctx) error {
	id, err := automationID(c)
	if err != nil {
		return err
	}
	automation, err := h.store.GetAutomation(c.Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(automation)
}

func (h *Handler) createAutomation(c fiber.Ctx) error {
	var body automationBody
	if err := bind(c, &body); err != nil {
		return err
	}
	// The defaults match the migration's, so a row created here and a row
	// created by hand behave the same.
	automation := store.Automation{
		LeadHours:     24,
		LookbackHours: 24,
		Deliver:       store.DeliverDM,
		Enabled:       true,
	}
	if body.Kind == nil || body.Name == nil || body.ScopeID == nil || body.ChannelID == nil {
		return badRequest("kind, name, scope_id and channel_id are required")
	}
	if err := apply(&automation, body); err != nil {
		return err
	}
	saved, err := h.store.CreateAutomation(c.Context(), automation)
	if err != nil {
		return err
	}
	h.adminEvent(c, "Automation created", map[string]any{
		"automation_id": saved.ID, "kind": saved.Kind,
		"name": saved.Name, "scope_id": saved.ScopeID,
	})
	return c.Status(fiber.StatusCreated).JSON(saved)
}

func (h *Handler) patchAutomation(c fiber.Ctx) error {
	id, err := automationID(c)
	if err != nil {
		return err
	}
	var body automationBody
	if err := bind(c, &body); err != nil {
		return err
	}
	// The row is read first so the update writes the fields the caller left
	// out unchanged. The store writes whole rows; the merge belongs here.
	automation, err := h.store.GetAutomation(c.Context(), id)
	if err != nil {
		return err
	}
	if err := apply(&automation, body); err != nil {
		return err
	}
	saved, err := h.store.UpdateAutomation(c.Context(), automation)
	if err != nil {
		return err
	}
	h.adminEvent(c, "Automation updated", map[string]any{
		"automation_id": saved.ID, "kind": saved.Kind, "enabled": saved.Enabled,
	})
	return c.JSON(saved)
}

func (h *Handler) deleteAutomation(c fiber.Ctx) error {
	id, err := automationID(c)
	if err != nil {
		return err
	}
	if err := h.store.DeleteAutomation(c.Context(), id); err != nil {
		return err
	}
	h.adminEvent(c, "Automation deleted", map[string]any{"automation_id": id})
	return c.SendStatus(fiber.StatusNoContent)
}

// apply merges a body onto an automation and validates the result. It runs on
// the merged row, not the body, so a PATCH cannot leave an invalid row behind
// by changing one half of a pair.
func apply(automation *store.Automation, body automationBody) error {
	if body.Kind != nil {
		automation.Kind = *body.Kind
	}
	if body.Name != nil {
		automation.Name = *body.Name
	}
	if body.ScopeID != nil {
		automation.ScopeID = *body.ScopeID
	}
	if body.ChannelID != nil {
		automation.ChannelID = *body.ChannelID
	}
	if body.LeadUserID != nil {
		// An empty string clears the lead; the column is nullable.
		if *body.LeadUserID == "" {
			automation.LeadUserID = nil
		} else {
			automation.LeadUserID = body.LeadUserID
		}
	}
	if body.LeadHours != nil {
		automation.LeadHours = *body.LeadHours
	}
	if body.LookbackHours != nil {
		automation.LookbackHours = *body.LookbackHours
	}
	if body.AnyAuthor != nil {
		automation.AnyAuthor = *body.AnyAuthor
	}
	if body.PostHour != nil {
		automation.PostHour = body.PostHour
	}
	if body.Deliver != nil {
		automation.Deliver = *body.Deliver
	}
	if body.Enabled != nil {
		automation.Enabled = *body.Enabled
	}
	return validateAutomation(*automation)
}

func validateAutomation(automation store.Automation) error {
	switch automation.Kind {
	case store.KindAnnounce, store.KindNudge:
	default:
		return badRequest("kind must be ANNOUNCE or NUDGE")
	}
	switch automation.Deliver {
	case store.DeliverDM, store.DeliverChannel:
	default:
		return badRequest("deliver must be DM or CHANNEL")
	}
	if name := automation.Name; name == "" || len(name) > 120 {
		return badRequest("name must be 1 to 120 characters")
	}
	if _, err := uuid.Parse(automation.ScopeID); err != nil {
		return badRequest("scope_id must be a UUID")
	}
	if !isDiscordID(automation.ChannelID) {
		return badRequest("channel_id must be a Discord id")
	}
	if automation.LeadUserID != nil && !isDiscordID(*automation.LeadUserID) {
		return badRequest("lead_user_id must be a Discord id")
	}
	// A week bounds both windows. Longer than that is a mistake, not a plan:
	// an announcement two weeks early is noise, and a lookback that long
	// re-reads events every sweep.
	if automation.LeadHours < 1 || automation.LeadHours > 168 {
		return badRequest("lead_hours must be 1 to 168")
	}
	if automation.LookbackHours < 1 || automation.LookbackHours > 168 {
		return badRequest("lookback_hours must be 1 to 168")
	}
	if automation.PostHour != nil && (*automation.PostHour < 0 || *automation.PostHour > 23) {
		return badRequest("post_hour must be 0 to 23")
	}
	if automation.Kind == store.KindNudge && automation.Deliver == store.DeliverDM && automation.LeadUserID == nil {
		return badRequest("a NUDGE delivered by DM needs lead_user_id")
	}
	return nil
}

func automationID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", badRequest("automation id must be a UUID")
	}
	return id, nil
}

// isDiscordID accepts a snowflake: digits only, and long enough to be one.
// Discord ids are 64-bit integers rendered in decimal, so the length is not
// fixed forever; the check refuses anything that is plainly not an id.
func isDiscordID(value string) bool {
	if len(value) < 15 || len(value) > 25 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func requireDiscordIDs(field string, values []string) error {
	for _, value := range values {
		if !isDiscordID(value) {
			return badRequest(field + " must contain only Discord ids")
		}
	}
	return nil
}
