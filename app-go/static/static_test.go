package static

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path"
	"strings"
	"testing"
)

func vendorEntries(t *testing.T) []VendorEntry {
	t.Helper()
	f, err := FS.Open("vendor/VERSIONS")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := ParseVersions(f)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// The committed vendor files must match the checksums pinned in VERSIONS.
func TestVendorChecksums(t *testing.T) {
	for _, e := range vendorEntries(t) {
		if e.SHA256 == "-" {
			t.Errorf("%s: checksum not pinned; run make vendor-js", e.File)
			continue
		}
		b, err := FS.ReadFile("vendor/" + e.File)
		if err != nil {
			t.Errorf("%s: %v", e.File, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != e.SHA256 {
			t.Errorf("%s: sha256 %s, VERSIONS says %s", e.File, got, e.SHA256)
		}
	}
}

// Every file in vendor/ is listed in VERSIONS, and the path constants point at listed files.
func TestVendorFilesListed(t *testing.T) {
	listed := map[string]bool{}
	for _, e := range vendorEntries(t) {
		listed[e.File] = true
	}
	files, err := fs.ReadDir(FS, "vendor")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Name() != "VERSIONS" && !listed[f.Name()] {
			t.Errorf("vendor/%s is not listed in VERSIONS", f.Name())
		}
	}
	for _, p := range []string{HTMX, HTMXAlpineCompat, Alpine} {
		if !strings.HasPrefix(p, "vendor/") || !listed[path.Base(p)] {
			t.Errorf("constant %q does not name a file listed in VERSIONS", p)
		}
	}
}

func TestParseVersions(t *testing.T) {
	ok := "# comment\n\nhtmx.js 4.0.2 " + strings.Repeat("a", 64) + " https://example.com/htmx.js\nnew.js 1.0 - https://example.com/new.js\n"
	entries, err := ParseVersions(strings.NewReader(ok))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Version != "4.0.2" || entries[1].SHA256 != "-" {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	bad := map[string]string{
		"too few fields": "htmx.js 4.0.2 https://example.com/x\n",
		"bad checksum":   "htmx.js 4.0.2 ABC https://example.com/x\n",
		"plain http":     "htmx.js 4.0.2 - http://example.com/x\n",
		"path in name":   "../htmx.js 4.0.2 - https://example.com/x\n",
		"dot-dot":        ".. 4.0.2 - https://example.com/x\n",
		"dot-file":       ".htmx.js 4.0.2 - https://example.com/x\n",
		"backslash":      `a\b.js 4.0.2 - https://example.com/x` + "\n",
		"duplicate":      "a.js 1 - https://example.com/a\na.js 2 - https://example.com/b\n",
	}
	for name, in := range bad {
		if _, err := ParseVersions(strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
