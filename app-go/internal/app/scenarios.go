package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Scenario is a scenario as the API and the UI show it (spec §6.1). Optional fields are
// nil when unset, and are written as null.
type Scenario struct {
	ID               int64              `json:"id"`
	Service          string             `json:"service"` // the service's path
	Name             string             `json:"name"`
	RequestMethod    *string            `json:"requestMethod"`
	RequestPath      *string            `json:"requestPath"`
	RequestCondition *string            `json:"requestCondition"`
	ResponseCode     *int               `json:"responseCode"`
	ResponseHeaders  map[string]*string `json:"responseHeaders"` // a nil value deletes the header (spec §7.8)
	ResponseBody     *string            `json:"responseBody"`
	SkipProxy        bool               `json:"skipProxy"`
}

// ScenarioInput is the body of a create (spec §6.4). Every field but service and name
// may be absent or null.
type ScenarioInput struct {
	Service          string             `json:"service"`
	Name             string             `json:"name"`
	RequestMethod    *string            `json:"requestMethod"`
	RequestPath      *string            `json:"requestPath"`
	RequestCondition *string            `json:"requestCondition"`
	ResponseCode     *int               `json:"responseCode"`
	ResponseHeaders  map[string]*string `json:"responseHeaders"`
	ResponseBody     *string            `json:"responseBody"`
	SkipProxy        bool               `json:"skipProxy"`
}

// ScenarioPatch is the body of an update: absent fields stay as they are, null clears an
// optional field (spec §6.1). service, name and skipProxy cannot be null.
type ScenarioPatch struct {
	Service          httpx.Optional[string]             `json:"service"`
	Name             httpx.Optional[string]             `json:"name"`
	RequestMethod    httpx.Optional[string]             `json:"requestMethod"`
	RequestPath      httpx.Optional[string]             `json:"requestPath"`
	RequestCondition httpx.Optional[string]             `json:"requestCondition"`
	ResponseCode     httpx.Optional[int]                `json:"responseCode"`
	ResponseHeaders  httpx.Optional[map[string]*string] `json:"responseHeaders"`
	ResponseBody     httpx.Optional[string]             `json:"responseBody"`
	SkipProxy        httpx.Optional[bool]               `json:"skipProxy"`
}

// Replace is the patch that sets every field to in's values: what saving the whole
// editor form means (spec §10.3 SC3).
func (in ScenarioInput) Replace() ScenarioPatch {
	opt := func(p *string) httpx.Optional[string] {
		if p == nil {
			return httpx.Null[string]()
		}
		return httpx.Some(*p)
	}
	code := httpx.Null[int]()
	if in.ResponseCode != nil {
		code = httpx.Some(*in.ResponseCode)
	}
	return ScenarioPatch{
		Service:          httpx.Some(in.Service),
		Name:             httpx.Some(in.Name),
		RequestMethod:    opt(in.RequestMethod),
		RequestPath:      opt(in.RequestPath),
		RequestCondition: opt(in.RequestCondition),
		ResponseCode:     code,
		ResponseHeaders:  httpx.Some(in.ResponseHeaders), // nil or empty is stored as NULL
		ResponseBody:     opt(in.ResponseBody),
		SkipProxy:        httpx.Some(in.SkipProxy),
	}
}

// RequestMethods are the methods a scenario can be limited to (spec §6.4), in the order
// the editor offers them.
var RequestMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

const maxScenarioName = 255

// ListScenarios returns the scenarios ordered by service path, then name (spec §5.3),
// with their full bodies. A non-empty service keeps only that service's scenarios; an
// unknown one gives an empty list. The list is never nil.
func (a *App) ListScenarios(ctx context.Context, service string) ([]Scenario, error) {
	rows, err := a.scenarios.List(ctx, service)
	if err != nil {
		return nil, err
	}
	out := make([]Scenario, 0, len(rows))
	for _, s := range rows {
		out = append(out, scenarioFromStore(s))
	}
	return out, nil
}

// GetScenario returns the scenario with id, or NotFound.
func (a *App) GetScenario(ctx context.Context, id int64) (Scenario, error) {
	s, err := a.scenarios.Get(ctx, id)
	if err != nil {
		return Scenario{}, StoreError(err, scenarioNotFound(id), nil)
	}
	return scenarioFromStore(s), nil
}

