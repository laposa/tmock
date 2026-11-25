package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
)

type userKey struct{}

// WithUser returns ctx carrying u, as the guards do.
func WithUser(ctx context.Context, u app.User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// UserFrom returns the user a guard put into ctx.
func UserFrom(ctx context.Context) (app.User, bool) {
	u, ok := ctx.Value(userKey{}).(app.User)
	return u, ok
}

// RequireUser is the REST API's `user` guard (spec §6.3): without a valid session it
// answers 401; otherwise next runs with the user in the context (UserFrom).
func (s *Sessions) RequireUser(next http.Handler) http.Handler {
	return s.guard(false, next)
}

// RequireAdmin is the REST API's `admin` guard: 401 without a session, 403 for a user who
// is not an admin.
func (s *Sessions) RequireAdmin(next http.Handler) http.Handler {
	return s.guard(true, next)
}

func (s *Sessions) guard(admin bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.User(r)
		switch {
		case errors.Is(err, ErrNoSession):
			httpx.WriteErr(w, r, s.logger, app.Unauthorized("Not logged in"))
			return
		case err != nil:
			httpx.WriteErr(w, r, s.logger, err)
			return
		case admin && !u.Admin:
			httpx.WriteErr(w, r, s.logger, app.Forbidden("Only admins may do this"))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}
