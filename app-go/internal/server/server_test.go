package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/logx"
)

func TestRoutes(t *testing.T) {
	ui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-From", "ui")
	})
	h := New(Options{UI: ui, Logger: slog.New(slog.DiscardHandler)})

	cases := []struct {
		method, path string
		wantStatus   int
		wantUI       bool
	}{
		{"GET", "/api/status", 200, false},
		{"GET", "/status", 200, false},
		{"HEAD", "/status", 200, false},
		{"POST", "/status", 405, false},
		{"DELETE", "/api/status", 405, false},
		{"GET", "/api/unknown", 404, false},
		{"GET", "/", 200, true},
		{"GET", "/static/app.css", 200, true},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.wantStatus {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.path, rec.Code, tc.wantStatus)
		}
		if got := rec.Header().Get("X-From") == "ui"; got != tc.wantUI {
			t.Errorf("%s %s: served by UI = %v, want %v", tc.method, tc.path, got, tc.wantUI)
		}
	}
}

func TestWithoutUI(t *testing.T) {
	rec := httptest.NewRecorder()
	New(Options{Logger: slog.New(slog.DiscardHandler)}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

// A panicking UI handler becomes a JSON 500, and the request log records the 500 with the
// route of the inner mux.
func TestPanicIsRecoveredAndLogged(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(logx.NewHandler(slog.LevelDebug, slog.NewJSONHandler(&logs, nil), nil))
	ui := http.NewServeMux()
	ui.HandleFunc("GET /boom/{id}", func(http.ResponseWriter, *http.Request) { panic("kaboom") })

	rec := httptest.NewRecorder()
	New(Options{UI: ui, Logger: logger}).ServeHTTP(rec, httptest.NewRequest("GET", "/boom/1", nil))
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"message":"Internal server error"`) {
		t.Fatalf("response %d %q", rec.Code, rec.Body.String())
	}

	var events []string
	var request map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatal(err)
		}
		events = append(events, m[logx.AttrEventName].(string))
		if m[logx.AttrEventName] == logx.EventHTTPRequest {
			request = m
		}
	}
	if strings.Join(events, ",") != logx.EventHTTPPanic+","+logx.EventHTTPRequest {
		t.Errorf("events %v", events)
	}
	if request[logx.AttrHTTPStatusCode] != float64(500) || request[logx.AttrHTTPRoute] != "/boom/{id}" {
		t.Errorf("request record %v", request)
	}
}

func TestHealthCheckLoggedAtDebug(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(logx.NewHandler(slog.LevelInfo, slog.NewTextHandler(&logs, nil), nil))
	h := New(Options{Logger: logger})
	for _, p := range []string{"/status", "/api/status"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}
	if logs.Len() != 0 {
		t.Errorf("health checks logged at info: %s", logs.String())
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/nope", nil))
	if !strings.Contains(logs.String(), "GET /api/nope 404") {
		t.Errorf("other request not logged: %q", logs.String())
	}
}

// Verify item: the headers of spec §9.4 on API and UI responses, but not on the proxy.
func TestSecurityHeadersAndProxyExemption(t *testing.T) {
	ui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("X-From", "proxy") })
	h := New(Options{UI: ui, Proxy: proxy, Logger: slog.New(slog.DiscardHandler), Production: true})

	for _, path := range []string{"/api/status", "/status", "/", "/api/unknown"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if !strings.HasPrefix(rec.Header().Get("Content-Security-Policy"), "default-src 'self'") ||
			rec.Header().Get("Strict-Transport-Security") == "" || rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
			t.Errorf("%s: headers %v", path, rec.Header())
		}
	}

	// The proxy is transparent: no security headers, and no cross-origin protection.
	r := httptest.NewRequest("POST", "/api/proxy/svc/x", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 || rec.Header().Get("X-From") != "proxy" || rec.Header().Get("Content-Security-Policy") != "" {
		t.Errorf("proxy: %d %v", rec.Code, rec.Header())
	}
}

// Verify item: a cross-origin browser POST is rejected, while curl passes.
func TestCrossOriginProtection(t *testing.T) {
	ui := http.NewServeMux()
	ui.HandleFunc("POST /ui/things", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) })
	h := New(Options{UI: ui, Logger: slog.New(slog.DiscardHandler)})

	r := httptest.NewRequest("POST", "http://tmock.test/ui/things", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "Cross-origin request rejected") {
		t.Errorf("cross-site: %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "http://tmock.test/ui/things", nil)) // like curl
	if rec.Code != 201 {
		t.Errorf("curl: %d %q", rec.Code, rec.Body.String())
	}

	// Without a proxy handler, /api/proxy/ is an unknown API route.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/proxy/svc", nil))
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `"status":404`) {
		t.Errorf("proxy without handler: %d %q", rec.Code, rec.Body.String())
	}
}

// Unknown API routes and wrong methods are answered in the §6.2 format.
func TestAPIErrorsAreJSON(t *testing.T) {
	h := New(Options{UI: http.NotFoundHandler(), Logger: slog.New(slog.DiscardHandler)})
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/unknown", `{"status":404,"message":"Not found"}`, 404},
		{"DELETE", "/api/status", `{"status":405,"message":"Method not allowed"}`, 405},
		{"POST", "/status", `{"status":405,"message":"Method not allowed"}`, 405},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.body {
			t.Errorf("%s %s: %d %q", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// A request rejected before routing has no http.route in the request log.
func TestRejectedRequestHasNoRoute(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(logx.NewHandler(slog.LevelDebug, slog.NewJSONHandler(&logs, nil), nil))
	h := New(Options{UI: http.NotFoundHandler(), Logger: logger})
	r := httptest.NewRequest("POST", "/ui/x", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	h.ServeHTTP(httptest.NewRecorder(), r)
	var rec map[string]any
	if err := json.Unmarshal(logs.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec[logx.AttrHTTPStatusCode] != float64(403) || rec[logx.AttrHTTPRoute] != nil {
		t.Errorf("log record %v", rec)
	}
}
