package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

func ip(p int) *int { return &p }

// fakeUpstream answers with a gzip body, an ETag and a few headers, and counts requests.
type fakeUpstream struct {
	*httptest.Server
	calls atomic.Int32
	last  atomic.Pointer[http.Header]
}

var upstreamBody = func() []byte {
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	_, _ = zw.Write([]byte(`{"from":"upstream"}`))
	_ = zw.Close()
	return b.Bytes()
}()

func newFakeUpstream(t *testing.T, contentType string) *fakeUpstream {
	u := &fakeUpstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		h := r.Header.Clone()
		u.last.Store(&h)
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Etag", `"v1"`)
		w.Header().Set("X-Keep", "upstream")
		w.Header().Set("X-Drop", "upstream")
		w.Header().Set("X-Replace", "upstream")
		_, _ = w.Write(upstreamBody)
	}))
	t.Cleanup(u.Close)
	return u
}

// mockCatalog has the service "svc" on upstream with the given scenarios, and the
// clients "qa" (ip 10.0.0.1) and "dev" (header X-Env: dev), with qa assigned to every
// scenario.
func mockCatalog(upstream string, scenarios ...store.Scenario) staticCatalog {
	var ids []int64
	for i := range scenarios {
		scenarios[i].ID = int64(i + 1)
		ids = append(ids, scenarios[i].ID)
	}
	data := store.CatalogData{
		Services: []store.CatalogService{{Service: store.Service{ID: 1, Path: "svc", Name: "Svc", UpstreamURL: upstream}, Scenarios: scenarios}},
		Clients: []store.CatalogClient{
			{ID: 1, Name: "qa", Condition: []byte(`{"ip":"10.0.0.1"}`), Scenarios: ids},
			{ID: 2, Name: "dev", Condition: []byte(`{"headerMatch":{"header":"X-Env","value":"dev"}}`)},
		},
	}
	return staticCatalog{snap: catalog.Build(data, nil, slog.New(slog.DiscardHandler))}
}

func TestSkipProxy(t *testing.T) {
	up := newFakeUpstream(t, "")
	logs, logger := logxtest.New()
	p := New(Options{Catalog: mockCatalog(up.URL, store.Scenario{
		Name: "down", ResponseCode: ip(503), SkipProxy: true, ResponseBody: sp(`{"error":"down"}`),
		ResponseHeaders: map[string]*string{"X-Mock": sp("1"), "X-Null": nil, "X-Tmock-Status": sp("spoofed")},
	}), Logger: logger})

	rec := do(p, "GET", "/api/proxy/svc/a?b=1", nil, "X-Real-Ip", "10.0.0.1")
	if up.calls.Load() != 0 {
		t.Error("the upstream was contacted")
	}
	h := rec.Header()
	if rec.Code != 503 || rec.Body.String() != `{"error":"down"}` || h.Get("X-Mock") != "1" ||
		h.Get("Content-Type") != "application/json" || h.Get("Content-Length") != "16" {
		t.Errorf("got %d %q %v", rec.Code, rec.Body, h)
	}
	if _, ok := h["X-Null"]; ok {
		t.Error("a null header was set")
	}
	if h.Get(HeaderStatus) != StatusMocked || h.Get(HeaderClient) != "qa" || h.Get(HeaderScenario) != "down" {
		t.Errorf("x-tmock-* = %q %q %q", h.Get(HeaderStatus), h.Get(HeaderClient), h.Get(HeaderScenario))
	}

	ev := logs.Events(logx.EventProxyRequest)
	if len(ev) != 1 {
		t.Fatalf("events %+v", logs.All())
	}
	r := ev[0]
	if !strings.HasPrefix(r.Message, "10.0.0.1 GET /api/proxy/svc/a?b=1 503 [client qa; scenario down; mocked: status, headers, body; proxy skipped] ") ||
		r.Attrs[logx.AttrClient] != "qa" || r.Attrs[logx.AttrScenario] != "down" ||
		r.Attrs[logx.AttrMocked] != "[status headers body]" || r.Attrs[logx.AttrProxySkipped] != "true" {
		t.Errorf("record %+v", r)
	}
}

// Spec §7.6: status 200 when unset; the content-type fallback for a body that is not JSON;
// a Content-Type of the scenario's wins.
func TestSkipProxyDefaults(t *testing.T) {
	p := New(Options{Catalog: mockCatalog("http://upstream.invalid",
		store.Scenario{Name: "a", RequestPath: sp("text"), SkipProxy: true, ResponseBody: sp("plain text")},
		store.Scenario{Name: "b", RequestPath: sp("html"), SkipProxy: true, ResponseBody: sp("<p>"), ResponseHeaders: map[string]*string{"content-type": sp("text/html")}},
		store.Scenario{Name: "c", RequestPath: sp("empty"), SkipProxy: true, ResponseBody: sp(""), ResponseCode: ip(204)},
	), Logger: slog.New(slog.DiscardHandler)})
	rec := do(p, "GET", "/api/proxy/svc/text", nil, "X-Real-Ip", "10.0.0.1")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" || rec.Body.String() != "plain text" {
		t.Errorf("text: %d %v %q", rec.Code, rec.Header(), rec.Body)
	}
	rec = do(p, "GET", "/api/proxy/svc/html", nil, "X-Real-Ip", "10.0.0.1")
	if rec.Header().Get("Content-Type") != "text/html" {
		t.Errorf("html: %v", rec.Header())
	}
	rec = do(p, "GET", "/api/proxy/svc/empty", nil, "X-Real-Ip", "10.0.0.1")
	if rec.Code != 204 || rec.Header().Get("Content-Length") != "" || rec.Body.Len() != 0 {
		t.Errorf("204: %d %v %q", rec.Code, rec.Header(), rec.Body)
	}
}

