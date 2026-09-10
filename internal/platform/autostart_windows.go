package platform

import (
	"golang.org/x/sys/windows/registry"
)

// runKeyPath is HKCU\Software\Microsoft\Windows\CurrentVersion\Run
// (F5 §3): per-user, no administrator rights required — HKLM is
// deliberately never used.
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// runValueName is the value name this agent registers itself under.
const runValueName = "LiroBridge"

// windowsAutostart writes one value under keyPath. keyPath is a field
// rather than a constant so a test can point it at a subkey that
// genuinely does not exist yet — which is the case this file gets wrong
// if SetEnabled only ever opens the key instead of creating it, and
// which cannot be exercised against the real Run key without deleting
// something that belongs to the user.
type windowsAutostart struct{ keyPath string }

// NewAutostart returns the Windows autostart backend, using
// golang.org/x/sys/windows/registry rather than hand-written registry
// syscalls — unlike the COM interop in internal/ui, there is a
// well-scoped, dependency-free standard-adjacent package for this
// (already part of the project's existing golang.org/x/sys dependency),
// so hand-writing it would add risk for no benefit (SPEC §8.6 prefers
// the smallest reasonable dependency, and this is already in the graph).
func NewAutostart() Autostart { return windowsAutostart{keyPath: runKeyPath} }

func (a windowsAutostart) IsEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, a.keyPath, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = k.Close() }()

	if _, _, err := k.GetStringValue(runValueName); err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a windowsAutostart) SetEnabled(enabled bool, exePath string) error {
	if !enabled {
		// Nothing to delete if the key is not there at all, which is
		// not an error: the requested state is already the actual one.
		k, err := registry.OpenKey(registry.CURRENT_USER, a.keyPath, registry.SET_VALUE)
		if err != nil {
			if err == registry.ErrNotExist {
				return nil
			}
			return err
		}
		defer func() { _ = k.Close() }()
		if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}

	// CreateKey, not OpenKey: Run is present in a normal user profile
	// but is not guaranteed to exist, and OpenKey on a missing key fails
	// with ERROR_FILE_NOT_FOUND — "The system cannot find the file
	// specified", which reads as a complaint about exePath and is not
	// one; the registry never looks at that string. That is exactly how
	// this surfaced (F6 §0c): a GitHub runner's fresh profile has no Run
	// key, and enabling autostart failed there with a message pointing
	// at the wrong thing. CreateKey opens an existing key unchanged and
	// creates it otherwise, which is the behaviour this always wanted.
	k, _, err := registry.CreateKey(registry.CURRENT_USER, a.keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return k.SetStringValue(runValueName, AutostartCommand(exePath))
}
