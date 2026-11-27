package app_test

import (
	"context"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
)

func ptr[T any](v T) *T { return &v }

// newScenarioApp is newApp with the services "svc" and "other".
func newScenarioApp(t *testing.T) (*app.App, *logxtest.Recorder) {
	t.Helper()
	a, _, logs := newApp(t)
	for _, p := range []string{"svc", "other"} {
		if _, err := a.CreateService(context.Background(), ann, app.ServiceInput{Path: p, Name: strings.ToUpper(p), UpstreamURL: "http://up.test"}); err != nil {
			t.Fatal(err)
		}
	}
	return a, logs
}

func TestScenarioValidation(t *testing.T) {
	t.Parallel()
	a, _ := newScenarioApp(t)
	ctx := context.Background()
	base := func(f func(*app.ScenarioInput)) app.ScenarioInput {
		in := app.ScenarioInput{Service: "svc", Name: "s"}
		f(&in)
		return in
	}
	cases := []struct {
		name  string
		in    app.ScenarioInput
		field string
		msg   string
	}{
		{"service missing", base(func(in *app.ScenarioInput) { in.Service = " " }), "service", "is required"},
		{"name missing", base(func(in *app.ScenarioInput) { in.Name = "" }), "name", "is required"},
		{"name too long", base(func(in *app.ScenarioInput) { in.Name = strings.Repeat("x", 256) }), "name", "must be 1–255 characters"},
		{"bad method", base(func(in *app.ScenarioInput) { in.RequestMethod = ptr("FETCH") }), "requestMethod", "must be one of GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS"},
		{"condition syntax", base(func(in *app.ScenarioInput) { in.RequestCondition = ptr("$request.method = 'GET'") }), "requestCondition", `column 17: "=" is not an operator; use "==" to compare`},
		{"condition unknown name", base(func(in *app.ScenarioInput) { in.RequestCondition = ptr(" req.x == 1") }), "requestCondition", `column 1: unknown name "req": values come from $request, strings need quotes`},
		{"condition unbalanced", base(func(in *app.ScenarioInput) { in.RequestCondition = ptr("($request.method == 'GET'") }), "requestCondition", `column 26: expected ")" but found end of condition`},
		{"code too low", base(func(in *app.ScenarioInput) { in.ResponseCode = ptr(99) }), "responseCode", "must be between 100 and 599"},
		{"code too high", base(func(in *app.ScenarioInput) { in.ResponseCode = ptr(600) }), "responseCode", "must be between 100 and 599"},
		{"code 0", base(func(in *app.ScenarioInput) { in.ResponseCode = ptr(0) }), "responseCode", "must be between 100 and 599"},
		{"header without name", base(func(in *app.ScenarioInput) { in.ResponseHeaders = map[string]*string{"": ptr("x")} }), "responseHeaders", "every header needs a name"},
		{"bad header name", base(func(in *app.ScenarioInput) { in.ResponseHeaders = map[string]*string{"X Y": ptr("x")} }), "responseHeaders", `"X Y" is not a valid header name`},
		{"same header twice", base(func(in *app.ScenarioInput) {
			in.ResponseHeaders = map[string]*string{"x-a": ptr("1"), "X-A": ptr("2")}
		}), "responseHeaders", `"X-A" and "x-a" are the same header`},
		{"line break in a value", base(func(in *app.ScenarioInput) { in.ResponseHeaders = map[string]*string{"X-A": ptr("a\r\nB: c")} }), "responseHeaders", `the value of "X-A" must not contain line breaks`},
		{"skipProxy without body", base(func(in *app.ScenarioInput) { in.SkipProxy = true }), "skipProxy", "requires a response body"},
		{"skipProxy with an empty body", base(func(in *app.ScenarioInput) { in.SkipProxy, in.ResponseBody = true, ptr("") }), "skipProxy", "requires a response body"},
	}
	for _, tc := range cases {
		_, err := a.CreateScenario(ctx, ann, tc.in)
		e := appErr(t, err, app.KindInvalid)
		if e.Details[tc.field] != tc.msg {
			t.Errorf("%s: details %v, want %s: %s", tc.name, e.Details, tc.field, tc.msg)
		}
	}

	// An unknown service is 404, after validation.
	_, err := a.CreateScenario(ctx, ann, app.ScenarioInput{Service: "nope", Name: "s"})
	if e := appErr(t, err, app.KindNotFound); e.Message != `Service "nope" not found` {
		t.Errorf("unknown service: %v", e)
	}
	if list, _ := a.ListScenarios(ctx, ""); len(list) != 0 {
		t.Errorf("invalid scenarios were stored: %+v", list)
	}
}

