package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/laposa/tmock/app-go/internal/db"
)

// Client is a row of the clients table with its scenario assignments. The token hash
// is never read back: HasToken says whether there is one.
type Client struct {
	ID        int64
	Name      string
	Enabled   bool
	Condition []byte // the jsonb condition; nil when NULL
	HasToken  bool
	Scenarios []AssignedScenario // ordered by service path, then scenario name
}

// AssignedScenario is a scenario assigned to a client, with what the UI shows of it.
type AssignedScenario struct {
	ID          int64
	ServicePath string
	Name        string
}

// ClientFields are the columns a create or an update sets.
type ClientFields struct {
	Name      string
	Enabled   bool
	Condition []byte // nil is stored as NULL
}

// Clients is the clients repository.
type Clients struct{ db *sql.DB }

// NewClients returns the clients repository on pool.
func NewClients(pool *sql.DB) *Clients { return &Clients{db: pool} }

const clientColumns = `id, name, enabled, condition, token_hash IS NOT NULL`

// List returns every client ORDER BY id, each with its assigned scenarios (spec §5.3).
func (r *Clients) List(ctx context.Context) ([]Client, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+clientColumns+` FROM clients ORDER BY id`)
	if err != nil {
		return nil, db.MapError(err)
	}
	var out []Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, db.MapError(err)
	}
	assigned, err := r.assignments(ctx, r.db, 0)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Scenarios = assigned[out[i].ID]
	}
	return out, nil
}

// Get returns the client with id and its assigned scenarios, or ErrNotFound.
func (r *Clients) Get(ctx context.Context, id int64) (Client, error) {
	return r.get(ctx, r.db, id)
}

// FindByTokenHash returns the id of the client whose token hash is hash, or ErrNotFound
// (spec §5.3: find by token).
func (r *Clients) FindByTokenHash(ctx context.Context, hash []byte) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE token_hash = $1`, hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, db.MapError(err)
}

// Create inserts a client with its assignments, in one transaction. A name that is taken
// returns ErrUnique, an unknown scenario id ErrForeignKey.
func (r *Clients) Create(ctx context.Context, f ClientFields, scenarios []int64) (Client, error) {
	var out Client
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `INSERT INTO clients (name, enabled, condition) VALUES ($1, $2, $3) RETURNING id`,
			f.Name, f.Enabled, jsonValue(f.Condition)).Scan(&id)
		if err != nil {
			return db.MapError(err)
		}
		if err := setScenarios(ctx, tx, id, scenarios); err != nil {
			return err
		}
		out, err = r.get(ctx, tx, id)
		return err
	})
	return out, err
}

// Update replaces the fields of the client with id and, when scenarios is not nil, its
// assignments (delete all, then insert the new set, spec §5.3), in one transaction. It
// returns ErrNotFound, ErrUnique for a taken name, or ErrForeignKey for an unknown
// scenario id.
func (r *Clients) Update(ctx context.Context, id int64, f ClientFields, scenarios []int64) (Client, error) {
	var out Client
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE clients SET name = $2, enabled = $3, condition = $4 WHERE id = $1`,
			id, f.Name, f.Enabled, jsonValue(f.Condition))
		if err != nil {
			return db.MapError(err)
		}
		if err := oneRow(res); err != nil {
			return err
		}
		if scenarios != nil {
			if err := setScenarios(ctx, tx, id, scenarios); err != nil {
				return err
			}
		}
		out, err = r.get(ctx, tx, id)
		return err
	})
	return out, err
}

// Delete removes the client with id, with its assignments (cascade), and returns it as it
// was, or ErrNotFound.
func (r *Clients) Delete(ctx context.Context, id int64) (Client, error) {
	var out Client
	err := r.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		if out, err = r.get(ctx, tx, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM clients WHERE id = $1`, id)
		return db.MapError(err)
	})
	return out, err
}

// DisableAll disables every client (spec §5.3) and returns how many were enabled.
func (r *Clients) DisableAll(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE clients SET enabled = false WHERE enabled`)
	if err != nil {
		return 0, db.MapError(err)
	}
	return res.RowsAffected()
}

// SetTokenHash stores hash as the client's token, replacing any other; nil removes it.
// It returns the client's name, or ErrNotFound.
func (r *Clients) SetTokenHash(ctx context.Context, id int64, hash []byte) (string, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `UPDATE clients SET token_hash = $2 WHERE id = $1 RETURNING name`, id, hash).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, db.MapError(err)
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *Clients) get(ctx context.Context, q querier, id int64) (Client, error) {
	c, err := scanClient(q.QueryRowContext(ctx, `SELECT `+clientColumns+` FROM clients WHERE id = $1`, id))
	if err != nil {
		return Client{}, err
	}
	assigned, err := r.assignments(ctx, q, id)
	if err != nil {
		return Client{}, err
	}
	c.Scenarios = assigned[id]
	return c, nil
}

// assignments returns the assigned scenarios of the client with id, or of every client
// when id is 0, keyed by client id, each list ordered by service path and scenario name.
func (r *Clients) assignments(ctx context.Context, q querier, id int64) (map[int64][]AssignedScenario, error) {
	rows, err := q.QueryContext(ctx, `SELECT cs.client_id, sc.id, sv.path, sc.name
		FROM client_scenarios cs
		JOIN scenarios sc ON sc.id = cs.scenario_id
		JOIN services sv ON sv.id = sc.service_id
		WHERE $1 = 0 OR cs.client_id = $1
		ORDER BY cs.client_id, sv.path, sc.name`, id)
	if err != nil {
		return nil, db.MapError(err)
	}
	defer rows.Close()
	out := map[int64][]AssignedScenario{}
	for rows.Next() {
		var client int64
		var a AssignedScenario
		if err := rows.Scan(&client, &a.ID, &a.ServicePath, &a.Name); err != nil {
			return nil, db.MapError(err)
		}
		out[client] = append(out[client], a)
	}
	return out, db.MapError(rows.Err())
}

// setScenarios replaces the assignments of client id with scenarios (no duplicates).
func setScenarios(ctx context.Context, tx *sql.Tx, id int64, scenarios []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM client_scenarios WHERE client_id = $1`, id); err != nil {
		return db.MapError(err)
	}
	if len(scenarios) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO client_scenarios (client_id, scenario_id) SELECT $1, unnest($2::bigint[])`, id, scenarios)
	return db.MapError(err)
}

func (r *Clients) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return db.MapError(err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if err := fn(tx); err != nil {
		return err
	}
	return db.MapError(tx.Commit())
}

// jsonValue is a nullable jsonb parameter: NULL for nil.
func jsonValue(b []byte) any {
	if b == nil {
		return nil
	}
	return b
}

func scanClient(s scanner) (Client, error) {
	var c Client
	err := s.Scan(&c.ID, &c.Name, &c.Enabled, &c.Condition, &c.HasToken)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Client{}, ErrNotFound
	case err != nil:
		return Client{}, db.MapError(err)
	}
	return c, nil
}

// ExistingScenarioIDs returns which of ids are scenarios that exist.
func (r *Scenarios) ExistingScenarioIDs(ctx context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM scenarios WHERE id = ANY($1::bigint[])`, ids)
	if err != nil {
		return nil, db.MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, db.MapError(err)
		}
		out[id] = true
	}
	return out, db.MapError(rows.Err())
}
