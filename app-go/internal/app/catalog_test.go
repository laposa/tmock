package app_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/condition"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
)

type countingCatalog struct{ n atomic.Int32 }

func (c *countingCatalog) Invalidate() { c.n.Add(1) }

// Spec §7.2: every mutation of services, scenarios and clients invalidates the catalog;
// reads, failed changes and user changes do not.
func TestMutationsInvalidateCatalog(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	_, logger := logxtest.New()
	cat := &countingCatalog{}
	a := app.New(pool, app.Options{Logger: logger, Hasher: fast, Catalog: cat})
	ctx := context.Background()

	step := func(name string, want bool, fn func() error) {
		t.Helper()
		before := cat.n.Load()
		err := fn()
		if got := cat.n.Load() > before; got != want {
			t.Errorf("%s (err %v): invalidated = %v, want %v", name, err, got, want)
		}
	}
	ip := "10.0.0.1"
	var scenarioID, clientID int64
	step("create service", true, func() error {
		_, err := a.CreateService(ctx, ann, app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "http://u"})
		return err
	})
	step("duplicate service", false, func() error {
		_, err := a.CreateService(ctx, ann, app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "http://u"})
		return err
	})
	step("list services", false, func() error { _, err := a.ListServices(ctx); return err })
	step("update service", true, func() error {
		_, err := a.UpdateService(ctx, ann, "s", app.ServicePatch{Name: httpx.Some("S2")})
		return err
	})
	step("create scenario", true, func() error {
		s, err := a.CreateScenario(ctx, ann, app.ScenarioInput{Service: "s", Name: "sc"})
		scenarioID = s.ID
		return err
	})
	step("update scenario", true, func() error {
		_, err := a.UpdateScenario(ctx, ann, scenarioID, app.ScenarioPatch{Name: httpx.Some("sc2")})
		return err
	})
	step("create client", true, func() error {
		c, err := a.CreateClient(ctx, ann, app.ClientInput{Name: "c", Condition: &condition.Condition{IP: &ip}})
		clientID = c.ID
		return err
	})
	step("update client", true, func() error {
		_, err := a.UpdateClient(ctx, ann, clientID, app.ClientPatch{Enabled: httpx.Some(true)})
		return err
	})
	step("assign scenario", true, func() error {
		_, err := a.SetClientScenario(ctx, ann, clientID, scenarioID, true)
		return err
	})
	step("disable all", true, func() error { return a.DisableAllClients(ctx, ann) })
	step("create user", false, func() error {
		_, err := a.CreateUser(ctx, ann, app.UserInput{Name: "U", Email: "u@example.com", Password: "password-1"})
		return err
	})
	step("delete scenario", true, func() error { _, err := a.DeleteScenario(ctx, ann, scenarioID); return err })
	step("delete client", true, func() error { _, err := a.DeleteClient(ctx, ann, clientID); return err })
	step("delete service", true, func() error { _, err := a.DeleteService(ctx, ann, "s"); return err })
}
