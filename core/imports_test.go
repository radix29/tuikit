package core

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// allowedExternal is everything outside the standard library and tuikit
// itself that a tuikit package may import. It is the whole of the library's
// dependency surface: a new entry here is a new requirement for every
// module that uses tuikit.
var allowedExternal = []string{
	"github.com/clipperhouse/displaywidth",
	"github.com/gdamore/tcell/v3",
	"github.com/gdamore/tcell/v3/color",
}

// testKitExternal is what a test-kit package may import beyond
// allowedExternal. tuikittest is imported by _test files only, and tcell/v3/vt
// lives in the tcell module tuikit already requires, so it adds no
// requirement for a module using tuikit's controls.
var testKitExternal = map[string][]string{
	"tuikittest": {"github.com/gdamore/tcell/v3/vt"},
}

// hostPrefixes are the applications tuikit was extracted from and the
// libraries they bring. A test of tuikit importing one would compile only
// inside that application's module.
var hostPrefixes = []string{
	"github.com/radix29/gossms",
	"github.com/radix29/gosmo",
}

// TestTuikitImportsOnlyItsDeclaredDependencies pins tuikit's dependency
// invariant, so the module stays a library any tcell program can take: a tuikit
// package imports the standard library, other tuikit packages, tcell and
// displaywidth, and nothing else — in particular nothing of an application
// built on it. Test files may use further third-party packages (tcell/v3/vt,
// for one), but not hostPrefixes.
//
// It lists ../... from core, so it covers every package of the module.
func TestTuikitImportsOnlyItsDeclaredDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not on PATH")
	}
	// The test's working directory is the core package, so ../... is tuikit.
	out, err := exec.Command("go", "list", "-f",
		`{{.ImportPath}}|{{join .Imports " "}}|{{join .TestImports " "}} {{join .XTestImports " "}}`,
		"../...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	type pkg struct {
		path         string
		imports, tst []string
	}
	var pkgs []pkg
	for line := range strings.Lines(string(out)) {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) != 3 {
			continue
		}
		pkgs = append(pkgs, pkg{parts[0], strings.Fields(parts[1]), strings.Fields(parts[2])})
	}
	// tuikit's import path is core's, minus "/core".
	var root string
	for _, p := range pkgs {
		if before, ok := strings.CutSuffix(p.path, "/core"); ok {
			root = before
		}
	}
	if root == "" || len(pkgs) < 2 {
		t.Fatalf("go list did not find tuikit's packages:\n%s", out)
	}

	isStd := func(imp string) bool {
		first, _, _ := strings.Cut(imp, "/")
		return !strings.Contains(first, ".")
	}
	inTuikit := func(imp string) bool { return imp == root || strings.HasPrefix(imp, root+"/") }

	for _, p := range pkgs {
		for _, imp := range p.imports {
			if isStd(imp) || inTuikit(imp) || slices.Contains(allowedExternal, imp) ||
				slices.Contains(testKitExternal[strings.TrimPrefix(p.path, root+"/")], imp) {
				continue
			}
			t.Errorf("%s imports %s; tuikit may import only the standard library, tuikit and %s",
				p.path, imp, strings.Join(allowedExternal, ", "))
		}
		for _, imp := range p.tst {
			if inTuikit(imp) || isStd(imp) {
				continue
			}
			if slices.ContainsFunc(hostPrefixes, func(h string) bool { return strings.HasPrefix(imp, h) }) {
				t.Errorf("a test of %s imports %s, which is outside tuikit", p.path, imp)
			}
		}
	}
}

// tuikitLayers is README.md § Dependency direction: the tuikit packages each
// tuikit package may import (production code only). A package missing here
// fails the test, so a new one is placed in the graph on purpose.
var tuikitLayers = map[string][]string{
	"theme":           {},
	"core":            {"theme"},
	"widgets":         {"core", "theme"},
	"charts":          {"core", "theme"},
	"controls/editor": {"core", "theme"},
	"controls":        {"core", "theme", "controls/editor"},
	"layout":          {"core", "theme"},
	"dialogs":         {"core", "theme", "widgets"},
	"propsheet":       {"core", "theme", "widgets", "dialogs", "controls", "controls/editor"},
	"tuikittest":      {"core"},
}

// TestTuikitLayering pins the one-way graph between tuikit's own packages:
// layout is a leaf over core/theme (the tab-strip geometry lives in core for
// that reason), controls/editor is a leaf too (controls reaches it only through
// ValueViewer), and only propsheet composes dialogs and controls.
func TestTuikitLayering(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not on PATH")
	}
	out, err := exec.Command("go", "list", "-f", `{{.ImportPath}}|{{join .Imports " "}}`, "../...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var root string
	type pkg struct {
		path    string
		imports []string
	}
	var pkgs []pkg
	for line := range strings.Lines(string(out)) {
		path, imps, ok := strings.Cut(strings.TrimSpace(line), "|")
		if !ok {
			continue
		}
		pkgs = append(pkgs, pkg{path, strings.Fields(imps)})
		if before, ok := strings.CutSuffix(path, "/core"); ok {
			root = before
		}
	}
	if root == "" {
		t.Fatalf("go list did not find tuikit's core:\n%s", out)
	}
	for _, p := range pkgs {
		name := strings.TrimPrefix(p.path, root+"/")
		allowed, known := tuikitLayers[name]
		if !known {
			t.Errorf("tuikit package %s is not in tuikitLayers; place it in README.md § Dependency direction and here", name)
			continue
		}
		for _, imp := range p.imports {
			dep, ok := strings.CutPrefix(imp, root+"/")
			if !ok {
				continue
			}
			if !slices.Contains(allowed, dep) {
				t.Errorf("%s imports %s; README.md § Dependency direction allows only %v", name, dep, allowed)
			}
		}
	}
}
