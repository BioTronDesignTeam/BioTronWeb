package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// handlerFunc is a handler that may fail. Returning the error, rather than
// writing it, keeps one place that turns errors into statuses and one place
// that tells the request log why a request failed.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func handle(fn handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			writeError(w, r, err)
		}
	})
}

// httpError is an error with the status it answers. Its message is what the
// log records; the body carries it in lower case.
type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }

var errBodyTooLarge = &httpError{status: http.StatusRequestEntityTooLarge, message: "Request Entity Too Large"}

func badRequest(message string) error {
	return &httpError{status: http.StatusBadRequest, message: message}
}

// writeError answers {"error": "..."} with the status the error maps to. A
// store miss is a 404 and a constraint is a 409; anything else is a 500 that
// names nothing internal.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	var known *httpError
	switch {
	case errors.As(err, &known):
		status = known.status
		message = strings.ToLower(known.message)
	case errors.Is(err, store.ErrNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, store.ErrConflict):
		status, message = http.StatusConflict, "the requested change conflicts with existing data"
	}
	if status >= 500 {
		log.Printf("request failed: %v", err)
	}
	httplog.SetError(r, err)
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		log.Printf("encode response: %v", err)
		status, raw = http.StatusInternalServerError, []byte(`{"error":"internal server error"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

// bind decodes a JSON body. A malformed body is the caller's fault, so it is
// a 400 rather than the 500 a decoder error would read as.
func bind(r *http.Request, destination any) error {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errBodyTooLarge
		}
		return badRequest("body must be a JSON object")
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return badRequest("body must be a JSON object")
	}
	return nil
}

// newCORS lets the admin page's origins make credentialed requests. A
// preflight is answered here with 204 and goes no further. Every other
// request gets Vary: Origin, and the allow headers when its origin is listed.
func newCORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[strings.ToLower(strings.TrimSpace(origin))] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			origin := r.Header.Get("Origin")
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			if origin == "" || !preflight {
				header.Add("Vary", "Origin")
				// An OPTIONS that is not a preflight is passed on bare.
				if origin != "" && r.Method != http.MethodOptions && allowed[strings.ToLower(origin)] {
					header.Set("Access-Control-Allow-Origin", origin)
					header.Set("Access-Control-Allow-Credentials", "true")
				}
				next.ServeHTTP(w, r)
				return
			}
			header.Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers, Origin")
			if allowed[strings.ToLower(origin)] {
				header.Set("Access-Control-Allow-Origin", origin)
				header.Set("Access-Control-Allow-Credentials", "true")
			}
			header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			header.Set("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// limitBody caps every request body; bind answers a body over it with 413.
func limitBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
