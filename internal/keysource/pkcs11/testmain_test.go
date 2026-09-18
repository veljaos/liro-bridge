package pkcs11

import (
	"os"
	"testing"
)

// TestMain makes this package's test binary answer the probe subcommand, the
// way the agent does.
//
// # What it is for, and what it is not for
//
// probeOutOfProcess spawns os.Executable(). Under `go test` that is
// pkcs11.test, not liro-bridge.exe, and a test binary that has never heard of
// ProbeSubcommand ignores the positional arguments and runs the whole suite —
// which calls Modules, which spawns again. Measured: `go test
// ./internal/keysource/pkcs11/...` took this machine from 254 processes to 827
// before it was killed.
//
// It is not what stops that happening. ChildMarker is; it is on the
// parent side, it reads the parent's own environment, and it holds for every
// binary whether or not the binary dispatches anything. A guard that depends on
// each future test binary remembering to add a TestMain is not a guard.
//
// What this is for is that the tests be about the thing. With the marker alone
// the suite terminates, but every Modules call inside a child returns "a probe
// child must not probe", and TestAFileThatIsNotAModuleIsAFailureAndNotACrash
// then passes while reading a refusal to spawn rather than a file that is not a
// module — a test whose input and expectation have quietly become the same
// thing. Dispatching here means the children do what the agent's children do,
// so the parent's tests exercise the real path.
//
// Because of that, deleting this is not silent: TestTheGuardIsWhatRefuses's
// control spawns a child and expects a module verdict back, and a test binary
// that runs its suite instead answers with test output, which is not a result.
func TestMain(m *testing.M) {
	// Before testing.M parses anything: these arguments are not test flags and
	// the flag package would reject them. Mirrors run() in cmd/liro-bridge.
	if len(os.Args) > 2 && os.Args[1] == ProbeSubcommand {
		// A child asked to die does so here rather than inside RunProbe, so
		// that nothing which can end this process on request exists outside a
		// test binary. crashIfAskedForTest returns only when the path is not a
		// sentinel, which is every real probe. See crash_windows_test.go.
		crashIfAskedForTest(os.Args[2])
		os.Exit(RunProbe(os.Args[2:], os.Stdout))
	}
	os.Exit(m.Run())
}
