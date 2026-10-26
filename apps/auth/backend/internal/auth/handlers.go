package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/cache"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/httpx"
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
	// StateCookiePath scopes the sign-in state cookie to the sign-in routes
	// as the browser sees them; see StateCookiePath. Empty means "/".
	StateCookiePath string
}

type Handler struct {
	Store  *store.Store
	Cache  *cache.Cache
	GitHub *GitHubClient
	// Members re-checks each operator's organization membership on their
	// requests, at most once per MembershipTTL.
	Members *Membership
	Signer  *Signer
	Cfg     Config
	// Events carries the audit trail to Grafana and PostHog. Access is asked for in a
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
// when no event destination is reachable, and on the request's completion event, which
// then names the cause instead of a bare status. An error can quote a path,
// which can hold a line break, so line breaks are escaped on stdout.
func fail(r *http.Request, err error, what string) {
	log.Printf("%s: %s", what, strings.ReplaceAll(strings.ReplaceAll(err.Error(), "\r", `\r`), "\n", `\n`))
	httplog.SetError(r, err)
}

func (h *Handler) LoginRedirect(w http.ResponseWriter, r *http.Request) {
	state, err := newToken()
	if err != nil {
		httpx.Error(w, r, http.StatusInternalServerError)
		return
	}
	verifier, err := newToken()
	if err != nil {
		httpx.Error(w, r, http.StatusInternalServerError)
		return
	}
	// The state, the origin to return to, and the PKCE verifier travel
	// together in one signed cookie, so none can be swapped on the way back
	// from GitHub.
	value, err := h.Signer.State(SignIn{
		State:    state,
		Return:   h.safeReturn(r.URL.Query().Get("redirect")),
		Verifier: verifier,
	})
	if err != nil {
		fail(r, err, "auth: sign state")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure follows COOKIE_SECURE, which config refuses to leave off with an https callback, a cookie domain, or SameSite=None: it is off only on plain-http localhost.
		Name:     stateCookie,
		Value:    value,
		Path:     h.Cfg.stateCookiePath(),
		HttpOnly: true,
		Secure:   h.Cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(stateLifetime),
	})
	httpx.Redirect(w, h.GitHub.AuthCodeURL(state, verifier))
}

func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	// One state serves one callback, whatever its result.
	signIn, ok := h.Signer.ReadState(cookieValue(r, stateCookie))
	dest := h.safeReturn(signIn.Return)
	clearStateCookie(w, h.Cfg)

	if !ok || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(signIn.State)) != 1 {
		httpx.Text(w, http.StatusBadRequest, "invalid OAuth state")
		return
	}
	if code == "" {
		httpx.Text(w, http.StatusBadRequest, "missing code")
		return
	}

	ctx := r.Context()
	tok, err := h.GitHub.Exchange(ctx, code, signIn.Verifier)
	if err != nil {
		h.signInFailed(r, err, "exchange", "auth: oauth exchange")
		httpx.Text(w, http.StatusBadGateway, "authentication failed, please try again")
		return
	}

	// The user token lives only in this function: it names the user, and the
	// membership is asked with the App's own token.
	user, err := h.GitHub.FetchUser(ctx, tok)
	if err != nil {
		h.signInFailed(r, err, "user", "auth: fetch user")
		httpx.Text(w, http.StatusBadGateway, "authentication failed, please try again")
		return
	}

	member, err := h.GitHub.IsOrgMember(ctx, user.Login)
	if err != nil {
		h.signInFailed(r, err, "membership", "auth: org membership check")
		httpx.Text(w, http.StatusBadGateway, "authentication failed, please try again")
		return
	}
	if h.Members != nil {
		h.Members.Keep(user.ID, member)
	}
	if !member {
		// A non-member's name is never stored or sent: the refusal carries
		// only the reason.
		h.Events.LogAsync(logclient.Warning, "Sign-in refused", map[string]any{"reason": "not a member"})
		httpx.Redirect(w, dest+"/?auth=denied")
		return
	}

	existing, existingErr := h.Store.GetOperator(ctx, user.ID)
	if existing != nil && existing.IsBanned {
		h.Events.LogAsync(logclient.Warning, "Sign-in refused", map[string]any{
			"reason": "banned", "login": user.Login,
		})
		httpx.Redirect(w, dest+"/?auth=banned")
		return
	}
	// GetOperator answers ErrNotFound only when the row is absent, so a first
	// sign-in is known exactly. Any other error leaves the flag off rather than
	// announcing an operator who has been here for months.
	newOperator := errors.Is(existingErr, store.ErrNotFound)

	forceSuper := h.isSuperuser(user.ID)
	if err := h.Store.UpsertOperator(ctx, store.Operator{
		GitHubID:  user.ID,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	}, forceSuper); err != nil {
		fail(r, err, "auth: upsert operator")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	h.invalidate(user.ID)

	expires := time.Now().Add(h.Cfg.SessionTTL)
	token, err := h.Signer.Session(Session{GitHubID: user.ID}, expires)
	if err != nil {
		fail(r, err, "auth: sign session")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}

	setSessionCookie(w, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.CookieDomain, h.Cfg.SessionTTL)
	h.Events.LogAsync(logclient.Info, "Operator signed in", map[string]any{
		"operator_id":    user.ID,
		"operator_login": user.Login,
		"new_operator":   newOperator,
	})
	httpx.Redirect(w, dest)
}

