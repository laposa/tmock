// Package db opens the PostgreSQL connection pool, maps driver errors to typed errors and
// runs the schema migrations (spec §3.3, §5.2).
//
// It is the only package that imports pgx (spec §1.3). Everything else talks to the
// database through database/sql and the errors defined here.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// Pool settings (spec §3.3).
const (
	maxOpenConns    = 10
	maxIdleConns    = 10
	connMaxIdleTime = 5 * time.Minute
)

// PingTimeout bounds the start-up connection check.
const PingTimeout = 10 * time.Second

// Open opens the connection pool for dsn and checks that the database answers. The error
// says where it tried to connect (with the password masked) and what to check, so a
// misconfigured start-up is easy to diagnose.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("database configuration: %w", err)
	}
	pool.SetMaxOpenConns(maxOpenConns)
	pool.SetMaxIdleConns(maxIdleConns)
	pool.SetConnMaxIdleTime(connMaxIdleTime)

	ctx, cancel := context.WithTimeout(ctx, PingTimeout)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, connectError(dsn, err)
	}
	return pool, nil
}

// connectError explains a failed start-up ping.
func connectError(dsn string, err error) error {
	where := Redact(dsn)
	var hint string
	switch {
	case strings.Contains(err.Error(), "server refused TLS connection"):
		hint = "the server does not accept TLS connections, but TLS is required " +
			"(DATABASE_ENABLE_SSL=true or sslmode=require in DATABASE_URL); " +
			"enable TLS on the server or turn it off in the configuration"
	case errors.Is(err, context.DeadlineExceeded):
		hint = fmt.Sprintf("no answer within %s; check the host and port and that PostgreSQL is running", PingTimeout)
	default:
		hint = "check DATABASE_URL (or DATABASE_*) and that PostgreSQL is running (make db-up)"
	}
	return fmt.Errorf("cannot connect to the database at %s: %s (cause: %w)", where, hint, err)
}

// Redact returns dsn with its password masked, safe to log (spec §2.4.3). A DSN in the
// key=value form is reduced to a placeholder, because it cannot be masked reliably.
func Redact(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return "(DSN in key=value form)"
	}
	return u.Redacted()
}

// Errors returned by the store for constraint violations (spec §5.3). Match them with
// errors.Is; errors.As with *ConstraintError gives the constraint's name.
var (
	ErrUnique     = errors.New("unique constraint violated")      // SQLSTATE 23505 → 409
	ErrForeignKey = errors.New("foreign key constraint violated") // SQLSTATE 23503 → 400
)

// ConstraintError is a unique or foreign-key violation.
type ConstraintError struct {
	Kind       error  // ErrUnique or ErrForeignKey
	Table      string // the table the statement changed
	Constraint string // for example "users_email_key"
	Detail     string // PostgreSQL's DETAIL line; may contain the conflicting values
	err        error  // the driver's error
}

func (e *ConstraintError) Error() string {
	return fmt.Sprintf("%v: %s (constraint %s)", e.Kind, e.Table, e.Constraint)
}

// Is makes errors.Is(err, ErrUnique) and errors.Is(err, ErrForeignKey) work.
func (e *ConstraintError) Is(target error) bool { return target == e.Kind }

// Unwrap returns the driver's error.
func (e *ConstraintError) Unwrap() error { return e.err }

// MapError turns unique (23505) and foreign-key (23503) violations into a
// *ConstraintError and returns every other error unchanged. It returns nil for nil.
func MapError(err error) error {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return err
	}
	var kind error
	switch pg.Code {
	case "23505":
		kind = ErrUnique
	case "23503":
		kind = ErrForeignKey
	default:
		return err
	}
	return &ConstraintError{Kind: kind, Table: pg.TableName, Constraint: pg.ConstraintName, Detail: pg.Detail, err: err}
}
