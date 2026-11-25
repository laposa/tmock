// Package auth holds the sessions shared by the REST API and the UI (spec §9.2): the
// session cookie, session lookup, logout, the hourly sweep of expired sessions, and the
// `user` and `admin` guards of the REST API (spec §6.3).
//
// The cookie holds 32 random bytes; the database stores only their HMAC-SHA256 under
// SESSION_SECRET, so a leaked sessions table cannot be replayed. Sessions last 24 hours
// from login, with no sliding window. Every lookup reads the user with the session, so
// deleting a user or changing their admin flag takes effect on their next request.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/store"
)

const (
	// CookieName is the session cookie, shared by the API and the UI.
	CookieName = "tmock_session"
	// Lifetime is how long a session lasts after login (absolute).
	Lifetime = 24 * time.Hour
	// SweepInterval is how often expired sessions are deleted.
	SweepInterval = time.Hour

	tokenBytes = 32
)

// ErrNoSession means the request has no valid session: no cookie, a malformed one, or
// one that is unknown or expired.
var ErrNoSession = errors.New("no valid session")

// Store is the part of store.Sessions that Sessions needs. Tests use an in-memory fake.
type Store interface {
	Create(ctx context.Context, id []byte, userID int64, expires time.Time) error
	User(ctx context.Context, id []byte, now time.Time) (store.User, error)
	Delete(ctx context.Context, id []byte) (userID int64, err error)
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// Sessions manages the sessions. Create it once with New.
type Sessions struct {
	store  Store
	secret []byte
	secure bool
	logger *slog.Logger
}

// Options configures Sessions.
type Options struct {
	// Secret is SESSION_SECRET; session ids are HMAC-SHA256(Secret, cookie value).
	Secret []byte
	// Secure sets the cookie's Secure attribute (production).
	Secure bool
	// Logger receives the logout and sweep events. Nil means slog.Default().
	Logger *slog.Logger
}

// New returns the session manager on st.
func New(st Store, opts Options) *Sessions {
	if len(opts.Secret) == 0 {
		panic("auth.New: empty session secret") // config always provides one
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Sessions{store: st, secret: opts.Secret, secure: opts.Secure, logger: logger}
}

// Start creates a new session for userID and sets the session cookie on w. Each login
// gets a new session id (spec §9.2).
func (s *Sessions) Start(ctx context.Context, w http.ResponseWriter, userID int64) error {
	raw := make([]byte, tokenBytes)
	_, _ = rand.Read(raw) // never fails (crypto/rand)
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.store.Create(ctx, s.id(raw), userID, time.Now().Add(Lifetime)); err != nil {
		return fmt.Errorf("creating a session: %w", err)
	}
	http.SetCookie(w, s.cookie(token, int(Lifetime/time.Second)))
	return nil
}

// User returns the user of the request's session, or ErrNoSession. It records the user
// for the request log (logx.SetUserID).
func (s *Sessions) User(r *http.Request) (app.User, error) {
	raw, ok := tokenFrom(r)
	if !ok {
		return app.User{}, ErrNoSession
	}
	u, err := s.store.User(r.Context(), s.id(raw), time.Now())
	if errors.Is(err, store.ErrNotFound) {
		return app.User{}, ErrNoSession
	}
	if err != nil {
		return app.User{}, fmt.Errorf("loading the session: %w", err)
	}
	logx.SetUserID(r.Context(), u.ID)
	return app.UserFromStore(u), nil
}

// End deletes the request's session, if there is one, and expires the cookie. It logs
// tmock.auth.logout when a session was deleted. Ending without a session is not an
// error, so logging out twice is harmless.
func (s *Sessions) End(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, s.cookie("", -1))
	raw, ok := tokenFrom(r)
	if !ok {
		return nil
	}
	userID, err := s.store.Delete(r.Context(), s.id(raw))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("deleting the session: %w", err)
	}
	logx.SetUserID(r.Context(), userID)
	s.logger.LogAttrs(r.Context(), slog.LevelInfo, fmt.Sprintf("user %d logged out", userID),
		slog.String(logx.AttrEventName, logx.EventLogout),
		slog.Int64(logx.AttrUserID, userID),
	)
	return nil
}

// Sweep deletes the expired sessions and logs tmock.session.swept.
func (s *Sessions) Sweep(ctx context.Context) (int64, error) {
	n, err := s.store.DeleteExpired(ctx, time.Now())
	if err != nil {
		return 0, fmt.Errorf("sweeping expired sessions: %w", err)
	}
	s.logger.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("swept %d expired sessions", n),
		slog.String(logx.AttrEventName, logx.EventSessionsSwept),
		slog.Int64(logx.AttrSessionsDeleted, n),
	)
	return n, nil
}

// RunSweeper sweeps once at start and then every SweepInterval, until ctx is cancelled.
// Failures are logged and retried at the next interval.
func (s *Sessions) RunSweeper(ctx context.Context) {
	t := time.NewTicker(SweepInterval)
	defer t.Stop()
	for {
		if _, err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			s.logger.LogAttrs(ctx, slog.LevelWarn, err.Error(),
				slog.String(logx.AttrExceptionMessage, err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// id is the database key of a cookie value.
func (s *Sessions) id(raw []byte) []byte {
	m := hmac.New(sha256.New, s.secret)
	m.Write(raw)
	return m.Sum(nil)
}

func (s *Sessions) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// tokenFrom returns the decoded session cookie, if it is well-formed.
func tokenFrom(r *http.Request) ([]byte, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(raw) != tokenBytes {
		return nil, false
	}
	return raw, true
}
