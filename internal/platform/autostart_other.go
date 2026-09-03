//go:build !windows

package platform

import "fmt"

type unsupportedAutostart struct{}

// NewAutostart returns the stub Autostart backend for platforms without
// an implementation yet (macOS/Linux arrive in phases 12/13).
func NewAutostart() Autostart { return unsupportedAutostart{} }

func (unsupportedAutostart) IsEnabled() (bool, error) { return false, nil }

func (unsupportedAutostart) SetEnabled(bool, string) error {
	return fmt.Errorf("platform: autostart is not supported on this platform yet")
}
