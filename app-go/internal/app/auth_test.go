package app_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/password"
	"github.com/laposa/tmock/app-go/internal/store"
)

// fast is a hasher with the fewest iterations a stored hash may have, so tests stay quick.
var fast = password.Hasher{Iterations: 1000}

func newApp(t *testing.T) (*app.App, *store.Users, *logxtest.Recorder) {
	t.Helper()
	pool := dbtest.Migrated(t)
	logs, logger := logxtest.New()
	return app.New(pool, app.Options{Logger: logger, Hasher: fast}), store.NewUsers(pool), logs
}

func addUser(t *testing.T, users *store.Users, email, pw string, h password.Hasher) store.User {
	t.Helper()
	hash, err := h.Hash(pw)
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(context.Background(), store.NewUser{Name: "Ann", Email: email, PasswordHash: hash, Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestLogin(t *testing.T) {
	t.Parallel()
	a, users, logs := newApp(t)
	ann := addUser(t, users, "Ann@Example.com", "s3cret-pass", fast)
	ctx := context.Background()

	// Email is matched case-insensitively and trimmed.
	u, err := a.Login(ctx, app.Credentials{Email: " ann@example.COM ", Password: "s3cret-pass"}, "192.0.2.1")
	if err != nil || u != (app.User{ID: ann.ID, Name: "Ann", Email: "Ann@Example.com", Admin: true}) {
		t.Fatalf("Login: %+v, %v", u, err)
	}
	ev := logs.Events(logx.EventLoginSucceeded)
	if len(ev) != 1 || ev[0].Level != slog.LevelInfo ||
		ev[0].Attrs[logx.AttrUserID] != "1" || ev[0].Attrs[logx.AttrClientAddress] != "192.0.2.1" {
		t.Errorf("succeeded events %+v", ev)
	}

	// A wrong password and an unknown email give the same error.
	for _, in := range []app.Credentials{
		{Email: "ann@example.com", Password: "wrong-password"},
		{Email: "nobody@example.com", Password: "s3cret-pass"},
	} {
		_, err := a.Login(ctx, in, "192.0.2.2")
		var e *app.Error
		if !errors.As(err, &e) || e.Kind != app.KindUnauthorized || e.Message != app.LoginFailedMessage {
			t.Errorf("%s: %v", in.Email, err)
		}
	}
	failed := logs.Events(logx.EventLoginFailed)
	if len(failed) != 2 {
		t.Fatalf("failed events %+v", failed)
	}
	for _, e := range failed {
		if e.Level != slog.LevelWarn || e.Attrs[logx.AttrClientAddress] != "192.0.2.2" {
			t.Errorf("failed event %+v", e)
		}
		// Spec §2.4.3: never the email or the password that was tried.
		for _, secret := range []string{"ann@example.com", "nobody@example.com", "wrong-password", "s3cret-pass"} {
			if strings.Contains(strings.ToLower(e.Text()), secret) {
				t.Errorf("failed event leaks %q: %s", secret, e.Text())
			}
		}
	}
}

func TestLoginValidation(t *testing.T) {
	t.Parallel()
	a, _, logs := newApp(t)
	_, err := a.Login(context.Background(), app.Credentials{Email: "  "}, "x")
	var e *app.Error
	if !errors.As(err, &e) || e.Kind != app.KindInvalid || e.Details["email"] != "is required" || e.Details["password"] != "is required" {
		t.Errorf("empty credentials: %v", err)
	}
	if n := len(logs.All()); n != 0 {
		t.Errorf("an invalid request was logged: %+v", logs.All())
	}
}

// A hash with fewer iterations than the current setting is replaced on login (spec §9.1).
func TestLoginRehashes(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	users := store.NewUsers(pool)
	old := addUser(t, users, "ann@example.com", "s3cret-pass", fast)
	stronger := password.Hasher{Iterations: 2000}
	a := app.New(pool, app.Options{Logger: slog.New(slog.DiscardHandler), Hasher: stronger})

	if _, err := a.Login(context.Background(), app.Credentials{Email: "ann@example.com", Password: "s3cret-pass"}, "x"); err != nil {
		t.Fatal(err)
	}
	u, _ := users.Get(context.Background(), old.ID)
	if u.PasswordHash == old.PasswordHash || !strings.HasPrefix(u.PasswordHash, "pbkdf2-sha256$2000$") {
		t.Errorf("hash not upgraded: %s", u.PasswordHash)
	}
	if ok, rehash, err := stronger.Verify("s3cret-pass", u.PasswordHash); !ok || rehash || err != nil {
		t.Errorf("new hash: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
}

// A corrupt stored hash is an unexpected error (a 500), never a successful login.
func TestLoginMalformedHash(t *testing.T) {
	t.Parallel()
	a, users, _ := newApp(t)
	if _, err := users.Create(context.Background(), store.NewUser{Name: "X", Email: "x@example.com", PasswordHash: "garbage"}); err != nil {
		t.Fatal(err)
	}
	_, err := a.Login(context.Background(), app.Credentials{Email: "x@example.com", Password: "whatever"}, "x")
	if err == nil || app.KindOf(err) != 0 || !errors.Is(err, password.ErrMalformed) {
		t.Errorf("malformed hash: %v", err)
	}
}
