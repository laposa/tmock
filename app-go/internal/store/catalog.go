package store

import (
	"context"
	"database/sql"

	"github.com/laposa/tmock/app-go/internal/db"
)

// CatalogData is everything the proxy's catalog is built from (spec §7.2), read in one
// consistent snapshot of the database.
type CatalogData struct {
	Services []CatalogService // ordered by path
	Clients  []CatalogClient  // the enabled clients, ordered by id
}

// CatalogService is a service with its scenarios, ordered by name (spec §5.3), the same
// order as the scenario list.
type CatalogService struct {
	Service
	Scenarios []Scenario
}

// CatalogClient is an enabled client with the ids of its assigned scenarios.
type CatalogClient struct {
	ID        int64
	Name      string
	Condition []byte // the jsonb condition; nil when NULL
	Scenarios []int64
}

// Catalog reads the catalog data.
type Catalog struct{ db *sql.DB }

// NewCatalog returns the catalog reader on pool.
func NewCatalog(pool *sql.DB) *Catalog { return &Catalog{db: pool} }

// Load reads every service with its scenarios and every enabled client with its
// assignments (spec §5.3, the two "snapshot" operations). The queries run in one
// read-only REPEATABLE READ transaction, so a change committed between them cannot leave
// a client pointing at a scenario the snapshot does not have.
func (r *Catalog) Load(ctx context.Context) (CatalogData, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return CatalogData{}, db.MapError(err)
	}
	defer tx.Rollback() //nolint:errcheck // read-only; nothing to keep

	var out CatalogData
	if out.Services, err = loadCatalogServices(ctx, tx); err != nil {
		return CatalogData{}, err
	}
	if out.Clients, err = loadCatalogClients(ctx, tx); err != nil {
		return CatalogData{}, err
	}
	return out, db.MapError(tx.Commit())
}

func loadCatalogServices(ctx context.Context, tx *sql.Tx) ([]CatalogService, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+serviceColumns+` FROM services ORDER BY path`)
	if err != nil {
		return nil, db.MapError(err)
	}
	var out []CatalogService
	index := map[int64]int{}
	for rows.Next() {
		s, err := scanService(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		index[s.ID] = len(out)
		out = append(out, CatalogService{Service: s})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, db.MapError(err)
	}

	// ORDER BY name within each service, as the scenario list (store.Scenarios.List) does.
	rows, err = tx.QueryContext(ctx, `WITH sc AS (SELECT * FROM scenarios) `+scenarioSelect+` ORDER BY sv.path, sc.name`)
	if err != nil {
		return nil, db.MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		s, err := scanScenario(rows)
		if err != nil {
			return nil, err
		}
		if i, ok := index[s.ServiceID]; ok {
			out[i].Scenarios = append(out[i].Scenarios, s)
		}
	}
	return out, db.MapError(rows.Err())
}

func loadCatalogClients(ctx context.Context, tx *sql.Tx) ([]CatalogClient, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, condition FROM clients WHERE enabled ORDER BY id`)
	if err != nil {
		return nil, db.MapError(err)
	}
	var out []CatalogClient
	index := map[int64]int{}
	for rows.Next() {
		var c CatalogClient
		if err := rows.Scan(&c.ID, &c.Name, &c.Condition); err != nil {
			rows.Close()
			return nil, db.MapError(err)
		}
		index[c.ID] = len(out)
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, db.MapError(err)
	}

	rows, err = tx.QueryContext(ctx, `SELECT cs.client_id, cs.scenario_id
		FROM client_scenarios cs JOIN clients c ON c.id = cs.client_id
		WHERE c.enabled ORDER BY cs.client_id, cs.scenario_id`)
	if err != nil {
		return nil, db.MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var client, scenario int64
		if err := rows.Scan(&client, &scenario); err != nil {
			return nil, db.MapError(err)
		}
		if i, ok := index[client]; ok {
			out[i].Scenarios = append(out[i].Scenarios, scenario)
		}
	}
	return out, db.MapError(rows.Err())
}
