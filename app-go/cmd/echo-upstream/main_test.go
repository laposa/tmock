package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestEcho(t *testing.T) {
	var log bytes.Buffer
	h := Handler(Options{}, &log)
	r := httptest.NewRequest("POST", "/a%2Fb/c?x=1&y", strings.NewReader(`{"k":1}`))
	r.Header.Set("X-Test", "v")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	var e Echo
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || e.Method != "POST" || e.URL != "/a%2Fb/c?x=1&y" || e.Path != "/a%2Fb/c" || e.Query != "x=1&y" ||
		e.Body != `{"k":1}` || e.Size != 7 || e.Headers["X-Test"][0] != "v" {
		t.Errorf("echo = %d %+v", rec.Code, e)
	}
	if !strings.Contains(log.String(), "POST /a%2Fb/c?x=1&y 7 bytes") {
		t.Errorf("log = %q", log.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("PUT", "/", bytes.NewReader([]byte{0xff, 0xfe})))
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Body != "" || !bytes.Equal(e.BodyB64, []byte{0xff, 0xfe}) {
		t.Errorf("binary body: %+v %v", e, err)
	}
}

func TestStatusAndGzip(t *testing.T) {
	h := Handler(Options{Status: 503, Gzip: true}, nil)
	r := httptest.NewRequest("GET", "/z", nil)
	r.Header.Set("Accept-Encoding", "br, gzip;q=0.8")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 503 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("got %d %v", rec.Code, rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	if !strings.Contains(string(body), `"path": "/z"`) {
		t.Errorf("body = %s", body)
	}

	// Not compressed for a client that does not accept gzip.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/z", nil))
	if rec.Header().Get("Content-Encoding") != "" || !strings.Contains(rec.Body.String(), `"path": "/z"`) {
		t.Errorf("uncompressed: %v %s", rec.Header(), rec.Body)
	}
}

func TestDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := Handler(Options{Delay: 2 * time.Second}, nil)
		start := time.Now()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if d := time.Since(start); d != 2*time.Second || rec.Code != http.StatusOK {
			t.Errorf("answered after %s with %d", d, rec.Code)
		}
	})
}

func TestNoBodyStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(Options{Status: 204}, nil).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 204 || rec.Body.Len() != 0 {
		t.Errorf("got %d with %d bytes", rec.Code, rec.Body.Len())
	}
}
