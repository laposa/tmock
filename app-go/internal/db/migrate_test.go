package db_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/laposa/tmock/app-go/internal/db"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/logx"
)

// capture is a goroutine-safe JSON log sink.
type capture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *capture) logger() *slog.Logger { return slog.New(slog.NewJSONHandler(c, nil)) }

func (c *capture) events(t *testing.T, name string) []map[string]any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(c.buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if m[logx.AttrEventName] == name {
			out = append(out, m)
		}
	}
	return out
}

func TestLoadEmbedded(t *testing.T) {
	t.Parallel()
	ms, err := db.Load(db.Migrations)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 || ms[0].Version != "0001" || ms[0].Name != "init" || !strings.Contains(ms[0].SQL, "CREATE TABLE users") {
		t.Fatalf("migrations %+v", ms)
	}
}

func TestLoadRejectsBadNames(t *testing.T) {
	t.Parallel()
	for name, fsys := range map[string]fstest.MapFS{
		"no version":    {"init.sql": {}},
		"short version": {"1_init.sql": {}},
		"upper case":    {"0001_Init.sql": {}},
		"not sql":       {"0001_init.txt": {}},
		"duplicate":     {"0001_a.sql": {}, "0001_b.sql": {}},
		"sub-directory": {"0001_dir/x.sql": {}},
	} {
		if _, err := db.Load(fsys); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	ms, err := db.Load(fstest.MapFS{"0002_b.sql": {}, "0001_a.sql": {}, "0010_c.sql": {}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range ms {
		got = append(got, m.Version+"_"+m.Name)
	}
	if !slices.Equal(got, []string{"0001_a", "0002_b", "0010_c"}) {
		t.Errorf("order %v", got)
	}
}

// Verify item: a fresh DB is migrated to the schema of spec §5.1; a second run is a no-op.
func TestMigrateFreshAndAgain(t *testing.T) {
	t.Parallel()
	pool := dbtest.Open(t)
	ctx := context.Background()
	logs := &capture{}

	if v, err := db.SchemaVersion(ctx, pool); err != nil || v != "" {
		t.Fatalf("fresh schema version %q, %v", v, err)
	}
	if p, err := db.Pending(ctx, pool); err != nil || !slices.Contains(p, "0001") {
		t.Fatalf("pending on a fresh schema: %v, %v", p, err)
	}

	applied, err := db.Migrate(ctx, pool, logs.logger())
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) == 0 || applied[0] != "0001" {
		t.Errorf("applied %v", applied)
	}
	for _, table := range []string{"services", "scenarios", "clients", "client_scenarios", "users", "sessions", "schema_migrations"} {
		var exists bool
		if err := pool.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Errorf("table %s missing (%v)", table, err)
		}
	}
	for _, index := range []string{"users_email_key", "sessions_expires_at_idx"} {
		var exists bool
		if err := pool.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, index).Scan(&exists); err != nil || !exists {
			t.Errorf("index %s missing (%v)", index, err)
		}
	}

	events := logs.events(t, logx.EventMigrationApplied)
	if len(events) != len(applied) {
		t.Fatalf("%d events for %d migrations", len(events), len(applied))
	}
	e := events[0]
	if e["level"] != "INFO" || e[logx.AttrMigrationVersion] != "0001" || e[logx.AttrMigrationName] != "init" || e[logx.AttrDurationMS] == nil {
		t.Errorf("event %v", e)
	}

	again := &capture{}
	applied, err = db.Migrate(ctx, pool, again.logger())
	if err != nil || len(applied) != 0 {
		t.Errorf("second run applied %v, %v", applied, err)
	}
	if n := len(again.events(t, logx.EventMigrationApplied)); n != 0 {
		t.Errorf("second run logged %d events", n)
	}
	if p, err := db.Pending(ctx, pool); err != nil || len(p) != 0 {
		t.Errorf("pending after migrate: %v, %v", p, err)
	}
}

// Verify item: two concurrent runs don't conflict; each migration is applied once.
func TestMigrateConcurrent(t *testing.T) {
	t.Parallel()
	dsn := dbtest.Schema(t)
	logs := &capture{}
	const runs = 4
	var wg sync.WaitGroup
	errs := make([]error, runs)
	for i := range runs {
		pool := dbtest.OpenDSN(t, dsn) // separate pools, like separate processes
		wg.Go(func() { _, errs[i] = db.Migrate(context.Background(), pool, logs.logger()) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("run %d: %v", i, err)
		}
	}
	ms, _ := db.Load(db.Migrations)
	if n := len(logs.events(t, logx.EventMigrationApplied)); n != len(ms) {
		t.Errorf("%d applied events, want %d", n, len(ms))
	}
}

// A failing migration is rolled back and not recorded; earlier ones stay applied.
func TestMigrateFailureRollsBack(t *testing.T) {
	t.Parallel()
	pool := dbtest.Open(t)
	ctx := context.Background()
	fsys := fstest.MapFS{
		"0001_ok.sql":  {Data: []byte(`CREATE TABLE a (id int);`)},
		"0002_bad.sql": {Data: []byte(`CREATE TABLE b (id int); SELECT no_such_function();`)},
	}
	applied, err := db.MigrateFS(ctx, pool, fsys, (&capture{}).logger())
	if err == nil || !strings.Contains(err.Error(), "migration 0002_bad failed") {
		t.Fatalf("got %v", err)
	}
	if !slices.Equal(applied, []string{"0001"}) {
		t.Errorf("applied %v", applied)
	}
	var a, b bool
	_ = pool.QueryRow(`SELECT to_regclass('a') IS NOT NULL, to_regclass('b') IS NOT NULL`).Scan(&a, &b)
	if !a || b {
		t.Errorf("table a exists %v (want true), b exists %v (want false)", a, b)
	}
	if v, _ := db.SchemaVersion(ctx, pool); v != "0001" {
		t.Errorf("schema version %q", v)
	}

	// The lock was released: fixing the file lets the next run continue.
	fsys["0002_bad.sql"] = &fstest.MapFile{Data: []byte(`CREATE TABLE b (id int);`)}
	if applied, err := db.MigrateFS(ctx, pool, fsys, (&capture{}).logger()); err != nil || !slices.Equal(applied, []string{"0002"}) {
		t.Errorf("retry: %v, %v", applied, err)
	}
}
