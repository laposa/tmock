package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/laposa/tmock/app-go/internal/condition"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Client is a client as the API and the UI show it (spec §6.1). The token itself is
// never shown again after it was generated; HasToken says whether there is one.
type Client struct {
	ID        int64                `json:"id"`
	Name      string               `json:"name"`
	Enabled   bool                 `json:"enabled"`
	Condition *condition.Condition `json:"condition"`
	Scenarios []int64              `json:"scenarios"` // never nil
	HasToken  bool                 `json:"hasToken"`
	// Assigned are the assigned scenarios with their service and name, for the UI's
	// chips (spec §10.3 C1), ordered by service path and scenario name.
	Assigned []AssignedScenario `json:"-"`
}

// AssignedScenario is a scenario assigned to a client.
type AssignedScenario struct {
	ID      int64
	Service string // the service's path
	Name    string
}

// ClientInput is the body of a create (spec §6.4).
type ClientInput struct {
	Name      string               `json:"name"`
	Enabled   bool                 `json:"enabled"`
	Condition *condition.Condition `json:"condition"`
	Scenarios []int64              `json:"scenarios"`
}

// ClientPatch is the body of an update (spec §6.4). name, enabled and scenarios cannot be
// null; null clears the condition. scenarios is the full list of assigned scenario ids.
type ClientPatch struct {
	Name      httpx.Optional[string]               `json:"name"`
	Enabled   httpx.Optional[bool]                 `json:"enabled"`
	Condition httpx.Optional[*condition.Condition] `json:"condition"`
	Scenarios httpx.Optional[[]int64]              `json:"scenarios"`
}

// TokenFields are the only ClientPatch fields, as JSON names, that a request with a
// client token may send (spec §6.3).
var TokenFields = []string{"enabled", "scenarios"}

// Token is a newly generated client token: shown once, never stored (spec §6.4).
type Token struct {
	Token string `json:"token"`
}

// TokenPrefix starts every client token.
const TokenPrefix = "tmck_"

const (
	maxClientName = 255
	tokenBytes    = 32
)

// NeedsConditionMessage is the error for enabling a client whose condition is empty
// (spec §6.4, §10.3 C2).
const NeedsConditionMessage = "a client needs a condition before it can be enabled"

// ListClients returns every client, ordered by id (spec §5.3). The list is never nil.
func (a *App) ListClients(ctx context.Context) ([]Client, error) {
	rows, err := a.clients.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Client, 0, len(rows))
	for _, c := range rows {
		cl, err := clientFromStore(c)
		if err != nil {
			return nil, err
		}
		out = append(out, cl)
	}
	return out, nil
}

// GetClient returns the client with id, or NotFound.
func (a *App) GetClient(ctx context.Context, id int64) (Client, error) {
	c, err := a.clients.Get(ctx, id)
	if err != nil {
		return Client{}, StoreError(err, clientNotFound(id), nil)
	}
	return clientFromStore(c)
}

// CreateClient validates in and creates the client. A taken name is a Conflict; unknown
// scenario ids are a validation error.
func (a *App) CreateClient(ctx context.Context, actor Actor, in ClientInput) (Client, error) {
	scenarios, unknown, err := a.checkScenarios(ctx, in.Scenarios)
	if err != nil {
		return Client{}, err
	}
	d := clientDraft{Name: strings.TrimSpace(in.Name), Enabled: in.Enabled, Condition: in.Condition}
	f, err := clientFields(d, unknown)
	if err != nil {
		return Client{}, err
	}
	c, err := a.clients.Create(ctx, f, scenarios)
	if err != nil {
		return Client{}, clientStoreError(err, 0, d.Name)
	}
	a.clientChanged(ctx, actor, logx.ActionCreate, c.ID, c.Name)
	return clientFromStore(c)
}

