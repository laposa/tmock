package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/liquid"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Spec §15.3 I13: end-to-end proxy tests that combine clients, conditions, request
// expressions and templates. Both sides are real HTTP servers: the client talks to the
// proxy over a socket, and the proxy to an upstream that echoes what it received. Every
// case is run once in order (with the log checked) and then many times concurrently, so
// that the race detector sees any request-scoped value that leaks into the shared
// snapshot.

// e2eEcho is the JSON the e2e upstream answers with.
type e2eEcho struct {
	Method        string `json:"method"`
	URI           string `json:"uri"`
	Body          string `json:"body"`
	ContentLength string `json:"contentLength"`
}

// e2eUpstream echoes each request and remembers the X-Req-Id of every request it got, so
// a case can tell whether the upstream was contacted even when its answer was replaced.
type e2eUpstream struct {
	*httptest.Server
	seen sync.Map // X-Req-Id → true
}

func newE2EUpstream(t *testing.T) *e2eUpstream {
	u := &e2eUpstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.seen.Store(r.Header.Get("X-Req-Id"), true)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", "yes")
		w.Header().Set("X-Upstream-Only", "yes")
		_ = json.NewEncoder(w).Encode(e2eEcho{Method: r.Method, URI: r.RequestURI, Body: string(body),
			ContentLength: r.Header.Get("Content-Length")})
	}))
	t.Cleanup(u.Close)
	return u
}

// vipTemplate is a skip-proxy body that uses assign, a for loop over a range, forloop,
// unless, math filters and the §8.3 date example.
const vipTemplate = liquid.Marker + `{% assign step = 10 %}{"tier":"vip","items":[` +
	`{% for i in (1..3) %}{{ i | times: step }}{% unless forloop.last %},{% endunless %}{% endfor %}],` +
	`"validFrom":"{{ 'now' | date: "%s" | plus: 432000 | date: "%Y-%m-%d" }}"}`

// e2eCatalog is the service "shop" on upstream/v1 with six scenarios (in name order, as
// the database returns them) and three clients:
//
//   - qa (1): 10.0.0.1, or the header X-Env: qa; has a–d and f;
//   - mobile (2): a User-Agent of MobileApp/<n>, except from 192.168.0.0/16; has c and e;
//   - lab (3): 2001:db8::/32; has none.
func e2eCatalog(upstream string) staticCatalog {
	scenarios := []store.Scenario{
		{ID: 1, Name: "a-maintenance", RequestMethod: sp("GET"), RequestPath: sp("health"), SkipProxy: true,
			ResponseCode: ip(503), ResponseBody: sp(`{"status":"maintenance"}`), ResponseHeaders: map[string]*string{"Retry-After": sp("120")}},
		{ID: 2, Name: "b-vip", RequestMethod: sp("POST"), RequestPath: sp("checkout"),
			RequestCondition: sp(`$request.body.tier == "vip" && $request.body.total >= 100`),
			SkipProxy:        true, ResponseCode: ip(201), ResponseBody: sp(vipTemplate)},
		{ID: 3, Name: "c-profile", RequestPath: sp("profile"),
			ResponseHeaders: map[string]*string{"X-Mock": sp("profile"), "X-Upstream-Only": nil}},
		{ID: 4, Name: "d-orders", RequestPath: sp("orders"),
			RequestCondition: sp(`$request.query.page > "1" || $request.headers["x-debug"] == "on"`),
			ResponseBody:     sp(liquid.Marker + `{"orders":[],"year":{{ 'now' | date: "%Y" }}}`)},
		{ID: 5, Name: "e-teapot", RequestMethod: sp("DELETE"), ResponseCode: ip(418)},
		{ID: 6, Name: "f-broken", RequestPath: sp("broken"), SkipProxy: true,
			ResponseBody: sp(liquid.Marker + `{{ 1 | divided_by: 0 }}`)},
	}
	data := store.CatalogData{
		Services: []store.CatalogService{{Service: store.Service{ID: 1, Path: "shop", Name: "Shop", UpstreamURL: upstream + "/v1"}, Scenarios: scenarios}},
		Clients: []store.CatalogClient{
			{ID: 1, Name: "qa", Condition: []byte(`{"or":[{"ip":"10.0.0.1"},{"headerMatch":{"header":"X-Env","value":"qa"}}]}`), Scenarios: []int64{1, 2, 3, 4, 6}},
			{ID: 2, Name: "mobile", Condition: []byte(`{"and":[{"headerRegex":{"header":"User-Agent","value":"^MobileApp/\\d+"}},{"cidr":"192.168.0.0/16","not":true}]}`), Scenarios: []int64{3, 5}},
			{ID: 3, Name: "lab", Condition: []byte(`{"cidr":"2001:db8::/32"}`)},
		},
	}
	return staticCatalog{snap: catalog.Build(data, nil, nil)}
}

