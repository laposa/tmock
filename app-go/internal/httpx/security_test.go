package httpx

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })

// Verify item: the headers of spec §9.4.
func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"Content-Security-Policy":      "default-src 'self'; base-uri 'self'; font-src 'self' data:; form-action 'self'; frame-ancestors 'self'; img-src 'self' data:; object-src 'none'; script-src 'self'; script-src-attr 'none'; style-src 'self'",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Referrer-Policy":              "no-referrer",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "SAMEORIGIN",
		"Strict-Transport-Security":    "",
	}
	rec := httptest.NewRecorder()
	SecurityHeaders(false, ok).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("development %s = %q, want %q", k, got, v)
		}
	}

	rec = httptest.NewRecorder()
	SecurityHeaders(true, ok).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := rec.Header().Get("Content-Security-Policy"); got != want["Content-Security-Policy"]+"; upgrade-insecure-requests" {
		t.Errorf("production CSP %q", got)
	}
	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains" {
		t.Errorf("production HSTS %q", got)
	}

	// A handler may replace a header (the swagger page's relaxed CSP).
	relaxed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self' https://cdn.example")
	})
	rec = httptest.NewRecorder()
	SecurityHeaders(false, relaxed).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'self' https://cdn.example" {
		t.Errorf("override %q", got)
	}
}

// Verify item: a cross-origin browser POST is rejected, while curl passes.
func TestCrossOrigin(t *testing.T) {
	t.Parallel()
	h := CrossOrigin(ok)
	cases := []struct {
		name    string
		method  string
		headers map[string]string
		want    int
	}{
		{"curl POST", "POST", nil, 200},
		{"same-origin browser POST", "POST", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://tmock.test"}, 200},
		{"user-initiated navigation POST", "POST", map[string]string{"Sec-Fetch-Site": "none"}, 200},
		{"cross-site browser POST", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, 403},
		{"same-site (other subdomain) PATCH", "PATCH", map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://other.tmock.test"}, 403},
		{"old browser, foreign Origin", "DELETE", map[string]string{"Origin": "https://evil.example"}, 403},
		{"old browser, matching Origin", "POST", map[string]string{"Origin": "http://tmock.test"}, 200},
		{"cross-site GET", "GET", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, 200},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, "http://tmock.test/api/service", nil)
		for k, v := range tc.headers {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, rec.Code, tc.want)
		}
		if tc.want == 403 && (rec.Header().Get("Content-Type") != "application/json" ||
			rec.Body.String() != `{"status":403,"message":"Cross-origin request rejected"}`+"\n") {
			t.Errorf("%s: body %q", tc.name, rec.Body.String())
		}
	}
}

func proxyRequest(method, origin string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(method, "/api/proxy/svc/x", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestProxyCORSReflects(t *testing.T) {
	t.Parallel()
	h := ProxyCORS(nil, ok)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, proxyRequest("GET", "https://app.example", nil))
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" ||
		!slices.Contains(rec.Header().Values("Vary"), "Origin") || rec.Body.String() != "ok" {
		t.Errorf("headers %v, body %q", rec.Header(), rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, proxyRequest("GET", "", nil))
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("no Origin, but %v", rec.Header())
	}
}

func TestProxyCORSAllowList(t *testing.T) {
	t.Parallel()
	h := ProxyCORS([]string{"https://app.example", "http://localhost:5173"}, ok)
	for origin, allowed := range map[string]bool{
		"https://app.example":   true,
		"HTTPS://APP.EXAMPLE":   true,
		"http://localhost:5173": true,
		"https://evil.example":  false,
		"http://app.example":    false,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, proxyRequest("POST", origin, nil))
		got := rec.Header().Get("Access-Control-Allow-Origin")
		if (got == origin) != allowed || (!allowed && got != "") {
			t.Errorf("%s: Allow-Origin %q", origin, got)
		}
		if rec.Body.String() != "ok" {
			t.Errorf("%s: the request must still be served", origin)
		}
		if !slices.Contains(rec.Header().Values("Vary"), "Origin") {
			t.Errorf("%s: no Vary: Origin", origin)
		}
	}
}

func TestProxyCORSPreflight(t *testing.T) {
	t.Parallel()
	called := false
	h := ProxyCORS([]string{"https://app.example"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, proxyRequest("OPTIONS", "https://app.example", map[string]string{
		"Access-Control-Request-Method":  "PATCH",
		"Access-Control-Request-Headers": "content-type, x-client-token",
	}))
	hd := rec.Header()
	if rec.Code != 204 || hd.Get("Access-Control-Allow-Origin") != "https://app.example" ||
		hd.Get("Access-Control-Allow-Methods") != "GET,HEAD,PUT,PATCH,POST,DELETE" ||
		hd.Get("Access-Control-Allow-Headers") != "content-type, x-client-token" ||
		hd.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("allowed preflight: %d %v", rec.Code, hd)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, proxyRequest("OPTIONS", "https://evil.example", map[string]string{"Access-Control-Request-Method": "POST"}))
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("refused preflight: %d %v", rec.Code, rec.Header())
	}
	if called {
		t.Error("a preflight was forwarded")
	}

	// A plain OPTIONS (not a preflight) is forwarded.
	h.ServeHTTP(httptest.NewRecorder(), proxyRequest("OPTIONS", "https://app.example", nil))
	if !called {
		t.Error("a plain OPTIONS was not forwarded")
	}
}

func TestProxyCORSUpstreamWins(t *testing.T) {
	t.Parallel()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Accept-Encoding, origin")
		w.WriteHeader(http.StatusTeapot)
	})
	rec := httptest.NewRecorder()
	ProxyCORS(nil, upstream).ServeHTTP(rec, proxyRequest("GET", "https://app.example", nil))
	if rec.Code != http.StatusTeapot || rec.Header().Get("Access-Control-Allow-Origin") != "*" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("%d %v", rec.Code, rec.Header())
	}
	if v := rec.Header().Values("Vary"); len(v) != 1 {
		t.Errorf("Vary duplicated: %v", v)
	}
}

func TestJSONFallback(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.Handle("GET /api/things/{id}", ok)
	mux.Handle("POST /api/things", ok)
	mux.Handle("/api/dir/", ok)
	h := JSONFallback(mux)

	cases := []struct {
		method, path string
		status       int
		body, allow  string
	}{
		{"GET", "/api/things/1", 200, "ok", ""},
		{"GET", "/api/nothing", 404, `{"status":404,"message":"Not found"}` + "\n", ""},
		{"DELETE", "/api/things/1", 405, `{"status":405,"message":"Method not allowed"}` + "\n", "GET, HEAD"},
		{"GET", "/api/things", 405, `{"status":405,"message":"Method not allowed"}` + "\n", "POST"},
		{"GET", "/api/dir", 0, "", ""}, // a redirect to /api/dir/ (301 or 307, by Go version)
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if tc.status == 0 {
			if rec.Code < 300 || rec.Code > 399 || rec.Header().Get("Location") != "/api/dir/" {
				t.Errorf("%s %s: %d Location=%q", tc.method, tc.path, rec.Code, rec.Header().Get("Location"))
			}
			continue
		}
		if rec.Code != tc.status || (tc.body != "" && rec.Body.String() != tc.body) || rec.Header().Get("Allow") != tc.allow {
			t.Errorf("%s %s: %d %q Allow=%q", tc.method, tc.path, rec.Code, rec.Body.String(), rec.Header().Get("Allow"))
		}
		if tc.status >= 400 && !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s %s: Content-Type %q", tc.method, tc.path, rec.Header().Get("Content-Type"))
		}
	}
}
