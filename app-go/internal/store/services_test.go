package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

func TestServices(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	services := store.NewServices(pool)
	ctx := context.Background()

	if list, err := services.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("empty list: %v, %v", list, err)
	}
	b, err := services.Create(ctx, store.ServiceFields{Path: "b", Name: "Beta", UpstreamURL: "http://b.test"})
	if err != nil || b.ID == 0 || b.Path != "b" || b.Name != "Beta" || b.UpstreamURL != "http://b.test" {
		t.Fatalf("create: %+v, %v", b, err)
	}
	if _, err := services.Create(ctx, store.ServiceFields{Path: "a", Name: "Alpha", UpstreamURL: "http://a.test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := services.Create(ctx, store.ServiceFields{Path: "b", Name: "Other", UpstreamURL: "http://x"}); !errors.Is(err, store.ErrUnique) {
		t.Errorf("duplicate path: %v", err)
	}
	// The path is matched exactly: "B" is another service.
	if _, err := services.Get(ctx, "B"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("get B: %v", err)
	}

	// ORDER BY name.
	list, err := services.List(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "Alpha" || list[1].Name != "Beta" {
		t.Errorf("list: %+v, %v", list, err)
	}

	// Renaming the path keeps the id, so scenarios follow.
	if _, err := pool.Exec(`INSERT INTO scenarios (service_id, name) VALUES ($1, 's1')`, b.ID); err != nil {
		t.Fatal(err)
	}
	renamed, err := services.Update(ctx, "b", store.ServiceFields{Path: "beta", Name: "Beta 2", UpstreamURL: "https://b.test/base"})
	if err != nil || renamed.ID != b.ID || renamed.Path != "beta" || renamed.Name != "Beta 2" {
		t.Fatalf("update: %+v, %v", renamed, err)
	}
	if got, err := services.Get(ctx, "beta"); err != nil || got != renamed {
		t.Errorf("get after rename: %+v, %v", got, err)
	}
	if _, err := services.Get(ctx, "b"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("old path: %v", err)
	}
	if _, err := services.Update(ctx, "beta", store.ServiceFields{Path: "a", Name: "x", UpstreamURL: "http://x"}); !errors.Is(err, store.ErrUnique) {
		t.Errorf("rename onto a taken path: %v", err)
	}
	if _, err := services.Update(ctx, "missing", store.ServiceFields{Path: "m", Name: "x", UpstreamURL: "http://x"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}

	// Delete cascades to the scenarios.
	deleted, err := services.Delete(ctx, "beta")
	if err != nil || deleted.ID != b.ID {
		t.Fatalf("delete: %+v, %v", deleted, err)
	}
	var n int
	if err := pool.QueryRow(`SELECT count(*) FROM scenarios`).Scan(&n); err != nil || n != 0 {
		t.Errorf("scenarios left after delete: %d, %v", n, err)
	}
	if _, err := services.Delete(ctx, "beta"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}
