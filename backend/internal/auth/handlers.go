package auth

import (
	"crypto/subtle"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"

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
		log.Printf("auth: oauth exchange: %v", err)
		return c.Status(fiber.StatusBadGateway).SendString("authentication failed, please try again")
	}

	member, err := h.GitHub.IsOrgMember(ctx, tok)
	if err != nil {
		log.Printf("auth: org membership check: %v", err)
		return c.Status(fiber.StatusBadGateway).SendString("authentication failed, please try again")
	}
	if !member {
		return c.Redirect().Status(fiber.StatusFound).To(dest + "/?auth=denied")
	}

	user, err := h.GitHub.FetchUser(ctx, tok)
	if err != nil {
		log.Printf("auth: fetch user: %v", err)
		return c.Status(fiber.StatusBadGateway).SendString("authentication failed, please try again")
	}

	existing, _ := h.Store.GetOperator(ctx, user.ID)
	if existing != nil && existing.IsBanned {
		return c.Redirect().Status(fiber.StatusFound).To(dest + "/?auth=banned")
	}

	forceSuper := h.Cfg.IsSuperuserID != nil && h.Cfg.IsSuperuserID(user.ID)
	if err := h.Store.UpsertOperator(ctx, store.Operator{
		GitHubID:  user.ID,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	}, forceSuper); err != nil {
		log.Printf("auth: upsert operator: %v", err)
		return fiber.ErrInternalServerError
	}

	token, err := newToken()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if err := h.Store.CreateSession(ctx, hashToken(token), user.ID, "",
		time.Now().Add(h.Cfg.SessionTTL), c.Get("User-Agent")); err != nil {
		log.Printf("auth: create session: %v", err)
		return fiber.ErrInternalServerError
	}

	setSessionCookie(c, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.CookieDomain, h.Cfg.SessionTTL)
	return c.Redirect().Status(fiber.StatusFound).To(dest)
}

func (h *Handler) Logout(c fiber.Ctx) error {
	if token := c.Cookies(SessionCookie); token != "" {
		hash := hashToken(token)
		if err := h.Store.DeleteSession(c.Context(), hash); err != nil {
			log.Printf("auth: delete session on logout: %v", err)
		}
		if h.Cache != nil {
			_ = h.Cache.InvalidateSession(c.Context(), hash)
		}
	}
	clearSessionCookie(c, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.CookieDomain)
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
		log.Printf("apps: list: %v", err)
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
		log.Printf("permissions: list: %v", err)
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
		log.Printf("apps: create: %v", err)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "could not create app"})
	}
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
		log.Printf("grants: list: %v", err)
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
		log.Printf("grants: create: %v", err)
		return fiber.ErrInternalServerError
	}
	if h.Cache != nil {
		_ = h.Cache.InvalidateGrants(c.Context(), *body.OperatorID)
	}
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
		log.Printf("org: list: %v", err)
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
	if banned {
		_, _ = h.Store.DeleteSessionsForOperator(c.Context(), id)
	}
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
