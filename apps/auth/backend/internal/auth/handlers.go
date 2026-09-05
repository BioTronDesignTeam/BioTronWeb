package auth

import (
	"crypto/subtle"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/biotron/go/logclient"
	"github.com/BioTronDesignTeam/biotron/go/logclient/fiberlog"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/cache"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

const stateCookie = "oauth_state"

type Config struct {
	FrontendURL    string
	AllowedOrigins []string
	CookieSecure   bool
	CookieSameSite string
	CookieDomain   string
	SessionTTL     time.Duration
	IsSuperuserID  func(int64) bool
}

type Handler struct {
	Store  *store.Store
	Cache  *cache.Cache
	GitHub *GitHubClient
	Cfg    Config
	// Events carries the audit trail to Logger. Access is asked for in a
	// meeting or on Discord and granted by hand, so the grant, the revoke, the
	// ban, and the manager flag are the only record of who decided what; each
	// one is logged with the actor and the target. Nil disables delivery
	// without changing behaviour.
	Events *logclient.Client
}

// audit records one change to access or standing. The payload names the actor
// and the target by id and login only; it never carries a session, a key, or
// a cookie.
func (h *Handler) audit(message string, actor *store.SessionOperator, payload map[string]any) {
	if h.Events == nil || actor == nil {
		return
	}
	payload["actor_id"] = actor.GitHubID
	payload["actor_login"] = actor.Login
	h.Events.LogAsync(logclient.Info, message, payload)
}

// fail records one failure twice: on stdout, which is all `docker logs` has
// when Logger is unreachable, and on the request's completion event, which
// then names the cause instead of a bare status.
func fail(c fiber.Ctx, err error, what string) {
	log.Printf("%s: %v", what, err)
	fiberlog.SetError(c, err)
}

func (h *Handler) LoginRedirect(c fiber.Ctx) error {
	state, err := newToken()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	c.Cookie(&fiber.Cookie{
		Name:     stateCookie,
		Value:    state,
		Path:     "/",
		HTTPOnly: true,
		Secure:   h.Cfg.CookieSecure,
		SameSite: fiber.CookieSameSiteLaxMode,
		Expires:  time.Now().Add(10 * time.Minute),
	})
	setReturnCookie(c, h.safeReturn(c.Query("redirect")), h.Cfg)
	return c.Redirect().Status(fiber.StatusFound).To(h.GitHub.AuthCodeURL(state))
}

func (h *Handler) Callback(c fiber.Ctx) error {
	state := c.Query("state")
	code := c.Query("code")
	cookieState := c.Cookies(stateCookie)
	dest := h.safeReturn(c.Cookies(returnCookie))
	clearStateCookie(c, h.Cfg)
	clearReturnCookie(c, h.Cfg)

	if state == "" || cookieState == "" ||
		subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return c.Status(fiber.StatusBadRequest).SendString("invalid OAuth state")
	}
	if code == "" {
		return c.Status(fiber.StatusBadRequest).SendString("missing code")
	}

	ctx := c.Context()
	tok, err := h.GitHub.Exchange(ctx, code)
	if err != nil {
		h.signInFailed(c, err, "exchange", "auth: oauth exchange")
		return c.Status(fiber.StatusBadGateway).SendString("authentication failed, please try again")
	}

	member, err := h.GitHub.IsOrgMember(ctx, tok)
	if err != nil {
		h.signInFailed(c, err, "membership", "auth: org membership check")
		return c.Status(fiber.StatusBadGateway).SendString("authentication failed, please try again")
	}
	if !member {
		// The login is unknown here: refusing before /user is fetched is what
		// keeps a non-member's name out of the store.
		h.Events.LogAsync(logclient.Warning, "Sign-in refused", map[string]any{"reason": "not a member"})
		return c.Redirect().Status(fiber.StatusFound).To(dest + "/?auth=denied")
	}

	user, err := h.GitHub.FetchUser(ctx, tok)
	if err != nil {
		h.signInFailed(c, err, "user", "auth: fetch user")
		return c.Status(fiber.StatusBadGateway).SendString("authentication failed, please try again")
	}

	existing, existingErr := h.Store.GetOperator(ctx, user.ID)
	if existing != nil && existing.IsBanned {
		h.Events.LogAsync(logclient.Warning, "Sign-in refused", map[string]any{
			"reason": "banned", "login": user.Login,
		})
		return c.Redirect().Status(fiber.StatusFound).To(dest + "/?auth=banned")
	}
	// GetOperator answers ErrNotFound only when the row is absent, so a first
	// sign-in is known exactly. Any other error leaves the flag off rather than
	// announcing an operator who has been here for months.
	newOperator := errors.Is(existingErr, store.ErrNotFound)

	forceSuper := h.Cfg.IsSuperuserID != nil && h.Cfg.IsSuperuserID(user.ID)
	if err := h.Store.UpsertOperator(ctx, store.Operator{
		GitHubID:  user.ID,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	}, forceSuper); err != nil {
		fail(c, err, "auth: upsert operator")
		return fiber.ErrInternalServerError
	}

	token, err := newToken()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if err := h.Store.CreateSession(ctx, hashToken(token), user.ID, "",
		time.Now().Add(h.Cfg.SessionTTL), c.Get("User-Agent")); err != nil {
		fail(c, err, "auth: create session")
		return fiber.ErrInternalServerError
	}

	setSessionCookie(c, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.CookieDomain, h.Cfg.SessionTTL)
	h.Events.LogAsync(logclient.Info, "Operator signed in", map[string]any{
		"operator_id":    user.ID,
		"operator_login": user.Login,
		"new_operator":   newOperator,
	})
	return c.Redirect().Status(fiber.StatusFound).To(dest)
}

