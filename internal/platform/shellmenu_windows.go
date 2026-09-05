//go:build windows

package platform

// The Explorer context-menu entry on .pdf files (F6 §2).
//
// Registered under HKCU\Software\Classes\SystemFileAssociations\.pdf\
// shell\, which is per-user and needs no administrator rights — the
// same constraint the installer works under (SPEC §15). HKCR and HKLM
// are deliberately never touched: a per-machine entry would need
// elevation, and writing one from a per-user application is how a
// signing agent ends up in a place its own uninstaller cannot reach.
//
// SystemFileAssociations rather than the .pdf progid: a progid belongs
// to whichever application currently owns PDFs, and adding a verb there
// means the entry disappears the day someone installs a different
// reader. SystemFileAssociations is keyed on the extension itself and
// survives that.

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// shellMenuKeyPath is where the verb lives. The final segment is this
// project's own verb name, which is what makes the entry findable and
// removable without touching anything else under shell\.
const (
	shellMenuParentPath = `Software\Classes\SystemFileAssociations\.pdf\shell`
	shellMenuVerb       = "LiroBridgeSign"
	shellMenuKeyPath    = shellMenuParentPath + `\` + shellMenuVerb
	shellMenuCommandKey = shellMenuKeyPath + `\command`
)

// windowsShellMenu implements ShellMenu against the real registry.
type windowsShellMenu struct{ keyPath string }

// NewShellMenu returns the Windows context-menu registration.
func NewShellMenu() ShellMenu { return windowsShellMenu{keyPath: shellMenuKeyPath} }

func (s windowsShellMenu) commandPath() string { return s.keyPath + `\command` }

// IsRegistered reports whether the entry is present.
func (s windowsShellMenu) IsRegistered() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, s.commandPath(), registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue("")
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	return v != "", nil
}

// Register writes the entry. label is the already-localised menu text
// (F6 §2 fixes the wording); exePath is this executable, and iconPath
// is where the menu icon comes from — normally the executable itself,
// whose first icon resource Explorer uses.
//
// Registering twice is not an error: the values are simply rewritten,
// which is also how a label changes when the user switches language.
func (s windowsShellMenu) Register(label, exePath, iconPath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, s.keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("creating the shell menu key: %w", err)
	}
	defer func() { _ = k.Close() }()

	// The default value is the menu text Explorer shows.
	if err := k.SetStringValue("", label); err != nil {
		return fmt.Errorf("writing the menu label: %w", err)
	}
	// "Icon" is what puts the Liro mark beside the entry (F6 §2).
	if iconPath != "" {
		if err := k.SetStringValue("Icon", iconPath); err != nil {
			return fmt.Errorf("writing the menu icon: %w", err)
		}
	}
	// MultiSelectModel=Player tells Explorer this verb can be invoked
	// for a multiple selection at all. Without it the entry disappears
	// from the menu the moment more than one file is selected, which is
	// exactly the case F6 §2 asks to work.
	if err := k.SetStringValue("MultiSelectModel", "Player"); err != nil {
		return fmt.Errorf("writing MultiSelectModel: %w", err)
	}

	ck, _, err := registry.CreateKey(registry.CURRENT_USER, s.commandPath(), registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("creating the shell menu command key: %w", err)
	}
	defer func() { _ = ck.Close() }()

	if err := ck.SetStringValue("", ShellMenuCommand(exePath)); err != nil {
		return fmt.Errorf("writing the menu command: %w", err)
	}
	return nil
}

// Unregister removes the entry and both keys it owns, leaving the rest
// of shell\ alone. F6 §2: "removed cleanly when switched off" — an
// entry that only stops working, but stays in the menu, is worse than
// one that was never there.
func (s windowsShellMenu) Unregister() error {
	// The command subkey first: a key with subkeys cannot be deleted.
	if err := registry.DeleteKey(registry.CURRENT_USER, s.commandPath()); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("removing the shell menu command key: %w", err)
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, s.keyPath); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("removing the shell menu key: %w", err)
	}
	return nil
}