// CreateScenario validates in and creates the scenario. An unknown service is NotFound;
// a name that is taken within the service is a Conflict.
func (a *App) CreateScenario(ctx context.Context, actor Actor, in ScenarioInput) (Scenario, error) {
	d := scenarioDraft{
		Service:          strings.TrimSpace(in.Service),
		Name:             strings.TrimSpace(in.Name),
		RequestMethod:    in.RequestMethod,
		RequestPath:      in.RequestPath,
		RequestCondition: in.RequestCondition,
		ResponseCode:     in.ResponseCode,
		ResponseHeaders:  in.ResponseHeaders,
		ResponseBody:     in.ResponseBody,
		SkipProxy:        in.SkipProxy,
	}
	f, err := a.scenarioFields(ctx, &d)
	if err != nil {
		return Scenario{}, err
	}
	s, err := a.scenarios.Create(ctx, f)
	if err != nil {
		return Scenario{}, StoreError(err, nil, scenarioNameTaken(d.Name, d.Service))
	}
	a.scenarioChanged(ctx, actor, logx.ActionCreate, s)
	return scenarioFromStore(s), nil
}

// UpdateScenario changes the fields present in p of the scenario with id. Setting
// service moves the scenario to another service.
func (a *App) UpdateScenario(ctx context.Context, actor Actor, id int64, p ScenarioPatch) (Scenario, error) {
	var v httpx.Validator
	v.Check(!p.Service.Null, "service", "is required")
	v.Check(!p.Name.Null, "name", "is required")
	v.Check(!p.SkipProxy.Null, "skipProxy", "must be true or false")
	if !v.OK() {
		return Scenario{}, Invalid(v.Details())
	}

	cur, err := a.scenarios.Get(ctx, id)
	if err != nil {
		return Scenario{}, StoreError(err, scenarioNotFound(id), nil)
	}
	d := scenarioDraft{
		Service:          cur.ServicePath,
		Name:             cur.Name,
		RequestMethod:    cur.RequestMethod,
		RequestPath:      cur.RequestPath,
		RequestCondition: cur.RequestCondition,
		ResponseCode:     cur.ResponseCode,
		ResponseHeaders:  cur.ResponseHeaders,
		ResponseBody:     cur.ResponseBody,
		SkipProxy:        cur.SkipProxy,
	}
	if s, ok := p.Service.Get(); ok {
		d.Service = strings.TrimSpace(s)
	}
	if s, ok := p.Name.Get(); ok {
		d.Name = strings.TrimSpace(s)
	}
	patchPtr(&d.RequestMethod, p.RequestMethod)
	patchPtr(&d.RequestPath, p.RequestPath)
	patchPtr(&d.RequestCondition, p.RequestCondition)
	patchPtr(&d.ResponseCode, p.ResponseCode)
	patchPtr(&d.ResponseBody, p.ResponseBody)
	if p.ResponseHeaders.Set {
		d.ResponseHeaders = p.ResponseHeaders.Value // nil when null
	}
	if b, ok := p.SkipProxy.Get(); ok {
		d.SkipProxy = b
	}
	f, err := a.scenarioFields(ctx, &d)
	if err != nil {
		return Scenario{}, err
	}
	s, err := a.scenarios.Update(ctx, id, f)
	if err != nil {
		// NotFound when the scenario was deleted since it was read.
		return Scenario{}, StoreError(err, scenarioNotFound(id), scenarioNameTaken(d.Name, d.Service))
	}
	a.scenarioChanged(ctx, actor, logx.ActionUpdate, s)
	return scenarioFromStore(s), nil
}

// DeleteScenario deletes the scenario with id, with its client assignments, and returns
// what was deleted.
func (a *App) DeleteScenario(ctx context.Context, actor Actor, id int64) (Scenario, error) {
	s, err := a.scenarios.Delete(ctx, id)
	if err != nil {
		return Scenario{}, StoreError(err, scenarioNotFound(id), nil)
	}
	a.scenarioChanged(ctx, actor, logx.ActionDelete, s)
	return scenarioFromStore(s), nil
}

// scenarioDraft is a scenario as it is about to be saved, before normalisation.
type scenarioDraft struct {
	Service          string
	Name             string
	RequestMethod    *string
	RequestPath      *string
	RequestCondition *string
	ResponseCode     *int
	ResponseHeaders  map[string]*string
	ResponseBody     *string
	SkipProxy        bool
}