// signInFailed reports one broken step of the GitHub handshake. The stage says
// which call failed; the code, the state, and the token never appear.
func (h *Handler) signInFailed(c fiber.Ctx, err error, stage, what string) {
	fail(c, err, what)
	h.Events.LogAsync(logclient.Error, "GitHub sign-in failed", map[string]any{
		"stage": stage, "error": err.Error(),
	})
}

func (h *Handler) Logout(c fiber.Ctx) error {
	if token := c.Cookies(SessionCookie); token != "" {
		hash := hashToken(token)
		if err := h.Store.DeleteSession(c.Context(), hash); err != nil {
			// The browser loses its cookie either way, so the request answers
			// 204 and its event stays Info. Only this event says the row
			// survived and the session still works for whoever holds the token.
			log.Printf("auth: delete session on logout: %v", err)
			h.Events.LogAsync(logclient.Error, "Session delete failed", map[string]any{"error": err.Error()})
		}
		if h.Cache != nil {
			_ = h.Cache.InvalidateSession(c.Context(), hash)
		}
	}
	clearSessionCookie(c, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.CookieDomain)
	if op := OperatorFrom(c); op != nil {
		h.Events.LogAsync(logclient.Info, "Operator signed out", map[string]any{
			"operator_id": op.GitHubID, "operator_login": op.Login,
		})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Me(c fiber.Ctx) error {
	op := OperatorFrom(c)
	if op == nil {
		return fiber.ErrUnauthorized
	}
	if op.IsGuest() && (c.Query("app") == "" || c.Query("app") != op.GuestAppID) {
		return fiber.ErrForbidden
	}
	return c.JSON(fiber.Map{
		"github_id":    op.GitHubID,
		"login":        op.Login,
		"name":         op.Name,
		"avatar_url":   op.AvatarURL,
		"is_superuser": op.IsSuperuser,
		"is_manager":   op.IsManager,
		"is_staff":     op.IsStaff(),
		"is_guest":     op.IsGuest(),
		"guest_app_id": op.GuestAppID,
	})
}

func (h *Handler) ListApps(c fiber.Ctx) error {
	apps, err := h.Store.ListApps(c.Context())
	if err != nil {
		fail(c, err, "apps: list")
		return fiber.ErrInternalServerError
	}
	if apps == nil {
		apps = []store.App{}
	}
	return c.JSON(apps)
}

func (h *Handler) ListPermissions(c fiber.Ctx) error {
	perms, err := h.Store.ListPermissions(c.Context())
	if err != nil {
		fail(c, err, "permissions: list")
		return fiber.ErrInternalServerError
	}
	if perms == nil {
		perms = []store.Permission{}
	}
	return c.JSON(perms)
}

func (h *Handler) CreateApp(c fiber.Ctx) error {
	var body struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.Bind().Body(&body); err != nil || body.ID == "" || body.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id and name required"})
	}
	if err := h.Store.CreateApp(c.Context(), body.ID, body.Name, body.Description); err != nil {
		fail(c, err, "apps: create")
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "could not create app"})
	}
	h.audit("Tool registered", OperatorFrom(c), map[string]any{"app_id": body.ID})
	return c.Status(fiber.StatusCreated).JSON(body)
}

func (h *Handler) MyGrants(c fiber.Ctx) error {
	op := OperatorFrom(c)
	var grants []store.Grant
	var err error
	if h.Cache != nil {
		grants, err = h.Cache.ListGrantsForOperator(c.Context(), op.GitHubID)
	} else {
		grants, err = h.Store.ListGrantsForOperator(c.Context(), op.GitHubID)
	}
	if err != nil {
		fail(c, err, "grants: list")
		return fiber.ErrInternalServerError
	}
	if grants == nil {
		grants = []store.Grant{}
	}
	return c.JSON(fiber.Map{
		"grants":      grants,
		"full_access": op.IsStaff(),
	})
}

