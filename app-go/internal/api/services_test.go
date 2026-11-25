package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
)

// Verify item: curl CRUD against /api/service works with a session cookie.
func TestServiceEndpoints(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c := e.login("bob@example.com") // any user, not only admins

	if rec := e.do("GET", "/api/service", "", c); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list: %d %s", rec.Code, rec.Body.String())
	}

	rec := e.do("POST", "/api/service", `{"path":"svc","name":"Svc","upstreamUrl":"http://up.test/base"}`, c)
	if rec.Code != http.StatusCreated || strings.TrimSpace(rec.Body.String()) != `{"path":"svc","name":"Svc","upstreamUrl":"http://up.test/base"}` {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do("GET", "/api/service/svc", "", c); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"Svc"`) {
		t.Errorf("get: %d %s", rec.Code, rec.Body.String())
	}

	// PATCH: renaming the path; absent fields are kept.
	rec = e.do("PATCH", "/api/service/svc", `{"path":"svc2"}`, c)
	var s app.Service
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &s) != nil || s != (app.Service{Path: "svc2", Name: "Svc", UpstreamURL: "http://up.test/base"}) {
		t.Errorf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do("GET", "/api/service/svc", "", c); rec.Code != 404 || !strings.Contains(rec.Body.String(), `"message":"Service \"svc\" not found"`) {
		t.Errorf("old path: %d %s", rec.Code, rec.Body.String())
	}

	rec = e.do("GET", "/api/service", "", c)
	var list []app.Service
	if json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list) != 1 || list[0].Path != "svc2" {
		t.Errorf("list: %s", rec.Body.String())
	}

	if rec := e.do("DELETE", "/api/service/svc2", "", c); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do("DELETE", "/api/service/svc2", "", c); rec.Code != 404 {
		t.Errorf("second delete: %d", rec.Code)
	}
}

func TestServiceEndpointErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c := e.login("ann@example.com")
	if rec := e.do("POST", "/api/service", `{"path":"svc","name":"Svc","upstreamUrl":"http://up.test"}`, c); rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do("POST", "/api/service", `{"path":"other","name":"Other","upstreamUrl":"http://up.test"}`, c); rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	cases := []struct {
		name, method, path, body string
		status                   int
		want                     string
	}{
		{"duplicate path", "POST", "/api/service", `{"path":"svc","name":"X","upstreamUrl":"http://x"}`, 409, `{"status":409,"message":"A service with path \"svc\" already exists"}`},
		{"rename onto a taken path", "PATCH", "/api/service/other", `{"path":"svc"}`, 409, `already exists`},
		{"path with /", "POST", "/api/service", `{"path":"a/b","name":"X","upstreamUrl":"http://x"}`, 400, `"path":"may contain only letters, digits, '.', '_' and '-'"`},
		{"bad URL", "POST", "/api/service", `{"path":"x","name":"X","upstreamUrl":"not a url"}`, 400, `"upstreamUrl":"must be an absolute http or https URL"`},
		{"missing fields", "POST", "/api/service", `{}`, 400, `"details":{"name":"is required","path":"is required","upstreamUrl":"is required"}`},
		{"unknown field", "POST", "/api/service", `{"path":"x","name":"X","upstreamUrl":"http://x","url":"y"}`, 400, `"url":"unknown field"`},
		{"wrong type", "POST", "/api/service", `{"path":1,"name":"X","upstreamUrl":"http://x"}`, 400, `"path":"must be a string"`},
		{"null name", "PATCH", "/api/service/svc", `{"name":null}`, 400, `"name":"is required"`},
		{"patch unknown", "PATCH", "/api/service/nope", `{"name":"x"}`, 404, `Service \"nope\" not found`},
		{"get unknown", "GET", "/api/service/nope", ``, 404, `not found`},
		{"a path with / is no route", "GET", "/api/service/a/b", ``, 404, `"status":404`},
		{"wrong method", "PUT", "/api/service/svc", `{}`, 405, `"status":405`},
	}
	for _, tc := range cases {
		rec := e.do(tc.method, tc.path, tc.body, c)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestServiceEndpointsNeedSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/service", ""},
		{"POST", "/api/service", `{"path":"svc","name":"Svc","upstreamUrl":"http://up.test"}`},
		{"GET", "/api/service/svc", ""},
		{"PATCH", "/api/service/svc", `{"name":"x"}`},
		{"DELETE", "/api/service/svc", ""},
	} {
		if rec := e.do(r.method, r.path, r.body, nil); rec.Code != 401 {
			t.Errorf("%s %s without session: %d", r.method, r.path, rec.Code)
		}
	}
	var n int
	if err := e.pool.QueryRow(`SELECT count(*) FROM services`).Scan(&n); err != nil || n != 0 {
		t.Errorf("services created without a session: %d, %v", n, err)
	}
}
