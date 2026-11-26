package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

func TestUsers(t *testing.T) {
	t.Parallel()
	users := store.NewUsers(dbtest.Migrated(t))
	ctx := context.Background()

	if list, err := users.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("empty list: %v, %v", list, err)
	}
	ann, err := users.Create(ctx, store.NewUser{Name: "Ann", Email: "Ann@Example.com", PasswordHash: "h1", Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	if ann.ID == 0 || ann.Email != "Ann@Example.com" || !ann.Admin || ann.CreatedAt.IsZero() {
		t.Errorf("created %+v", ann)
	}
	bob, err := users.Create(ctx, store.NewUser{Name: "Bob", Email: "bob@example.com", PasswordHash: "h2"})
	if err != nil {
		t.Fatal(err)
	}

	// Emails are unique regardless of case.
	if _, err := users.Create(ctx, store.NewUser{Name: "Ann 2", Email: "ann@EXAMPLE.com", PasswordHash: "h"}); !errors.Is(err, store.ErrUnique) {
		t.Errorf("duplicate email: %v", err)
	}

	if got, err := users.GetByEmail(ctx, "ANN@example.COM"); err != nil || got.ID != ann.ID {
		t.Errorf("GetByEmail: %+v, %v", got, err)
	}
	if _, err := users.GetByEmail(ctx, "nobody@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown email: %v", err)
	}
	if got, err := users.Get(ctx, bob.ID); err != nil || got != bob {
		t.Errorf("Get: %+v, %v", got, err)
	}
	if _, err := users.Get(ctx, bob.ID+100); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}

	list, err := users.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != ann.ID || list[1].ID != bob.ID {
		t.Errorf("List: %+v, %v", list, err)
	}
	if n, err := users.CountAdmins(ctx); err != nil || n != 1 {
		t.Errorf("CountAdmins: %d, %v", n, err)
	}

	if err := users.SetPasswordHash(ctx, bob.ID, "h3"); err != nil {
		t.Fatal(err)
	}
	if got, _ := users.Get(ctx, bob.ID); got.PasswordHash != "h3" {
		t.Errorf("hash after SetPasswordHash: %q", got.PasswordHash)
	}
	if err := users.SetPasswordHash(ctx, bob.ID+100, "h"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetPasswordHash on unknown id: %v", err)
	}
}

func TestUsersCreateFirst(t *testing.T) {
	t.Parallel()
	users := store.NewUsers(dbtest.Migrated(t))
	ctx := context.Background()

	// Concurrent calls create exactly one user.
	const n = 5
	var wg sync.WaitGroup
	created := make([]bool, n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			_, created[i], errs[i] = users.CreateFirst(ctx, store.NewUser{Name: "Admin", Email: "admin@example.com", PasswordHash: "h", Admin: true})
		})
	}
	wg.Wait()
	count := 0
	for i := range n {
		if errs[i] != nil {
			t.Errorf("call %d: %v", i, errs[i])
		}
		if created[i] {
			count++
		}
	}
	if count != 1 {
		t.Errorf("%d users created, want 1", count)
	}

	if _, ok, err := users.CreateFirst(ctx, store.NewUser{Name: "Other", Email: "other@example.com", PasswordHash: "h"}); ok || err != nil {
		t.Errorf("with users present: created=%v err=%v", ok, err)
	}
	if list, _ := users.List(ctx); len(list) != 1 || list[0].Email != "admin@example.com" {
		t.Errorf("users %+v", list)
	}
}

func TestUsersModify(t *testing.T) {
	t.Parallel()
	users := store.NewUsers(dbtest.Migrated(t))
	ctx := context.Background()
	bob, err := users.Create(ctx, store.NewUser{Name: "Bob", Email: "bob@example.com", PasswordHash: "h1"})
	if err != nil {
		t.Fatal(err)
	}
	same := func(cur store.User, _ int) (store.UserFields, error) {
		return store.UserFields{Name: cur.Name, Email: cur.Email, Admin: cur.Admin}, nil
	}
	if got, changed, err := users.Modify(ctx, bob.ID, same); err != nil || changed || got != bob {
		t.Errorf("no change: %+v, %v, %v", got, changed, err)
	}

	got, changed, err := users.Modify(ctx, bob.ID, func(cur store.User, admins int) (store.UserFields, error) {
		return store.UserFields{Name: "Robert", Email: cur.Email, Admin: true}, nil
	})
	if err != nil || !changed || got.Name != "Robert" || !got.Admin || got.PasswordHash != "h1" {
		t.Errorf("change: %+v, %v, %v", got, changed, err)
	}
	// A new password alone is a change.
	got, changed, err = users.Modify(ctx, bob.ID, func(cur store.User, _ int) (store.UserFields, error) {
		return store.UserFields{Name: cur.Name, Email: cur.Email, PasswordHash: "h2", Admin: cur.Admin}, nil
	})
	if err != nil || !changed || got.PasswordHash != "h2" {
		t.Errorf("password: %+v, %v, %v", got, changed, err)
	}

	refused := errors.New("refused")
	if _, _, err := users.Modify(ctx, bob.ID, func(store.User, int) (store.UserFields, error) {
		return store.UserFields{}, refused
	}); !errors.Is(err, refused) {
		t.Errorf("fn error: %v", err)
	}
	if _, _, err := users.Modify(ctx, bob.ID+100, same); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}
