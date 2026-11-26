package web

import (
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/condition/builder"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/views/pages"
)

// registerClients adds the clients page (spec §10.3 C1–C8) and its fragments.
func (u *ui) registerClients(mux *http.ServeMux) {
	mux.Handle("GET /{$}", u.guarded(false, u.clientsPage))
	mux.Handle("GET /ui/clients/table", u.guarded(false, u.clientsTable))
	mux.Handle("GET /ui/clients/new", u.guarded(false, u.newClientForm))
	mux.Handle("POST /ui/clients", u.guarded(false, u.createClient))
	mux.Handle("GET /ui/clients/{id}/edit", u.guarded(false, u.editClientForm))
	mux.Handle("POST /ui/clients/{id}", u.guarded(false, u.updateClient))
	mux.Handle("DELETE /ui/clients/{id}", u.guarded(false, u.deleteClient))
	mux.Handle("POST /ui/clients/{id}/enabled", u.guarded(false, u.setClientEnabled))
	mux.Handle("GET /ui/clients/{id}/scenarios", u.guarded(false, u.assignmentsModal))
	mux.Handle("POST /ui/clients/{id}/scenarios/{scenario}", u.guarded(false, u.setAssignment))
	mux.Handle("DELETE /ui/clients/{id}/scenarios/{scenario}", u.guarded(false, u.removeAssignment))
	mux.Handle("GET /ui/clients/{id}/condition", u.guarded(false, u.conditionBuilder))
	mux.Handle("POST /ui/clients/{id}/condition/edit", u.guarded(false, u.editCondition))
	mux.Handle("POST /ui/clients/{id}/condition", u.guarded(false, u.saveCondition))
	mux.Handle("POST /ui/clients/{id}/token", u.guarded(false, u.generateToken))
	mux.Handle("DELETE /ui/clients/{id}/token", u.guarded(false, u.revokeToken))
}