// e2eCase is one request and what the client should get back.
type e2eCase struct {
	name                  string
	method, path          string
	header                []string // name, value, …
	body, contentType     string
	status                int
	tmockStatus           string
	client, scenario      string
	upstream              bool              // whether the upstream was contacted
	echo                  *e2eEcho          // when set, the body is the upstream's echo, which must match
	wantBody              func(string) bool // when set, checks the body tmock wrote
	header1, header1Value string            // a response header and its value ("" = absent)
	header2, header2Value string
	logContains           string // a fragment of the proxy log line
	renderFailed          bool   // whether tmock.template.render_failed is logged
	upstreamContentLength bool   // the upstream got the body with a Content-Length
}

const qaIP, otherIP = "10.0.0.1", "10.9.9.9"

func e2eCases() []e2eCase {
	jsonCT := "application/json"
	isVIP := func(b string) bool {
		// 'now' + 5 days, in the local zone; either side of midnight.
		for _, d := range []time.Time{time.Now(), time.Now().Add(-time.Minute)} {
			want := `{"tier":"vip","items":[10,20,30],"validFrom":"` + d.AddDate(0, 0, 5).Format("2006-01-02") + `"}`
			if b == want {
				return true
			}
		}
		return false
	}
	isOrders := func(b string) bool {
		y := time.Now().Year()
		return b == fmt.Sprintf(`{"orders":[],"year":%d}`, y) || b == fmt.Sprintf(`{"orders":[],"year":%d}`, y-1)
	}
	return []e2eCase{
		{name: "no client: passthrough", method: "GET", path: "/health?x=1", header: []string{"X-Real-Ip", otherIP},
			status: 200, tmockStatus: StatusProxied, upstream: true, echo: &e2eEcho{Method: "GET", URI: "/v1/health?x=1"},
			header1: "X-Upstream-Only", header1Value: "yes", logContains: " GET /api/proxy/shop/health?x=1 200 "},
		{name: "qa by ip: skip proxy", method: "GET", path: "/health", header: []string{"X-Real-Ip", qaIP},
			status: 503, tmockStatus: StatusMocked, client: "qa", scenario: "a-maintenance",
			wantBody: func(b string) bool { return b == `{"status":"maintenance"}` },
			header1:  "Retry-After", header1Value: "120", header2: "Content-Type", header2Value: jsonCT,
			logContains: " 503 [client qa; scenario a-maintenance; mocked: status, headers, body; proxy skipped] "},
		{name: "path is case-insensitive, slashes trimmed", method: "GET", path: "/HEALTH/", header: []string{"X-Real-Ip", qaIP},
			status: 503, tmockStatus: StatusMocked, client: "qa", scenario: "a-maintenance",
			wantBody: func(b string) bool { return b == `{"status":"maintenance"}` }},
		{name: "qa by header: method filter, no scenario", method: "POST", path: "/health", header: []string{"X-Real-Ip", otherIP, "X-Env", "qa"},
			body: "ping", contentType: "text/plain", status: 200, tmockStatus: StatusProxied, client: "qa", upstream: true,
			echo: &e2eEcho{Method: "POST", URI: "/v1/health", Body: "ping"}, logContains: " 200 [client qa] "},
		{name: "body condition true: template", method: "POST", path: "/checkout", header: []string{"X-Real-Ip", qaIP},
			body: `{"tier":"vip","total":150}`, contentType: jsonCT,
			status: 201, tmockStatus: StatusMocked, client: "qa", scenario: "b-vip", wantBody: isVIP,
			header1: "Content-Type", header1Value: jsonCT},
		{name: "body condition false: body forwarded unchanged", method: "POST", path: "/checkout", header: []string{"X-Real-Ip", qaIP},
			body: `{"tier":"vip","total":50}`, contentType: jsonCT, status: 200, tmockStatus: StatusProxied, client: "qa", upstream: true,
			echo: &e2eEcho{Method: "POST", URI: "/v1/checkout", Body: `{"tier":"vip","total":50}`}, upstreamContentLength: true},
		// A form body is a map of strings: "150" >= 100 compares a string with a number,
		// which is false (spec §7.4).
		{name: "form body: string is not a number", method: "POST", path: "/checkout", header: []string{"X-Real-Ip", qaIP},
			body: "tier=vip&total=150", contentType: "application/x-www-form-urlencoded", status: 200, tmockStatus: StatusProxied,
			client: "qa", upstream: true, echo: &e2eEcho{Method: "POST", URI: "/v1/checkout", Body: "tier=vip&total=150"}},
		{name: "query condition: upstream body replaced by a template", method: "GET", path: "/orders?page=2", header: []string{"X-Real-Ip", qaIP},
			status: 200, tmockStatus: StatusMocked, client: "qa", scenario: "d-orders", upstream: true, wantBody: isOrders,
			header1: "X-Upstream", header1Value: "yes", header2: "Content-Type", header2Value: jsonCT,
			logContains: " 200 [client qa; scenario d-orders; mocked: body] "},
		{name: "query condition false", method: "GET", path: "/orders?page=1", header: []string{"X-Real-Ip", qaIP},
			status: 200, tmockStatus: StatusProxied, client: "qa", upstream: true, echo: &e2eEcho{Method: "GET", URI: "/v1/orders?page=1"}},
		{name: "header condition", method: "GET", path: "/orders", header: []string{"X-Real-Ip", qaIP, "X-Debug", "on"},
			status: 200, tmockStatus: StatusMocked, client: "qa", scenario: "d-orders", upstream: true, wantBody: isOrders},
		{name: "mobile: headers only, upstream body kept", method: "GET", path: "/profile", header: []string{"X-Real-Ip", "10.0.0.5", "User-Agent", "MobileApp/7"},
			status: 200, tmockStatus: StatusMocked, client: "mobile", scenario: "c-profile", upstream: true,
			echo:    &e2eEcho{Method: "GET", URI: "/v1/profile"},
			header1: "X-Mock", header1Value: "profile", header2: "X-Upstream-Only", header2Value: "",
			logContains: "[client mobile; scenario c-profile; mocked: headers] "},
		{name: "mobile excluded by not cidr", method: "GET", path: "/profile", header: []string{"X-Real-Ip", "192.168.1.4", "User-Agent", "MobileApp/7"},
			status: 200, tmockStatus: StatusProxied, upstream: true, echo: &e2eEcho{Method: "GET", URI: "/v1/profile"},
			header1: "X-Upstream-Only", header1Value: "yes"},
		{name: "two clients: first one with the winner is reported", method: "GET", path: "/profile", header: []string{"X-Real-Ip", qaIP, "User-Agent", "MobileApp/7"},
			status: 200, tmockStatus: StatusMocked, client: "qa", scenario: "c-profile", upstream: true, echo: &e2eEcho{Method: "GET", URI: "/v1/profile"}},
		{name: "two clients: winner only assigned to the second", method: "DELETE", path: "/items/9", header: []string{"X-Real-Ip", qaIP, "User-Agent", "MobileApp/7"},
			status: 418, tmockStatus: StatusMocked, client: "mobile", scenario: "e-teapot", upstream: true,
			echo: &e2eEcho{Method: "DELETE", URI: "/v1/items/9"}, logContains: " 418 [client mobile; scenario e-teapot; mocked: status] "},
		{name: "ipv6 client without scenarios", method: "GET", path: "/profile", header: []string{"X-Real-Ip", "2001:db8::1"},
			status: 200, tmockStatus: StatusProxied, client: "lab", upstream: true, echo: &e2eEcho{Method: "GET", URI: "/v1/profile"}},
		{name: "template fails at run time: body as stored", method: "GET", path: "/broken", header: []string{"X-Real-Ip", qaIP},
			status: 200, tmockStatus: StatusMocked, client: "qa", scenario: "f-broken", renderFailed: true,
			wantBody: func(b string) bool { return b == liquid.Marker+`{{ 1 | divided_by: 0 }}` },
			header1:  "Content-Type", header1Value: "text/plain; charset=utf-8"},
		{name: "spoofed tmock headers are replaced", method: "GET", path: "/health", header: []string{"X-Real-Ip", otherIP, "X-Tmock-Scenario", "fake", "X-Tmock-Status", "mocked"},
			status: 200, tmockStatus: StatusProxied, upstream: true, echo: &e2eEcho{Method: "GET", URI: "/v1/health"}},
	}
}

