package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/api"
	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/password"
	"github.com/laposa/tmock/app-go/internal/server"
	"github.com/laposa/tmock/app-go/internal/store"
)

var fast = password.Hasher{Iterations: 1000}

type env struct {
	t    *testing.T
	pool *sql.DB
	h    http.Handler
}

// newEnv serves the whole application (as cmd/tmock wires it, without the UI) on a fresh
// database with two users: an admin and a regular user, both with password "s3cret-pass".
func newEnv(t *testing.T) *env {
	t.Helper()
	pool := dbtest.Migrated(t)
	logger := slog.New(slog.DiscardHandler)
	users := store.NewUsers(pool)
	hash, _ := fast.Hash("s3cret-pass")
	for _, u := range []store.NewUser{
		{Name: "Ann", Email: "ann@example.com", PasswordHash: hash, Admin: true},
		{Name: "Bob", Email: "bob@example.com", PasswordHash: hash},
	} {
		if _, err := users.Create(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	uc := app.New(pool, app.Options{Logger: logger, Hasher: fast})
	sessions := auth.New(store.NewSessions(pool), auth.Options{Secret: []byte("0123456789abcdef0123456789abcdef"), Logger: logger})
	return &env{t: t, pool: pool, h: server.New(server.Options{API: api.New(uc, sessions, logger), Logger: logger})}
}

func (e *env) do(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return rec
}

func (e *env) login(email string) *http.Cookie {
	e.t.Helper()
	rec := e.do("POST", "/api/auth/login", `{"email":"`+email+`","password":"s3cret-pass"}`, nil)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("login %s: %d %s", email, rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	e.t.Fatal("no session cookie")
	return nil
}

func TestLoginMeLogout(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	rec := e.do("POST", "/api/auth/login", `{"email":"BOB@example.com","password":"s3cret-pass"}`, nil)
	var u app.User
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &u) != nil || u.Email != "bob@example.com" || u.Admin {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("login response mentions the password: %s", rec.Body.String())
	}
	cookie := rec.Result().Cookies()[0]

	if rec := e.do("GET", "/api/auth/me", "", cookie); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"Bob"`) {
		t.Errorf("me: %d %s", rec.Code, rec.Body.String())
	}

	rec = e.do("POST", "/api/auth/logout", "", cookie)
	if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("logout: %d %v", rec.Code, rec.Header())
	}
	if rec := e.do("GET", "/api/auth/me", "", cookie); rec.Code != 401 {
		t.Errorf("me after logout: %d", rec.Code)
	}
	// Logout without a session is still 204.
	if rec := e.do("POST", "/api/auth/logout", "", nil); rec.Code != http.StatusNoContent {
		t.Errorf("second logout: %d", rec.Code)
	}
}

func TestLoginFailures(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cases := []struct {
		name, body string
		status     int
		want       string
	}{
		{"wrong password", `{"email":"ann@example.com","password":"nope-nope"}`, 401, `{"status":401,"message":"Invalid email or password"}`},
		{"unknown email", `{"email":"zed@example.com","password":"s3cret-pass"}`, 401, `{"status":401,"message":"Invalid email or password"}`},
		{"missing fields", `{}`, 400, `"details":{"email":"is required","password":"is required"}`},
		{"unknown field", `{"email":"a","password":"b","remember":true}`, 400, `"remember":"unknown field"`},
		{"not JSON", `email=a`, 400, `Malformed JSON`},
	}
	for _, tc := range cases {
		rec := e.do("POST", "/api/auth/login", tc.body, nil)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s: sets a cookie", tc.name)
		}
	}
}

func TestMeNeedsSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	bad := &http.Cookie{Name: auth.CookieName, Value: "forged"}
	for _, c := range []*http.Cookie{nil, bad} {
		if rec := e.do("GET", "/api/auth/me", "", c); rec.Code != 401 || !strings.Contains(rec.Body.String(), "Not logged in") {
			t.Errorf("cookie %v: %d %s", c, rec.Code, rec.Body.String())
		}
	}
}

// Verify item: a deleted user's session stops working, at once.
func TestDeletedUsersSessionEnds(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cookie := e.login("bob@example.com")
	if _, err := e.pool.Exec(`DELETE FROM users WHERE email = 'bob@example.com'`); err != nil {
		t.Fatal(err)
	}
	if rec := e.do("GET", "/api/auth/me", "", cookie); rec.Code != 401 {
		t.Errorf("me: %d", rec.Code)
	}
}

// Changing the admin flag shows in the next request.
func TestAdminFlagIsCurrent(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cookie := e.login("bob@example.com")
	if _, err := e.pool.Exec(`UPDATE users SET admin = true WHERE email = 'bob@example.com'`); err != nil {
		t.Fatal(err)
	}
	if rec := e.do("GET", "/api/auth/me", "", cookie); !strings.Contains(rec.Body.String(), `"admin":true`) {
		t.Errorf("me: %s", rec.Body.String())
	}
}

// The login endpoint is protected against cross-site browser posts (spec §9.3).
func TestLoginCrossOrigin(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"ann@example.com","password":"s3cret-pass"}`))
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 403 || rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("cross-site login: %d", rec.Code)
	}
}
