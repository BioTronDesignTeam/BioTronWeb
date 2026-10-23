// Package httpx writes the few kinds of answer the API gives. Each one is
// byte-for-byte what the Fiber version of this service sent, because the
// frontends and the other backends were written against those answers.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// maxBody is the largest request body read, Fiber's default limit.
const maxBody = 4 << 20

// ErrBody is the error of a request body that is not one JSON value of
// Content-Type application/json.
var ErrBody = errors.New("request body is not JSON")

// JSON answers status with v as JSON: one encoding, no trailing newline.
func JSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Printf("encode response: %v", err)
		Status(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// Text answers status with a plain-text body.
func Text(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// Status answers status with its reason phrase as the body, or no body for a
// status that allows none.
func Status(w http.ResponseWriter, status int) {
	if status == http.StatusNoContent || status == http.StatusNotModified || status < 200 {
		w.WriteHeader(status)
		return
	}
	Text(w, status, http.StatusText(status))
}

// Error answers status like Status and names the status as the cause on the
// request's completion event. Use it where the Fiber handler returned a bare
// fiber.Err*; a handler that already recorded a cause uses Status.
func Error(w http.ResponseWriter, r *http.Request, status int) {
	httplog.SetError(r, errors.New(http.StatusText(status)))
	Status(w, status)
}

// Redirect sends the browser to location with 302 Found and no body.
func Redirect(w http.ResponseWriter, location string) {
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusFound)
}

// Decode reads a JSON request body into out. Only Content-Type
// application/json is read; anything else, an empty body, or more than one
// value is ErrBody, which every caller answers with 400.
func Decode(w http.ResponseWriter, r *http.Request, out any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return ErrBody
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		return ErrBody
	}
	if err := json.Unmarshal(body, out); err != nil {
		return ErrBody
	}
	return nil
}
