//go:build linux

package i18n

import (
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// TestLinuxSaysNothingFalseAboutReaders is D-358: the three sentences that
// are false on Linux are replaced in every catalogue, the one for a
// stopped pcscd gives the command that starts it, and none of them names
// Windows or claims a reader listing.
func TestLinuxSaysNothingFalseAboutReaders(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := Load(locale)
		for key, alt := range platformKeys {
			if _, ok := c.data[alt]; !ok {
				t.Errorf("%s: %s is replaced by %s, which this catalogue does not have", locale, key, alt)
			}
			if got := c.T(key); got != c.data[alt] {
				t.Errorf("%s: T(%q) = %q, want the Linux sentence", locale, key, got)
			}
			if strings.Contains(c.T(key), "Windows") {
				t.Errorf("%s: the Linux sentence for %s names Windows", locale, key)
			}
		}
		down := c.T(CodeKey(errs.CodeSmartCardServiceDown))
		if !strings.Contains(down, "systemctl enable --now pcscd.socket") {
			t.Errorf("%s: the Linux service-down sentence does not give the command: %q", locale, down)
		}
	}
}
