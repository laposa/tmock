// Package api holds the REST handlers (spec §6). Handlers are thin: decode, call a use
// case in internal/app, write the result (spec §2.1).
package api

import (
	"log/slog"
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/clientip"
	"github.com/laposa/tmock/app-go/internal/httpx"
)

// API is the REST API's handlers.
type API struct {
	app      *app.App
	sessions *auth.Sessions
	logger   *slog.Logger
}

// New returns the handlers. A nil logger means slog.Default().
func New(a *app.App, s *auth.Sessions, logger *slog.Logger) *API {
	if logger == nil {
		logger = slog.Default()
	}
	return &API{app: a, sessions: s, logger: logger}
}

// Register adds the routes of spec §6.4 to mux. The health checks are registered by
// internal/server, which also serves them without an API.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.Handle("GET /api/auth/me", a.sessions.RequireUser(http.HandlerFunc(a.me)))
	a.registerServices(mux)
	a.registerScenarios(mux)
	a.registerClients(mux)
}

// login is POST /api/auth/login: 200 with the user and the session cookie, 401 for a
// wrong email or password.
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in app.Credentials
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	addr := clientip.From(r)
	u, err := a.app.Login(r.Context(), in, addr)
	if err == nil {
		err = a.sessions.Start(r.Context(), w, u.ID, addr)
	}
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

// logout is POST /api/auth/logout: 204, whether or not there was a session.
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.sessions.End(w, r); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.NoContent(w)
}

// me is GET /api/auth/me (guard: user).
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	httpx.WriteJSON(w, http.StatusOK, u)
}
