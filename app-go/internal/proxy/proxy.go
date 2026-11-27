// Package proxy is the proxy and mock engine at /api/proxy/ (spec §7). It looks the
// service up in the catalog snapshot, finds the matching clients and the winning
// scenario, and then answers from the scenario alone (skip proxy) or forwards the request
// to the upstream and modifies its response. A body that is a template is rendered on
// every request (internal/liquid).
//
// The package is backend code: standard library only.
package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/httpx"
)

// Catalog returns the current catalog snapshot; *catalog.Cache is one.
type Catalog interface {
	Get(ctx context.Context) (*catalog.Snapshot, error)
}

// Options configures the proxy.
type Options struct {
	Catalog Catalog
	Logger  *slog.Logger // nil means slog.Default()

	APIKey                 string        // SERVICES_API_KEY: sent upstream as Apikey
	RemoveForwardedHeaders bool          // PROXY_REMOVE_FORWARDED_HEADERS
	UpstreamTimeout        time.Duration // PROXY_UPSTREAM_TIMEOUT: connect and wait for response headers; 0 = 30s
	CORSAllowedOrigins     []string      // PROXY_CORS_ALLOWED_ORIGINS; empty reflects any origin (spec §9.5)

	// Transport sends the upstream requests. Nil means a transport of the proxy's own,
	// built from UpstreamTimeout.
	Transport http.RoundTripper
}

// Header names set by tmock (spec §7.7, §7.8).
const (
	HeaderStatus   = "X-Tmock-Status"
	HeaderClient   = "X-Tmock-Client"
	HeaderScenario = "X-Tmock-Scenario"

	StatusProxied = "proxied"
	StatusMocked  = "mocked"
)

// forwardedHeaders are removed from upstream requests when PROXY_REMOVE_FORWARDED_HEADERS
// is true (spec §4.2).
var forwardedHeaders = []string{"X-Forwarded-Host", "X-Forwarded-Port", "X-Forwarded-Proto",
	"X-Forwarded-Scheme", "X-Scheme", "X-Forwarded-Server"}

// restoredHeaders are the forwarded headers that httputil.ReverseProxy removes before
// calling Rewrite. The proxy passes the client's headers on unchanged (spec §7.7), so it
// puts back the ones the client sent.
var restoredHeaders = []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"}

// Proxy is the handler of /api/proxy/.
type Proxy struct {
	opts    Options
	logger  *slog.Logger
	handler http.Handler // CORS around the mux
	rp      *httputil.ReverseProxy
}

// New returns the proxy handler.
func New(opts Options) *Proxy {
	p := &Proxy{opts: opts, logger: opts.Logger}
	if p.logger == nil {
		p.logger = slog.Default()
	}
	transport := opts.Transport
	if transport == nil {
		transport = newTransport(opts.UpstreamTimeout)
	}
	p.rp = &httputil.ReverseProxy{
		Rewrite:        p.rewrite,
		Transport:      transport,
		ModifyResponse: p.modifyResponse,
		ErrorHandler:   p.upstreamFailed,
		ErrorLog:       slog.NewLogLogger(p.logger.Handler(), slog.LevelWarn),
	}
	// The proxy receives the path as sent (internal/server), so it routes with a mux of
	// its own, which cleans paths such as /api/proxy/../x by redirecting.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/proxy/{service}", p.serve)
	mux.HandleFunc("/api/proxy/{service}/{rest...}", p.serve)
	mux.HandleFunc("/api/proxy/", p.serve) // no service name: 404
	// CORS applies to every proxy response, mocked or not; preflights never reach the mux.
	p.handler = httpx.ProxyCORS(opts.CORSAllowedOrigins, mux)
	return p
}

// newTransport clones http.DefaultTransport with the upstream timeout. Compression is
// left to the client and the upstream: the transport adds no Accept-Encoding and never
// decompresses, so compressed bodies pass through as they are (spec §7.8).
func newTransport(timeout time.Duration) *http.Transport {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext
	t.ResponseHeaderTimeout = timeout
	t.DisableCompression = true
	t.MaxIdleConnsPerHost = 32
	return t
}

// ServeHTTP logs every proxy request (spec §7.9), including the ones the mux answers
// itself, such as the redirect to a clean path, and CORS preflights.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &recorder{ResponseWriter: w}
	ex := &exchange{}
	r = r.WithContext(context.WithValue(r.Context(), exchangeKey{}, ex))
	defer func() {
		status := rec.status()
		v := recover()
		if v != nil && v != http.ErrAbortHandler && rec.code == 0 {
			// httpx.Recover, outside the proxy, answers 500 for a panic before the
			// response started.
			status = http.StatusInternalServerError
		}
		// Also when ReverseProxy aborts a response whose body could not be copied (it
		// panics with http.ErrAbortHandler).
		p.logRequest(r, status, rec.size, ex, time.Since(start))
		if v != nil {
			panic(v)
		}
	}()
	p.handler.ServeHTTP(rec, r)
}

// exchange is the request-scoped state of one proxy request. The snapshot it points into
// is shared and must not be modified.
type exchange struct {
	service *catalog.Service
	rest    string // the escaped path after /api/proxy/{service}/, without the leading /
	sel     selection
	failure error // the upstream error, if the upstream call failed
}

type exchangeKey struct{}

func exchangeFrom(r *http.Request) *exchange {
	ex, _ := r.Context().Value(exchangeKey{}).(*exchange)
	return ex
}

