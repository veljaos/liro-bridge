//go:build !windows && !linux

package platform

import "fmt"

type unsupportedAutostart struct{}

// NewAutostart returns the stub Autostart backend for platforms without
// an implementation yet: macOS, which is F13 and deferred (SPEC §19).
// Linux has autostart_linux.go.
func NewAutostart() Autostart { return unsupportedAutostart{} }

func (unsupportedAutostart) IsEnabled() (bool, error) { return false, nil }

func (unsupportedAutostart) SetEnabled(bool, string) error {
	return fmt.Errorf("platform: autostart is not supported on this platform yet")
}

// EnsureAutostart is the startup reconcile autostart_linux.go explains.
// There is no entry to reconcile here.
func EnsureAutostart(bool, string) error {
	return fmt.Errorf("platform: autostart is not supported on this platform yet")
}
