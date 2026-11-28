package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/store"
)

// Spec §7.7: the dashboard's session cookie never reaches an upstream. Any user can point
// a service at a server of their own, and a browser sends the cookie on proxy requests
// (same origin, Path=/), so forwarding it would hand that user the session of whoever
// follows a link to the service. The client's other cookies pass unchanged.
func TestSessionCookieNotForwarded(t *testing.T) {
	up, ch := echoUpstream(t)
	p := New(Options{Catalog: services("svc", up.URL), Logger: slog.New(slog.DiscardHandler)})

	do(p, "GET", "/api/proxy/svc/x", nil, "Cookie", "app=1; "+httpx.SessionCookieName+"=SECRET; theme=dark")
	if got := (<-ch).Header.Values("Cookie"); !slices.Equal(got, []string{"app=1; theme=dark"}) {
		t.Errorf("upstream got Cookie %q", got)
	}
	do(p, "GET", "/api/proxy/svc/x", nil, "Cookie", httpx.SessionCookieName+"=SECRET")
	if got := (<-ch).Header.Values("Cookie"); got != nil {
		t.Errorf("upstream got Cookie %q", got)
	}
}

// Spec §7.8: neither an upstream nor a scenario can set or clear the dashboard's session
// cookie; their other cookies pass.
func TestSessionCookieNotSet(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", httpx.SessionCookieName+"=fixed; Path=/")
		w.Header().Add("Set-Cookie", "app=1; Path=/")
		_, _ = w.Write([]byte("ok"))
	}))
	defer up.Close()
	setSession := map[string]*string{"Set-Cookie": sp(httpx.SessionCookieName + "=; Max-Age=0; Path=/")}
	p := New(Options{Catalog: mockCatalog(up.URL,
		store.Scenario{Name: "a-skip", RequestPath: sp("skip"), SkipProxy: true, ResponseBody: sp("{}"), ResponseHeaders: setSession},
		store.Scenario{Name: "b-modify", RequestPath: sp("modify"), ResponseHeaders: setSession},
	), Logger: slog.New(slog.DiscardHandler)})

	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/api/proxy/svc/passthrough", []string{"app=1; Path=/"}}, // no client matches
		{"/api/proxy/svc/skip", nil},
		{"/api/proxy/svc/modify", nil}, // the scenario's Set-Cookie replaced the upstream's
	} {
		ip := "10.9.9.9"
		if tc.path != "/api/proxy/svc/passthrough" {
			ip = "10.0.0.1" // client qa
		}
		rec := do(p, "GET", tc.path, nil, "X-Real-Ip", ip)
		if got := rec.Header().Values("Set-Cookie"); !slices.Equal(got, tc.want) {
			t.Errorf("%s: Set-Cookie %q, want %q", tc.path, got, tc.want)
		}
	}
}
