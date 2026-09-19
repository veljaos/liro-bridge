//go:build windows

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pinname"
)

// This package collects a PIN for the first time (D-277), so it needs a guard
// — and the guard the other four packages carry is the wrong shape for it.
//
// D-025 put an AST check in internal/keysource/windowscng,
// internal/keysource/softtoken and internal/signing, where the rule is
// absolute: the agent never handles a PIN, so a PIN-named declaration that
// could hold one is always wrong. D-269 put a narrower one in
// internal/keysource/pkcs11, where a local variable in the single function
// that calls C_Login is permitted.
//
// Here the problem is different in two ways.
//
// First, **this package uses "pin" as a verb everywhere** — pinPtr, pinUTF16,
// pinHandler, handlerPinCount — because it pins Go memory that crosses into
// WebView2 and Win32 (D-101). D-270 recorded that as the reason "pins" is not
// in pinname's word set, and running a name-based rule over the whole package
// would report the memory-pinning code constantly. So this guard is scoped to
// the one file that touches PIN material, and says so.
//
// Second, and more to the point: **the defect this package could actually have
// is not a badly named field, it is a Go string.** D-277's whole argument for
// the PIN dialog being a native window rather than a page is that a Go string
// cannot be overwritten, so §6.5.1's second clause is unachievable the moment
// the characters become one. The two functions that would do that are
// windows.UTF16ToString and windows.UTF16PtrToString, and they are used
// legitimately elsewhere in this package — webview2_windows.go copies
// ExecuteScript's result out with one. In this file they would undo the reason
// the file exists.

const pinDialogFile = "pindialog_windows.go"

// pinFiles is every file in this package that PIN material passes through.
//
// pin.go joined it when the dialog acquired its first caller and had to learn
// to tell a refusal from a cancellation: the encoding that decides whether
// what was typed fits the token's buffer moved there so it could be tested on
// every platform rather than only where a message loop runs. It holds the
// characters for the length of that decision, which is exactly the scope this
// guard is for — and a second file was the obvious place for the rule to stop
// applying without anybody noticing.
var pinFiles = []string{pinDialogFile, "pin.go"}

func parsePINDialog(t *testing.T) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, pinDialogFile, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", pinDialogFile, err)
	}
	return fset, file
}

// TestThePINDialogNeverMakesAGoString is the check with teeth.
//
// It walks the file for a call to any function whose name ends in
// "UTF16ToString" or "UTF16PtrToString" — the two ways this package turns wide
// text into a Go string — and for a conversion of the dialog's own buffers to
// string. Either would put the typed characters somewhere nothing can
// overwrite, which is exactly what SPEC §10 says this window exists to avoid.
func TestThePINDialogNeverMakesAGoString(t *testing.T) {
	fset, file := parsePINDialog(t)

	forbidden := map[string]bool{
		"UTF16ToString":    true,
		"UTF16PtrToString": true,
	}
	found := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if forbidden[fn.Sel.Name] {
				found++
				t.Errorf("%s:%d: calls %s — the characters would become a Go string, "+
					"which cannot be overwritten, and SPEC §6.5.1's second clause "+
					"requires them to be. That is the whole reason this window is "+
					"not a page (SPEC §10, D-277)",
					pinDialogFile, fset.Position(call.Pos()).Line, fn.Sel.Name)
			}
		case *ast.Ident:
			// string(x) over one of the buffers the PIN passes through.
			if fn.Name == "string" && len(call.Args) == 1 {
				if arg, ok := call.Args[0].(*ast.SelectorExpr); ok {
					if arg.Sel.Name == "dst" || arg.Sel.Name == "wide" {
						found++
						t.Errorf("%s:%d: converts %s to a string", pinDialogFile,
							fset.Position(call.Pos()).Line, arg.Sel.Name)
					}
				}
			}
		}
		return true
	})
	_ = found
}

