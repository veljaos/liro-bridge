//go:build !windows

package windowscng

import "fmt"

// unsupportedConn is the ncryptConn used on platforms other than
// Windows. CNG is a Windows-only API; macOS and Linux get their own key
// sources in phases 12 and 13 (SPEC §11.11). Enumerate already reports
// no certificates on these platforms (F1), so in practice nothing calls
// Open with a real thumbprint here — this exists only so the package
// (and anything built on top of it) cross-compiles.
type unsupportedConn struct{}

func newConn() ncryptConn { return unsupportedConn{} }

func (unsupportedConn) findAndAcquire(string) ([]byte, ncryptKeyHandle, bool, error) {
	return nil, 0, false, fmt.Errorf("windowscng: not supported on this platform")
}

func (unsupportedConn) setWindowHandle(ncryptKeyHandle, uintptr) error {
	return fmt.Errorf("windowscng: not supported on this platform")
}

func (unsupportedConn) signHash(ncryptKeyHandle, []byte) ([]byte, error) {
	return nil, fmt.Errorf("windowscng: not supported on this platform")
}

func (unsupportedConn) freeKey(ncryptKeyHandle) error { return nil }
