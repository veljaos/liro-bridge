//go:build linux

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The PIN guard for this platform's dialog, which F12 §5 requires to
// cover whatever new package holds it.
//
// It is `pin_test.go`'s rule pointed at a different file and a different
// set of forbidden calls, because **the way a PIN becomes a Go string
// here is not the way it does on Windows.** There the two functions are
// windows.UTF16ToString and windows.UTF16PtrToString. Here GTK's text is
// already UTF-8 and the equivalents are cgo's C.GoString and
// C.GoStringN, plus gotk4's own Text() and GetText() accessors — every
// one of which returns a `string`, which cannot be overwritten, which is
// exactly what SPEC §6.5.1 clause 2 requires these bytes to be.
//
// That is why the dialog reads the entry through three C helpers rather
// than through the binding: `liro_pin_len` gives a length and no bytes,
// and `liro_pin_copy` copies into the caller's own buffer. The guard
// below is what keeps somebody from reaching for the obvious
// `entry.Text()` later, which would compile, work, and quietly undo the
// reason this file exists.
const pinDialogLinuxFile = "pindialog_linux.go"

// forbiddenHere is every way this file could turn the typed characters
// into a Go string.
var forbiddenHere = map[string]bool{
	"GoString":  true, // C.GoString
	"GoStringN": true, // C.GoStringN
	"Text":      true, // gtk.Editable.Text(), gtk.PasswordEntry.Text()
	"GetText":   true,
}

func parseLinuxPINDialog(t *testing.T, path string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return fset, file
}

// pinStringCalls returns every call in the file that would make a Go
// string out of what was typed.
func pinStringCalls(fset *token.FileSet, file *ast.File) []string {
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if forbiddenHere[fn.Sel.Name] {
				found = append(found, fn.Sel.Name+" at line "+
					itoa(fset.Position(call.Pos()).Line))
			}
		case *ast.Ident:
			// string(dst) and friends.
			if fn.Name == "string" && len(call.Args) == 1 {
				if arg, ok := call.Args[0].(*ast.Ident); ok && (arg.Name == "dst" || arg.Name == "pin") {
					found = append(found, "string("+arg.Name+") at line "+
						itoa(fset.Position(call.Pos()).Line))
				}
			}
		}
		return true
	})
	return found
}

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

// TestTheLinuxPINDialogNeverMakesAGoString is the check with teeth.
func TestTheLinuxPINDialogNeverMakesAGoString(t *testing.T) {
	fset, file := parseLinuxPINDialog(t, pinDialogLinuxFile)
	if found := pinStringCalls(fset, file); len(found) != 0 {
		t.Errorf("%s makes a Go string out of the typed characters: %s\n"+
			"A Go string cannot be overwritten, and SPEC §6.5.1 clause 2 requires these bytes "+
			"to be. That is the whole reason this window is not a page (SPEC §10, D-277), and "+
			"it is why the entry is read through liro_pin_len and liro_pin_copy rather than "+
			"through the binding's Text().",
			pinDialogLinuxFile, strings.Join(found, ", "))
	}
}

// TestTheLinuxPINStringRuleWouldActuallyFire is the other half: a
// matcher that cannot match passes for ever, and this project has
// recorded that failure mode often enough to check for it every time
// (D-158, D-224, D-270, and twice today).
func TestTheLinuxPINStringRuleWouldActuallyFire(t *testing.T) {
	const src = `package ui

func leak(entry *thing, dst []byte) {
	pin := entry.Text()
	_ = pin
	_ = C.GoString(entry.raw)
	_ = string(dst)
}
`
	dir := t.TempDir()
	path := dir + "/leak.go"
	if err := writeFile(path, src); err != nil {
		t.Fatal(err)
	}
	fset, file := parseLinuxPINDialog(t, path)
	found := pinStringCalls(fset, file)
	if len(found) < 3 {
		t.Fatalf("the rule found %d of the three ways to leak a PIN in source that does all "+
			"three (%v), so it would have found nothing in source that did none", len(found), found)
	}
}

// TestTheDialogOverwritesTheEntryBeforeTheWindowGoes is SPEC §6.5.1
// clause 2's first exception, measured (D-350) and now guarded.
//
// GTK's copy of the typed characters is GTK's memory: this program
// cannot wipe it, but it can overwrite it through the widget's own
// interface, and it must do so *before* the window is destroyed rather
// than by destroying it. A file that had lost that call would still
// work, still pass every test about what the dialog returns, and leave
// the PIN in a locked page until GTK happened to reuse it.
func TestTheDialogOverwritesTheEntryBeforeTheWindowGoes(t *testing.T) {
	src, err := readFile(pinDialogLinuxFile)
	if err != nil {
		t.Fatal(err)
	}
	clear := strings.Index(src, "C.liro_pin_clear(")
	if clear < 0 {
		t.Fatal("the dialog never overwrites the entry through the widget's own interface")
	}
	destroy := strings.Index(src, "win.Destroy()")
	if destroy < 0 {
		t.Fatal("the dialog never destroys its window")
	}
	if clear > destroy {
		t.Error("the entry is overwritten after the window is destroyed, which relies on " +
			"destruction to do it — SPEC §6.5.1 clause 2's first exception says it must be " +
			"overwritten through the control's own interface instead")
	}
}

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}
