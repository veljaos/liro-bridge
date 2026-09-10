package main

import (
	"bytes"
	"strings"
	"testing"
)

// The two signing paths with no consent window are listed by --help
// when they exist and are absent from it when they do not, and a
// release binary refuses `sign-digest` by name rather than printing the
// usage text and exiting 0.
//
// One test for both build configurations, rather than one test each
// behind opposite build tags. The Windows CI job builds this suite
// *with* the softtoken tag, so a check that only compiles without it
// would never run there — which is D-221's rule and the reason D-222
// arranged `sign` the way it did.
func TestTheCommandsWithNoConsentWindowAreListedExactlyWhenTheyExist(t *testing.T) {
	withIsolatedHome(t)
	var out bytes.Buffer

	if code := run([]string{"--help"}, &out); code != 0 {
		t.Fatalf("--help exit code = %d, want 0", code)
	}
	help := out.String()

	for _, name := range []string{"sign-digest", "sign-no-consent"} {
		listed := strings.Contains(help, name)
		if listed != hasNoConsentPaths {
			t.Errorf("--help lists %q: %v; this build has the paths with no consent window: %v",
				name, listed, hasNoConsentPaths)
		}
	}

	// `sign` and `certs` are in every build, so a --help that listed
	// nothing at all would not pass this test for the wrong reason.
	for _, name := range []string{"sign", "certs"} {
		if !strings.Contains(help, name) {
			t.Errorf("--help does not list %q", name)
		}
	}
}

func TestAReleaseBuildRefusesSignDigestByNameRatherThanPrintingUsage(t *testing.T) {
	if hasNoConsentPaths {
		t.Skip("this build has sign-digest")
	}
	withIsolatedHome(t)
	var out bytes.Buffer

	code := run([]string{"sign-digest", "--thumbprint", "ABCD", "--digest", strings.Repeat("ab", 32)}, &out)
	if code != 2 {
		t.Errorf("exit code = %d, want 2: a command that used to sign and now does nothing must not report success", code)
	}
}