func (h *Handler) CreateGrant(c fiber.Ctx) error {
	var body struct {
		OperatorID    *int64 `json:"operator_id"`
		AppID         string `json:"app_id"`
		PermissionKey string `json:"permission_key"`
	}
	if err := c.Bind().Body(&body); err != nil || body.OperatorID == nil || *body.OperatorID < 0 || body.AppID == "" || body.PermissionKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "operator_id, app_id, permission_key required"})
	}
	if err := h.Store.CreateGrant(c.Context(), *body.OperatorID, body.AppID, body.PermissionKey); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "permission not found"})
		}
		fail(c, err, "grants: create")
		return fiber.ErrInternalServerError
	}
	if h.Cache != nil {
		_ = h.Cache.InvalidateGrants(c.Context(), *body.OperatorID)
	}
	h.audit("Permission granted", OperatorFrom(c), map[string]any{
		"target_id": *body.OperatorID, "app": body.AppID, "permission": body.PermissionKey,
	})
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteGrant(c fiber.Ctx) error {
	var body struct {
		OperatorID    *int64 `json:"operator_id"`
		AppID         string `json:"app_id"`
		PermissionKey string `json:"permission_key"`
	}
	if err := c.Bind().Body(&body); err != nil || body.OperatorID == nil || *body.OperatorID < 0 || body.AppID == "" || body.PermissionKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "operator_id, app_id, permission_key required"})
	}
	if err := h.Store.DeleteGrant(c.Context(), *body.OperatorID, body.AppID, body.PermissionKey); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}
	if h.Cache != nil {
		_ = h.Cache.InvalidateGrants(c.Context(), *body.OperatorID)
	}
	h.audit("Permission revoked", OperatorFrom(c), map[string]any{
		"target_id": *body.OperatorID, "app": body.AppID, "permission": body.PermissionKey,
	})
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Check(c fiber.Ctx) error {
	op := OperatorFrom(c)
	appID := c.Query("app")
	key := c.Query("permission")
	if key == "" {
		key = c.Query("action") // back-compat alias
	}
	if appID == "" || key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "app and permission query params required"})
	}
	ok, err := h.Store.Allowed(c.Context(), op, appID, key)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(fiber.Map{"allowed": ok})
}

func (h *Handler) ListOrgMembers(c fiber.Ctx) error {
	members, err := h.Store.ListOrgMembers(c.Context())
	if err != nil {
		fail(c, err, "org: list")
		return fiber.ErrInternalServerError
	}
	if members == nil {
		members = []store.OrgMember{}
	}
	return c.JSON(members)
}

func (h *Handler) MemberGrants(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	grants, err := h.Store.ListGrantsForOperator(c.Context(), id)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if grants == nil {
		grants = []store.Grant{}
	}
	return c.JSON(grants)
}

func (h *Handler) BanMember(c fiber.Ctx) error {
	return h.setBanned(c, true)
}

func (h *Handler) UnbanMember(c fiber.Ctx) error {
	return h.setBanned(c, false)
}

func (h *Handler) setBanned(c fiber.Ctx, banned bool) error {
	actor := OperatorFrom(c)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if id == store.GuestGitHubID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot ban the guest account"})
	}
	if id == actor.GitHubID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot ban yourself"})
	}
	target, err := h.Store.GetOperator(c.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}
	if target.IsSuperuser && !actor.IsSuperuser {
		return fiber.ErrForbidden
	}
	if err := h.Store.SetBanned(c.Context(), id, banned); err != nil {
		return fiber.ErrInternalServerError
	}
	message := "Operator unbanned"
	if banned {
		_, _ = h.Store.DeleteSessionsForOperator(c.Context(), id)
		message = "Operator banned"
	}
	h.audit(message, actor, map[string]any{"target_id": id, "target_login": target.Login})
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) SetManager(c fiber.Ctx) error {
	actor := OperatorFrom(c)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	var body struct {
		Manager bool `json:"manager"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return fiber.ErrBadRequest
	}
	if id == store.GuestGitHubID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot change the guest account"})
	}
	if id == actor.GitHubID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot change your own manager flag"})
	}
	if err := h.Store.SetManager(c.Context(), id, body.Manager); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}
	_, _ = h.Store.DeleteSessionsForOperator(c.Context(), id)
	message := "Manager flag removed"
	if body.Manager {
		message = "Manager flag set"
	}
	payload := map[string]any{"target_id": id}
	if target, err := h.Store.GetOperator(c.Context(), id); err == nil {
		payload["target_login"] = target.Login
	}
	h.audit(message, actor, payload)
	return c.SendStatus(fiber.StatusNoContent)
}

func clearStateCookie(c fiber.Ctx, cfg Config) {
	c.Cookie(&fiber.Cookie{
		Name:     stateCookie,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: fiber.CookieSameSiteLaxMode,
		Expires:  time.Now().Add(-time.Hour),
	})
}