func (p *Proxy) serve(w http.ResponseWriter, r *http.Request) {
	snap, err := p.opts.Catalog.Get(r.Context())
	if err != nil {
		// Logged by the catalog as tmock.catalog.rebuild_failed.
		plain(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	svc, ok := snap.Service(r.PathValue("service"))
	if !ok {
		plain(w, http.StatusNotFound, "Service not found")
		return
	}
	ex := exchangeFrom(r)
	ex.service = svc
	ex.rest = restOf(r)
	sel, err := selectScenario(svc, matchingClients(snap.Clients(), r), r.Method, requestPath(r), &requestEnv{r: r})
	ex.sel = sel
	switch {
	case errors.Is(err, errBodyTooLarge):
		plain(w, http.StatusRequestEntityTooLarge, "Request Entity Too Large")
		return
	case err != nil:
		// The client went away, or sent a broken body, while a request condition needed it.
		plain(w, http.StatusBadRequest, "Bad Request")
		return
	}
	if skipsProxy(ex.sel.scenario) {
		writeMock(w, ex.sel, p.responseBody(r.Context(), ex.sel.scenario))
		return
	}
	p.rp.ServeHTTP(w, r)
}

// restOf returns the part of the escaped request path after /api/proxy/{service}/,
// escaped as the client sent it, so that encoded characters such as %2F reach the
// upstream unchanged.
func restOf(r *http.Request) string {
	after := strings.TrimPrefix(r.URL.EscapedPath(), "/api/proxy/")
	_, rest, _ := strings.Cut(after, "/")
	return rest
}

// rewrite builds the upstream request (spec §7.7).
func (p *Proxy) rewrite(pr *httputil.ProxyRequest) {
	ex := exchangeFrom(pr.In)
	target := ex.service.Upstream
	out := pr.Out

	out.URL.Scheme = target.Scheme
	out.URL.Host = target.Host
	out.URL.User = nil
	// Path and RawPath, so that url.URL sends the escaped path as it is.
	path := joinPath(target.EscapedPath(), ex.rest)
	out.URL.Path, out.URL.RawPath = path, path
	if unescaped, err := url.PathUnescape(path); err == nil {
		out.URL.Path = unescaped
	}
	out.URL.RawQuery = joinQuery(target.RawQuery, pr.In.URL.RawQuery)
	out.Host = "" // the Host header is the upstream's host

	h := out.Header
	for _, k := range restoredHeaders {
		if v, ok := pr.In.Header[k]; ok {
			h[k] = append([]string(nil), v...)
		}
	}
	if p.opts.RemoveForwardedHeaders {
		for _, k := range forwardedHeaders {
			h.Del(k)
		}
	}
	if p.opts.APIKey != "" {
		h.Set("Apikey", p.opts.APIKey)
	}
	// "mocked" means a scenario applies, even though the request still goes upstream.
	setTmockHeaders(h, status(ex.sel), ex.sel)
}

// status is the x-tmock-status value for sel.
func status(sel selection) string {
	if sel.scenario != nil {
		return StatusMocked
	}
	return StatusProxied
}

// joinPath appends rest to the upstream's base path: http://h/base + content →
// /base/content. An empty rest is the base path, or / (spec §7.7).
func joinPath(base, rest string) string {
	if rest == "" {
		if base == "" {
			return "/"
		}
		return base
	}
	return strings.TrimSuffix(base, "/") + "/" + rest
}

// joinQuery joins the upstream URL's own query, if it has one, with the request's.
func joinQuery(base, req string) string {
	switch {
	case base == "":
		return req
	case req == "":
		return base
	}
	return base + "&" + req
}

// modifyResponse applies the winning scenario to the upstream response (spec §7.8).
// Without one, status, headers and body pass through untouched (the body streamed), with
// only the x-tmock-* headers set.
func (p *Proxy) modifyResponse(resp *http.Response) error {
	ex := exchangeFrom(resp.Request)
	if ex.sel.scenario == nil {
		setTmockHeaders(resp.Header, StatusProxied, ex.sel)
		return nil
	}
	// Rendered once the upstream has answered, so 'now' is the time of the response.
	modifyUpstream(resp, ex.sel, p.responseBody(resp.Request.Context(), ex.sel.scenario))
	return nil
}

// upstreamFailed answers 502 when the upstream cannot be reached (connection refused,
// DNS, timeout; spec §7.7).
func (p *Proxy) upstreamFailed(w http.ResponseWriter, r *http.Request, err error) {
	if ex := exchangeFrom(r); ex != nil && r.Context().Err() == nil {
		ex.failure = err // a client that went away is not an upstream failure
	}
	if ex := exchangeFrom(r); ex != nil {
		setTmockHeaders(w.Header(), status(ex.sel), ex.sel)
	}
	plain(w, http.StatusBadGateway, "Bad Gateway")
}

// plain writes a short text/plain response.
func plain(w http.ResponseWriter, status int, msg string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}

// recorder records the status and the number of body bytes written to the client.
type recorder struct {
	http.ResponseWriter
	code int
	size int64
}

func (r *recorder) WriteHeader(code int) {
	if r.code == 0 && (code >= 200 || code == http.StatusSwitchingProtocols) {
		r.code = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.size += int64(n)
	return n, err
}

func (r *recorder) status() int {
	if r.code == 0 {
		return http.StatusOK
	}
	return r.code
}

// Unwrap lets http.ResponseController reach the underlying writer: ReverseProxy flushes
// streamed responses and hijacks upgraded connections through it.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
