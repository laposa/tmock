package otlp

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/laposa/tmock/app-go/internal/logx"
)

var update = flag.Bool("update", false, "rewrite golden files")

var fixedTime = time.Date(2026, 11, 24, 8, 30, 0, 123456789, time.UTC)

// collector is an httptest OTLP endpoint that records every request.
type collector struct {
	mu       sync.Mutex
	requests []received
	respond  func(w http.ResponseWriter, n int) // n = 0-based request number; nil = 200 {}
}

type received struct {
	header http.Header
	body   []byte // decompressed
}

func (c *collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		body, _ = io.ReadAll(zr)
	}
	c.mu.Lock()
	n := len(c.requests)
	c.requests = append(c.requests, received{header: r.Header.Clone(), body: body})
	c.mu.Unlock()
	if c.respond != nil {
		c.respond(w, n)
		return
	}
	_, _ = io.WriteString(w, "{}")
}

func (c *collector) all() []received {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]received(nil), c.requests...)
}

func newServer(t *testing.T, c *collector) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	return srv
}

// diagnostics captures what the exporter reports about itself.
type diagnostics struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (d *diagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.buf.Write(p)
}

func (d *diagnostics) String() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.buf.String()
}

func (d *diagnostics) logger() *slog.Logger { return slog.New(slog.NewTextHandler(d, nil)) }

func record(level slog.Level, msg string, attrs ...slog.Attr) slog.Record {
	r := slog.NewRecord(fixedTime, level, msg, 0)
	r.AddAttrs(attrs...)
	return r
}

