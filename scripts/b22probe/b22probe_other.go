//go:build !windows

package main

import (
	"fmt"
	"os"
)

// b22probe is Windows-only, and the question it asks is Windows-only with it.
// It measures what UI Automation, MSAA and a cross-process WM_GETTEXT are told
// about the text typed into the shipped PIN dialog (D-395, open-item B24). The
// Linux counterpart question — what the accessibility bus is told — is D-384's,
// and the probe that answered it is scripts/a11yprobe with scripts/pinmem.
//
// This file exists so that `go vet ./...` and `go list ./...` are valid on
// every platform, the way scripts/cngprobe's does. It matters for one specific
// reason beyond tidiness: ci.yml asserts by exact name that only four packages
// depend on internal/ui on linux, and a package that reached the dialog from
// here would break that list rather than fail a build.
func main() {
	fmt.Fprintln(os.Stderr,
		"b22probe: Windows only. It measures what Windows accessibility is told\n"+
			"about the PIN dialog's text; on this platform the equivalent question\n"+
			"is the accessibility bus's, answered by scripts/a11yprobe (D-384).")
	os.Exit(1)
}
