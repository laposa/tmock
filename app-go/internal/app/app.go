// Package app holds the use cases shared by the REST API and the UI (spec §2.1). It owns
// every business rule: validation and its messages, the enable rule, admin rules, password
// checks, token handling and cache invalidation. internal/api and internal/web are thin
// layers that decode input, call a use case and render its result; neither talks to
// internal/store directly.
//
// Conventions for use cases:
//
//   - A use case is a method on *App that takes a context.Context and plain input structs
//     (PATCH inputs use httpx.Optional[T] fields) and returns plain output structs.
//   - Expected failures are returned as *Error (Invalid, NotFound, Conflict, …), with a
//     message fit to show to the user; validation collects every problem with
//     httpx.Validator and returns Invalid(details). Store errors are translated with
//     StoreError. Anything else is unexpected and ends up as a logged 500.
//   - Changes are logged as tmock.entity.changed events (spec §2.4.2).
package app

import (
	"database/sql"
	"log/slog"

	"github.com/laposa/tmock/app-go/internal/password"
	"github.com/laposa/tmock/app-go/internal/store"
)

// App is the set of use cases. Create it once, with New, and share it between the REST
// API and the UI.
type App struct {
	logger   *slog.Logger
	hasher   password.Hasher
	users    *store.Users
	services *store.Services
}

// Options configures the use cases. The zero value is ready for production.
type Options struct {
	Logger *slog.Logger    // nil means slog.Default()
	Hasher password.Hasher // the zero value uses password.DefaultIterations
}

// New returns the use cases on pool.
func New(pool *sql.DB, opts Options) *App {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &App{logger: logger, hasher: opts.Hasher, users: store.NewUsers(pool), services: store.NewServices(pool)}
}

// User is a user as the API and the UI show it (spec §6.1): never with the password.
type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Admin bool   `json:"admin"`
}

// UserFromStore converts a users row. internal/auth uses it for the user of a session.
func UserFromStore(u store.User) User {
	return User{ID: u.ID, Name: u.Name, Email: u.Email, Admin: u.Admin}
}