// signInFailed reports one broken step of the GitHub handshake. The stage says
// which call failed; the code, the state, and the token never appear.
func (h *Handler) signInFailed(r *http.Request, err error, stage, what string) {
	fail(r, err, what)
	h.Events.LogAsync(logclient.Error, "GitHub sign-in failed", map[string]any{
		"stage": stage, "error": err.Error(),
	})
}

// Logout removes the session cookie. There is no session store, so a copy of
// the cookie stays valid until it expires; a ban is what ends it everywhere.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.CookieDomain)
	if op := OperatorFrom(r); op != nil {
		h.Events.LogAsync(logclient.Info, "Operator signed out", map[string]any{
			"operator_id": op.GitHubID, "operator_login": op.Login,
		})
	}
	httpx.Status(w, http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	op := OperatorFrom(r)
	if op == nil {
		httpx.Error(w, r, http.StatusUnauthorized)
		return
	}
	app := r.URL.Query().Get("app")
	if op.IsGuest() && (app == "" || app != op.GuestAppID) {
		httpx.Error(w, r, http.StatusForbidden)
		return
	}
	httpx.JSON(w, http.StatusOK, operatorBody(op))
}

// operatorBody is the operator as /auth/me and /v1/session describe them.
func operatorBody(op *store.SessionOperator) map[string]any {
	return map[string]any{
		"github_id":    op.GitHubID,
		"login":        op.Login,
		"name":         op.Name,
		"avatar_url":   op.AvatarURL,
		"is_superuser": op.IsSuperuser,
		"is_manager":   op.IsManager,
		"is_staff":     op.IsStaff(),
		"is_guest":     op.IsGuest(),
		"guest_app_id": op.GuestAppID,
	}
}

// maxSessionPermissions caps the permissions one /v1/session call may ask
// about. No tool declares more than a few.
const maxSessionPermissions = 16

// Session answers /auth/me and /v1/check in one call: who is signed in, and
// which of the asked permissions they hold on one tool.
//
//	GET /v1/session?app=calendar&permission=write
//	{"operator": {...as /auth/me...}, "permissions": {"write": true}}
//
// A tool that asked the two separately could see the session end between the
// calls and mistake that for an outage. The rules are those of the two
// routes: no usable session is 401 (from RequireSession), and a guest asking
// about any tool but its own is 403, as /auth/me answers. Asking about no
// permission is allowed and returns only the operator.
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	op := OperatorFrom(r)
	if op == nil {
		httpx.Error(w, r, http.StatusUnauthorized)
		return
	}
	query := r.URL.Query()
	appID := query.Get("app")
	keys := query["permission"]
	if appID == "" || len(keys) > maxSessionPermissions || slices.Contains(keys, "") {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "app query param required, with at most 16 non-empty permission params"})
		return
	}
	if op.IsGuest() && appID != op.GuestAppID {
		httpx.Error(w, r, http.StatusForbidden)
		return
	}
	permissions := make(map[string]bool, len(keys))
	for _, key := range keys {
		ok, err := h.allowed(r.Context(), op, appID, key)
		if err != nil {
			fail(r, err, "auth: session permissions")
			httpx.Status(w, http.StatusInternalServerError)
			return
		}
		permissions[key] = ok
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"operator": operatorBody(op), "permissions": permissions})
}

func (h *Handler) ListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := h.Store.ListApps(r.Context())
	if err != nil {
		fail(r, err, "apps: list")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	if apps == nil {
		apps = []store.App{}
	}
	httpx.JSON(w, http.StatusOK, apps)
}

