package logx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// capture is a slog.Handler that keeps every record, flattened to key → value strings.
type capture struct {
	mu      *sync.Mutex
	records *[]captured
	attrs   []slog.Attr
	group   string
}

type captured struct {
	ctx   context.Context
	level slog.Level
	msg   string
	attrs map[string]string
}

func newCapture() *capture { return &capture{mu: &sync.Mutex{}, records: &[]captured{}} }

func (c *capture) Enabled(context.Context, slog.Level) bool { return true }

func (c *capture) Handle(ctx context.Context, r slog.Record) error {
	m := map[string]string{}
	for _, a := range c.attrs {
		m[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if c.group != "" {
			key = c.group + "." + key
		}
		m[key] = a.Value.String()
		return true
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	*c.records = append(*c.records, captured{ctx: ctx, level: r.Level, msg: r.Message, attrs: m})
	return nil
}

func (c *capture) WithAttrs(as []slog.Attr) slog.Handler {
	n := *c
	n.attrs = append(append([]slog.Attr{}, c.attrs...), as...)
	return &n
}

func (c *capture) WithGroup(name string) slog.Handler {
	n := *c
	n.group = name
	return &n
}

func (c *capture) all() []captured {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]captured(nil), *c.records...)
}

const validTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestParseTraceparent(t *testing.T) {
	tc, ok := ParseTraceparent(validTraceparent)
	if !ok || tc.TraceIDHex() != "4bf92f3577b34da6a3ce929d0e0e4736" || tc.SpanIDHex() != "00f067aa0ba902b7" || tc.Flags != 1 {
		t.Fatalf("got %+v %v", tc, ok)
	}
	for _, bad := range []string{
		"",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",  // other version
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01",  // upper case
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",  // zero trace id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",  // zero span id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-", // too long
		"00-4bf92f3577b34da6a3ce929d0e0e473x-00f067aa0ba902b7-01",  // not hex
		"00_4bf92f3577b34da6a3ce929d0e0e4736_00f067aa0ba902b7_01",  // separators
	} {
		if _, ok := ParseTraceparent(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestRoute(t *testing.T) {
	for in, want := range map[string]string{
		"":                        "",
		"/":                       "/",
		"GET /{$}":                "/{$}",
		"GET /api/service/{path}": "/api/service/{path}",
		"POST example.com/x":      "/x",
		"example.com/":            "/",
	} {
		if got := Route(in); got != want {
			t.Errorf("Route(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFanout(t *testing.T) {
	var console bytes.Buffer
	export := newCapture()
	h := NewHandler(slog.LevelInfo, NewConsoleHandler(&console, "json", slog.LevelDebug), export)
	logger := slog.New(h).With("app", "tmock").WithGroup("req")

	tc, _ := ParseTraceparent(validTraceparent)
	ctx := ContextWithTrace(context.Background(), tc)
	logger.DebugContext(ctx, "filtered out")
	logger.InfoContext(ctx, "hello", "id", 7)

	var line map[string]any
	if err := json.Unmarshal(console.Bytes(), &line); err != nil {
		t.Fatalf("console output %q: %v", console.String(), err)
	}
	req, _ := line["req"].(map[string]any)
	if line["msg"] != "hello" || line["app"] != "tmock" || req["id"] != float64(7) {
		t.Errorf("console line %v", line)
	}
	if req["trace_id"] != tc.TraceIDHex() || req["span_id"] != tc.SpanIDHex() {
		t.Errorf("console trace ids missing: %v", line)
	}

	recs := export.all()
	if len(recs) != 1 {
		t.Fatalf("export got %d records", len(recs))
	}
	r := recs[0]
	if r.msg != "hello" || r.attrs["app"] != "tmock" || r.attrs["req.id"] != "7" {
		t.Errorf("export record %+v", r)
	}
	if _, ok := r.attrs["req.trace_id"]; ok {
		t.Error("export record got console-only trace attributes")
	}
	if got, ok := TraceFromContext(r.ctx); !ok || got != tc {
		t.Error("export handler did not get the trace context")
	}
}

func TestFanoutWithoutExport(t *testing.T) {
	var console bytes.Buffer
	logger := slog.New(NewHandler(slog.LevelWarn, NewConsoleHandler(&console, "text", slog.LevelDebug), nil))
	logger.Info("no")
	logger.With("a", 1).WithGroup("g").Warn("yes", "b", 2)
	out := console.String()
	if strings.Contains(out, "msg=no") || !strings.Contains(out, "msg=yes a=1 g.b=2") {
		t.Errorf("console %q", out)
	}
}

func serveLogged(t *testing.T, h http.Handler, r *http.Request) []captured {
	t.Helper()
	logs := newCapture()
	logger := slog.New(NewHandler(slog.LevelDebug, logs, nil))
	RequestLog(logger, h).ServeHTTP(httptest.NewRecorder(), r)
	return logs.all()
}

func TestRequestLog(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/service/{path}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	r := httptest.NewRequest("GET", "/api/service/users?expand=1", nil)
	r.RemoteAddr = "192.0.2.10:5555"
	r.Header.Set("Traceparent", validTraceparent)

	recs := serveLogged(t, mux, r)
	if len(recs) != 1 {
		t.Fatalf("%d records", len(recs))
	}
	got := recs[0]
	if got.level != slog.LevelInfo {
		t.Errorf("level %v", got.level)
	}
	if !strings.HasPrefix(got.msg, "192.0.2.10 GET /api/service/users?expand=1 418 ") || !strings.HasSuffix(got.msg, "ms") {
		t.Errorf("message %q", got.msg)
	}
	want := map[string]string{
		AttrEventName:      EventHTTPRequest,
		AttrHTTPMethod:     "GET",
		AttrHTTPRoute:      "/api/service/{path}",
		AttrURLPath:        "/api/service/users",
		AttrURLQuery:       "expand=1",
		AttrHTTPStatusCode: "418",
		AttrClientAddress:  "192.0.2.10",
	}
	for k, v := range want {
		if got.attrs[k] != v {
			t.Errorf("%s = %q, want %q", k, got.attrs[k], v)
		}
	}
	if _, ok := got.attrs[AttrDurationMS]; !ok {
		t.Error("no duration")
	}
	if tc, ok := TraceFromContext(got.ctx); !ok || tc.TraceIDHex() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Error("trace context not in the record's context")
	}
}

func TestRequestLogDefaults(t *testing.T) {
	// Nothing written: logged as 200. Health checks: debug. No route, no query: no attrs.
	noop := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for path, wantLevel := range map[string]slog.Level{"/status": slog.LevelDebug, "/api/status": slog.LevelDebug, "/other": slog.LevelInfo} {
		recs := serveLogged(t, noop, httptest.NewRequest("GET", path, nil))
		if len(recs) != 1 || recs[0].level != wantLevel || recs[0].attrs[AttrHTTPStatusCode] != "200" {
			t.Fatalf("%s: %+v", path, recs)
		}
		for _, k := range []string{AttrHTTPRoute, AttrURLQuery} {
			if _, ok := recs[0].attrs[k]; ok {
				t.Errorf("%s: unexpected %s", path, k)
			}
		}
	}
}

func TestRequestLogUserID(t *testing.T) {
	identified := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { SetUserID(r.Context(), 42) })
	recs := serveLogged(t, identified, httptest.NewRequest("GET", "/x", nil))
	if len(recs) != 1 || recs[0].attrs[AttrUserID] != "42" {
		t.Errorf("records %+v", recs)
	}
	recs = serveLogged(t, http.NotFoundHandler(), httptest.NewRequest("GET", "/x", nil))
	if _, ok := recs[0].attrs[AttrUserID]; ok {
		t.Error("user.id without a user")
	}
	SetUserID(context.Background(), 1) // outside RequestLog: no effect, no panic
}

func TestRequestLogSkippedBelowLevel(t *testing.T) {
	logs := newCapture()
	logger := slog.New(NewHandler(slog.LevelInfo, logs, nil))
	RequestLog(logger, http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/status", nil))
	if n := len(logs.all()); n != 0 {
		t.Errorf("health check logged at info: %d records", n)
	}
}

func TestRequestLogOnPanic(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		panic(http.ErrAbortHandler)
	})
	logs := newCapture()
	h := RequestLog(slog.New(NewHandler(slog.LevelDebug, logs, nil)), panicking)
	func() {
		defer func() { _ = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	}()
	if recs := logs.all(); len(recs) != 1 || recs[0].attrs[AttrHTTPStatusCode] != "500" {
		t.Errorf("records %+v", recs)
	}
}

// Spec §2.4.3: credentials, header values and bodies never reach the log. A real login
// runs in TestLoginNeverLogsSecrets (secrets_test.go); later iterations add a token PATCH
// and a proxied request.
func TestRequestLogNeverLogsSecrets(t *testing.T) {
	secrets := []string{"sess-cookie-value", "bearer-token-value", "client-token-value", "api-key-value", "body-password-value", "user-agent-value"}
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"body-password-value"}`))
	r.Header.Set("Cookie", "tmock_session=sess-cookie-value")
	r.Header.Set("Authorization", "Bearer bearer-token-value")
	r.Header.Set("X-Client-Token", "client-token-value")
	r.Header.Set("Apikey", "api-key-value")
	r.Header.Set("User-Agent", "user-agent-value")
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "tmock_session", Value: "sess-cookie-value"})
		w.WriteHeader(http.StatusOK)
	})

	for _, rec := range serveLogged(t, echo, r) {
		all := rec.msg
		for k, v := range rec.attrs {
			all += " " + k + "=" + v
		}
		for _, s := range secrets {
			if strings.Contains(all, s) {
				t.Errorf("record leaks %q: %s", s, all)
			}
		}
	}
}
