package pkcs11

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pinname"
)

// This is the guard SPEC §6.5.1's second clause rests on, and it was written
// before any of the backend it constrains (D-269).
//
// internal/keysource/windowscng, internal/keysource/softtoken and
// internal/signing each carry a test of this shape already (D-025), and in
// those packages the rule is absolute: the agent never handles a PIN at all,
// so a field or parameter named after one is always wrong. Here the rule is
// different, because §6.5.1 permits exactly one place for a PIN to be — a
// local variable, inside the single function that obtains it, passes it to
// C_Login and overwrites it.
//
// So this test draws a boundary rather than a prohibition, and the boundary
// is the interesting part:
//
//   - A struct field, a function parameter or a named result: refused. Each
//     of those is a way for the PIN to be held, or to travel to a second
//     function, and §6.5.1 forbids both.
//   - A package-level var or const: refused. That one outlives every call.
//   - A local variable inside a function: allowed. That is the one place.
//
// # What this test does not establish
//
// That the PIN is actually overwritten, that it is not copied into something
// with an innocent name, and that no closure captures it. A syntax tree
// cannot see any of those. They are read for, not tested for — this test
// closes the hole that can be closed mechanically and leaves the rest
// visible rather than pretending to cover it.

type finding struct {
	line int
	kind string
	name string
}

// findPINsThatOutliveTheCall reports every place in one file where a PIN could
// be held beyond the call that uses it, or handed to another function.
func findPINsThatOutliveTheCall(fset *token.FileSet, file *ast.File) []finding {
	var found []finding
	add := func(pos token.Pos, kind, name string) {
		found = append(found, finding{fset.Position(pos).Line, kind, name})
	}

	// Package-level var and const. Walked over file.Decls rather than through
	// ast.Inspect, so that a local declaration inside a function body — the one
	// place §6.5.1 allows — is not reached.
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || (gen.Tok != token.VAR && gen.Tok != token.CONST) {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, id := range value.Names {
				if pinname.Names(id.Name) && pinname.DeclarationCouldCarryAPIN(value.Type, value.Values) {
					add(id.Pos(), "package-level "+gen.Tok.String(), id.Name)
				}
			}
		}
	}

	// Struct fields, interface methods, function parameters and named results.
	ast.Inspect(file, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok {
			return true
		}
		for _, id := range field.Names {
			if pinname.Names(id.Name) && pinname.CouldCarryAPIN(field.Type) {
				add(id.Pos(), "field or parameter", id.Name)
			}
		}
		return true
	})

	sort.Slice(found, func(i, j int) bool { return found[i].line < found[j].line })
	return found
}

// TestNoPINIsHeldWhereItCouldOutliveTheCall walks this package's own source.
func TestNoPINIsHeldWhereItCouldOutliveTheCall(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
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
		for _, f := range findPINsThatOutliveTheCall(fset, file) {
			t.Errorf("%s:%d: %s %q — SPEC §6.5.1 allows a PIN only as a local "+
				"variable in the one function that passes it to C_Login and "+
				"overwrites it; it may not be held here", name, f.line, f.kind, f.name)
		}
	}
	if checked == 0 {
		t.Fatal("no non-test Go file was checked; this test would pass for the wrong reason")
	}
}

// TestThePINRuleWouldActuallyFire is the other half. A rule whose matcher
// never matches passes for ever, and this project has recorded that failure
// mode enough times to check for it every time (D-224, D-158, D-161).
//
// It also pins the boundary, which is what makes this rule different from
// D-025's: a local variable must NOT be reported, because that is the one
// place §6.5.1 permits.
func TestThePINRuleWouldActuallyFire(t *testing.T) {
	const src = `package pkcs11

import "runtime"

var cachedPIN []byte         // package-level: caught
const defaultPin = "0000"    // package-level: caught

type session struct {
	pin     []byte           // field: caught
	userPIN []byte           // field: caught, and the regexp would miss it
	pinner  runtime.Pinner   // NOT a PIN
	spinner int              // NOT a PIN
}

func login(hSession uint32, pin []byte) error { return nil }   // parameter: caught

func prompt() (pin []byte, err error) { return nil, nil }      // named result: caught

func loginOnce() error {
	var pin []byte           // local: the one place §6.5.1 allows -- NOT caught
	pinBytes := pin          // local: NOT caught
	_ = pinBytes
	return nil
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, 0)
	if err != nil {
		t.Fatalf("parsing the synthetic fixture: %v", err)
	}

	var got []string
	for _, f := range findPINsThatOutliveTheCall(fset, file) {
		got = append(got, f.name)
	}

	want := []string{"cachedPIN", "defaultPin", "pin", "userPIN", "pin", "pin"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the rule caught %v, want %v", got, want)
	}

	for _, allowed := range []string{"pinner", "spinner", "pinBytes"} {
		for _, g := range got {
			if g == allowed {
				t.Errorf("%q was reported and must not be: it is not a PIN, or it is "+
					"a local variable, which is the one place SPEC §6.5.1 allows", allowed)
			}
		}
	}
}
