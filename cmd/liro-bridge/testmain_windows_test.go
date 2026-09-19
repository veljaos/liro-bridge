//go:build windows

package main

// TestMain answers the two PKCS#11 subcommands, and closes the shared
// windows (sharedwindow_windows_test.go) after the last test that
// borrowed one has finished — they outlive every individual test by
// design, so no single test can own their teardown.
//
// It also sweeps the temporary config homes an earlier run could not
// delete, before and after. A WebView2 browser process group holds
// files under one of those directories open after the window that made
// it has closed, and was measured still running two hours after the
// test binary that started it had exited — so `go test ./...` left
// about six megabytes in %TEMP% every time it ran, and 196 runs had
// left 1.26 GB (FTEST Group 3, C-6). Sweeping is what makes that stop
// accumulating rather than accumulate more slowly.
//
// # Why the subcommands, which is new
//
// This package became a PKCS#11 parent when the agent started reaching
// modules (F11 §4). Discovery spawns os.Executable() with
// pkcs11.ProbeSubcommand; a worker spawns it with worker.Subcommand.
// Under `go test` os.Executable() is liro-bridge.test.exe, and a test
// binary that has never heard of those arguments ignores them and runs
// the whole suite instead.
//
// That is D-293's process explosion, and it is worse here than the
// noise it was there. Observed once, on the run that added the listing:
// a child of this binary, invoked as `liro-bridge.test.exe pkcs11-probe
// C:\WINDOWS\System32\aetpkss1.dll`, was running the window tests — it
// had started an entire WebView2 browser process group — and was still
// holding its parent's standard error open a minute after the suite had
// reported PASS. `go test` printed "Test I/O incomplete 1m0s after
// exiting" and failed a run in which every test had passed. The child
// had to be killed by PID.
//
// **It has not been reproduced since**, and that is said here rather
// than left implied: whether a probe child hangs or merely runs a
// second copy of the suite and exits depends on what the window tests
// do, which is not deterministic. So this dispatch is justified by the
// cause that was observed in a running process, not by a red-to-green
// transition — there is no red to point at on demand.
//
// pkcs11.ChildMarker is what stops the recursion going further, and it
// held: nothing spawned a third generation. What a guard on the parent
// side cannot do is make the second generation do the right thing,
// which is what this is for. internal/keysource/pkcs11's own TestMain
// says the same in its own words.
import (
	"os"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11/worker"
)

func TestMain(m *testing.M) {
	// Before testing.M parses anything: these arguments are not test flags and
	// the flag package would reject them. Mirrors run() itself, which dispatches
	// both of these before it does anything else.
	if len(os.Args) > 2 && os.Args[1] == pkcs11.ProbeSubcommand {
		os.Exit(pkcs11.RunProbe(os.Args[2:], os.Stdout))
	}
	if len(os.Args) > 2 && os.Args[1] == worker.Subcommand {
		os.Exit(worker.Run(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}

	sweepStaleConfigHomes()
	code := m.Run()
	closeSharedPlacementWindow()
	closeSharedWindows()
	sweepStaleConfigHomes()
	os.Exit(code)
}
