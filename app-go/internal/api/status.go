// Package api holds the REST handlers (spec §6).
package api

import "net/http"

// Status is the health check served at /api/status and /status (spec §6.4).
func Status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
}
