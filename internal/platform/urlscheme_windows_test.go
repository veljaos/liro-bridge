//go:build windows

package platform

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The scheme registered, read back and removed — under a key of the
// test's own, so a link clicked on the developer's machine during a run
// still reaches the installed agent.
func TestURLSchemeRoundTrip(t *testing.T) {
	const root = `Software\LiroBridgeTestScratch\Scheme`
	s := URLScheme{keyPath: root + `\` + LinkScheme}
	cleanup := func() {
		_ = s.Unregister()
		_ = registry.DeleteKey(registry.CURRENT_USER, root)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\LiroBridgeTestScratch`)
	}
	cleanup()
	t.Cleanup(cleanup)

	const exe = `C:\Users\Petar Petrović\AppData\Local\Liro\liro-bridge.exe`
	if err := s.Register(exe); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := s.Command()
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if want := `"` + exe + `" open-link "%1"`; got != want {
		t.Fatalf("the registered command is %q, want %q", got, want)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, s.keyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("opening the scheme key: %v", err)
	}
	if _, _, err := k.GetStringValue("URL Protocol"); err != nil {
		t.Errorf("the scheme key has no URL Protocol value, so Windows would not treat it as a scheme: %v", err)
	}
	_ = k.Close()

	if err := s.Unregister(); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, s.keyPath, registry.QUERY_VALUE); err != registry.ErrNotExist {
		t.Fatalf("the scheme key is still there after Unregister (err %v)", err)
	}
}
