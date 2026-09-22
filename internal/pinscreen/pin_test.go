package pinscreen

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

// This is the guard SPEC §6.5.1's second clause requires of a package a PIN
// passes through, and it is written with the package rather than after it, for
// D-269's reason: a test written after the code is a test written around
// whatever that code already does.
//
// The rule is the same boundary internal/keysource/pkcs11 and its worker both
// draw. §6.5.1 permits exactly one place for a PIN to be:
//
//   - a struct field, a function parameter or a named result: refused. Each is
//     a way for the PIN to be held, or to travel to a second function.
//   - a package-level var or const: refused. That one outlives every call.
//   - a local variable inside one function: allowed.
//
// Here the boundary is narrower still, and worth stating because it is the
// whole job of this package: **no PIN should be anywhere in it at all.** Entry
// hands the caller's buffer to the dialog and hands back a length. There is no
// local that holds a PIN either, and nothing in this package reads one.
//
// # This is the sixth copy of this walker in the tree
//
// internal/keysource/windowscng, internal/keysource/softtoken,
// internal/signing, internal/keysource/pkcs11 and its worker carry the other
// five, and internal/ui carries a scoped variant. D-270 unified the *matcher*
// into internal/pinname and recorded that a project-wide check was "the better
// end state for whoever is next given the room for it"; D-290 recorded that it
// was "more clearly overdue at five than at four" and named internal/pinname
// as the natural home, since the matcher already lives there and is already
// measured absent from the release binary.
//
// At six it is overdue again, and this commit is not the place: converging six
// packages' tests is its own change with its own CI step, and this one's job
// is that the guard exists before the code does. The count is recorded rather
// than the caution, so that whoever is next given the room meets a number.

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

	// Package-level var and const, walked over file.Decls rather than through
	// ast.Inspect so that a local declaration inside a function body — the one
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
//
// Every file, including the one for the platform this test is not running on:
// the rule is about what is declared, a syntax tree does not need a compiler,
// and a guard that only saw the files its own GOOS selected would be a guard
// with a hole in it exactly the shape of D-295's two lint arms that were the
// same arm.
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
			t.Errorf("%s:%d: %s %q — nothing in this package may hold a PIN: it "+
				"hands the caller's buffer to the dialog and hands back a length "+
				"(SPEC §6.5.1 clause 2)", name, f.line, f.kind, f.name)
		}
	}
	// Four: pinscreen.go, pinscreen_windows.go, pinscreen_linux.go,
	// pinscreen_other.go. Named rather than "more than nothing", so that a
	// file added to this package without being thought about is a failure
	// rather than a silent widening — **which is what happened when
	// pinscreen_linux.go arrived (D-350)**, and the count is why anybody
	// looked at the new file against this rule at all. It was three until
	// F12 §5 gave this platform a dialog to open.
	if checked != 4 {
		t.Fatalf("checked %d non-test Go files, want 4 — a file has been added to or "+
			"removed from this package, and this guard is about all of them", checked)
	}
}

// TestThePINRuleWouldActuallyFire is the other half. A guard that has never
// been seen to fire is not a guard (D-031, D-224, D-296's first question), and
// this one walks a package that correctly contains no PIN at all — so without
// this it would pass for the emptiest possible reason there is.
func TestThePINRuleWouldActuallyFire(t *testing.T) {
	const src = `package pinscreen

var lastPIN []byte      // package-level: caught
const defaultPin = "00" // package-level: caught

type screen struct {
	pin     []byte // field: caught
	userPIN string // field: caught, and a word-boundary regexp would miss it
	pinShown bool  // NOT a PIN: a bool cannot hold one
}

func show(dst []byte, cardPIN []byte) (pinLength int) { // cardPIN: caught
	var localPIN []byte // local: allowed
	_ = localPIN
	return 0
}
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
	for _, want := range []string{"lastPIN", "defaultPin", "pin", "userPIN", "cardPIN"} {
		if !got[want] {
			t.Errorf("the guard did not catch %q, which it must", want)
		}
	}
	for _, mustNot := range []string{"pinShown", "pinLength", "localPIN", "dst"} {
		if got[mustNot] {
			t.Errorf("the guard caught %q, which is not a PIN", mustNot)
		}
	}
}
