package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/laposa/tmock/app-go/internal/db"
)

// Scenario is a row of the scenarios table, with the path of its service. Optional
// columns are nil when NULL (spec §5.1).
type Scenario struct {
	ID               int64
	ServiceID        int64
	ServicePath      string
	Name             string
	RequestMethod    *string
	RequestPath      *string
	RequestCondition *string
	ResponseCode     *int
	ResponseHeaders  map[string]*string // nil when NULL; a nil value is JSON null
	ResponseBody     *string
	SkipProxy        bool
}

// ScenarioFields are the columns a create or an update sets.
type ScenarioFields struct {
	ServiceID        int64
	Name             string
	RequestMethod    *string
	RequestPath      *string
	RequestCondition *string
	ResponseCode     *int
	ResponseHeaders  map[string]*string // nil is stored as NULL
	ResponseBody     *string
	SkipProxy        bool
}

// Scenarios is the scenarios repository.
type Scenarios struct{ db *sql.DB }

// NewScenarios returns the scenarios repository on pool.
func NewScenarios(pool *sql.DB) *Scenarios { return &Scenarios{db: pool} }

// scenarioSelect selects the scenarios in the relation sc, joined with their services.
// Writes use it on their RETURNING rows (a CTE named sc), so that every query returns
// the service path as well.
const scenarioSelect = `SELECT sc.id, sc.service_id, sv.path, sc.name, sc.request_method, sc.request_path,
	sc.request_condition, sc.response_code, sc.response_headers, sc.response_body, sc.skip_proxy
	FROM sc JOIN services sv ON sv.id = sc.service_id`

const scenarioReturning = `RETURNING id, service_id, name, request_method, request_path, request_condition,
	response_code, response_headers, response_body, skip_proxy`

// List returns the scenarios ORDER BY service path, name (spec §5.3). A non-empty
// servicePath keeps only that service's scenarios (compared exactly, spec §7.1).
func (r *Scenarios) List(ctx context.Context, servicePath string) ([]Scenario, error) {
	rows, err := r.db.QueryContext(ctx, `WITH sc AS (SELECT * FROM scenarios) `+scenarioSelect+`
		WHERE $1 = '' OR sv.path = $1 ORDER BY sv.path, sc.name`, servicePath)
	if err != nil {
		return nil, db.MapError(err)
	}
	defer rows.Close()
	var out []Scenario
	for rows.Next() {
		s, err := scanScenario(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, db.MapError(rows.Err())
}

// Get returns the scenario with id, or ErrNotFound.
func (r *Scenarios) Get(ctx context.Context, id int64) (Scenario, error) {
	return scanScenario(r.db.QueryRowContext(ctx, `WITH sc AS (SELECT * FROM scenarios WHERE id = $1) `+scenarioSelect, id))
}

// Create inserts a scenario. A name that is taken within the service returns ErrUnique,
// a service that does not exist (any more) ErrForeignKey.
func (r *Scenarios) Create(ctx context.Context, f ScenarioFields) (Scenario, error) {
	headers, err := headersValue(f.ResponseHeaders)
	if err != nil {
		return Scenario{}, err
	}
	return scanScenario(r.db.QueryRowContext(ctx, `WITH sc AS (
		INSERT INTO scenarios (service_id, name, request_method, request_path, request_condition,
			response_code, response_headers, response_body, skip_proxy)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) `+scenarioReturning+`) `+scenarioSelect,
		f.ServiceID, f.Name, f.RequestMethod, f.RequestPath, f.RequestCondition,
		f.ResponseCode, headers, f.ResponseBody, f.SkipProxy))
}

// Update replaces every field of the scenario with id. It returns ErrNotFound if there
// is no such scenario, ErrUnique if the name is taken within the service, and
// ErrForeignKey if the service does not exist.
func (r *Scenarios) Update(ctx context.Context, id int64, f ScenarioFields) (Scenario, error) {
	headers, err := headersValue(f.ResponseHeaders)
	if err != nil {
		return Scenario{}, err
	}
	return scanScenario(r.db.QueryRowContext(ctx, `WITH sc AS (
		UPDATE scenarios SET service_id = $2, name = $3, request_method = $4, request_path = $5,
			request_condition = $6, response_code = $7, response_headers = $8, response_body = $9,
			skip_proxy = $10
		WHERE id = $1 `+scenarioReturning+`) `+scenarioSelect,
		id, f.ServiceID, f.Name, f.RequestMethod, f.RequestPath, f.RequestCondition,
		f.ResponseCode, headers, f.ResponseBody, f.SkipProxy))
}

// Delete removes the scenario with id, and through the cascade its client assignments.
// It returns the deleted row, or ErrNotFound.
func (r *Scenarios) Delete(ctx context.Context, id int64) (Scenario, error) {
	return scanScenario(r.db.QueryRowContext(ctx, `WITH sc AS (DELETE FROM scenarios WHERE id = $1 `+scenarioReturning+`) `+scenarioSelect, id))
}

// headersValue is the response_headers parameter: NULL for nil, the JSON otherwise.
func headersValue(h map[string]*string) (any, error) {
	if h == nil {
		return nil, nil
	}
	b, err := json.Marshal(h)
	if err != nil {
		return nil, fmt.Errorf("store: encoding response headers: %w", err)
	}
	return b, nil
}

func scanScenario(s scanner) (Scenario, error) {
	var (
		v       Scenario
		code    sql.NullInt64
		headers []byte // nullable jsonb: []byte, not json.RawMessage (spec §3.3)
	)
	err := s.Scan(&v.ID, &v.ServiceID, &v.ServicePath, &v.Name, &v.RequestMethod, &v.RequestPath,
		&v.RequestCondition, &code, &headers, &v.ResponseBody, &v.SkipProxy)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Scenario{}, ErrNotFound
	case err != nil:
		return Scenario{}, db.MapError(err)
	}
	if code.Valid {
		c := int(code.Int64)
		v.ResponseCode = &c
	}
	if headers != nil {
		if err := json.Unmarshal(headers, &v.ResponseHeaders); err != nil {
			return Scenario{}, fmt.Errorf("store: scenario %d: response headers: %w", v.ID, err)
		}
	}
	return v, nil
}
