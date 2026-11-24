package logx

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/laposa/tmock/app-go/internal/clientip"
)

// RequestLog is the request-log middleware (spec §9.6). It parses an incoming traceparent
// into the request context (spec §2.4.4) and, when the request finishes, logs one
// tmock.http.request event with the line "<ip> <METHOD> <url> <status> <ms>ms". Health
// checks are logged at debug level. Header values and bodies are never logged.
//
// It records the response even if the handler panics, so wrap it around the panic
// recovery middleware, not inside it.
func RequestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if tc, ok := ParseTraceparent(r.Header.Get("Traceparent")); ok {
			r = r.WithContext(ContextWithTrace(r.Context(), tc))
		}
		rec := &statusRecorder{ResponseWriter: w}
		defer func() {
			// r.Pattern is filled in by the ServeMux that routed the request.
			logRequest(logger, r, rec.status(), time.Since(start))
		}()
		next.ServeHTTP(rec, r)
	})
}

func logRequest(logger *slog.Logger, r *http.Request, status int, d time.Duration) {
	level := slog.LevelInfo
	if r.URL.Path == "/status" || r.URL.Path == "/api/status" {
		level = slog.LevelDebug
	}
	ctx := r.Context()
	if !logger.Enabled(ctx, level) {
		return
	}
	ip := clientip.From(r)
	ms := d.Milliseconds()
	attrs := []slog.Attr{
		slog.String(AttrEventName, EventHTTPRequest),
		slog.String(AttrHTTPMethod, r.Method),
		slog.String(AttrURLPath, r.URL.Path),
		slog.Int(AttrHTTPStatusCode, status),
		slog.String(AttrClientAddress, ip),
		slog.Int64(AttrDurationMS, ms),
	}
	if route := Route(r.Pattern); route != "" {
		attrs = append(attrs, slog.String(AttrHTTPRoute, route))
	}
	if r.URL.RawQuery != "" {
		attrs = append(attrs, slog.String(AttrURLQuery, r.URL.RawQuery))
	}
	msg := fmt.Sprintf("%s %s %s %d %dms", ip, r.Method, r.URL.RequestURI(), status, ms)
	logger.LogAttrs(ctx, level, msg, attrs...)
}

// Route turns a ServeMux pattern such as "GET example.com/api/{id}" into the http.route
// value "/api/{id}": the path template without method and host.
func Route(pattern string) string {
	if _, rest, ok := strings.Cut(pattern, " "); ok {
		pattern = rest
	}
	if i := strings.IndexByte(pattern, '/'); i > 0 {
		pattern = pattern[i:]
	}
	return pattern
}

// statusRecorder remembers the response status code.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.code == 0 && code >= 200 { // 1xx responses are informational
		s.code = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) status() int {
	if s.code == 0 {
		return http.StatusOK // the handler wrote nothing; net/http sends 200
	}
	return s.code
}

// Unwrap lets http.ResponseController reach the underlying writer (Flush, deadlines).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
