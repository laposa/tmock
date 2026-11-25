package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/laposa/tmock/app-go/internal/logx"
)

//go:embed migrations/*.sql
var embedded embed.FS

// Migrations are the schema migrations shipped with the binary.
var Migrations fs.FS = mustSub(embedded, "migrations")

// lockKey is the pg_advisory_lock key that serialises migration runs ("tmock" in ASCII).
const lockKey int64 = 0x746d6f636b

// Migration is one migration file, such as 0001_init.sql.
type Migration struct {
	Version string // "0001"
	Name    string // "init"
	SQL     string
}

var migrationFile = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// Load reads the migrations in fsys in version order. Every file must be named
// NNNN_name.sql, and versions must be unique.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, e := range entries {
		m := migrationFile.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			return nil, fmt.Errorf("migration %q: the name must look like 0001_name.sql", e.Name())
		}
		if len(out) > 0 && out[len(out)-1].Version == m[1] {
			return nil, fmt.Errorf("migration %q: version %s is used twice", e.Name(), m[1])
		}
		b, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Version: m[1], Name: m[2], SQL: string(b)})
	}
	return out, nil // fs.ReadDir sorts by name, which is version order
}

// Migrate applies the embedded migrations that have not been applied yet (spec §5.2) and
// returns the versions it applied.
func Migrate(ctx context.Context, pool *sql.DB, logger *slog.Logger) ([]string, error) {
	return MigrateFS(ctx, pool, Migrations, logger)
}

// MigrateFS applies the migrations in fsys that are not yet recorded in
// schema_migrations, in version order, each in its own transaction. The whole run holds a
// session-level advisory lock, so concurrent runs (for example several replicas starting
// at once) wait for each other instead of conflicting; a run that waited finds the work
// done and applies nothing.
//
// Each applied migration is logged as a tmock.db.migration.applied event. On failure the
// failing migration is rolled back and the error names it; earlier ones stay applied.
func MigrateFS(ctx context.Context, pool *sql.DB, fsys fs.FS, logger *slog.Logger) (applied []string, err error) {
	migrations, err := Load(fsys)
	if err != nil {
		return nil, err
	}

	// The advisory lock belongs to a session, so lock, migrate and unlock on one connection.
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return nil, fmt.Errorf("taking the migration lock: %w", err)
	}
	defer func() {
		// Use a fresh context: ctx may be cancelled, and the lock must not outlive the run.
		// If the unlock fails, the connection is discarded, which releases the lock too.
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, uerr := conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock($1)`, lockKey); uerr != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, fmt.Errorf("creating schema_migrations: %w", err)
	}
	done, err := appliedVersions(ctx, conn)
	if err != nil {
		return nil, err
	}

	for _, m := range migrations {
		if done[m.Version] {
			continue
		}
		start := time.Now()
		if err := apply(ctx, conn, m); err != nil {
			return applied, fmt.Errorf("migration %s_%s failed (rolled back): %w", m.Version, m.Name, err)
		}
		applied = append(applied, m.Version)
		logger.LogAttrs(ctx, slog.LevelInfo, fmt.Sprintf("applied migration %s_%s", m.Version, m.Name),
			slog.String(logx.AttrEventName, logx.EventMigrationApplied),
			slog.String(logx.AttrMigrationVersion, m.Version),
			slog.String(logx.AttrMigrationName, m.Name),
			slog.Int64(logx.AttrDurationMS, time.Since(start).Milliseconds()),
		)
	}
	return applied, nil
}

func appliedVersions(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("reading schema_migrations: %w", err)
	}
	defer rows.Close()
	done := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		done[v] = true
	}
	return done, rows.Err()
}

func apply(ctx context.Context, conn *sql.Conn, m Migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	// Without arguments the statement is sent with the simple query protocol, so a file
	// may hold several statements.
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, m.Version); err != nil {
		return err
	}
	return tx.Commit()
}

// Pending returns the versions of the embedded migrations that are not applied yet.
func Pending(ctx context.Context, pool *sql.DB) ([]string, error) {
	return PendingFS(ctx, pool, Migrations)
}

// PendingFS returns the versions of the migrations in fsys that are not recorded in
// schema_migrations, using the same rule as MigrateFS: a version counts as applied only if
// it is recorded, so a lower version added after a higher one was applied is still pending.
// Without a schema_migrations table, every migration is pending.
func PendingFS(ctx context.Context, pool *sql.DB, fsys fs.FS) ([]string, error) {
	migrations, err := Load(fsys)
	if err != nil {
		return nil, err
	}
	done, err := appliedVersions(ctx, pool)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "42P01" { // undefined_table: nothing applied yet
		done, err = map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range migrations {
		if !done[m.Version] {
			out = append(out, m.Version)
		}
	}
	return out, nil
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