func (h *Handler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.Store.ListPermissions(r.Context())
	if err != nil {
		fail(r, err, "permissions: list")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	if perms == nil {
		perms = []store.Permission{}
	}
	httpx.JSON(w, http.StatusOK, perms)
}

func (h *Handler) CreateApp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := httpx.Decode(w, r, &body); err != nil || body.ID == "" || body.Name == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "id and name required"})
		return
	}
	if err := h.Store.CreateApp(r.Context(), body.ID, body.Name, body.Description); err != nil {
		fail(r, err, "apps: create")
		httpx.JSON(w, http.StatusConflict, map[string]any{"error": "could not create app"})
		return
	}
	h.audit("Tool registered", OperatorFrom(r), map[string]any{"app_id": body.ID})
	httpx.JSON(w, http.StatusCreated, body)
}

func (h *Handler) MyGrants(w http.ResponseWriter, r *http.Request) {
	op := OperatorFrom(r)
	grants, err := h.Cache.Grants(r.Context(), op.GitHubID)
	if err != nil {
		fail(r, err, "grants: list")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	if grants == nil {
		grants = []store.Grant{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"grants":      grants,
		"full_access": op.IsStaff(),
	})
}

// grantBody is the body of POST and DELETE /grants. The operator is a GitHub id,
// always positive: the guest account (0) never holds a grant.
type grantBody struct {
	OperatorID    *int64 `json:"operator_id"`
	AppID         string `json:"app_id"`
	PermissionKey string `json:"permission_key"`
}

func readGrantBody(w http.ResponseWriter, r *http.Request) (grantBody, bool) {
	var body grantBody
	if err := httpx.Decode(w, r, &body); err != nil || body.OperatorID == nil || *body.OperatorID <= 0 || body.AppID == "" || body.PermissionKey == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "Choose a member, a tool, and a permission."})
		return body, false
	}
	return body, true
}

func (h *Handler) CreateGrant(w http.ResponseWriter, r *http.Request) {
	body, ok := readGrantBody(w, r)
	if !ok {
		return
	}
	if err := h.Store.CreateGrant(r.Context(), *body.OperatorID, body.AppID, body.PermissionKey); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.JSON(w, http.StatusNotFound, map[string]any{"error": "That permission doesn't exist for this tool."})
			return
		}
		fail(r, err, "grants: create")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	h.invalidate(*body.OperatorID)
	h.audit("Permission granted", OperatorFrom(r), map[string]any{
		"target_id": *body.OperatorID, "app": body.AppID, "permission": body.PermissionKey,
	})
	httpx.Status(w, http.StatusNoContent)
}

func (h *Handler) DeleteGrant(w http.ResponseWriter, r *http.Request) {
	body, ok := readGrantBody(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteGrant(r.Context(), *body.OperatorID, body.AppID, body.PermissionKey); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, r, http.StatusNotFound)
			return
		}
		httpx.Error(w, r, http.StatusInternalServerError)
		return
	}
	h.invalidate(*body.OperatorID)
	h.audit("Permission revoked", OperatorFrom(r), map[string]any{
		"target_id": *body.OperatorID, "app": body.AppID, "permission": body.PermissionKey,
	})
	httpx.Status(w, http.StatusNoContent)
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	op := OperatorFrom(r)
	query := r.URL.Query()
	appID := query.Get("app")
	key := query.Get("permission")
	if key == "" {
		key = query.Get("action") // back-compat alias
	}
	if appID == "" || key == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "app and permission query params required"})
		return
	}
	ok, err := h.allowed(r.Context(), op, appID, key)
	if err != nil {
		httpx.Error(w, r, http.StatusInternalServerError)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"allowed": ok})
}

func (h *Handler) ListOrgMembers(w http.ResponseWriter, r *http.Request) {
	members, err := h.Store.ListOrgMembers(r.Context())
	if err != nil {
		fail(r, err, "org: list")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	if members == nil {
		members = []store.OrgMember{}
	}
	// The stored flag mirrors SUPERUSER_GITHUB_IDS as of startup; the list is
	// what decides.
	for i := range members {
		members[i].IsSuperuser = h.isSuperuser(members[i].GitHubID)
	}
	httpx.JSON(w, http.StatusOK, members)
}

func (h *Handler) MemberGrants(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest)
		return
	}
	grants, err := h.Store.ListGrantsForOperator(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, http.StatusInternalServerError)
		return
	}
	if grants == nil {
		grants = []store.Grant{}
	}
	httpx.JSON(w, http.StatusOK, grants)
}

func (h *Handler) BanMember(w http.ResponseWriter, r *http.Request) {
	h.setBanned(w, r, true)
}

func (h *Handler) UnbanMember(w http.ResponseWriter, r *http.Request) {
	h.setBanned(w, r, false)
}

