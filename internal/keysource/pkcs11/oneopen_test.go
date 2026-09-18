package pkcs11

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// openers are the ways a module gets loaded and C_Initialize gets called.
var openers = map[string]bool{
	"openModule":              true,
	"openModuleLocking":       true,
	"openModuleWith":          true,
	"openModuleProbingLayout": true,
	"Hold":                    true,
}

// oneShots are the exported entry points that open a module, do one thing, and
// close it. A function that already has a module open must not call one: that
// is a second C_Initialize inside the first.
var oneShots = map[string]bool{
	"Enumerate": true,
	"List":      true,
	"ChainFor":  true,
	"Open":      true,
}

// TestNothingHoldingAModuleOpensAnother is the property the Enumerate refactor
// exists to create, checked where it can actually be broken.
//
// # What it is protecting
//
// Before this, Enumerate opened a module, did one thing and closed it, and List
// and ChainFor each called Enumerate. That is correct for a caller who wants an
// answer now. It is wrong for the worker, whose entire reason to exist is that
// C_Initialize must not be repeated: a worker serving list, then chainfor,
// would have paid for two, and D-272 measured that one real module dies inside
// its own C_Initialize about once in a hundred calls. Paying it per request
// turns a startup problem into a listing problem, and a listing that fails one
// time in a hundred is the kind of defect people learn to re-run instead of
// read (D-297).
//
// So the work moved into methods that take an already-open *module, and the
// three exported entry points became those methods with an open and a close
// around them. The regression this guards against is the obvious edit — a
// module-taking method reaching for the exported name it used to call, because
// s.Enumerate(ctx) compiles just as happily as s.enumerate(ctx, m) and reads
// almost the same.
//
// # Why a syntax-tree check rather than counting calls at run time
//
// Counting C_Initialize would mean a counter in shipped code whose only reader
// is a test, which is the scaffolding D-100 keeps out of the product. And the
// property is about what is *declared* — which is what this project already
// reads the syntax tree for: the PIN guards (D-025, D-270), AllCodes (D-158),
// the loopback bind (D-185), the flag names (D-224).
//
// It reads the files from disk rather than through the build, so it asks the
// same question in the Windows and the Linux views and cannot answer
// differently in the one nobody runs it in (D-295).
func TestNothingHoldingAModuleOpensAnother(t *testing.T) {
	fset := token.NewFileSet()
	holders := 0
	sawOpener := false

	for _, file := range nonTestFiles(t) {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if !holdsAModule(fn) {
				if calls(fn, openers) {
					sawOpener = true
				}
				continue
			}
			holders++
			for _, bad := range calledNames(fn, openers) {
				t.Errorf("%s at %s:%d already has a module open and calls %s.\n\n"+
					"That is a second C_Initialize inside the first. D-272 measured one "+
					"real module dying inside its own C_Initialize about once in a "+
					"hundred calls, and D-297 is why the worker holds one open rather "+
					"than paying per request.\n\n"+
					"It wants the *module it was handed.",
					declName(fn), filepath.Base(file), fset.Position(fn.Pos()).Line, bad)
			}
			for _, bad := range calledNames(fn, oneShots) {
				t.Errorf("%s at %s:%d already has a module open and calls %s.\n\n"+
					"The exported entry points open a module, do one thing and close "+
					"it, so calling one from a function that already holds one opens a "+
					"second — which is exactly what the worker exists to avoid "+
					"(D-297).\n\n"+
					"The unexported method of the same name takes the module it was "+
					"handed: enumerate, list, chainFor.",
					declName(fn), filepath.Base(file), fset.Position(fn.Pos()).Line, bad)
			}
		}
	}

	// Both halves of the positive control, because a rule that has never been
	// seen to have anything to say passes for ever (D-031, D-296's first
	// question). If there are no module-taking functions, the refactor has been
	// undone; if nothing opens a module, this check is reading the wrong files.
	if holders == 0 {
		t.Error("no function in this package takes a *module or holds one on a " +
			"LiveModule, so this check has nothing to be about. Either the " +
			"Enumerate refactor was undone or these files are no longer where " +
			"the work is.")
	}
	if !sawOpener {
		t.Error("nothing in this package calls openModule, so this check is not " +
			"reading the files it is about")
	}
}

// holdsAModule reports whether fn already has an initialised module in hand:
// it takes one, or it is a method on the type whose whole purpose is holding
// one open.
func holdsAModule(fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		for _, f := range fn.Recv.List {
			switch recv := typeName(f.Type); recv {
			case "*module", "module", "*LiveModule", "LiveModule":
				return true
			}
		}
	}
	if fn.Type.Params == nil {
		return false
	}
	for _, p := range fn.Type.Params.List {
		if typeName(p.Type) == "*module" {
			return true
		}
	}
	return false
}

// calledNames returns the names in want that fn calls, as bare identifiers or
// as the selector of a method call. It does not resolve types, and does not
// need to: the names it looks for are unique in this package.
func calledNames(fn *ast.FuncDecl, want map[string]bool) []string {
	var found []string
	seen := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch f := call.Fun.(type) {
		case *ast.Ident:
			name = f.Name
		case *ast.SelectorExpr:
			name = f.Sel.Name
		}
		if want[name] && !seen[name] {
			seen[name] = true
			found = append(found, name)
		}
		return true
	})
	return found
}

func calls(fn *ast.FuncDecl, want map[string]bool) bool {
	return len(calledNames(fn, want)) > 0
}

func declName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return typeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// typeName renders a type expression well enough to recognise *module. It is
// deliberately shallow: anything it cannot name is something this rule is not
// about.
func typeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeName(t.X)
	case *ast.SelectorExpr:
		return typeName(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}

// nonTestFiles lists this package's shipped Go files. The rule reads the code
// that runs, not itself.
func nonTestFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		t.Fatal("no non-test Go file was found; this check would pass for the wrong reason")
	}
	return out
}
