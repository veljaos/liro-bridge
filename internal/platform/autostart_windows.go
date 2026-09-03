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

type windowsAutostart struct{}

// NewAutostart returns the Windows autostart backend, using
// golang.org/x/sys/windows/registry rather than hand-written registry
// syscalls — unlike the COM interop in internal/ui, there is a
// well-scoped, dependency-free standard-adjacent package for this
// (already part of the project's existing golang.org/x/sys dependency),
// so hand-writing it would add risk for no benefit (SPEC §8.6 prefers
// the smallest reasonable dependency, and this is already in the graph).
func NewAutostart() Autostart { return windowsAutostart{} }

func (windowsAutostart) IsEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
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

func (windowsAutostart) SetEnabled(enabled bool, exePath string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()

	if !enabled {
		if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	return k.SetStringValue(runValueName, `"`+exePath+`"`)
}