// Verify item: unset vs value for every toggled field.
func TestScenarioUnsetAndValues(t *testing.T) {
	t.Parallel()
	a, _ := newScenarioApp(t)
	ctx := context.Background()

	if list, err := a.ListScenarios(ctx, ""); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty list must be [] not nil: %#v, %v", list, err)
	}

	// Values are normalised: the method upper-cased, the path trimmed of '/', white space
	// trimmed (but not from the body).
	s, err := a.CreateScenario(ctx, ann, app.ScenarioInput{
		Service: " svc ", Name: " Full ",
		RequestMethod: ptr(" post "), RequestPath: ptr(" /orders/42/ "), RequestCondition: ptr(" $request.method == 'POST' "),
		ResponseCode:    ptr(201),
		ResponseHeaders: map[string]*string{"Content-Type": ptr("application/json"), "ETag": nil},
		ResponseBody:    ptr("  {\"ok\": true}\n"), SkipProxy: true,
	})
	want := app.Scenario{ID: s.ID, Service: "svc", Name: "Full",
		RequestMethod: ptr("POST"), RequestPath: ptr("orders/42"), RequestCondition: ptr("$request.method == 'POST'"),
		ResponseCode:    ptr(201),
		ResponseHeaders: map[string]*string{"Content-Type": ptr("application/json"), "ETag": nil},
		ResponseBody:    ptr("  {\"ok\": true}\n"), SkipProxy: true}
	if err != nil || !reflect.DeepEqual(s, want) {
		t.Fatalf("create: %+v, %v", s, err)
	}
	if got, err := a.GetScenario(ctx, s.ID); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("get: %+v, %v", got, err)
	}

	// Empty values are unset, the same as null (spec §5.1).
	e, err := a.CreateScenario(ctx, ann, app.ScenarioInput{Service: "svc", Name: "Empty",
		RequestMethod: ptr(""), RequestPath: ptr("/"), RequestCondition: ptr("  "),
		ResponseHeaders: map[string]*string{}, ResponseBody: ptr("")})
	if err != nil || e.RequestMethod != nil || e.RequestPath != nil || e.RequestCondition != nil || e.ResponseHeaders != nil || e.ResponseBody != nil {
		t.Errorf("empty values: %+v, %v", e, err)
	}

	// PATCH: absent keeps, null clears, a value sets; for every optional field.
	if got, err := a.UpdateScenario(ctx, ann, s.ID, app.ScenarioPatch{}); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("empty patch: %+v, %v", got, err)
	}
	got, err := a.UpdateScenario(ctx, ann, s.ID, app.ScenarioPatch{
		RequestMethod: httpx.Null[string](), RequestPath: httpx.Null[string](), RequestCondition: httpx.Null[string](),
		ResponseCode: httpx.Null[int](), ResponseHeaders: httpx.Null[map[string]*string](),
		ResponseBody: httpx.Null[string](), SkipProxy: httpx.Some(false),
	})
	if err != nil || !reflect.DeepEqual(got, app.Scenario{ID: s.ID, Service: "svc", Name: "Full"}) {
		t.Errorf("clearing every field: %+v, %v", got, err)
	}
	got, err = a.UpdateScenario(ctx, ann, s.ID, app.ScenarioPatch{
		RequestMethod: httpx.Some("delete"), ResponseCode: httpx.Some(404),
		ResponseHeaders: httpx.Some(map[string]*string{"X-A": ptr("1")}), ResponseBody: httpx.Some("gone"),
	})
	if err != nil || *got.RequestMethod != "DELETE" || *got.ResponseCode != 404 || *got.ResponseHeaders["X-A"] != "1" || *got.ResponseBody != "gone" || got.RequestPath != nil {
		t.Errorf("setting fields: %+v, %v", got, err)
	}

	// Clearing the body while skipProxy stays true is refused.
	if _, err := a.UpdateScenario(ctx, ann, s.ID, app.ScenarioPatch{SkipProxy: httpx.Some(true)}); err != nil {
		t.Fatal(err)
	}
	_, err = a.UpdateScenario(ctx, ann, s.ID, app.ScenarioPatch{ResponseBody: httpx.Null[string]()})
	if e := appErr(t, err, app.KindInvalid); e.Details["skipProxy"] != "requires a response body" {
		t.Errorf("clearing the body of a skip-proxy scenario: %v", e.Details)
	}

	// null on the required fields.
	_, err = a.UpdateScenario(ctx, ann, s.ID, app.ScenarioPatch{Service: httpx.Null[string](), Name: httpx.Null[string](), SkipProxy: httpx.Null[bool]()})
	if e := appErr(t, err, app.KindInvalid); len(e.Details) != 3 || e.Details["name"] != "is required" || e.Details["skipProxy"] != "must be true or false" {
		t.Errorf("null required fields: %v", e.Details)
	}
}

