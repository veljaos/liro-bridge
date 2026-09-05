//go:build windows

package platform

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// scratchShellMenuPath is a key of this test's own making, so nothing
// here touches the real Explorer registration on the developer's
// machine — right-clicking a PDF during a test run must not change what
// the owner sees.
const scratchShellMenuRoot = `Software\LiroBridgeTestScratch\Shell`

func newScratchShellMenu(t *testing.T) windowsShellMenu {
	t.Helper()
	m := windowsShellMenu{keyPath: scratchShellMenuRoot + `\LiroBridgeSign`}
	cleanup := func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, m.commandPath())
		_ = registry.DeleteKey(registry.CURRENT_USER, m.keyPath)
		_ = registry.DeleteKey(registry.CURRENT_USER, scratchShellMenuRoot)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\LiroBridgeTestScratch`)
	}
	cleanup()
	t.Cleanup(cleanup)
	return m
}

// TestShellMenuRoundTrip is F6 §2: registered, present, removed
// cleanly. "Cleanly" is the half worth testing — an entry that stops
// working but stays in the menu is worse than one that was never there.
func TestShellMenuRoundTrip(t *testing.T) {
	m := newScratchShellMenu(t)

	if on, err := m.IsRegistered(); err != nil || on {
		t.Fatalf("IsRegistered before registering = %t (err %v), want false", on, err)
	}

	const label = "Potpiši koristeći Liro Bridge"
	const exe = `C:\Users\Test\AppData\Local\Liro\liro-bridge.exe`
	if err := m.Register(label, exe, exe); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if on, err := m.IsRegistered(); err != nil || !on {
		t.Fatalf("IsRegistered after registering = %t (err %v), want true", on, err)
	}

	// Registering twice is how the label follows a language change.
	if err := m.Register("Sign with Liro Bridge", exe, exe); err != nil {
		t.Fatalf("Register (second time): %v", err)
	}

	if err := m.Unregister(); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if on, err := m.IsRegistered(); err != nil || on {
		t.Fatalf("IsRegistered after unregistering = %t (err %v), want false", on, err)
	}
	// Both keys are gone, not just the value: an entry left behind with
	// no command is still an entry in the menu.
	for _, path := range []string{m.commandPath(), m.keyPath} {
		if _, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE); err != registry.ErrNotExist {
			t.Fatalf("%s still exists after Unregister (err %v)", path, err)
		}
	}
	// And unregistering again is not an error.
	if err := m.Unregister(); err != nil {
		t.Fatalf("Unregister (second time): %v", err)
	}
}

// TestShellMenuWritesWhatExplorerNeeds pins the three values that make
// the entry appear, carry an icon, and survive a multiple selection.
func TestShellMenuWritesWhatExplorerNeeds(t *testing.T) {
	m := newScratchShellMenu(t)
	const label = "Potpiši koristeći Liro Bridge"
	const exe = `C:\Program Files\Liro\liro-bridge.exe`
	if err := m.Register(label, exe, exe); err != nil {
		t.Fatalf("Register: %v", err)
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, m.keyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("opening the verb key: %v", err)
	}
	defer func() { _ = k.Close() }()

	if got, _, err := k.GetStringValue(""); err != nil || got != label {
		t.Fatalf("menu label = %q (err %v), want %q", got, err, label)
	}
	if got, _, err := k.GetStringValue("Icon"); err != nil || got != exe {
		t.Fatalf("Icon = %q (err %v), want the executable", got, err)
	}
	// Without MultiSelectModel the entry vanishes from the menu the
	// moment more than one file is selected — which is the case F6 §2
	// exists for.
	if got, _, err := k.GetStringValue("MultiSelectModel"); err != nil || got != "Player" {
		t.Fatalf("MultiSelectModel = %q (err %v), want Player", got, err)
	}

	ck, err := registry.OpenKey(registry.CURRENT_USER, m.commandPath(), registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("opening the command key: %v", err)
	}
	defer func() { _ = ck.Close() }()
	cmd, _, err := ck.GetStringValue("")
	if err != nil {
		t.Fatalf("reading the command: %v", err)
	}
	if cmd != ShellMenuCommand(exe) {
		t.Fatalf("command = %q, want %q", cmd, ShellMenuCommand(exe))
	}
}

// TestShellMenuCommandQuotesEverything is why the command is built
// rather than concatenated at the call site. A per-user install lives
// under a profile directory named after the user, and Serbian document
// names contain spaces constantly; an unquoted %1 hands the agent half
// a name.
func TestShellMenuCommandQuotesEverything(t *testing.T) {
	got := ShellMenuCommand(`C:\Users\Petar Petrović\AppData\Local\Liro\liro-bridge.exe`)
	if !strings.HasPrefix(got, `"C:\Users\Petar Petrović\AppData\Local\Liro\liro-bridge.exe"`) {
		t.Fatalf("the executable path is not quoted: %q", got)
	}
	if !strings.HasSuffix(got, `"%1"`) {
		t.Fatalf("the file argument is not quoted: %q", got)
	}
	if !strings.Contains(got, ShellMenuVerbFlag) {
		t.Fatalf("the command does not carry %s, so an Explorer invocation is not recognisable as one: %q",
			ShellMenuVerbFlag, got)
	}
}

// TestShellMenuIsRegisteredUnderHKCU is SPEC §15's per-user, no
// administrator rights constraint, stated as a test rather than only in
// a comment: HKLM or HKCR would need elevation this program does not
// have and should not ask for.
func TestShellMenuIsRegisteredUnderHKCU(t *testing.T) {
	if !strings.HasPrefix(shellMenuKeyPath, `Software\Classes\SystemFileAssociations\.pdf\shell\`) {
		t.Fatalf("the menu key path is %q; F6 §2 names HKCU\\Software\\Classes\\SystemFileAssociations\\.pdf\\shell\\",
			shellMenuKeyPath)
	}
	if strings.Contains(shellMenuKeyPath, "HKEY_") || strings.HasPrefix(shellMenuKeyPath, `\`) {
		t.Fatalf("the menu key path names a hive: %q — NewShellMenu opens it under CURRENT_USER", shellMenuKeyPath)
	}
}
