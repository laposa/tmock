package app

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Service is a service as the API and the UI show it (spec §6.1). The API and the proxy
// address services by Path; the surrogate id stays internal.
type Service struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	UpstreamURL string `json:"upstreamUrl"`
}

// ServiceInput is the body of a create (spec §6.4).
type ServiceInput struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	UpstreamURL string `json:"upstreamUrl"`
}

// ServicePatch is the body of an update: absent fields stay as they are (spec §6.1).
// All three fields are required, so null is a validation error.
type ServicePatch struct {
	Path        httpx.Optional[string] `json:"path"`
	Name        httpx.Optional[string] `json:"name"`
	UpstreamURL httpx.Optional[string] `json:"upstreamUrl"`
}

// Limits from the schema (spec §5.1).
const (
	maxServicePath = 255
	maxServiceName = 255
	maxUpstreamURL = 2048
)

var servicePathRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ListServices returns every service, ordered by name (spec §5.3). The list is never nil,
// so the API answers [] rather than null.
func (a *App) ListServices(ctx context.Context) ([]Service, error) {
	rows, err := a.services.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(rows))
	for _, s := range rows {
		out = append(out, serviceFromStore(s))
	}
	return out, nil
}

// GetService returns the service with path, or NotFound.
func (a *App) GetService(ctx context.Context, path string) (Service, error) {
	s, err := a.services.Get(ctx, path)
	if err != nil {
		return Service{}, StoreError(err, serviceNotFound(path), nil)
	}
	return serviceFromStore(s), nil
}

// CreateService validates in and creates the service. A path that is taken is a Conflict.
func (a *App) CreateService(ctx context.Context, actor Actor, in ServiceInput) (Service, error) {
	f := store.ServiceFields{
		Path:        strings.TrimSpace(in.Path),
		Name:        strings.TrimSpace(in.Name),
		UpstreamURL: strings.TrimSpace(in.UpstreamURL),
	}
	if err := validateService(f); err != nil {
		return Service{}, err
	}
	s, err := a.services.Create(ctx, f)
	if err != nil {
		return Service{}, StoreError(err, nil, servicePathTaken(f.Path))
	}
	a.serviceChanged(ctx, actor, logx.ActionCreate, s)
	return serviceFromStore(s), nil
}

// UpdateService changes the fields present in p of the service with path. Renaming the
// path keeps the service's scenarios, which follow it (spec §5.1).
func (a *App) UpdateService(ctx context.Context, actor Actor, path string, p ServicePatch) (Service, error) {
	var v httpx.Validator
	for field, o := range map[string]httpx.Optional[string]{"path": p.Path, "name": p.Name, "upstreamUrl": p.UpstreamURL} {
		v.Check(!o.Null, field, "is required")
	}
	if !v.OK() {
		return Service{}, Invalid(v.Details())
	}

	cur, err := a.services.Get(ctx, path)
	if err != nil {
		return Service{}, StoreError(err, serviceNotFound(path), nil)
	}
	f := store.ServiceFields{Path: cur.Path, Name: cur.Name, UpstreamURL: cur.UpstreamURL}
	if s, ok := p.Path.Get(); ok {
		f.Path = strings.TrimSpace(s)
	}
	if s, ok := p.Name.Get(); ok {
		f.Name = strings.TrimSpace(s)
	}
	if s, ok := p.UpstreamURL.Get(); ok {
		f.UpstreamURL = strings.TrimSpace(s)
	}
	if err := validateService(f); err != nil {
		return Service{}, err
	}
	s, err := a.services.Update(ctx, path, f)
	if err != nil {
		// NotFound when the service was deleted or renamed since it was read.
		return Service{}, StoreError(err, serviceNotFound(path), servicePathTaken(f.Path))
	}
	a.serviceChanged(ctx, actor, logx.ActionUpdate, s)
	return serviceFromStore(s), nil
}

// DeleteService deletes the service with path, with its scenarios and their client
// assignments, and returns what was deleted.
func (a *App) DeleteService(ctx context.Context, actor Actor, path string) (Service, error) {
	s, err := a.services.Delete(ctx, path)
	if err != nil {
		return Service{}, StoreError(err, serviceNotFound(path), nil)
	}
	a.serviceChanged(ctx, actor, logx.ActionDelete, s)
	return serviceFromStore(s), nil
}

// validateService checks the fields of a service as they will be stored (spec §6.4).
func validateService(f store.ServiceFields) error {
	var v httpx.Validator
	if v.Required("path", f.Path != "") && v.Length("path", f.Path, 1, maxServicePath) {
		v.Check(servicePathRe.MatchString(f.Path), "path", "may contain only letters, digits, '.', '_' and '-'")
		// "." and ".." match the pattern, but /api/proxy/../x is cleaned to another URL, so
		// such a service could never be reached through the proxy.
		v.Check(f.Path != "." && f.Path != "..", "path", `must not be "." or ".."`)
	}
	if v.Required("name", f.Name != "") {
		v.Length("name", f.Name, 1, maxServiceName)
	}
	if v.Required("upstreamUrl", f.UpstreamURL != "") && v.Length("upstreamUrl", f.UpstreamURL, 1, maxUpstreamURL) {
		v.Check(isUpstreamURL(f.UpstreamURL), "upstreamUrl", "must be an absolute http or https URL")
	}
	if !v.OK() {
		return Invalid(v.Details())
	}
	return nil
}

// isUpstreamURL reports whether s is an absolute http or https URL with a host name.
// Hostname, not Host: "http://:8080" has a Host (":8080") but no host to connect to.
func isUpstreamURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.Opaque == ""
}

func serviceFromStore(s store.Service) Service {
	return Service{Path: s.Path, Name: s.Name, UpstreamURL: s.UpstreamURL}
}

func serviceNotFound(path string) *Error { return NotFound("Service %q not found", path) }

func servicePathTaken(path string) *Error {
	return Conflict("A service with path %q already exists", path)
}

// serviceChanged logs tmock.entity.changed for a service (spec §2.4.2). Besides the id,
// it names the service's path (after the change), which is how people know it.
func (a *App) serviceChanged(ctx context.Context, actor Actor, action string, s store.Service) {
	a.entityChanged(ctx, actor, logx.EntityService, s.ID, action, slog.String(logx.AttrService, s.Path))
}
