package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/views/pages"
)

// registerScenarios adds the scenarios page (spec §10.3 SC1–SC5) and its fragments.
func (u *ui) registerScenarios(mux *http.ServeMux) {
	mux.Handle("GET /scenarios", u.guarded(false, u.scenariosPage))
	mux.Handle("GET /ui/scenarios/new", u.guarded(false, u.newScenarioForm))
	mux.Handle("POST /ui/scenarios", u.guarded(false, u.createScenario))
	mux.Handle("GET /ui/scenarios/{id}/edit", u.guarded(false, u.editScenarioForm))
	mux.Handle("POST /ui/scenarios/{id}", u.guarded(false, u.updateScenario))
	mux.Handle("DELETE /ui/scenarios/{id}", u.guarded(false, u.deleteScenario))
}

func (u *ui) scenariosPage(w http.ResponseWriter, r *http.Request, user app.User) {
	list, services, err := u.scenariosAndServices(r)
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.render(w, r, http.StatusOK, pages.Scenarios(pages.ScenariosProps{Admin: user.Admin, Scenarios: list, Services: services}))
}

// newScenarioForm is the editor with empty fields (SC2).
func (u *ui) newScenarioForm(w http.ResponseWriter, r *http.Request, user app.User) {
	u.scenarioForm(w, r, user, http.StatusOK, pages.ScenarioFormProps{})
}

// editScenarioForm is the editor of a stored scenario. ?focus=headers or body opens it
// scrolled there ("Show Headers", "Show Body", SC1).
func (u *ui) editScenarioForm(w http.ResponseWriter, r *http.Request, user app.User) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		u.fail(w, r, app.NotFound("Scenario not found"))
		return
	}
	s, err := u.app.GetScenario(r.Context(), id)
	if err != nil {
		u.fail(w, r, err)
		return
	}
	p := pages.ScenarioFormProps{ID: s.ID, Values: pages.ScenarioValues(s)}
	if f := r.URL.Query().Get("focus"); f == pages.FocusHeaders || f == pages.FocusBody {
		p.Focus = f
	}
	u.scenarioForm(w, r, user, http.StatusOK, p)
}

func (u *ui) createScenario(w http.ResponseWriter, r *http.Request, user app.User) {
	values, ok := u.scenarioValues(w, r)
	if !ok {
		return
	}
	p := pages.ScenarioFormProps{Values: values, Unsaved: true}
	in, err := scenarioInput(values)
	if err == nil {
		var s app.Scenario
		if s, err = u.app.CreateScenario(r.Context(), app.UserActor(user.ID), in); err == nil {
			u.scenariosChanged(w, r, `Scenario "`+s.Name+`" created`)
			return
		}
	}
	u.scenarioFormError(w, r, user, p, err)
}

func (u *ui) updateScenario(w http.ResponseWriter, r *http.Request, user app.User) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		u.fail(w, r, app.NotFound("Scenario not found"))
		return
	}
	values, ok := u.scenarioValues(w, r)
	if !ok {
		return
	}
	p := pages.ScenarioFormProps{ID: id, Values: values, Unsaved: true}
	in, err := scenarioInput(values)
	if err == nil {
		var s app.Scenario
		if s, err = u.app.UpdateScenario(r.Context(), app.UserActor(user.ID), id, in.Replace()); err == nil {
			u.scenariosChanged(w, r, `Scenario "`+s.Name+`" updated`)
			return
		}
	}
	u.scenarioFormError(w, r, user, p, err)
}

func (u *ui) deleteScenario(w http.ResponseWriter, r *http.Request, user app.User) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		u.fail(w, r, app.NotFound("Scenario not found"))
		return
	}
	s, err := u.app.DeleteScenario(r.Context(), app.UserActor(user.ID), id)
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.scenariosChanged(w, r, `Scenario "`+s.Name+`" deleted`)
}

// scenarioValues reads the editor form. Bodies can be large, so the limit is the REST
// API's, not the other forms'.
func (u *ui) scenarioValues(w http.ResponseWriter, r *http.Request) (pages.ScenarioFormValues, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, httpx.MaxBodyBytes)
	if err := r.ParseForm(); err != nil {
		status, msg := http.StatusBadRequest, "Bad request"
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status, msg = http.StatusRequestEntityTooLarge, fmt.Sprintf("The scenario is larger than %d MB", httpx.MaxBodyBytes>>20)
		}
		if isHTMX(r) {
			w.Header().Set("HX-Reswap", "none")
			toast(w, toastError, "The scenario was not saved", msg)
		}
		http.Error(w, msg, status)
		return pages.ScenarioFormValues{}, false
	}
	f := r.PostForm
	on := func(name string) bool { return f.Get(name+"Enabled") != "" }
	v := pages.ScenarioFormValues{
		Name: f.Get("name"), Service: f.Get("service"),
		MethodOn: on("requestMethod"), Method: f.Get("requestMethod"),
		PathOn: on("requestPath"), Path: f.Get("requestPath"),
		ConditionOn: on("requestCondition"), Condition: f.Get("requestCondition"),
		CodeOn: on("responseCode"), Code: f.Get("responseCode"),
		HeadersOn: on("responseHeaders"),
		BodyOn:    on("responseBody"),
		// Browsers send a textarea's line breaks as CRLF.
		Body:      strings.ReplaceAll(f.Get("responseBody"), "\r\n", "\n"),
		SkipProxy: f.Get("skipProxy") != "",
	}
	names, vals := f["headerName"], f["headerValue"]
	for i, n := range names {
		row := pages.HeaderRow{Name: n}
		if i < len(vals) {
			row.Value = vals[i]
		}
		if strings.TrimSpace(row.Name) == "" && strings.TrimSpace(row.Value) == "" {
			continue // an empty row, such as the one shown when there are none
		}
		v.Headers = append(v.Headers, row)
	}
	return v, true
}

