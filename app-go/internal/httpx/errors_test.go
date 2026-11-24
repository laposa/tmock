package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
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
