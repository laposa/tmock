package web

import (
	"errors"
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/clientip"
	"github.com/laposa/tmock/app-go/internal/views/pages"
)

// maxFormBytes limits form bodies; the UI's forms are small.
const maxFormBytes = 1 << 20

// guarded runs page for a logged-in user (spec §10.3 Layout): anonymous users are sent to
// /login and, when admin is set, users who are not admins to /.
func (u *ui) guarded(admin bool, page func(http.ResponseWriter, *http.Request, app.User)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := u.sessions.User(r)
		switch {
		case errors.Is(err, auth.ErrNoSession):
			redirect(w, r, "/login")
			return
		case err != nil:
			u.fail(w, r, err)
			return
		case admin && !user.Admin:
			redirect(w, r, "/")
			return
		}
		page(w, r.WithContext(auth.WithUser(r.Context(), user)), user)
	})
}

// loginPage is GET /login; a logged-in user goes straight to /.
func (u *ui) loginPage(w http.ResponseWriter, r *http.Request) {
	_, err := u.sessions.User(r)
	switch {
	case err == nil:
		redirect(w, r, "/")
	case errors.Is(err, auth.ErrNoSession):
		u.render(w, r, http.StatusOK, pages.Login(pages.LoginProps{}))
	default:
		u.fail(w, r, err)
	}
}

// login is POST /login. A failed attempt re-renders the form with the message, as 422
// (spec §10.1): htmx swaps just the form, a plain submit gets the whole page.
func (u *ui) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	in := app.Credentials{Email: r.PostForm.Get("email"), Password: r.PostForm.Get("password")}
	addr := clientip.From(r)
	user, err := u.app.Login(r.Context(), in, addr)
	if err == nil {
		err = u.sessions.Start(r.Context(), w, user.ID, addr)
	}
	var appErr *app.Error
	switch {
	case err == nil:
		redirect(w, r, "/")
	case errors.As(err, &appErr):
		p := pages.LoginProps{Email: in.Email, FieldErrors: appErr.Details}
		if appErr.Kind != app.KindInvalid {
			p.Error = appErr.Message
		}
		if isHTMX(r) {
			u.render(w, r, http.StatusUnprocessableEntity, pages.LoginForm(p))
		} else {
			u.render(w, r, http.StatusUnprocessableEntity, pages.Login(p))
		}
	default:
		u.fail(w, r, err)
	}
}

// logout is POST /logout.
func (u *ui) logout(w http.ResponseWriter, r *http.Request) {
	if err := u.sessions.End(w, r); err != nil {
		u.fail(w, r, err)
		return
	}
	redirect(w, r, "/login")
}
