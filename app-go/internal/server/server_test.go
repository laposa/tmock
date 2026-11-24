package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutes(t *testing.T) {
	ui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-From", "ui")
	})
	h := New(Options{UI: ui})

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
	New(Options{}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}
