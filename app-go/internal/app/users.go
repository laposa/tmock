package app

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/store"
)

// UserInput is the body of a create (spec §6.4).
type UserInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Admin    bool   `json:"admin"`
}

// UserPatch is the body of an admin's update of a user (spec §6.4). No field can be null.
type UserPatch struct {
	Name     httpx.Optional[string] `json:"name"`
	Email    httpx.Optional[string] `json:"email"`
	Password httpx.Optional[string] `json:"password"`
	Admin    httpx.Optional[bool]   `json:"admin"`
}

// ProfilePatch is the body of a user's update of their own account (spec §6.4). A new
// password needs the current one.
type ProfilePatch struct {
	Name            httpx.Optional[string] `json:"name"`
	Email           httpx.Optional[string] `json:"email"`
	CurrentPassword httpx.Optional[string] `json:"currentPassword"`
	NewPassword     httpx.Optional[string] `json:"newPassword"`
}

// Rules of spec §6.4.
const (
	maxUserName    = 255
	maxEmail       = 254 // RFC 5321
	MinPassword    = 8
	maxPassword    = 1024 // a bound on the work one login or save costs
	ownAccountMsg  = "You cannot delete your own account"
	lastAdminMsg   = "The last admin cannot be deleted"
	stayAdminMsg   = "the last admin must stay an admin"
	needCurrentMsg = "is required to set a new password"
	wrongCurrent   = "is not correct"
)

// ListUsers returns every user, ordered by id (spec §5.3). The list is never nil.
func (a *App) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := a.users.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(rows))
	for _, u := range rows {
		out = append(out, UserFromStore(u))
	}
	return out, nil
}

// GetUser returns the user with id, or NotFound.
func (a *App) GetUser(ctx context.Context, id int64) (User, error) {
	u, err := a.users.Get(ctx, id)
	if err != nil {
		return User{}, StoreError(err, userNotFound(id), nil)
	}
	return UserFromStore(u), nil
}

// CreateUser validates in and creates the user. A taken email (ignoring case) is a
// Conflict.
func (a *App) CreateUser(ctx context.Context, actor Actor, in UserInput) (User, error) {
	in.Name, in.Email = strings.TrimSpace(in.Name), strings.TrimSpace(in.Email)
	var v httpx.Validator
	checkName(&v, in.Name)
	checkEmail(&v, in.Email)
	checkPassword(&v, "password", in.Password)
	if !v.OK() {
		return User{}, Invalid(v.Details())
	}
	hash, err := a.hasher.Hash(in.Password)
	if err != nil {
		return User{}, err
	}
	u, err := a.users.Create(ctx, store.NewUser{Name: in.Name, Email: in.Email, PasswordHash: hash, Admin: in.Admin})
	if err != nil {
		return User{}, StoreError(err, nil, emailTaken(in.Email))
	}
	a.entityChanged(ctx, actor, logx.EntityUser, u.ID, logx.ActionCreate)
	return UserFromStore(u), nil
}

// UpdateUser is an admin's change of the user with id: the fields present in p. The last
// admin cannot lose the admin flag. A change to nothing new is not saved or logged.
func (a *App) UpdateUser(ctx context.Context, actor Actor, id int64, p UserPatch) (User, error) {
	var v httpx.Validator
	v.Check(!p.Name.Null, "name", "is required")
	v.Check(!p.Email.Null, "email", "is required")
	v.Check(!p.Password.Null, "password", "must be a string")
	v.Check(!p.Admin.Null, "admin", "must be true or false")
	name, email := trimmed(p.Name), trimmed(p.Email)
	if s, ok := name.Get(); ok {
		checkName(&v, s)
	}
	if s, ok := email.Get(); ok {
		checkEmail(&v, s)
	}
	pw, setPW := p.Password.Get()
	if setPW {
		checkPassword(&v, "password", pw)
	}
	if !v.OK() {
		return User{}, Invalid(v.Details())
	}
	var hash string
	if setPW {
		var err error
		if hash, err = a.hasher.Hash(pw); err != nil {
			return User{}, err
		}
	}
	var newEmail string
	u, changed, err := a.users.Modify(ctx, id, func(cur store.User, admins int) (store.UserFields, error) {
		f := store.UserFields{Name: cur.Name, Email: cur.Email, PasswordHash: hash, Admin: cur.Admin}
		if s, ok := name.Get(); ok {
			f.Name = s
		}
		if s, ok := email.Get(); ok {
			f.Email = s
		}
		if b, ok := p.Admin.Get(); ok {
			f.Admin = b
		}
		if cur.Admin && !f.Admin && admins <= 1 {
			return store.UserFields{}, InvalidField("admin", stayAdminMsg)
		}
		newEmail = f.Email
		return f, nil
	})
	if err != nil {
		return User{}, StoreError(err, userNotFound(id), emailTaken(newEmail))
	}
	if changed {
		a.entityChanged(ctx, actor, logx.EntityUser, u.ID, logx.ActionUpdate)
	}
	return UserFromStore(u), nil
}

