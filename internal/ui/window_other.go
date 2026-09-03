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
