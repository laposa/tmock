package store_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"strings"
	"sync"
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

// Modify locks the client's row from the read to the write: concurrent read-modify-writes
// of one client run one after the other, so each sees the one before and none is lost.
func TestClientModifyIsSerialised(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	clients := store.NewClients(pool)
	ctx := context.Background()
	c, err := clients.Create(ctx, store.ClientFields{Name: "n"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	const writers = 10
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := clients.Modify(ctx, c.ID, func(cur store.Client) (store.ClientFields, []int64, error) {
				return store.ClientFields{Name: cur.Name + "+"}, nil, nil
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := clients.Get(ctx, c.ID)
	if want := "n" + strings.Repeat("+", writers); err != nil || got.Name != want {
		t.Errorf("name %q, want %q (a write was lost): %v", got.Name, want, err)
	}

	// An error from fn changes nothing; a missing client is ErrNotFound before fn runs.
	boom := errors.New("boom")
	if _, err := clients.Modify(ctx, c.ID, func(store.Client) (store.ClientFields, []int64, error) {
		return store.ClientFields{Name: "changed"}, nil, boom
	}); !errors.Is(err, boom) {
		t.Errorf("fn error: %v", err)
	}
	if got, _ := clients.Get(ctx, c.ID); got.Name == "changed" {
		t.Error("a failed Modify changed the client")
	}
	if _, err := clients.Modify(ctx, 999999, func(store.Client) (store.ClientFields, []int64, error) {
		t.Error("fn ran for a missing client")
		return store.ClientFields{}, nil, nil
	}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("missing client: %v", err)
	}
}

// SetScenario changes one assignment and leaves the others; concurrent calls do not undo
// each other.
func TestClientSetScenario(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	services, scenarios, clients := store.NewServices(pool), store.NewScenarios(pool), store.NewClients(pool)
	ctx := context.Background()
	svc, err := services.Create(ctx, store.ServiceFields{Path: "s", Name: "S", UpstreamURL: "http://s"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i := range 10 {
		s, err := scenarios.Create(ctx, store.ScenarioFields{ServiceID: svc.ID, Name: "sc" + strconv.Itoa(i)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.ID)
	}
	c, err := clients.Create(ctx, store.ClientFields{Name: "c"}, []int64{ids[0]})
	if err != nil {
		t.Fatal(err)
	}

	got, changed, err := clients.SetScenario(ctx, c.ID, ids[1], true)
	if err != nil || !changed || len(got.Scenarios) != 2 {
		t.Errorf("assign: %+v, %v, %v", got.Scenarios, changed, err)
	}
	if _, changed, err := clients.SetScenario(ctx, c.ID, ids[1], true); err != nil || changed {
		t.Errorf("assign again: changed %v, %v", changed, err)
	}
	got, changed, err = clients.SetScenario(ctx, c.ID, ids[0], false)
	if err != nil || !changed || len(got.Scenarios) != 1 || got.Scenarios[0].ID != ids[1] {
		t.Errorf("remove: %+v, %v, %v", got.Scenarios, changed, err)
	}
	if _, changed, err := clients.SetScenario(ctx, c.ID, ids[0], false); err != nil || changed {
		t.Errorf("remove again: changed %v, %v", changed, err)
	}
	if _, _, err := clients.SetScenario(ctx, 999999, ids[0], true); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("missing client: %v", err)
	}
	if _, _, err := clients.SetScenario(ctx, c.ID, 999999, true); !errors.Is(err, store.ErrForeignKey) {
		t.Errorf("unknown scenario: %v", err)
	}

	// Concurrent toggles end up all applied: none reads a list that another one is about
	// to overwrite.
	var wg sync.WaitGroup
	for _, id := range ids[2:] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := clients.SetScenario(ctx, c.ID, id, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, err = clients.Get(ctx, c.ID)
	if err != nil || len(got.Scenarios) != len(ids)-1 {
		t.Errorf("after concurrent toggles: %d assignments, want %d: %v", len(got.Scenarios), len(ids)-1, err)
	}
}
