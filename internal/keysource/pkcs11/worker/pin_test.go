package worker

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

// This is the guard SPEC §6.5.1's amended second clause requires of this
// package — "the AST guard ... extends to the worker's own package, which is
// new and must carry one from its first commit rather than acquire one
// afterwards" — and it is written here before any of the code it constrains,
// for D-269's reason: a test written after a backend is a test written around
// whatever that backend already does.
//
// The rule is the same boundary internal/keysource/pkcs11's own guard draws,
// and it is a boundary rather than a prohibition. §6.5.1 permits exactly one
// place for a PIN to be:
//
//   - a struct field, a function parameter or a named result: refused. Each
//     is a way for the PIN to be held, or to travel to a second function.
//   - a package-level var or const: refused. That one outlives every call.
//   - a local variable inside one function: allowed. That is the one place.
//
// In this package the clause bites harder than it does in the binding,
// because here the PIN arrives over a pipe: the read must go straight into the
// pinned buffer that C_Login is given, and anything named after a PIN that can
// hold bytes and is not a local is evidence that a copy was made on the way.
//
// # What this does not establish
//
// That the PIN is actually overwritten, that no closure captures it, and that
// it was not copied into something with an innocent name. A syntax tree cannot
// see any of those; they are read for. This guard closes the accidental case,
// which is the one that arrives without anybody deciding to make it.
//
// # A note for whoever is next given the room
//
// This is the fifth copy of this walker in the tree — internal/keysource/
// windowscng, internal/keysource/softtoken, internal/signing and
// internal/keysource/pkcs11 carry the other four. D-270 unified the *matcher*
// into internal/pinname and recorded that a project-wide check was "the better
// end state for whoever is next given the room for it". It is more clearly
// overdue with five than it was with four, and the natural home is now
// internal/pinname rather than a new script, since the matcher already lives
// there and it is already measured absent from the release binary (D-270).
// Not done here: converging five packages' tests is its own change, and this
// commit's job is that the guard exists before the code does.

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

// TestThePINRuleWouldActuallyFire is the other half. A guard that has never
// been seen to fire is not a guard (D-031, D-224), and this one currently
// walks a package with nothing in it but a doc comment — so without this, it
// would pass for the emptiest possible reason.
func TestThePINRuleWouldActuallyFire(t *testing.T) {
	const src = `package worker

import "runtime"

var cachedPIN []byte      // package-level: caught
const defaultPin = "0000" // package-level: caught

type reader struct {
	pin     []byte         // field: caught
	userPIN []byte         // field: caught, and a word-boundary regexp would miss it
	pinner  runtime.Pinner // NOT a PIN: this package pins memory
	pinned  bool           // NOT a PIN
}

func readPIN(dst []byte, pinBytes []byte) int { // parameter: caught
	var localPIN []byte // local: allowed, this is the one place
	_ = localPIN
	return 0
}

func ok(n int) (pinCount int) { return 0 } // NOT a PIN: an int cannot hold one
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}

	got := map[string]bool{}
	for _, f := range findPINsThatOutliveTheCall(fset, file) {
		got[f.name] = true
	}

	for _, want := range []string{"cachedPIN", "defaultPin", "pin", "userPIN", "pinBytes"} {
		if !got[want] {
			t.Errorf("the guard did not catch %q, which it must", want)
		}
	}
	for _, mustNot := range []string{"pinner", "pinned", "localPIN", "pinCount"} {
		if got[mustNot] {
			t.Errorf("the guard caught %q, which is not a PIN — this package pins "+
				"memory and a rule that reports that is a rule people work around "+
				"(D-270)", mustNot)
		}
	}
}
