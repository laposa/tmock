package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/laposa/tmock/app-go/internal/config"
	"github.com/laposa/tmock/app-go/internal/logx"
)

func envOf(m map[string]string) config.Lookup { return config.MapLookup(m) }

func TestRunCommands(t *testing.T) {
	cases := []struct {
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{[]string{"help"}, 0, "Usage: tmock", ""},
		{[]string{"--help"}, 0, "Usage: tmock", ""},
		{[]string{"bogus"}, 2, "", `unknown command "bogus"`},
		{[]string{"serve", "extra"}, 2, "", "serve takes no arguments"},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		code := run(context.Background(), tc.args, envOf(nil), &out, &errOut)
		if code != tc.wantCode {
			t.Errorf("%v: exit code %d, want %d", tc.args, code, tc.wantCode)
		}
		if !strings.Contains(out.String(), tc.wantStdout) || !strings.Contains(errOut.String(), tc.wantStderr) {
			t.Errorf("%v: stdout %q, stderr %q", tc.args, out.String(), errOut.String())
		}
	}
}

// Verify item: TMOCK_ENV=production without SESSION_SECRET exits with a clear error.
func TestRunRejectsInvalidConfig(t *testing.T) {
	var errOut bytes.Buffer
	code := run(context.Background(), nil, envOf(map[string]string{"TMOCK_ENV": "production", "PORT": "nope"}), io.Discard, &errOut)
	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	for _, want := range []string{"tmock: invalid configuration:", "  - PORT: must be a whole number", "  - SESSION_SECRET: is required when TMOCK_ENV=production"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, errOut.String())
		}
	}
}

// Verify item: a grpc protocol or malformed OTLP headers stop start-up.
func TestRunRejectsUnsupportedOTLP(t *testing.T) {
	for _, env := range []map[string]string{
		{"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc"},
		{"OTEL_EXPORTER_OTLP_HEADERS": "api-key"},
	} {
		var errOut bytes.Buffer
		if code := run(context.Background(), nil, envOf(env), io.Discard, &errOut); code != 1 || !strings.Contains(errOut.String(), "OTEL_EXPORTER_OTLP_") {
			t.Errorf("%v: exit %d, stderr %q", env, code, errOut.String())
		}
	}
}

func TestLoadConfigReadsDotenv(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("PORT=4000\nLOG_LEVEL=debug\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(envOf(map[string]string{"LOG_LEVEL": "warn"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 4000 || cfg.LogLevel != slog.LevelWarn {
		t.Errorf("port %d (from .env), level %v (environment must win)", cfg.Port, cfg.LogLevel)
	}

	if err := os.WriteFile(".env", []byte("PORT 4000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(envOf(nil)); err == nil || !strings.Contains(err.Error(), ".env line 1") {
		t.Errorf("bad .env: %v", err)
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) events(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(b.buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func findEvent(events []map[string]any, name string) map[string]any {
	for _, e := range events {
		if e[logx.AttrEventName] == name {
			return e
		}
	}
	return nil
}

type serveResult struct {
	err  error
	done chan struct{}
}

func startServe(t *testing.T, ctx context.Context, h http.Handler, drain time.Duration) (string, *syncBuffer, *serveResult) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	res := &serveResult{done: make(chan struct{})}
	go func() {
		defer close(res.done)
		res.err = serve(ctx, logger, ln, h, "test", drain)
	}()
	return "http://" + ln.Addr().String(), logs, res
}

// Verify item: shutting down lets a slow request finish.
func TestServeDrainsOpenRequests(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	})
	ctx, cancel := context.WithCancel(context.Background())
	url, logs, res := startServe(t, ctx, slow, 10*time.Second)

	type reply struct {
		body string
		err  error
	}
	replies := make(chan reply, 1)
	go func() {
		resp, err := http.Get(url)
		if err != nil {
			replies <- reply{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		replies <- reply{string(b), err}
	}()
	<-started
	cancel() // like Ctrl-C

	select {
	case <-res.done:
		t.Fatal("serve returned while a request was still running")
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := net.DialTimeout("tcp", strings.TrimPrefix(url, "http://"), time.Second); err == nil {
		t.Error("new connections are still accepted during shutdown")
	}
	close(release)
	if r := <-replies; r.err != nil || r.body != "finished" {
		t.Errorf("slow request: %q, %v", r.body, r.err)
	}
	<-res.done
	if res.err != nil {
		t.Fatal(res.err)
	}

	events := logs.events(t)
	if e := findEvent(events, logx.EventServerStarted); e == nil || e[logx.AttrEnv] != "test" || e[logx.AttrServerPort] == nil {
		t.Errorf("started event %v", e)
	}
	if e := findEvent(events, logx.EventServerStopped); e == nil || e[logx.AttrShutdownForced] != false {
		t.Errorf("stopped event %v", e)
	}
}

func TestServeForcesShutdownAfterDrainTimeout(t *testing.T) {
	started := make(chan struct{})
	stuck := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	url, logs, res := startServe(t, ctx, stuck, 50*time.Millisecond)
	go func() {
		if resp, err := http.Get(url); err == nil {
			resp.Body.Close()
		}
	}()
	<-started // the request is running
	cancel()
	select {
	case <-res.done:
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after the drain timeout")
	}
	if res.err != nil {
		t.Fatal(res.err)
	}
	if e := findEvent(logs.events(t), logx.EventServerStopped); e == nil || e[logx.AttrShutdownForced] != true {
		t.Errorf("stopped event %v", e)
	}
}

func TestResourceAttributes(t *testing.T) {
	cfg := config.Config{Env: "staging"}
	attrs := resourceAttributes(cfg)
	if attrs["service.name"] != "tmock" || attrs["deployment.environment.name"] != "staging" || attrs["service.version"] == "" {
		t.Errorf("defaults %v", attrs)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(attrs["service.instance.id"]) {
		t.Errorf("instance id %q is not a v4 UUID", attrs["service.instance.id"])
	}

	cfg.OTLP.ResourceAttributes = map[string]string{"service.name": "from-attrs", "deployment.environment.name": "prod-eu", "team": "core"}
	attrs = resourceAttributes(cfg)
	if attrs["service.name"] != "from-attrs" || attrs["deployment.environment.name"] != "prod-eu" || attrs["team"] != "core" {
		t.Errorf("overrides %v", attrs)
	}
	cfg.OTLP.ServiceName = "from-env"
	if got := resourceAttributes(cfg)["service.name"]; got != "from-env" {
		t.Errorf("OTEL_SERVICE_NAME must win, got %q", got)
	}
}
