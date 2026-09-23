//go:build linux

package ui

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// TestTheDialogsCCallsWorkOnAnEntryInsideAWindow runs the three C calls the
// PIN dialog makes, on a real GtkPasswordEntry inside a real GtkWindow.
//
// **This is the test that did not exist, and D-355 is what that cost.**
// Every earlier check of this dialog read its source; none executed the
// handlers, because only a person can press its buttons (D-094). So the
// first time anybody pressed OK the agent panicked: the dialog passed C a
// Go pointer where it meant the entry.
//
// Inside a window is the condition that matters. Widget.Native() — the
// expression that was wrong — returns nil for an entry with no toplevel,
// and a Go wrapper for one inside a window; the precondition below asserts
// the second, so that this test is run under the state the defect needed.
//
// The entry's content is set through GTK's own API. Nothing is clicked,
// typed or activated.
func TestTheDialogsCCallsWorkOnAnEntryInsideAWindow(t *testing.T) {
	const value = "123456"
	var (
		wrapperIsLive           bool
		length, copied, cleared int
		dst                     = make([]byte, 32)
	)
	err := theUIThread.do(func() {
		win := gtk.NewWindow()
		entry := gtk.NewPasswordEntry()
		win.SetChild(entry)
		wrapperIsLive = entry.Widget.Native() != nil

		entry.SetText(value)
		length = pinLen(entry)
		copied = pinCopy(entry, dst, len(dst))
		pinClear(entry)
		cleared = pinLen(entry)
		win.Destroy()
	})
	if errors.Is(err, ErrNoDisplay) {
		t.Skip("no windowing system: ", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !wrapperIsLive {
		t.Fatal("precondition: inside a window, Widget.Native() should be a live wrapper, and this test " +
			"exists to run under that state")
	}
	if length != len(value) {
		t.Errorf("pinLen = %d, want %d", length, len(value))
	}
	if copied != len(value) || string(dst[:copied]) != value {
		t.Errorf("pinCopy wrote %d bytes %q, want %q", copied, dst[:copied], value)
	}
	if cleared != 0 {
		t.Errorf("after pinClear the entry still holds %d bytes", cleared)
	}
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
