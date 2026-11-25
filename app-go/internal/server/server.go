// Package server assembles the application's single http.Handler (spec §2.2).
//
// It is a backend package, so it must not import the UI (spec §1.3): the UI handler is
// built by cmd/tmock and passed in.
package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/laposa/tmock/app-go/internal/api"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
)

// Options configures the handler.
type Options struct {
	// UI serves every path outside /api/ and /status. If nil, those paths are 404.
	UI http.Handler
	// Proxy serves /api/proxy/ (from I09). It gets no security headers and no
	// cross-origin protection (spec §9.3, §9.4); it applies its own CORS (httpx.ProxyCORS).
	// If nil, /api/proxy/ paths are unknown API routes. It receives the path as sent, so it
	// must route with its own http.ServeMux, which cleans paths such as /api/proxy/../x.
	Proxy http.Handler
	// Logger receives the request log, panics and unexpected errors. Nil means slog.Default().
	Logger *slog.Logger
	// Production adds HSTS and upgrade-insecure-requests (spec §9.4).
	Production bool
}

// New returns the handler for the whole application:
//
//	request log → panic recovery → /api/proxy/*  → proxy
//	                             → everything else → security headers → cross-origin protection → routes
//
// The request log is outermost, so it also records panics.
func New(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// The API has its own mux, so that an unknown API path is a JSON 404 and a wrong
	// method a JSON 405, instead of falling through to the UI.
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/status", api.Status)
	apiMux.HandleFunc("GET /status", api.Status)
	apiHandler := httpx.JSONFallback(apiMux)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/status", apiHandler)
	if opts.UI != nil {
		mux.Handle("/", opts.UI)
	}
	protected := httpx.SecurityHeaders(opts.Production, httpx.CrossOrigin(mux))

	// A prefix check rather than a mux, so the request log's http.route only ever comes
	// from the mux that actually routed the request.
	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.Proxy != nil && strings.HasPrefix(r.URL.Path, "/api/proxy/") {
			opts.Proxy.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
	return logx.RequestLog(logger, httpx.Recover(logger, root))
}
