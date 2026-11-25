package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/laposa/tmock/app-go/internal/config"
	"github.com/laposa/tmock/app-go/internal/db"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/password"
	"github.com/laposa/tmock/app-go/internal/store"
)

var errNoDatabase = errors.New("no database configured: set DATABASE_URL, or DATABASE_HOST, DATABASE_NAME and DATABASE_USER (see .env.example)")

// openDatabase connects to the configured database (spec §3.3).
func openDatabase(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, errNoDatabase
	}
	return db.Open(ctx, cfg.DatabaseURL)
}

// migrate applies pending migrations and says so when there were none.
func migrate(ctx context.Context, logger *slog.Logger, pool *sql.DB) error {
	applied, err := db.Migrate(ctx, pool, logger)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		logger.InfoContext(ctx, "the database schema is up to date")
	}
	return nil
}

// requireSchema fails when migrations are pending, so that serve and seed do not run
// against an old schema.
func requireSchema(ctx context.Context, pool *sql.DB) error {
	pending, err := db.Pending(ctx, pool)
	if err != nil {
		return fmt.Errorf("checking the database schema: %w", err)
	}
	if len(pending) > 0 {
		return fmt.Errorf("the database schema is not up to date (pending migrations: %s); run `tmock migrate` or set MIGRATE_ON_START=true",
			strings.Join(pending, ", "))
	}
	return nil
}

// seed creates the first admin from ADMIN_* if there are no users yet; otherwise it does
// nothing. ADMIN_EMAIL and ADMIN_PASSWORD are only required when a user is created.
func seed(ctx context.Context, logger *slog.Logger, pool *sql.DB, admin config.Admin, hasher password.Hasher) error {
	if err := requireSchema(ctx, pool); err != nil {
		return err
	}
	users := store.NewUsers(pool)
	exists, err := users.Any(ctx)
	if err != nil {
		return err
	}
	if exists {
		logger.InfoContext(ctx, "users already exist; nothing to seed")
		return nil
	}

	var problems []string
	if admin.Email == "" {
		problems = append(problems, "ADMIN_EMAIL is required to create the first admin")
	} else if !strings.Contains(admin.Email, "@") {
		problems = append(problems, "ADMIN_EMAIL must be an email address")
	}
	if admin.Password == "" {
		problems = append(problems, "ADMIN_PASSWORD is required to create the first admin")
	}
	if len(problems) > 0 {
		return fmt.Errorf("cannot create the first admin: %s", strings.Join(problems, "; "))
	}

	hash, err := hasher.Hash(admin.Password)
	if err != nil {
		return err
	}
	u, created, err := users.CreateFirst(ctx, store.NewUser{Name: admin.Name, Email: admin.Email, PasswordHash: hash, Admin: true})
	if err != nil {
		return fmt.Errorf("creating the admin user: %w", err)
	}
	if !created {
		logger.InfoContext(ctx, "another process created the first user meanwhile; nothing to seed")
		return nil
	}
	logger.LogAttrs(ctx, slog.LevelInfo, fmt.Sprintf("created the first admin user (id %d)", u.ID),
		slog.String(logx.AttrEventName, logx.EventUserSeeded),
		slog.Int64(logx.AttrUserID, u.ID),
	)
	return nil
}
