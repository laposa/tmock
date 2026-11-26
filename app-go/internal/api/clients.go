package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
)

// TokenHeader carries a client token (spec §6.3).
const TokenHeader = "X-Client-Token"

// registerClients adds the client endpoints of spec §6.4: the token-or-user guard on GET
// and PATCH /api/client/{id}, the user guard everywhere else.
func (a *API) registerClients(mux *http.ServeMux) {
	user := func(h http.HandlerFunc) http.Handler { return a.sessions.RequireUser(h) }
	mux.Handle("GET /api/client", user(a.listClients))
	mux.Handle("POST /api/client", user(a.createClient))
	mux.Handle("POST /api/client/disable-all", user(a.disableAllClients))
	mux.Handle("GET /api/client/{id}", a.tokenOrUser(http.HandlerFunc(a.getClient)))
	mux.Handle("PATCH /api/client/{id}", a.tokenOrUser(http.HandlerFunc(a.updateClient)))
	mux.Handle("DELETE /api/client/{id}", user(a.deleteClient))
	mux.Handle("POST /api/client/{id}/token", user(a.generateToken))
	mux.Handle("DELETE /api/client/{id}/token", user(a.revokeToken))
}

type clientActorKey struct{}

// tokenOrUser is the token-or-user guard (spec §6.3). With an X-Client-Token header the
// token must belong to a client (else 401), {id} must be that client (else 403) and the
// body may hold only enabled and scenarios (else 403, naming the fields); the session is
// not looked at. Without the header, the user guard applies.
func (a *API) tokenOrUser(next http.Handler) http.Handler {
	user := a.sessions.RequireUser(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(TokenHeader)
		if token == "" {
			user.ServeHTTP(w, r)
			return
		}
		clientID, err := a.app.ClientByToken(r.Context(), token)
		if err != nil {
			httpx.WriteErr(w, r, a.logger, err)
			return
		}
		id, err := httpx.PathID(r, "id")
		if err != nil {
			httpx.WriteErr(w, r, a.logger, err)
			return
		}
		if id != clientID {
			httpx.WriteErr(w, r, a.logger, app.Forbidden("This token belongs to another client"))
			return
		}
		if r.Method == http.MethodPatch {
			fields, ok, err := bodyFields(w, r)
			if err != nil {
				httpx.WriteErr(w, r, a.logger, err)
				return
			}
			if ok {
				var bad []string
				for _, f := range fields {
					if !slices.Contains(app.TokenFields, f) {
						bad = append(bad, f)
					}
				}
				if len(bad) > 0 {
					httpx.WriteErr(w, r, a.logger, app.Forbidden("A client token may only change enabled and scenarios, not: %s", strings.Join(bad, ", ")))
					return
				}
			}
		}
		ctx := context.WithValue(r.Context(), clientActorKey{}, clientID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bodyFields reads the request body and returns the names of its top-level fields, sorted,
// leaving the body in place for the handler. ok is false when the body is not a JSON
// object; the handler's decoder then reports the problem (400).
func bodyFields(w http.ResponseWriter, r *http.Request) (fields []string, ok bool, err error) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, httpx.MaxBodyBytes))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		return nil, false, httpx.NewError(http.StatusRequestEntityTooLarge, fmt.Sprintf("Request body is larger than %d MB", httpx.MaxBodyBytes>>20))
	case err != nil:
		return nil, false, httpx.NewError(http.StatusBadRequest, "The request body could not be read")
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	var obj map[string]json.RawMessage
	if json.Unmarshal(b, &obj) != nil {
		return nil, false, nil
	}
	for k := range obj {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	return fields, true, nil
}

func (a *API) listClients(w http.ResponseWriter, r *http.Request) {
	list, err := a.app.ListClients(r.Context())
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (a *API) createClient(w http.ResponseWriter, r *http.Request) {
	var in app.ClientInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	c, err := a.app.CreateClient(r.Context(), actor(r), in)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (a *API) getClient(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	c, err := a.app.GetClient(r.Context(), id)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (a *API) updateClient(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	var p app.ClientPatch
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	c, err := a.app.UpdateClient(r.Context(), actor(r), id, p)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (a *API) deleteClient(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	if _, err := a.app.DeleteClient(r.Context(), actor(r), id); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.NoContent(w)
}

func (a *API) disableAllClients(w http.ResponseWriter, r *http.Request) {
	if err := a.app.DisableAllClients(r.Context(), actor(r)); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.NoContent(w)
}

func (a *API) generateToken(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	t, err := a.app.GenerateClientToken(r.Context(), actor(r), id)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, t)
}

func (a *API) revokeToken(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	if err := a.app.RevokeClientToken(r.Context(), actor(r), id); err != nil {
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	httpx.NoContent(w)
}
