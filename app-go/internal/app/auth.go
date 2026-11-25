package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Credentials is the body of a login (spec §6.4).
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginFailedMessage is the one message for an unknown email and a wrong password, so a
// login attempt cannot tell which it was (spec §9.1).
const LoginFailedMessage = "Invalid email or password"

// Login checks the credentials and returns the user. The caller then starts the session
// with internal/auth, which logs tmock.auth.login.succeeded once the session exists.
// clientAddr is only logged.
//
// An unknown email and a wrong password both return the same Unauthorized error, and
// both cost one password hash (spec §9.1). A hash with fewer iterations than the current
// setting is replaced after a successful login. A failed attempt is logged as
// tmock.auth.login.failed, never with the email or password.
func (a *App) Login(ctx context.Context, in Credentials, clientAddr string) (User, error) {
	var v httpx.Validator
	v.Required("email", strings.TrimSpace(in.Email) != "")
	v.Required("password", in.Password != "")
	if !v.OK() {
		return User{}, Invalid(v.Details())
	}

	u, err := a.users.GetByEmail(ctx, strings.TrimSpace(in.Email))
	if errors.Is(err, store.ErrNotFound) {
		a.hasher.Dummy(in.Password)
		return User{}, a.loginFailed(ctx, clientAddr)
	}
	if err != nil {
		return User{}, err
	}
	ok, rehash, err := a.hasher.Verify(in.Password, u.PasswordHash)
	if err != nil {
		return User{}, fmt.Errorf("checking the password of user %d: %w", u.ID, err)
	}
	if !ok {
		return User{}, a.loginFailed(ctx, clientAddr)
	}
	if rehash {
		a.rehash(ctx, u.ID, in.Password)
	}
	return UserFromStore(u), nil
}

func (a *App) loginFailed(ctx context.Context, clientAddr string) error {
	a.logger.LogAttrs(ctx, slog.LevelWarn, "login failed: wrong email or password",
		slog.String(logx.AttrEventName, logx.EventLoginFailed),
		slog.String(logx.AttrClientAddress, clientAddr),
	)
	return Unauthorized(LoginFailedMessage)
}

// rehash stores a hash with the current parameters. A failure is logged and otherwise
// ignored: the old hash still works, and the next login tries again.
func (a *App) rehash(ctx context.Context, userID int64, pw string) {
	hash, err := a.hasher.Hash(pw)
	if err == nil {
		err = a.users.SetPasswordHash(ctx, userID, hash)
	}
	if err != nil {
		a.logger.LogAttrs(ctx, slog.LevelWarn, "re-hashing the password failed: "+err.Error(),
			slog.Int64(logx.AttrUserID, userID))
	}
}
