package proxy

import (
	"bytes"
	"crypto/sha256"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

const seedUser = "4a30d84d-b820-4396-81b3-bc010aa95c01"

// Spec §7.4, §7.5: a JSON body condition (the legacy seed example) decides whether the
// scenario applies; when it does not, the request is proxied with its body unchanged.
func TestRequestConditionOnBody(t *testing.T) {
	up, seenCh := echoUpstream(t)
	p := New(Options{Catalog: mockCatalog(up.URL,
		store.Scenario{Name: "demo", RequestMethod: sp("POST"), RequestCondition: sp(`$request.body.userId == "` + seedUser + `"`),
			SkipProxy: true, ResponseBody: sp(`{"demo":true}`)},
	)})

	body := `{"userId":"` + seedUser + `"}`
	rec := do(p, "POST", "/api/proxy/svc/content", strings.NewReader(body), "X-Real-Ip", "10.0.0.1", "Content-Type", "application/json")
	if rec.Code != 200 || rec.Body.String() != `{"demo":true}` || rec.Header().Get(HeaderScenario) != "demo" {
		t.Errorf("match: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
	if len(seenCh) != 0 {
		t.Error("the upstream was contacted")
	}

	other := `{"userId":"someone-else","pad":"` + strings.Repeat("x", 1000) + `"}`
	rec = do(p, "POST", "/api/proxy/svc/content", strings.NewReader(other), "X-Real-Ip", "10.0.0.1", "Content-Type", "application/json")
	if rec.Code != 201 || rec.Header().Get(HeaderStatus) != StatusProxied || rec.Header().Get(HeaderClient) != "qa" {
		t.Errorf("no match: %d %v", rec.Code, rec.Header())
	}
	s := <-seenCh
	if s.BodyLen != len(other) || s.BodySum != sha256.Sum256([]byte(other)) || s.Header.Get("Content-Length") != strconv.Itoa(len(other)) {
		t.Errorf("upstream got %d bytes, Content-Length %q", s.BodyLen, s.Header.Get("Content-Length"))
	}
}

// Spec §7.4: the rest of the namespace, from a real request.
func TestRequestConditionNamespace(t *testing.T) {
	p := New(Options{Catalog: mockCatalog("http://upstream.invalid",
		store.Scenario{Name: "a-form", RequestCondition: sp(`$request.body.user == 'bob' && $request.body.n == '1'`), SkipProxy: true, ResponseBody: sp("form")},
		store.Scenario{Name: "b-rest", SkipProxy: true, ResponseBody: sp("rest"), RequestCondition: sp(
			`$request.method == 'PUT' && $request.path == '/api/proxy/svc/a b' && $request.query.page == '2' && ` +
				`$request.headers["x-multi"] == 'one, two' && $request.headers.host == 'example.com' && $request.ip == '10.0.0.1'`)},
		store.Scenario{Name: "c-fallback", SkipProxy: true, ResponseBody: sp("fallback")},
	)})
	cases := []struct {
		method, target, ct, body string
		hdr                      []string
		want                     string
	}{
		{"POST", "/api/proxy/svc/x", "application/x-www-form-urlencoded", "user=bob&n=1&n=2", nil, "form"},
		{"POST", "/api/proxy/svc/x", "text/plain", "user=bob&n=1", nil, "fallback"},
		{"PUT", "/api/proxy/svc/a%20b?page=2&page=3", "", "", []string{"X-Multi", "one", "X-Multi", "two"}, "rest"},
		{"PUT", "/api/proxy/svc/a%20b?page=3", "", "", []string{"X-Multi", "one", "X-Multi", "two"}, "fallback"},
	}
	for _, tc := range cases {
		hdr := append([]string{"X-Real-Ip", "10.0.0.1"}, tc.hdr...)
		if tc.ct != "" {
			hdr = append(hdr, "Content-Type", tc.ct)
		}
		rec := do(p, tc.method, "http://example.com"+tc.target, strings.NewReader(tc.body), hdr...)
		if rec.Body.String() != tc.want {
			t.Errorf("%s %s: %q, want %q", tc.method, tc.target, rec.Body, tc.want)
		}
	}
}

// Spec §7.7: the body is buffered only when a candidate scenario's condition needs it; a
// condition that does not leaves it streamed (no Content-Length added).
func TestBodyNotBufferedWithoutBodyCondition(t *testing.T) {
	up, seenCh := echoUpstream(t)
	p := New(Options{Catalog: mockCatalog(up.URL,
		store.Scenario{Name: "a-headers", RequestCondition: sp(`$request.headers["x-mock"] == 'yes'`), ResponseCode: ip(202)},
		store.Scenario{Name: "b-body-other-path", RequestPath: sp("other"), RequestCondition: sp(`$request.body.a`), ResponseCode: ip(203)},
	)})
	chunked := func() io.Reader { return io.MultiReader(strings.NewReader("streamed "), strings.NewReader("body")) }

	rec := do(p, "POST", "/api/proxy/svc/x", chunked(), "X-Real-Ip", "10.0.0.1")
	s := <-seenCh
	if rec.Code != 201 || s.BodyLen != len("streamed body") || s.Header.Get("Content-Length") != "" {
		t.Errorf("%d, upstream got %d bytes, Content-Length %q", rec.Code, s.BodyLen, s.Header.Get("Content-Length"))
	}

	// The body condition of a candidate reads it, and the upstream still gets it whole.
	rec = do(p, "POST", "/api/proxy/svc/other", chunked(), "X-Real-Ip", "10.0.0.1")
	s = <-seenCh
	if rec.Code != 201 || s.BodyLen != len("streamed body") || s.Header.Get("Content-Length") != "13" {
		t.Errorf("%d, upstream got %d bytes, Content-Length %q", rec.Code, s.BodyLen, s.Header.Get("Content-Length"))
	}
}

// Spec §7.7: a body a condition needs is buffered up to 10 MB; a larger one is 413.
func TestBodyConditionTooLarge(t *testing.T) {
	up, seenCh := echoUpstream(t)
	logs, logger := logxtest.New()
	p := New(Options{Logger: logger, Catalog: mockCatalog(up.URL,
		store.Scenario{Name: "body", RequestCondition: sp(`$request.body.a == 1`), ResponseCode: ip(202)},
	)})

	big := bytes.Repeat([]byte("x"), httpx.MaxBodyBytes+1)
	rec := do(p, "POST", "/api/proxy/svc/x", bytes.NewReader(big), "X-Real-Ip", "10.0.0.1")
	if rec.Code != 413 || len(seenCh) != 0 {
		t.Errorf("%d %q, upstream calls %d", rec.Code, rec.Body, len(seenCh))
	}
	if ev := logs.Events(logx.EventProxyRequest); len(ev) != 1 || ev[0].Attrs[logx.AttrHTTPStatusCode] != "413" {
		t.Errorf("events %+v", logs.All())
	}

	// Exactly the limit is fine.
	rec = do(p, "POST", "/api/proxy/svc/x", bytes.NewReader(big[:httpx.MaxBodyBytes]), "X-Real-Ip", "10.0.0.1")
	if rec.Code != 201 {
		t.Errorf("at the limit: %d", rec.Code)
	}
	if s := <-seenCh; s.BodyLen != httpx.MaxBodyBytes {
		t.Errorf("upstream got %d bytes", s.BodyLen)
	}

	// A client that is not matched never has its body read.
	rec = do(p, "POST", "/api/proxy/svc/x", bytes.NewReader(big), "X-Real-Ip", "10.0.0.2")
	if rec.Code != 201 {
		t.Errorf("unmatched: %d", rec.Code)
	}
	if s := <-seenCh; s.BodyLen != len(big) {
		t.Errorf("upstream got %d bytes", s.BodyLen)
	}
}

// A body that cannot be read while a condition needs it is 400.
func TestBodyConditionReadError(t *testing.T) {
	p := New(Options{Catalog: mockCatalog("http://upstream.invalid",
		store.Scenario{Name: "body", RequestCondition: sp(`$request.body.a == 1`), SkipProxy: true, ResponseBody: sp("x")},
	)})
	rec := do(p, "POST", "/api/proxy/svc/x", io.MultiReader(strings.NewReader("{"), errReader{}), "X-Real-Ip", "10.0.0.1")
	if rec.Code != 400 {
		t.Errorf("%d %q", rec.Code, rec.Body)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
