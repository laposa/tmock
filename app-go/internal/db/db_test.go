package db_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/laposa/tmock/app-go/internal/db"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
)

func TestRedact(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"postgres://u:hunter2@h:5432/d?sslmode=disable": "postgres://u:xxxxx@h:5432/d?sslmode=disable",
		"postgres://u@h/d": "postgres://u@h/d",
		"postgres://u@h/d?password=hunter2&sslmode=disable": "postgres://u@h/d?password=xxxxx&sslmode=disable",
		"host=h user=u password=hunter2":                    "(DSN in key=value form)",
	} {
		if got := db.Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapErrorPassesOtherErrors(t *testing.T) {
	t.Parallel()
	other := errors.New("boom")
	if db.MapError(nil) != nil || db.MapError(other) != other || db.MapError(sql.ErrNoRows) != sql.ErrNoRows {
		t.Error("MapError must return non-constraint errors unchanged")
	}
}

// Verify item (spec §3.3): an unreachable database fails start-up with a clear message,
// and the message never contains the password.
func TestOpenUnreachable(t *testing.T) {
	t.Parallel()
	// A port that was just free: nothing listens there, so the connection is refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	start := time.Now()
	_, err = db.Open(context.Background(), "postgres://tmock:hunter2@"+addr+"/tmock?sslmode=disable&connect_timeout=5")
	if err == nil {
		t.Fatal("expected an error")
	}
	t.Logf("%s after %s", err, time.Since(start))
	msg := err.Error()
	for _, want := range []string{"cannot connect to the database at postgres://tmock:xxxxx@" + addr + "/tmock", "PostgreSQL is running"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "hunter2") {
		t.Errorf("error leaks the password:\n%s", msg)
	}
}

// Verify item: DATABASE_ENABLE_SSL=true (sslmode=require) against a server without TLS
// fails with a clear error. The compose PostgreSQL has no TLS.
func TestOpenRequireTLSAgainstPlainServer(t *testing.T) {
	t.Parallel()
	u, err := url.Parse(dbtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	var tls bool
	pool := dbtest.OpenDSN(t, u.String())
	if err := pool.QueryRow(`SELECT current_setting('ssl') = 'on'`).Scan(&tls); err != nil {
		t.Fatal(err)
	}
	if tls {
		t.Skip("the test server has TLS enabled")
	}

	q := u.Query()
	q.Set("sslmode", "require")
	u.RawQuery = q.Encode()
	_, err = db.Open(context.Background(), u.String())
	if err == nil || !strings.Contains(err.Error(), "the server does not accept TLS connections") {
		t.Errorf("got %v", err)
	}
}

func TestPoolSettings(t *testing.T) {
	t.Parallel()
	pool := dbtest.Open(t)
	if got := pool.Stats().MaxOpenConnections; got != 10 {
		t.Errorf("MaxOpenConnections = %d, want 10", got)
	}
}

// Verify item: NULLs, bytea and jsonb round-trip (spec §3.3).
func TestRoundTrips(t *testing.T) {
	t.Parallel()
	pool := dbtest.Open(t)
	ctx := context.Background()
	if _, err := pool.ExecContext(ctx, `CREATE TABLE rt (
		id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		t text, n integer, b boolean, j jsonb, by bytea, ts timestamptz)`); err != nil {
		t.Fatal(err)
	}

	t.Run("NULLs", func(t *testing.T) {
		var id int64
		err := pool.QueryRowContext(ctx, `INSERT INTO rt (t, n, b, j, by, ts) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			nil, sql.NullInt64{}, (*bool)(nil), nil, []byte(nil), sql.NullTime{}).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		var (
			s  sql.NullString
			n  sql.NullInt64
			b  *bool
			j  []byte
			by []byte
			ts sql.NullTime
		)
		err = pool.QueryRowContext(ctx, `SELECT t, n, b, j, by, ts FROM rt WHERE id = $1`, id).Scan(&s, &n, &b, &j, &by, &ts)
		if err != nil {
			t.Fatal(err)
		}
		if s.Valid || n.Valid || b != nil || j != nil || by != nil || ts.Valid {
			t.Errorf("expected NULLs, got %v %v %v %q %q %v", s, n, b, j, by, ts)
		}
		// database/sql cannot store NULL in a named byte slice such as json.RawMessage, so
		// nullable jsonb columns must be scanned into []byte (spec §3.3).
		var raw json.RawMessage
		if err := pool.QueryRowContext(ctx, `SELECT j FROM rt WHERE id = $1`, id).Scan(&raw); err == nil {
			t.Error("scanning NULL into json.RawMessage now works; spec §3.3 can be relaxed")
		}
		var nulls int
		if err := pool.QueryRowContext(ctx, `SELECT num_nulls(t, n, b, j, by, ts) FROM rt WHERE id = $1`, id).Scan(&nulls); err != nil || nulls != 6 {
			t.Errorf("stored %d NULLs (err %v), want 6", nulls, err)
		}
	})

	t.Run("values", func(t *testing.T) {
		all := make([]byte, 256)
		for i := range all {
			all[i] = byte(i)
		}
		when := time.Date(2026, 11, 24, 13, 14, 15, 123456000, time.UTC)
		doc := json.RawMessage(`{"Content-Type": "application/json", "X-Null": null, "n": [1, 2.5, "ü"]}`)

		var id int64
		err := pool.QueryRowContext(ctx, `INSERT INTO rt (t, n, b, j, by, ts) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			"", 0, false, doc, all, when).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		var (
			s   sql.NullString
			n   sql.NullInt64
			b   sql.NullBool
			j   []byte
			by  []byte
			ts  time.Time
			raw json.RawMessage
		)
		if err := pool.QueryRowContext(ctx, `SELECT t, n, b, j, by, ts, j FROM rt WHERE id = $1`, id).Scan(&s, &n, &b, &j, &by, &ts, &raw); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, j) {
			t.Errorf("json.RawMessage %s differs from []byte %s", raw, j)
		}
		if !s.Valid || s.String != "" || !n.Valid || n.Int64 != 0 || !b.Valid || b.Bool {
			t.Errorf("empty values must not become NULL: %v %v %v", s, n, b)
		}
		if !bytes.Equal(by, all) {
			t.Errorf("bytea: got %x", by)
		}
		if !ts.Equal(when) {
			t.Errorf("timestamptz: got %v, want %v", ts, when)
		}
		var got, want any
		if err := json.Unmarshal(j, &got); err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(doc, &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("jsonb: got %s", j)
		}

		// An empty bytea is not NULL.
		var empty []byte
		var isNull bool
		if err := pool.QueryRowContext(ctx, `SELECT $1::bytea, $1::bytea IS NULL`, []byte{}).Scan(&empty, &isNull); err != nil || isNull || len(empty) != 0 {
			t.Errorf("empty bytea: %q, NULL=%v, err %v", empty, isNull, err)
		}
	})
}

// Verify item: unique and foreign-key violations are mapped to typed errors.
func TestConstraintErrors(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	ctx := context.Background()

	var id int64
	if err := pool.QueryRowContext(ctx, `INSERT INTO services (path, name, upstream_url) VALUES ('users', 'Users', 'http://u') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}

	_, err := pool.ExecContext(ctx, `INSERT INTO services (path, name, upstream_url) VALUES ('users', 'Again', 'http://u')`)
	err = db.MapError(err)
	var ce *db.ConstraintError
	if !errors.Is(err, db.ErrUnique) || errors.Is(err, db.ErrForeignKey) || !errors.As(err, &ce) {
		t.Fatalf("duplicate path: %v", err)
	}
	if ce.Table != "services" || ce.Constraint != "services_path_key" {
		t.Errorf("unique: table %q, constraint %q", ce.Table, ce.Constraint)
	}

	_, err = pool.ExecContext(ctx, `INSERT INTO scenarios (service_id, name) VALUES ($1, 'x')`, id+1000)
	err = db.MapError(err)
	if !errors.Is(err, db.ErrForeignKey) || errors.Is(err, db.ErrUnique) || !errors.As(err, &ce) {
		t.Fatalf("missing service: %v", err)
	}
	if ce.Table != "scenarios" || ce.Constraint != "scenarios_service_id_fkey" {
		t.Errorf("foreign key: table %q, constraint %q", ce.Table, ce.Constraint)
	}
	if strings.Contains(err.Error(), "1000") {
		t.Errorf("Error() must not include the DETAIL values: %v", err)
	}

	// A CHECK violation is not mapped.
	_, err = pool.ExecContext(ctx, `INSERT INTO services (path, name, upstream_url) VALUES ('bad path', 'x', 'http://u')`)
	if err == nil || errors.Is(db.MapError(err), db.ErrUnique) || errors.Is(db.MapError(err), db.ErrForeignKey) {
		t.Errorf("check violation: %v", err)
	}
}

// Verify item: a context timeout cancels a running query.
func TestContextTimeoutCancelsQuery(t *testing.T) {
	t.Parallel()
	pool := dbtest.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := pool.ExecContext(ctx, `SELECT pg_sleep(30)`)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the query ran for %s after its deadline", d)
	}

	// The pool is still usable.
	var one int
	if err := pool.QueryRow(`SELECT 1`).Scan(&one); err != nil || one != 1 {
		t.Fatalf("pool after cancel: %v", err)
	}
}
