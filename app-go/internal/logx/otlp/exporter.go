// Package otlp exports slog records as OpenTelemetry log records over OTLP/HTTP with the
// JSON encoding (spec §2.4.5). It is a small, standard-library replacement for the
// OpenTelemetry Go SDK's log pipeline.
//
// Records go into a bounded queue that one goroutine drains in batches. Logging never
// blocks: when the queue is full, records are dropped and counted. The exporter's own
// problems are reported to Options.Diagnostics, which must not feed back into the exporter.
package otlp

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/rand/v2"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/laposa/tmock/app-go/internal/logx"
)

// Options configures an Exporter. Zero values get the OpenTelemetry defaults.
type Options struct {
	Endpoint string            // full URL, e.g. http://localhost:4318/v1/logs
	Headers  map[string]string // added to every request
	Timeout  time.Duration     // per HTTP attempt (default 10s)
	Gzip     bool

	Resource     map[string]string // resource attributes
	ScopeName    string            // instrumentation scope
	ScopeVersion string

	ScheduleDelay      time.Duration // max wait before a batch is sent (default 1s)
	ExportTimeout      time.Duration // per batch, including retries (default 30s)
	MaxQueueSize       int           // default 2048
	MaxExportBatchSize int           // default 512, at most MaxQueueSize

	// Diagnostics receives export failures and drop counts. It must write to the console
	// only, never back into this exporter. Nil discards them.
	Diagnostics *slog.Logger

	// Client sends the requests. Nil means a client of the exporter's own, separate from
	// any other HTTP client in the process.
	Client *http.Client
}

// Exporter batches records and sends them to an OTLP/HTTP collector.
type Exporter struct {
	opts     Options
	client   *http.Client
	diag     *slog.Logger
	resource []keyValue
	now      func() time.Time

	mu      sync.RWMutex // guards stopped and the closing of queue
	stopped bool
	queue   chan logRecord

	runCtx    context.Context // cancelled when the shutdown deadline passes
	cancelRun context.CancelFunc
	done      chan struct{}

	droppedQueue  atomic.Int64 // queue full
	droppedFailed atomic.Int64 // batch could not be delivered
}

// dropReportInterval limits how often drop counts are reported (spec §2.4.5).
const dropReportInterval = time.Minute

// New starts an exporter. Call Shutdown to flush and stop it.
func New(opts Options) *Exporter {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.ScheduleDelay <= 0 {
		opts.ScheduleDelay = time.Second
	}
	if opts.ExportTimeout <= 0 {
		opts.ExportTimeout = 30 * time.Second
	}
	if opts.MaxQueueSize <= 0 {
		opts.MaxQueueSize = 2048
	}
	if opts.MaxExportBatchSize <= 0 {
		opts.MaxExportBatchSize = 512
	}
	opts.MaxExportBatchSize = min(opts.MaxExportBatchSize, opts.MaxQueueSize)

	e := &Exporter{
		opts:   opts,
		client: opts.Client,
		diag:   opts.Diagnostics,
		now:    time.Now,
		queue:  make(chan logRecord, opts.MaxQueueSize),
		done:   make(chan struct{}),
	}
	if e.client == nil {
		e.client = &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	}
	if e.diag == nil {
		e.diag = slog.New(slog.DiscardHandler)
	}
	for _, k := range slices.Sorted(maps.Keys(opts.Resource)) {
		e.resource = append(e.resource, keyValue{Key: k, Value: stringValue(opts.Resource[k])})
	}
	e.runCtx, e.cancelRun = context.WithCancel(context.Background())
	go e.run()
	return e
}

// Handler returns the slog.Handler that feeds this exporter.
func (e *Exporter) Handler() slog.Handler { return &handler{e: e} }

// Shutdown stops accepting records, sends everything still queued and stops the exporter.
// It returns ctx's error if the deadline passed before the queue was flushed; the records
// still waiting are then dropped. Calling it again does nothing.
func (e *Exporter) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		<-e.done
		return nil
	}
	e.stopped = true
	close(e.queue)
	e.mu.Unlock()

	stop := context.AfterFunc(ctx, e.cancelRun) // abort in-flight exports at the deadline
	defer stop()
	<-e.done
	e.cancelRun()
	return ctx.Err()
}

func (e *Exporter) enqueue(rec logRecord) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.stopped {
		return // after shutdown there is nobody left to send it
	}
	select {
	case e.queue <- rec:
	default:
		e.droppedQueue.Add(1)
	}
}

// run is the export goroutine: it collects batches and sends one at a time.
func (e *Exporter) run() {
	defer close(e.done)
	report := time.NewTicker(dropReportInterval)
	defer report.Stop()

	var (
		batch []logRecord
		timer *time.Timer
		due   <-chan time.Time
	)
	send := func() {
		if timer != nil {
			timer.Stop()
		}
		due = nil
		e.export(batch)
		batch = nil
	}
	for {
		select {
		case rec, ok := <-e.queue:
			if !ok { // Shutdown: the queue is drained
				if len(batch) > 0 {
					send()
				}
				e.reportDropped()
				return
			}
			batch = append(batch, rec)
			if len(batch) == 1 {
				timer = time.NewTimer(e.opts.ScheduleDelay)
				due = timer.C
			}
			if len(batch) >= e.opts.MaxExportBatchSize {
				send()
			}
		case <-due:
			send()
		case <-report.C:
			e.reportDropped()
		}
	}
}