// UpdateClient changes the fields present in p of the client with id. The enable rule is
// checked on the result: a client that is (or stays) enabled needs a condition. The
// client's row is locked from the read to the write, so concurrent updates of one client
// run one after the other and none is lost.
func (a *App) UpdateClient(ctx context.Context, actor Actor, id int64, p ClientPatch) (Client, error) {
	var v httpx.Validator
	v.Check(!p.Name.Null, "name", "is required")
	v.Check(!p.Enabled.Null, "enabled", "must be true or false")
	v.Check(!p.Scenarios.Null, "scenarios", "must be a list of scenario ids")
	if !v.OK() {
		return Client{}, Invalid(v.Details())
	}
	var scenarios []int64 // nil keeps the assignments
	var unknown string
	if ids, ok := p.Scenarios.Get(); ok {
		var err error
		if scenarios, unknown, err = a.checkScenarios(ctx, ids); err != nil {
			return Client{}, err
		}
	}

	var name string // for the Conflict message
	c, err := a.clients.Modify(ctx, id, func(cur store.Client) (store.ClientFields, []int64, error) {
		curCond, err := decodeCondition(cur)
		if err != nil {
			return store.ClientFields{}, nil, err
		}
		d := clientDraft{Name: cur.Name, Enabled: cur.Enabled, Condition: curCond}
		if s, ok := p.Name.Get(); ok {
			d.Name = strings.TrimSpace(s)
		}
		if b, ok := p.Enabled.Get(); ok {
			d.Enabled = b
		}
		if p.Condition.Set {
			d.Condition = p.Condition.Value // nil when null
		}
		name = d.Name
		f, err := clientFields(d, unknown)
		return f, scenarios, err
	})
	if err != nil {
		return Client{}, clientStoreError(err, id, name)
	}
	a.clientChanged(ctx, actor, logx.ActionUpdate, c.ID, c.Name)
	return clientFromStore(c)
}

// SetClientScenario assigns the scenario to the client, or removes it, keeping the
// other assignments: the assignment modal's switches and the chips' ✕ (spec §10.3 C5,
// C6). It changes the one assignment only, so concurrent toggles do not undo each other.
// A scenario that is already in the wanted state changes nothing and logs nothing.
func (a *App) SetClientScenario(ctx context.Context, actor Actor, id, scenarioID int64, assigned bool) (Client, error) {
	c, changed, err := a.clients.SetScenario(ctx, id, scenarioID, assigned)
	switch {
	case errors.Is(err, store.ErrForeignKey):
		return Client{}, Invalid(map[string]string{"scenarios": "unknown scenario ids: " + strconv.FormatInt(scenarioID, 10)}).wrap(err)
	case err != nil:
		return Client{}, StoreError(err, clientNotFound(id), nil)
	}
	if changed {
		a.clientChanged(ctx, actor, logx.ActionUpdate, c.ID, c.Name)
	}
	return clientFromStore(c)
}

// DeleteClient deletes the client with id, with its assignments, and returns it.
func (a *App) DeleteClient(ctx context.Context, actor Actor, id int64) (Client, error) {
	c, err := a.clients.Delete(ctx, id)
	if err != nil {
		return Client{}, StoreError(err, clientNotFound(id), nil)
	}
	a.clientChanged(ctx, actor, logx.ActionDelete, c.ID, c.Name)
	return clientFromStore(c)
}

// DisableAllClients disables every client (spec §6.4).
func (a *App) DisableAllClients(ctx context.Context, actor Actor) error {
	n, err := a.clients.DisableAll(ctx)
	if err != nil {
		return err
	}
	a.invalidateCatalog()
	attrs := append(actor.attrs(),
		slog.String(logx.AttrEventName, logx.EventEntityChanged),
		slog.String(logx.AttrEntityType, logx.EntityClient),
		slog.String(logx.AttrEntityAction, logx.ActionDisableAll),
		slog.Int64(logx.AttrClientsDisabled, n),
	)
	a.logger.LogAttrs(ctx, slog.LevelInfo, fmt.Sprintf("clients: disable-all (%d disabled)", n), attrs...)
	return nil
}

// GenerateClientToken gives the client a new token, replacing any other, and returns it.
// Only its SHA-256 is stored, so this is the only time it is shown (spec §6.4, §12).
func (a *App) GenerateClientToken(ctx context.Context, actor Actor, id int64) (Token, error) {
	raw := make([]byte, tokenBytes)
	_, _ = rand.Read(raw) // never fails (crypto/rand)
	token := TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	name, err := a.clients.SetTokenHash(ctx, id, tokenHash(token))
	if err != nil {
		return Token{}, StoreError(err, clientNotFound(id), nil)
	}
	a.clientChanged(ctx, actor, logx.ActionTokenGenerate, id, name)
	return Token{Token: token}, nil
}

// RevokeClientToken removes the client's token. A client without one is not an error.
func (a *App) RevokeClientToken(ctx context.Context, actor Actor, id int64) error {
	name, err := a.clients.SetTokenHash(ctx, id, nil)
	if err != nil {
		return StoreError(err, clientNotFound(id), nil)
	}
	a.clientChanged(ctx, actor, logx.ActionTokenRevoke, id, name)
	return nil
}

