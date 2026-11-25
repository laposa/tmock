package app_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
)

var ann = app.UserActor(7)

func appErr(t *testing.T, err error, kind app.Kind) *app.Error {
	t.Helper()
	var e *app.Error
	if !errors.As(err, &e) || e.Kind != kind {
		t.Fatalf("got %v, want an *app.Error of kind %d", err, kind)
	}
	return e
}

func TestServiceValidation(t *testing.T) {
	t.Parallel()
	a, _, logs := newApp(t)
	ctx := context.Background()
	long := strings.Repeat("x", 256)
	cases := []struct {
		name  string
		in    app.ServiceInput
		field string
		msg   string
	}{
		{"path missing", app.ServiceInput{Name: "S", UpstreamURL: "http://u"}, "path", "is required"},
		{"path blank", app.ServiceInput{Path: "  ", Name: "S", UpstreamURL: "http://u"}, "path", "is required"},
		{"path with /", app.ServiceInput{Path: "a/b", Name: "S", UpstreamURL: "http://u"}, "path", "may contain only letters, digits, '.', '_' and '-'"},
		{"path with space", app.ServiceInput{Path: "a b", Name: "S", UpstreamURL: "http://u"}, "path", "may contain only letters, digits, '.', '_' and '-'"},
		{"path ..", app.ServiceInput{Path: "..", Name: "S", UpstreamURL: "http://u"}, "path", `must not be "." or ".."`},
		{"path too long", app.ServiceInput{Path: long, Name: "S", UpstreamURL: "http://u"}, "path", "must be 1–255 characters"},
		{"name missing", app.ServiceInput{Path: "s", UpstreamURL: "http://u"}, "name", "is required"},
		{"name too long", app.ServiceInput{Path: "s", Name: long, UpstreamURL: "http://u"}, "name", "must be 1–255 characters"},
		{"url missing", app.ServiceInput{Path: "s", Name: "S"}, "upstreamUrl", "is required"},
		{"url relative", app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "/api"}, "upstreamUrl", "must be an absolute http or https URL"},
		{"url without scheme", app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "example.com"}, "upstreamUrl", "must be an absolute http or https URL"},
		{"url ftp", app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "ftp://example.com"}, "upstreamUrl", "must be an absolute http or https URL"},
		{"url without host", app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "http://"}, "upstreamUrl", "must be an absolute http or https URL"},
		{"url with only a port", app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "http://:8080"}, "upstreamUrl", "must be an absolute http or https URL"},
		{"url malformed", app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "http://[::1"}, "upstreamUrl", "must be an absolute http or https URL"},
		{"url too long", app.ServiceInput{Path: "s", Name: "S", UpstreamURL: "http://u/" + strings.Repeat("x", 2040)}, "upstreamUrl", "must be 1–2048 characters"},
	}
	for _, tc := range cases {
		_, err := a.CreateService(ctx, ann, tc.in)
		e := appErr(t, err, app.KindInvalid)
		if e.Details[tc.field] != tc.msg {
			t.Errorf("%s: details %v, want %s: %s", tc.name, e.Details, tc.field, tc.msg)
		}
	}
	// Every problem is reported at once.
	_, err := a.CreateService(ctx, ann, app.ServiceInput{})
	if e := appErr(t, err, app.KindInvalid); len(e.Details) != 3 {
		t.Errorf("empty input: %v", e.Details)
	}
	if list, _ := a.ListServices(ctx); len(list) != 0 {
		t.Errorf("invalid services were stored: %+v", list)
	}
	if n := len(logs.All()); n != 0 {
		t.Errorf("failed creates were logged: %+v", logs.All())
	}
}

