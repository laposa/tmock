package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestImportBoundary checks the real module.
func TestImportBoundary(t *testing.T) {
	pkgs := listPackages(t)
	if len(pkgs) == 0 {
		t.Fatal("go list returned no packages")
	}
	for _, p := range Check(pkgs) {
		t.Error(p)
	}
}

func listPackages(t *testing.T) []Package {
	t.Helper()
	touchSources(t)
	// `go test` puts $GOROOT/bin first on PATH, so this finds the go that runs the test.
	cmd := exec.Command("go", "list", "-deps", "-json", Module+"/...")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}
	var pkgs []Package
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p Package
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// touchSources reads every Go source file and go.mod of the module. `go test` caches
// results based on the files the test binary opens; the files `go list` reads in a
// subprocess are invisible to it. Reading them here makes any source change re-run the test.
func touchSources(t *testing.T) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root not found at %s: %v", root, err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "bin") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") || d.Name() == "go.mod" {
			_, err = os.ReadFile(path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func std(paths ...string) []Package {
	var ps []Package
	for _, p := range paths {
		ps = append(ps, Package{ImportPath: p, Standard: true})
	}
	return ps
}

func own(rel string, imports, deps []string) Package {
	return Package{ImportPath: Module + "/" + rel, Imports: imports, Deps: deps}
}

const (
	g     = "maragu.dev/gomponents"
	gHTML = "maragu.dev/gomponents/html"
	pgx   = "github.com/jackc/pgx/v5/stdlib"
	web   = Module + "/internal/web"
	db    = Module + "/internal/db"
)

func TestCheckRules(t *testing.T) {
	base := std("fmt", "net/http", "database/sql")
	cases := []struct {
		name string
		pkg  Package
		want string // substring of an expected problem; "" = no problems at all
	}{
		{"stdlib only", own("internal/api", []string{"net/http"}, []string{"net/http"}), ""},
		{"UI imports gomponents", own("internal/web", []string{gHTML}, []string{g, gHTML}), ""},
		{"views subpackage imports gomponents", own("internal/views/pages", []string{g}, []string{g}), ""},
		{"db imports pgx", own("internal/db", []string{pgx, "database/sql"}, []string{pgx}), ""},
		{"store reaches pgx via db", own("internal/store", []string{db}, []string{db, pgx}), ""},
		{"composition root reaches UI", own("cmd/tmock", []string{web}, []string{web, g}), ""},

		{"backend imports gomponents", own("internal/api", []string{g}, []string{g}), "internal/api imports maragu.dev/gomponents"},
		{"store imports pgx directly", own("internal/store", []string{pgx}, []string{pgx}), "internal/store imports github.com/jackc/pgx/v5/stdlib"},
		{"unknown module", own("internal/api", []string{"example.com/lib"}, []string{"example.com/lib"}), "example.com/lib, which is not in the allow-list"},
		{"backend reaches UI package", own("internal/server", []string{web}, []string{web}), "internal/server depends on UI package internal/web"},
		{"backend reaches gomponents indirectly", own("internal/app", []string{Module + "/internal/other"}, []string{g}), "internal/app depends on maragu.dev/gomponents"},
		{"views prefix is not a wildcard for viewsx", own("internal/viewsx", []string{g}, []string{g}), "internal/viewsx imports maragu.dev/gomponents"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Check(append(base, tc.pkg))
			switch {
			case tc.want == "" && len(got) != 0:
				t.Errorf("unexpected problems: %v", got)
			case tc.want != "" && !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, tc.want) }):
				t.Errorf("got %v, want a problem containing %q", got, tc.want)
			}
		})
	}
}

func TestCheckIgnoresForeignPackages(t *testing.T) {
	// A third-party package's own imports are not our concern.
	got := Check([]Package{{ImportPath: g, Imports: []string{"example.com/other"}}})
	if len(got) != 0 {
		t.Errorf("unexpected problems: %v", got)
	}
}
