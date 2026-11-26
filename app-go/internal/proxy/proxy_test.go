package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

// staticCatalog serves one snapshot.
type staticCatalog struct {
	snap *catalog.Snapshot
	err  error
}

func (c staticCatalog) Get(context.Context) (*catalog.Snapshot, error) { return c.snap, c.err }

// services builds a catalog with the given path → upstream URL services.
func services(kv ...string) staticCatalog {
	var data store.CatalogData
	for i := 0; i+1 < len(kv); i += 2 {
		data.Services = append(data.Services, store.CatalogService{Service: store.Service{
			ID: int64(i + 1), Path: kv[i], Name: kv[i], UpstreamURL: kv[i+1]}})
	}
	return staticCatalog{snap: catalog.Build(data, nil, slog.New(slog.DiscardHandler))}
}

// seen is what the upstream received.
type seen struct {
	Method, RequestURI, Host string
	Header                   http.Header
	BodyLen                  int
	BodySum                  [32]byte
}

// echoUpstream records each request on the returned channel and answers 200 "ok".
func echoUpstream(t *testing.T) (*httptest.Server, chan seen) {
	t.Helper()
	ch := make(chan seen, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ch <- seen{Method: r.Method, RequestURI: r.RequestURI, Host: r.Host, Header: r.Header.Clone(),
			BodyLen: len(body), BodySum: sha256.Sum256(body)}
		w.Header().Set("X-Upstream", "yes")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func do(h http.Handler, method, target string, body io.Reader, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, body)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Add(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestPathAndQueryJoining(t *testing.T) {
	up, ch := echoUpstream(t)
	p := New(Options{Catalog: services(
		"plain", up.URL,
		"slash", up.URL+"/",
		"base", up.URL+"/base",
		"baseslash", up.URL+"/base/",
		"query", up.URL+"/q?key=1",
	), Logger: slog.New(slog.DiscardHandler)})

	cases := []struct{ target, want string }{
		{"/api/proxy/plain", "/"},
		{"/api/proxy/plain/", "/"},
		{"/api/proxy/plain/a/b?x=1", "/a/b?x=1"},
		{"/api/proxy/plain/a/b/?x=1&x=2&y", "/a/b/?x=1&x=2&y"},
		{"/api/proxy/slash/a", "/a"},
		{"/api/proxy/base", "/base"},
		{"/api/proxy/base/content", "/base/content"},
		{"/api/proxy/baseslash/content", "/base/content"},
		{"/api/proxy/baseslash", "/base/"},
		{"/api/proxy/plain/a%2Fb/c%20d", "/a%2Fb/c%20d"}, // escaped as the client sent it
		{"/api/proxy/plain/?q=%2F%20", "/?q=%2F%20"},
		{"/api/proxy/query/x?y=2", "/q/x?key=1&y=2"},
		{"/api/proxy/query/x", "/q/x?key=1"},
	}
	for _, tc := range cases {
		rec := do(p, "GET", tc.target, nil)
		if rec.Code != http.StatusCreated {
			t.Errorf("%s: status %d %s", tc.target, rec.Code, rec.Body)
			continue
		}
		if got := (<-ch).RequestURI; got != tc.want {
			t.Errorf("%s: upstream got %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestPassthroughResponse(t *testing.T) {
	up, ch := echoUpstream(t)
	p := New(Options{Catalog: services("svc", up.URL), Logger: slog.New(slog.DiscardHandler)})
	rec := do(p, "POST", "/api/proxy/svc/x", strings.NewReader("hello"))
	if rec.Code != http.StatusCreated || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Upstream") != "yes" || rec.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("upstream headers not passed: %v", rec.Header())
	}
	if rec.Header().Get(HeaderStatus) != StatusProxied {
		t.Errorf("x-tmock-status = %q", rec.Header().Get(HeaderStatus))
	}
	for _, h := range []string{"Content-Security-Policy", "X-Frame-Options", HeaderClient, HeaderScenario} {
		if rec.Header().Get(h) != "" {
			t.Errorf("%s set on a proxied response", h)
		}
	}
	if s := <-ch; s.Method != "POST" || s.BodyLen != 5 {
		t.Errorf("upstream saw %+v", s)
	}
}

func TestRequestHeaders(t *testing.T) {
	up, ch := echoUpstream(t)
	upHost := strings.TrimPrefix(up.URL, "http://")
	forwarded := []string{
		"X-Forwarded-Host", "public.example", "X-Forwarded-Port", "443", "X-Forwarded-Proto", "https",
		"X-Forwarded-Scheme", "https", "X-Scheme", "https", "X-Forwarded-Server", "edge",
	}
	send := func(opts Options, extra ...string) seen {
		t.Helper()
		opts.Catalog, opts.Logger = services("svc", up.URL), slog.New(slog.DiscardHandler)
		hdr := append([]string{
			"X-Custom", "1", "X-Custom", "2", "Connection", "X-Hop", "X-Hop", "secret",
			"Te", "gzip", "Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			"Tracestate", "a=b",
		}, extra...)
		rec := do(New(opts), "GET", "http://tmock.local/api/proxy/svc/", nil, hdr...)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status %d", rec.Code)
		}
		return <-ch
	}

	s := send(Options{}, forwarded...)
	if s.Host != upHost {
		t.Errorf("Host = %q, want the upstream's %q", s.Host, upHost)
	}
	if got := s.Header.Values("X-Custom"); len(got) != 2 {
		t.Errorf("X-Custom = %v", got)
	}
	for _, h := range []string{"X-Hop", "Te", "Apikey"} {
		if s.Header.Get(h) != "" {
			t.Errorf("%s forwarded: %q", h, s.Header.Get(h))
		}
	}
	if s.Header.Get("X-Forwarded-For") != "" {
		t.Errorf("X-Forwarded-For added: %q", s.Header.Get("X-Forwarded-For"))
	}
	if s.Header.Get("Traceparent") == "" || s.Header.Get("Tracestate") != "a=b" {
		t.Error("trace context not forwarded")
	}
	if s.Header.Get(HeaderStatus) != StatusProxied {
		t.Errorf("x-tmock-status = %q", s.Header.Get(HeaderStatus))
	}
	for i := 0; i < len(forwarded); i += 2 {
		if got := s.Header.Get(forwarded[i]); got != forwarded[i+1] {
			t.Errorf("by default %s = %q, want it forwarded as %q", forwarded[i], got, forwarded[i+1])
		}
	}

	// The client's own X-Forwarded-For passes unchanged; nothing is appended.
	s = send(Options{}, "X-Forwarded-For", "203.0.113.7")
	if got := s.Header.Values("X-Forwarded-For"); len(got) != 1 || got[0] != "203.0.113.7" {
		t.Errorf("X-Forwarded-For = %v", got)
	}

	s = send(Options{APIKey: "key-123", RemoveForwardedHeaders: true}, forwarded...)
	if s.Header.Get("Apikey") != "key-123" {
		t.Errorf("Apikey = %q", s.Header.Get("Apikey"))
	}
	for i := 0; i < len(forwarded); i += 2 {
		if got := s.Header.Get(forwarded[i]); got != "" {
			t.Errorf("%s = %q, want it removed", forwarded[i], got)
		}
	}
}

// A body bigger than the API's 10 MB limit is streamed to the upstream unchanged, and a
// response is streamed back to the client as the upstream writes it.
func TestStreaming(t *testing.T) {
	up, ch := echoUpstream(t)
	p := New(Options{Catalog: services("svc", up.URL), Logger: slog.New(slog.DiscardHandler)})
	front := httptest.NewServer(p)
	defer front.Close()

	big := make([]byte, 24<<20)
	_, _ = rand.Read(big)
	resp, err := http.Post(front.URL+"/api/proxy/svc/upload", "application/octet-stream", bytes.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if s := <-ch; s.BodyLen != len(big) || s.BodySum != sha256.Sum256(big) {
		t.Fatalf("upstream got %d bytes, want %d unchanged", s.BodyLen, len(big))
	}

	// The upstream sends a first chunk, then waits until the client has seen it.
	release := make(chan struct{})
	streaming := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "second\n")
	}))
	defer streaming.Close()
	p.opts.Catalog = services("stream", streaming.URL)
	resp, err = http.Get(front.URL + "/api/proxy/stream/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 6)
	if _, err := io.ReadFull(resp.Body, buf); err != nil || string(buf) != "first\n" {
		t.Fatalf("first chunk: %q %v", buf, err)
	}
	close(release)
	rest, _ := io.ReadAll(resp.Body)
	if string(rest) != "second\n" {
		t.Errorf("rest = %q", rest)
	}
}

// A compressed upstream body reaches the client compressed, byte for byte.
func TestGzipPassthrough(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, _ = zw.Write(bytes.Repeat([]byte(`{"hello":"world"}`), 100))
	_ = zw.Close()
	var gotAE string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAE = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(compressed.Len()))
		_, _ = w.Write(compressed.Bytes())
	}))
	defer up.Close()
	p := New(Options{Catalog: services("svc", up.URL), Logger: slog.New(slog.DiscardHandler)})

	rec := do(p, "GET", "/api/proxy/svc/", nil, "Accept-Encoding", "gzip")
	if gotAE != "gzip" {
		t.Errorf("upstream Accept-Encoding = %q", gotAE)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" || !bytes.Equal(rec.Body.Bytes(), compressed.Bytes()) {
		t.Errorf("body changed: encoding %q, %d bytes", rec.Header().Get("Content-Encoding"), rec.Body.Len())
	}

	// A client that asks for no compression gets none added by the proxy's transport.
	do(p, "GET", "/api/proxy/svc/", nil)
	if gotAE != "" {
		t.Errorf("transport added Accept-Encoding %q", gotAE)
	}
}

