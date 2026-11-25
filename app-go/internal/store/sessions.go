package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/laposa/tmock/app-go/internal/db"
)

// Sessions is the sessions repository (spec §5.3, §9.2). A session id is the HMAC of the
// cookie value, computed by internal/auth; the repository never sees the cookie itself.
//
// Times are passed in rather than taken from the database's now(), so that expiry is
// decided by the application's clock, the same clock that set expires_at.
type Sessions struct{ db *sql.DB }

// NewSessions returns the sessions repository on pool.
func NewSessions(pool *sql.DB) *Sessions { return &Sessions{db: pool} }

// Create stores a session for userID that expires at expires. An unknown user returns
// ErrForeignKey.
func (r *Sessions) Create(ctx context.Context, id []byte, userID int64, expires time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3)`, id, userID, expires)
	return db.MapError(err)
}

// User returns the user of the session id if the session has not expired at now, or
// ErrNotFound. The user is read in the same query, so a deleted user (whose sessions
// the cascade removed) or a changed admin flag takes effect on the next request.
func (r *Sessions) User(ctx context.Context, id []byte, now time.Time) (User, error) {
	return scanUser(r.db.QueryRowContext(ctx,
		`SELECT u.id, u.name, u.email, u.password_hash, u.admin, u.created_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.id = $1 AND s.expires_at > $2`, id, now))
}

// Delete removes the session id and returns its user's id, or ErrNotFound if there is no
// such session (expired sessions count as existing until they are swept).
func (r *Sessions) Delete(ctx context.Context, id []byte) (userID int64, err error) {
	err = r.db.QueryRowContext(ctx, `DELETE FROM sessions WHERE id = $1 RETURNING user_id`, id).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return userID, db.MapError(err)
}

// DeleteExpired removes every session that has expired at now and returns how many.
func (r *Sessions) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now)
	if err != nil {
		return 0, db.MapError(err)
	}
	return res.RowsAffected()
}
