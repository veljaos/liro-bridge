//go:build !windows

package ui

// NewWindow is unsupported outside Windows (see ErrUnsupportedPlatform).
// This exists only so the package — and anything built on top of it —
// cross-compiles, mirroring
// internal/keysource/windowscng/conn_other.go's unsupportedConn.
func NewWindow(Options) (Window, error) {
	return nil, ErrUnsupportedPlatform
}

func detectRuntime() (bool, string, error) {
	return false, "", ErrUnsupportedPlatform
}

func showRuntimeMissingMessage(string, string) {}

func pickFolder(uintptr, string) (string, bool, error) {
	return "", false, ErrUnsupportedPlatform
}

// pickFiles has no other-platform implementation this phase either. It
// reports ErrUnsupportedPlatform rather than pretending the user
// cancelled, which would leave a caller silently doing nothing.
func pickFiles(uintptr, string, string, string) ([]string, bool, error) {
	return nil, false, ErrUnsupportedPlatform
}
