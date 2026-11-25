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

	"github.com/laposa/tmock/app-go/internal/store"
)

// App is the set of use cases. Create it once, with New, and share it between the REST
// API and the UI.
type App struct {
	logger *slog.Logger
	users  *store.Users
}

// New returns the use cases on pool. A nil logger means slog.Default().
func New(pool *sql.DB, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}
	return &App{logger: logger, users: store.NewUsers(pool)}
}
