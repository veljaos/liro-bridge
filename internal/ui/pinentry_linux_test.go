//go:build linux

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
)

// The PIN field's own logic, driven by keyvals rather than by anybody's hands
// (D-094: calling this program's own handler forges nothing at the
// compositor). It replaced the GtkPasswordEntry and its three C helpers in
// D-385, because the entry told the accessibility bus what was typed.

func typeInto(t *testing.T, page []byte, n int, keyval uint, state gdk.ModifierType, want fieldAction) int {
	t.Helper()
	next, got := fieldKey(page, n, keyval, state)
	if got != want {
		t.Fatalf("keyval %d with state %v: action %v, want %v", keyval, state, got, want)
	}
	return next
}

// Characters arrive in the page as UTF-8 — Latin and Cyrillic, which is what
// a PIN typed on a Serbian keyboard can hold — and Backspace takes back a
// whole character and overwrites its bytes.
func TestTheFieldWritesUTF8AndBackspaceOverwritesAWholeCharacter(t *testing.T) {
	page := make([]byte, 16)
	n := 0
	n = typeInto(t, page, n, gdk.KEY_a, 0, fieldChanged)
	n = typeInto(t, page, n, gdk.KEY_Cyrillic_pe, 0, fieldChanged)
	if n != 3 || string(page[:n]) != "a\u043f" {
		t.Fatalf("after a and п the field holds %q (%d bytes), want %q", page[:n], n, "a\u043f")
	}
	n = typeInto(t, page, n, gdk.KEY_BackSpace, 0, fieldChanged)
	if n != 1 || page[1] != 0 || page[2] != 0 {
		t.Fatalf("Backspace left n=%d and bytes %v; want 1 and the two bytes of п overwritten", n, page[:3])
	}
	n = typeInto(t, page, n, gdk.KEY_BackSpace, 0, fieldChanged)
	if n != 0 || page[0] != 0 {
		t.Fatalf("Backspace left n=%d, byte %d", n, page[0])
	}
	if typeInto(t, page, 0, gdk.KEY_BackSpace, 0, fieldIgnored) != 0 {
		t.Fatal("Backspace on an empty field changed its length")
	}
}

// What is not the field's: a shortcut (Ctrl+V is not a way in — the clipboard
// is another exposure) and Tab, which must move focus. Enter and Escape answer.
func TestTheFieldRefusesShortcutsAndAnswersEnterAndEscape(t *testing.T) {
	page := make([]byte, 16)
	if n := typeInto(t, page, 0, gdk.KEY_v, gdk.ControlMask, fieldIgnored); n != 0 || page[0] != 0 {
		t.Fatalf("Ctrl+V wrote into the field: n=%d", n)
	}
	typeInto(t, page, 0, gdk.KEY_Tab, 0, fieldIgnored)
	typeInto(t, page, 0, gdk.KEY_Return, 0, fieldAccept)
	typeInto(t, page, 0, gdk.KEY_KP_Enter, 0, fieldAccept)
	typeInto(t, page, 0, gdk.KEY_Escape, 0, fieldCancel)
}

// A full page takes nothing more rather than writing past it.
func TestAFullFieldTakesNothingMore(t *testing.T) {
	page := make([]byte, 2)
	n := typeInto(t, page, 0, gdk.KEY_Cyrillic_pe, 0, fieldChanged)
	if n = typeInto(t, page, n, gdk.KEY_a, 0, fieldIgnored); n != 2 {
		t.Fatalf("a full field grew to %d", n)
	}
}

// The field says it has focus: a caret after the dots while it does, and only
// dots — never a character — in either state (D-385: the first field gave no
// sign of focus at all).
func TestTheFieldShowsACaretOnlyWhileFocused(t *testing.T) {
	if got := fieldDisplay(0, true); got != fieldCaret {
		t.Errorf("an empty focused field shows %q, want the caret alone", got)
	}
	if got := fieldDisplay(3, true); got != "\u25CF\u25CF\u25CF"+fieldCaret {
		t.Errorf("three characters, focused: %q", got)
	}
	if got := fieldDisplay(3, false); got != "\u25CF\u25CF\u25CF" {
		t.Errorf("three characters, not focused: %q — the caret must go with the focus", got)
	}
}

// The page is locked — read from the kernel, not from the call's return — and
// after the wipe it is zero, read back through its fixed address rather than
// through a slice header (SPEC §6.5.1 clause 2: a loop that was elided and a
// loop that ran look identical through the header).
func TestTheFieldsPageIsLockedAndTheWipeReachesIt(t *testing.T) {
	page, err := lockedPage()
	if err != nil {
		t.Fatal(err)
	}
	defer releasePage(page)
	at := (*[10]byte)(unsafe.Pointer(&page[0]))
	if kb := lockedKB(t, uintptr(unsafe.Pointer(at))); kb <= 0 {
		t.Errorf("the field's page is not locked: smaps says Locked %d kB for its mapping", kb)
	}
	copy(page, "0123456789")
	wipeField(page[:10])
	for i, b := range *at {
		if b != 0 {
			t.Fatalf("byte %d at the page's own address is %d after the wipe", i, b)
		}
	}
}

// lockedKB is the Locked size smaps reports for the mapping holding addr.
func lockedKB(t *testing.T, addr uintptr) int {
	t.Helper()
	b, err := os.ReadFile("/proc/self/smaps")
	if err != nil {
		t.Fatal(err)
	}
	inside := false
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 0 && strings.Contains(f[0], "-") && !strings.HasSuffix(f[0], ":") {
			bounds := strings.SplitN(f[0], "-", 2)
			lo, err1 := strconv.ParseUint(bounds[0], 16, 64)
			hi, err2 := strconv.ParseUint(bounds[1], 16, 64)
			inside = err1 == nil && err2 == nil && uintptr(lo) <= addr && addr < uintptr(hi)
			continue
		}
		if inside && len(f) >= 2 && f[0] == "Locked:" {
			kb, _ := strconv.Atoi(f[1])
			return kb
		}
	}
	t.Fatalf("no mapping in smaps holds %#x", addr)
	return 0
}

// TestNothingInThisPackageCallsWidgetNative forbids the expression itself.
//
// gotk4's Widget.Native() is gtk_widget_get_native(), which returns a Go
// wrapper for the toplevel; it is never the widget's own C pointer, and
// converted with unsafe.Pointer it compiles and passes go vet (D-355).
func TestNothingInThisPackageCallsWidgetNative(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		if found := widgetNativeCalls(t, f, nil); len(found) > 0 {
			t.Errorf("%s calls Widget.Native() at %s: that is the widget's toplevel as a Go wrapper, "+
				"not its C pointer; use the embedded Object's Native() (D-355)", f, strings.Join(found, ", "))
		}
	}
}

// TestTheWidgetNativeRuleWouldActuallyFire is the control.
func TestTheWidgetNativeRuleWouldActuallyFire(t *testing.T) {
	src := []byte("package ui\n\nfunc f(entry *thing) { _ = entry.Widget.Native() }\n")
	if found := widgetNativeCalls(t, "bad.go", src); len(found) != 1 {
		t.Fatalf("the rule found %d calls in source that makes one", len(found))
	}
}

func widgetNativeCalls(t *testing.T, path string, src []byte) []string {
	t.Helper()
	if src == nil {
		var err error
		if src, err = os.ReadFile(path); err != nil {
			t.Fatal(err)
		}
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Native" {
			return true
		}
		if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Widget" {
			found = append(found, fset.Position(call.Pos()).String())
		}
		return true
	})
	return found
}