// ClientByToken returns the id of the client that token belongs to, or Unauthorized
// (spec §6.3).
func (a *App) ClientByToken(ctx context.Context, token string) (int64, error) {
	id, err := a.clients.FindByTokenHash(ctx, tokenHash(token))
	if err != nil {
		return 0, StoreError(err, Unauthorized("Unknown client token"), nil)
	}
	return id, nil
}

// tokenHash is what is stored of a token: its SHA-256 (spec §5.1). Tokens are long and
// random, so a plain hash is enough; it also keeps tokens imported from the legacy
// database working (spec §13.1).
func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// clientDraft is a client as it is about to be saved, without its assignments.
type clientDraft struct {
	Name      string
	Enabled   bool
	Condition *condition.Condition
}

// checkScenarios normalises the scenario ids of a create or an update (sorted, duplicates
// dropped, never nil) and, if some do not exist, returns the validation message for them.
// It runs before the client's row is locked, so that clientFields needs no database.
func (a *App) checkScenarios(ctx context.Context, ids []int64) (norm []int64, unknown string, err error) {
	norm = slices.Compact(slices.Sorted(slices.Values(ids)))
	if norm == nil {
		norm = []int64{}
	}
	exist, err := a.scenarios.ExistingScenarioIDs(ctx, norm)
	if err != nil {
		return nil, "", err
	}
	var missing []string
	for _, id := range norm {
		if !exist[id] {
			missing = append(missing, strconv.FormatInt(id, 10))
		}
	}
	if len(missing) > 0 {
		unknown = "unknown scenario ids: " + strings.Join(missing, ", ")
	}
	return norm, unknown, nil
}

// clientFields validates d (spec §6.4), with unknown, the message of checkScenarios, if
// any, and returns the columns to store.
func clientFields(d clientDraft, unknown string) (store.ClientFields, error) {
	var v httpx.Validator
	if v.Required("name", d.Name != "") {
		v.Length("name", d.Name, 1, maxClientName)
	}
	for _, p := range condition.Validate(d.Condition) {
		v.Add("condition"+pathSuffix(p.Path), p.Message)
	}
	if d.Enabled && condition.Empty(d.Condition) {
		v.Add("enabled", NeedsConditionMessage)
	}
	if unknown != "" {
		v.Add("scenarios", unknown)
	}
	if !v.OK() {
		return store.ClientFields{}, Invalid(v.Details())
	}
	f := store.ClientFields{Name: d.Name, Enabled: d.Enabled}
	if d.Condition != nil {
		b, err := json.Marshal(d.Condition)
		if err != nil {
			return store.ClientFields{}, fmt.Errorf("encoding the condition: %w", err)
		}
		f.Condition = b
	}
	return f, nil
}

// pathSuffix joins a condition problem's path to "condition": "" → "", "ip" → ".ip",
// "or[0].ip" → ".or[0].ip".
func pathSuffix(p string) string {
	if p == "" {
		return ""
	}
	return "." + p
}

// clientStoreError translates a failed write: a taken name is a Conflict next to the
// name, a scenario deleted meanwhile a 400.
func clientStoreError(err error, id int64, name string) error {
	taken := Conflict("A client named %q already exists", name)
	taken.Field = "name"
	var notFound *Error
	if id != 0 {
		notFound = clientNotFound(id)
	}
	return StoreError(err, notFound, taken)
}

func clientFromStore(c store.Client) (Client, error) {
	cond, err := decodeCondition(c)
	if err != nil {
		return Client{}, err
	}
	out := Client{ID: c.ID, Name: c.Name, Enabled: c.Enabled, Condition: cond, HasToken: c.HasToken, Scenarios: []int64{}}
	for _, s := range c.Scenarios {
		out.Scenarios = append(out.Scenarios, s.ID)
		out.Assigned = append(out.Assigned, AssignedScenario{ID: s.ID, Service: s.ServicePath, Name: s.Name})
	}
	slices.Sort(out.Scenarios)
	return out, nil
}

func decodeCondition(c store.Client) (*condition.Condition, error) {
	if c.Condition == nil {
		return nil, nil
	}
	var cond *condition.Condition
	if err := json.Unmarshal(c.Condition, &cond); err != nil {
		return nil, fmt.Errorf("client %d: stored condition: %w", c.ID, err)
	}
	return cond, nil
}

func clientNotFound(id int64) *Error { return NotFound("Client %d not found", id) }

// clientChanged logs tmock.entity.changed for a client (spec §2.4.2), with its name.
func (a *App) clientChanged(ctx context.Context, actor Actor, action string, id int64, name string) {
	a.entityChanged(ctx, actor, logx.EntityClient, id, action, slog.String(logx.AttrClient, name))
}
