package store_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

func TestClients(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	services, scenarios, clients := store.NewServices(pool), store.NewScenarios(pool), store.NewClients(pool)
	ctx := context.Background()

	svcB, _ := services.Create(ctx, store.ServiceFields{Path: "b", Name: "B", UpstreamURL: "http://b"})
	svcA, _ := services.Create(ctx, store.ServiceFields{Path: "a", Name: "A", UpstreamURL: "http://a"})
	s1, _ := scenarios.Create(ctx, store.ScenarioFields{ServiceID: svcB.ID, Name: "z"})
	s2, _ := scenarios.Create(ctx, store.ScenarioFields{ServiceID: svcA.ID, Name: "y"})
	s3, _ := scenarios.Create(ctx, store.ScenarioFields{ServiceID: svcB.ID, Name: "x"})

	if list, err := clients.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("empty list: %v, %v", list, err)
	}
	cond := []byte(`{"ip": "10.0.0.1"}`)
	c, err := clients.Create(ctx, store.ClientFields{Name: "one", Enabled: true, Condition: cond}, []int64{s1.ID, s2.ID, s3.ID})
	if err != nil || c.ID == 0 || c.Name != "one" || !c.Enabled || string(c.Condition) != `{"ip": "10.0.0.1"}` || c.HasToken {
		t.Fatalf("create: %+v, %v", c, err)
	}
	// Assignments are ordered by service path, then scenario name.
	if len(c.Scenarios) != 3 || c.Scenarios[0] != (store.AssignedScenario{ID: s2.ID, ServicePath: "a", Name: "y"}) ||
		c.Scenarios[1].Name != "x" || c.Scenarios[2].Name != "z" {
		t.Errorf("assignments: %+v", c.Scenarios)
	}
	two, err := clients.Create(ctx, store.ClientFields{Name: "two"}, nil)
	if err != nil || two.Condition != nil || two.Enabled || len(two.Scenarios) != 0 {
		t.Errorf("create bare: %+v, %v", two, err)
	}
	if _, err := clients.Create(ctx, store.ClientFields{Name: "one"}, nil); !errors.Is(err, store.ErrUnique) {
		t.Errorf("duplicate name: %v", err)
	}
	if _, err := clients.Create(ctx, store.ClientFields{Name: "three"}, []int64{999999}); !errors.Is(err, store.ErrForeignKey) {
		t.Errorf("unknown scenario: %v", err)
	}
	if list, _ := clients.List(ctx); len(list) != 2 {
		t.Errorf("a failed create left a client behind: %+v", list)
	}

	// ORDER BY id, with assignments per client.
	list, err := clients.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != c.ID || len(list[0].Scenarios) != 3 || len(list[1].Scenarios) != 0 {
		t.Errorf("list: %+v, %v", list, err)
	}

	// Update: nil scenarios keeps them, an empty list clears them.
	u, err := clients.Update(ctx, c.ID, store.ClientFields{Name: "uno"}, nil)
	if err != nil || u.Name != "uno" || u.Enabled || u.Condition != nil || len(u.Scenarios) != 3 {
		t.Errorf("update keeping scenarios: %+v, %v", u, err)
	}
	u, err = clients.Update(ctx, c.ID, store.ClientFields{Name: "uno"}, []int64{s3.ID})
	if err != nil || len(u.Scenarios) != 1 || u.Scenarios[0].ID != s3.ID {
		t.Errorf("update replacing scenarios: %+v, %v", u, err)
	}
	if _, err := clients.Update(ctx, c.ID, store.ClientFields{Name: "uno"}, []int64{999999}); !errors.Is(err, store.ErrForeignKey) {
		t.Errorf("update with an unknown scenario: %v", err)
	}
	if g, _ := clients.Get(ctx, c.ID); len(g.Scenarios) != 1 {
		t.Errorf("a failed update changed the assignments: %+v", g.Scenarios)
	}
	if _, err := clients.Update(ctx, two.ID, store.ClientFields{Name: "uno"}, nil); !errors.Is(err, store.ErrUnique) {
		t.Errorf("rename onto a taken name: %v", err)
	}
	if _, err := clients.Update(ctx, 999999, store.ClientFields{Name: "x"}, nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}

	// Deleting a scenario removes its assignments.
	if _, err := scenarios.Delete(ctx, s3.ID); err != nil {
		t.Fatal(err)
	}
	if g, _ := clients.Get(ctx, c.ID); len(g.Scenarios) != 0 {
		t.Errorf("assignment of a deleted scenario: %+v", g.Scenarios)
	}

	// Tokens: only the hash is stored; find by hash; nil removes it.
	hash := sha256.Sum256([]byte("tmck_secret"))
	if name, err := clients.SetTokenHash(ctx, c.ID, hash[:]); err != nil || name != "uno" {
		t.Fatalf("set token: %q, %v", name, err)
	}
	if id, err := clients.FindByTokenHash(ctx, hash[:]); err != nil || id != c.ID {
		t.Errorf("find by token: %d, %v", id, err)
	}
	if g, _ := clients.Get(ctx, c.ID); !g.HasToken {
		t.Error("HasToken is false after setting a token")
	}
	var plain int
	if err := pool.QueryRow(`SELECT count(*) FROM clients WHERE position('tmck_secret'::bytea in token_hash) > 0`).Scan(&plain); err != nil || plain != 0 {
		t.Errorf("plaintext token stored: %d, %v", plain, err)
	}
	if _, err := clients.SetTokenHash(ctx, c.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := clients.FindByTokenHash(ctx, hash[:]); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("revoked token still found: %v", err)
	}
	if _, err := clients.SetTokenHash(ctx, 999999, hash[:]); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("token for a missing client: %v", err)
	}

	// Disable all.
	if _, err := clients.Update(ctx, two.ID, store.ClientFields{Name: "two", Enabled: true, Condition: cond}, nil); err != nil {
		t.Fatal(err)
	}
	if n, err := clients.DisableAll(ctx); err != nil || n != 1 {
		t.Errorf("disable all: %d, %v", n, err)
	}
	if g, _ := clients.Get(ctx, two.ID); g.Enabled {
		t.Error("still enabled after disable-all")
	}

	// Delete returns the client as it was; assignments go with it.
	if _, err := clients.Update(ctx, two.ID, store.ClientFields{Name: "two"}, []int64{s1.ID}); err != nil {
		t.Fatal(err)
	}
	d, err := clients.Delete(ctx, two.ID)
	if err != nil || d.Name != "two" || len(d.Scenarios) != 1 {
		t.Errorf("delete: %+v, %v", d, err)
	}
	var n int
	if err := pool.QueryRow(`SELECT count(*) FROM client_scenarios WHERE client_id = $1`, two.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("assignments left: %d, %v", n, err)
	}
	if _, err := clients.Delete(ctx, two.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}

	// ExistingScenarioIDs.
	exist, err := scenarios.ExistingScenarioIDs(ctx, []int64{s1.ID, s3.ID, 999999})
	if err != nil || !exist[s1.ID] || exist[s3.ID] || exist[999999] || len(exist) != 1 {
		t.Errorf("existing ids: %v, %v", exist, err)
	}
}