// DeleteUser deletes the user with id, with their sessions. Admins cannot delete their own
// account, and the last admin cannot be deleted (spec §6.4).
func (a *App) DeleteUser(ctx context.Context, actor Actor, id int64) (User, error) {
	if actor.UserID == id {
		return User{}, BadRequest(ownAccountMsg)
	}
	u, err := a.users.Delete(ctx, id, func(cur store.User, admins int) error {
		if cur.Admin && admins <= 1 {
			return BadRequest(lastAdminMsg)
		}
		return nil
	})
	if err != nil {
		return User{}, StoreError(err, userNotFound(id), nil)
	}
	a.entityChanged(ctx, actor, logx.EntityUser, u.ID, logx.ActionDelete)
	return UserFromStore(u), nil
}

// UpdateProfile is a user's change of their own account (spec §6.4, §10.3 M1): the fields
// present in p. A new password needs the correct current password. A change to nothing
// new is not saved or logged.
func (a *App) UpdateProfile(ctx context.Context, userID int64, p ProfilePatch) (User, error) {
	var v httpx.Validator
	v.Check(!p.Name.Null, "name", "is required")
	v.Check(!p.Email.Null, "email", "is required")
	v.Check(!p.CurrentPassword.Null, "currentPassword", "must be a string")
	v.Check(!p.NewPassword.Null, "newPassword", "must be a string")
	name, email := trimmed(p.Name), trimmed(p.Email)
	if s, ok := name.Get(); ok {
		checkName(&v, s)
	}
	if s, ok := email.Get(); ok {
		checkEmail(&v, s)
	}
	newPW, setPW := p.NewPassword.Get()
	current, _ := p.CurrentPassword.Get()
	if setPW {
		checkPassword(&v, "newPassword", newPW)
		v.Check(current != "", "currentPassword", needCurrentMsg)
	}
	if !v.OK() {
		return User{}, Invalid(v.Details())
	}

	// The (slow) password work happens before the lock. The hash that was checked must
	// still be the stored one when the change is saved.
	var checked, hash string
	if setPW {
		cur, err := a.users.Get(ctx, userID)
		if err != nil {
			return User{}, StoreError(err, userNotFound(userID), nil)
		}
		ok, _, err := a.hasher.Verify(current, cur.PasswordHash)
		if err != nil {
			return User{}, fmt.Errorf("checking the password of user %d: %w", userID, err)
		}
		if !ok {
			return User{}, InvalidField("currentPassword", wrongCurrent)
		}
		checked = cur.PasswordHash
		if hash, err = a.hasher.Hash(newPW); err != nil {
			return User{}, err
		}
	}
	var newEmail string
	u, changed, err := a.users.Modify(ctx, userID, func(cur store.User, _ int) (store.UserFields, error) {
		if setPW && cur.PasswordHash != checked {
			return store.UserFields{}, InvalidField("currentPassword", wrongCurrent)
		}
		f := store.UserFields{Name: cur.Name, Email: cur.Email, PasswordHash: hash, Admin: cur.Admin}
		if s, ok := name.Get(); ok {
			f.Name = s
		}
		if s, ok := email.Get(); ok {
			f.Email = s
		}
		newEmail = f.Email
		return f, nil
	})
	if err != nil {
		return User{}, StoreError(err, userNotFound(userID), emailTaken(newEmail))
	}
	if changed {
		a.entityChanged(ctx, UserActor(userID), logx.EntityUser, u.ID, logx.ActionUpdate)
	}
	return UserFromStore(u), nil
}

func checkName(v *httpx.Validator, s string) {
	if v.Required("name", s != "") {
		v.Length("name", s, 1, maxUserName)
	}
}

// checkEmail accepts a bare address ("a@example.com"), as net/mail parses it, of at most
// 254 characters.
func checkEmail(v *httpx.Validator, s string) {
	if !v.Required("email", s != "") {
		return
	}
	addr, err := mail.ParseAddress(s)
	ok := err == nil && addr.Name == "" && addr.Address == s && utf8.RuneCountInString(s) <= maxEmail
	v.Check(ok, "email", "must be an email address")
}

func checkPassword(v *httpx.Validator, field, pw string) {
	n := utf8.RuneCountInString(pw)
	if v.Check(n >= MinPassword, field, fmt.Sprintf("must be at least %d characters", MinPassword)) {
		v.Check(n <= maxPassword, field, fmt.Sprintf("must be at most %d characters", maxPassword))
	}
}

// trimmed is o with its value trimmed of white space.
func trimmed(o httpx.Optional[string]) httpx.Optional[string] {
	o.Value = strings.TrimSpace(o.Value)
	return o
}

func emailTaken(email string) *Error {
	e := Conflict("A user with email %q already exists", email)
	e.Field = "email"
	return e
}

func userNotFound(id int64) *Error { return NotFound("User %d not found", id) }
