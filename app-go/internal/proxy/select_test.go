package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/expr"
)

func mustExpr(src string) *expr.Expr {
	e, err := expr.Parse(src)
	if err != nil {
		panic(err)
	}
	return e
}

// fakeEnv is a conditionEnv that records whether the body was asked for.
type fakeEnv struct {
	req       expr.Request
	err       error
	calls     int
	bodyAsked bool
}

func (f *fakeEnv) Request(needBody bool) (*expr.Request, error) {
	f.calls++
	f.bodyAsked = f.bodyAsked || needBody
	return &f.req, f.err
}

func sp(s string) *string { return &s }

func client(id int64, name string, scenarios ...int64) *catalog.Client {
	set := map[int64]bool{}
	for _, s := range scenarios {
		set[s] = true
	}
	return &catalog.Client{ID: id, Name: name, Scenarios: set}
}

// Spec §7.5: order, the method and path filters, case-insensitivity and the reported
// client.
func TestSelectScenario(t *testing.T) {
	// In name order, as the catalog holds them.
	svc := &catalog.Service{Path: "users", Scenarios: []catalog.Scenario{
		{ID: 1, Name: "a-orders-get", RequestMethod: sp("GET"), RequestPath: sp("Orders/42")},
		{ID: 2, Name: "b-orders-any", RequestPath: sp("orders/42")},
		{ID: 3, Name: "c-any"},
		{ID: 4, Name: "d-with-condition", Condition: mustExpr("$request.method == 'GET'")},
		{ID: 5, Name: "e-post", RequestMethod: sp("POST")},
	}}
	cases := []struct {
		name         string
		clients      []*catalog.Client
		method, path string
		scenario     string // "" for none
		client       string // "" for none
	}{
		{"no clients", nil, "GET", "orders/42", "", ""},
		{"first in name order", []*catalog.Client{client(1, "qa", 1, 2, 3)}, "GET", "orders/42", "a-orders-get", "qa"},
		{"path case-insensitive", []*catalog.Client{client(1, "qa", 1)}, "GET", "ORDERS/42", "a-orders-get", "qa"},
		{"method filter", []*catalog.Client{client(1, "qa", 1, 2)}, "PUT", "orders/42", "b-orders-any", "qa"},
		{"method is case-sensitive", []*catalog.Client{client(1, "qa", 1)}, "get", "orders/42", "", "qa"},
		{"path filter", []*catalog.Client{client(1, "qa", 1, 2, 3)}, "GET", "orders/43", "c-any", "qa"},
		{"empty path", []*catalog.Client{client(1, "qa", 1, 2)}, "GET", "", "", "qa"},
		{"unassigned is skipped", []*catalog.Client{client(1, "qa", 3)}, "GET", "orders/42", "c-any", "qa"},
		{"request condition true", []*catalog.Client{client(1, "qa", 4)}, "GET", "x", "d-with-condition", "qa"},
		{"request condition false", []*catalog.Client{client(1, "qa", 4)}, "PUT", "x", "", "qa"},
		{"request condition false: next scenario", []*catalog.Client{client(1, "qa", 4, 5)}, "POST", "x", "e-post", "qa"},
		{"later scenario", []*catalog.Client{client(1, "qa", 5)}, "POST", "x", "e-post", "qa"},
		{"nothing fits: first client", []*catalog.Client{client(1, "qa", 5), client(2, "dev")}, "GET", "x", "", "qa"},
		{"reported: first client with the winner", []*catalog.Client{client(1, "qa", 5), client(2, "dev", 3), client(3, "ci", 3)}, "GET", "x", "c-any", "dev"},
		{"any client's assignment counts", []*catalog.Client{client(1, "qa", 3), client(2, "dev", 1)}, "GET", "orders/42", "a-orders-get", "dev"},
	}
	for _, tc := range cases {
		env := &fakeEnv{req: expr.Request{Method: tc.method}}
		sel, err := selectScenario(svc, tc.clients, tc.method, tc.path, env)
		if err != nil {
			t.Fatal(err)
		}
		var gotScenario, gotClient string
		if sel.scenario != nil {
			gotScenario = sel.scenario.Name
		}
		if sel.client != nil {
			gotClient = sel.client.Name
		}
		if gotScenario != tc.scenario || gotClient != tc.client {
			t.Errorf("%s: scenario %q client %q, want %q %q", tc.name, gotScenario, gotClient, tc.scenario, tc.client)
		}
	}
}

// Spec §7.5, §7.7: a request condition is evaluated only for a scenario that passes the
// other filters, and the body is asked for only by a condition that needs it.
func TestSelectConditionLazily(t *testing.T) {
	svc := &catalog.Service{Path: "users", Scenarios: []catalog.Scenario{
		{ID: 1, Name: "a-body", RequestMethod: sp("POST"), Condition: mustExpr("$request.body.id == 1")},
		{ID: 2, Name: "b-headers", Condition: mustExpr("$request.headers.x == 'y'")},
		{ID: 3, Name: "c-plain"},
	}}
	qa := []*catalog.Client{client(1, "qa", 1, 2, 3)}

	env := &fakeEnv{}
	if sel, _ := selectScenario(svc, qa, "GET", "", env); sel.scenario.Name != "c-plain" || env.bodyAsked || env.calls != 1 {
		t.Errorf("GET: %s, body asked %v, calls %d", sel.scenario.Name, env.bodyAsked, env.calls)
	}
	env = &fakeEnv{}
	if sel, _ := selectScenario(svc, []*catalog.Client{client(1, "qa", 3)}, "POST", "", env); sel.scenario.Name != "c-plain" || env.calls != 0 {
		t.Errorf("unassigned: %s, calls %d", sel.scenario.Name, env.calls)
	}
	env = &fakeEnv{req: expr.Request{Body: map[string]any{"id": 1.0}}}
	if sel, _ := selectScenario(svc, qa, "POST", "", env); sel.scenario.Name != "a-body" || !env.bodyAsked {
		t.Errorf("POST: %s, body asked %v", sel.scenario.Name, env.bodyAsked)
	}

	boom := errors.New("boom")
	env = &fakeEnv{err: boom}
	sel, err := selectScenario(svc, qa, "POST", "", env)
	if !errors.Is(err, boom) || sel.scenario != nil || sel.client.Name != "qa" {
		t.Errorf("error: %v %+v", err, sel)
	}
}

// Spec §7.5 step 2: the path without the query and without leading or trailing "/".
func TestRequestPath(t *testing.T) {
	var got string
	mux := http.NewServeMux() // the proxy's patterns
	record := func(w http.ResponseWriter, r *http.Request) { got = requestPath(r) }
	mux.HandleFunc("/api/proxy/{service}", record)
	mux.HandleFunc("/api/proxy/{service}/{rest...}", record)
	for target, want := range map[string]string{
		"/api/proxy/users/orders/42/?page=2": "orders/42",
		"/api/proxy/users/orders/42":         "orders/42",
		"/api/proxy/users/":                  "",
		"/api/proxy/users":                   "",
		"/api/proxy/users/a%20b/":            "a b",
	} {
		got = "unset"
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", target, nil))
		if got != want {
			t.Errorf("%s: %q, want %q", target, got, want)
		}
	}
}
