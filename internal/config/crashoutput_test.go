package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// crashHelperEnv makes a copy of this test binary the child of the test
// below, and names the directory it should set up crash output in.
const crashHelperEnv = "LIRO_CONFIG_CRASH_HELPER_DIR"

// crashMarker is what the child panics with, so the parent can tell its
// report from anything else that might be in the file.
const crashMarker = "liro crash-output test: the child is dying on purpose"

// TestCrashHelper is not a test. It is this binary dying the way the
// agent died in D-454 — inside the runtime, with nothing deferred able to
// run — after setting up crash output, in a process of its own.
func TestCrashHelper(t *testing.T) {
	dir := os.Getenv(crashHelperEnv)
	if dir == "" {
		t.Skip("not the helper; this runs only as a child of the test below")
	}
	if err := SetupCrashOutput(dir); err != nil {
		t.Fatalf("SetupCrashOutput: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		panic(crashMarker)
	}()
	<-done
}

// TestACrashIsRecordedInTheLogDirectory is the regression test for D-454's
// finding: the agent died and nothing of the program's recorded it. The
// child must end with the runtime's report of its panic in crash.log —
// the marker and a goroutine stack — rather than only on a stderr nobody
// may be holding.
func TestACrashIsRecordedInTheLogDirectory(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashHelper$")
	cmd.Env = append(os.Environ(), crashHelperEnv+"="+dir)
	if err := cmd.Run(); err == nil {
		t.Fatal("the helper exited cleanly; it was meant to die of an unrecovered panic, so nothing was tested")
	}

	b, err := os.ReadFile(filepath.Join(dir, crashFileName))
	if err != nil {
		t.Fatalf("reading the crash file: %v", err)
	}
	report := string(b)
	if !strings.Contains(report, crashMarker) {
		t.Fatalf("crash.log does not carry the child's panic:\n%s", report)
	}
	if !strings.Contains(report, "goroutine ") {
		t.Fatalf("crash.log carries the panic but no stack, so it is not the runtime's report:\n%s", report)
	}
}
