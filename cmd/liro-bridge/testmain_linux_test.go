//go:build linux

package main

import (
	"io"
	"os"
	"testing"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// forbidCoreDumpsChild makes this test binary, started with it set, a
// process that has done exactly what every process of the real binary but
// the chooser helper does first — platform.ForbidCoreDumps — and then waits
// for its stdin to end. It is the control in chooserhelper_linux_test.go:
// a process of this user that the portal's /proc check refuses.
const forbidCoreDumpsChild = "LIRO_TEST_FORBID_CORE_DUMPS_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(forbidCoreDumpsChild) == "1" {
		_ = platform.ForbidCoreDumps()
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
