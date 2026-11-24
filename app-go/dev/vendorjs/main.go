// Command vendorjs re-downloads the vendored JavaScript listed in static/vendor/VERSIONS
// and verifies each file against its pinned SHA-256 (run it with `make vendor-js`).
//
// A file is only written when its checksum matches. An entry whose checksum is "-" (a new
// or upgraded library) is downloaded and its checksum is written into VERSIONS; check that
// value against the publisher before committing it.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/laposa/tmock/app-go/static"
)

const maxSize = 5 << 20 // vendored files are well below this

// unpinned matches the "-" checksum field between its surrounding whitespace.
var unpinned = regexp.MustCompile(`(\s)-(\s)`)

func main() {
	dir := flag.String("dir", "static/vendor", "directory that holds VERSIONS and the vendored files")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := run(ctx, http.DefaultClient, *dir, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "vendorjs: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, client *http.Client, dir string, out io.Writer) error {
	versionsPath := filepath.Join(dir, "VERSIONS")
	raw, err := os.ReadFile(versionsPath)
	if err != nil {
		return err
	}
	entries, err := static.ParseVersions(bytes.NewReader(raw))
	if err != nil {
		return err
	}

	// Download and verify everything first, so a single bad file changes nothing.
	contents := make([][]byte, len(entries))
	pinned := map[string]string{}
	for i, e := range entries {
		b, err := download(ctx, client, e.URL)
		if err != nil {
			return fmt.Errorf("%s: %w", e.File, err)
		}
		sum := sha256.Sum256(b)
		got := hex.EncodeToString(sum[:])
		switch e.SHA256 {
		case "-":
			pinned[e.File] = got
		case got:
		default:
			return fmt.Errorf("%s: checksum mismatch: downloaded %s, VERSIONS pins %s (nothing was written)", e.File, got, e.SHA256)
		}
		contents[i] = b
	}

	for i, e := range entries {
		if err := writeAtomic(filepath.Join(dir, e.File), contents[i]); err != nil {
			return err
		}
		fmt.Fprintf(out, "ok  %-30s %s\n", e.File, e.Version)
	}
	if len(pinned) > 0 {
		if err := writeAtomic(versionsPath, pin(raw, pinned)); err != nil {
			return err
		}
		for _, f := range slices.Sorted(maps.Keys(pinned)) {
			fmt.Fprintf(out, "pinned %s sha256 %s — verify it against the publisher before committing\n", f, pinned[f])
		}
	}

	// Files that are no longer listed are reported, not deleted.
	listed := map[string]bool{"VERSIONS": true}
	for _, e := range entries {
		listed[e.File] = true
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if !listed[f.Name()] {
			fmt.Fprintf(out, "note: %s is not listed in VERSIONS; delete it if it is no longer used\n", f.Name())
		}
	}
	return nil
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSize {
		return nil, errors.New("file is larger than 5 MB")
	}
	return b, nil
}

// pin replaces the "-" checksum of the given files in the VERSIONS text, keeping the rest
// of each line (and every comment) as it is.
func pin(raw []byte, sums map[string]string) []byte {
	lines := strings.SplitAfter(string(raw), "\n")
	for i, line := range lines {
		f := strings.Fields(line)
		if len(f) == 4 && f[2] == "-" && sums[f[0]] != "" {
			lines[i] = unpinned.ReplaceAllString(line, "${1}"+sums[f[0]]+"${2}")
		}
	}
	return []byte(strings.Join(lines, ""))
}

func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vendorjs-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