func shutdown(t *testing.T, e *Exporter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

type stringer struct{}

func (stringer) String() string { return "from String()" }

// TestGoldenMapping checks the full slog → OTLP JSON mapping of spec §2.4.5.
func TestGoldenMapping(t *testing.T) {
	c := &collector{}
	srv := newServer(t, c)
	e := New(Options{
		Endpoint:     srv.URL + "/v1/logs",
		Headers:      map[string]string{"Api-Key": "secret"},
		Resource:     map[string]string{"service.name": "tmock", "service.version": "v1.2.3", "deployment.environment.name": "test"},
		ScopeName:    "github.com/laposa/tmock/app-go",
		ScopeVersion: "v1.2.3",
	})
	e.now = func() time.Time { return fixedTime.Add(time.Millisecond) }

	tc, _ := logx.ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	ctx := logx.ContextWithTrace(context.Background(), tc)
	h := e.Handler().WithAttrs([]slog.Attr{slog.String("app", "tmock")}).WithGroup("req").WithAttrs([]slog.Attr{slog.Int("id", 7)})

	_ = h.Handle(ctx, record(slog.LevelInfo, "every value type",
		slog.String(logx.AttrEventName, "tmock.test.event"), // inside group "req", so not the event name
		slog.String("string", "text"),
		slog.Bool("bool", true),
		slog.Bool("false", false),
		slog.Int("int", -42),
		slog.Uint64("uint", 42),
		slog.Uint64("huge", math.MaxUint64),
		slog.Float64("float", 1.5),
		slog.Float64("nan", math.NaN()),
		slog.Time("time", fixedTime),
		slog.Duration("duration", 1500*time.Millisecond),
		slog.Any("strings", []string{"a", "b"}),
		slog.Any("ints", []int64{1, 2}),
		slog.Any("error", errors.New("boom")),
		slog.Any("stringer", stringer{}),
		slog.Any("other", map[string]int{"x": 1}),
		slog.Group("nested", slog.String("a", "1"), slog.Group("deeper", slog.Int("b", 2))),
		slog.Group("", slog.String("inlined", "yes")),
		slog.Group("empty"),
		slog.Attr{},
	))
	plain := e.Handler()
	_ = plain.Handle(context.Background(), record(slog.LevelInfo, "an event", slog.String(logx.AttrEventName, logx.EventHTTPRequest)))
	for _, l := range []slog.Level{slog.LevelDebug - 8, slog.LevelDebug, slog.LevelInfo + 2, slog.LevelWarn, slog.LevelError, slog.LevelError + 20} {
		_ = plain.Handle(context.Background(), record(l, "level "+l.String()))
	}
	shutdown(t, e)

	reqs := c.all()
	if len(reqs) != 1 {
		t.Fatalf("%d requests, want 1", len(reqs))
	}
	if got := reqs[0].header; got.Get("Content-Type") != "application/json" || got.Get("Api-Key") != "secret" || got.Get("User-Agent") != "tmock/v1.2.3" {
		t.Errorf("headers %v", got)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, reqs[0].body, "", "  "); err != nil {
		t.Fatal(err)
	}
	pretty.WriteByte('\n')
	golden := filepath.Join("testdata", "golden.json")
	if *update {
		if err := os.WriteFile(golden, pretty.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(pretty.Bytes(), want) {
		t.Errorf("export request differs from %s (run go test -update after checking)\n%s", golden, pretty.String())
	}
}

func TestSeverity(t *testing.T) {
	for l, want := range map[slog.Level]int{slog.LevelDebug: 5, slog.LevelInfo: 9, slog.LevelWarn: 13, slog.LevelError: 17, slog.LevelInfo + 1: 10, -100: 1, 100: 24} {
		if got := severity(l); got != want {
			t.Errorf("severity(%v) = %d, want %d", l, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("é", maxString) // 2 bytes each
	got := truncate(long)
	if len(got) > maxString || !strings.HasSuffix(got, truncatedSuffix) || !strings.HasPrefix(got, "éé") {
		t.Errorf("len %d, suffix ok %v", len(got), strings.HasSuffix(got, truncatedSuffix))
	}
	if trimmed := strings.TrimSuffix(got, truncatedSuffix); strings.ContainsRune(trimmed, '\uFFFD') || len(trimmed)%2 != 0 {
		t.Error("cut inside a UTF-8 sequence")
	}
	if s := strings.Repeat("a", maxString); truncate(s) != s {
		t.Error("string of exactly maxString bytes was truncated")
	}
}

func TestGzip(t *testing.T) {
	c := &collector{}
	srv := newServer(t, c)
	e := New(Options{Endpoint: srv.URL, Gzip: true})
	_ = e.Handler().Handle(context.Background(), record(slog.LevelInfo, "compressed"))
	shutdown(t, e)
	reqs := c.all()
	if len(reqs) != 1 || reqs[0].header.Get("Content-Encoding") != "gzip" || !bytes.Contains(reqs[0].body, []byte(`"compressed"`)) {
		t.Errorf("requests %+v", reqs)
	}
}

func TestPartialSuccessIsReported(t *testing.T) {
	c := &collector{respond: func(w http.ResponseWriter, _ int) {
		_, _ = io.WriteString(w, `{"partialSuccess":{"rejectedLogRecords":"2","errorMessage":"too old"}}`)
	}}
	srv := newServer(t, c)
	var diag diagnostics
	e := New(Options{Endpoint: srv.URL, Diagnostics: diag.logger()})
	_ = e.Handler().Handle(context.Background(), record(slog.LevelInfo, "x"))
	shutdown(t, e)
	if out := diag.String(); !strings.Contains(out, "rejected some log records: too old") || !strings.Contains(out, "tmock.otlp.rejected_records=2") {
		t.Errorf("diagnostics %q", out)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	c := &collector{respond: func(w http.ResponseWriter, _ int) { http.Error(w, "bad payload", http.StatusBadRequest) }}
	srv := newServer(t, c)
	var diag diagnostics
	e := New(Options{Endpoint: srv.URL, Diagnostics: diag.logger()})
	_ = e.Handler().Handle(context.Background(), record(slog.LevelInfo, "x"))
	shutdown(t, e)
	if n := len(c.all()); n != 1 {
		t.Errorf("%d attempts, want 1", n)
	}
	out := diag.String()
	for _, want := range []string{"OTLP log export failed", "400 Bad Request: bad payload", "event.name=tmock.otlp.export_failed", "dropped 1 log records (queue full: 0, export failed: 1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, out)
		}
	}
}

func TestFlushOnShutdown(t *testing.T) {
	c := &collector{}
	srv := newServer(t, c)
	e := New(Options{Endpoint: srv.URL, ScheduleDelay: time.Hour, MaxExportBatchSize: 2})
	for i := range 5 {
		_ = e.Handler().Handle(context.Background(), record(slog.LevelInfo, fmt.Sprint(i)))
	}
	shutdown(t, e)
	if got := countRecords(t, c.all()); got != 5 {
		t.Errorf("%d records exported, want 5", got)
	}
	// After shutdown, records are ignored and a second Shutdown is harmless.
	_ = e.Handler().Handle(context.Background(), record(slog.LevelInfo, "late"))
	shutdown(t, e)
	if got := countRecords(t, c.all()); got != 5 {
		t.Errorf("%d records after the second shutdown", got)
	}
}

func countRecords(t *testing.T, reqs []received) int {
	t.Helper()
	n := 0
	for _, r := range reqs {
		var req exportRequest
		if err := json.Unmarshal(r.body, &req); err != nil {
			t.Fatal(err)
		}
		n += len(req.ResourceLogs[0].ScopeLogs[0].LogRecords)
	}
	return n
}

// ── Timing tests: synctest with an in-memory transport ─────────────────────────────

// fakeTransport answers requests in memory. Real connections would leave goroutines
// blocked on network reads, which keeps synctest's fake clock from advancing.
type fakeTransport struct {
	mu       sync.Mutex
	times    []time.Time // when each request arrived
	sizes    []int       // records per request
	respond  func(n int, r *http.Request) (*http.Response, error)
	requests chan struct{} // optional: receives one value per request
}

func (f *fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	var req exportRequest
	_ = json.Unmarshal(body, &req)
	f.mu.Lock()
	n := len(f.times)
	f.times = append(f.times, time.Now())
	f.sizes = append(f.sizes, len(req.ResourceLogs[0].ScopeLogs[0].LogRecords))
	f.mu.Unlock()
	if f.requests != nil {
		f.requests <- struct{}{}
	}
	if f.respond != nil {
		return f.respond(n, r)
	}
	return response(200, nil), nil
}

func (f *fakeTransport) snapshot() ([]time.Time, []int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.times...), append([]int(nil), f.sizes...)
}

func response(code int, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{StatusCode: code, Status: fmt.Sprintf("%d %s", code, http.StatusText(code)), Header: header, Body: io.NopCloser(strings.NewReader("{}"))}
}

func newFake(f *fakeTransport, opts Options) *Exporter {
	opts.Endpoint = "http://collector.invalid/v1/logs"
	opts.Client = &http.Client{Transport: f}
	return New(opts)
}

func logN(e *Exporter, n int) {
	for i := range n {
		_ = e.Handler().Handle(context.Background(), record(slog.LevelInfo, fmt.Sprint(i)))
	}
}

func TestBatchBySize(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{}
		e := newFake(f, Options{MaxExportBatchSize: 3, ScheduleDelay: time.Second})
		start := time.Now()
		logN(e, 7)
		synctest.Wait()
		if _, sizes := f.snapshot(); fmt.Sprint(sizes) != "[3 3]" {
			t.Errorf("before the delay: batches %v, want [3 3]", sizes)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		times, sizes := f.snapshot()
		if fmt.Sprint(sizes) != "[3 3 1]" || times[2].Sub(start) != time.Second {
			t.Errorf("batches %v at %v", sizes, times)
		}
		shutdown(t, e)
	})
}

func TestBatchByDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{}
		e := newFake(f, Options{ScheduleDelay: 200 * time.Millisecond})
		logN(e, 1)
		time.Sleep(199 * time.Millisecond)
		synctest.Wait()
		if times, _ := f.snapshot(); len(times) != 0 {
			t.Fatal("sent before the schedule delay")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if _, sizes := f.snapshot(); fmt.Sprint(sizes) != "[1]" {
			t.Errorf("batches %v", sizes)
		}
		shutdown(t, e)
	})
}

func TestQueueOverflowDropsAndReports(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		f := &fakeTransport{respond: func(n int, _ *http.Request) (*http.Response, error) {
			if n == 0 {
				<-release // hold the first export in flight
			}
			return response(200, nil), nil
		}}
		var diag diagnostics
		e := newFake(f, Options{MaxQueueSize: 2, MaxExportBatchSize: 1, Diagnostics: diag.logger()})

		logN(e, 1) // taken by the exporter and stuck in flight
		synctest.Wait()
		logN(e, 4) // 2 fit in the queue, 2 are dropped
		if n := e.droppedQueue.Load(); n != 2 {
			t.Errorf("dropped %d, want 2", n)
		}
		close(release)
		time.Sleep(dropReportInterval)
		synctest.Wait()
		if out := diag.String(); !strings.Contains(out, "dropped 2 log records (queue full: 2, export failed: 0)") {
			t.Errorf("diagnostics %q", out)
		}
		shutdown(t, e)
		if _, sizes := f.snapshot(); len(sizes) != 3 {
			t.Errorf("%d exports, want 3", len(sizes))
		}
	})
}

