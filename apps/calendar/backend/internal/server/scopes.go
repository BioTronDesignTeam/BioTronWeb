package server

import (
	"net/http"
	"strings"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/store"
)

func (h *Handler) listScopes(w http.ResponseWriter, r *http.Request) error {
	scopes, err := h.store.ListScopes(r.Context(), false)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, nonNilScopes(scopes))
}

func (h *Handler) listAdminScopes(w http.ResponseWriter, r *http.Request) error {
	scopes, err := h.store.ListScopes(r.Context(), true)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, nonNilScopes(scopes))
}

func (h *Handler) createScope(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Kind     string `json:"kind"`
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := decodeBody(r, &body); err != nil {
		return err
	}
	body.Kind = strings.ToUpper(strings.TrimSpace(body.Kind))
	body.Name = strings.TrimSpace(body.Name)
	if body.Kind != model.ScopeProject && body.Kind != model.ScopeSubteam {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "kind must be PROJECT or SUBTEAM"})
	}
	if len(body.Name) < 2 || len(body.Name) > 80 || body.ParentID == "" {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name and parent_id are required"})
	}
	parent, err := h.store.GetScope(r.Context(), body.ParentID)
	if err != nil {
		return err
	}
	expectedParent := model.ScopeTeam
	if body.Kind == model.ScopeSubteam {
		expectedParent = model.ScopeProject
	}
	if parent.Kind != expectedParent || parent.Status != model.ScopeActive {
		return writeJSON(w, http.StatusConflict, map[string]any{"error": "the selected parent is not an active compatible scope"})
	}
	active, err := h.store.ScopeIsActive(r.Context(), parent.ID)
	if err != nil {
		return err
	}
	if !active {
		return writeJSON(w, http.StatusConflict, map[string]any{"error": "the selected parent belongs to an archived scope"})
	}
	parentID := parent.ID
	scope, err := h.store.CreateScope(r.Context(), model.Scope{
		Kind: body.Kind, Name: body.Name, Slug: store.Slug(body.Name), ParentID: &parentID,
	})
	if err != nil {
		return err
	}
	h.adminEvent(r, "Scope created", map[string]any{
		"scope_id": scope.ID, "kind": scope.Kind, "name": scope.Name, "parent_id": parentID,
	})
	return writeJSON(w, http.StatusCreated, scope)
}

func (h *Handler) renameScope(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &body); err != nil {
		return err
	}
	body.Name = strings.TrimSpace(body.Name)
	if len(body.Name) < 2 || len(body.Name) > 80 {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name must be between 2 and 80 characters"})
	}
	scope, err := h.store.RenameScope(r.Context(), r.PathValue("id"), body.Name, store.Slug(body.Name))
	if err != nil {
		return err
	}
	h.adminEvent(r, "Scope renamed", map[string]any{"scope_id": scope.ID, "name": scope.Name})
	return writeJSON(w, http.StatusOK, scope)
}

func (h *Handler) archiveScope(w http.ResponseWriter, r *http.Request) error {
	scope, err := h.store.SetScopeArchived(r.Context(), r.PathValue("id"), true)
	if err != nil {
		return err
	}
	h.adminEvent(r, "Scope archived", map[string]any{"scope_id": scope.ID})
	return writeJSON(w, http.StatusOK, scope)
}

func (h *Handler) restoreScope(w http.ResponseWriter, r *http.Request) error {
	scope, err := h.store.GetScope(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if scope.ParentID != nil {
		active, err := h.store.ScopeIsActive(r.Context(), *scope.ParentID)
		if err != nil {
			return err
		}
		if !active {
			return writeJSON(w, http.StatusConflict, map[string]any{"error": "restore the parent project first"})
		}
	}
	scope, err = h.store.SetScopeArchived(r.Context(), scope.ID, false)
	if err != nil {
		return err
	}
	h.adminEvent(r, "Scope restored", map[string]any{"scope_id": scope.ID})
	return writeJSON(w, http.StatusOK, scope)
}

func (h *Handler) deleteScope(w http.ResponseWriter, r *http.Request) error {
	// The row is gone once the delete returns, so its id has to be read from
	// the path.
	scopeID := r.PathValue("id")
	if err := h.store.DeleteScope(r.Context(), scopeID); err != nil {
		return err
	}
	h.adminEvent(r, "Scope deleted", map[string]any{"scope_id": scopeID})
	return noContent(w)
}

func nonNilScopes(scopes []model.Scope) []model.Scope {
	if scopes == nil {
		return []model.Scope{}
	}
	return scopes
}
