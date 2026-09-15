package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// pkcs11Package is the backend this gate is about.
const pkcs11Package = "github.com/veljaos/liro-bridge/internal/keysource/pkcs11"

// agentPackage is the shipped binary.
const agentPackage = "github.com/veljaos/liro-bridge/cmd/liro-bridge"

// TestTheAgentCannotReachThePKCS11BackendYet fails the moment anything in the
// agent's dependency graph imports internal/keysource/pkcs11.
//
// # Why this exists, and why it is a test rather than a rule in checkdeps
//
// One PKCS#11 module on a real machine — NetSeT 1.1.0.0, the build MUP's own
// middleware installs — intermittently kills the process inside its own
// C_Initialize, about once in a hundred calls, and D-272 measured that no Go
// process can survive it: recover() catches neither the C++ exception nor the
// fail-fast, and a vectored handler does not help. There is no in-process
// remedy. The remedy is to probe out of process, and it is deferred (D-275).
//
// Today that costs nothing, because nothing outside the package imports it and
// the agent therefore never loads a module. The crash kills
// `go test ./internal/keysource/pkcs11/` and not the program. **Wiring
// discovery in is what connects it to the agent**, and after that a person
// with a Pošta card and MUP's middleware installed has a one-in-a-hundred
// chance of the agent dying on startup with nothing on screen and nothing in
// the log.
//
// So this is not an architectural rule — scripts/checkdeps holds those, they
// are SPEC §4.2's, and they are permanent. This one is a condition with an
// expiry: F11 §4 is *supposed* to wire this in, once the remedy exists.
// Deleting this test is part of doing that, and the failure message says so.
//
// It lives in cmd/liro-bridge rather than beside the package it guards for one
// reason: this is the package whoever writes §4 will be editing, and a gate
// that fires somewhere else is a gate they meet after the change rather than
// during it (D-108's "one rule, one place", applied to where it is read).
func TestTheAgentCannotReachThePKCS11BackendYet(t *testing.T) {
	deps := depsOf(t, agentPackage)

	// The positive control. A check that only looks for absence passes for the
	// wrong reason the moment the mechanism breaks — a renamed package, a
	// `go list` that answered nothing, a typo in the constant above (D-031's
	// two-directional discipline). If `go list -deps` cannot see this package
	// where it certainly is, it cannot be trusted to see it where it must not
	// be.
	if control := depsOf(t, pkcs11Package); !control[pkcs11Package] {
		t.Fatalf("go list -deps %s does not list the package itself, so this check "+
			"cannot see what it is looking for", pkcs11Package)
	}

	if deps[pkcs11Package] {
		t.Fatalf(`the agent now imports %s.

That connects a crash path to the shipped binary. One module on a real machine
(NetSeT 1.1.0.0, installed by MUP's own middleware) dies inside C_Initialize
about once in a hundred calls, and D-272 measured that no Go process survives
it: recover() catches neither termination and a vectored handler does not help.
With this import in place, a person holding a Pošta card with that middleware
installed can have the agent die on startup, silently.

Read docs/decisions.md D-275 before going further. It names the remedy —
probing each module in a child process, so that one which kills it becomes a
Failure in a list (F11 §3) — and records why it was deferred rather than built.

If you have built that remedy, this test is what you delete as part of it.`,
			pkcs11Package)
	}
}

// depsOf returns the transitive dependency set of one package, as the Windows
// build sees it.
//
// GOOS is pinned rather than inherited: the artefact this is about is the
// Windows agent, and a Linux CI runner asking about its own view would be
// answering a different question — internal/keysource/pkcs11 is Windows-only
// past module_other.go, so the Linux view could report it absent for a reason
// that has nothing to do with whether the agent imports it.
func depsOf(t *testing.T, pkg string) map[string]bool {
	t.Helper()

	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}

	deps := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			deps[line] = true
		}
	}
	if len(deps) == 0 {
		t.Fatalf("go list -deps %s returned nothing", pkg)
	}
	return deps
}
