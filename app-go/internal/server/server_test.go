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
