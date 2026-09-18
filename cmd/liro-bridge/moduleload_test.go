package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This replaces pkcs11reach_test.go, which D-275 put in the way of the agent
// importing internal/keysource/pkcs11 at all. That gate had an expiry written
// into it — "F11 §4 is *supposed* to wire this in, once the remedy exists.
// Deleting this test is part of doing that" — and the remedy now exists, so it
// is deleted and this stands in its place.
//
// # Why the property had to change rather than simply be dropped
//
// The gate guarded the import because, while the load was in-process, an
// import was a fair proxy for "this binary can be killed by somebody else's
// DLL". It is not a fair proxy any more and it is not the property that
// matters. The agent now legitimately imports the package — it has to, to
// dispatch the probe subcommand and to call Modules, which spawns children —
// and what must stay true is narrower and stronger:
//
//	The agent process loads a PKCS#11 module only when it IS the probe child.
//
// openModule is unexported, so the only way to load one from outside the
// package is pkcs11.RunProbe. This test therefore asserts that RunProbe has
// exactly one call site in the whole of cmd/liro-bridge, and that it sits in
// run's probe branch — which is the branch that exists only when this process
// was spawned by another one of ours for that purpose.
//
// A second call site is how "the agent loads a module in its own process"
// comes back, and it would come back looking reasonable: somebody wanting to
// check a configured path quickly, without the cost of a spawn. D-272 is why
// that is not available, and the reason is measured rather than cautious — one
// real module kills its host about once in a hundred calls and no Go process
// survives it.
func TestTheAgentLoadsAModuleOnlyWhenItIsTheProbeChild(t *testing.T) {
	const loaderFunc = "RunProbe"

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	fset := token.NewFileSet()
	type site struct {
		file string
		line int
		fn   string
	}
	var sites []site
	checked := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		checked++

		// Which function each call is inside, so the message can say where and
		// so the one permitted site can be identified by more than a line
		// number that moves whenever anything above it does.
		var enclosing string
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				enclosing = node.Name.Name
			case *ast.SelectorExpr:
				pkg, ok := node.X.(*ast.Ident)
				if ok && pkg.Name == "pkcs11" && node.Sel.Name == loaderFunc {
					sites = append(sites, site{name, fset.Position(node.Pos()).Line, enclosing})
				}
			}
			return true
		})
	}

	if checked == 0 {
		t.Fatal("no non-test Go file was checked; this test would pass for the wrong reason")
	}

	switch {
	case len(sites) == 0:
		t.Fatalf("pkcs11.%s is never called in cmd/liro-bridge.\n\n"+
			"Either the probe subcommand is no longer dispatched — in which case "+
			"discovery cannot work at all, because Modules spawns this binary and "+
			"expects it to answer — or the name changed and this check has stopped "+
			"looking at anything.", loaderFunc)
	case len(sites) > 1:
		var where []string
		for _, s := range sites {
			where = append(where, s.file+":"+itoa(s.line)+" in "+s.fn+"()")
		}
		t.Fatalf("pkcs11.%s has %d call sites: %s\n\n"+
			"It must have exactly one, in run()'s probe branch. Every other call "+
			"site loads a vendor PKCS#11 module into the agent's own process, and "+
			"D-272 measured that one real module kills its host about once in a "+
			"hundred calls, in two ways, neither of which any Go process survives.\n\n"+
			"Whatever the second site wants, it wants Modules or Sources, which "+
			"spawn a child and turn a module that kills it into a Failure in a list.",
			loaderFunc, len(sites), strings.Join(where, ", "))
	case sites[0].fn != "run":
		t.Fatalf("pkcs11.%s is called from %s() at %s:%d, not from run().\n\n"+
			"The one permitted call site is run()'s probe branch, which is reached "+
			"only when this process was spawned as a probe child. A call from "+
			"anywhere else is the agent loading a module in its own process.",
			loaderFunc, sites[0].fn, sites[0].file, sites[0].line)
	}
}

// itoa avoids pulling strconv in for one number in one message.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
