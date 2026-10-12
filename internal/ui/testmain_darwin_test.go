//go:build darwin && cgo

package ui

import (
	"os"
	"testing"
)

// TestMain runs the tests beside AppKit's loop, as main runs the program
// (RunMain): a window can only be opened from the main thread's loop.
func TestMain(m *testing.M) { os.Exit(RunMain(m.Run)) }
