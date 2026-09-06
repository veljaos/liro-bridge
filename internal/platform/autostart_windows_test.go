package platform

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// TestWindowsAutostartRoundTrip exercises the real
// HKCU\...\Run key (F5 §3), scoped entirely to this project's own
// "LiroBridge" value name — never touching any other value already
// present there — and always leaves the key exactly as it found it.
//
// It skips, rather than fails, when the Run key does not exist and this
// process cannot create it: that is an environment with no user profile
// to write autostart into, and a test that fails for that reason
// teaches people to ignore failures (F6 §0c). The behaviour when the key
// is merely absent is covered by
// TestWindowsAutostartCreatesTheKeyWhenItDoesNotExist below, which does
// not depend on the environment at all.
func TestWindowsAutostartRoundTrip(t *testing.T) {
	if err := ensureRunKeyUsable(); err != nil {
		t.Skipf("no writable HKCU\\%s on this machine (%v); "+
			"autostart has nothing to register itself with here", runKeyPath, err)
	}

	a := NewAutostart()
	keepAutostartValue(t, runKeyPath)

	if err := a.SetEnabled(true, testExePath); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	enabled, err := a.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled (after enable): %v", err)
	}
	if !enabled {
		t.Fatal("IsEnabled() = false after SetEnabled(true)")
	}

	if err := a.SetEnabled(false, testExePath); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	enabled, err = a.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled (after disable): %v", err)
	}
	if enabled {
		t.Fatal("IsEnabled() = true after SetEnabled(false)")
	}
}

// keepAutostartValue snapshots this project's own Run value verbatim —
// whether it is there and, if so, what it points at — and puts exactly
// that back when the test ends.
//
// Restoring only *whether* the entry exists is not enough, and this is
// not hypothetical: this test used to record a bool from IsEnabled and
// restore it with SetEnabled(before, testExePath), so on any machine
// where autostart was on, one `go test ./...` rewrote the developer's
// real autostart entry to point at C:\test\liro-bridge.exe — a path
// that does not exist, leaving the agent silently not starting with
// Windows. D-134 recorded that defect and left it; this is the fix.
func keepAutostartValue(t *testing.T, keyPath string) {
	t.Helper()

	before, present := readRunValue(t, keyPath)
	t.Cleanup(func() {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
		if err != nil {
			t.Errorf("restoring HKCU\\%s: %v", keyPath, err)
			return
		}
		defer func() { _ = k.Close() }()
		if !present {
			if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
				t.Errorf("removing HKCU\\%s\\%s: %v", keyPath, runValueName, err)
			}
			return
		}
		if err := k.SetStringValue(runValueName, before); err != nil {
			t.Errorf("restoring HKCU\\%s\\%s: %v", keyPath, runValueName, err)
		}
	})
}

func readRunValue(t *testing.T, keyPath string) (string, bool) {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer func() { _ = k.Close() }()
	s, _, err := k.GetStringValue(runValueName)
	if err != nil {
		return "", false
	}
	return s, true
}

// TestAutostartTestsPutBackWhatTheyFound is the regression test for the
// defect above, exercised against a scratch key so it can seed a value
// of its own rather than depending on what this machine happens to hold.
//
// It fails against the previous shape of TestWindowsAutostartRoundTrip:
// recording only IsEnabled()'s bool and restoring with
// SetEnabled(before, testExePath) leaves the *seeded* value replaced by
// testExePath, which is what actually happened on this machine.
func TestAutostartTestsPutBackWhatTheyFound(t *testing.T) {
	deleteScratchKey(t)
	t.Cleanup(func() { deleteScratchKey(t) })

	const ownersValue = `"C:\Users\Somebody\Desktop\liro-bridge\liro-bridge.exe"`
	k, _, err := registry.CreateKey(registry.CURRENT_USER, scratchKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("creating scratch key: %v", err)
	}
	if err := k.SetStringValue(runValueName, ownersValue); err != nil {
		t.Fatalf("seeding the value: %v", err)
	}
	_ = k.Close()

	// A nested test is the only way to observe what a t.Cleanup actually
	// restores: cleanups run when that test ends, not when this one does.
	inner := func(t *testing.T) {
		keepAutostartValue(t, scratchKeyPath)
		a := windowsAutostart{keyPath: scratchKeyPath}
		if err := a.SetEnabled(true, testExePath); err != nil {
			t.Fatalf("SetEnabled(true): %v", err)
		}
		if err := a.SetEnabled(false, testExePath); err != nil {
			t.Fatalf("SetEnabled(false): %v", err)
		}
	}
	if !t.Run("round trip", inner) {
		t.Fatal("the inner test failed; the restoration check below means nothing")
	}

	got, present := readRunValue(t, scratchKeyPath)
	if !present {
		t.Fatalf("the value is gone: a test that found autostart enabled left it disabled")
	}
	if got != ownersValue {
		t.Fatalf("autostart value = %q after a test run, want it untouched at %q", got, ownersValue)
	}
}

// TestAutostartTestsDoNotInventAnEntry is the other direction: a machine
// with no autostart entry must not have one after a test run.
func TestAutostartTestsDoNotInventAnEntry(t *testing.T) {
	deleteScratchKey(t)
	t.Cleanup(func() { deleteScratchKey(t) })

	inner := func(t *testing.T) {
		keepAutostartValue(t, scratchKeyPath)
		a := windowsAutostart{keyPath: scratchKeyPath}
		if err := a.SetEnabled(true, testExePath); err != nil {
			t.Fatalf("SetEnabled(true): %v", err)
		}
	}
	if !t.Run("enable only", inner) {
		t.Fatal("the inner test failed; the restoration check below means nothing")
	}

	if got, present := readRunValue(t, scratchKeyPath); present {
		t.Fatalf("a test left an autostart entry behind, pointing at %q", got)
	}
}

