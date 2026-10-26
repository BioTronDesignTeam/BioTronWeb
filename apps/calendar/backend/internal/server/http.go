package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// bodyLimit is the largest request body the API reads.
const bodyLimit = 256 * 1024

// handlerFunc is a route that may fail. serve answers a returned error with
// the JSON error body and attaches it to the request's log event.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (h *Handler) serve(handle handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := handle(w, r); err != nil {
			httplog.SetError(r, err)
			writeError(w, err)
		}
	})
}

// httpError is a refusal with a status of its own. The message is the
// standard status text; the client sees it in lower case.
type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }

func newHTTPError(status int) *httpError {
	return &httpError{status: status, message: http.StatusText(status)}
}

var (
	errBadRequest   = newHTTPError(http.StatusBadRequest)
	errNotFound     = newHTTPError(http.StatusNotFound)
	errBodyTooLarge = newHTTPError(http.StatusRequestEntityTooLarge)
)

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	var refusal *httpError
	switch {
	case errors.As(err, &refusal):
		status = refusal.status
		message = strings.ToLower(refusal.message)
	case errors.Is(err, store.ErrNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, store.ErrConflict):
		status, message = http.StatusConflict, "the requested change conflicts with existing calendar data"
	}
	if status >= 500 {
		log.Printf("request failed: %v", err)
	}
	_ = writeJSON(w, status, map[string]any{"error": message})
}

// writeJSON sends value as the whole body.
func writeJSON(w http.ResponseWriter, status int, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	return nil
}

// decodeBody reads a JSON request body into out. A body whose Content-Type is
// not application/json, or that does not parse, is a 400; one over bodyLimit
// is a 413.
func decodeBody(r *http.Request, out any) error {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		return errBadRequest
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil {
		return errBadRequest
	}
	if len(body) > bodyLimit {
		return errBodyTooLarge
	}
	if err := json.Unmarshal(body, out); err != nil {
		return errBadRequest
	}
	return nil
}
