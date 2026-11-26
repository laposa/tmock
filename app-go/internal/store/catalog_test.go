package store_test

import (
	"context"
	"slices"
	"testing"

	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

func TestCatalogLoad(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	ctx := context.Background()
	cat := store.NewCatalog(pool)

	if data, err := cat.Load(ctx); err != nil || len(data.Services) != 0 || len(data.Clients) != 0 {
		t.Fatalf("empty: %+v, %v", data, err)
	}

	for _, q := range []string{
		`INSERT INTO services (path, name, upstream_url) VALUES ('b', 'B', 'http://b'), ('a', 'A', 'http://a'), ('empty', 'E', 'http://e')`,
		`INSERT INTO scenarios (service_id, name, request_method, response_code, response_headers, response_body, skip_proxy)
		 SELECT id, n, 'GET', 500, '{"X-A":"1","X-B":null}', 'body', true FROM services, unnest(ARRAY['zeta','Alpha','beta']) n WHERE path = 'a'`,
		`INSERT INTO scenarios (service_id, name) SELECT id, 'only' FROM services WHERE path = 'b'`,
		`INSERT INTO clients (name, enabled, condition) VALUES ('off', false, '{"ip":"10.0.0.1"}'), ('on2', true, '{"ip":"10.0.0.2"}'), ('on3', true, '{"cidr":"10.0.0.0/8"}')`,
		`INSERT INTO client_scenarios SELECT c.id, s.id FROM clients c, scenarios s WHERE c.name IN ('off', 'on2')`,
	} {
		if _, err := pool.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	data, err := cat.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	for _, s := range data.Services {
		paths = append(paths, s.Path)
	}
	if !slices.Equal(paths, []string{"a", "b", "empty"}) {
		t.Errorf("services = %v", paths)
	}
	// Scenarios in the order the scenario list shows them (ORDER BY name in SQL).
	list, err := store.NewScenarios(pool).List(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	a := data.Services[0]
	if len(a.Scenarios) != len(list) {
		t.Fatalf("scenarios of a: %+v", a.Scenarios)
	}
	for i := range list {
		if a.Scenarios[i].ID != list[i].ID {
			t.Errorf("scenario %d: %q, the list has %q", i, a.Scenarios[i].Name, list[i].Name)
		}
	}
	sc := a.Scenarios[0]
	if *sc.RequestMethod != "GET" || *sc.ResponseCode != 500 || *sc.ResponseBody != "body" || !sc.SkipProxy ||
		*sc.ResponseHeaders["X-A"] != "1" || sc.ResponseHeaders["X-B"] != nil {
		t.Errorf("scenario fields: %+v", sc)
	}
	if len(data.Services[2].Scenarios) != 0 {
		t.Errorf("empty service has scenarios")
	}

	// Only enabled clients, by id, with their assignment sets.
	if len(data.Clients) != 2 || data.Clients[0].Name != "on2" || data.Clients[1].Name != "on3" {
		t.Fatalf("clients = %+v", data.Clients)
	}
	if len(data.Clients[0].Scenarios) != 4 || len(data.Clients[1].Scenarios) != 0 {
		t.Errorf("assignments = %v, %v", data.Clients[0].Scenarios, data.Clients[1].Scenarios)
	}
	if string(data.Clients[0].Condition) != `{"ip": "10.0.0.2"}` {
		t.Errorf("condition = %s", data.Clients[0].Condition)
	}
}
