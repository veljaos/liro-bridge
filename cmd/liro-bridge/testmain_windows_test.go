//go:build windows

package main

// TestMain closes the shared windows (sharedwindow_windows_test.go)
// after the last test that borrowed one has finished — they outlive
// every individual test by design, so no single test can own their
// teardown.
import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	closeSharedPlacementWindow()
	closeSharedWindows()
	os.Exit(code)
}
