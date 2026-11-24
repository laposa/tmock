// Package static embeds the UI's CSS, JavaScript and images, including the vendored
// JavaScript libraries listed in vendor/VERSIONS.
package static

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// FS holds every file served under /static/.
//
//go:embed app.css app.js logo.svg favicon.ico vendor
var FS embed.FS

// Paths of the vendored files, relative to FS. The layout links them in this order
// (spec §10.1). Keep them in sync with vendor/VERSIONS; the package test checks it.
const (
	HTMX             = "vendor/htmx-4.0.2.min.js"
	HTMXAlpineCompat = "vendor/hx-alpine-compat-4.0.2.js"
	Alpine           = "vendor/alpinejs-csp-3.17.5.min.js"
)

// VendorEntry is one line of vendor/VERSIONS.
type VendorEntry struct {
	File    string // file name inside vendor/
	Version string
	SHA256  string // lower-case hex, or "-" when not pinned yet
	URL     string
}

var (
	sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)
	// A plain file name. No paths, and no leading dot: go:embed skips dot-files.
	fileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// ParseVersions reads the vendor/VERSIONS format: one entry per line with the fields
// file, version, sha256 and url separated by whitespace. Blank lines and lines starting
// with '#' are ignored. A sha256 of "-" means "not pinned yet" (make vendor-js fills it in).
func ParseVersions(r io.Reader) ([]VendorEntry, error) {
	var entries []VendorEntry
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 4 {
			return nil, fmt.Errorf("VERSIONS line %d: want 4 fields (file version sha256 url), got %d", n, len(f))
		}
		e := VendorEntry{File: f[0], Version: f[1], SHA256: f[2], URL: f[3]}
		switch {
		case !fileName.MatchString(e.File) || e.File == "VERSIONS":
			return nil, fmt.Errorf("VERSIONS line %d: invalid file name %q", n, e.File)
		case e.SHA256 != "-" && !sha256Hex.MatchString(e.SHA256):
			return nil, fmt.Errorf("VERSIONS line %d: sha256 must be 64 lower-case hex digits or \"-\"", n)
		case !strings.HasPrefix(e.URL, "https://"):
			return nil, fmt.Errorf("VERSIONS line %d: url must use https", n)
		case seen[e.File]:
			return nil, fmt.Errorf("VERSIONS line %d: duplicate file %q", n, e.File)
		}
		seen[e.File] = true
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
