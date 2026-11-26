package store_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

func ptr[T any](v T) *T { return &v }

func TestScenarios(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	services, scenarios := store.NewServices(pool), store.NewScenarios(pool)
	ctx := context.Background()

	b, err := services.Create(ctx, store.ServiceFields{Path: "b", Name: "Beta", UpstreamURL: "http://b.test"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := services.Create(ctx, store.ServiceFields{Path: "a", Name: "Zulu", UpstreamURL: "http://a.test"})
	if err != nil {
		t.Fatal(err)
	}
	if list, err := scenarios.List(ctx, ""); err != nil || len(list) != 0 {
		t.Fatalf("empty list: %v, %v", list, err)
	}

	// Every column round-trips, NULLs included; a null header value stays null.
	full := store.ScenarioFields{
		ServiceID: b.ID, Name: "full",
		RequestMethod: ptr("GET"), RequestPath: ptr("orders/42"), RequestCondition: ptr(`$request.query.x == "1"`),
		ResponseCode:    ptr(503),
		ResponseHeaders: map[string]*string{"Content-Type": ptr("application/json"), "X-Drop": nil},
		ResponseBody:    ptr(`{"a":1}`), SkipProxy: true,
	}
	got, err := scenarios.Create(ctx, full)
	if err != nil || got.ID == 0 || got.ServicePath != "b" {
		t.Fatalf("create: %+v, %v", got, err)
	}
	want := store.Scenario{ID: got.ID, ServiceID: b.ID, ServicePath: "b", Name: "full",
		RequestMethod: full.RequestMethod, RequestPath: full.RequestPath, RequestCondition: full.RequestCondition,
		ResponseCode: full.ResponseCode, ResponseHeaders: full.ResponseHeaders, ResponseBody: full.ResponseBody, SkipProxy: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("create returned %+v", got)
	}
	if g, err := scenarios.Get(ctx, got.ID); err != nil || !reflect.DeepEqual(g, want) {
		t.Errorf("get: %+v, %v", g, err)
	}
	empty, err := scenarios.Create(ctx, store.ScenarioFields{ServiceID: b.ID, Name: "empty"})
	if err != nil || empty.RequestMethod != nil || empty.RequestPath != nil || empty.RequestCondition != nil ||
		empty.ResponseCode != nil || empty.ResponseHeaders != nil || empty.ResponseBody != nil || empty.SkipProxy {
		t.Errorf("create with NULLs: %+v, %v", empty, err)
	}
	var nulls int
	if err := pool.QueryRow(`SELECT num_nulls(request_method, request_path, request_condition, response_code, response_headers, response_body) FROM scenarios WHERE id = $1`, empty.ID).Scan(&nulls); err != nil || nulls != 6 {
		t.Errorf("stored %d NULLs, %v", nulls, err)
	}

	// Names are unique within a service only.
	if _, err := scenarios.Create(ctx, store.ScenarioFields{ServiceID: b.ID, Name: "full"}); !errors.Is(err, store.ErrUnique) {
		t.Errorf("duplicate name: %v", err)
	}
	if _, err := scenarios.Create(ctx, store.ScenarioFields{ServiceID: a.ID, Name: "full"}); err != nil {
		t.Errorf("same name in another service: %v", err)
	}
	if _, err := scenarios.Create(ctx, store.ScenarioFields{ServiceID: 999999, Name: "x"}); !errors.Is(err, store.ErrForeignKey) {
		t.Errorf("unknown service: %v", err)
	}

	// ORDER BY service path, name; the filter matches the path exactly.
	list, err := scenarios.List(ctx, "")
	var order []string
	for _, s := range list {
		order = append(order, s.ServicePath+"/"+s.Name)
	}
	if err != nil || reflect.DeepEqual(order, []string{"a/full", "b/empty", "b/full"}) == false {
		t.Errorf("list: %v, %v", order, err)
	}
	if list, err := scenarios.List(ctx, "b"); err != nil || len(list) != 2 {
		t.Errorf("filtered: %+v, %v", list, err)
	}
	if list, err := scenarios.List(ctx, "B"); err != nil || len(list) != 0 {
		t.Errorf("filter is exact: %+v, %v", list, err)
	}

	// Update replaces every field, and may move the scenario to another service.
	moved, err := scenarios.Update(ctx, empty.ID, store.ScenarioFields{ServiceID: a.ID, Name: "moved", ResponseCode: ptr(201)})
	if err != nil || moved.ServicePath != "a" || moved.Name != "moved" || *moved.ResponseCode != 201 {
		t.Errorf("update: %+v, %v", moved, err)
	}
	cleared, err := scenarios.Update(ctx, got.ID, store.ScenarioFields{ServiceID: b.ID, Name: "full"})
	if err != nil || cleared.RequestMethod != nil || cleared.ResponseHeaders != nil || cleared.ResponseBody != nil || cleared.SkipProxy {
		t.Errorf("clearing: %+v, %v", cleared, err)
	}
	if _, err := scenarios.Update(ctx, moved.ID, store.ScenarioFields{ServiceID: a.ID, Name: "full"}); !errors.Is(err, store.ErrUnique) {
		t.Errorf("rename onto a taken name: %v", err)
	}
	if _, err := scenarios.Update(ctx, 999999, store.ScenarioFields{ServiceID: a.ID, Name: "x"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}

	deleted, err := scenarios.Delete(ctx, moved.ID)
	if err != nil || deleted.ID != moved.ID || deleted.ServicePath != "a" {
		t.Errorf("delete: %+v, %v", deleted, err)
	}
	if _, err := scenarios.Delete(ctx, moved.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if _, err := scenarios.Get(ctx, moved.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("get deleted: %v", err)
	}
}
