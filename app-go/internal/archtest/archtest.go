// Package archtest enforces the module's import boundary (spec §1.3).
//
// Two rules apply to every package of the module (test files excluded):
//
//  1. Direct imports. A package may import a third-party module only if the allow-list
//     below grants that module to that package. Everything else uses the standard library.
//  2. UI isolation. Only UI packages and the composition root (cmd/tmock) may depend on UI
//     packages or on gomponents, directly or indirectly. This keeps the backend free of UI
//     code even though it shares a module with it.
//
// The package itself contains no production code; the checks run in its tests.
package archtest

import (
	"fmt"
	"slices"
	"strings"
)

// Module is the module path.
const Module = "github.com/laposa/tmock/app-go"

// Allowed lists, per third-party module, the module-relative package patterns that may
// import it directly. A pattern ending in "/..." also matches every package below it.
// Changing this list needs an entry in spec §3.2.
var Allowed = map[string][]string{
	"maragu.dev/gomponents":   {"internal/web", "internal/views/..."},
	"github.com/jackc/pgx/v5": {"internal/db"},
}

// UIPackages are the module-relative patterns of the UI layer.
var UIPackages = []string{"internal/web", "internal/views/..."}

// CompositionRoots may depend on everything.
var CompositionRoots = []string{"cmd/tmock"}

// uiModules are third-party modules that only the UI may depend on, even indirectly.
var uiModules = []string{"maragu.dev/gomponents"}

// Package is the part of `go list -json` output the rules need.
type Package struct {
	ImportPath string
	Standard   bool
	Imports    []string
	Deps       []string
}

// Check applies both rules to the module's packages. all must contain every package the
// module's packages depend on (as `go list -deps` returns them), so that standard-library
// packages can be told apart from third-party ones.
func Check(all []Package) []string {
	std := map[string]bool{}
	for _, p := range all {
		if p.Standard {
			std[p.ImportPath] = true
		}
	}

	var problems []string
	for _, p := range all {
		rel, ok := relative(p.ImportPath)
		if !ok {
			continue // not one of our packages
		}

		// Rule 1: direct third-party imports.
		for _, imp := range p.Imports {
			if std[imp] || imp == "C" {
				continue
			}
			if _, own := relative(imp); own {
				continue
			}
			mod := moduleOf(imp)
			if mod == "" {
				problems = append(problems, fmt.Sprintf("%s imports %s, which is not in the allow-list (spec §1.3, §3.2)", rel, imp))
				continue
			}
			if !matchesAny(rel, Allowed[mod]) {
				problems = append(problems, fmt.Sprintf("%s imports %s; only %v may import %s (spec §1.3)", rel, imp, Allowed[mod], mod))
			}
		}

		// Rule 2: UI isolation, over all transitive dependencies.
		if matchesAny(rel, UIPackages) || matchesAny(rel, CompositionRoots) {
			continue
		}
		for _, dep := range p.Deps {
			if depRel, own := relative(dep); own && matchesAny(depRel, UIPackages) {
				problems = append(problems, fmt.Sprintf("%s depends on UI package %s; backend packages must not (spec §1.3)", rel, depRel))
			}
			for _, m := range uiModules {
				if dep == m || strings.HasPrefix(dep, m+"/") {
					problems = append(problems, fmt.Sprintf("%s depends on %s; only UI packages may (spec §1.3)", rel, dep))
				}
			}
		}
	}
	slices.Sort(problems)
	return slices.Compact(problems)
}

// relative returns the module-relative path of one of our packages ("" for the root).
func relative(importPath string) (string, bool) {
	if importPath == Module {
		return "", true
	}
	rest, ok := strings.CutPrefix(importPath, Module+"/")
	return rest, ok
}

// moduleOf returns the allow-listed module that provides importPath, or "".
func moduleOf(importPath string) string {
	for mod := range Allowed {
		if importPath == mod || strings.HasPrefix(importPath, mod+"/") {
			return mod
		}
	}
	return ""
}

func matchesAny(rel string, patterns []string) bool {
	for _, pat := range patterns {
		if base, ok := strings.CutSuffix(pat, "/..."); ok {
			if rel == base || strings.HasPrefix(rel, base+"/") {
				return true
			}
		} else if rel == pat {
			return true
		}
	}
	return false
}
