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

// RequireSession accepts a session cookie this service signed that has not
// expired, of an operator who is not banned and who is still a member of the
// organization, asked of GitHub at most once per MembershipTTL. A guest has
// no GitHub identity and skips the membership check.
func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := cookieValue(r, SessionCookie)
		if token == "" {
			httpx.Error(w, r, http.StatusUnauthorized)
			return
		}
		session, ok := h.Signer.ReadSession(token)
		if !ok {
			httpx.Error(w, r, http.StatusUnauthorized)
			return
		}
		// The cookie only names the operator. Roles and the ban come from the
		// store through the cache, which every change invalidates, so a ban
		// refuses the very next request. The superuser flag comes from
		// SUPERUSER_GITHUB_IDS, so removing an id and redeploying applies too.
		member, err := h.Cache.Operator(r.Context(), session.GitHubID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httpx.Error(w, r, http.StatusUnauthorized)
				return
			}
			fail(r, err, "auth: session lookup")
			httpx.Status(w, http.StatusServiceUnavailable)
			return
		}
		if member.IsBanned {
			httpx.Error(w, r, http.StatusUnauthorized)
			return
		}
		if session.GuestAppID != "" {
			if !h.guestKeyStands(w, r, session) {
				return
			}
		} else if !h.stillMember(w, r, member.GitHubID, member.Login) {
			return
		}
		op := &store.SessionOperator{
			GitHubID:    member.GitHubID,
			Login:       member.Login,
			Name:        member.Name,
			AvatarURL:   member.AvatarURL,
			IsSuperuser: h.isSuperuser(member.GitHubID),
			IsManager:   member.IsManager,
			GuestAppID:  session.GuestAppID,
		}
		// Every route that reaches a handler passes here first, so naming the
		// operator once is what puts an actor on each request event.
		httplog.SetActor(r, op.Login)
		next.ServeHTTP(w, withOperator(r, op))
	})
}

// stillMember answers the request and returns false unless the operator is
// still a member of the organization. Someone removed from it is refused
// with 401, as a ban is, so every tool sends them to sign in, where GitHub
// refuses them. When GitHub cannot answer and no answer of the last
// MembershipTTL is kept, the request fails closed with 503. The check goes
// by login, so an operator who renamed their GitHub account must sign in
// again to store the new one.
func (h *Handler) stillMember(w http.ResponseWriter, r *http.Request, githubID int64, login string) bool {
	if h.Members == nil {
		fail(r, errors.New("no membership check configured"), "auth: membership")
		httpx.Status(w, http.StatusServiceUnavailable)
		return false
	}
	member, err := h.Members.Member(r.Context(), githubID, login)
	if err != nil {
		fail(r, err, "auth: membership re-check")
		httpx.Status(w, http.StatusServiceUnavailable)
		return false
	}
	if !member {
		httplog.SetError(r, errors.New("no longer a member of the organization"))
		httpx.Error(w, r, http.StatusUnauthorized)
		return false
	}
	return true
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

// RequireNotGuest refuses a guest session with 403. A guest holds one tool's
// telemetry and needs nothing of Auth but /auth/me and /v1/check.
func (h *Handler) RequireNotGuest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op := OperatorFrom(r)
		if op == nil || op.IsGuest() {
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