// export sends one batch, retrying transient failures until ExportTimeout has passed.
func (e *Exporter) export(batch []logRecord) {
	ctx, cancel := context.WithTimeout(e.runCtx, e.opts.ExportTimeout)
	defer cancel()

	body, err := e.encode(batch)
	if err != nil {
		e.fail(len(batch), err)
		return
	}
	for attempt := 0; ; attempt++ {
		retry, wait, err := e.send(ctx, body)
		if err == nil {
			return
		}
		if !retry {
			e.fail(len(batch), err)
			return
		}
		if wait <= 0 {
			wait = backoff(attempt)
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < wait {
			e.fail(len(batch), fmt.Errorf("%w; no time left to retry within OTEL_BLRP_EXPORT_TIMEOUT", err))
			return
		}
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			e.fail(len(batch), fmt.Errorf("%w; stopped retrying: %w", err, ctx.Err()))
			return
		}
	}
}

func (e *Exporter) encode(batch []logRecord) ([]byte, error) {
	b, err := json.Marshal(exportRequest{ResourceLogs: []resourceLogs{{
		Resource: resource{Attributes: e.resource},
		ScopeLogs: []scopeLogs{{
			Scope:      scope{Name: e.opts.ScopeName, Version: e.opts.ScopeVersion},
			LogRecords: batch,
		}},
	}}})
	if err != nil || !e.opts.Gzip {
		return b, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// send makes one HTTP attempt. It reports whether a failure is worth retrying and, if the
// collector said so, how long to wait first.
func (e *Exporter) send(ctx context.Context, body []byte) (retry bool, wait time.Duration, err error) {
	attemptCtx, cancel := context.WithTimeout(ctx, e.opts.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, e.opts.Endpoint, bytes.NewReader(body))
	if err != nil {
		return false, 0, err
	}
	for k, v := range e.opts.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.opts.Gzip {
		req.Header.Set("Content-Encoding", "gzip")
	}
	req.Header.Set("User-Agent", "tmock/"+e.opts.ScopeVersion)

	res, err := e.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, 0, err // export timeout or shutdown: stop
		}
		return true, 0, err // network error or attempt timeout: retry
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))

	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		e.checkPartialSuccess(data)
		return false, 0, nil
	case res.StatusCode == http.StatusTooManyRequests, res.StatusCode == http.StatusBadGateway,
		res.StatusCode == http.StatusServiceUnavailable, res.StatusCode == http.StatusGatewayTimeout:
		return true, retryAfter(res.Header.Get("Retry-After")), fmt.Errorf("collector responded %s", res.Status)
	default:
		return false, 0, fmt.Errorf("collector responded %s: %s", res.Status, snippet(data))
	}
}

func (e *Exporter) checkPartialSuccess(data []byte) {
	var res exportResponse
	if json.Unmarshal(data, &res) != nil || res.PartialSuccess == nil || res.PartialSuccess.RejectedLogRecords <= 0 {
		return
	}
	e.diag.LogAttrs(context.Background(), slog.LevelWarn, "OTLP collector rejected some log records: "+res.PartialSuccess.ErrorMessage,
		slog.String(logx.AttrEventName, logx.EventOTLPPartialSuccess),
		slog.Int64(logx.AttrOTLPRejectedRecords, int64(res.PartialSuccess.RejectedLogRecords)),
		slog.String(logx.AttrOTLPEndpoint, e.opts.Endpoint),
	)
}

func (e *Exporter) fail(records int, err error) {
	e.droppedFailed.Add(int64(records))
	e.diag.LogAttrs(context.Background(), slog.LevelWarn, "OTLP log export failed, records dropped: "+err.Error(),
		slog.String(logx.AttrEventName, logx.EventOTLPExportFailed),
		slog.Int(logx.AttrOTLPRecords, records),
		slog.String(logx.AttrOTLPEndpoint, e.opts.Endpoint),
	)
}

func (e *Exporter) reportDropped() {
	queue, failed := e.droppedQueue.Swap(0), e.droppedFailed.Swap(0)
	if queue+failed == 0 {
		return
	}
	e.diag.LogAttrs(context.Background(), slog.LevelWarn,
		fmt.Sprintf("OTLP exporter dropped %d log records (queue full: %d, export failed: %d)", queue+failed, queue, failed),
		slog.String(logx.AttrEventName, logx.EventOTLPDropped),
		slog.Int64(logx.AttrOTLPDroppedQueue, queue),
		slog.Int64(logx.AttrOTLPDroppedFailed, failed),
	)
}

// backoff returns about 1s, 2s, 4s, … (capped at 32s), with ±20% jitter.
func backoff(attempt int) time.Duration {
	d := time.Second << min(attempt, 5)
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
}

// retryAfter parses a Retry-After header: seconds or an HTTP date. 0 means "not given".
func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil {
		return time.Duration(max(s, 0)) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0)
	}
	return 0
}

// snippet shortens a response body for an error message.
func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
