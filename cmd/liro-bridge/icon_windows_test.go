//go:build windows

package main

// Task 4 (F5 second-real-run review): every window must carry the real
// Liro mark in its title bar (ICON_SMALL) and in Alt+Tab (ICON_BIG).
// The check reads the icons back out of the live window with
// WM_GETICON rather than asserting internal/ui called SetIcon — a
// LoadImageW failure (a missing or unreadable icon file) returns 0 and
// is deliberately silent, so "the call happened" proves nothing about
// what the user actually sees.
//
// It lives here, not in internal/ui, for the same reason every other
// window test does: WebView2 will not open a second instance of one
// user data folder from another process, so two packages opening
// windows under "go test ./..." — which runs packages in parallel —
// deadlock on each other.
import (
	"testing"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
)

const (
	wmGetIcon = 0x007F

	iconSmall = 0
	iconBig   = 1
)

var procSendMessageWForTest = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageW")

func TestWindowsCarryTheLiroIcon(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := sharedSettingsWindow(t, c, config.Default())

	for _, tc := range []struct {
		name string
		kind uintptr
	}{
		{"ICON_SMALL (title bar)", iconSmall},
		{"ICON_BIG (Alt+Tab)", iconBig},
	} {
		h, _, _ := procSendMessageWForTest.Call(win.Handle(), wmGetIcon, tc.kind, 0)
		if h == 0 {
			t.Errorf("WM_GETICON %s returned 0 — the window still shows the Windows placeholder", tc.name)
		}
	}
}