func TestUnknownService(t *testing.T) {
	up, ch := echoUpstream(t)
	logs, logger := logxtest.New()
	p := New(Options{Catalog: services("svc", up.URL), Logger: logger})
	for _, target := range []string{"/api/proxy/nope", "/api/proxy/nope/a", "/api/proxy/SVC/a", "/api/proxy/"} {
		rec := do(p, "GET", target, nil)
		if rec.Code != http.StatusNotFound || rec.Body.String() != "Service not found" ||
			rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("%s: %d %q %q", target, rec.Code, rec.Body, rec.Header().Get("Content-Type"))
		}
	}
	select {
	case s := <-ch:
		t.Errorf("upstream called: %+v", s)
	default:
	}
	ev := logs.Events(logx.EventProxyRequest)
	if len(ev) != 4 || ev[0].Attrs[logx.AttrHTTPStatusCode] != "404" || ev[0].Attrs[logx.AttrService] != "" {
		t.Errorf("events: %+v", ev)
	}
}

func TestCatalogUnavailable(t *testing.T) {
	p := New(Options{Catalog: staticCatalog{err: errors.New("db down")}, Logger: slog.New(slog.DiscardHandler)})
	rec := do(p, "GET", "/api/proxy/svc/", nil)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
		t.Errorf("got %d %q", rec.Code, rec.Body)
	}
}

func TestUpstreamDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens there any more: connection refused

	logs, logger := logxtest.New()
	p := New(Options{Catalog: services("svc", "http://"+addr+"/base"), Logger: logger})
	rec := do(p, "GET", "/api/proxy/svc/a?b=1", nil)
	if rec.Code != http.StatusBadGateway || rec.Body.String() != "Bad Gateway" ||
		rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("got %d %q", rec.Code, rec.Body)
	}

	failed := logs.Events(logx.EventUpstreamFailed)
	if len(failed) != 1 {
		t.Fatalf("upstream_failed events: %+v", logs.All())
	}
	f := failed[0]
	if f.Level != slog.LevelError || f.Attrs[logx.AttrService] != "svc" || f.Attrs[logx.AttrServerAddress] != "127.0.0.1" ||
		f.Attrs[logx.AttrErrorType] != "connection_refused" || f.Attrs[logx.AttrExceptionMessage] == "" {
		t.Errorf("upstream_failed = %+v", f)
	}
	req := logs.Events(logx.EventProxyRequest)
	if len(req) != 1 || req[0].Attrs[logx.AttrHTTPStatusCode] != "502" {
		t.Errorf("proxy.request = %+v", req)
	}
}

func TestUpstreamTimeout(t *testing.T) {
	stop := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // never answers in time
		case <-stop:
		case <-r.Context().Done():
		}
	}))
	defer up.Close()
	defer close(stop)
	logs, logger := logxtest.New()
	p := New(Options{Catalog: services("svc", up.URL), Logger: logger, UpstreamTimeout: 50 * time.Millisecond})
	if rec := do(p, "GET", "/api/proxy/svc/", nil); rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d", rec.Code)
	}
	if f := logs.Events(logx.EventUpstreamFailed); len(f) != 1 || f[0].Attrs[logx.AttrErrorType] != "timeout" {
		t.Errorf("upstream_failed = %+v", f)
	}
}

