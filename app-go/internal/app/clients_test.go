package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/condition"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
)

func cond(t *testing.T, s string) *condition.Condition {
	t.Helper()
	c, err := condition.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// newClientApp is newScenarioApp with the scenarios svc/a and svc/b; it returns their ids.
func newClientApp(t *testing.T) (*app.App, *logxtest.Recorder, int64, int64) {
	t.Helper()
	a, logs := newScenarioApp(t)
	sa, err := a.CreateScenario(context.Background(), ann, app.ScenarioInput{Service: "svc", Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	sb, err := a.CreateScenario(context.Background(), ann, app.ScenarioInput{Service: "svc", Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	return a, logs, sa.ID, sb.ID
}

// Verify items: the enable rule, nested condition validation with field paths, unknown
// scenario ids.
func TestClientValidation(t *testing.T) {
	t.Parallel()
	a, _, sa, _ := newClientApp(t)
	ctx := context.Background()
	cases := []struct {
		name  string
		in    app.ClientInput
		field string
		msg   string
	}{
		{"name missing", app.ClientInput{Name: " "}, "name", "is required"},
		{"name too long", app.ClientInput{Name: strings.Repeat("x", 256)}, "name", "must be 1–255 characters"},
		{"enabled without condition", app.ClientInput{Name: "c", Enabled: true}, "enabled", app.NeedsConditionMessage},
		{"enabled with an empty condition", app.ClientInput{Name: "c", Enabled: true, Condition: cond(t, `{"and":[{"or":[]}]}`)}, "enabled", app.NeedsConditionMessage},
		{"nested bad cidr", app.ClientInput{Name: "c", Condition: cond(t, `{"or":[{"ip":"1.2.3.4"},{"and":[{"cidr":"nope"}]}]}`)}, "condition.or[1].and[0].cidr", "must be a CIDR prefix, such as 10.0.0.0/8 or 2001:db8::/32"},
		{"two predicates at the root", app.ClientInput{Name: "c", Condition: cond(t, `{"ip":"1.2.3.4","cidr":"1.0.0.0/8"}`)}, "condition", "must have exactly one of and, or, headerMatch, headerRegex, ip, cidr (has 2)"},
		{"bad regex", app.ClientInput{Name: "c", Condition: cond(t, `{"headerRegex":{"header":"x","value":"["}}`)}, "condition.headerRegex.value", "is not a valid regular expression: missing closing ]: `[`"},
		{"unknown scenarios", app.ClientInput{Name: "c", Scenarios: []int64{999998, sa, 999999}}, "scenarios", "unknown scenario ids: 999998, 999999"},
	}
	for _, tc := range cases {
		_, err := a.CreateClient(ctx, ann, tc.in)
		e := appErr(t, err, app.KindInvalid)
		if e.Details[tc.field] != tc.msg {
			t.Errorf("%s: details %v, want %s: %s", tc.name, e.Details, tc.field, tc.msg)
		}
	}
	if list, _ := a.ListClients(ctx); len(list) != 0 {
		t.Errorf("invalid clients were stored: %+v", list)
	}
}

func TestClientCRUD(t *testing.T) {
	t.Parallel()
	a, logs, sa, sb := newClientApp(t)
	ctx := context.Background()

	if list, err := a.ListClients(ctx); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty list must be [] not nil: %#v, %v", list, err)
	}

	// Duplicate scenario ids are dropped; the list is sorted.
	c, err := a.CreateClient(ctx, ann, app.ClientInput{Name: " Tester ", Enabled: true, Condition: cond(t, `{"headerMatch":{"header":"x-test","value":"1"}}`), Scenarios: []int64{sb, sa, sb}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c)
	want := `{"id":` + itoa(c.ID) + `,"name":"Tester","enabled":true,"condition":{"headerMatch":{"header":"x-test","value":"1"}},"scenarios":[` + itoa(sa) + `,` + itoa(sb) + `],"hasToken":false}`
	if string(b) != want {
		t.Errorf("create:\n got %s\nwant %s", b, want)
	}
	if len(c.Assigned) != 2 || c.Assigned[0] != (app.AssignedScenario{ID: sa, Service: "svc", Name: "a"}) {
		t.Errorf("assigned: %+v", c.Assigned)
	}

	// A bare client: disabled, no condition, no scenarios ([] not null).
	bare, err := a.CreateClient(ctx, ann, app.ClientInput{Name: "bare"})
	if b, _ := json.Marshal(bare); err != nil || !strings.Contains(string(b), `"enabled":false,"condition":null,"scenarios":[],"hasToken":false`) {
		t.Errorf("bare: %s, %v", b, err)
	}
	_, err = a.CreateClient(ctx, ann, app.ClientInput{Name: "bare"})
	if e := appErr(t, err, app.KindConflict); e.Message != `A client named "bare" already exists` || e.Field != "name" {
		t.Errorf("duplicate: %+v", e)
	}

	// The enable rule on patches: enabling bare fails; giving it a condition and enabling
	// it in one patch works; clearing the condition of an enabled client fails.
	_, err = a.UpdateClient(ctx, ann, bare.ID, app.ClientPatch{Enabled: httpx.Some(true)})
	if e := appErr(t, err, app.KindInvalid); e.Details["enabled"] != app.NeedsConditionMessage {
		t.Errorf("enable without condition: %v", e.Details)
	}
	got, err := a.UpdateClient(ctx, ann, bare.ID, app.ClientPatch{Enabled: httpx.Some(true), Condition: httpx.Some(cond(t, `{"ip":"10.1.1.1"}`))})
	if err != nil || !got.Enabled || got.Condition == nil {
		t.Errorf("enable with condition: %+v, %v", got, err)
	}
	_, err = a.UpdateClient(ctx, ann, bare.ID, app.ClientPatch{Condition: httpx.Null[*condition.Condition]()})
	if e := appErr(t, err, app.KindInvalid); e.Details["enabled"] != app.NeedsConditionMessage {
		t.Errorf("clear condition while enabled: %v", e.Details)
	}
	got, err = a.UpdateClient(ctx, ann, bare.ID, app.ClientPatch{Enabled: httpx.Some(false), Condition: httpx.Null[*condition.Condition]()})
	if err != nil || got.Enabled || got.Condition != nil {
		t.Errorf("disable and clear: %+v, %v", got, err)
	}

	// Patch: absent keeps, scenarios replaces the list, null on required fields is 400.
	got, err = a.UpdateClient(ctx, ann, c.ID, app.ClientPatch{Scenarios: httpx.Some([]int64{sb})})
	if err != nil || got.Name != "Tester" || !got.Enabled || len(got.Scenarios) != 1 || got.Scenarios[0] != sb {
		t.Errorf("patch scenarios: %+v, %v", got, err)
	}
	got, err = a.UpdateClient(ctx, ann, c.ID, app.ClientPatch{Scenarios: httpx.Some([]int64{})})
	if err != nil || len(got.Scenarios) != 0 {
		t.Errorf("clear scenarios: %+v, %v", got, err)
	}
	_, err = a.UpdateClient(ctx, ann, c.ID, app.ClientPatch{Name: httpx.Null[string](), Enabled: httpx.Null[bool](), Scenarios: httpx.Null[[]int64]()})
	if e := appErr(t, err, app.KindInvalid); len(e.Details) != 3 {
		t.Errorf("nulls: %v", e.Details)
	}
	_, err = a.UpdateClient(ctx, ann, c.ID, app.ClientPatch{Name: httpx.Some("bare")})
	appErr(t, err, app.KindConflict)
	_, err = a.UpdateClient(ctx, ann, 999999, app.ClientPatch{})
	if e := appErr(t, err, app.KindNotFound); e.Message != "Client 999999 not found" {
		t.Errorf("missing: %v", e)
	}

	// SetClientScenario: add, add again (no change), remove.
	got, err = a.SetClientScenario(ctx, ann, c.ID, sa, true)
	if err != nil || len(got.Scenarios) != 1 || got.Scenarios[0] != sa {
		t.Errorf("assign: %+v, %v", got, err)
	}
	if got, err = a.SetClientScenario(ctx, ann, c.ID, sa, true); err != nil || len(got.Scenarios) != 1 {
		t.Errorf("assign twice: %+v, %v", got, err)
	}
	if got, err = a.SetClientScenario(ctx, ann, c.ID, sa, false); err != nil || len(got.Scenarios) != 0 {
		t.Errorf("unassign: %+v, %v", got, err)
	}
	_, err = a.SetClientScenario(ctx, ann, c.ID, 999999, true)
	appErr(t, err, app.KindInvalid)

	// Disable all.
	if err := a.DisableAllClients(ctx, ann); err != nil {
		t.Fatal(err)
	}
	for _, cl := range must(a.ListClients(ctx)) {
		if cl.Enabled {
			t.Errorf("%s still enabled", cl.Name)
		}
	}

	if d, err := a.DeleteClient(ctx, ann, c.ID); err != nil || d.Name != "Tester" {
		t.Errorf("delete: %+v, %v", d, err)
	}
	_, err = a.DeleteClient(ctx, ann, c.ID)
	appErr(t, err, app.KindNotFound)

	var actions []string
	for _, e := range logs.Events(logx.EventEntityChanged) {
		if e.Attrs[logx.AttrEntityType] == logx.EntityClient {
			actions = append(actions, e.Attrs[logx.AttrEntityAction]+":"+e.Attrs[logx.AttrClient])
		}
	}
	if got := strings.Join(actions, " "); got != "create:Tester create:bare update:bare update:bare update:Tester update:Tester update:Tester update:Tester disable-all: delete:Tester" {
		t.Errorf("events: %s", got)
	}
}

// Verify item: tokens. Generated tokens look like tmck_<base64url of 32 bytes>; only the
// hash is stored; a new token replaces the old one; revoking removes it.
func TestClientTokens(t *testing.T) {
	t.Parallel()
	a, logs, _, _ := newClientApp(t)
	ctx := context.Background()
	c, err := a.CreateClient(ctx, ann, app.ClientInput{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ClientByToken(ctx, "tmck_nope"); appErr(t, err, app.KindUnauthorized).Message != "Unknown client token" {
		t.Errorf("unknown token: %v", err)
	}

	t1, err := a.GenerateClientToken(ctx, ann, c.ID)
	if err != nil || !strings.HasPrefix(t1.Token, "tmck_") || len(t1.Token) != len("tmck_")+43 {
		t.Fatalf("token: %q, %v", t1.Token, err)
	}
	if id, err := a.ClientByToken(ctx, t1.Token); err != nil || id != c.ID {
		t.Errorf("by token: %d, %v", id, err)
	}
	if got, _ := a.GetClient(ctx, c.ID); !got.HasToken {
		t.Error("hasToken false")
	}
	t2, _ := a.GenerateClientToken(ctx, ann, c.ID)
	if t2.Token == t1.Token {
		t.Error("regenerate gave the same token")
	}
	if _, err := a.ClientByToken(ctx, t1.Token); err == nil {
		t.Error("the old token still works after regenerating")
	}
	if err := a.RevokeClientToken(ctx, ann, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ClientByToken(ctx, t2.Token); err == nil {
		t.Error("revoked token still works")
	}
	if err := a.RevokeClientToken(ctx, ann, c.ID); err != nil {
		t.Errorf("revoking without a token: %v", err)
	}
	_, err = a.GenerateClientToken(ctx, ann, 999999)
	appErr(t, err, app.KindNotFound)
	appErr(t, a.RevokeClientToken(ctx, ann, 999999), app.KindNotFound)

	// A change made with a token is logged with the client as actor (spec §2.4.2), and no
	// record contains a token.
	if _, err := a.UpdateClient(ctx, app.ClientTokenActor(c.ID), c.ID, app.ClientPatch{Scenarios: httpx.Some([]int64{})}); err != nil {
		t.Fatal(err)
	}
	events := logs.Events(logx.EventEntityChanged)
	last := events[len(events)-1]
	for k, v := range map[string]string{logx.AttrActor: "client-token", logx.AttrClientID: itoa(c.ID), logx.AttrEntityAction: "update"} {
		if last.Attrs[k] != v {
			t.Errorf("%s = %q, want %q", k, last.Attrs[k], v)
		}
	}
	if _, ok := last.Attrs[logx.AttrUserID]; ok {
		t.Error("a token change carries user.id")
	}
	var gen []string
	for _, e := range events {
		gen = append(gen, e.Attrs[logx.AttrEntityAction])
		for _, tok := range []string{t1.Token, t2.Token, t1.Token[5:], t2.Token[5:]} {
			if strings.Contains(e.Text(), tok) {
				t.Errorf("record leaks a token: %s", e.Text())
			}
		}
	}
	if got := strings.Join(gen, " "); !strings.Contains(got, "token-generate token-generate token-revoke token-revoke") {
		t.Errorf("token events: %s", got)
	}
}

// The tmock.entity.changed event for a client and for disable-all (spec §15.1).
func TestClientChangedEvents(t *testing.T) {
	t.Parallel()
	a, logs, _, _ := newClientApp(t)
	ctx := context.Background()
	c, err := a.CreateClient(ctx, ann, app.ClientInput{Name: "Phone"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DisableAllClients(ctx, ann); err != nil {
		t.Fatal(err)
	}
	var events []logxtest.Record
	for _, e := range logs.Events(logx.EventEntityChanged) {
		if e.Attrs[logx.AttrEntityType] == "client" {
			events = append(events, e)
		}
	}
	if len(events) != 2 {
		t.Fatalf("events: %+v", events)
	}
	for i, want := range []map[string]string{
		{logx.AttrEntityAction: "create", logx.AttrEntityID: itoa(c.ID), logx.AttrClient: "Phone", logx.AttrActor: "user", logx.AttrUserID: "7"},
		{logx.AttrEntityAction: "disable-all", logx.AttrClientsDisabled: "0", logx.AttrActor: "user", logx.AttrUserID: "7"},
	} {
		if events[i].Level != slog.LevelInfo {
			t.Errorf("event %d level %v", i, events[i].Level)
		}
		for k, v := range want {
			if events[i].Attrs[k] != v {
				t.Errorf("event %d: %s = %q, want %q", i, k, events[i].Attrs[k], v)
			}
		}
	}
	if _, ok := events[1].Attrs[logx.AttrEntityID]; ok {
		t.Error("disable-all carries an entity id")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