func TestRetryHonoursRetryAfter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{respond: func(n int, _ *http.Request) (*http.Response, error) {
			switch n {
			case 0:
				return response(503, http.Header{"Retry-After": {"5"}}), nil
			case 1:
				return nil, errors.New("connection refused")
			}
			return response(200, nil), nil
		}}
		var diag diagnostics
		e := newFake(f, Options{ScheduleDelay: time.Millisecond, Diagnostics: diag.logger()})
		start := time.Now()
		logN(e, 1)
		time.Sleep(time.Minute)
		synctest.Wait()

		times, _ := f.snapshot()
		if len(times) != 3 {
			t.Fatalf("%d attempts, want 3", len(times))
		}
		if d := times[1].Sub(times[0]); d != 5*time.Second {
			t.Errorf("waited %v after 503 with Retry-After: 5", d)
		}
		if d := times[2].Sub(times[1]); d < 1600*time.Millisecond || d > 2400*time.Millisecond {
			t.Errorf("backoff after the 2nd failure was %v, want about 2s", d)
		}
		if times[0].Sub(start) != time.Millisecond {
			t.Errorf("first attempt after %v", times[0].Sub(start))
		}
		if out := diag.String(); out != "" {
			t.Errorf("a batch that got through was reported: %s", out)
		}
		shutdown(t, e)
	})
}

func TestRetryGivesUpAfterExportTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{respond: func(int, *http.Request) (*http.Response, error) { return response(502, nil), nil }}
		var diag diagnostics
		e := newFake(f, Options{ScheduleDelay: time.Millisecond, ExportTimeout: 10 * time.Second, Diagnostics: diag.logger()})
		logN(e, 1)
		time.Sleep(time.Minute)
		synctest.Wait()
		times, _ := f.snapshot()
		if len(times) < 3 || times[len(times)-1].Sub(times[0]) > 10*time.Second {
			t.Errorf("attempts at %v", times)
		}
		if out := diag.String(); !strings.Contains(out, "no time left to retry") || !strings.Contains(out, "502 Bad Gateway") {
			t.Errorf("diagnostics %q", out)
		}
		shutdown(t, e)
	})
}

func TestShutdownDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{respond: func(_ int, r *http.Request) (*http.Response, error) {
			<-r.Context().Done() // a collector that never answers
			return nil, r.Context().Err()
		}}
		e := newFake(f, Options{ScheduleDelay: time.Hour, Timeout: time.Hour})
		logN(e, 3)
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Shutdown = %v, want DeadlineExceeded", err)
		}
		if d := time.Since(start); d != 5*time.Second {
			t.Errorf("Shutdown took %v, want the 5s deadline", d)
		}
	})
}

func TestLoggingNeverBlocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{respond: func(_ int, r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}}
		e := newFake(f, Options{MaxQueueSize: 10, MaxExportBatchSize: 1})
		start := time.Now()
		logN(e, 10_000)
		if d := time.Since(start); d != 0 {
			t.Errorf("logging took %v of fake time; it must not wait", d)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = e.Shutdown(ctx)
	})
}

func TestRetryAfterParsing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for in, want := range map[string]time.Duration{
			"":     0,
			"7":    7 * time.Second,
			"-3":   0,
			"soon": 0,
			time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat): 90 * time.Second,
		} {
			if got := retryAfter(in); got != want {
				t.Errorf("retryAfter(%q) = %v, want %v", in, got, want)
			}
		}
	})
}
