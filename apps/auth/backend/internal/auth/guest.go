package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"net/http"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/httpx"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// easternZone is Waterloo/Toronto local time (EDT/EST, DST-aware).
var easternZone = func() *time.Location {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		panic("auth: load America/Toronto (is time/tzdata embedded?): " + err.Error())
	}
	return loc
}()

// guestSessionExpiry returns the next Eastern midnight — the instant the daily
// guest key rotates — so a guest session only lasts until the key it used expires.
func guestSessionExpiry(now time.Time) time.Time {
	t := now.In(easternZone)
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, easternZone)
}

func easternDay(now time.Time) string {
	return now.In(easternZone).Format("2006-01-02")
}

// newGuestKey returns a 12-char base32 code (~60 bits of entropy).
func newGuestKey() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return s[:12], nil
}

func formatGuestKey(k string) string {
	var sb strings.Builder
	for i, r := range k {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func normalizeGuestKey(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// StaffProductDailyKeys returns today's independently generated key for every
// product that has opted into daily-key guest access.
func (h *Handler) StaffProductDailyKeys(w http.ResponseWriter, r *http.Request) {
	apps, err := h.Store.ListDailyKeyApps(r.Context())
	if err != nil {
		fail(r, err, "auth: list daily-key products")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	keys := make([]store.ProductDailyKey, 0, len(apps))
	day := easternDay(time.Now())
	for _, app := range apps {
		candidate, err := newGuestKey()
		if err != nil {
			fail(r, err, "auth: generate product daily key")
			httpx.Status(w, http.StatusInternalServerError)
			return
		}
		key, err := h.Store.EnsureProductDailyKey(r.Context(), app.ID, day, candidate)
		if err != nil {
			fail(r, err, "auth: ensure product daily key for "+app.ID)
			httpx.Status(w, http.StatusInternalServerError)
			return
		}
		key.AppName = app.Name
		key.Key = formatGuestKey(key.Key)
		keys = append(keys, key)
	}
	httpx.JSON(w, http.StatusOK, keys)
}

// GuestLogin validates today's key for one product and starts a session scoped
// to that product. A key from one app can never authenticate another app.
func (h *Handler) GuestLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AppID string `json:"app_id"`
		Key   string `json:"key"`
	}
	if err := httpx.Decode(w, r, &body); err != nil || strings.TrimSpace(body.AppID) == "" || strings.TrimSpace(body.Key) == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "app_id and key required"})
		return
	}

	now := time.Now()
	appID := strings.TrimSpace(body.AppID)
	candidate, err := newGuestKey()
	if err != nil {
		fail(r, err, "auth: generate guest key")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	gk, err := h.Store.EnsureProductDailyKey(r.Context(), appID, easternDay(now), candidate)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			h.Events.LogAsync(logclient.Warning, "Guest key refused", map[string]any{"app_id": appID})
			httpx.JSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or expired key"})
			return
		}
		fail(r, err, "auth: ensure product daily key")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	if subtle.ConstantTimeCompare([]byte(normalizeGuestKey(body.Key)), []byte(gk.Key)) != 1 {
		h.Events.LogAsync(logclient.Warning, "Guest key refused", map[string]any{"app_id": gk.AppID})
		httpx.JSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or expired key"})
		return
	}

	if err := h.Store.UpsertOperator(r.Context(), store.Operator{
		GitHubID: store.GuestGitHubID,
		Login:    store.GuestLogin,
		Name:     "Guest",
	}, false); err != nil {
		fail(r, err, "auth: upsert guest operator")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}

	expiry := guestSessionExpiry(now)
	token, err := h.Signer.Session(Session{
		GitHubID:   store.GuestGitHubID,
		GuestAppID: gk.AppID,
		GuestDay:   gk.Day,
		GuestKeyID: h.Signer.GuestKeyID(gk.AppID, gk.Day, gk.Key),
	}, expiry)
	if err != nil {
		fail(r, err, "auth: sign guest session")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}

	setSessionCookie(w, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.CookieDomain, time.Until(expiry))
	h.Events.LogAsync(logclient.Info, "Guest signed in", map[string]any{"app_id": gk.AppID})
	httpx.Status(w, http.StatusNoContent)
}

// RotateProductDailyKey replaces today's key of one product. Every guest
// session checks its key on each request, so the sessions that used the old
// key end at their next request; staff hand out the new one.
func (h *Handler) RotateProductDailyKey(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("app")
	candidate, err := newGuestKey()
	if err != nil {
		fail(r, err, "auth: generate guest key")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	key, err := h.Store.RotateProductDailyKey(r.Context(), appID, easternDay(time.Now()), candidate)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.JSON(w, http.StatusNotFound, map[string]any{"error": "no daily key for this product"})
			return
		}
		fail(r, err, "auth: rotate product daily key")
		httpx.Status(w, http.StatusInternalServerError)
		return
	}
	if h.Cache != nil {
		h.Cache.InvalidateGuestKeys(appID)
	}
	h.audit("Guest key rotated", OperatorFrom(r), map[string]any{"app_id": appID})
	key.Key = formatGuestKey(key.Key)
	httpx.JSON(w, http.StatusOK, key)
}

// guestKeyStands reports whether a guest session's key is still the current
// key of its tool and day, and the tool still uses daily keys. It answers the
// request and returns false otherwise: 401 for a rotated or withdrawn key,
// 503 when the store cannot say.
func (h *Handler) guestKeyStands(w http.ResponseWriter, r *http.Request, session Session) bool {
	key, err := h.Cache.GuestKey(r.Context(), session.GuestAppID, session.GuestDay)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, r, http.StatusUnauthorized)
			return false
		}
		fail(r, err, "auth: guest key lookup")
		httpx.Status(w, http.StatusServiceUnavailable)
		return false
	}
	want := h.Signer.GuestKeyID(session.GuestAppID, session.GuestDay, key)
	if subtle.ConstantTimeCompare([]byte(want), []byte(session.GuestKeyID)) != 1 {
		httpx.Error(w, r, http.StatusUnauthorized)
		return false
	}
	return true
}