func TestServiceCRUD(t *testing.T) {
	t.Parallel()
	a, _, logs := newApp(t)
	ctx := context.Background()

	if list, err := a.ListServices(ctx); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty list must be [] not nil: %#v, %v", list, err)
	}

	// Input is trimmed; the scheme may be upper-case; base paths and ports are fine.
	s, err := a.CreateService(ctx, ann, app.ServiceInput{Path: " svc-1.v2_x ", Name: " Svc ", UpstreamURL: " HTTPS://example.com:8443/base "})
	want := app.Service{Path: "svc-1.v2_x", Name: "Svc", UpstreamURL: "HTTPS://example.com:8443/base"}
	if err != nil || s != want {
		t.Fatalf("create: %+v, %v", s, err)
	}
	if got, err := a.GetService(ctx, "svc-1.v2_x"); err != nil || got != want {
		t.Errorf("get: %+v, %v", got, err)
	}

	// Duplicate path.
	_, err = a.CreateService(ctx, ann, app.ServiceInput{Path: "svc-1.v2_x", Name: "Other", UpstreamURL: "http://o"})
	if e := appErr(t, err, app.KindConflict); e.Message != `A service with path "svc-1.v2_x" already exists` || e.StatusCode() != 409 {
		t.Errorf("duplicate: %v", e)
	}

	// An empty patch changes nothing; absent fields are kept.
	if got, err := a.UpdateService(ctx, ann, "svc-1.v2_x", app.ServicePatch{}); err != nil || got != want {
		t.Errorf("empty patch: %+v, %v", got, err)
	}
	got, err := a.UpdateService(ctx, ann, "svc-1.v2_x", app.ServicePatch{Path: httpx.Some("svc")})
	if err != nil || got != (app.Service{Path: "svc", Name: "Svc", UpstreamURL: "HTTPS://example.com:8443/base"}) {
		t.Errorf("rename: %+v, %v", got, err)
	}

	// null on a required field, and an invalid value, are 400.
	_, err = a.UpdateService(ctx, ann, "svc", app.ServicePatch{Name: httpx.Null[string](), UpstreamURL: httpx.Some("nope")})
	if e := appErr(t, err, app.KindInvalid); e.Details["name"] != "is required" {
		t.Errorf("null name: %v", e.Details)
	}
	_, err = a.UpdateService(ctx, ann, "svc", app.ServicePatch{Path: httpx.Some("a/b")})
	appErr(t, err, app.KindInvalid)

	// Renaming onto a taken path is a conflict; an unknown service is not found.
	if _, err := a.CreateService(ctx, ann, app.ServiceInput{Path: "other", Name: "Other", UpstreamURL: "http://o"}); err != nil {
		t.Fatal(err)
	}
	_, err = a.UpdateService(ctx, ann, "other", app.ServicePatch{Path: httpx.Some("svc")})
	appErr(t, err, app.KindConflict)
	_, err = a.UpdateService(ctx, ann, "missing", app.ServicePatch{Name: httpx.Some("x")})
	if e := appErr(t, err, app.KindNotFound); e.Message != `Service "missing" not found` {
		t.Errorf("update missing: %v", e)
	}
	_, err = a.GetService(ctx, "missing")
	appErr(t, err, app.KindNotFound)

	// Ordered by name.
	list, _ := a.ListServices(ctx)
	if len(list) != 2 || list[0].Path != "other" || list[1].Path != "svc" {
		t.Errorf("list: %+v", list)
	}

	deleted, err := a.DeleteService(ctx, ann, "svc")
	if err != nil || deleted.Name != "Svc" {
		t.Errorf("delete: %+v, %v", deleted, err)
	}
	_, err = a.DeleteService(ctx, ann, "svc")
	appErr(t, err, app.KindNotFound)

	// One tmock.entity.changed per successful change (spec §2.4.2).
	events := logs.Events(logx.EventEntityChanged)
	var actions []string
	for _, e := range events {
		actions = append(actions, e.Attrs[logx.AttrEntityAction]+":"+e.Attrs[logx.AttrService])
	}
	wantActions := "create:svc-1.v2_x update:svc-1.v2_x update:svc create:other delete:svc"
	if strings.Join(actions, " ") != wantActions {
		t.Errorf("events %v, want %s", actions, wantActions)
	}
}

// The tmock.entity.changed event: name, level and attributes (spec §15.1).
func TestServiceChangedEvent(t *testing.T) {
	t.Parallel()
	a, _, logs := newApp(t)
	if _, err := a.CreateService(context.Background(), ann, app.ServiceInput{Path: "svc", Name: "Svc", UpstreamURL: "http://u"}); err != nil {
		t.Fatal(err)
	}
	events := logs.Events(logx.EventEntityChanged)
	if len(events) != 1 {
		t.Fatalf("events: %+v", logs.All())
	}
	e := events[0]
	if e.Level != slog.LevelInfo {
		t.Errorf("level %v", e.Level)
	}
	for k, v := range map[string]string{
		logx.AttrEntityType:   "service",
		logx.AttrEntityAction: "create",
		logx.AttrActor:        "user",
		logx.AttrUserID:       "7",
		logx.AttrService:      "svc",
	} {
		if e.Attrs[k] != v {
			t.Errorf("%s = %q, want %q", k, e.Attrs[k], v)
		}
	}
	if id := e.Attrs[logx.AttrEntityID]; id == "" || id == "0" {
		t.Errorf("entity id %q", id)
	}
}