// testExePath is a path that deliberately does not exist. The registry
// stores this string verbatim and never resolves it, which is the point:
// if a future change ever makes autostart validate the path, this test
// is what says so.
const testExePath = `C:\test\liro-bridge.exe`

// scratchKeyPath is a subkey of these tests' own making, under HKCU,
// that no installation ever creates — deliberately not under a name any
// real feature might one day keep configuration in, since
// deleteScratchKey removes its parent too. Every test below deletes it.
const (
	scratchRootPath = `Software\LiroBridgeTestScratch`
	scratchKeyPath  = scratchRootPath + `\Run`
)

// TestWindowsAutostartCreatesTheKeyWhenItDoesNotExist is the regression
// test for F6 §0c. The reported failure was
//
//	SetEnabled(true): The system cannot find the file specified.
//
// on the GitHub runner, which reads as a complaint about the executable
// path — and is not one. It is ERROR_FILE_NOT_FOUND from opening a
// registry key that is not there: a fresh runner profile has no
// HKCU\...\Run. Nothing here resolves testExePath, and this test proves
// that by succeeding with a path that does not exist.
func TestWindowsAutostartCreatesTheKeyWhenItDoesNotExist(t *testing.T) {
	deleteScratchKey(t)
	t.Cleanup(func() { deleteScratchKey(t) })

	a := windowsAutostart{keyPath: scratchKeyPath}

	// Precondition: the key really is absent, so this test is exercising
	// the reported situation and not passing for an unrelated reason.
	if _, err := registry.OpenKey(registry.CURRENT_USER, scratchKeyPath, registry.QUERY_VALUE); err != registry.ErrNotExist {
		t.Fatalf("scratch key was not absent before the test: %v", err)
	}
	enabled, err := a.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled with no key: %v", err)
	}
	if enabled {
		t.Fatal("IsEnabled() = true with no registry key at all")
	}

	if err := a.SetEnabled(true, testExePath); err != nil {
		t.Fatalf("SetEnabled(true) with no key present: %v", err)
	}
	enabled, err = a.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled after enable: %v", err)
	}
	if !enabled {
		t.Fatal("IsEnabled() = false after SetEnabled(true) created the key")
	}
}

// TestWindowsAutostartDisableWithNoKeyIsNotAnError covers the other half
// of the same situation: asking for the state the machine is already in
// is not a failure to report to the user.
func TestWindowsAutostartDisableWithNoKeyIsNotAnError(t *testing.T) {
	deleteScratchKey(t)
	t.Cleanup(func() { deleteScratchKey(t) })

	a := windowsAutostart{keyPath: scratchKeyPath}
	if err := a.SetEnabled(false, testExePath); err != nil {
		t.Fatalf("SetEnabled(false) with no key present: %v", err)
	}
}

// TestWindowsAutostartLeavesNeighbouringValuesAlone pins the property the
// round-trip test's comment claims but never checked: this code writes
// and deletes exactly one value name and never disturbs anything beside
// it. That matters because the round-trip test runs against the user's
// real Run key, where a neighbour is somebody else's startup entry.
func TestWindowsAutostartLeavesNeighbouringValuesAlone(t *testing.T) {
	deleteScratchKey(t)
	t.Cleanup(func() { deleteScratchKey(t) })

	const neighbour = "SomebodyElsesApp"
	const neighbourValue = `"C:\Windows\notepad.exe"`

	k, _, err := registry.CreateKey(registry.CURRENT_USER, scratchKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("creating scratch key: %v", err)
	}
	if err := k.SetStringValue(neighbour, neighbourValue); err != nil {
		t.Fatalf("seeding neighbouring value: %v", err)
	}
	_ = k.Close()

	a := windowsAutostart{keyPath: scratchKeyPath}
	if err := a.SetEnabled(true, testExePath); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if err := a.SetEnabled(false, testExePath); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}

	k, err = registry.OpenKey(registry.CURRENT_USER, scratchKeyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("reopening scratch key: %v", err)
	}
	defer func() { _ = k.Close() }()
	got, _, err := k.GetStringValue(neighbour)
	if err != nil {
		t.Fatalf("the neighbouring value did not survive: %v", err)
	}
	if got != neighbourValue {
		t.Fatalf("neighbouring value = %q, want it untouched", got)
	}
}

// ensureRunKeyUsable reports whether the real Run key can be opened for
// writing, creating it if this profile has none. It is only a
// precondition check: it adds no value and deletes nothing.
func ensureRunKeyUsable() error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	return k.Close()
}

func deleteScratchKey(t *testing.T) {
	t.Helper()
	if err := registry.DeleteKey(registry.CURRENT_USER, scratchKeyPath); err != nil && err != registry.ErrNotExist {
		t.Fatalf("deleting scratch key: %v", err)
	}
	// The leaf's parent exists only because these tests made it; leaving
	// it behind would litter the user's own registry a little more with
	// every run. DeleteKey refuses a key that still has subkeys, so this
	// cannot remove anything that is still in use.
	if err := registry.DeleteKey(registry.CURRENT_USER, scratchRootPath); err != nil && err != registry.ErrNotExist {
		t.Logf("leaving HKCU\\%s behind: %v", scratchRootPath, err)
	}
}
