package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/static"
)

func get(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	res := rec.Result()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, string(b)
}

func TestComingSoonPage(t *testing.T) {
	res, body := get(t, Handler(), "/")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type %q", ct)
	}
	for _, want := range []string{
		"<!doctype html>",
		"<title>Coming soon · Tmock</title>",
		"Tmock — coming soon",
		`<meta name="htmx-config" content="{&#34;includeIndicatorCSS&#34;:false,&#34;history&#34;:false}">`,
		`x-data="jsStatus"`,
		`hx-get="/api/status"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	assertNoEval(t, body)
}

// Scripts load in the order spec §10.1 requires, all deferred.
func TestScriptOrder(t *testing.T) {
	_, body := get(t, Handler(), "/")
	re := regexp.MustCompile(`<script src="([^"]+)" defer></script>`)
	var got []string
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		got = append(got, m[1])
	}
	want := []string{
		"/static/" + static.HTMX,
		"/static/" + static.HTMXAlpineCompat,
		"/static/app.js",
		"/static/" + static.Alpine,
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("scripts\n got %v\nwant %v", got, want)
	}
}

func TestStaticFiles(t *testing.T) {
	h := Handler()
	cases := []struct {
		path, wantType, wantCache string
		wantStatus                int
	}{
		{"/static/app.css", "text/css; charset=utf-8", "no-cache", 200},
		{"/static/app.js", "text/javascript; charset=utf-8", "no-cache", 200},
		{"/static/logo.svg", "image/svg+xml", "no-cache", 200},
		{"/static/" + static.HTMX, "text/javascript; charset=utf-8", "public, max-age=31536000, immutable", 200},
		{"/static/" + static.Alpine, "text/javascript; charset=utf-8", "public, max-age=31536000, immutable", 200},
		{"/static/vendor/VERSIONS", "", "", 404},
		{"/static/", "", "", 404},
		{"/static/vendor/", "", "", 404},
		{"/static/missing.js", "", "", 404},
		{"/elsewhere", "", "", 404},
	}
	for _, tc := range cases {
		res, _ := get(t, h, tc.path)
		if res.StatusCode != tc.wantStatus {
			t.Errorf("%s: status %d, want %d", tc.path, res.StatusCode, tc.wantStatus)
			continue
		}
		if tc.wantStatus != 200 {
			continue
		}
		if ct := res.Header.Get("Content-Type"); ct != tc.wantType {
			t.Errorf("%s: Content-Type %q, want %q", tc.path, ct, tc.wantType)
		}
		if cc := res.Header.Get("Cache-Control"); cc != tc.wantCache {
			t.Errorf("%s: Cache-Control %q, want %q", tc.path, cc, tc.wantCache)
		}
	}
}

func TestStaticETag(t *testing.T) {
	h := Handler()
	res, _ := get(t, h, "/static/app.css")
	etag := res.Header.Get("ETag")
	if len(etag) != 34 || etag[0] != '"' {
		t.Fatalf("ETag %q", etag)
	}
	req := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("conditional GET: status %d, %d body bytes; want 304 and no body", rec.Code, rec.Body.Len())
	}

	if res, _ := get(t, h, "/static/vendor/VERSIONS"); res.Header.Get("ETag") != "" {
		t.Error("404 response carries an ETag")
	}
}

func TestPostNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /: status %d", rec.Code)
	}
}

var (
	scriptTag     = regexp.MustCompile(`<script\b[^>]*>`)
	evalAttribute = regexp.MustCompile(`\s(?:data-)?hx-on[:\-]|="\s*(?:js|javascript):|\shx-trigger="[^"]*\[`)
)

// evalProblems enforces spec §10.1: no inline scripts and none of the htmx features that
// need eval (hx-on, js:/javascript: values, trigger filters), so CSP can stay strict.
func evalProblems(html string) []string {
	var problems []string
	for _, tag := range scriptTag.FindAllString(html, -1) {
		if !strings.Contains(tag, " src=") {
			problems = append(problems, "inline script: "+tag)
		}
	}
	for _, m := range evalAttribute.FindAllString(html, -1) {
		problems = append(problems, "eval-based htmx feature: "+strings.TrimSpace(m))
	}
	return problems
}

func assertNoEval(t *testing.T, html string) {
	t.Helper()
	for _, p := range evalProblems(html) {
		t.Error(p)
	}
}

func TestEvalProblemsCatchesViolations(t *testing.T) {
	for _, html := range []string{
		`<script>alert(1)</script>`,
		`<button hx-on:click="x()">`,
		`<div data-hx-on-click="x()">`,
		`<div hx-vals="js:{a: 1}">`,
		`<div hx-trigger="keyup[key=='Enter']">`,
	} {
		if len(evalProblems(html)) == 0 {
			t.Errorf("not caught: %s", html)
		}
	}
	if p := evalProblems(`<script src="/static/app.js" defer></script><button hx-get="/x">`); len(p) != 0 {
		t.Errorf("false positive: %v", p)
	}
}
