package web

import (
	"errors"
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/views/pages"
)

// registerAccounts adds the accounts page (spec §10.3 A1–A4, admins only) and My Account
// (M1, every user).
func (u *ui) registerAccounts(mux *http.ServeMux) {
	mux.Handle("GET /accounts", u.guarded(true, u.accountsPage))
	mux.Handle("GET /ui/accounts/new", u.guarded(true, u.newAccountForm))
	mux.Handle("POST /ui/accounts", u.guarded(true, u.createAccount))
	mux.Handle("GET /ui/accounts/{id}/edit", u.guarded(true, u.editAccountForm))
	mux.Handle("POST /ui/accounts/{id}", u.guarded(true, u.updateAccount))
	mux.Handle("DELETE /ui/accounts/{id}", u.guarded(true, u.deleteAccount))
	mux.Handle("GET /my-account", u.guarded(false, u.myAccountPage))
	mux.Handle("POST /my-account", u.guarded(false, u.saveMyAccount))
}

func (u *ui) accountsPage(w http.ResponseWriter, r *http.Request, _ app.User) {
	list, err := u.app.ListUsers(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.render(w, r, http.StatusOK, pages.Accounts(list))
}

// newAccountForm is the add modal (A2).
func (u *ui) newAccountForm(w http.ResponseWriter, r *http.Request, _ app.User) {
	u.accountForm(w, r, http.StatusOK, pages.AccountFormProps{})
}

// editAccountForm is the edit modal (A3).
func (u *ui) editAccountForm(w http.ResponseWriter, r *http.Request, _ app.User) {
	id, ok := u.pathID(w, r, "id")
	if !ok {
		return
	}
	us, err := u.app.GetUser(r.Context(), id)
	if err != nil {
		u.fail(w, r, err)
		return
	}
	u.accountForm(w, r, http.StatusOK, pages.AccountFormProps{ID: us.ID, Name: us.Name, Email: us.Email, Admin: us.Admin, Original: us})
}

func (u *ui) createAccount(w http.ResponseWriter, r *http.Request, user app.User) {
	if !parseForm(w, r) {
		return
	}
	f := r.PostForm
	in := app.UserInput{Name: f.Get("name"), Email: f.Get("email"), Password: f.Get("password"), Admin: f.Get("admin") != ""}
	created, err := u.app.CreateUser(r.Context(), app.UserActor(user.ID), in)
	if err != nil {
		u.accountFormError(w, r, pages.AccountFormProps{Name: in.Name, Email: in.Email, Admin: in.Admin}, err)
		return
	}
	u.accountsChanged(w, r, `User "`+created.Name+`" created`)
}

// updateAccount saves the edit modal. Only the fields that differ from the values the
// modal was opened with are sent (A3), so a field that someone else changed meanwhile
// is not set back; a blank password keeps the current one.
func (u *ui) updateAccount(w http.ResponseWriter, r *http.Request, user app.User) {
	id, ok := u.pathID(w, r, "id")
	if !ok || !parseForm(w, r) {
		return
	}
	f := r.PostForm
	p := pages.AccountFormProps{ID: id, Name: f.Get("name"), Email: f.Get("email"), Admin: f.Get("admin") != "",
		Original: app.User{ID: id, Name: f.Get(pages.OrigName), Email: f.Get(pages.OrigEmail), Admin: f.Get(pages.OrigAdmin) != ""}}
	var patch app.UserPatch
	if p.Name != p.Original.Name {
		patch.Name = httpx.Some(p.Name)
	}
	if p.Email != p.Original.Email {
		patch.Email = httpx.Some(p.Email)
	}
	if p.Admin != p.Original.Admin {
		patch.Admin = httpx.Some(p.Admin)
	}
	if pw := f.Get("password"); pw != "" {
		patch.Password = httpx.Some(pw)
	}
	saved, err := u.app.UpdateUser(r.Context(), app.UserActor(user.ID), id, patch)
	if err != nil {
		u.accountFormError(w, r, p, err)
		return
	}
	u.accountsChanged(w, r, `User "`+saved.Name+`" updated`)
}

// deleteAccount is Delete (A4; it asks first). Refusals (your own account, the last
// admin) are toasts.
func (u *ui) deleteAccount(w http.ResponseWriter, r *http.Request, user app.User) {
	id, ok := u.pathID(w, r, "id")
	if !ok {
		return
	}
	deleted, err := u.app.DeleteUser(r.Context(), app.UserActor(user.ID), id)
	var appErr *app.Error
	switch {
	case err == nil:
		u.accountsChanged(w, r, `User "`+deleted.Name+`" deleted`)
	case errors.As(err, &appErr) && appErr.Kind == app.KindInvalid && isHTMX(r):
		w.Header().Set("HX-Reswap", "none")
		toast(w, toastError, "The user was not deleted", appErr.Message)
		w.WriteHeader(http.StatusUnprocessableEntity)
	default:
		u.fail(w, r, err)
	}
}

func (u *ui) accountForm(w http.ResponseWriter, r *http.Request, status int, p pages.AccountFormProps) {
	if isHTMX(r) {
		u.render(w, r, status, pages.AccountForm(p))
		return
	}
	u.render(w, r, status, pages.AccountFormPage(p))
}

// accountFormError shows a failed save as 422 with the form re-rendered (spec §10.2).
func (u *ui) accountFormError(w http.ResponseWriter, r *http.Request, p pages.AccountFormProps, err error) {
	var appErr *app.Error
	if !errors.As(err, &appErr) || (appErr.Kind != app.KindInvalid && appErr.Kind != app.KindConflict) {
		u.fail(w, r, err)
		return
	}
	p.FieldErrors = map[string]string{}
	for k, msg := range appErr.Details {
		p.FieldErrors[k] = msg
	}
	switch {
	case appErr.Field != "":
		p.FieldErrors[appErr.Field] = appErr.Message
	case len(appErr.Details) == 0:
		p.Error = appErr.Message
	}
	toast(w, toastError, "The user was not saved", detailsText(appErr))
	u.accountForm(w, r, http.StatusUnprocessableEntity, p)
}

// accountsChanged answers a save or delete: the table (HX-Retarget), a toast, the modal
// closed; without JavaScript, back to /accounts.
func (u *ui) accountsChanged(w http.ResponseWriter, r *http.Request, message string) {
	if !isHTMX(r) {
		redirect(w, r, "/accounts")
		return
	}
	list, err := u.app.ListUsers(r.Context())
	if err != nil {
		u.fail(w, r, err)
		return
	}
	w.Header().Set("HX-Retarget", "#"+pages.AccountsTableID)
	w.Header().Set("HX-Reswap", "outerHTML")
	toast(w, toastSuccess, message, "")
	closeModal(w)
	u.render(w, r, http.StatusOK, pages.AccountsTable(list))
}

func (u *ui) myAccountPage(w http.ResponseWriter, r *http.Request, user app.User) {
	u.render(w, r, http.StatusOK, pages.MyAccount(pages.MyAccountProps{Admin: user.Admin, Name: user.Name, Email: user.Email}))
}

// saveMyAccount saves My Account (M1): the name and email if they changed and, with the
// current password, a new password. The form comes back with the password fields empty.
func (u *ui) saveMyAccount(w http.ResponseWriter, r *http.Request, user app.User) {
	if !parseForm(w, r) {
		return
	}
	f := r.PostForm
	p := pages.MyAccountProps{Admin: user.Admin, Name: f.Get("name"), Email: f.Get("email")}
	var patch app.ProfilePatch
	if p.Name != user.Name {
		patch.Name = httpx.Some(p.Name)
	}
	if p.Email != user.Email {
		patch.Email = httpx.Some(p.Email)
	}
	if pw := f.Get("newPassword"); pw != "" || f.Get("confirmPassword") != "" {
		if pw != f.Get("confirmPassword") {
			u.myAccountError(w, r, p, app.InvalidField("confirmPassword", "does not match the new password"))
			return
		}
		patch.NewPassword = httpx.Some(pw)
		patch.CurrentPassword = httpx.Some(f.Get("currentPassword"))
	}
	saved, err := u.app.UpdateProfile(r.Context(), user.ID, patch)
	if err != nil {
		u.myAccountError(w, r, p, err)
		return
	}
	if !isHTMX(r) {
		redirect(w, r, "/my-account")
		return
	}
	toast(w, toastSuccess, "Your account was saved", "")
	u.render(w, r, http.StatusOK, pages.MyAccountForm(pages.MyAccountProps{Admin: saved.Admin, Name: saved.Name, Email: saved.Email}))
}

func (u *ui) myAccountError(w http.ResponseWriter, r *http.Request, p pages.MyAccountProps, err error) {
	var appErr *app.Error
	if !errors.As(err, &appErr) || (appErr.Kind != app.KindInvalid && appErr.Kind != app.KindConflict) {
		u.fail(w, r, err)
		return
	}
	p.FieldErrors = map[string]string{}
	for k, msg := range appErr.Details {
		p.FieldErrors[k] = msg
	}
	switch {
	case appErr.Field != "":
		p.FieldErrors[appErr.Field] = appErr.Message
	case len(appErr.Details) == 0:
		p.Error = appErr.Message
	}
	toast(w, toastError, "Your account was not saved", detailsText(appErr))
	if isHTMX(r) {
		u.render(w, r, http.StatusUnprocessableEntity, pages.MyAccountForm(p))
		return
	}
	u.render(w, r, http.StatusUnprocessableEntity, pages.MyAccount(p))
}
