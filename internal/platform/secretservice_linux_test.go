//go:build linux

package platform

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

// TestNothingRaisesASecretServicePrompt is the guard the whole
// locked-keyring design rests on.
//
// Measured on GNOME 46 (D-347): every Secret Service call this program
// makes against a *locked* collection answers without showing anything
// — SearchItems returns the item in the locked list, GetSecret and
// CreateItem return errors, and Unlock returns a prompt object and
// leaves the collection locked. **A dialog appears only when
// Prompt.Prompt() is called.** The case that matters is an agent
// started at login, which asks before the person has unlocked
// anything; a modal password dialog in front of a desktop that has
// just appeared is not something this program may cause.
//
// So the property is not "we handle prompts well", it is "this program
// contains no call that can raise one", and that is a property of the
// source rather than of a run. It is checked over the AST rather than
// by grepping for a string, so that a comment mentioning Prompt cannot
// satisfy it and a renamed constant cannot hide it.
func TestNothingRaisesASecretServicePrompt(t *testing.T) {
	files := parsePackage(t, ".", true)

	// The method name on org.freedesktop.Secret.Prompt that shows a
	// dialog. Dismiss and the Completed signal are the two that do not.
	const forbidden = ".Prompt"

	var found []string
	for name, file := range files {
		for _, member := range dbusCallsIn(file) {
			if strings.HasSuffix(member, forbidden) {
				found = append(found, filepath.Base(name)+": "+member)
			}
		}
	}
	if len(files) == 0 {
		t.Fatal("no files were parsed, so this guard asserted nothing")
	}
	if len(found) != 0 {
		t.Errorf("this package calls a D-Bus method that can raise a keyring dialog: %v\n"+
			"An autostarted agent must never ask a person for a password before they have asked "+
			"for anything (F12 §7, D-347). A locked keyring falls back to the file store.", found)
	}
}

// parsePackage parses every .go file in dir, by name rather than
// through go/parser's ParseDir — which is deprecated, and which decides
// for itself which files belong to which package. This guard wants
// every file in the directory regardless of build tag: a call that can
// raise a dialog matters whichever platform it is compiled for.
func parsePackage(t *testing.T, dir string, skipTests bool) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if skipTests && strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		out[name] = f
	}
	return out
}

// dbusCallsIn returns the "interface.Member" string of every D-Bus
// method this file calls.
func dbusCallsIn(file *ast.File) []string {
	var members []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Call" && sel.Sel.Name != "CallWithContext") {
			return true
		}
		for _, arg := range call.Args {
			if member, ok := dbusMemberOf(arg); ok {
				members = append(members, member)
			}
		}
		return true
	})
	return members
}

// dbusMemberOf recovers the "interface.Member" string from a call
// argument, whether it was written as a literal or built by
// concatenating a constant with a literal — which is how every call in
// this package spells it.
func dbusMemberOf(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strings.Trim(v.Value, `"`), true
		}
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		left, lok := dbusMemberOf(v.X)
		right, rok := dbusMemberOf(v.Y)
		if !lok && !rok {
			return "", false
		}
		return left + right, true
	case *ast.Ident:
		// A bare constant identifier: this package spells the
		// interface as a constant and the member as a literal, so the
		// literal half is what carries ".Prompt" and the identifier
		// contributes nothing to match on.
		return "", false
	}
	return "", false
}