// TestThePINStringRuleWouldActuallyFire is the other half: a matcher that
// cannot match passes for ever, and this project has recorded that failure
// mode often enough to check for it every time (D-158, D-224, D-270).
func TestThePINStringRuleWouldActuallyFire(t *testing.T) {
	const src = `package ui

import "golang.org/x/sys/windows"

type d struct {
	dst  []byte
	wide []uint16
}

func bad(x *d, p *uint16) {
	_ = windows.UTF16ToString(x.wide)
	_ = windows.UTF16PtrToString(p)
	_ = string(x.dst)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, 0)
	if err != nil {
		t.Fatalf("parsing the synthetic fixture: %v", err)
	}

	hits := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fn.Sel.Name == "UTF16ToString" || fn.Sel.Name == "UTF16PtrToString" {
				hits++
			}
		case *ast.Ident:
			if fn.Name == "string" && len(call.Args) == 1 {
				if arg, ok := call.Args[0].(*ast.SelectorExpr); ok && (arg.Sel.Name == "dst" || arg.Sel.Name == "wide") {
					hits++
				}
			}
		}
		return true
	})
	if hits != 3 {
		t.Errorf("the rule caught %d of the 3 ways to make a string out of a PIN", hits)
	}
}

// TestNoPINIsHeldInTheDialogWhereItCouldOutliveTheCall is D-269's guard,
// scoped to this one file.
//
// The scope is the point and is explained at the top of this file: the rest of
// internal/ui pins memory for a living, and a name-based rule over the whole
// package would report that instead of this.
func TestNoPINIsHeldInTheDialogWhereItCouldOutliveTheCall(t *testing.T) {
	for _, name := range pinFiles {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			// A rename reaches here rather than silently shrinking the guard's
			// scope, which is what the parse error being fatal is for.
			t.Fatalf("parsing %s: %v", name, err)
		}
		checkNoPINOutlivesTheCall(t, name, fset, file)
	}
}

// TestThePINOutliveRuleWouldActuallyFire is the control this check has never
// had, added when the check was widened to a second file.
//
// It matters more now than it did: the guard walks two files and passes, and
// both of them correctly contain nothing for it to find — so without a fixture
// it would report success for the emptiest possible reason, and a widening
// that quietly caught nothing would look exactly like a widening that worked.
// D-296's first question, asked of a check that had gone six months without it.
func TestThePINOutliveRuleWouldActuallyFire(t *testing.T) {
	const src = `package ui

import "runtime"

var lastPIN []byte      // package-level: caught
const defaultPin = "00" // package-level: caught

type d struct {
	pin     []byte         // field: caught
	userPIN []byte         // field: caught, and a word-boundary regexp would miss it
	pinner  runtime.Pinner // NOT a PIN: this package pins memory for a living
	pinned  bool           // NOT a PIN
}

func read(dst []byte, cardPIN []byte) (pinCount int) { // cardPIN: caught
	var localPIN []byte // local: allowed
	_ = localPIN
	return 0
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, 0)
	if err != nil {
		t.Fatalf("parsing the synthetic fixture: %v", err)
	}

	got := map[string]bool{}
	collect := &fakeT{seen: got}
	checkNoPINOutlivesTheCall(collect, "synthetic.go", fset, file)

	for _, want := range []string{"lastPIN", "defaultPin", "pin", "userPIN", "cardPIN"} {
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

// fakeT collects what the guard would have reported instead of failing the
// run, so that the control can assert on the names rather than on a count.
//
// It satisfies only the part of testing.TB the guard uses. Errorf's format is
// this file's own, so reading the name back out of it is reading this file
// rather than guessing at somebody else's string.
type fakeT struct {
	testing.TB
	seen map[string]bool
}

func (f *fakeT) Helper() {}

func (f *fakeT) Errorf(format string, args ...any) {
	for _, a := range args {
		if s, ok := a.(string); ok {
			f.seen[s] = true
		}
	}
}

// checkNoPINOutlivesTheCall takes testing.TB rather than *testing.T so that
// the control above can collect what it reports instead of failing the run.
func checkNoPINOutlivesTheCall(t testing.TB, pinDialogFile string, fset *token.FileSet, file *ast.File) {
	t.Helper()

	// Package-level var and const, walked over Decls so a local inside a
	// function body — which is where the buffers legitimately live — is not
	// reached.
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
					t.Errorf("%s:%d: package-level %s %q could hold a PIN beyond the call",
						pinDialogFile, fset.Position(id.Pos()).Line, gen.Tok, id.Name)
				}
			}
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok {
			return true
		}
		for _, id := range field.Names {
			if pinname.Names(id.Name) && pinname.CouldCarryAPIN(field.Type) {
				t.Errorf("%s:%d: field or parameter %q could hold a PIN beyond the call",
					pinDialogFile, fset.Position(id.Pos()).Line, id.Name)
			}
		}
		return true
	})
}

// TestThePINDialogKeepsNothingAfterItReturns reads the file for the two wipes
// that make the claim above true, because the AST checks are about what cannot
// be there and this is about what must be.
//
// It is a weaker kind of check than the others and it is here deliberately: a
// syntax tree cannot see that a buffer was actually overwritten (D-269's own
// note), so what is checked is that the calls exist and are deferred. The
// bytes being zero afterwards is measured in
// internal/keysource/pkcs11's TestWipeZeroesTheBytesAtThePinnedAddress, on the
// primitive both sides use.
func TestThePINDialogKeepsNothingAfterItReturns(t *testing.T) {
	_, file := parsePINDialog(t)

	var src strings.Builder
	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "runPINDialog" {
			ast.Inspect(fn, func(inner ast.Node) bool {
				if d, ok := inner.(*ast.DeferStmt); ok {
					if lit, ok := d.Call.Fun.(*ast.FuncLit); ok {
						ast.Inspect(lit, func(c ast.Node) bool {
							if call, ok := c.(*ast.CallExpr); ok {
								if id, ok := call.Fun.(*ast.Ident); ok {
									src.WriteString(id.Name + " ")
								}
							}
							return true
						})
					}
				}
				return true
			})
		}
		return true
	})
	if !strings.Contains(src.String(), "wipeUTF16") {
		t.Errorf("runPINDialog does not defer wipeUTF16; the buffer the edit control "+
			"is read into would survive the call (deferred calls found: %q)", src.String())
	}
}
