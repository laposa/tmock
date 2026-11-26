package api

import (
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
)

// registerScenarios adds the scenario endpoints of spec §6.4 (guard: user). {id} is the
// scenario's numeric id, written canonically (httpx.PathID).
func (a *API) registerScenarios(mux *http.ServeMux) {
	user := func(h http.HandlerFunc) http.Handler { return a.sessions.RequireUser(h) }
	mux.Handle("GET /api/scenario", user(a.listScenarios))
	mux.Handle("POST /api/scenario", user(a.createScenario))
	mux.Handle("GET /api/scenario/{id}", user(a.getScenario))
	mux.Handle("PATCH /api/scenario/{id}", user(a.updateScenario))
	mux.Handle("DELETE /api/scenario/{id}", user(a.deleteScenario))
}

// listScenarios is GET /api/scenario, optionally ?service=<path>.
func (a *API) listScenarios(w http.ResponseWriter, r *http.Request) {
	list, err := a.app.ListScenarios(r.Context(), r.URL.Query().Get("service"))
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (a *API) createScenario(w http.ResponseWriter, r *http.Request) {
	var in app.ScenarioInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	s, err := a.app.CreateScenario(r.Context(), actor(r), in)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, s)
}

func (a *API) getScenario(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	s, err := a.app.GetScenario(r.Context(), id)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

func (a *API) updateScenario(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	var p app.ScenarioPatch
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	s, err := a.app.UpdateScenario(r.Context(), actor(r), id, p)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

func (a *API) deleteScenario(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	if _, err := a.app.DeleteScenario(r.Context(), actor(r), id); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.NoContent(w)
}
