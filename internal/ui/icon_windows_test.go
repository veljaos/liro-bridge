//go:build windows

package ui

// Task 4 (F5 second-real-run review): the icon-size arithmetic behind
// setWindowIcons. The end-to-end check — that a real window's title bar
// and Alt+Tab entry actually carry the mark — lives in
// cmd/liro-bridge (TestWindowsCarryTheLiroIcon): every test in this
// project that creates a WebView2 window has to live in one package,
// because they all share one user data folder and WebView2 refuses to
// open a second one against it from another process, which is exactly
// what "go test ./..." does when two packages both open windows.
import "testing"

// TestSystemIconSizeIsPositive guards the size arithmetic
// setWindowIcons depends on: LoadImageW with a zero width or height
// means "use the resource's own size", which silently produces a
// wrong-sized icon rather than an error.
func TestSystemIconSizeIsPositive(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cx, cy uintptr
	}{
		{"small", smCXSMICON, smCYSMICON},
		{"big", smCXICON, smCYICON},
	} {
		cx, cy := systemIconSize(0, tc.cx, tc.cy)
		if cx == 0 || cy == 0 {
			t.Errorf("systemIconSize(%s) = %d x %d, want both positive", tc.name, cx, cy)
		}
	}
}
