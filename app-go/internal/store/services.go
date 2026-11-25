package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/laposa/tmock/app-go/internal/db"
)

// Service is a row of the services table.
type Service struct {
	ID          int64
	Path        string
	Name        string
	UpstreamURL string
}

// ServiceFields are the columns a create or an update sets.
type ServiceFields struct {
	Path        string
	Name        string
	UpstreamURL string
}

// Services is the services repository.
type Services struct{ db *sql.DB }

// NewServices returns the services repository on pool.
func NewServices(pool *sql.DB) *Services { return &Services{db: pool} }

const serviceColumns = `id, path, name, upstream_url`

// List returns every service ORDER BY name (spec §5.3), then path, so services with the
// same name keep a stable order.
func (r *Services) List(ctx context.Context) ([]Service, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+serviceColumns+` FROM services ORDER BY name, path`)
	if err != nil {
		return nil, db.MapError(err)
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		s, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, db.MapError(rows.Err())
}

// Get returns the service with path (compared exactly, spec §7.1), or ErrNotFound.
func (r *Services) Get(ctx context.Context, path string) (Service, error) {
	return scanService(r.db.QueryRowContext(ctx, `SELECT `+serviceColumns+` FROM services WHERE path = $1`, path))
}

// Create inserts a service. A path that is already taken returns ErrUnique.
func (r *Services) Create(ctx context.Context, f ServiceFields) (Service, error) {
	return scanService(r.db.QueryRowContext(ctx,
		`INSERT INTO services (path, name, upstream_url) VALUES ($1, $2, $3) RETURNING `+serviceColumns,
		f.Path, f.Name, f.UpstreamURL))
}

// Update replaces the fields of the service with path, which may rename its path; its
// scenarios follow, because they reference the id (spec §5.1). It returns ErrNotFound if
// there is no such service, and ErrUnique if the new path is taken.
func (r *Services) Update(ctx context.Context, path string, f ServiceFields) (Service, error) {
	return scanService(r.db.QueryRowContext(ctx,
		`UPDATE services SET path = $2, name = $3, upstream_url = $4 WHERE path = $1 RETURNING `+serviceColumns,
		path, f.Path, f.Name, f.UpstreamURL))
}

// Delete removes the service with path, and through the cascade its scenarios and their
// client assignments. It returns the deleted row, or ErrNotFound.
func (r *Services) Delete(ctx context.Context, path string) (Service, error) {
	return scanService(r.db.QueryRowContext(ctx, `DELETE FROM services WHERE path = $1 RETURNING `+serviceColumns, path))
}

func scanService(s scanner) (Service, error) {
	var v Service
	err := s.Scan(&v.ID, &v.Path, &v.Name, &v.UpstreamURL)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Service{}, ErrNotFound
	case err != nil:
		return Service{}, db.MapError(err)
	}
	return v, nil
}
