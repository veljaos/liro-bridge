//go:build linux

package chooser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// # Why these guards exist, for whoever is about to change them
//
// This package is the one process of this binary that keeps its dumpable
// flag (D-408, D-410). Every other process clears it (D-376), so that a crash
// cannot carry a PIN or a signing session away and so that, on a machine with
// ptrace_scope 0 such as stock Fedora, no other program of the same user can
// read its memory. This one cannot clear it, because the desktop's portal
// refuses any caller whose /proc/PID/root it cannot open, and without the
// portal a person on Fedora 44 cannot give this program a document at all.
//
// So the process is safe for one reason only: **it holds nothing worth
// taking**. Its title, its filters, the paths a person chose. It is started
// with an empty environment and told everything on stdin, because a crash
// under the core limit alone still writes the environment and command line
// into the system journal (D-409).
//
// That reason survives only as long as the process cannot reach anything that
// holds more. A helper that could import the key sources, the PKCS#11 worker,
// the configuration, the audit log, the protocol server, the pairings or the
// windows is one edit away from holding a secret while dumpable, and the edit
// would look harmless to somebody who did not know why this process is shaped
// the way it is. A helper that can import os/exec is one edit away from
// starting something else that inherits its dumpability. **These guards are
// what stops that edit, and they are meant to be hard to argue with.** If one
// of them is in your way, the answer is almost certainly that the code you are
// adding belongs in the parent (internal/ui/filedialog_linux.go), not here.

// moduleAllowed is every package of this module the helper may reach,
// directly or not. Nothing else of this module is allowed, which covers the
// ones named in the owner's rule — keysource, pkcs11, config, audit,
// protocol, pairing, ui — and whatever is added after them.
var moduleAllowed = map[string]bool{
	"github.com/veljaos/liro-bridge/internal/chooser":            true,
	"github.com/veljaos/liro-bridge/internal/platform/corelimit": true,
}

const modulePath = "github.com/veljaos/liro-bridge/"

// TestTheHelperReachesNothingOfThisProgramThatHoldsMore reads the helper's
// whole dependency graph as the toolchain builds it for Linux, not only this
// directory's import lines: an allowed package that later imports a
// forbidden one is the same hole.
func TestTheHelperReachesNothingOfThisProgramThatHoldsMore(t *testing.T) {
	deps := goListDeps(t)

	// The instrument's control: a graph that did not include this package
	// and its one allowed dependency was not the helper's graph.
	for want := range moduleAllowed {
		if !deps[want] {
			t.Fatalf("go list -deps did not list %s; this is not the helper's graph, and the check below would pass for the wrong reason", want)
		}
	}

	for dep := range deps {
		if strings.HasPrefix(dep, modulePath) && !moduleAllowed[dep] {
			t.Errorf("the chooser helper reaches %s. It keeps its dumpable flag and must hold nothing but what a person chooses; see the comment at the top of this file", dep)
		}
	}
}

// TestTheHelperCannotStartAnything is os/exec, which the helper must not
// import. It is also godbus's, which imports it to run dbus-launch when a
// session bus is looked for with autolaunch. The helper never looks for one:
// it connects to the address it is given, with dbus.Connect, which has no
// route to dbus-launch. So the rule is: no os/exec in this package's own
// code, and none of godbus's functions that can autolaunch.
func TestTheHelperCannotStartAnything(t *testing.T) {
	files := packageFiles(t)
	if len(files) == 0 {
		t.Fatal("found no non-test Go files in this package; the check below would pass for the wrong reason")
	}
	sawConnect := false
	for name, f := range files {
		for _, imp := range f.Imports {
			if path := strings.Trim(imp.Path.Value, `"`); path == "os/exec" {
				t.Errorf("%s imports os/exec. The helper is dumpable, and whatever it started would be too; see the comment at the top of this file", name)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "dbus" {
				return true
			}
			if strings.Contains(sel.Sel.Name, "SessionBus") {
				t.Errorf("%s calls dbus.%s, which can run dbus-launch; the helper connects to the address it was given with dbus.Connect", name, sel.Sel.Name)
			}
			if sel.Sel.Name == "Connect" {
				sawConnect = true
			}
			return true
		})
	}
	// The control: the selector matcher has to be able to see the call the
	// helper does make, or its silence about the ones it must not is no
	// evidence.
	if !sawConnect {
		t.Error("found no dbus.Connect in this package; the selector check above cannot see what it is looking for")
	}
}

// goListDeps is the transitive import graph of this package, as built for
// Linux, without tests.
func goListDeps(t *testing.T) map[string]bool {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	cmd.Env = append(os.Environ(), "GOOS=linux")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	deps := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		deps[strings.TrimSpace(line)] = true
	}
	return deps
}

// packageFiles parses this directory's non-test Go files.
func packageFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = f
	}
	return files
}
