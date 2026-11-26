package logx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/api"
	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/password"
	"github.com/laposa/tmock/app-go/internal/server"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Spec §2.4.3: a login (failed and successful), a request with the session and a logout
// leave neither the password, its hash, the email that was tried, nor the session cookie
// in any record.
func TestLoginNeverLogsSecrets(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	hasher := password.Hasher{Iterations: 1000}
	hash, _ := hasher.Hash("pass-word-value")
	if _, err := store.NewUsers(pool).Create(context.Background(), store.NewUser{Name: "Ann", Email: "ann@example.com", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	logs, logger := logxtest.New()
	sessions := auth.New(store.NewSessions(pool), auth.Options{Secret: []byte("session-secret-value-0123456789ab"), Logger: logger})
	h := server.New(server.Options{API: api.New(app.New(pool, app.Options{Logger: logger, Hasher: hasher}), sessions, logger), Logger: logger})

	do := func(method, path, body string, c *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if c != nil {
			r.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	do("POST", "/api/auth/login", `{"email":"tried@example.com","password":"wrong-pass-value"}`, nil)
	rec := do("POST", "/api/auth/login", `{"email":"ann@example.com","password":"pass-word-value"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("login: %d", rec.Code)
	}
	cookie := rec.Result().Cookies()[0]
	do("GET", "/api/auth/me", "", cookie)
	do("POST", "/api/auth/logout", "", cookie)

	secrets := []string{"pass-word-value", "wrong-pass-value", "tried@example.com", "ann@example.com", cookie.Value, hash, "session-secret-value"}
	if len(logs.All()) < 5 {
		t.Fatalf("only %d records", len(logs.All()))
	}
	for _, r := range logs.All() {
		for _, s := range secrets {
			if strings.Contains(r.Text(), s) {
				t.Errorf("record leaks %q: %s", s, r.Text())
			}
		}
	}
}

// Spec §2.4.3: generating a client token and using it (X-Client-Token) leaves the token
// in no record.
func TestClientTokenNeverLogged(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	hasher := password.Hasher{Iterations: 1000}
	hash, _ := hasher.Hash("pass-word-value")
	if _, err := store.NewUsers(pool).Create(context.Background(), store.NewUser{Name: "Ann", Email: "ann@example.com", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	logs, logger := logxtest.New()
	sessions := auth.New(store.NewSessions(pool), auth.Options{Secret: []byte("session-secret-value-0123456789ab"), Logger: logger})
	h := server.New(server.Options{API: api.New(app.New(pool, app.Options{Logger: logger, Hasher: hasher}), sessions, logger), Logger: logger})
	do := func(method, path, body string, c *http.Cookie, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if c != nil {
			r.AddCookie(c)
		}
		if token != "" {
			r.Header.Set("X-Client-Token", token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	cookie := do("POST", "/api/auth/login", `{"email":"ann@example.com","password":"pass-word-value"}`, nil, "").Result().Cookies()[0]
	if rec := do("POST", "/api/client", `{"name":"c","condition":{"ip":"10.0.0.1"}}`, cookie, ""); rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec := do("POST", "/api/client/1/token", "", cookie, "")
	var tok struct{ Token string }
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &tok) != nil {
		t.Fatalf("token: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do("PATCH", "/api/client/1", `{"enabled":true}`, nil, tok.Token); rec.Code != 200 {
		t.Fatalf("token PATCH: %d %s", rec.Code, rec.Body.String())
	}
	do("PATCH", "/api/client/1", `{"name":"x"}`, nil, tok.Token)    // 403
	do("PATCH", "/api/client/1", `{"enabled":true}`, nil, "tmck_x") // 401

	if len(logs.Events(logx.EventEntityChanged)) < 3 {
		t.Fatalf("records: %+v", logs.All())
	}
	for _, r := range logs.All() {
		for _, s := range []string{tok.Token, strings.TrimPrefix(tok.Token, "tmck_"), "tmck_x"} {
			if strings.Contains(r.Text(), s) {
				t.Errorf("record leaks %q: %s", s, r.Text())
			}
		}
	}
}