// scenarioInput turns the editor's fields into a use-case input (SC3): a field that is
// switched off is unset, and Skip Proxy counts only while Body is on. An empty header
// value is a header the response drops (null). The problems only the form can have (a
// code that is not a number, a header twice) are returned as a validation error.
func scenarioInput(v pages.ScenarioFormValues) (app.ScenarioInput, error) {
	in := app.ScenarioInput{Service: v.Service, Name: v.Name, SkipProxy: v.BodyOn && v.SkipProxy}
	set := func(on bool, s string) *string {
		if !on {
			return nil
		}
		return &s
	}
	in.RequestMethod = set(v.MethodOn, v.Method)
	in.RequestPath = set(v.PathOn, v.Path)
	in.RequestCondition = set(v.ConditionOn, v.Condition)
	in.ResponseBody = set(v.BodyOn, v.Body)

	var val httpx.Validator
	if c := strings.TrimSpace(v.Code); v.CodeOn && c != "" {
		n, err := strconv.Atoi(c)
		if val.Check(err == nil, "responseCode", "must be a whole number") {
			in.ResponseCode = &n
		}
	}
	if v.HeadersOn {
		in.ResponseHeaders = map[string]*string{}
		for _, r := range v.Headers {
			name := strings.TrimSpace(r.Name)
			if _, dup := in.ResponseHeaders[name]; dup {
				val.Add("responseHeaders", fmt.Sprintf("%q appears twice", name))
			}
			var value *string
			if r.Value != "" {
				value = &r.Value
			}
			in.ResponseHeaders[name] = value
		}
	}
	if !val.OK() {
		return in, app.Invalid(val.Details())
	}
	return in, nil
}

// scenarioForm answers with the editor: the modal content for htmx, a page of its own
// for a browser without JavaScript. It fills in the services for the Service select.
func (u *ui) scenarioForm(w http.ResponseWriter, r *http.Request, user app.User, status int, p pages.ScenarioFormProps) {
	services, err := u.app.ListServices(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	p.Services = services
	if isHTMX(r) {
		u.render(w, r, status, pages.ScenarioForm(p))
		return
	}
	u.render(w, r, status, pages.ScenarioFormPage(user.Admin, p))
}

// scenarioFormError shows a failed save (spec §10.2): validation and conflict errors as
// 422 with the editor re-rendered and an error toast. An error about one field (a taken
// name, an unknown service) is shown next to that field. Anything else goes through
// fail.
func (u *ui) scenarioFormError(w http.ResponseWriter, r *http.Request, user app.User, p pages.ScenarioFormProps, err error) {
	var appErr *app.Error
	if !errors.As(err, &appErr) {
		u.fail(w, r, err)
		return
	}
	switch {
	case appErr.Field != "":
		p.FieldErrors = map[string]string{appErr.Field: appErr.Message}
	case appErr.Kind == app.KindInvalid:
		p.FieldErrors = appErr.Details
		if len(appErr.Details) == 0 {
			p.Error = appErr.Message
		}
	case appErr.Kind == app.KindConflict:
		p.Error = appErr.Message
	default:
		u.fail(w, r, err) // for example the scenario was deleted meanwhile
		return
	}
	toast(w, toastError, "The scenario was not saved", appErr.Message)
	u.scenarioForm(w, r, user, http.StatusUnprocessableEntity, p)
}

// scenariosChanged answers a successful save or delete. htmx gets the refreshed list in
// place of the old one (HX-Retarget), a success toast, and the modal closed; a browser
// without JavaScript goes back to the scenarios page.
func (u *ui) scenariosChanged(w http.ResponseWriter, r *http.Request, message string) {
	if !isHTMX(r) {
		redirect(w, r, "/scenarios")
		return
	}
	list, services, err := u.scenariosAndServices(r)
	if err != nil {
		u.fail(w, r, err)
		return
	}
	w.Header().Set("HX-Retarget", "#"+pages.ScenariosListID)
	w.Header().Set("HX-Reswap", "outerHTML")
	toast(w, toastSuccess, message, "")
	closeModal(w)
	u.render(w, r, http.StatusOK, pages.ScenariosList(list, services))
}

func (u *ui) scenariosAndServices(r *http.Request) ([]app.Scenario, []app.Service, error) {
	list, err := u.app.ListScenarios(r.Context(), "")
	if err != nil {
		return nil, nil, err
	}
	services, err := u.app.ListServices(r.Context())
	return list, services, err
}
