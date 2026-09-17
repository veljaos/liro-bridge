package pkcs11

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ProbeSubcommand is the argument that puts this program's own binary into
// the one mode that loads a PKCS#11 module.
//
// It is a constant here rather than in the worker package because this is the
// side that spawns, and this package must not import the worker: the worker
// imports this one for the binding, so an import back would be a cycle.
//
// Every dispatch matches against this constant rather than against a literal,
// so the two agreeing is a compile-time fact and not something a test could
// usefully assert. What is worth asserting is that the name cannot be confused
// with a path or with a real subcommand, since the agent decides on args[0]
// alone; probe_test.go does that.
const ProbeSubcommand = "pkcs11-probe"

// ProbeTimeout bounds a child that hangs rather than dies.
//
// D-272 measured a module killing its process, in two ways, and established
// that no Go process survives either. It did not measure a module that stops
// answering, and nothing says one cannot: a module that blocks in its own
// DllMain or waits on a reader that will never answer would leave a child
// alive and silent for ever. A probe that can hang is a worse defect than the
// crash it exists to contain, because the crash at least ends.
//
// Ten seconds, against a measured cost of about 30ms per candidate: 300
// sequential probes of NetSeT 1.1.0.0, each one a process spawn, a LoadLibrary,
// C_Initialize, C_GetInfo and C_Finalize, took 9.786s — 33ms each, and 28ms
// each over a second run of 100. Four candidates is therefore roughly an eighth
// of a second for a whole listing, and the timeout is three hundred times the
// cost of the thing it bounds. A module that has not answered C_GetInfo in ten
// seconds is not going to.
const ProbeTimeout = 10 * time.Second

// probeChildMarker is set on every child this package spawns, and its only
// purpose is to stop a second generation.
//
// # It exists because the recursion actually happened
//
// probeOutOfProcess spawns os.Executable(). In the agent that is the agent,
// which dispatches ProbeSubcommand as the first thing run() does and answers.
// In a *test binary* it is the test binary, which has never heard of the
// subcommand, ignores the positional arguments, and runs the whole test suite
// — including the tests that call Modules, each of which spawns again. That is
// not a hypothetical: `go test ./internal/keysource/pkcs11/...` took this
// machine from 254 processes to 827 before it was killed.
//
// # Why the guard is here and not only in a TestMain
//
// A TestMain in this package would make this package's own test binary answer
// the subcommand, and it does (testmain_test.go). But it protects exactly one
// binary. The property that has to hold is about every binary: whatever
// os.Executable() turns out to be, and whether or not it dispatches anything,
// one accidental generation must not become an unbounded number.
//
// So the check is on the parent side and reads the *parent's own* environment.
// A process that was itself spawned as a probe refuses to spawn one, whatever
// it decided to do with the arguments it was given. Depth is bounded at one by
// construction rather than by every future binary remembering to dispatch.
//
// The child never reads this variable; only a would-be grandparent does. It can
// subtract a capability and cannot add one, which is why it does not disturb
// RunProbe's contract that it acts on its argument and nothing else.
const probeChildMarker = "LIRO_BRIDGE_PKCS11_PROBE_CHILD"

// errProbeRecursion is what a candidate becomes when the process asking about
// it is itself a probe child.
//
// It is an ordinary Failure rather than a panic because the caller is Modules,
// whose whole contract is that nothing about one candidate is fatal. A person
// who somehow reached this gets a listing where every module says why it could
// not be asked, which is recoverable and legible; a fork bomb is neither.
var errProbeRecursion = errors.New("a probe child must not probe: this process was spawned to load one module and nothing else")

// probeResult is what a child reports on its standard output: one JSON object
// and nothing else.
//
// It is deliberately small. The child is the process that may be killed by
// somebody else's code, so everything it is trusted to produce is a fact about
// one module rather than anything the parent acts on directly.
type probeResult struct {
	OK                 bool   `json:"ok"`
	Manufacturer       string `json:"manufacturer,omitempty"`
	LibraryDescription string `json:"libraryDescription,omitempty"`
	Err                string `json:"error,omitempty"`
}

// errWorkerDied is what a candidate becomes when the child did not survive
// loading it. It is a Failure in a list, which is what F11 §3 asks discovery
// to produce and what D-272 measured to be impossible in-process.
var errWorkerDied = errors.New("the probe process did not survive loading this module")

// errWorkerSilent is a child that neither answered nor died.
var errWorkerSilent = errors.New("the probe process did not answer in time")

// probeChildEnv is the environment a probe child is given.
//
// It is a function rather than a line inside probeOutOfProcess so that there is
// something to test. Deleting the marker is the whole fork bomb back — no child
// would be marked, so no child would refuse to spawn — and that deletion is
// invisible from outside the process: a child that answers correctly answers
// exactly the same way whether or not it was marked. A guard whose removal
// nothing notices is not one.
//
// The marker is appended to the real environment rather than replacing it. A
// vendor module reads the environment during DllMain — SystemRoot, PATH, the
// user's temporary directory — so a child probing with an empty environment
// would be probing under conditions the agent never runs in, and a module that
// then behaved differently would have been measured in the wrong process.
func probeChildEnv() []string {
	return append(os.Environ(), probeChildMarker+"=1")
}

// probeOutOfProcess loads one candidate in a child process and reports what it
// found, turning any way the child failed to answer into an ordinary error.
//
// This is D-275's remedy. The whole of its reason is that there is no
// in-process one: recover() catches neither of the two terminations a real
// module produces, and a vectored handler does not help, both measured.
//
// Nothing here is fatal to this process, by construction rather than by care —
// the only thing that touches the module is a child, and every way a child can
// end is an error value.
func probeOutOfProcess(ctx context.Context, path string) (probeResult, error) {
	if os.Getenv(probeChildMarker) != "" {
		return probeResult{}, errProbeRecursion
	}

	self, err := os.Executable()
	if err != nil {
		return probeResult{}, fmt.Errorf("finding this program's own binary: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, self, ProbeSubcommand, path)

	// The child inherits nothing it does not need. It gets no standard input:
	// the one thing that ever travels a pipe into a child of this program is a
	// PIN (SPEC §6.5.1 clause 2), and a probe has no business being able to
	// receive one.
	cmd.Stdin = nil

	// Its standard error is this program's, so a module that writes to the
	// console during DllMain — measured: Nexus's personal64.dll does — lands
	// where every other diagnostic does rather than being swallowed.
	cmd.Stderr = os.Stderr

	cmd.Env = probeChildEnv()

	out, runErr := cmd.Output()

	if ctx.Err() != nil {
		return probeResult{}, errWorkerSilent
	}
	if runErr != nil {
		// Any non-zero exit is the child not surviving: a fail-fast, a C++
		// throw, an access violation, or a refusal. Which one is in the exit
		// code, and the caller wants the path and the fact rather than the
		// number.
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return probeResult{}, fmt.Errorf("%w (exit 0x%X)", errWorkerDied, uint32(exitErr.ExitCode()))
		}
		return probeResult{}, fmt.Errorf("running the probe: %w", runErr)
	}

	var res probeResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &res); err != nil {
		return probeResult{}, fmt.Errorf("the probe answered something that is not a result: %w", err)
	}
	if !res.OK {
		return res, errors.New(res.Err)
	}
	return res, nil
}
