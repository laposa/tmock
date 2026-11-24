// Package server assembles the application's single http.Handler (spec §2.2).
//
// It is a backend package, so it must not import the UI (spec §1.3): the UI handler is
// built by cmd/tmock and passed in.
package server

import (
	"net/http"

	"github.com/laposa/tmock/app-go/internal/api"
)

// Options configures the handler.
type Options struct {
	// UI serves every path outside /api/ and /status. If nil, those paths are 404.
	UI http.Handler
}

// New returns the handler for the whole application.
func New(opts Options) http.Handler {
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
	return mux
}
