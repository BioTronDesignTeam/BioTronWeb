package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// Refusal is one request the client turned away.
type Refusal struct {
	// Status is 401 (sign in), 403 (signed in, not permitted, or a mutating
	// request without X-Requested-With), or 503 (Auth could not answer).
	Status int
	// Message is the default body text.
	Message string
	// Permission is the permission the route asked for, if any.
	Permission string
	// Err is the cause: ErrUnauthenticated, ErrForbidden, ErrMissingXHR, or
	// the outage. Require has already recorded it with httplog.SetError.
	Err error
}

// ErrorWriter answers a refusal. See WithErrorWriter.
type ErrorWriter func(w http.ResponseWriter, r *http.Request, refusal Refusal)

type sessionKey struct{}

// Require builds middleware that lets a request through only if its session
// holds permission on this tool.
//
// Wrap a whole prefix, not single handlers, so a route added under it later
// cannot forget the check:
//
//	mux.Handle("/v1/admin/", authz.Require("admin")(admin))
//
// The answers:
//
//	mutating method without X-Requested-With  -> 403, before Auth is asked
//	no cookie, or Auth refuses the session    -> 401 (sign in)
//	signed in without the permission          -> 403
//	Auth unreachable, unset, or unreadable    -> 503 (fail closed, never open)
//
// A request let through carries the session (SessionFrom, OperatorFrom), and
// its request event names the operator.
func (c *Client) Require(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A form on another site cannot set this header, and a browser
			// preflights a request that does, so it stops a cross-site post.
			if mutating(r.Method) && r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				c.refuse(w, r, Refusal{http.StatusForbidden, "X-Requested-With header required", permission, ErrMissingXHR})
				return
			}
			session, err := c.Lookup(r.Context(), r.Header.Get("Cookie"), permission)
			if err != nil {
				c.unavailable(w, r, permission, err)
				return
			}
			if !session.Authenticated() {
				c.refuse(w, r, Refusal{http.StatusUnauthorized, "authentication required", permission, ErrUnauthenticated})
				return
			}
			if !session.Can(permission) {
				c.refuse(w, r, Refusal{http.StatusForbidden, "permission required", permission, ErrForbidden})
				return
			}
			httplog.SetActor(r, session.Operator.Login)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, session)))
		})
	}
}

// StatusField names the JSON field that reports one permission in the status
// body, such as {Permission: "write", Field: "can_write"}.
type StatusField struct {
	Permission string
	Field      string
}

// StatusHandler serves the /v1/auth/status a tool's frontend reads to draw
// itself:
//
//	{"operator": {...}, "can_write": true, "permissions": {"write": true}, ...extra}
//
// operator is left out when no one is signed in. Each field reports its
// permission, in the order given; permissions repeats them by name; extra adds
// the tool's own keys, sorted. An anonymous caller gets 200 with every
// permission false. Only an Auth that cannot answer is an error: 503.
func (c *Client) StatusHandler(fields []StatusField, extra func(*http.Request) map[string]any) http.Handler {
	permissions := make([]string, len(fields))
	for i, field := range fields {
		permissions[i] = field.Permission
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := c.Lookup(r.Context(), r.Header.Get("Cookie"), permissions...)
		if err != nil {
			c.unavailable(w, r, "", err)
			return
		}
		var body object
		if session.Operator != nil {
			body.add("operator", session.Operator)
		}
		held := make(map[string]bool, len(fields))
		for _, field := range fields {
			body.add(field.Field, session.Can(field.Permission))
			held[field.Permission] = session.Can(field.Permission)
		}
		body.add("permissions", held)
		if extra != nil {
			more := extra(r)
			keys := make([]string, 0, len(more))
			for key := range more {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				body.add(key, more[key])
			}
		}
		encoded, err := body.encode()
		if err != nil {
			httplog.SetError(r, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		writeBody(w, http.StatusOK, encoded)
	})
}

// SessionFrom returns the session Require let through.
func SessionFrom(r *http.Request) (Session, bool) {
	session, ok := r.Context().Value(sessionKey{}).(Session)
	return session, ok
}

// OperatorFrom returns the operator Require let through, or nil on a request
// that did not pass Require.
func OperatorFrom(r *http.Request) *Operator {
	session, _ := SessionFrom(r)
	return session.Operator
}

// unavailable answers 503 for an Auth that could not answer, and says so on
// stdout and as an Error event: the response alone does not say why.
func (c *Client) unavailable(w http.ResponseWriter, r *http.Request, permission string, err error) {
	log.Printf("authorization check for %s failed: %v", c.appID, err)
	payload := map[string]any{"error": err.Error()}
	if permission != "" {
		payload["permission"] = permission
	}
	c.events.LogAsync(logclient.Error, "Authorization service unavailable", payload)
	c.refuse(w, r, Refusal{http.StatusServiceUnavailable, "authorization service unavailable", permission, err})
}

func (c *Client) refuse(w http.ResponseWriter, r *http.Request, refusal Refusal) {
	httplog.SetError(r, refusal.Err)
	c.writeError(w, r, refusal)
}

// writeJSONError is the default refusal body: compact JSON, no trailing
// newline, charset in the content type, as Exo's Fiber app answered.
func writeJSONError(w http.ResponseWriter, _ *http.Request, refusal Refusal) {
	var body object
	body.add("error", refusal.Message)
	if errors.Is(refusal.Err, ErrForbidden) {
		body.add("permission", refusal.Permission)
	}
	encoded, err := body.encode()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeBody(w, refusal.Status, encoded)
}

func writeBody(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func mutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// object is a JSON object that keeps its keys in the order they were added,
// so a body reads the same as the struct it replaced.
type object struct {
	keys   []string
	values []any
}

func (o *object) add(key string, value any) {
	o.keys = append(o.keys, key)
	o.values = append(o.values, value)
}

func (o *object) encode() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buffer.WriteByte(',')
		}
		name, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(o.values[i])
		if err != nil {
			return nil, err
		}
		buffer.Write(name)
		buffer.WriteByte(':')
		buffer.Write(value)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}
