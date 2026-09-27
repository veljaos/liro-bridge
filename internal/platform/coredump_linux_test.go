//go:build linux

package platform

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// coreDumpChildEnv marks the re-executed test binary as the child that calls
// ForbidCoreDumps, so the test process itself is never made undumpable.
const coreDumpChildEnv = "LIRO_TEST_FORBID_CORE_DUMPS_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(coreDumpChildEnv) == "1" {
		os.Exit(forbidCoreDumpsChild())
	}
	os.Exit(m.Run())
}

// forbidCoreDumpsChild calls ForbidCoreDumps and reports what the kernel
// says afterwards — not what the call returned, which D-376 and the owner's
// ruling do not accept as evidence.
func forbidCoreDumpsChild() int {
	// Raise the soft limit first, as anybody starting the program may, so
	// that the zero afterwards is this program's doing and not inherited.
	var lim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &lim); err == nil {
		lim.Cur = lim.Max
		_ = unix.Setrlimit(unix.RLIMIT_CORE, &lim)
	}
	if err := ForbidCoreDumps(); err != nil {
		fmt.Print("error " + err.Error() + "\n")
		return 1
	}
	dumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err != nil {
		fmt.Print("error " + err.Error() + "\n")
		return 1
	}
	f, err := os.Open("/proc/self/limits")
	if err != nil {
		fmt.Print("error " + err.Error() + "\n")
		return 1
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "Max core file size") {
			fields := strings.Fields(sc.Text())
			fmt.Print("core " + fields[4] + " " + fields[5] + "\n")
		}
	}
	if dumpable == 0 {
		fmt.Print("dumpable 0\n")
	} else {
		fmt.Print("dumpable nonzero\n")
	}
	return 0
}

// After ForbidCoreDumps the kernel reports a core limit of zero, soft and
// hard, and the process not dumpable. Read from the kernel in a child, so the
// test binary keeps its own settings. D-376 measured what these two do
// against apport; this keeps them from quietly going away.
func TestForbidCoreDumpsIsWhatTheKernelReports(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), coreDumpChildEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("child: %v, output %q", err, out)
	}
	got := string(out)
	for _, want := range []string{"core 0 0\n", "dumpable 0\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("after ForbidCoreDumps the kernel reports %q, want it to include %q", got, want)
		}
	}
}
