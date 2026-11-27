// Package catalog is the proxy's cached view of the services, scenarios and enabled
// clients (spec §7.2).
//
// A Snapshot is immutable once built: the proxy reads it without locks, and per-request
// values live in the proxy's own request-scoped structs. A Cache holds the current
// snapshot behind an atomic.Pointer and rebuilds it lazily, when it is older than the TTL
// or after Invalidate, with at most one rebuild running at a time.
//
// The package is backend code: standard library only.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/laposa/tmock/app-go/internal/condition"
	"github.com/laposa/tmock/app-go/internal/expr"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Snapshot is one immutable view of the catalog. Never modify it or anything it holds.
type Snapshot struct {
	services  map[string]*Service
	clients   []Client
	scenarios int
}

// Service is a service as the proxy uses it.
type Service struct {
	ID          int64
	Path        string
	Name        string
	UpstreamURL string   // after SERVICES_UPSTREAM_OVERRIDES
	Upstream    *url.URL // UpstreamURL parsed
	// Scenarios in name order, as the database returns them (spec §5.3); the catalog does
	// not re-sort them, so the proxy and the scenario list always agree.
	Scenarios []Scenario
}

// Scenario is a scenario of a service. Optional fields are nil when unset.
type Scenario struct {
	ID               int64
	Name             string
	RequestMethod    *string
	RequestPath      *string
	RequestCondition *string
	Condition        *expr.Expr // RequestCondition parsed; nil when unset
	ResponseCode     *int
	ResponseHeaders  map[string]*string
	ResponseBody     *string
	SkipProxy        bool
}

// Client is an enabled client.
type Client struct {
	ID        int64
	Name      string
	Condition *condition.Condition
	Match     *condition.Matcher // Condition compiled
	Scenarios map[int64]bool     // the ids of its assigned scenarios
}

// Service returns the service whose path is exactly path (spec §7.1).
func (s *Snapshot) Service(path string) (*Service, bool) {
	svc, ok := s.services[path]
	return svc, ok
}

// Clients returns the enabled clients, ordered by id. The slice must not be modified.
func (s *Snapshot) Clients() []Client { return s.clients }

// Counts returns how many services, scenarios and clients the snapshot holds.
func (s *Snapshot) Counts() (services, scenarios, clients int) {
	return len(s.services), s.scenarios, len(s.clients)
}

// Build turns the catalog data into a snapshot. overrides maps service paths to the
// upstream URL to use instead of the stored one (SERVICES_UPSTREAM_OVERRIDES). A service
// whose upstream URL does not parse, a scenario whose request condition does not parse,
// or a client whose condition does not compile or is empty, cannot have been saved by
// internal/app; it is left out and reported to logger rather than failing the whole
// catalog. (An empty condition would match every request.)
func Build(data store.CatalogData, overrides map[string]string, logger *slog.Logger) *Snapshot {
	if logger == nil {
		logger = slog.Default()
	}
	snap := &Snapshot{services: make(map[string]*Service, len(data.Services))}
	for _, s := range data.Services {
		raw := s.UpstreamURL
		if o, ok := overrides[s.Path]; ok {
			raw = o
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			logger.Warn(fmt.Sprintf("catalog: service %q left out: upstream URL %q is not an absolute URL", s.Path, raw))
			continue
		}
		svc := &Service{ID: s.ID, Path: s.Path, Name: s.Name, UpstreamURL: raw, Upstream: u,
			Scenarios: make([]Scenario, 0, len(s.Scenarios))}
		for _, sc := range s.Scenarios {
			var cond *expr.Expr
			if sc.RequestCondition != nil {
				var err error
				if cond, err = expr.Parse(*sc.RequestCondition); err != nil {
					logger.Warn(fmt.Sprintf("catalog: scenario %d left out: its request condition does not parse: %v", sc.ID, err))
					continue
				}
			}
			svc.Scenarios = append(svc.Scenarios, Scenario{
				ID: sc.ID, Name: sc.Name,
				RequestMethod: sc.RequestMethod, RequestPath: sc.RequestPath, RequestCondition: sc.RequestCondition, Condition: cond,
				ResponseCode: sc.ResponseCode, ResponseHeaders: sc.ResponseHeaders, ResponseBody: sc.ResponseBody,
				SkipProxy: sc.SkipProxy,
			})
		}
		snap.services[s.Path] = svc
		snap.scenarios += len(svc.Scenarios)
	}
	for _, c := range data.Clients {
		cond, err := condition.Parse(string(c.Condition))
		var match *condition.Matcher
		if err == nil && condition.Empty(cond) {
			err = errors.New("it is empty")
		}
		if err == nil {
			match, err = condition.Compile(cond)
		}
		if err != nil {
			logger.Warn(fmt.Sprintf("catalog: client %d left out: its condition cannot be used: %v", c.ID, err))
			continue
		}
		ids := make(map[int64]bool, len(c.Scenarios))
		for _, id := range c.Scenarios {
			ids[id] = true
		}
		snap.clients = append(snap.clients, Client{ID: c.ID, Name: c.Name, Condition: cond, Match: match, Scenarios: ids})
	}
	return snap
}

// Loader reads the catalog data; *store.Catalog is one.
type Loader interface {
	Load(ctx context.Context) (store.CatalogData, error)
}