// Spec §7.8: the scenario's headers over the upstream's (null deletes), its status, its
// body without ETag, Content-Encoding and Content-Length, and the x-tmock-* headers; the
// upstream request carries x-tmock-status: mocked.
func TestModifyUpstreamResponse(t *testing.T) {
	up := newFakeUpstream(t, "")
	logs, logger := logxtest.New()
	p := New(Options{Catalog: mockCatalog(up.URL, store.Scenario{
		Name: "broken", ResponseCode: ip(500), ResponseBody: sp("oops"),
		ResponseHeaders: map[string]*string{"X-Drop": nil, "X-Replace": sp("scenario"), "X-New": sp("1")},
	}), Logger: logger})

	rec := do(p, "POST", "/api/proxy/svc/x", strings.NewReader("in"), "X-Real-Ip", "10.0.0.1", "Accept-Encoding", "gzip")
	if up.calls.Load() != 1 {
		t.Fatalf("upstream calls = %d", up.calls.Load())
	}
	sent := *up.last.Load()
	if sent.Get(HeaderStatus) != StatusMocked || sent.Get(HeaderClient) != "qa" || sent.Get(HeaderScenario) != "broken" {
		t.Errorf("upstream request x-tmock-* = %v", sent)
	}
	h := rec.Header()
	if rec.Code != 500 || rec.Body.String() != "oops" {
		t.Errorf("got %d %q", rec.Code, rec.Body)
	}
	for k, want := range map[string]string{
		"X-Keep": "upstream", "X-Replace": "scenario", "X-New": "1", "X-Drop": "", "Etag": "", "Content-Encoding": "",
		"Content-Type": "text/plain; charset=utf-8", HeaderStatus: StatusMocked, HeaderClient: "qa", HeaderScenario: "broken",
	} {
		if got := h.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	ev := logs.Events(logx.EventProxyRequest)
	if len(ev) != 1 || !strings.Contains(ev[0].Message, " 500 [client qa; scenario broken; mocked: status, headers, body] ") ||
		ev[0].Attrs[logx.AttrProxySkipped] != "false" {
		t.Errorf("events %+v", ev)
	}
}

// Without a replaced body, the upstream's body is streamed untouched (still compressed),
// and its Content-Type is kept.
func TestModifyKeepsUpstreamBody(t *testing.T) {
	up := newFakeUpstream(t, "application/json")
	p := New(Options{Catalog: mockCatalog(up.URL, store.Scenario{Name: "teapot", ResponseCode: ip(418)}),
		Logger: slog.New(slog.DiscardHandler)})
	rec := do(p, "GET", "/api/proxy/svc/", nil, "X-Real-Ip", "10.0.0.1", "Accept-Encoding", "gzip")
	if rec.Code != 418 || !bytes.Equal(rec.Body.Bytes(), upstreamBody) ||
		rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Etag") != `"v1"` ||
		rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get(HeaderStatus) != StatusMocked {
		t.Errorf("got %d %v", rec.Code, rec.Header())
	}
}

// A body that replaces the upstream's: the upstream's Content-Type stays (step 4 only
// fills a missing one).
func TestReplacedBodyKeepsContentType(t *testing.T) {
	up := newFakeUpstream(t, "text/html")
	p := New(Options{Catalog: mockCatalog(up.URL, store.Scenario{Name: "s", ResponseBody: sp(`{"a":1}`)}),
		Logger: slog.New(slog.DiscardHandler)})
	rec := do(p, "GET", "/api/proxy/svc/", nil, "X-Real-Ip", "10.0.0.1")
	if rec.Code != 200 || rec.Body.String() != `{"a":1}` || rec.Header().Get("Content-Type") != "text/html" {
		t.Errorf("got %d %q %v", rec.Code, rec.Body, rec.Header())
	}
}

// A client matched but no scenario applies: proxied, with x-tmock-client on the request
// and the response.
func TestClientWithoutScenario(t *testing.T) {
	up := newFakeUpstream(t, "application/json")
	logs, logger := logxtest.New()
	p := New(Options{Catalog: mockCatalog(up.URL, store.Scenario{Name: "only-post", RequestMethod: sp("POST"), ResponseCode: ip(500)}),
		Logger: logger})
	rec := do(p, "GET", "/api/proxy/svc/", nil, "X-Env", "dev")
	sent := *up.last.Load()
	if sent.Get(HeaderStatus) != StatusProxied || sent.Get(HeaderClient) != "dev" || sent.Get(HeaderScenario) != "" {
		t.Errorf("upstream request: %v", sent)
	}
	if rec.Code != 200 || rec.Header().Get(HeaderStatus) != StatusProxied || rec.Header().Get(HeaderClient) != "dev" ||
		rec.Header().Get(HeaderScenario) != "" {
		t.Errorf("response: %d %v", rec.Code, rec.Header())
	}
	ev := logs.Events(logx.EventProxyRequest)
	if len(ev) != 1 || !strings.Contains(ev[0].Message, " 200 [client dev] ") || ev[0].Attrs[logx.AttrClient] != "dev" ||
		ev[0].Attrs[logx.AttrScenario] != "" {
		t.Errorf("events %+v", ev)
	}

	// No client at all: no bracket, no client headers.
	rec = do(p, "GET", "/api/proxy/svc/", nil, "X-Real-Ip", "10.9.9.9")
	if rec.Header().Get(HeaderClient) != "" || strings.Contains(logs.Events(logx.EventProxyRequest)[1].Message, "[") {
		t.Errorf("unmatched: %v", rec.Header())
	}
}

// A client or scenario name with a line break cannot fail the request.
func TestNamesAreHeaderSafe(t *testing.T) {
	up := newFakeUpstream(t, "")
	data := store.CatalogData{
		Services: []store.CatalogService{{Service: store.Service{ID: 1, Path: "svc", Name: "S", UpstreamURL: up.URL},
			Scenarios: []store.Scenario{{ID: 1, Name: "two\nlines", ResponseCode: ip(201)}}}},
		Clients: []store.CatalogClient{{ID: 1, Name: "a\r\nb", Condition: []byte(`{"cidr":"0.0.0.0/0"}`), Scenarios: []int64{1}}},
	}
	p := New(Options{Catalog: staticCatalog{snap: catalog.Build(data, nil, nil)}, Logger: slog.New(slog.DiscardHandler)})
	rec := do(p, "GET", "/api/proxy/svc/", nil)
	if rec.Code != 201 || rec.Header().Get(HeaderClient) != "a  b" || rec.Header().Get(HeaderScenario) != "two lines" {
		t.Errorf("got %d %v", rec.Code, rec.Header())
	}
}

// Spec §9.5: a preflight is answered with 204 and never forwarded; other responses,
// mocked or proxied, carry the reflected origin; the upstream's own CORS headers win.
func TestProxyCORS(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/own" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
	}))
	defer up.Close()
	logs, logger := logxtest.New()
	p := New(Options{Catalog: mockCatalog(up.URL, store.Scenario{Name: "m", RequestPath: sp("mock"), SkipProxy: true, ResponseBody: sp("x")}),
		Logger: logger})

	rec := do(p, "OPTIONS", "/api/proxy/svc/a", nil, "Origin", "https://app.example",
		"Access-Control-Request-Method", "POST", "Access-Control-Request-Headers", "x-custom")
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" ||
		rec.Header().Get("Access-Control-Allow-Methods") != "GET,HEAD,PUT,PATCH,POST,DELETE" ||
		rec.Header().Get("Access-Control-Allow-Headers") != "x-custom" || calls.Load() != 0 {
		t.Errorf("preflight: %d %v (upstream calls %d)", rec.Code, rec.Header(), calls.Load())
	}
	if n := len(logs.Events(logx.EventProxyRequest)); n != 1 {
		t.Errorf("preflight logged %d times", n)
	}

	for _, target := range []string{"/api/proxy/svc/proxied", "/api/proxy/svc/mock", "/api/proxy/nope"} {
		rec = do(p, "GET", target, nil, "Origin", "https://app.example", "X-Real-Ip", "10.0.0.1")
		if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" ||
			rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("%s: %v", target, rec.Header())
		}
	}
	rec = do(p, "GET", "/api/proxy/svc/own", nil, "Origin", "https://app.example")
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("upstream CORS overridden: %v", rec.Header())
	}

	// With an allow-list, other origins are not reflected.
	p = New(Options{Catalog: mockCatalog(up.URL), Logger: slog.New(slog.DiscardHandler), CORSAllowedOrigins: []string{"https://ok.example"}})
	rec = do(p, "GET", "/api/proxy/svc/", nil, "Origin", "https://evil.example")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("unlisted origin reflected: %v", rec.Header())
	}
}

// The mocked body is used verbatim before I12, template marker included.
func TestTemplateNotRenderedYet(t *testing.T) {
	body := "<% TEMPLATE: LiquidJS %>{{ 1 | plus: 1 }}"
	p := New(Options{Catalog: mockCatalog("http://upstream.invalid", store.Scenario{Name: "t", SkipProxy: true, ResponseBody: sp(body)}),
		Logger: slog.New(slog.DiscardHandler)})
	rec := do(p, "GET", "/api/proxy/svc/", nil, "X-Real-Ip", "10.0.0.1")
	if got, _ := io.ReadAll(rec.Body); string(got) != body {
		t.Errorf("body = %q", got)
	}
}
