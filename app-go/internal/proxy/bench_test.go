package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Spec §15.3 I13: a benchmark of the no-scenario path. The upstream is an in-memory
// transport, so the numbers are the proxy's own cost (routing, selection, rewriting,
// copying, logging), not the network's.
//
//	go test ./internal/proxy -run '^$' -bench Passthrough -benchmem
//
// Results are noted in app-go/README.md ("Proxy benchmark").

// staticTransport answers every request with the same 200 and body, without a network.
type staticTransport struct{ body []byte }

func (t staticTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/json"}, "Content-Length": {strconv.Itoa(len(t.body))}},
		ContentLength: int64(len(t.body)),
		Body:          io.NopCloser(bytes.NewReader(t.body)),
		Request:       r,
	}, nil
}

// discardWriter is a reusable ResponseWriter, so the benchmark counts the proxy's
// allocations and not httptest.ResponseRecorder's.
type discardWriter struct{ h http.Header }

func (w *discardWriter) Header() http.Header         { return w.h }
func (w *discardWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *discardWriter) WriteHeader(int)             {}

func benchProxy(b *testing.B, clients []store.CatalogClient, target string) {
	data := store.CatalogData{
		Services: []store.CatalogService{{Service: store.Service{ID: 1, Path: "svc", Name: "Svc", UpstreamURL: "http://upstream.test/base"}}},
		Clients:  clients,
	}
	snap := catalog.Build(data, nil, slog.New(slog.DiscardHandler))
	// The log line is formatted, as in production, but written nowhere.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(Options{Catalog: staticCatalog{snap: snap}, Logger: logger,
		Transport: staticTransport{body: bytes.Repeat([]byte("x"), 1024)}})
	r := httptest.NewRequest("GET", target, nil)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("User-Agent", "bench")
	r.Header.Set("X-Forwarded-For", "10.0.0.9")
	w := &discardWriter{h: http.Header{}}

	b.ReportAllocs()
	for b.Loop() {
		clear(w.h)
		p.ServeHTTP(w, r)
	}
}

// BenchmarkPassthrough: no client matches, so the request is forwarded as it is.
func BenchmarkPassthrough(b *testing.B) {
	benchProxy(b, nil, "/api/proxy/svc/orders/42?page=2")
}

// BenchmarkPassthroughClients: three clients are checked and one matches, but none of
// its scenarios applies (the service has none), so the request is still forwarded as it
// is.
func BenchmarkPassthroughClients(b *testing.B) {
	benchProxy(b, []store.CatalogClient{
		{ID: 1, Name: "a", Condition: []byte(`{"ip":"10.9.9.9"}`)},
		{ID: 2, Name: "b", Condition: []byte(`{"headerRegex":{"header":"User-Agent","value":"^curl/"}}`)},
		{ID: 3, Name: "c", Condition: []byte(`{"cidr":"10.0.0.0/8"}`)},
	}, "/api/proxy/svc/orders/42?page=2")
}