// A client that goes away is not an upstream failure.
func TestClientCancelIsNotAnUpstreamFailure(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer up.Close()
	logs, logger := logxtest.New()
	p := New(Options{Catalog: services("svc", up.URL), Logger: logger})
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequestWithContext(ctx, "GET", "/api/proxy/svc/", nil)
	time.AfterFunc(20*time.Millisecond, cancel)
	p.ServeHTTP(httptest.NewRecorder(), r)
	if f := logs.Events(logx.EventUpstreamFailed); len(f) != 0 {
		t.Errorf("upstream_failed logged for a cancelled client: %+v", f)
	}
	if n := len(logs.Events(logx.EventProxyRequest)); n != 1 {
		t.Errorf("%d proxy.request events", n)
	}
}

func TestProxyRequestEvent(t *testing.T) {
	up, _ := echoUpstream(t)
	logs, logger := logxtest.New()
	p := New(Options{Catalog: services("svc", up.URL), Logger: logger, APIKey: "api-key-value"})
	do(p, "GET", "/api/proxy/svc/a/b?x=1", nil, "X-Real-Ip", "10.1.2.3", "X-Client-Token", "tmck_secret",
		"Authorization", "Bearer auth-value", "Cookie", "c=cookie-value")

	ev := logs.Events(logx.EventProxyRequest)
	if len(ev) != 1 {
		t.Fatalf("events: %+v", logs.All())
	}
	r := ev[0]
	want := map[string]string{
		logx.AttrHTTPMethod:     "GET",
		logx.AttrURLPath:        "/api/proxy/svc/a/b",
		logx.AttrURLQuery:       "x=1",
		logx.AttrHTTPStatusCode: "201",
		logx.AttrClientAddress:  "10.1.2.3",
		logx.AttrService:        "svc",
		logx.AttrServerAddress:  "127.0.0.1",
		logx.AttrHTTPBodySize:   "2",
	}
	for k, v := range want {
		if r.Attrs[k] != v {
			t.Errorf("%s = %q, want %q", k, r.Attrs[k], v)
		}
	}
	if r.Level != slog.LevelInfo || r.Attrs[logx.AttrDurationMS] == "" {
		t.Errorf("record = %+v", r)
	}
	if !strings.HasPrefix(r.Message, "10.1.2.3 GET /api/proxy/svc/a/b?x=1 201 ") || !strings.HasSuffix(r.Message, "ms 2B") {
		t.Errorf("message = %q", r.Message)
	}
	// Spec §2.4.3: header values are never logged.
	for _, rec := range logs.All() {
		for _, s := range []string{"api-key-value", "tmck_secret", "auth-value", "cookie-value"} {
			if strings.Contains(rec.Text(), s) {
				t.Errorf("record leaks %q: %s", s, rec.Text())
			}
		}
	}
}

func TestFormatSize(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0B", 512: "512B", 1023: "1023B", 1024: "1KB", 1536: "1.5KB", 10 * 1024: "10KB",
		2 << 20: "2MB", 3<<30 + 1<<29: "3.5GB",
	} {
		if got := FormatSize(n); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// The upstream sees the path and query the client sent (the I09 manual check, automated).
func TestEchoShape(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"uri": r.RequestURI})
	}))
	defer up.Close()
	p := New(Options{Catalog: services("svc", up.URL), Logger: slog.New(slog.DiscardHandler)})
	rec := do(p, "GET", "/api/proxy/svc/a/b?x=1", nil)
	if !strings.Contains(rec.Body.String(), `"/a/b?x=1"`) {
		t.Errorf("body = %s", rec.Body)
	}
}
