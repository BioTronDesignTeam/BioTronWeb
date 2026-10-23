package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/httpx"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

type operatorKey struct{}

// withOperator returns r carrying the operator that RequireSession loaded.
func withOperator(r *http.Request, op *store.SessionOperator) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), operatorKey{}, op))
}

// RequireSession accepts any unexpired session. Org membership is not
// re-checked here — see DEFERRED.md (C2) before shipping a control path.
func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := cookieValue(r, SessionCookie)
		if token == "" {
			httpx.Error(w, r, http.StatusUnauthorized)
			return
		}
		var op *store.SessionOperator
		var err error
		if h.Cache != nil {
			op, err = h.Cache.GetSessionOperator(r.Context(), hashToken(token))
		} else {
			op, err = h.Store.GetSessionOperator(r.Context(), hashToken(token))
		}
		if err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrBanned) {
				httpx.Error(w, r, http.StatusUnauthorized)
				return
			}
			fail(r, err, "auth: session lookup")
			httpx.Status(w, http.StatusServiceUnavailable)
			return
		}
		// Every route that reaches a handler passes here first, so naming the
		// operator once is what puts an actor on each request event.
		httplog.SetActor(r, op.Login)
		next.ServeHTTP(w, withOperator(r, op))
	})
}

func (h *Handler) RequireXHR(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Requested-With") == "" {
			httpx.Error(w, r, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) RequireSuperuser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op := OperatorFrom(r)
		if op == nil || !op.IsSuperuser {
			httpx.Error(w, r, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) RequireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op := OperatorFrom(r)
		if op == nil || !op.IsStaff() {
			httpx.Error(w, r, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func OperatorFrom(r *http.Request) *store.SessionOperator {
	op, _ := r.Context().Value(operatorKey{}).(*store.SessionOperator)
	return op
}
