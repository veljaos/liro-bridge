package platform

import "testing"

// TestWindowsAutostartRoundTrip exercises the real
// HKCU\...\Run key (F5 §3), scoped entirely to this project's own
// "LiroBridge" value name — never touching any other value already
// present there — and always leaves the key exactly as it found it.
func TestWindowsAutostartRoundTrip(t *testing.T) {
	a := NewAutostart()

	before, err := a.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled (before): %v", err)
	}
	t.Cleanup(func() {
		if err := a.SetEnabled(before, `C:\test\liro-bridge.exe`); err != nil {
			t.Errorf("restoring autostart state: %v", err)
		}
	})

	if err := a.SetEnabled(true, `C:\test\liro-bridge.exe`); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	enabled, err := a.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled (after enable): %v", err)
	}
	if !enabled {
		t.Fatal("IsEnabled() = false after SetEnabled(true)")
	}

	if err := a.SetEnabled(false, `C:\test\liro-bridge.exe`); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	enabled, err = a.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled (after disable): %v", err)
	}
	if enabled {
		t.Fatal("IsEnabled() = true after SetEnabled(false)")
	}
}
