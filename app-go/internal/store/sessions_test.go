package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

func TestSessions(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	users, sessions := store.NewUsers(pool), store.NewSessions(pool)
	ctx := context.Background()

	ann, err := users.Create(ctx, store.NewUser{Name: "Ann", Email: "ann@example.com", PasswordHash: "h"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	id := []byte("session-1")
	if err := sessions.Create(ctx, id, ann.ID, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if u, err := sessions.User(ctx, id, now); err != nil || u.ID != ann.ID || u.Email != ann.Email {
		t.Errorf("User: %+v, %v", u, err)
	}
	if _, err := sessions.User(ctx, []byte("other"), now); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	// Expiry is absolute and decided by the time passed in.
	if _, err := sessions.User(ctx, id, now.Add(24*time.Hour)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("at expiry: %v", err)
	}

	// The admin flag is read with the session, so a change applies on the next request.
	if _, err := pool.Exec(`UPDATE users SET admin = true WHERE id = $1`, ann.ID); err != nil {
		t.Fatal(err)
	}
	if u, _ := sessions.User(ctx, id, now); !u.Admin {
		t.Error("admin flag not current")
	}

	if err := sessions.Create(ctx, []byte("x"), ann.ID+100, now); !errors.Is(err, store.ErrForeignKey) {
		t.Errorf("unknown user: %v", err)
	}
	if err := sessions.Create(ctx, id, ann.ID, now); !errors.Is(err, store.ErrUnique) {
		t.Errorf("duplicate id: %v", err)
	}

	if uid, err := sessions.Delete(ctx, id); err != nil || uid != ann.ID {
		t.Errorf("Delete: %d, %v", uid, err)
	}
	if _, err := sessions.Delete(ctx, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second Delete: %v", err)
	}
}

func TestSessionsSweepAndCascade(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	users, sessions := store.NewUsers(pool), store.NewSessions(pool)
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

	ann, _ := users.Create(ctx, store.NewUser{Name: "Ann", Email: "ann@example.com", PasswordHash: "h"})
	bob, _ := users.Create(ctx, store.NewUser{Name: "Bob", Email: "bob@example.com", PasswordHash: "h"})
	for id, exp := range map[string]time.Time{"old-1": now.Add(-time.Hour), "old-2": now, "live": now.Add(time.Minute)} {
		if err := sessions.Create(ctx, []byte(id), ann.ID, exp); err != nil {
			t.Fatal(err)
		}
	}
	if err := sessions.Create(ctx, []byte("bob"), bob.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if n, err := sessions.DeleteExpired(ctx, now); err != nil || n != 2 {
		t.Errorf("DeleteExpired: %d, %v", n, err)
	}
	if _, err := sessions.User(ctx, []byte("live"), now); err != nil {
		t.Errorf("live session swept: %v", err)
	}

	// Deleting a user ends their sessions (ON DELETE CASCADE).
	if _, err := pool.Exec(`DELETE FROM users WHERE id = $1`, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.User(ctx, []byte("bob"), now); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleted user's session: %v", err)
	}
}
