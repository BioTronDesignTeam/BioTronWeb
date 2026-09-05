package server

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/store"
)

func (h *Handler) listScopes(c fiber.Ctx) error {
	scopes, err := h.store.ListScopes(c.Context(), false)
	if err != nil {
		return err
	}
	return c.JSON(nonNilScopes(scopes))
}

func (h *Handler) listAdminScopes(c fiber.Ctx) error {
	scopes, err := h.store.ListScopes(c.Context(), true)
	if err != nil {
		return err
	}
	return c.JSON(nonNilScopes(scopes))
}

func (h *Handler) createScope(c fiber.Ctx) error {
	var body struct {
		Kind     string `json:"kind"`
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return fiber.ErrBadRequest
	}
	body.Kind = strings.ToUpper(strings.TrimSpace(body.Kind))
	body.Name = strings.TrimSpace(body.Name)
	if body.Kind != model.ScopeProject && body.Kind != model.ScopeSubteam {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "kind must be PROJECT or SUBTEAM"})
	}
	if len(body.Name) < 2 || len(body.Name) > 80 || body.ParentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name and parent_id are required"})
	}
	parent, err := h.store.GetScope(c.Context(), body.ParentID)
	if err != nil {
		return err
	}
	expectedParent := model.ScopeTeam
	if body.Kind == model.ScopeSubteam {
		expectedParent = model.ScopeProject
	}
	if parent.Kind != expectedParent || parent.Status != model.ScopeActive {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "the selected parent is not an active compatible scope"})
	}
	active, err := h.store.ScopeIsActive(c.Context(), parent.ID)
	if err != nil {
		return err
	}
	if !active {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "the selected parent belongs to an archived scope"})
	}
	parentID := parent.ID
	scope, err := h.store.CreateScope(c.Context(), model.Scope{
		Kind: body.Kind, Name: body.Name, Slug: store.Slug(body.Name), ParentID: &parentID,
	})
	if err != nil {
		return err
	}
	h.adminEvent(c, "Scope created", map[string]any{
		"scope_id": scope.ID, "kind": scope.Kind, "name": scope.Name, "parent_id": parentID,
	})
	return c.Status(fiber.StatusCreated).JSON(scope)
}

func (h *Handler) renameScope(c fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return fiber.ErrBadRequest
	}
	body.Name = strings.TrimSpace(body.Name)
	if len(body.Name) < 2 || len(body.Name) > 80 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name must be between 2 and 80 characters"})
	}
	scope, err := h.store.RenameScope(c.Context(), c.Params("id"), body.Name, store.Slug(body.Name))
	if err != nil {
		return err
	}
	h.adminEvent(c, "Scope renamed", map[string]any{"scope_id": scope.ID, "name": scope.Name})
	return c.JSON(scope)
}

func (h *Handler) archiveScope(c fiber.Ctx) error {
	scope, err := h.store.SetScopeArchived(c.Context(), c.Params("id"), true)
	if err != nil {
		return err
	}
	h.adminEvent(c, "Scope archived", map[string]any{"scope_id": scope.ID})
	return c.JSON(scope)
}

func (h *Handler) restoreScope(c fiber.Ctx) error {
	scope, err := h.store.GetScope(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if scope.ParentID != nil {
		active, err := h.store.ScopeIsActive(c.Context(), *scope.ParentID)
		if err != nil {
			return err
		}
		if !active {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "restore the parent project first"})
		}
	}
	scope, err = h.store.SetScopeArchived(c.Context(), scope.ID, false)
	if err != nil {
		return err
	}
	h.adminEvent(c, "Scope restored", map[string]any{"scope_id": scope.ID})
	return c.JSON(scope)
}

func (h *Handler) deleteScope(c fiber.Ctx) error {
	// The row is gone once the delete returns, so its id has to be read from
	// the path. Params points into fasthttp's pooled buffer and the event is
	// marshalled on another goroutine, so the copy is what keeps the event
	// from naming whatever scope the next request happens to carry.
	scopeID := strings.Clone(c.Params("id"))
	if err := h.store.DeleteScope(c.Context(), scopeID); err != nil {
		return err
	}
	h.adminEvent(c, "Scope deleted", map[string]any{"scope_id": scopeID})
	return c.SendStatus(fiber.StatusNoContent)
}

func nonNilScopes(scopes []model.Scope) []model.Scope {
	if scopes == nil {
		return []model.Scope{}
	}
	return scopes
}
