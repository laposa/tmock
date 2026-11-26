package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// clientEnv is a session, the service svc with scenarios a and b, and their ids.
func clientEnv(t *testing.T) (*env, *http.Cookie, int64, int64) {
	t.Helper()
	e, c := scenarioEnv(t)
	ids := make([]int64, 2)
	for i, n := range []string{"a", "b"} {
		rec := e.do("POST", "/api/scenario", `{"service":"svc","name":"`+n+`"}`, c)
		var s struct{ ID int64 }
		if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &s) != nil {
			t.Fatalf("scenario: %d %s", rec.Code, rec.Body.String())
		}
		ids[i] = s.ID
	}
	return e, c, ids[0], ids[1]
}

func createClient(t *testing.T, e *env, c *http.Cookie, body string) int64 {
	t.Helper()
	rec := e.do("POST", "/api/client", body, c)
	var cl struct{ ID int64 }
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &cl) != nil {
		t.Fatalf("create client: %d %s", rec.Code, rec.Body.String())
	}
	return cl.ID
}

// doToken sends a request with X-Client-Token and, optionally, a session cookie.
func (e *env) doToken(method, path, body, token string, cookie *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Client-Token", token)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return rec
}

func TestClientEndpoints(t *testing.T) {
	t.Parallel()
	e, c, sa, sb := clientEnv(t)

	if rec := e.do("GET", "/api/client", "", c); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list: %d %s", rec.Code, rec.Body.String())
	}
	rec := e.do("POST", "/api/client", fmt.Sprintf(`{"name":"phone","enabled":true,"condition":{"or":[{"ip":"10.0.0.1"},{"not":true,"cidr":"10.0.0.0/8"}]},"scenarios":[%d,%d]}`, sb, sa), c)
	var cl struct{ ID int64 }
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &cl) != nil {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	want := fmt.Sprintf(`{"id":%d,"name":"phone","enabled":true,"condition":{"or":[{"ip":"10.0.0.1"},{"not":true,"cidr":"10.0.0.0/8"}]},"scenarios":[%d,%d],"hasToken":false}`, cl.ID, sa, sb)
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("create:\n got %s\nwant %s", rec.Body.String(), want)
	}
	path := fmt.Sprintf("/api/client/%d", cl.ID)
	if rec := e.do("GET", path, "", c); strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("get: %s", rec.Body.String())
	}

	rec = e.do("PATCH", path, `{"name":"tablet","condition":{"and":[]},"enabled":false,"scenarios":[]}`, c)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"tablet","enabled":false,"condition":{"and":[]},"scenarios":[],"hasToken":false`) {
		t.Errorf("patch: %d %s", rec.Code, rec.Body.String())
	}

	// Token: 201 with tmck_…, hasToken true, revoke 204.
	rec = e.do("POST", path+"/token", "", c)
	var tok struct{ Token string }
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &tok) != nil || !strings.HasPrefix(tok.Token, "tmck_") {
		t.Fatalf("token: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do("GET", path, "", c); !strings.Contains(rec.Body.String(), `"hasToken":true`) || strings.Contains(rec.Body.String(), tok.Token) {
		t.Errorf("after token: %s", rec.Body.String())
	}
	// Verify item: the DB contains no plaintext tokens.
	var plain int
	if err := e.pool.QueryRow(`SELECT count(*) FROM clients WHERE position(convert_to($1, 'UTF8') in token_hash) > 0 OR encode(token_hash, 'escape') LIKE '%tmck%'`, tok.Token).Scan(&plain); err != nil || plain != 0 {
		t.Errorf("plaintext token in the DB: %d, %v", plain, err)
	}
	if rec := e.do("DELETE", path+"/token", "", c); rec.Code != 204 {
		t.Errorf("revoke: %d", rec.Code)
	}

	// disable-all.
	other := createClient(t, e, c, `{"name":"other","enabled":true,"condition":{"ip":"1.2.3.4"}}`)
	if rec := e.do("POST", "/api/client/disable-all", "", c); rec.Code != 204 || rec.Body.Len() != 0 {
		t.Errorf("disable-all: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do("GET", fmt.Sprintf("/api/client/%d", other), "", c); !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("after disable-all: %s", rec.Body.String())
	}

	if rec := e.do("DELETE", path, "", c); rec.Code != 204 {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec := e.do("GET", path, "", c); rec.Code != 404 || !strings.Contains(rec.Body.String(), fmt.Sprintf(`Client %d not found`, cl.ID)) {
		t.Errorf("get deleted: %d %s", rec.Code, rec.Body.String())
	}
}

func TestClientEndpointErrors(t *testing.T) {
	t.Parallel()
	e, c, sa, _ := clientEnv(t)
	id := createClient(t, e, c, `{"name":"c"}`)
	path := fmt.Sprintf("/api/client/%d", id)
	cases := []struct {
		name, method, path, body string
		status                   int
		want                     string
	}{
		{"duplicate name", "POST", "/api/client", `{"name":"c"}`, 409, `{"status":409,"message":"A client named \"c\" already exists"}`},
		{"enable without condition", "POST", "/api/client", `{"name":"x","enabled":true}`, 400, `"enabled":"a client needs a condition before it can be enabled"`},
		{"enable by patch", "PATCH", path, `{"enabled":true}`, 400, `"enabled":"a client needs a condition before it can be enabled"`},
		{"nested condition path", "POST", "/api/client", `{"name":"x","condition":{"and":[{"or":[{"ip":"x"}]}]}}`, 400, `"condition.and[0].or[0].ip":"must be an IPv4 or IPv6 address"`},
		{"unknown field in a condition", "POST", "/api/client", `{"name":"x","condition":{"and":[{"ipp":"1.2.3.4"}]}}`, 400, `"ipp":"unknown field"`},
		{"unknown scenario ids", "POST", "/api/client", fmt.Sprintf(`{"name":"x","scenarios":[%d,999999]}`, sa), 400, `"scenarios":"unknown scenario ids: 999999"`},
		{"scenarios not ids", "POST", "/api/client", `{"name":"x","scenarios":["a"]}`, 400, `"scenarios[0]":"must be a whole number"`},
		{"scenarios null", "PATCH", path, `{"scenarios":null}`, 400, `"scenarios":"must be a list of scenario ids"`},
		{"unknown client", "PATCH", "/api/client/999999", `{"name":"x"}`, 404, `Client 999999 not found`},
		{"token for unknown client", "POST", "/api/client/999999/token", ``, 404, `Client 999999 not found`},
		{"revoke for unknown client", "DELETE", "/api/client/999999/token", ``, 404, `Client 999999 not found`},
		{"bad id", "GET", "/api/client/abc", ``, 400, `id must be a positive whole number`},
		{"disable-all is POST only", "GET", "/api/client/disable-all", ``, 400, `id must be a positive whole number`},
	}
	for _, tc := range cases {
		rec := e.do(tc.method, tc.path, tc.body, c)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

// Verify item: the token cases of the token-or-user guard (spec §6.3).
func TestClientTokenGuard(t *testing.T) {
	t.Parallel()
	e, c, sa, sb := clientEnv(t)
	mine := createClient(t, e, c, `{"name":"mine","condition":{"ip":"10.0.0.1"}}`)
	other := createClient(t, e, c, `{"name":"other"}`)
	rec := e.do("POST", fmt.Sprintf("/api/client/%d/token", mine), "", c)
	var tok struct{ Token string }
	if json.Unmarshal(rec.Body.Bytes(), &tok) != nil {
		t.Fatal(rec.Body.String())
	}
	minePath, otherPath := fmt.Sprintf("/api/client/%d", mine), fmt.Sprintf("/api/client/%d", other)

	cases := []struct {
		name, method, path, body, token string
		status                          int
		want                            string
	}{
		{"wrong token", "PATCH", minePath, `{"enabled":true}`, "tmck_wrong", 401, `{"status":401,"message":"Unknown client token"}`},
		{"wrong token on GET", "GET", minePath, ``, "tmck_wrong", 401, `Unknown client token`},
		{"other client", "PATCH", otherPath, `{"enabled":false}`, tok.Token, 403, `{"status":403,"message":"This token belongs to another client"}`},
		{"other client GET", "GET", otherPath, ``, tok.Token, 403, `another client`},
		{"disallowed fields", "PATCH", minePath, `{"enabled":true,"name":"x","condition":null}`, tok.Token, 403, `{"status":403,"message":"A client token may only change enabled and scenarios, not: condition, name"}`},
		{"valid GET without session", "GET", minePath, ``, tok.Token, 200, `"name":"mine"`},
		{"valid PATCH without session", "PATCH", minePath, fmt.Sprintf(`{"enabled":true,"scenarios":[%d,%d]}`, sa, sb), tok.Token, 200, fmt.Sprintf(`"enabled":true,"condition":{"ip":"10.0.0.1"},"scenarios":[%d,%d]`, sa, sb)},
		{"token PATCH still validated", "PATCH", minePath, `{"scenarios":[999999]}`, tok.Token, 400, `unknown scenario ids: 999999`},
		{"malformed body reaches the decoder", "PATCH", minePath, `{"enabled":`, tok.Token, 400, `Malformed JSON`},
		{"token cannot delete", "DELETE", minePath, ``, tok.Token, 401, `Not logged in`},
		{"token cannot list", "GET", "/api/client", ``, tok.Token, 401, `Not logged in`},
	}
	for _, tc := range cases {
		rec := e.doToken(tc.method, tc.path, tc.body, tc.token, nil)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	// A token wins over a session: with a valid session but another client's token, 403.
	if rec := e.doToken("PATCH", otherPath, `{"enabled":false}`, tok.Token, c); rec.Code != 403 {
		t.Errorf("token with session: %d", rec.Code)
	}
	// Without the header the session guard applies.
	if rec := e.do("PATCH", otherPath, `{"name":"renamed"}`, nil); rec.Code != 401 {
		t.Errorf("no token, no session: %d", rec.Code)
	}
	if rec := e.do("PATCH", otherPath, `{"name":"renamed"}`, c); rec.Code != 200 {
		t.Errorf("session without token: %d %s", rec.Code, rec.Body.String())
	}
}

func TestClientEndpointsNeedSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/client", ""},
		{"POST", "/api/client", `{"name":"x"}`},
		{"GET", "/api/client/1", ""},
		{"PATCH", "/api/client/1", `{"name":"x"}`},
		{"DELETE", "/api/client/1", ""},
		{"POST", "/api/client/disable-all", ""},
		{"POST", "/api/client/1/token", ""},
		{"DELETE", "/api/client/1/token", ""},
	} {
		if rec := e.do(r.method, r.path, r.body, nil); rec.Code != 401 {
			t.Errorf("%s %s without session: %d", r.method, r.path, rec.Code)
		}
	}
}
