// Package httpx holds HTTP helpers shared by the API and the UI: the error response format
// (spec §6.2) and panic recovery. I03 adds JSON decoding, validation and Optional[T].
package httpx

import (
	"encoding/json"
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
