package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// userID returns the id of the user in a 200/201 response.
func userID(t *testing.T, body []byte) int64 {
	t.Helper()
	var u struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &u); err != nil || u.ID == 0 {
		t.Fatalf("no user in %s: %v", body, err)
	}
	return u.ID
}

func TestUserEndpoints(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	admin := e.login("ann@example.com")

	rec := e.do("POST", "/api/user", `{"name":"Cy","email":"cy@example.com","password":"password1","admin":true}`, admin)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"name":"Cy","email":"cy@example.com","admin":true}`) {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "assword") {
		t.Errorf("the response shows the password: %s", rec.Body)
	}
	cy := userID(t, rec.Body.Bytes())
	path := "/api/user/" + strconv.FormatInt(cy, 10)

	rec = e.do("GET", "/api/user", "", admin)
	if rec.Code != http.StatusOK || strings.Count(rec.Body.String(), `"id":`) != 3 {
		t.Errorf("list: %d %s", rec.Code, rec.Body)
	}
	rec = e.do("PATCH", path, `{"admin":false,"password":"password2"}`, admin)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"admin":false`) {
		t.Errorf("patch: %d %s", rec.Code, rec.Body)
	}
	e.login2(t, "cy@example.com", "password2")
	if rec := e.do("DELETE", path, "", admin); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("DELETE", path, "", admin); rec.Code != http.StatusNotFound {
		t.Errorf("delete again: %d %s", rec.Code, rec.Body)
	}
}

func TestUserEndpointErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	admin := e.login("ann@example.com")
	me := userID(t, e.do("GET", "/api/user/profile", "", admin).Body.Bytes())

	for name, c := range map[string]struct {
		method, path, body string
		status             int
		want               string
	}{
		"validation":      {"POST", "/api/user", `{"name":"","email":"x","password":"short"}`, 400, `"password":"must be at least 8 characters"`},
		"duplicate":       {"POST", "/api/user", `{"name":"B","email":"BOB@example.com","password":"password1"}`, 409, `A user with email \"BOB@example.com\" already exists`},
		"unknown field":   {"POST", "/api/user", `{"name":"B","email":"b@x","password":"password1","role":"x"}`, 400, `"role":"unknown field"`},
		"own account":     {"DELETE", "/api/user/" + strconv.FormatInt(me, 10), "", 400, "You cannot delete your own account"},
		"demote last":     {"PATCH", "/api/user/" + strconv.FormatInt(me, 10), `{"admin":false}`, 400, `"admin":"the last admin must stay an admin"`},
		"not found":       {"PATCH", "/api/user/999999", `{"name":"x"}`, 404, "User 999999 not found"},
		"bad id":          {"DELETE", "/api/user/007", "", 400, ""},
		"null name":       {"PATCH", "/api/user/" + strconv.FormatInt(me, 10), `{"name":null}`, 400, `"name":"is required"`},
		"no get by id":    {"GET", "/api/user/1", "", 405, ""},
		"profile null":    {"PATCH", "/api/user/profile", `{"email":null}`, 400, `"email":"is required"`},
		"profile admin":   {"PATCH", "/api/user/profile", `{"admin":false}`, 400, `"admin":"unknown field"`},
		"wrong current":   {"PATCH", "/api/user/profile", `{"currentPassword":"nope-nope","newPassword":"password9"}`, 400, `"currentPassword":"is not correct"`},
		"missing current": {"PATCH", "/api/user/profile", `{"newPassword":"password9"}`, 400, `"currentPassword":"is required to set a new password"`},
	} {
		rec := e.do(c.method, c.path, c.body, admin)
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: %d %s, want %d with %s", name, rec.Code, rec.Body, c.status, c.want)
		}
	}
}

func TestProfileEndpoints(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	bob := e.login("bob@example.com")

	rec := e.do("GET", "/api/user/profile", "", bob)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"email":"bob@example.com","admin":false`) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	rec = e.do("PATCH", "/api/user/profile", `{"name":"Robert","currentPassword":"s3cret-pass","newPassword":"another-pass"}`, bob)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Robert"`) {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}
	e.login2(t, "bob@example.com", "another-pass")
	rec = e.do("PATCH", "/api/user/profile", `{"email":"ANN@example.com"}`, bob)
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate email: %d %s", rec.Code, rec.Body)
	}
}

// The profile is for every user; the rest is for admins (401 without a session, 403 for
// a user who is not an admin).
func TestUserEndpointGuards(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	bob := e.login("bob@example.com")
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/user", ""},
		{"POST", "/api/user", `{"name":"x","email":"x@x","password":"password1"}`},
		{"PATCH", "/api/user/1", `{"admin":false}`},
		{"DELETE", "/api/user/1", ""},
	} {
		if rec := e.do(c.method, c.path, c.body, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: %d", c.method, c.path, rec.Code)
		}
		if rec := e.do(c.method, c.path, c.body, bob); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a non-admin: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
	for _, m := range []string{"GET", "PATCH"} {
		if rec := e.do(m, "/api/user/profile", `{}`, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s profile without a session: %d", m, rec.Code)
		}
	}
	// Revoking a user's admin flag takes effect on their next request (spec §9.2).
	ann := e.login("ann@example.com")
	cy := userID(t, e.do("POST", "/api/user", `{"name":"Cy","email":"cy@example.com","password":"password1","admin":true}`, ann).Body.Bytes())
	cySession := e.login2(t, "cy@example.com", "password1")
	if rec := e.do("GET", "/api/user", "", cySession); rec.Code != http.StatusOK {
		t.Fatalf("as an admin: %d", rec.Code)
	}
	e.do("PATCH", "/api/user/"+strconv.FormatInt(cy, 10), `{"admin":false}`, ann)
	if rec := e.do("GET", "/api/user", "", cySession); rec.Code != http.StatusForbidden {
		t.Errorf("after the flag was revoked: %d", rec.Code)
	}
}

// login2 logs in with a password other than the default one.
func (e *env) login2(t *testing.T, email, pw string) *http.Cookie {
	t.Helper()
	rec := e.do("POST", "/api/auth/login", `{"email":"`+email+`","password":"`+pw+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", email, rec.Code, rec.Body)
	}
	return rec.Result().Cookies()[0]
}
