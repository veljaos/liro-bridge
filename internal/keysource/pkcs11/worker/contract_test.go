package worker

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// modulePath is this project's own module path.
const modulePath = "github.com/veljaos/liro-bridge"

// selfPackage is this package, module-relative. go list -deps includes the
// package it was asked about, which is the subject of this rule rather than
// one of its dependencies.
const selfPackage = "internal/keysource/pkcs11/worker"

// allowedInternal is what this package's dependency closure may contain from
// internal/, and it is an allow-list rather than a deny-list on purpose.
//
// D-259 settled the shape for a different rule and the reasoning carries: "a
// rule that enumerates what is forbidden is a rule that is one unknown scheme
// from being wrong. The rule here enumerates what is allowed, which is one
// thing and is known." A deny-list naming internal/pades, internal/signing and
// internal/consent would be correct today and silent about internal/audit, or
// about whatever exists in a year.
//
// Each entry is here because the worker genuinely needs it:
//
//   - internal/keysource/pkcs11 is the binding it exists to call.
//   - internal/keysource is the Certificate/Thumbprint vocabulary it answers
//     in, so the agent does not have to translate.
//   - internal/errs is the error-code vocabulary SPEC §7 makes the entire
//     cross-boundary error representation, and is a dependency-free leaf
//     (D-024's own reasoning for letting internal/trust import it).
//
// Nothing else. In particular this closure must never contain internal/pades
// (SPEC §4.2 rule 2 already forbids it for everything under internal/keysource
// and this restates it where it is read), internal/signing, internal/consent,
// internal/ui, internal/api, internal/cli, internal/audit or internal/jobs —
// which between them are every way this process could learn what a document
// is, show a screen, or record an approval.
var allowedInternal = map[string]bool{
	"internal/keysource":        true,
	"internal/keysource/pkcs11": true,
	"internal/errs":             true,
}

// TestTheWorkerCannotReachAnythingThatKnowsWhatADocumentIs is F12 §2's "it
// signs nothing, it holds no consent" as a property of the build rather than
// as a sentence in a doc comment.
//
// It reads the whole dependency closure rather than this package's import
// block, because an import two packages away is the same reachability as an
// import here — which is the distinction scripts/checkdeps was built around
// (its own comment: "which is what makes checkPackage catch indirect
// violations, not only direct ones").
//
// GOOS is pinned to windows rather than inherited, for the reason D-275 gave
// when it pinned the same variable: the binding is Windows-only past
// module_other.go, so the Linux view would report packages absent for a reason
// that has nothing to do with whether this package can reach them. Pinning it
// asks about the artefact this rule is about.
func TestTheWorkerCannotReachAnythingThatKnowsWhatADocumentIs(t *testing.T) {
	deps := depsOf(t, ".")

	// The positive control, first. A check that only looks for absence passes
	// for the wrong reason the moment the mechanism breaks — a renamed
	// package, a `go list` that answered nothing, a module path typo
	// (D-031's two-directional discipline, and D-275's own control). If this
	// package is not in its own closure, `go list` cannot be trusted to
	// report what is.
	if !deps[modulePath+"/internal/keysource/pkcs11/worker"] {
		t.Fatalf("go list -deps does not list this package itself, so this " +
			"check cannot see what it is looking for")
	}

	for dep := range deps {
		rel, ok := strings.CutPrefix(dep, modulePath+"/")
		if !ok {
			continue // the standard library and third-party packages are not this rule's subject
		}
		if !strings.HasPrefix(rel, "internal/") {
			continue
		}
		if rel == selfPackage {
			continue // go list -deps includes the package itself; it is the subject, not a dependency
		}
		if allowedInternal[rel] {
			continue
		}
		t.Errorf("the worker's dependency closure contains %s.\n"+
			"\n"+
			"This process loads a vendor PKCS#11 module and nothing else may\n"+
			"be reachable from it: F12 §2 requires that it signs nothing, holds\n"+
			"no consent, and cannot be driven into signing by anything but the\n"+
			"parent that spawned it. Reaching a package that knows what a\n"+
			"document is, can show a screen, or can record an approval is how\n"+
			"that stops being true.\n"+
			"\n"+
			"If this package genuinely needs it, that is a decision to record\n"+
			"rather than an entry to add to allowedInternal.", rel)
	}
}

// TestTheContractRuleWouldActuallyFire is the other half, because an
// allow-list that nothing has ever violated is an allow-list nobody has
// checked. It asks the same question of the agent itself, which certainly
// reaches internal/pades, and requires the mechanism to say so.
//
// The agent is the right control rather than an arbitrary package: it is
// exactly what this rule exists to keep out of the worker's closure. If the
// allow-list would not refuse the agent, it would not refuse anything.
//
// internal/cli was the first choice and is the wrong one, measured: without
// the softtoken tag it does not reach internal/pades at all, because D-222 put
// the only path that does behind that tag. A control that depends on a build
// tag proves nothing about the build everyone ships.
func TestTheContractRuleWouldActuallyFire(t *testing.T) {
	const knownToReachPades = modulePath + "/cmd/liro-bridge"

	deps := depsOf(t, knownToReachPades)
	if !deps[modulePath+"/internal/pades"] {
		t.Fatalf("%s no longer reaches internal/pades, so this control proves "+
			"nothing; pick a package that does", knownToReachPades)
	}

	var refused []string
	for dep := range deps {
		rel, ok := strings.CutPrefix(dep, modulePath+"/")
		if !ok || !strings.HasPrefix(rel, "internal/") {
			continue
		}
		if !allowedInternal[rel] {
			refused = append(refused, rel)
		}
	}
	if len(refused) == 0 {
		t.Fatal("the allow-list refused nothing in a package that reaches " +
			"internal/pades, so it would refuse nothing here either")
	}
}

// depsOf returns the full dependency closure of pkg, as go list sees it for
// the Windows build.
func depsOf(t *testing.T, pkg string) map[string]bool {
	t.Helper()

	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Env = append(cmd.Environ(), "GOOS=windows", "GOARCH=amd64")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, stderr.String())
	}

	deps := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			deps[line] = true
		}
	}
	if len(deps) == 0 {
		t.Fatalf("go list -deps %s returned nothing", pkg)
	}
	return deps
}
