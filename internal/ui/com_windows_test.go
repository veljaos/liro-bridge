//go:build windows

package ui

import "testing"

// TestKnownGUIDsParse guards against a transcription error in the
// hand-copied IID literals (com_windows.go's package doc comment):
// every GUID this package hardcodes must parse without panicking.
// mustGUID panics on a malformed literal, so iidCoreWebView2_3's own
// package-level initialisation already exercises this once at load
// time — this test exists to give that fact an explicit, named,
// re-runnable check rather than relying on "the package failed to
// import" as the only signal.
func TestKnownGUIDsParse(t *testing.T) {
	const iidCoreWebView2_3Literal = "a0d6df20-3b92-416d-aa0c-437a9c727857"

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("mustGUID(%q) panicked: %v", iidCoreWebView2_3Literal, r)
		}
	}()
	got := mustGUID(iidCoreWebView2_3Literal)
	if got != iidCoreWebView2_3 {
		t.Fatalf("mustGUID(%q) = %+v, want the package's own iidCoreWebView2_3 (%+v)", iidCoreWebView2_3Literal, got, iidCoreWebView2_3)
	}
}
