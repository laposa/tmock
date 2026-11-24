// Package server assembles the application's single http.Handler (spec §2.2).
//
// It is a backend package, so it must not import the UI (spec §1.3): the UI handler is
// built by cmd/tmock and passed in.
package server

import (
	"log/slog"
	"net/http"

	"github.com/laposa/tmock/app-go/internal/api"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
)

// Options configures the handler.
type Options struct {
	// UI serves every path outside /api/ and /status. If nil, those paths are 404.
	UI http.Handler
	// Logger receives the request log and panics. Nil means slog.Default().
	Logger *slog.Logger
}

// New returns the handler for the whole application: the routes, wrapped in the request
// log (outermost, so it also records panics) and panic recovery.
func New(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// The API has its own mux so that a wrong method on an API path is a 405 and an
	// unknown API path is a 404, instead of falling through to the UI.
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/status", api.Status)
	apiMux.HandleFunc("GET /status", api.Status)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiMux)
	mux.Handle("/status", apiMux)
	if opts.UI != nil {
		mux.Handle("/", opts.UI)
	}
	return logx.RequestLog(logger, httpx.Recover(logger, mux))
}
