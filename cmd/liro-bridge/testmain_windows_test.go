//go:build windows

package main

// TestMain closes the shared windows (sharedwindow_windows_test.go)
// after the last test that borrowed one has finished — they outlive
// every individual test by design, so no single test can own their
// teardown.
//
// It also sweeps the temporary config homes an earlier run could not
// delete, before and after. A WebView2 browser process group holds
// files under one of those directories open after the window that made
// it has closed, and was measured still running two hours after the
// test binary that started it had exited — so `go test ./...` left
// about six megabytes in %TEMP% every time it ran, and 196 runs had
// left 1.26 GB (FTEST Group 3, C-6). Sweeping is what makes that stop
// accumulating rather than accumulate more slowly.
import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	sweepStaleConfigHomes()
	code := m.Run()
	closeSharedPlacementWindow()
	closeSharedWindows()
	sweepStaleConfigHomes()
	os.Exit(code)
}
