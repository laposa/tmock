package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/password"
	"github.com/laposa/tmock/app-go/internal/store"
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
	for line := range bytes.SplitSeq(bytes.TrimSpace(b.buf.Bytes()), []byte("\n")) {
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

func TestRunNeedsDatabase(t *testing.T) {
	for _, cmd := range []string{"serve", "migrate", "seed"} {
		var out bytes.Buffer
		if code := run(context.Background(), []string{cmd}, envOf(nil), &out, io.Discard); code != 1 || !strings.Contains(out.String(), "no database configured") {
			t.Errorf("%s: exit %d, output %q", cmd, code, out.String())
		}
	}
	var errOut bytes.Buffer
	if code := run(context.Background(), []string{"migrate", "now"}, envOf(nil), io.Discard, &errOut); code != 2 || !strings.Contains(errOut.String(), "migrate takes no arguments") {
		t.Errorf("migrate now: exit %d, stderr %q", code, errOut.String())
	}
}

// dbEnv is the configuration for a run against a fresh test schema.
func dbEnv(t *testing.T) map[string]string {
	return map[string]string{"DATABASE_URL": dbtest.Schema(t), "LOG_FORMAT": "json"}
}

// runJSON runs a command and returns its exit code and the log records it wrote.
func runJSON(t *testing.T, ctx context.Context, env map[string]string, args ...string) (int, []map[string]any) {
	t.Helper()
	out := &syncBuffer{}
	code := run(ctx, args, envOf(env), out, io.Discard)
	return code, out.events(t)
}

func message(events []map[string]any) string {
	var msgs []string
	for _, e := range events {
		msgs = append(msgs, fmt.Sprint(e["msg"]))
	}
	return strings.Join(msgs, "\n")
}

// Verify items: migrate creates the schema and is a no-op the second time; seed creates
// the admin once and a second run does nothing.
func TestRunMigrateAndSeed(t *testing.T) {
	t.Parallel()
	env := dbEnv(t)
	ctx := context.Background()

	if code, events := runJSON(t, ctx, env, "seed"); code != 1 || !strings.Contains(message(events), "pending migrations: 0001") {
		t.Errorf("seed before migrate: exit %d\n%s", code, message(events))
	}

	code, events := runJSON(t, ctx, env, "migrate")
	if code != 0 || findEvent(events, logx.EventMigrationApplied) == nil {
		t.Fatalf("migrate: exit %d\n%s", code, message(events))
	}
	code, events = runJSON(t, ctx, env, "migrate")
	if code != 0 || findEvent(events, logx.EventMigrationApplied) != nil || !strings.Contains(message(events), "up to date") {
		t.Errorf("second migrate: exit %d\n%s", code, message(events))
	}

	code, events = runJSON(t, ctx, env, "seed")
	if code != 1 || !strings.Contains(message(events), "ADMIN_EMAIL is required") || !strings.Contains(message(events), "ADMIN_PASSWORD is required") {
		t.Errorf("seed without ADMIN_*: exit %d\n%s", code, message(events))
	}

	env["ADMIN_NAME"] = "Root"
	env["ADMIN_EMAIL"] = "Root@Example.com"
	env["ADMIN_PASSWORD"] = "correct horse"
	code, events = runJSON(t, ctx, env, "seed")
	e := findEvent(events, logx.EventUserSeeded)
	if code != 0 || e == nil || e["level"] != "INFO" || e[logx.AttrUserID] == nil {
		t.Fatalf("seed: exit %d, event %v\n%s", code, e, message(events))
	}
	for _, rec := range events {
		if b, _ := json.Marshal(rec); strings.Contains(string(b), "correct horse") || strings.Contains(string(b), "pbkdf2") {
			t.Errorf("the password or its hash was logged: %s", b)
		}
	}

	code, events = runJSON(t, ctx, env, "seed")
	if code != 0 || findEvent(events, logx.EventUserSeeded) != nil || !strings.Contains(message(events), "nothing to seed") {
		t.Errorf("second seed: exit %d\n%s", code, message(events))
	}

	pool := dbtest.OpenDSN(t, env["DATABASE_URL"])
	users, err := store.NewUsers(pool).List(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("users %+v, %v", users, err)
	}
	u := users[0]
	if u.Name != "Root" || u.Email != "Root@Example.com" || !u.Admin {
		t.Errorf("admin %+v", u)
	}
	if ok, rehash, err := password.Verify("correct horse", u.PasswordHash); !ok || rehash || err != nil {
		t.Errorf("stored hash: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
}

func TestRunServeChecksSchema(t *testing.T) {
	t.Parallel()
	env := dbEnv(t)
	env["PORT"] = "1" // never reached: serve stops before listening
	if code, events := runJSON(t, context.Background(), env, "serve"); code != 1 || !strings.Contains(message(events), "MIGRATE_ON_START=true") {
		t.Errorf("serve on an empty schema: exit %d\n%s", code, message(events))
	}

	// With MIGRATE_ON_START it migrates and then serves; the test stops the server as
	// soon as it reports that it is listening.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	env["PORT"] = fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()
	env["MIGRATE_ON_START"] = "true"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &stopOnStart{cancel: cancel}
	code := run(ctx, []string{"serve"}, envOf(env), out, io.Discard)
	events := out.events(t)
	if code != 0 || findEvent(events, logx.EventMigrationApplied) == nil || findEvent(events, logx.EventServerStopped) == nil {
		t.Errorf("serve with MIGRATE_ON_START: exit %d\n%s", code, message(events))
	}
}

// stopOnStart cancels the server's context once it logs tmock.server.started.
type stopOnStart struct {
	syncBuffer
	cancel context.CancelFunc
}

func (s *stopOnStart) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte(logx.EventServerStarted)) {
		s.cancel()
	}
	return s.syncBuffer.Write(p)
}