// TestTheGuardCanSeeAPrompt is the control for the test above.
//
// A guard that scans for something and finds nothing is indistinguishable
// from a guard that cannot scan (D-304's second question), so this one
// is pointed at source that does contain the call and must find it.
func TestTheGuardCanSeeAPrompt(t *testing.T) {
	dir := t.TempDir()
	const src = `package probe

func run(conn busish) {
	conn.Object("d", "p").Call(ssPrmpt+".Prompt", 0, "")
}
`
	if err := os.WriteFile(filepath.Join(dir, "probe.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	hits := 0
	for _, file := range parsePackage(t, dir, false) {
		for _, member := range dbusCallsIn(file) {
			if strings.HasSuffix(member, ".Prompt") {
				hits++
			}
		}
	}
	if hits == 0 {
		t.Fatal("the prompt guard found nothing in source that calls Prompt.Prompt, so it would " +
			"have found nothing in source that did not either")
	}
}

// TestNoSessionBusFallsBackWithAReason covers the branch every minimal
// desktop takes, and the one an agent takes when it is started outside
// a graphical session.
//
// SPEC §6.4 requires "a clear warning in the log" for this branch, and
// F12 §7 asks additionally that a person be able to see which store
// they got. Both need the store to carry *why*, so that is what is
// asserted: a fallback with an empty reason is a fallback nobody can
// act on.
func TestNoSessionBusFallsBackWithAReason(t *testing.T) {
	restore := ssSessionBus
	ssSessionBus = func() (*dbus.Conn, error) { return nil, errors.New("no bus here") }
	t.Cleanup(func() { ssSessionBus = restore })

	store, err := NewSecretStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSecretStore: %v", err)
	}
	d := store.Describe()
	if d.Mechanism != MechanismEncryptedFile {
		t.Errorf("with no session bus the store is %q, want %q", d.Mechanism, MechanismEncryptedFile)
	}
	if !d.Fallback {
		t.Error("the file branch does not report itself as a fallback, so nothing downstream can warn about it")
	}
	if d.Reason == "" {
		t.Error("the fallback carries no reason, so the log line and the settings window have nothing to say")
	}

	// And it is a working store, not a refusal: F12 §7 requires that a
	// missing or refused Secret Service fall back cleanly rather than
	// stop the agent starting.
	want := []byte("a device secret, 32 bytes of it!")
	if err := store.Set("device", want); err != nil {
		t.Fatalf("the fallback store will not Set: %v", err)
	}
	if got, err := store.Get("device"); err != nil || string(got) != string(want) {
		t.Fatalf("the fallback store round trip gave %q, %v", got, err)
	}
}

// TestTheStoredItemIsFindableByItsAttributes pins the attribute map,
// which is the Secret Service's only notion of a key. A change to any
// of the three makes every already-stored secret unfindable while
// every test that stores and reads back in one process still passes.
func TestTheStoredItemIsFindableByItsAttributes(t *testing.T) {
	a := ssAttributes("app-1234")
	if a[ssAttrName] != "app-1234" {
		t.Errorf("the name attribute is %q", a[ssAttrName])
	}
	if a[ssAttrApp] != ssAppValue {
		t.Errorf("the application attribute is %q, want %q", a[ssAttrApp], ssAppValue)
	}
	if a[ssAttrSchema] != ssSchemaValue {
		t.Errorf("the schema attribute is %q, want %q", a[ssAttrSchema], ssSchemaValue)
	}
	if len(a) != 3 {
		t.Errorf("the attribute map has %d entries, want 3: an extra one is also a key, and every "+
			"secret stored before it was added becomes unfindable", len(a))
	}
	if b := ssAttributes("app-5678"); b[ssAttrName] == a[ssAttrName] {
		t.Error("two different names produced the same attributes, so one secret would overwrite the other")
	}
}

// TestTheLabelNamesTheProgram: a person looking at Seahorse or
// KWalletManager sees a list of items, and one labelled with nothing
// but an identifier is one nobody can decide about.
func TestTheLabelNamesTheProgram(t *testing.T) {
	if !strings.Contains(ssLabel("app-1234"), "Liro Bridge") {
		t.Errorf("the keyring label %q does not name this program", ssLabel("app-1234"))
	}
	if !strings.Contains(ssLabel("app-1234"), "app-1234") {
		t.Errorf("the keyring label %q does not name the pairing", ssLabel("app-1234"))
	}
}

// TestTheRealSecretServiceRoundTrip is the only test here that talks to
// a keyring, and it is the reason the branch can be believed at all.
//
// **It skips where there is no Secret Service and says so**, which is
// every CI runner this project has: headless, no session bus, nothing
// owning org.freedesktop.secrets. That makes it a test that mostly does
// not run, which is [[D-221]]'s shape and is accepted here with its
// eyes open — the alternative is a D-Bus client whose every assertion
// is about a fake, on a branch whose entire risk is what a real daemon
// does. The skip names the reason rather than passing silently.
//
// It writes into whatever collection the desktop calls default, which
// on a developer's machine is their own login keyring, so it uses a
// name nobody could mistake for real and deletes it again — and checks
// that the delete worked, because a test that litters somebody's
// keyring is worse than one that does not run.
func TestTheRealSecretServiceRoundTrip(t *testing.T) {
	store, err := newSecretServiceStore()
	if err != nil {
		t.Skipf("no usable Secret Service here, so this branch is untested on this machine: %v", err)
	}

	const name = "liro-bridge-self-test-delete-me"
	want := []byte("a device secret, 32 bytes of it!")
	t.Cleanup(func() {
		if err := store.Delete(name); err != nil {
			t.Errorf("cleaning up: %v", err)
		}
	})

	if _, err := store.Get(name); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("before anything was stored, Get returned %v, want ErrSecretNotFound", err)
	}
	if err := store.Set(name, want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get(name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Get returned %q, want %q", got, want)
	}

	// Replacing rather than accumulating: CreateItem is called with
	// replace=true, and a store that appended would leave two items
	// with the same attributes and hand back whichever the service
	// listed first.
	second := []byte("a different device secret......!")
	if err := store.Set(name, second); err != nil {
		t.Fatalf("Set again: %v", err)
	}
	unlocked, locked, err := store.search(t.Context(), name)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if n := len(unlocked) + len(locked); n != 1 {
		t.Errorf("after storing twice under one name the keyring holds %d items, want 1", n)
	}
	if got, err := store.Get(name); err != nil || string(got) != string(second) {
		t.Fatalf("after replacing, Get returned %q, %v", got, err)
	}

	if err := store.Delete(name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(name); !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("after Delete, Get returned %v, want ErrSecretNotFound", err)
	}

	// And the description a person is shown is the one that was used.
	if d := store.Describe(); d.Mechanism != MechanismSecretService || d.Fallback {
		t.Errorf("a working keyring describes itself as %+v", d)
	}
}
