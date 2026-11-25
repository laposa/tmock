package web

import (
	"errors"
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/views/pages"
)

// registerServices adds the services page (spec §10.3 S1–S3) and its fragments.
func (u *ui) registerServices(mux *http.ServeMux) {
	mux.Handle("GET /services", u.guarded(false, u.servicesPage))
	mux.Handle("GET /ui/services/new", u.guarded(false, u.newServiceForm))
	mux.Handle("POST /ui/services", u.guarded(false, u.createService))
	mux.Handle("GET /ui/services/{path}/edit", u.guarded(false, u.editServiceForm))
	mux.Handle("POST /ui/services/{path}", u.guarded(false, u.updateService))
	mux.Handle("DELETE /ui/services/{path}", u.guarded(false, u.deleteService))
}

func (u *ui) servicesPage(w http.ResponseWriter, r *http.Request, user app.User) {
	list, err := u.app.ListServices(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.render(w, r, http.StatusOK, pages.Services(pages.ServicesProps{Admin: user.Admin, Services: list}))
}

// newServiceForm is the add modal (S2).
func (u *ui) newServiceForm(w http.ResponseWriter, r *http.Request, user app.User) {
	u.serviceForm(w, r, user, http.StatusOK, pages.ServiceFormProps{})
}

// editServiceForm is the edit modal (S3).
func (u *ui) editServiceForm(w http.ResponseWriter, r *http.Request, user app.User) {
	s, err := u.app.GetService(r.Context(), r.PathValue("path"))
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.serviceForm(w, r, user, http.StatusOK, pages.ServiceFormProps{
		Path:   s.Path,
		Values: app.ServiceInput{Path: s.Path, Name: s.Name, UpstreamURL: s.UpstreamURL},
	})
}

func (u *ui) createService(w http.ResponseWriter, r *http.Request, user app.User) {
	in, ok := u.serviceInput(w, r)
	if !ok {
		return
	}
	s, err := u.app.CreateService(r.Context(), app.UserActor(user.ID), in)
	if err != nil {
		u.serviceFormError(w, r, user, pages.ServiceFormProps{Values: in}, err)
		return
	}
	u.servicesChanged(w, r, `Service "`+s.Name+`" created`)
}

func (u *ui) updateService(w http.ResponseWriter, r *http.Request, user app.User) {
	in, ok := u.serviceInput(w, r)
	if !ok {
		return
	}
	path := r.PathValue("path")
	s, err := u.app.UpdateService(r.Context(), app.UserActor(user.ID), path, app.ServicePatch{
		Path:        httpx.Some(in.Path),
		Name:        httpx.Some(in.Name),
		UpstreamURL: httpx.Some(in.UpstreamURL),
	})
	if err != nil {
		u.serviceFormError(w, r, user, pages.ServiceFormProps{Path: path, Values: in}, err)
		return
	}
	u.servicesChanged(w, r, `Service "`+s.Name+`" updated`)
}

func (u *ui) deleteService(w http.ResponseWriter, r *http.Request, user app.User) {
	s, err := u.app.DeleteService(r.Context(), app.UserActor(user.ID), r.PathValue("path"))
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.servicesChanged(w, r, `Service "`+s.Name+`" deleted`)
}

// serviceInput reads the add or edit form.
func (u *ui) serviceInput(w http.ResponseWriter, r *http.Request) (app.ServiceInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return app.ServiceInput{}, false
	}
	f := r.PostForm
	return app.ServiceInput{Path: f.Get("path"), Name: f.Get("name"), UpstreamURL: f.Get("upstreamUrl")}, true
}

// serviceForm answers with a service form: the modal content for htmx, a page of its own
// for a browser without JavaScript.
func (u *ui) serviceForm(w http.ResponseWriter, r *http.Request, user app.User, status int, p pages.ServiceFormProps) {
	if isHTMX(r) {
		u.render(w, r, status, pages.ServiceForm(p))
		return
	}
	u.render(w, r, status, pages.ServiceFormPage(user.Admin, p))
}

// serviceFormError shows a failed save (spec §10.2): validation and conflict errors as
// 422 with the form re-rendered and an error toast; anything else through fail.
func (u *ui) serviceFormError(w http.ResponseWriter, r *http.Request, user app.User, p pages.ServiceFormProps, err error) {
	var appErr *app.Error
	if !errors.As(err, &appErr) || (appErr.Kind != app.KindInvalid && appErr.Kind != app.KindConflict) {
		u.fail(w, r, err)
		return
	}
	p.FieldErrors = appErr.Details
	if appErr.Kind == app.KindConflict {
		p.Error = appErr.Message
	}
	toast(w, toastError, "The service was not saved", appErr.Message)
	u.serviceForm(w, r, user, http.StatusUnprocessableEntity, p)
}

// servicesChanged answers a successful save or delete. htmx gets the refreshed table in
// place of the old one (HX-Retarget), a success toast, and the modal closed; a browser
// without JavaScript goes back to the services page.
func (u *ui) servicesChanged(w http.ResponseWriter, r *http.Request, message string) {
	if !isHTMX(r) {
		redirect(w, r, "/services")
		return
	}
	list, err := u.app.ListServices(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	w.Header().Set("HX-Retarget", "#"+pages.ServicesTableID)
	w.Header().Set("HX-Reswap", "outerHTML")
	toast(w, toastSuccess, message, "")
	closeModal(w)
	u.render(w, r, http.StatusOK, pages.ServicesTable(list))
}
