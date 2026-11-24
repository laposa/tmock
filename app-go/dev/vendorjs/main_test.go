package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// fakeCDN serves files over TLS, because VERSIONS only accepts https URLs.
func fakeCDN(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setup(t *testing.T, versions string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSIONS"), []byte(versions), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDownloadsVerifiedFiles(t *testing.T) {
	srv := fakeCDN(t, map[string]string{"/a.js": "alpha", "/b.js": "beta"})
	dir := setup(t, "# header\na.js 1.0 "+sum("alpha")+" "+srv.URL+"/a.js\nb.js 2.0 "+sum("beta")+" "+srv.URL+"/b.js\n")
	if err := os.WriteFile(filepath.Join(dir, "old.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := run(context.Background(), srv.Client(), dir, &out); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dir, "a.js")) != "alpha" || read(t, filepath.Join(dir, "b.js")) != "beta" {
		t.Error("files not written")
	}
	if !strings.Contains(out.String(), "old.js is not listed") {
		t.Errorf("stale file not reported: %s", out.String())
	}
}

func TestChecksumMismatchWritesNothing(t *testing.T) {
	srv := fakeCDN(t, map[string]string{"/a.js": "alpha", "/b.js": "tampered"})
	dir := setup(t, "a.js 1.0 "+sum("alpha")+" "+srv.URL+"/a.js\nb.js 2.0 "+sum("beta")+" "+srv.URL+"/b.js\n")

	err := run(context.Background(), srv.Client(), dir, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "b.js: checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.js")); !os.IsNotExist(err) {
		t.Error("a.js was written although b.js failed")
	}
}

func TestPinsNewChecksum(t *testing.T) {
	srv := fakeCDN(t, map[string]string{"/a.js": "alpha"})
	dir := setup(t, "# keep me\na.js   1.0   -   "+srv.URL+"/a.js\n")

	if err := run(context.Background(), srv.Client(), dir, io.Discard); err != nil {
		t.Fatal(err)
	}
	want := "# keep me\na.js   1.0   " + sum("alpha") + "   " + srv.URL + "/a.js\n"
	if got := read(t, filepath.Join(dir, "VERSIONS")); got != want {
		t.Errorf("VERSIONS\n got %q\nwant %q", got, want)
	}
}

func TestDownloadErrors(t *testing.T) {
	srv := fakeCDN(t, nil)
	dir := setup(t, "a.js 1.0 - "+srv.URL+"/missing.js\n")
	if err := run(context.Background(), srv.Client(), dir, io.Discard); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v", err)
	}
}
