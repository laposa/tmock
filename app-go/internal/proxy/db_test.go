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
