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

// This file replaces pkcs11reach_test.go, which D-275 put in the way of the
// agent importing internal/keysource/pkcs11 at all. That gate had an expiry
// written into it — "F11 §4 is *supposed* to wire this in, once the remedy
// exists. Deleting this test is part of doing that" — and the remedy now
// exists, so it is deleted and this stands in its place.
//
// It was pkcs11probe_test.go until the worker arrived and there were two of
// these entry points rather than one. The rule is the same for both, so the
// file is named for the rule.
//
// # Why the property had to change rather than simply be dropped
//
// The gate guarded the import because, while the load was in-process, an
// import was a fair proxy for "this binary can be killed by somebody else's
// DLL". It is not a fair proxy any more and it is not the property that
// matters. The agent now legitimately imports both packages — it has to, to
// dispatch the two subcommands and to call Modules, which spawns children —
// and what must stay true is narrower and stronger:
//
//	The agent process loads a PKCS#11 module only when it IS one of the two
//	children spawned for that purpose.
//
// openModule is unexported, so the only ways to load one from outside the
// package are pkcs11.RunProbe and worker.Run. This test therefore asserts that
// each has exactly one call site in the whole of cmd/liro-bridge, and that each
// sits in run() — which is where the two branches are that exist only when this
// process was spawned by another one of ours.
//
// A second call site is how "the agent loads a module in its own process" comes
// back, and it would come back looking reasonable: somebody wanting to check a
// configured path quickly, without the cost of a spawn. D-272 is why that is
// not available, and the reason is measured rather than cautious — one real
// module kills its host about once in a hundred calls and no Go process
// survives it.
func TestTheAgentLoadsAModuleOnlyWhenItIsAChildSpawnedForIt(t *testing.T) {
	// The two entry points, and what each one is for. They are two rather than
	// one deliberately (D-297): a throwaway child per candidate where a crash is
	// the expected outcome, and one child held open where a session is.
	loaders := []struct{ pkg, fn, what string }{
		{"pkcs11", "RunProbe", "the discovery probe: one candidate, loaded in a child that then dies"},
		{"worker", "Run", "the worker: one module, held open, answering a pipe"},
	}

	for _, loader := range loaders {
		sites := callSitesOf(t, loader.pkg, loader.fn)
		name := loader.pkg + "." + loader.fn

		switch {
		case len(sites) == 0:
			t.Errorf("%s is never called in cmd/liro-bridge — %s.\n\n"+
				"Either the subcommand is no longer dispatched, in which case the "+
				"agent spawns children that do not answer, or the name changed and "+
				"this check has stopped looking at anything.", name, loader.what)
		case len(sites) > 1:
			var where []string
			for _, s := range sites {
				where = append(where, s.file+":"+itoa(s.line)+" in "+s.fn+"()")
			}
			t.Errorf("%s has %d call sites: %s\n\n"+
				"It must have exactly one, in run(). Every other call site loads a "+
				"vendor PKCS#11 module into the agent's own process, and D-272 "+
				"measured that one real module kills its host about once in a hundred "+
				"calls, in two ways, neither of which any Go process survives.\n\n"+
				"Whatever the second site wants, it wants Modules or Sources, which "+
				"spawn a child and turn a module that kills it into a Failure in a "+
				"list.", name, len(sites), strings.Join(where, ", "))
		case sites[0].fn != "run":
			t.Errorf("%s is called from %s() at %s:%d, not from run().\n\n"+
				"The one permitted call site is run()'s dispatch, which is reached "+
				"only when this process was spawned as that kind of child. A call "+
				"from anywhere else is the agent loading a module in its own process.",
				name, sites[0].fn, sites[0].file, sites[0].line)
		}
	}
}

type site struct {
	file string
	line int
	fn   string
}

// callSitesOf finds every pkg.fn(...) in cmd/liro-bridge's shipped files, with
// the function each one is inside so that a message can say where rather than
// only at which line — a line number moves whenever anything above it does.
func callSitesOf(t *testing.T, pkg, fn string) []site {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	fset := token.NewFileSet()
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

		var enclosing string
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				enclosing = node.Name.Name
			case *ast.SelectorExpr:
				qualifier, ok := node.X.(*ast.Ident)
				if ok && qualifier.Name == pkg && node.Sel.Name == fn {
					sites = append(sites, site{name, fset.Position(node.Pos()).Line, enclosing})
				}
			}
			return true
		})
	}

	if checked == 0 {
		t.Fatal("no non-test Go file was checked; this test would pass for the wrong reason")
	}
	return sites
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
