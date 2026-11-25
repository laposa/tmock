package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/logx"
)

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "Validation failed", map[string]string{"email": "must be an email address"})
	if rec.Code != 400 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d, headers %v", rec.Code, rec.Header())
	}
	want := `{"status":400,"message":"Validation failed","details":{"email":"must be an email address"}}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	WriteError(rec, http.StatusNotFound, "Not found", nil)
	if got := rec.Body.String(); got != `{"status":404,"message":"Not found"}`+"\n" {
		t.Errorf("body without details %q", got)
	}
}

func TestRecover(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := Recover(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(errors.New("boom"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/x", nil))
	if rec.Code != 500 || rec.Body.String() != `{"status":500,"message":"Internal server error"}`+"\n" {
		t.Errorf("response %d %q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Error("panic details leaked into the response")
	}

	var rec1 map[string]any
	if err := json.Unmarshal(logs.Bytes(), &rec1); err != nil {
		t.Fatalf("log %q: %v", logs.String(), err)
	}
	if rec1["level"] != "ERROR" || rec1[logx.AttrEventName] != logx.EventHTTPPanic ||
		rec1[logx.AttrExceptionType] != "*errors.errorString" || rec1[logx.AttrExceptionMessage] != "boom" ||
		rec1[logx.AttrHTTPMethod] != "GET" || rec1[logx.AttrURLPath] != "/api/x" ||
		!strings.Contains(rec1[logx.AttrExceptionStacktrace].(string), "TestRecover") {
		t.Errorf("log record %v", rec1)
	}
}

func TestRecoverAfterResponseStarted(t *testing.T) {
	h := Recover(slog.New(slog.DiscardHandler), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("late")
	}))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	t.Error("no panic")
}

func TestRecoverPassesAbortThrough(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(slog.New(slog.NewTextHandler(&logs, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recovered %v", v)
		}
		if logs.Len() != 0 {
			t.Errorf("deliberate abort was logged: %s", logs.String())
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

// A panic must not leave the handler's own headers on the 500: a stale Content-Length
// would truncate it, and an upstream's Set-Cookie must not leak onto it.
func TestRecoverDropsHandlerHeaders(t *testing.T) {
	srv := httptest.NewServer(Recover(slog.New(slog.DiscardHandler), SecurityHeaders(false, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "5000")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Set-Cookie", "upstream=1")
		panic("boom")
	}))))
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != 500 || string(body) != `{"status":500,"message":"Internal server error"}`+"\n" {
		t.Fatalf("%d %q %v", res.StatusCode, body, err)
	}
	if res.Header.Get("Set-Cookie") != "" || res.Header.Get("Content-Encoding") != "" {
		t.Errorf("handler headers kept: %v", res.Header)
	}
	if res.Header.Get("Content-Security-Policy") != CSP {
		t.Errorf("security headers dropped: %v", res.Header)
	}
}