func (h *Handler) setBanned(w http.ResponseWriter, r *http.Request, banned bool) {
	actor := OperatorFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest)
		return
	}
	if id == store.GuestGitHubID {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "Guests can't be banned. Rotate the Daily guest key to sign them out."})
		return
	}
	if id == actor.GitHubID {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "You can't ban yourself."})
		return
	}
	target, err := h.Store.GetOperator(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, r, http.StatusNotFound)
			return
		}
		httpx.Error(w, r, http.StatusInternalServerError)
		return
	}
	if h.isSuperuser(target.GitHubID) && !actor.IsSuperuser {
		httplog.SetError(r, errors.New(http.StatusText(http.StatusForbidden)))
		httpx.JSON(w, http.StatusForbidden, map[string]any{"error": "Only a superuser can ban or unban a superuser."})
		return
	}
	if err := h.Store.SetBanned(r.Context(), id, banned); err != nil {
		httpx.Error(w, r, http.StatusInternalServerError)
		return
	}
	// Every request reads the ban through the cache, so forgetting the
	// operator here refuses their next request on every device.
	h.invalidate(id)
	message := "Operator unbanned"
	if banned {
		message = "Operator banned"
	}
	h.audit(message, actor, map[string]any{"target_id": id, "target_login": target.Login})
	httpx.Status(w, http.StatusNoContent)
}

func (h *Handler) SetManager(w http.ResponseWriter, r *http.Request) {
	actor := OperatorFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest)
		return
	}
	var body struct {
		Manager bool `json:"manager"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		httpx.Error(w, r, http.StatusBadRequest)
		return
	}
	if id == store.GuestGitHubID {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "Guests can't be made managers."})
		return
	}
	if id == actor.GitHubID {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "You can't change your own Manager role."})
		return
	}
	if err := h.Store.SetManager(r.Context(), id, body.Manager); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, r, http.StatusNotFound)
			return
		}
		httpx.Error(w, r, http.StatusInternalServerError)
		return
	}
	h.invalidate(id)
	message := "Manager flag removed"
	if body.Manager {
		message = "Manager flag set"
	}
	payload := map[string]any{"target_id": id}
	if target, err := h.Store.GetOperator(r.Context(), id); err == nil {
		payload["target_login"] = target.Login
	}
	h.audit(message, actor, payload)
	httpx.Status(w, http.StatusNoContent)
}

func clearStateCookie(w http.ResponseWriter, cfg Config) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure follows COOKIE_SECURE, which config refuses to leave off with an https callback, a cookie domain, or SameSite=None: it is off only on plain-http localhost.
		Name:     stateCookie,
		Value:    "",
		Path:     cfg.stateCookiePath(),
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(-time.Hour),
	})
}

// allowed reports whether op may use permissionKey on appID. Staff hold every
// permission; a guest holds its tool's telemetry and nothing else; a member
// needs a grant.
func (h *Handler) allowed(ctx context.Context, op *store.SessionOperator, appID, permissionKey string) (bool, error) {
	if op.IsStaff() {
		return true, nil
	}
	if op.IsGuest() {
		return op.GuestAppID == appID && (permissionKey == "live" || permissionKey == "historical"), nil
	}
	return h.Cache.HasGrant(ctx, op.GitHubID, appID, permissionKey)
}

// isSuperuser reports whether SUPERUSER_GITHUB_IDS lists githubID. The list is
// read on every request, so the stored flag is only a mirror for other readers.
func (h *Handler) isSuperuser(githubID int64) bool {
	return h.Cfg.IsSuperuserID != nil && h.Cfg.IsSuperuserID(githubID)
}

// invalidate makes the next request see an operator's changed standing or
// grants at once.
func (h *Handler) invalidate(githubID int64) {
	if h.Cache != nil {
		h.Cache.Invalidate(githubID)
	}
}

// StateCookiePath returns the path to scope the sign-in state cookie to: the
// folder of the callback as the browser reaches it, such as
// /api/auth/github behind the edge's /api/ prefix or /auth/github on
// localhost, so the cookie goes only to the login and the callback. A
// callback URL of another shape gets "/".
func StateCookiePath(callbackURL string) string {
	u, err := url.Parse(callbackURL)
	if err != nil {
		return "/"
	}
	// A folder must be one absolute path. One that starts with // or /\ could
	// be read as a host, so it gets "/" too.
	folder, ok := strings.CutSuffix(u.Path, "/callback")
	if !ok || len(folder) < 2 || folder[0] != '/' || folder[1] == '/' || folder[1] == '\\' {
		return "/"
	}
	return folder
}

func (cfg Config) stateCookiePath() string {
	if cfg.StateCookiePath == "" {
		return "/"
	}
	return cfg.StateCookiePath
}
