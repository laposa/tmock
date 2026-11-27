package proxy_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/condition"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/proxy"
	"github.com/laposa/tmock/app-go/internal/store"
)

func upstream(t *testing.T, name string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, name+" "+r.URL.RequestURI())
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Spec §7.2, §15.3 I09: with the real store, app and cache, a change made through the
// use cases is visible on the next proxy request, long before the TTL.
func TestMutationVisibleOnNextRequest(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	_, logger := logxtest.New()
	cat := catalog.NewCache(store.NewCatalog(pool), catalog.Options{TTL: time.Hour, Logger: logger,
		Overrides: map[string]string{"overridden": upstream(t, "override").URL}})
	a := app.New(pool, app.Options{Logger: logger, Catalog: cat})
	p := proxy.New(proxy.Options{Catalog: cat, Logger: logger})
	ctx := context.Background()
	actor := app.UserActor(1)
	get := func(target string) (int, string) {
		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		return rec.Code, rec.Body.String()
	}

	if code, _ := get("/api/proxy/svc/x"); code != http.StatusNotFound {
		t.Fatalf("before create: %d", code)
	}
	one, two := upstream(t, "one"), upstream(t, "two")
	if _, err := a.CreateService(ctx, actor, app.ServiceInput{Path: "svc", Name: "Svc", UpstreamURL: one.URL}); err != nil {
		t.Fatal(err)
	}
	if code, body := get("/api/proxy/svc/x?y=1"); code != 200 || body != "one /x?y=1" {
		t.Fatalf("after create: %d %q", code, body)
	}
	if _, err := a.UpdateService(ctx, actor, "svc", app.ServicePatch{UpstreamURL: httpx.Some(two.URL + "/base")}); err != nil {
		t.Fatal(err)
	}
	if code, body := get("/api/proxy/svc/x"); code != 200 || body != "two /base/x" {
		t.Fatalf("after update: %d %q", code, body)
	}
	if _, err := a.UpdateService(ctx, actor, "svc", app.ServicePatch{Path: httpx.Some("renamed")}); err != nil {
		t.Fatal(err)
	}
	if code, _ := get("/api/proxy/svc/x"); code != http.StatusNotFound {
		t.Errorf("old path after rename: %d", code)
	}
	if code, _ := get("/api/proxy/renamed/x"); code != 200 {
		t.Errorf("new path after rename: %d", code)
	}

	// SERVICES_UPSTREAM_OVERRIDES wins over the stored URL.
	if _, err := a.CreateService(ctx, actor, app.ServiceInput{Path: "overridden", Name: "O", UpstreamURL: "http://unreachable.invalid"}); err != nil {
		t.Fatal(err)
	}
	if code, body := get("/api/proxy/overridden/a"); code != 200 || body != "override /a" {
		t.Errorf("override: %d %q", code, body)
	}

	if _, err := a.DeleteService(ctx, actor, "renamed"); err != nil {
		t.Fatal(err)
	}
	if code, _ := get("/api/proxy/renamed/x"); code != http.StatusNotFound {
		t.Errorf("after delete: %d", code)
	}
}

// Spec §15.3 I10 manual check, automated: a client with condition ip = <the caller> and a
// 500-body scenario mocks; with skip proxy the upstream sees nothing; disabling the client
// is visible on the next request.
func TestMockingThroughUseCases(t *testing.T) {
	t.Parallel()
	pool := dbtest.Migrated(t)
	_, logger := logxtest.New()
	cat := catalog.NewCache(store.NewCatalog(pool), catalog.Options{TTL: time.Hour, Logger: logger})
	a := app.New(pool, app.Options{Logger: logger, Catalog: cat})
	p := proxy.New(proxy.Options{Catalog: cat, Logger: logger})
	ctx := context.Background()
	actor := app.UserActor(1)
	var calls int
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.WriteString(w, "upstream")
	}))
	defer up.Close()
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/proxy/svc/orders", nil)
		r.RemoteAddr = "127.0.0.1:5000"
		p.ServeHTTP(rec, r)
		return rec
	}

	if _, err := a.CreateService(ctx, actor, app.ServiceInput{Path: "svc", Name: "Svc", UpstreamURL: up.URL}); err != nil {
		t.Fatal(err)
	}
	code, body := 500, `{"error":"boom"}`
	sc, err := a.CreateScenario(ctx, actor, app.ScenarioInput{Service: "svc", Name: "boom", ResponseCode: &code, ResponseBody: &body})
	if err != nil {
		t.Fatal(err)
	}
	ip := "127.0.0.1"
	cl, err := a.CreateClient(ctx, actor, app.ClientInput{Name: "local", Enabled: true,
		Condition: &condition.Condition{IP: &ip}, Scenarios: []int64{sc.ID}})
	if err != nil {
		t.Fatal(err)
	}

	rec := get()
	if rec.Code != 500 || rec.Body.String() != body || rec.Header().Get("X-Tmock-Status") != "mocked" ||
		rec.Header().Get("X-Tmock-Client") != "local" || rec.Header().Get("X-Tmock-Scenario") != "boom" || calls != 1 {
		t.Fatalf("mocked: %d %q %v (upstream calls %d)", rec.Code, rec.Body, rec.Header(), calls)
	}

	if _, err := a.UpdateScenario(ctx, actor, sc.ID, app.ScenarioPatch{SkipProxy: httpx.Some(true)}); err != nil {
		t.Fatal(err)
	}
	if rec := get(); rec.Code != 500 || calls != 1 {
		t.Fatalf("skip proxy: %d, upstream calls %d", rec.Code, calls)
	}

	if _, err := a.UpdateClient(ctx, actor, cl.ID, app.ClientPatch{Enabled: httpx.Some(false)}); err != nil {
		t.Fatal(err)
	}
	if rec := get(); rec.Code != 200 || rec.Body.String() != "upstream" || rec.Header().Get("X-Tmock-Status") != "proxied" {
		t.Fatalf("after disabling: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
}
