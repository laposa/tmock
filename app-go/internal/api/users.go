package api

import (
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/httpx"
)

// registerUsers adds the user endpoints of spec §6.4: the profile for every user, the
// rest for admins.
func (a *API) registerUsers(mux *http.ServeMux) {
	user := func(h http.HandlerFunc) http.Handler { return a.sessions.RequireUser(h) }
	admin := func(h http.HandlerFunc) http.Handler { return a.sessions.RequireAdmin(h) }
	mux.Handle("GET /api/user/profile", user(a.me))
	mux.Handle("PATCH /api/user/profile", user(a.updateProfile))
	mux.Handle("GET /api/user", admin(a.listUsers))
	mux.Handle("POST /api/user", admin(a.createUser))
	mux.Handle("PATCH /api/user/{id}", admin(a.updateUser))
	mux.Handle("DELETE /api/user/{id}", admin(a.deleteUser))
}

func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	var p app.ProfilePatch
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	me, _ := auth.UserFrom(r.Context())
	u, err := a.app.UpdateProfile(r.Context(), me.ID, p)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	list, err := a.app.ListUsers(r.Context())
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var in app.UserInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	u, err := a.app.CreateUser(r.Context(), actor(r), in)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, u)
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	var p app.UserPatch
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	u, err := a.app.UpdateUser(r.Context(), actor(r), id, p)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	if _, err := a.app.DeleteUser(r.Context(), actor(r), id); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.NoContent(w)
}
