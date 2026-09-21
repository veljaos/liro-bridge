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

// TestTheAuditDirectoryDoesNotDependOnWhereTheProgramWasStarted is the
// guard for a defect that reached two real signatures.
//
// newAuditStore asked platform.ConfigDir for the **literal** "windows"
// rather than runtime.GOOS. That branch is
// filepath.Join(LOCALAPPDATA, "Liro"); LOCALAPPDATA is empty on linux;
// filepath.Join("", "Liro") is the relative path "Liro". So the agent
// wrote SPEC §6.7's hash chain into ./Liro/audit — a different chain
// for every directory anybody ever started it from, each believing it
// is the only one. The first two signatures on this platform went into
// a git repository, and what noticed was `git status`, not a test.
//
// Absolute is the property worth asserting: an audit log whose location
// depends on the caller's working directory is not one log.
func TestTheAuditDirectoryDoesNotDependOnWhereTheProgramWasStarted(t *testing.T) {
	dir := auditDir()
	if !filepath.IsAbs(dir) {
		t.Fatalf("auditDir() = %q, which is relative: the audit log would follow the working directory", dir)
	}
	if !strings.HasSuffix(dir, "audit") {
		t.Errorf("auditDir() = %q, want a path ending in the audit directory", dir)
	}
}

// TestNoPlatformIsNamedWhereRuntimeGOOSBelongs catches the class rather
// than the instance.
//
// platform.ConfigDir and platform.LogDir take the operating system as
// an argument so that tests can ask for another one. That argument is a
// question about the machine the program is running on, and the only
// honest answer in shipped code is runtime.GOOS — a string literal is a
// claim that this file runs on one platform, which is exactly the claim
// F12 §3 stopped being true for this package.
//
// Every file, including the ones this GOOS does not build, because the
// rule is about what is written rather than what is compiled here
// (D-295's lesson about a view that cannot see its own other half).
func TestNoPlatformIsNamedWhereRuntimeGOOSBelongs(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		checked++
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "platform" {
				return true
			}
			switch sel.Sel.Name {
			case "ConfigDir", "LogDir", "RuntimeDir", "StateDir", "DataDir":
			default:
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			t.Errorf("%s:%d: platform.%s is passed the literal %s — shipped code asks runtime.GOOS, "+
				"because naming a platform here is how the audit log ended up in a relative directory (D-339)",
				name, fset.Position(lit.Pos()).Line, sel.Sel.Name, lit.Value)
			return true
		})
	}
	if checked == 0 {
		t.Fatal("no non-test Go files were parsed, so this guard checked nothing")
	}
}
