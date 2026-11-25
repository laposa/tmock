package api

import (
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/httpx"
)

// registerServices adds the service endpoints of spec §6.4 (guard: user). {path} is the
// service's path, which never contains "/"; a request for a/b is an unknown route.
func (a *API) registerServices(mux *http.ServeMux) {
	user := func(h http.HandlerFunc) http.Handler { return a.sessions.RequireUser(h) }
	mux.Handle("GET /api/service", user(a.listServices))
	mux.Handle("POST /api/service", user(a.createService))
	mux.Handle("GET /api/service/{path}", user(a.getService))
	mux.Handle("PATCH /api/service/{path}", user(a.updateService))
	mux.Handle("DELETE /api/service/{path}", user(a.deleteService))
}

func (a *API) listServices(w http.ResponseWriter, r *http.Request) {
	list, err := a.app.ListServices(r.Context())
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (a *API) createService(w http.ResponseWriter, r *http.Request) {
	var in app.ServiceInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	s, err := a.app.CreateService(r.Context(), actor(r), in)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, s)
}

func (a *API) getService(w http.ResponseWriter, r *http.Request) {
	s, err := a.app.GetService(r.Context(), r.PathValue("path"))
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

func (a *API) updateService(w http.ResponseWriter, r *http.Request) {
	var p app.ServicePatch
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	s, err := a.app.UpdateService(r.Context(), actor(r), r.PathValue("path"), p)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

func (a *API) deleteService(w http.ResponseWriter, r *http.Request) {
	if _, err := a.app.DeleteService(r.Context(), actor(r), r.PathValue("path")); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.NoContent(w)
}

// actor is the logged-in user that a guard put into the request's context.
func actor(r *http.Request) app.Actor {
	u, _ := auth.UserFrom(r.Context())
	return app.UserActor(u.ID)
}
