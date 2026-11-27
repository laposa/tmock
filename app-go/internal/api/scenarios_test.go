package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func scenarioEnv(t *testing.T) (*env, *http.Cookie) {
	t.Helper()
	e := newEnv(t)
	c := e.login("bob@example.com") // any user, not only admins
	for _, p := range []string{"svc", "other"} {
		if rec := e.do("POST", "/api/service", `{"path":"`+p+`","name":"`+p+`","upstreamUrl":"http://up.test"}`, c); rec.Code != 201 {
			t.Fatalf("create service: %d %s", rec.Code, rec.Body.String())
		}
	}
	return e, c
}

// Verify item: CRUD on /api/scenario with a session cookie; every optional field is
// present, as null when unset (spec §6.1).
func TestScenarioEndpoints(t *testing.T) {
	t.Parallel()
	e, c := scenarioEnv(t)

	if rec := e.do("GET", "/api/scenario", "", c); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list: %d %s", rec.Code, rec.Body.String())
	}

	rec := e.do("POST", "/api/scenario", `{"service":"svc","name":"bare"}`, c)
	var bare struct{ ID int64 }
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &bare) != nil {
		t.Fatalf("create bare: %d %s", rec.Code, rec.Body.String())
	}
	wantBare := fmt.Sprintf(`{"id":%d,"service":"svc","name":"bare","requestMethod":null,"requestPath":null,"requestCondition":null,`+
		`"responseCode":null,"responseHeaders":null,"responseBody":null,"skipProxy":false}`, bare.ID)
	if strings.TrimSpace(rec.Body.String()) != wantBare {
		t.Errorf("create bare:\n got %s\nwant %s", rec.Body.String(), wantBare)
	}

	rec = e.do("POST", "/api/scenario", `{"service":"svc","name":"full","requestMethod":"get","requestPath":"/orders/","requestCondition":"$request.ip == '1.2.3.4'",`+
		`"responseCode":503,"responseHeaders":{"Content-Type":"application/json","ETag":null},"responseBody":"{\"down\":true}","skipProxy":true}`, c)
	var full struct{ ID int64 }
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &full) != nil {
		t.Fatalf("create full: %d %s", rec.Code, rec.Body.String())
	}
	wantFull := fmt.Sprintf(`{"id":%d,"service":"svc","name":"full","requestMethod":"GET","requestPath":"orders","requestCondition":"$request.ip == '1.2.3.4'",`+
		`"responseCode":503,"responseHeaders":{"Content-Type":"application/json","ETag":null},"responseBody":"{\"down\":true}","skipProxy":true}`, full.ID)
	if got := e.do("GET", fmt.Sprintf("/api/scenario/%d", full.ID), "", c).Body.String(); strings.TrimSpace(got) != wantFull {
		t.Errorf("get full:\n got %s\nwant %s", got, wantFull)
	}

	// PATCH: null clears, absent keeps.
	rec = e.do("PATCH", fmt.Sprintf("/api/scenario/%d", full.ID), `{"responseHeaders":null,"skipProxy":false,"responseBody":null,"name":"renamed"}`, c)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"renamed","requestMethod":"GET"`) ||
		!strings.Contains(rec.Body.String(), `"responseCode":503,"responseHeaders":null,"responseBody":null,"skipProxy":false`) {
		t.Errorf("patch: %d %s", rec.Code, rec.Body.String())
	}

	// ?service= filters; full bodies are listed.
	if rec := e.do("GET", "/api/scenario?service=other", "", c); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("filter other: %s", rec.Body.String())
	}
	rec = e.do("GET", "/api/scenario?service=svc", "", c)
	var list []map[string]any
	if json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list) != 2 || list[0]["name"] != "bare" || list[1]["name"] != "renamed" {
		t.Errorf("filter svc: %s", rec.Body.String())
	}

	if rec := e.do("DELETE", fmt.Sprintf("/api/scenario/%d", bare.ID), "", c); rec.Code != 204 || rec.Body.Len() != 0 {
		t.Errorf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do("GET", fmt.Sprintf("/api/scenario/%d", bare.ID), "", c); rec.Code != 404 ||
		!strings.Contains(rec.Body.String(), fmt.Sprintf(`"message":"Scenario %d not found"`, bare.ID)) {
		t.Errorf("get deleted: %d %s", rec.Code, rec.Body.String())
	}
}

func TestScenarioEndpointErrors(t *testing.T) {
	t.Parallel()
	e, c := scenarioEnv(t)
	rec := e.do("POST", "/api/scenario", `{"service":"svc","name":"s"}`, c)
	var s struct{ ID int64 }
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &s) != nil {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id := fmt.Sprintf("/api/scenario/%d", s.ID)
	cases := []struct {
		name, method, path, body string
		status                   int
		want                     string
	}{
		{"duplicate name", "POST", "/api/scenario", `{"service":"svc","name":"s"}`, 409, `{"status":409,"message":"A scenario named \"s\" already exists in service \"svc\""}`},
		{"unknown service", "POST", "/api/scenario", `{"service":"nope","name":"s"}`, 404, `Service \"nope\" not found`},
		{"missing fields", "POST", "/api/scenario", `{}`, 400, `"details":{"name":"is required","service":"is required"}`},
		{"skipProxy needs a body", "POST", "/api/scenario", `{"service":"svc","name":"x","skipProxy":true}`, 400, `"skipProxy":"requires a response body"`},
		{"bad method", "POST", "/api/scenario", `{"service":"svc","name":"x","requestMethod":"FETCH"}`, 400, `"requestMethod":"must be one of`},
		{"bad condition", "POST", "/api/scenario", `{"service":"svc","name":"x","requestCondition":"$request.body.a =="}`, 400, `"requestCondition":"column 19: expected a value but found end of condition"`},
		{"bad template", "POST", "/api/scenario", `{"service":"svc","name":"x","responseBody":"<% TEMPLATE: LiquidJS %>{{ 1 | }}"}`, 400, `"responseBody":"line 1, column 32: expected a filter name after \"|\", but found nothing"`},
		{"bad code", "POST", "/api/scenario", `{"service":"svc","name":"x","responseCode":42}`, 400, `"responseCode":"must be between 100 and 599"`},
		{"header value not a string", "POST", "/api/scenario", `{"service":"svc","name":"x","responseHeaders":{"X-A":1}}`, 400, `"responseHeaders.X-A":"must be a string"`},
		{"headers not an object", "POST", "/api/scenario", `{"service":"svc","name":"x","responseHeaders":["a"]}`, 400, `"responseHeaders":"must be an object"`},
		{"unknown field", "POST", "/api/scenario", `{"service":"svc","name":"x","body":"y"}`, 400, `"body":"unknown field"`},
		{"skipProxy null", "PATCH", id, `{"skipProxy":null}`, 400, `"skipProxy":"must be true or false"`},
		{"patch onto a taken name", "PATCH", id, `{"service":"other"}`, 200, `"service":"other"`},
		{"patch unknown", "PATCH", "/api/scenario/999999", `{"name":"x"}`, 404, `Scenario 999999 not found`},
		{"non-canonical id", "GET", "/api/scenario/007", ``, 400, `id must be a positive whole number`},
		{"id not a number", "DELETE", "/api/scenario/svc", ``, 400, `id must be a positive whole number`},
		{"wrong method", "PUT", id, `{}`, 405, `"status":405`},
	}
	for _, tc := range cases {
		rec := e.do(tc.method, tc.path, tc.body, c)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestScenarioEndpointsNeedSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/scenario", ""},
		{"POST", "/api/scenario", `{"service":"svc","name":"s"}`},
		{"GET", "/api/scenario/1", ""},
		{"PATCH", "/api/scenario/1", `{"name":"x"}`},
		{"DELETE", "/api/scenario/1", ""},
	} {
		if rec := e.do(r.method, r.path, r.body, nil); rec.Code != 401 {
			t.Errorf("%s %s without session: %d", r.method, r.path, rec.Code)
		}
	}
}
