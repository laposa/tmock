// Package dbtest gives DB integration tests a database of their own (spec §11.2).
//
// Tests run only when TMOCK_TEST_DATABASE_URL is set (make test-db); otherwise they are
// skipped. Each test gets a fresh schema t_<random>, used through search_path and dropped
// when the test ends, so tests can run in parallel against one database.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"

	"github.com/laposa/tmock/app-go/internal/db"
)

// EnvVar holds the DSN of the test database.
const EnvVar = "TMOCK_TEST_DATABASE_URL"

// URL returns the test database's DSN, or skips the test if it is not configured.
func URL(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv(EnvVar)
	if dsn == "" {
		t.Skip(EnvVar + " is not set; skipping DB test (run make test-db)")
	}
	return dsn
}

// Schema creates a fresh schema, dropped when the test ends, and returns a DSN whose
// search_path points at it.
func Schema(t testing.TB) string {
	t.Helper()
	base := URL(t)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("%s must be a URL: %v", EnvVar, err)
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	schema := "t_" + hex.EncodeToString(b[:])

	admin, err := db.Open(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Errorf("dropping schema %s: %v", schema, err)
		}
	})

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

// Open returns a pool on a fresh, empty schema. It is closed when the test ends.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	return OpenDSN(t, Schema(t))
}

// OpenDSN opens a pool on dsn, closed when the test ends.
func OpenDSN(t testing.TB, dsn string) *sql.DB {
	t.Helper()
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// Migrated returns a pool on a fresh schema with every migration applied.
func Migrated(t testing.TB) *sql.DB {
	t.Helper()
	pool := Open(t)
	if _, err := db.Migrate(context.Background(), pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return pool
}