func (u *ui) clientsPage(w http.ResponseWriter, r *http.Request, user app.User) {
	list, err := u.app.ListClients(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.render(w, r, http.StatusOK, pages.Clients(pages.ClientsProps{Admin: user.Admin, Clients: list}))
}

// clientsTable is the table alone; it reloads itself on pages.ClientsChangedEvent.
func (u *ui) clientsTable(w http.ResponseWriter, r *http.Request, user app.User) {
	list, err := u.app.ListClients(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.render(w, r, http.StatusOK, pages.ClientsTable(list))
}

// newClientForm is the add modal (C3).
func (u *ui) newClientForm(w http.ResponseWriter, r *http.Request, user app.User) {
	u.clientForm(w, r, user, http.StatusOK, pages.ClientFormProps{})
}

// editClientForm is the edit modal (C4).
func (u *ui) editClientForm(w http.ResponseWriter, r *http.Request, user app.User) {
	c, ok := u.pathClient(w, r)
	if !ok {
		return
	}
	u.clientForm(w, r, user, http.StatusOK, pages.ClientFormProps{ID: c.ID, Name: c.Name})
}

// createClient creates a client from the add modal: name only, disabled.
func (u *ui) createClient(w http.ResponseWriter, r *http.Request, user app.User) {
	if !parseForm(w, r) {
		return
	}
	name := r.PostForm.Get("name")
	c, err := u.app.CreateClient(r.Context(), app.UserActor(user.ID), app.ClientInput{Name: name})
	if err != nil {
		u.clientFormError(w, r, user, pages.ClientFormProps{Name: name}, err)
		return
	}
	u.clientsChanged(w, r, http.StatusOK, toastSuccess, `Client "`+c.Name+`" created`, "", true)
}

// updateClient saves the edit modal: the name.
func (u *ui) updateClient(w http.ResponseWriter, r *http.Request, user app.User) {
	id, ok := u.pathID(w, r, "id")
	if !ok || !parseForm(w, r) {
		return
	}
	p := pages.ClientFormProps{ID: id, Name: r.PostForm.Get("name")}
	c, err := u.app.UpdateClient(r.Context(), app.UserActor(user.ID), id, app.ClientPatch{Name: httpx.Some(p.Name)})
	if err != nil {
		// The form shows the typed name; Delete names the client as stored.
		stored, getErr := u.app.GetClient(r.Context(), id)
		if getErr != nil {
			u.fail(w, r, getErr)
			return
		}
		p.StoredName = stored.Name
		u.clientFormError(w, r, user, p, err)
		return
	}
	u.clientsChanged(w, r, http.StatusOK, toastSuccess, `Client "`+c.Name+`" updated`, "", true)
}

func (u *ui) deleteClient(w http.ResponseWriter, r *http.Request, user app.User) {
	id, ok := u.pathID(w, r, "id")
	if !ok {
		return
	}
	c, err := u.app.DeleteClient(r.Context(), app.UserActor(user.ID), id)
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.clientsChanged(w, r, http.StatusOK, toastSuccess, `Client "`+c.Name+`" deleted`, "", true)
}

// setClientEnabled is the table's switch (C2). Either way the answer is the table as
// stored, so a refused change puts the switch back.
func (u *ui) setClientEnabled(w http.ResponseWriter, r *http.Request, user app.User) {
	id, ok := u.pathID(w, r, "id")
	if !ok || !parseForm(w, r) {
		return
	}
	enabled := r.PostForm.Get("enabled") != ""
	c, err := u.app.UpdateClient(r.Context(), app.UserActor(user.ID), id, app.ClientPatch{Enabled: httpx.Some(enabled)})
	var appErr *app.Error
	switch {
	case err == nil:
		state := "disabled"
		if c.Enabled {
			state = "enabled"
		}
		u.clientsChanged(w, r, http.StatusOK, toastSuccess, `Client "`+c.Name+`" `+state, "", false)
	case errors.As(err, &appErr) && appErr.Kind == app.KindInvalid:
		verb := "disabled"
		if enabled {
			verb = "enabled"
		}
		u.clientsChanged(w, r, http.StatusUnprocessableEntity, toastError, "The client was not "+verb, detailsText(appErr), false)
	default:
		u.fail(w, r, err)
	}
}

// assignmentsModal is the assignment modal (C6).
func (u *ui) assignmentsModal(w http.ResponseWriter, r *http.Request, user app.User) {
	c, ok := u.pathClient(w, r)
	if !ok {
		return
	}
	list, err := u.app.ListScenarios(r.Context(), "")
	if err != nil {
		u.fail(w, r, err)
		return
	}
	services, err := u.app.ListServices(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	p := pages.AssignmentsProps{Client: c, Scenarios: list, Services: services}
	if isHTMX(r) {
		u.render(w, r, http.StatusOK, pages.Assignments(p))
		return
	}
	u.render(w, r, http.StatusOK, pages.AssignmentsPage(user.Admin, p))
}

// setAssignment is a switch of the assignment modal (C6): it saves at once and answers
// with the switch as stored; the table behind the modal reloads.
func (u *ui) setAssignment(w http.ResponseWriter, r *http.Request, user app.User) {
	id, ok := u.pathID(w, r, "id")
	if !ok {
		return
	}
	sid, ok := u.pathID(w, r, "scenario")
	if !ok || !parseForm(w, r) {
		return
	}
	want := r.PostForm.Get("assigned") != ""
	s, err := u.app.GetScenario(r.Context(), sid)
	if err != nil {
		u.fail(w, r, err)
		return
	}
	c, err := u.app.SetClientScenario(r.Context(), app.UserActor(user.ID), id, sid, want)
	if err != nil {
		var appErr *app.Error
		if !errors.As(err, &appErr) || appErr.Kind != app.KindInvalid {
			u.fail(w, r, err)
			return
		}
		cur, gerr := u.app.GetClient(r.Context(), id)
		if gerr != nil {
			u.fail(w, r, gerr)
			return
		}
		toast(w, toastError, "The assignment was not saved", detailsText(appErr))
		u.render(w, r, http.StatusUnprocessableEntity, pages.AssignmentSwitch(id, s, slices.Contains(cur.Scenarios, sid)))
		return
	}
	msg := `Scenario "` + s.Name + `" assigned to "` + c.Name + `"`
	if !want {
		msg = "Scenario removed from client"
	}
	toast(w, toastSuccess, msg, "")
	trigger(w, pages.ClientsChangedEvent, map[string]string{})
	if !isHTMX(r) {
		redirect(w, r, "/")
		return
	}
	u.render(w, r, http.StatusOK, pages.AssignmentSwitch(id, s, slices.Contains(c.Scenarios, sid)))
}

// removeAssignment is a chip's ✕ (C5).
func (u *ui) removeAssignment(w http.ResponseWriter, r *http.Request, user app.User) {
	id, ok := u.pathID(w, r, "id")
	if !ok {
		return
	}
	sid, ok := u.pathID(w, r, "scenario")
	if !ok {
		return
	}
	if _, err := u.app.SetClientScenario(r.Context(), app.UserActor(user.ID), id, sid, false); err != nil {
		u.fail(w, r, err)
		return
	}
	u.clientsChanged(w, r, http.StatusOK, toastSuccess, "Scenario removed from client", "", false)
}

// generateToken is Generate and Regenerate (C8): the new token in the modal, shown once.
func (u *ui) generateToken(w http.ResponseWriter, r *http.Request, user app.User) {
	c, ok := u.pathClient(w, r)
	if !ok {
		return
	}
	t, err := u.app.GenerateClientToken(r.Context(), app.UserActor(user.ID), c.ID)
	if err != nil {
		u.fail(w, r, err)
		return
	}
	if !isHTMX(r) {
		u.render(w, r, http.StatusOK, pages.TokenPage(user.Admin, c.Name, t.Token))
		return
	}
	trigger(w, pages.ClientsChangedEvent, map[string]string{})
	u.render(w, r, http.StatusOK, pages.TokenModal(c.Name, t.Token))
}

// revokeToken is Revoke (C8; it asks first).
func (u *ui) revokeToken(w http.ResponseWriter, r *http.Request, user app.User) {
	c, ok := u.pathClient(w, r)
	if !ok {
		return
	}
	if err := u.app.RevokeClientToken(r.Context(), app.UserActor(user.ID), c.ID); err != nil {
		u.fail(w, r, err)
		return
	}
	u.clientsChanged(w, r, http.StatusOK, toastSuccess, `Token of "`+c.Name+`" revoked`, "", false)
}

// clientForm answers with a client form: modal content for htmx, a page without.
func (u *ui) clientForm(w http.ResponseWriter, r *http.Request, user app.User, status int, p pages.ClientFormProps) {
	if isHTMX(r) {
		u.render(w, r, status, pages.ClientForm(p))
		return
	}
	u.render(w, r, status, pages.ClientFormPage(user.Admin, p))
}

// clientFormError shows a failed save of the add or edit modal (spec §10.2) as 422 with
// the form re-rendered and an error toast.
func (u *ui) clientFormError(w http.ResponseWriter, r *http.Request, user app.User, p pages.ClientFormProps, err error) {
	var appErr *app.Error
	if !errors.As(err, &appErr) || (appErr.Kind != app.KindInvalid && appErr.Kind != app.KindConflict) {
		u.fail(w, r, err)
		return
	}
	p.FieldErrors = map[string]string{}
	for k, msg := range appErr.Details {
		if k == "name" {
			p.FieldErrors["name"] = msg
		} else {
			p.Error = detailsText(appErr)
		}
	}
	switch {
	case appErr.Field != "":
		p.FieldErrors[appErr.Field] = appErr.Message
	case len(appErr.Details) == 0:
		p.Error = appErr.Message
	}
	toast(w, toastError, "The client was not saved", appErr.Message)
	u.clientForm(w, r, user, http.StatusUnprocessableEntity, p)
}

// clientsChanged answers a change made from the table or a modal: the table as stored
// (HX-Retarget), a toast, and, if closeIt, the modal closed. A browser without JavaScript
// goes back to /.
func (u *ui) clientsChanged(w http.ResponseWriter, r *http.Request, status int, typ, title, message string, closeIt bool) {
	if !isHTMX(r) {
		redirect(w, r, "/")
		return
	}
	list, err := u.app.ListClients(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	w.Header().Set("HX-Retarget", "#"+pages.ClientsTableID)
	w.Header().Set("HX-Reswap", "outerHTML")
	toast(w, typ, title, message)
	if closeIt {
		closeModal(w)
	}
	u.render(w, r, status, pages.ClientsTable(list))
}

// pathClient loads the client of the {id} path value, or answers 404.
func (u *ui) pathClient(w http.ResponseWriter, r *http.Request) (app.Client, bool) {
	id, ok := u.pathID(w, r, "id")
	if !ok {
		return app.Client{}, false
	}
	c, err := u.app.GetClient(r.Context(), id)
	if err != nil {
		u.fail(w, r, err)
		return app.Client{}, false
	}
	return c, true
}

// pathID parses a numeric path value; anything else is answered as not found.
func (u *ui) pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := httpx.PathID(r, name)
	if err != nil {
		u.fail(w, r, app.NotFound("Not found"))
		return 0, false
	}
	return id, true
}

// parseForm reads a small form body; a bad one is a 400.
func parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return false
	}
	return true
}

// detailsText is a validation error's details as one sentence for a toast.
func detailsText(e *app.Error) string {
	if len(e.Details) == 0 {
		return e.Message
	}
	msgs := make([]string, 0, len(e.Details))
	for _, m := range e.Details {
		msgs = append(msgs, m)
	}
	sort.Strings(msgs)
	s := strings.Join(msgs, "; ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// conditionBuilder is the condition builder (C7, spec §10.5), with the stored condition.
func (u *ui) conditionBuilder(w http.ResponseWriter, r *http.Request, user app.User) {
	c, ok := u.pathClient(w, r)
	if !ok {
		return
	}
	u.renderBuilder(w, r, user, http.StatusOK, pages.ConditionBuilderProps{
		ClientID: c.ID, ClientName: c.Name, Tree: builder.FromCondition(c.Condition),
	})
}

// editCondition applies one edit of the builder to the posted tree and answers with the
// builder; nothing is saved. A form that is not a tree, or an edit that does not apply,
// is a 400: the builder never sends one.
func (u *ui) editCondition(w http.ResponseWriter, r *http.Request, user app.User) {
	c, ok := u.pathClient(w, r)
	if !ok || !parseForm(w, r) {
		return
	}
	tree, err := builder.ParseForm(r.PostForm)
	if err == nil {
		var e builder.Edit
		if e, err = builder.ParseEdit(r.PostForm.Get(pages.EditParam)); err == nil {
			err = builder.Apply(&tree, e)
		}
	}
	if err != nil {
		u.fail(w, r, app.BadRequest("The condition could not be changed"))
		return
	}
	u.renderBuilder(w, r, user, http.StatusOK, pages.ConditionBuilderProps{ClientID: c.ID, ClientName: c.Name, Tree: tree, Unsaved: true})
}

// saveCondition is the builder's Save: the posted tree becomes the client's condition
// (an empty tree: none). Problems are shown next to the expressions they are about.
func (u *ui) saveCondition(w http.ResponseWriter, r *http.Request, user app.User) {
	c, ok := u.pathClient(w, r)
	if !ok || !parseForm(w, r) {
		return
	}
	tree, err := builder.ParseForm(r.PostForm)
	if err != nil {
		u.fail(w, r, app.BadRequest("The condition could not be read"))
		return
	}
	saved, err := u.app.UpdateClient(r.Context(), app.UserActor(user.ID), c.ID, app.ClientPatch{Condition: httpx.Some(tree.Condition())})
	var appErr *app.Error
	switch {
	case err == nil:
		u.clientsChanged(w, r, http.StatusOK, toastSuccess, `Condition of "`+saved.Name+`" saved`, "", true)
	case errors.As(err, &appErr) && appErr.Kind == app.KindInvalid:
		p := pages.ConditionBuilderProps{ClientID: c.ID, ClientName: c.Name, Tree: tree, Errors: map[string]string{}, Unsaved: true}
		for k, msg := range appErr.Details {
			switch {
			case k == "enabled": // msg is app.NeedsConditionMessage
				p.Error = "This client is enabled, and an enabled client needs a condition. Add an expression, or disable the client first."
			case k == "condition" || strings.HasPrefix(k, "condition."):
				path, field := builder.FieldOf(strings.TrimPrefix(strings.TrimPrefix(k, "condition"), "."))
				if field != "" {
					path += "." + field
				}
				p.Errors[path] = msg
			default:
				p.Error = detailsText(appErr)
			}
		}
		toast(w, toastError, "The condition was not saved", detailsText(appErr))
		u.renderBuilder(w, r, user, http.StatusUnprocessableEntity, p)
	default:
		u.fail(w, r, err)
	}
}

// renderBuilder answers with the builder: modal content for htmx, a page without.
func (u *ui) renderBuilder(w http.ResponseWriter, r *http.Request, user app.User, status int, p pages.ConditionBuilderProps) {
	if isHTMX(r) {
		u.render(w, r, status, pages.ConditionBuilder(p))
		return
	}
	u.render(w, r, status, pages.ConditionBuilderPage(user.Admin, p))
}