// run sends the case's request through the proxy server and checks the answer. It
// returns problems instead of failing, so it can run from many goroutines.
func (tc e2eCase) run(proxyURL string, up *e2eUpstream, id string) []string {
	var problems []string
	fail := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	var body io.Reader
	if tc.body != "" {
		body = strings.NewReader(tc.body)
	}
	req, err := http.NewRequest(tc.method, proxyURL+"/api/proxy/shop"+tc.path, body)
	if err != nil {
		return []string{err.Error()}
	}
	req.Header.Set("X-Req-Id", id)
	if tc.contentType != "" {
		req.Header.Set("Content-Type", tc.contentType)
	}
	for i := 0; i+1 < len(tc.header); i += 2 {
		req.Header.Set(tc.header[i], tc.header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []string{err.Error()}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		fail("reading the body: %v", err)
	}
	got := string(b)

	if resp.StatusCode != tc.status {
		fail("status %d, want %d (body %q)", resp.StatusCode, tc.status, got)
	}
	h := resp.Header
	if h.Get(HeaderStatus) != tc.tmockStatus || h.Get(HeaderClient) != tc.client || h.Get(HeaderScenario) != tc.scenario {
		fail("x-tmock-* = %q %q %q, want %q %q %q", h.Get(HeaderStatus), h.Get(HeaderClient), h.Get(HeaderScenario),
			tc.tmockStatus, tc.client, tc.scenario)
	}
	if _, ok := up.seen.Load(id); ok != tc.upstream {
		fail("upstream contacted: %v, want %v", ok, tc.upstream)
	}
	for _, hv := range [][2]string{{tc.header1, tc.header1Value}, {tc.header2, tc.header2Value}} {
		if hv[0] != "" && h.Get(hv[0]) != hv[1] {
			fail("%s = %q, want %q", hv[0], h.Get(hv[0]), hv[1])
		}
	}
	switch {
	case tc.echo != nil:
		var e e2eEcho
		if err := json.Unmarshal(b, &e); err != nil {
			fail("not an echo: %q", got)
			break
		}
		if e.Method != tc.echo.Method || e.URI != tc.echo.URI || e.Body != tc.echo.Body {
			fail("upstream saw %+v, want %+v", e, *tc.echo)
		}
		if tc.upstreamContentLength && e.ContentLength != strconv.Itoa(len(tc.body)) {
			fail("upstream Content-Length %q, want %d", e.ContentLength, len(tc.body))
		}
	case tc.wantBody != nil:
		if !tc.wantBody(got) {
			fail("body %q", got)
		}
		if cl := h.Get("Content-Length"); cl != strconv.Itoa(len(b)) {
			fail("Content-Length %q for %d bytes", cl, len(b))
		}
	}
	return problems
}

func TestEndToEnd(t *testing.T) {
	up := newE2EUpstream(t)
	logs, logger := logxtest.New()
	p := New(Options{Catalog: e2eCatalog(up.URL), Logger: logger})
	srv := httptest.NewServer(p)
	defer srv.Close()
	cases := e2eCases()

	for i, tc := range cases {
		before := len(logs.Events(logx.EventTemplateFailed))
		for _, pr := range tc.run(srv.URL, up, "seq-"+strconv.Itoa(i)) {
			t.Errorf("%s: %s", tc.name, pr)
		}
		ev := logs.Events(logx.EventProxyRequest)
		if len(ev) != i+1 {
			t.Fatalf("%s: %d proxy.request events, want %d", tc.name, len(ev), i+1)
		}
		if line := ev[i].Message; tc.logContains != "" && !strings.Contains(line, tc.logContains) {
			t.Errorf("%s: log line %q lacks %q", tc.name, line, tc.logContains)
		}
		if failed := len(logs.Events(logx.EventTemplateFailed)) - before; failed != map[bool]int{true: 1}[tc.renderFailed] {
			t.Errorf("%s: %d render_failed events", tc.name, failed)
		}
	}
	if t.Failed() {
		return
	}

	// The same requests, many at a time: the outcome must not depend on what runs
	// alongside (go test -race checks the shared snapshot is only read).
	const rounds = 20
	var wg sync.WaitGroup
	var failures atomic.Int32
	for r := range rounds {
		for i, tc := range cases {
			wg.Go(func() {
				for _, pr := range tc.run(srv.URL, up, fmt.Sprintf("par-%d-%d", r, i)) {
					if failures.Add(1) <= 10 {
						t.Errorf("concurrent %s: %s", tc.name, pr)
					}
				}
			})
		}
	}
	wg.Wait()
	if n := len(logs.Events(logx.EventProxyRequest)); n != len(cases)*(rounds+1) {
		t.Errorf("%d proxy.request events, want %d", n, len(cases)*(rounds+1))
	}
}
