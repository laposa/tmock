package components

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The UI packages other than this one. None of them may set a class attribute
// (spec §10.7): their markup comes from components.
var otherUIPackages = []string{"../layout", "../pages", "../hx", "../../web"}

// classAttr matches attribute names that set classes: class, and Alpine's x-bind:class
// and :class.
var classAttr = regexp.MustCompile(`^(?:x-bind)?:?class$`)

// classProblems lists every place in the Go source src that sets a class attribute:
// a call of html.Class or html.ClassMap (under any import name), or g.Attr / html.Attr
// with a class attribute name.
func classProblems(fset *token.FileSet, file *ast.File) []string {
	htmlNames := map[string]bool{}
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path != "maragu.dev/gomponents/html" && path != "maragu.dev/gomponents" {
			continue
		}
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		htmlNames[name] = true
	}
	var problems []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || !htmlNames[pkg.Name] {
			return true
		}
		switch sel.Sel.Name {
		case "Class", "ClassMap":
			problems = append(problems, fset.Position(call.Pos()).String()+": "+pkg.Name+"."+sel.Sel.Name)
		case "Attr":
			if len(call.Args) > 0 {
				if lit, ok := call.Args[0].(*ast.BasicLit); ok {
					name, _ := strconv.Unquote(lit.Value)
					if classAttr.MatchString(name) {
						problems = append(problems, fset.Position(call.Pos()).String()+": attribute "+name)
					}
				}
			}
		}
		return true
	})
	return problems
}

func TestOnlyComponentsSetClasses(t *testing.T) {
	fset := token.NewFileSet()
	checked := 0
	for _, dir := range otherUIPackages {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file, err := parser.ParseFile(fset, path, src, 0)
			if err != nil {
				return err
			}
			checked++
			for _, p := range classProblems(fset, file) {
				t.Errorf("%s sets a class attribute; use a component from internal/views/components instead (spec §10.7)", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 4 {
		t.Fatalf("only %d files checked; are the package paths right?", checked)
	}
}

func TestClassProblemsCatchesViolations(t *testing.T) {
	bad := `package p
import (
	g "maragu.dev/gomponents"
	html "maragu.dev/gomponents/html"
)
var _ = html.Class("x")
var _ = html.ClassMap{}
var _ = g.Attr("class", "x")
var _ = html.Attr("x-bind:class", "x")
var _ = g.Attr(":class", "x")
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "bad.go", bad, 0)
	if err != nil {
		t.Fatal(err)
	}
	// html.ClassMap{} is a composite literal, not a call; the other four are caught.
	if got := classProblems(fset, file); len(got) != 4 {
		t.Errorf("caught %d: %v", len(got), got)
	}

	good := `package p
import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)
var _ = h.ID("class")
var _ = g.Attr("x-data", "class")
var _ = g.Attr("data-class", "x")
`
	file, err = parser.ParseFile(fset, "good.go", good, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := classProblems(fset, file); len(got) != 0 {
		t.Errorf("false positives: %v", got)
	}
}