// Verify item: duplicate names within a service.
func TestScenarioCRUD(t *testing.T) {
	t.Parallel()
	a, logs := newScenarioApp(t)
	ctx := context.Background()

	s, err := a.CreateScenario(ctx, ann, app.ScenarioInput{Service: "svc", Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.CreateScenario(ctx, ann, app.ScenarioInput{Service: "svc", Name: "b"})
	if e := appErr(t, err, app.KindConflict); e.Message != `A scenario named "b" already exists in service "svc"` || e.StatusCode() != 409 {
		t.Errorf("duplicate: %v", e)
	}
	// The same name in another service is fine.
	o, err := a.CreateScenario(ctx, ann, app.ScenarioInput{Service: "other", Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateScenario(ctx, ann, app.ScenarioInput{Service: "svc", Name: "a"}); err != nil {
		t.Fatal(err)
	}

	// Ordered by service path, then name; the filter keeps one service.
	list, _ := a.ListScenarios(ctx, "")
	var order []string
	for _, x := range list {
		order = append(order, x.Service+"/"+x.Name)
	}
	if strings.Join(order, " ") != "other/b svc/a svc/b" {
		t.Errorf("list order: %v", order)
	}
	if list, _ := a.ListScenarios(ctx, "svc"); len(list) != 2 {
		t.Errorf("filter: %+v", list)
	}
	if list, err := a.ListScenarios(ctx, "nope"); err != nil || list == nil || len(list) != 0 {
		t.Errorf("unknown service filter: %#v, %v", list, err)
	}

	// Moving into a service where the name is taken is a conflict; to an unknown one 404.
	_, err = a.UpdateScenario(ctx, ann, o.ID, app.ScenarioPatch{Service: httpx.Some("svc")})
	if e := appErr(t, err, app.KindConflict); !strings.Contains(e.Message, `in service "svc"`) {
		t.Errorf("move onto a taken name: %v", e)
	}
	_, err = a.UpdateScenario(ctx, ann, o.ID, app.ScenarioPatch{Service: httpx.Some("nope")})
	appErr(t, err, app.KindNotFound)
	moved, err := a.UpdateScenario(ctx, ann, o.ID, app.ScenarioPatch{Service: httpx.Some("svc"), Name: httpx.Some("c")})
	if err != nil || moved.Service != "svc" || moved.Name != "c" {
		t.Errorf("move: %+v, %v", moved, err)
	}

	_, err = a.GetScenario(ctx, 999999)
	if e := appErr(t, err, app.KindNotFound); e.Message != "Scenario 999999 not found" {
		t.Errorf("get missing: %v", e)
	}
	_, err = a.UpdateScenario(ctx, ann, 999999, app.ScenarioPatch{})
	appErr(t, err, app.KindNotFound)

	if d, err := a.DeleteScenario(ctx, ann, s.ID); err != nil || d.Name != "b" {
		t.Errorf("delete: %+v, %v", d, err)
	}
	_, err = a.DeleteScenario(ctx, ann, s.ID)
	appErr(t, err, app.KindNotFound)

	// Deleting a service deletes its scenarios.
	if _, err := a.DeleteService(ctx, ann, "svc"); err != nil {
		t.Fatal(err)
	}
	if list, _ := a.ListScenarios(ctx, ""); len(list) != 0 {
		t.Errorf("scenarios left after deleting their service: %+v", list)
	}

	var actions []string
	for _, e := range logs.Events(logx.EventEntityChanged) {
		if e.Attrs[logx.AttrEntityType] == logx.EntityScenario {
			actions = append(actions, e.Attrs[logx.AttrEntityAction]+":"+e.Attrs[logx.AttrService]+"/"+e.Attrs[logx.AttrScenario])
		}
	}
	if got := strings.Join(actions, " "); got != "create:svc/b create:other/b create:svc/a update:svc/c delete:svc/b" {
		t.Errorf("events: %s", got)
	}
}

// The tmock.entity.changed event for a scenario: name, level and attributes (spec §15.1).
func TestScenarioChangedEvent(t *testing.T) {
	t.Parallel()
	a, logs := newScenarioApp(t)
	s, err := a.CreateScenario(context.Background(), ann, app.ScenarioInput{Service: "svc", Name: "Slow"})
	if err != nil {
		t.Fatal(err)
	}
	var events []logxtest.Record
	for _, e := range logs.Events(logx.EventEntityChanged) {
		if e.Attrs[logx.AttrEntityType] == "scenario" {
			events = append(events, e)
		}
	}
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	e := events[0]
	if e.Level != slog.LevelInfo {
		t.Errorf("level %v", e.Level)
	}
	for k, v := range map[string]string{
		logx.AttrEntityAction: "create",
		logx.AttrEntityID:     strconv.FormatInt(s.ID, 10),
		logx.AttrActor:        "user",
		logx.AttrUserID:       "7",
		logx.AttrService:      "svc",
		logx.AttrScenario:     "Slow",
	} {
		if e.Attrs[k] != v {
			t.Errorf("%s = %q, want %q", k, e.Attrs[k], v)
		}
	}
}
