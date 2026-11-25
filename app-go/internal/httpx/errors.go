// Package httpx holds the HTTP plumbing shared by the REST API and the UI (spec §6, §9):
// the error format, JSON decoding and encoding, Optional[T], the validation helper, panic
// recovery, security headers, cross-origin protection and the proxy's CORS middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/laposa/tmock/app-go/internal/logx"
)

// ErrorBody is the shape of every API error response (spec §6.2).
type ErrorBody struct {
	Status  int               `json:"status"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// PublicError is an error whose status, message and details may be shown to the client.
// *Error and the use-case errors of internal/app implement it; any other error is
// answered with a generic 500 and logged.
type PublicError interface {
	error
	StatusCode() int
	PublicMessage() string
	FieldDetails() map[string]string
}

// Error is an HTTP-level error, such as a malformed request body.
type Error struct {
	Status  int
	Message string
	Details map[string]string // field path → message, for validation errors
}

// NewError returns an *Error with the given status and message.
func NewError(status int, message string) *Error {
	return &Error{Status: status, Message: message}
}

func (e *Error) Error() string                   { return fmt.Sprintf("%d %s", e.Status, e.Message) }
func (e *Error) StatusCode() int                 { return e.Status }
func (e *Error) PublicMessage() string           { return e.Message }
func (e *Error) FieldDetails() map[string]string { return e.Details }

// WriteError writes an error response in the §6.2 format.
func WriteError(w http.ResponseWriter, status int, message string, details map[string]string) {
	b, err := json.Marshal(ErrorBody{Status: status, Message: message, Details: details})
	if err != nil { // cannot happen for these types
		b = []byte(`{"status":500,"message":"Internal server error"}`)
		status = http.StatusInternalServerError
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}

// WriteErr answers a request with err: a PublicError as it is, anything else as a 500
// whose details are logged, never returned (spec §6.2).
func WriteErr(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var pe PublicError
	if errors.As(err, &pe) {
		WriteError(w, pe.StatusCode(), pe.PublicMessage(), pe.FieldDetails())
		return
	}
	logger.LogAttrs(r.Context(), slog.LevelError, "request failed: "+err.Error(),
		slog.String(logx.AttrHTTPMethod, r.Method),
		slog.String(logx.AttrURLPath, r.URL.Path),
		slog.String(logx.AttrErrorType, fmt.Sprintf("%T", err)),
		slog.String(logx.AttrExceptionMessage, err.Error()),
	)
	WriteError(w, http.StatusInternalServerError, "Internal server error", nil)
}

// Recover turns a panic in next into a 500 response and a tmock.http.panic event, so one
// bad request cannot take the process down. If the handler had already started the
// response, a clean 500 is impossible; the connection is aborted instead, so the client
// sees a failed request rather than a truncated "successful" one.
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw := &writeTracker{ResponseWriter: w}
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v) // a deliberate abort; net/http handles it quietly
			}
			logger.LogAttrs(r.Context(), slog.LevelError, fmt.Sprintf("handler panicked: %v", v),
				slog.String(logx.AttrEventName, logx.EventHTTPPanic),
				slog.String(logx.AttrHTTPMethod, r.Method),
				slog.String(logx.AttrURLPath, r.URL.Path),
				slog.String(logx.AttrExceptionType, fmt.Sprintf("%T", v)),
				slog.String(logx.AttrExceptionMessage, fmt.Sprint(v)),
				slog.String(logx.AttrExceptionStacktrace, string(debug.Stack())),
			)
			if tw.wrote {
				panic(http.ErrAbortHandler)
			}
			WriteError(w, http.StatusInternalServerError, "Internal server error", nil)
		}()
		next.ServeHTTP(tw, r)
	})
}

// writeTracker remembers whether the response has been started.
type writeTracker struct {
	http.ResponseWriter
	wrote bool
}

func (t *writeTracker) WriteHeader(code int) {
	if code >= 200 {
		t.wrote = true
	}
	t.ResponseWriter.WriteHeader(code)
}

func (t *writeTracker) Write(b []byte) (int, error) {
	t.wrote = true
	return t.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (t *writeTracker) Unwrap() http.ResponseWriter { return t.ResponseWriter }
