// Package store holds the repositories: plain database/sql code over the schema of
// spec §5.1. Repositories do no validation and hold no business rules; that is
// internal/app's job (spec §2.1).
//
// Constraint violations are returned as ErrUnique and ErrForeignKey (match them with
// errors.Is), a missing row as ErrNotFound.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/laposa/tmock/app-go/internal/db"
)

// Errors returned by the repositories (spec §5.3).
var (
	ErrNotFound   = errors.New("not found")
	ErrUnique     = db.ErrUnique     // → 409
	ErrForeignKey = db.ErrForeignKey // → 400
)

// User is a row of the users table. PasswordHash is secret: never log or return it.
type User struct {
	ID           int64
	Name         string
	Email        string
	PasswordHash string
	Admin        bool
	CreatedAt    time.Time
}

// NewUser holds the fields set when creating a user.
type NewUser struct {
	Name         string
	Email        string
	PasswordHash string
	Admin        bool
}

// Users is the users repository.
type Users struct{ db *sql.DB }

// NewUsers returns the users repository on pool.
func NewUsers(pool *sql.DB) *Users { return &Users{db: pool} }

const userColumns = `id, name, email, password_hash, admin, created_at`

// Create inserts a user. An email that is already taken (compared case-insensitively)
// returns ErrUnique.
func (r *Users) Create(ctx context.Context, u NewUser) (User, error) {
	row := r.db.QueryRowContext(ctx,
		`INSERT INTO users (name, email, password_hash, admin) VALUES ($1, $2, $3, $4)
		 RETURNING `+userColumns,
		u.Name, u.Email, u.PasswordHash, u.Admin)
	return scanUser(row)
}

// CreateFirst inserts u only if the users table is empty, and reports whether it did.
// The table is locked for the check, so concurrent calls create at most one user.
func (r *Users) CreateFirst(ctx context.Context, u NewUser) (User, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, false, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	// EXCLUSIVE blocks other writers (and other CreateFirst calls) but not readers.
	if _, err := tx.ExecContext(ctx, `LOCK TABLE users IN EXCLUSIVE MODE`); err != nil {
		return User{}, false, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&exists); err != nil {
		return User{}, false, err
	}
	if exists {
		return User{}, false, nil
	}
	created, err := scanUser(tx.QueryRowContext(ctx,
		`INSERT INTO users (name, email, password_hash, admin) VALUES ($1, $2, $3, $4)
		 RETURNING `+userColumns,
		u.Name, u.Email, u.PasswordHash, u.Admin))
	if err != nil {
		return User{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, false, db.MapError(err)
	}
	return created, true, nil
}

// Get returns the user with id, or ErrNotFound.
func (r *Users) Get(ctx context.Context, id int64) (User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

// GetByEmail looks a user up by email, ignoring case (spec §9.1), or returns ErrNotFound.
func (r *Users) GetByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email))
}

// List returns every user ORDER BY id (spec §5.3).
func (r *Users) List(ctx context.Context) ([]User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Any reports whether at least one user exists.
func (r *Users) Any(ctx context.Context) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&exists)
	return exists, err
}

// CountAdmins returns the number of admins, for the delete and demote rules (spec §5.3).
func (r *Users) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE admin`).Scan(&n)
	return n, err
}

// SetPasswordHash replaces a user's password hash, for example after a re-hash on login
// (spec §9.1). It returns ErrNotFound if the user does not exist.
func (r *Users) SetPasswordHash(ctx context.Context, id int64, hash string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, hash)
	if err != nil {
		return err
	}
	return oneRow(res)
}

// UserFields are the columns an update sets. An empty PasswordHash keeps the password.
type UserFields struct {
	Name         string
	Email        string
	PasswordHash string
	Admin        bool
}

// Modify is a read-modify-write of the user with id, for the rules that depend on the
// other users (spec §6.4: the last admin stays an admin). In one transaction it takes a
// lock that every other Modify and Delete also takes, so they run one after the other;
// sessions and logins can still read users meanwhile. fn gets the user and the number of
// admins, and returns the fields to store; an error from fn is returned as it is and
// nothing changes. fn must not use the database. Modify returns the user as stored,
// ErrNotFound, or ErrUnique for a taken email.
func (r *Users) Modify(ctx context.Context, id int64, fn func(cur User, admins int) (UserFields, error)) (User, error) {
	var out User
	err := r.locked(ctx, id, func(tx *sql.Tx, cur User, admins int) error {
		f, err := fn(cur, admins)
		if err != nil {
			return err
		}
		hash := f.PasswordHash
		if hash == "" {
			hash = cur.PasswordHash
		}
		out, err = scanUser(tx.QueryRowContext(ctx,
			`UPDATE users SET name = $2, email = $3, password_hash = $4, admin = $5 WHERE id = $1 RETURNING `+userColumns,
			id, f.Name, f.Email, hash, f.Admin))
		return err
	})
	return out, err
}

// Delete removes the user with id, with their sessions (cascade), under the lock of
// Modify, if check (given the user and the number of admins) returns nil; otherwise it
// returns check's error and deletes nothing. It returns the user as they were, or
// ErrNotFound.
func (r *Users) Delete(ctx context.Context, id int64, check func(cur User, admins int) error) (User, error) {
	var out User
	err := r.locked(ctx, id, func(tx *sql.Tx, cur User, admins int) error {
		if err := check(cur, admins); err != nil {
			return err
		}
		out = cur
		_, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
		return db.MapError(err)
	})
	return out, err
}

// locked runs fn in a transaction that holds the users write lock (SHARE ROW EXCLUSIVE
// conflicts with itself and with writes, not with reads), with the user and the number
// of admins as they are under the lock.
func (r *Users) locked(ctx context.Context, id int64, fn func(tx *sql.Tx, cur User, admins int) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return db.MapError(err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if _, err := tx.ExecContext(ctx, `LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return db.MapError(err)
	}
	cur, err := scanUser(tx.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	if err != nil {
		return err
	}
	var admins int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE admin`).Scan(&admins); err != nil {
		return db.MapError(err)
	}
	if err := fn(tx, cur, admins); err != nil {
		return err
	}
	return db.MapError(tx.Commit())
}

type scanner interface{ Scan(dest ...any) error }

func scanUser(s scanner) (User, error) {
	var u User
	err := s.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Admin, &u.CreatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return User{}, ErrNotFound
	case err != nil:
		return User{}, db.MapError(err)
	}
	return u, nil
}

func oneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
