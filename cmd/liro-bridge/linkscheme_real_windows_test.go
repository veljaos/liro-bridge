//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// realHKCUEnv is set by CI's windows job, whose user profile is thrown
// away with the runner. A person's machine does not set it: this test
// writes and then removes HKCU\Software\Classes\liro-bridge, which on a
// machine with the agent installed is the agent's own registration.
const realHKCUEnv = "LIRO_TOUCH_REAL_HKCU"

// TestTheLinkSchemeIsWrittenAtStartAndRemovedByTheUninstallNotice is
// worklist 0.5 (D-473): the scheme the agent registers at every start,
// under the real key — TestURLSchemeRoundTrip uses a scratch one — and
// its removal by uninstall-notice, which the installer runs before it
// removes the files. Neither had ever run.
//
// Not shown: Windows' shell resolving the scheme, or a browser opening
// a link (worklist 3.1).
func TestTheLinkSchemeIsWrittenAtStartAndRemovedByTheUninstallNotice(t *testing.T) {
	if os.Getenv(realHKCUEnv) != "1" {
		t.Skip("writes the real HKCU\\Software\\Classes\\liro-bridge; CI sets " + realHKCUEnv + "=1, a person's machine does not")
	}
	withIsolatedHome(t)
	const command = `Software\Classes\liro-bridge\shell\open\command`

	applyStartupRegistrations(config.Default())
	k, err := registry.OpenKey(registry.CURRENT_USER, command, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("after the agent's start-up registrations the link scheme's command key is not there: %v", err)
	}
	got, _, err := k.GetStringValue("")
	_ = k.Close()
	if err != nil || got != platform.LinkCommand(exePath()) {
		t.Fatalf("the link scheme runs %q (%v), want %q", got, err, platform.LinkCommand(exePath()))
	}

	if code := runUninstallNotice([]string{"--quiet"}, io.Discard, config.Default()); code != 0 {
		t.Fatalf("uninstall-notice --quiet returned %d", code)
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\liro-bridge`, registry.QUERY_VALUE); err == nil {
		_ = k.Close()
		t.Fatal("uninstall-notice left HKCU\\Software\\Classes\\liro-bridge behind: a link would still run a binary the installer is about to remove")
	} else if !errors.Is(err, registry.ErrNotExist) {
		t.Fatalf("reading the key after the uninstall notice: %v", err)
	}
}
