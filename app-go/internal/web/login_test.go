package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/password"
	"github.com/laposa/tmock/app-go/internal/store"
)

// newDBHandler is the UI handler on a fresh database with one user,
// ann@example.com / s3cret-pass.
func newDBHandler(t *testing.T) http.Handler {
	t.Helper()
	pool := dbtest.Migrated(t)
	hasher := password.Hasher{Iterations: 1000}
	hash, _ := hasher.Hash("s3cret-pass")
	if _, err := store.NewUsers(pool).Create(context.Background(), store.NewUser{Name: "Ann", Email: "ann@example.com", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	return Handler(Deps{
		App:      app.New(pool, app.Options{Logger: logger, Hasher: hasher}),
		Sessions: auth.New(store.NewSessions(pool), auth.Options{Secret: []byte("0123456789abcdef0123456789abcdef"), Logger: logger}),
		Logger:   logger,
	})
}

func postLogin(h http.Handler, email, pw string, htmx bool) *httptest.ResponseRecorder {
	form := url.Values{"email": {email}, "password": {pw}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		r.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestLoginForm(t *testing.T) {
	t.Parallel()
	h := newDBHandler(t)

	// A plain submit is redirected with a 303; htmx gets HX-Redirect.
	rec := postLogin(h, "ann@example.com", "s3cret-pass", false)
	if rec.Code != 303 || rec.Header().Get("Location") != "/" {
		t.Fatalf("plain login: %d %v", rec.Code, rec.Header())
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Name != auth.CookieName {
		t.Fatalf("cookie %v", cookie)
	}
	if rec := serve(h, "GET", "/", cookie, false); rec.Code != 200 {
		t.Errorf("after login: %d", rec.Code)
	}
	if rec := postLogin(h, "ann@example.com", "s3cret-pass", true); rec.Code != 200 || rec.Header().Get("HX-Redirect") != "/" || rec.Header().Get("Set-Cookie") == "" {
		t.Errorf("htmx login: %d %v", rec.Code, rec.Header())
	}
}

// Verify item: a wrong password shows the error inline, as 422 (spec §10.1).
func TestLoginFormErrors(t *testing.T) {
	t.Parallel()
	h := newDBHandler(t)

	rec := postLogin(h, "ann@example.com", "wrong-pass", true)
	body := rec.Body.String()
	if rec.Code != 422 || rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("wrong password: %d %v", rec.Code, rec.Header())
	}
	// htmx gets just the form, with the message and the email kept.
	if !strings.HasPrefix(body, `<form class="form"`) || strings.Contains(body, "<html") {
		t.Errorf("not a form fragment: %s", body)
	}
	for _, want := range []string{`role="alert">Invalid email or password</div>`, `value="ann@example.com"`} {
		if !strings.Contains(body, want) {
			t.Errorf("fragment lacks %q", want)
		}
	}
	if strings.Contains(body, "wrong-pass") {
		t.Error("the password is echoed back")
	}

	// A plain submit gets the whole page.
	rec = postLogin(h, "nobody@example.com", "s3cret-pass", false)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "<title>Login · Tmock</title>") ||
		!strings.Contains(rec.Body.String(), "Invalid email or password") {
		t.Errorf("plain wrong email: %d %s", rec.Code, rec.Body.String())
	}

	// Empty fields: the messages are next to the fields, not in the alert.
	rec = postLogin(h, "", "", true)
	body = rec.Body.String()
	if rec.Code != 422 || strings.Contains(body, `role="alert"`) ||
		!strings.Contains(body, `<p class="field__error" id="email-message">is required</p>`) ||
		!strings.Contains(body, `<p class="field__error" id="password-message">is required</p>`) {
		t.Errorf("empty fields: %d %s", rec.Code, body)
	}
}

func TestToast(t *testing.T) {
	rec := httptest.NewRecorder()
	toast(rec, toastSuccess, "Saved", `Client "x" <enabled>`)
	var got map[string]map[string]string
	if err := json.Unmarshal([]byte(rec.Header().Get("HX-Trigger")), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"target": "body", "type": "success", "title": "Saved", "message": `Client "x" <enabled>`}
	for k, v := range want {
		if got["toast"][k] != v {
			t.Errorf("%s = %q, want %q", k, got["toast"][k], v)
		}
	}
}

// fail maps errors as spec §10.2 says.
func TestFail(t *testing.T) {
	logs, logger := logxtest.New()
	u := &ui{logger: logger}
	cases := []struct {
		name    string
		err     error
		htmx    bool
		status  int
		message string
		logged  bool
	}{
		{"not found, htmx", app.NotFound("Service %q not found", "svc"), true, 404, `Service "svc" not found`, false},
		{"unexpected, htmx", errors.New("db down: password=x"), true, 500, "Something went wrong. Please try again.", true},
		{"unexpected, plain", errors.New("db down"), false, 500, "Something went wrong. Please try again.", true},
	}
	for _, tc := range cases {
		before := len(logs.All())
		r := httptest.NewRequest("GET", "/x", nil)
		if tc.htmx {
			r.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		u.fail(rec, r, tc.err)
		if rec.Code != tc.status {
			t.Errorf("%s: status %d", tc.name, rec.Code)
		}
		if tc.htmx {
			if rec.Header().Get("HX-Reswap") != "none" || !strings.Contains(rec.Header().Get("HX-Trigger"), `"type":"error"`) ||
				!strings.Contains(rec.Header().Get("HX-Trigger"), jsonString(tc.message)) {
				t.Errorf("%s: headers %v", tc.name, rec.Header())
			}
		} else if !strings.Contains(rec.Body.String(), tc.message) {
			t.Errorf("%s: body %q", tc.name, rec.Body.String())
		}
		if logged := len(logs.All()) > before; logged != tc.logged {
			t.Errorf("%s: logged = %v", tc.name, logged)
		}
		if strings.Contains(rec.Header().Get("HX-Trigger")+rec.Body.String(), "db down") {
			t.Errorf("%s: the error's details reached the user", tc.name)
		}
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
