package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/auth"
)

// sessionCookie logs userID in directly and returns the cookie.
func sessionCookie(t *testing.T, s *auth.Sessions, userID int64) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := s.Start(context.Background(), rec, userID, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	return rec.Result().Cookies()[0]
}

func serve(h http.Handler, method, path string, c *http.Cookie, htmx bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if c != nil {
		r.AddCookie(c)
	}
	if htmx {
		r.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// Verify items: the route guards of spec §10.3 Layout, for plain and htmx requests.
func TestRouteGuards(t *testing.T) {
	h, sessions, _ := newHandler(t)
	admin, user := sessionCookie(t, sessions, 1), sessionCookie(t, sessions, 2)
	cases := []struct {
		path     string
		cookie   *http.Cookie
		status   int
		location string // redirect target, "" when the page is shown
	}{
		{"/", nil, 303, "/login"},
		{"/services", nil, 303, "/login"},
		{"/scenarios", nil, 303, "/login"},
		{"/accounts", nil, 303, "/login"},
		{"/my-account", nil, 303, "/login"},
		{"/", user, 200, ""},
		{"/services", user, 200, ""},
		{"/scenarios", user, 200, ""},
		{"/my-account", user, 200, ""},
		{"/accounts", user, 303, "/"},
		{"/accounts", admin, 200, ""},
		{"/login", nil, 200, ""},
		{"/login", user, 303, "/"},
	}
	for _, tc := range cases {
		rec := serve(h, "GET", tc.path, tc.cookie, false)
		if rec.Code != tc.status || rec.Header().Get("Location") != tc.location {
			t.Errorf("GET %s (cookie %v): %d → %q, want %d → %q", tc.path, tc.cookie != nil, rec.Code, rec.Header().Get("Location"), tc.status, tc.location)
		}
		// htmx ignores 3xx headers, so it gets HX-Redirect on a 200 (spec §10.2).
		if tc.location != "" {
			rec := serve(h, "GET", tc.path, tc.cookie, true)
			if rec.Code != 200 || rec.Header().Get("HX-Redirect") != tc.location || rec.Header().Get("Location") != "" {
				t.Errorf("htmx GET %s: %d HX-Redirect=%q", tc.path, rec.Code, rec.Header().Get("HX-Redirect"))
			}
		}
	}
}

// The pages a user sees: the navigation matches their role, nothing is cached, and no
// page uses what would need eval (spec §10.1).
func TestPlaceholderPages(t *testing.T) {
	h, sessions, _ := newHandler(t)
	admin, user := sessionCookie(t, sessions, 1), sessionCookie(t, sessions, 2)

	rec := serve(h, "GET", "/services", user, false)
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Services · Tmock</title>",
		`<a class="nav__link" href="/services" aria-current="page">Services</a>`,
		"iteration I05",
		`x-data="jsStatus"`,
		`hx-get="/api/status"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/services lacks %q", want)
		}
	}
	if strings.Contains(body, `href="/accounts"`) {
		t.Error("non-admin sees Accounts")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control %q", cc)
	}
	assertNoEval(t, body)

	if body := serve(h, "GET", "/", admin, false).Body.String(); !strings.Contains(body, `href="/accounts"`) {
		t.Error("admin does not see Accounts")
	}
	if body := serve(h, "GET", "/login", nil, false).Body.String(); !strings.Contains(body, `<title>Login · Tmock</title>`) {
		t.Errorf("login page: %s", body)
	} else {
		assertNoEval(t, body)
	}
}

// A deleted user's session stops working in the UI as well.
func TestDeletedUserIsLoggedOut(t *testing.T) {
	h, sessions, st := newHandler(t)
	c := sessionCookie(t, sessions, 2)
	st.DeleteUser(2)
	if rec := serve(h, "GET", "/", c, false); rec.Code != 303 || rec.Header().Get("Location") != "/login" {
		t.Errorf("deleted user: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLogout(t *testing.T) {
	h, sessions, st := newHandler(t)
	c := sessionCookie(t, sessions, 2)
	rec := serve(h, "POST", "/logout", c, false)
	if rec.Code != 303 || rec.Header().Get("Location") != "/login" || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("logout: %d %v", rec.Code, rec.Header())
	}
	if st.Len() != 0 {
		t.Error("session not deleted")
	}
	if rec := serve(h, "GET", "/", c, false); rec.Code != 303 {
		t.Errorf("after logout: %d", rec.Code)
	}
	if rec := serve(h, "POST", "/logout", nil, true); rec.Code != 200 || rec.Header().Get("HX-Redirect") != "/login" {
		t.Errorf("htmx logout without session: %d %v", rec.Code, rec.Header())
	}
	if rec := serve(h, "GET", "/logout", c, false); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /logout: %d", rec.Code)
	}
}
