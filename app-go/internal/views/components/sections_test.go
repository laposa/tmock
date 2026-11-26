package components

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every component file has its own section in static/app.css, headed with the file's
// name (spec §10.6, §15.1).
func TestEveryComponentHasACSSSection(t *testing.T) {
	css, err := os.ReadFile("../../../static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "doc.go" {
			continue
		}
		if !strings.Contains(string(css), "/* ── "+f+" ──") {
			t.Errorf("static/app.css has no section for %s", f)
		}
	}
}