// scenarioFields normalises d (spec §5.1: empty means unset), validates it (spec §6.4)
// and resolves its service. Normalising before validating means that "" and null are the
// same thing everywhere, and that a skipProxy check sees the body as it will be stored.
func (a *App) scenarioFields(ctx context.Context, d *scenarioDraft) (store.ScenarioFields, error) {
	d.RequestMethod = normalise(d.RequestMethod, func(s string) string { return strings.ToUpper(strings.TrimSpace(s)) })
	d.RequestPath = normalise(d.RequestPath, func(s string) string { return strings.Trim(strings.TrimSpace(s), "/") })
	d.RequestCondition = normalise(d.RequestCondition, strings.TrimSpace)
	// The body is kept as written: leading white space can matter, and the template
	// marker must stay at the very start (spec §8.1).
	d.ResponseBody = normalise(d.ResponseBody, func(s string) string { return s })
	if len(d.ResponseHeaders) == 0 {
		d.ResponseHeaders = nil
	}

	var v httpx.Validator
	v.Required("service", d.Service != "")
	if v.Required("name", d.Name != "") {
		v.Length("name", d.Name, 1, maxScenarioName)
	}
	if m := d.RequestMethod; m != nil {
		v.Check(slices.Contains(RequestMethods, *m), "requestMethod", "must be one of "+strings.Join(RequestMethods, ", "))
	}
	if c := d.ResponseCode; c != nil {
		v.Check(*c >= 100 && *c <= 599, "responseCode", "must be between 100 and 599")
	}
	if msg := headersProblem(d.ResponseHeaders); msg != "" {
		v.Add("responseHeaders", msg)
	}
	v.Check(!d.SkipProxy || d.ResponseBody != nil, "skipProxy", "requires a response body")
	if !v.OK() {
		return store.ScenarioFields{}, Invalid(v.Details())
	}

	svc, err := a.services.Get(ctx, d.Service)
	if err != nil {
		notFound := serviceNotFound(d.Service)
		notFound.Field = "service"
		return store.ScenarioFields{}, StoreError(err, notFound, nil)
	}
	return store.ScenarioFields{
		ServiceID:        svc.ID,
		Name:             d.Name,
		RequestMethod:    d.RequestMethod,
		RequestPath:      d.RequestPath,
		RequestCondition: d.RequestCondition,
		ResponseCode:     d.ResponseCode,
		ResponseHeaders:  d.ResponseHeaders,
		ResponseBody:     d.ResponseBody,
		SkipProxy:        d.SkipProxy,
	}, nil
}

// headersProblem checks response headers: every name must be a valid HTTP header name,
// no two names may differ only in case (HTTP header names are case-insensitive, so the
// proxy could not tell them apart), and no value may contain a line break. It returns
// the first problem, in name order, or "".
func headersProblem(h map[string]*string) string {
	names := make([]string, 0, len(h))
	for n := range h {
		names = append(names, n)
	}
	sort.Strings(names)
	seen := map[string]string{}
	for _, n := range names {
		switch {
		case n == "":
			return "every header needs a name"
		case !isHeaderName(n):
			return fmt.Sprintf("%q is not a valid header name", n)
		}
		canon := http.CanonicalHeaderKey(n)
		if prev, ok := seen[canon]; ok {
			return fmt.Sprintf("%q and %q are the same header", prev, n)
		}
		seen[canon] = n
		if val := h[n]; val != nil && strings.ContainsAny(*val, "\r\n\x00") {
			return fmt.Sprintf("the value of %q must not contain line breaks", n)
		}
	}
	return ""
}

// isHeaderName reports whether s is an HTTP token (RFC 9110 §5.1).
func isHeaderName(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return s != ""
}

// normalise applies clean to *p and returns nil when p is nil or the result is empty.
func normalise(p *string, clean func(string) string) *string {
	if p == nil {
		return nil
	}
	s := clean(*p)
	if s == "" {
		return nil
	}
	return &s
}

// patchPtr applies an optional field of a PATCH to an optional value: absent keeps it,
// null clears it, a value sets it.
func patchPtr[T any](dst **T, o httpx.Optional[T]) {
	switch {
	case !o.Set:
	case o.Null:
		*dst = nil
	default:
		v := o.Value
		*dst = &v
	}
}

func scenarioFromStore(s store.Scenario) Scenario {
	return Scenario{
		ID:               s.ID,
		Service:          s.ServicePath,
		Name:             s.Name,
		RequestMethod:    s.RequestMethod,
		RequestPath:      s.RequestPath,
		RequestCondition: s.RequestCondition,
		ResponseCode:     s.ResponseCode,
		ResponseHeaders:  s.ResponseHeaders,
		ResponseBody:     s.ResponseBody,
		SkipProxy:        s.SkipProxy,
	}
}

func scenarioNotFound(id int64) *Error { return NotFound("Scenario %d not found", id) }

func scenarioNameTaken(name, service string) *Error {
	e := Conflict("A scenario named %q already exists in service %q", name, service)
	e.Field = "name"
	return e
}

// scenarioChanged logs tmock.entity.changed for a scenario (spec §2.4.2), with its name
// and its service's path, which is how people know it.
func (a *App) scenarioChanged(ctx context.Context, actor Actor, action string, s store.Scenario) {
	a.entityChanged(ctx, actor, logx.EntityScenario, s.ID, action,
		slog.String(logx.AttrService, s.ServicePath), slog.String(logx.AttrScenario, s.Name))
}
