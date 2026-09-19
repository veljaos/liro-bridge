package worker

import (
	"context"
	"errors"
	"io"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// Sources returns one worker-backed Source per usable module on this machine,
// and one Failure per candidate that was not one.
//
// It is pkcs11.Sources' counterpart and answers the same question, differing
// only in where the module ends up: in a child process rather than in this one.
// The discovery half is pkcs11.Modules' unchanged — a throwaway probe child per
// candidate, which is right there and wrong for a session (see Worker).
//
// stderr is used twice: once for the probe child discovery spawns per candidate
// and once for the worker child kept per usable module. A probe child that dies
// inside C_Initialize prints a Go runtime crash dump, and until this parameter
// reached it that dump went to the console of whatever asked for a listing.
//
// It is asked for a writer per module rather than given one for all of them. A vendor module is not required to be quiet and one of them is measured
// not to be — Nexus's personal64.dll writes from inside DllMain (D-303) — and a
// line from a module that does not say which module is a line nobody can act
// on. One writer shared between children could not carry that, because what
// arrives is a raw copy of a pipe with nothing to attach it to.
//
// # No worker is started here
//
// A Worker starts lazily on its first request, and this function does not force
// that. A module that discovery accepted is a module that answered C_GetInfo in
// a child thirty milliseconds ago, and spawning a second child immediately to
// prove it again would double the cost of a listing to re-measure something
// nothing has had a chance to change.
//
// What that means for the caller is that this function's Failures are
// *discovery's* — a module that would not load — and that a worker which dies
// later produces its own, at the moment it dies, through ListAll or through
// whatever asked it. That is the division F11 §4 asks for: a dead worker is a
// row in a list rather than an error that ends one.
func Sources(configured string, entry pkcs11.PINEntry, stderr func(modulePath string) io.Writer) ([]Source, []pkcs11.Failure) {
	usable, failures := pkcs11.Modules(configured, stderr)
	return sourcesFor(usable, entry, stderr), failures
}

// sourcesFor is Sources without the discovery, so the one property that
// matters here can be measured on a machine with no PKCS#11 module at all.
//
// That property is that each Source keeps the candidate it was built from. It
// is invisible until something goes wrong — Failure is the only thing that
// reads it — so a test against the discovery half would be vacuous on CI and
// would pass on a developer machine for reasons that have nothing to do with
// the code (D-296's first question).
func sourcesFor(usable []pkcs11.Candidate, entry pkcs11.PINEntry, stderr func(modulePath string) io.Writer) []Source {
	out := make([]Source, 0, len(usable))
	for _, c := range usable {
		out = append(out, Source{
			w:         New(c.Path, stderr(c.Path)),
			entry:     entry,
			candidate: c,
		})
	}
	return out
}

// Failure turns an error this Source produced into a row, with the candidate
// this Source came from attached.
//
// The module path is the point. ErrWorkerDied says a process stopped answering
// and deliberately does not say which module killed it, because from the
// parent's side there is nothing to tell the causes apart (see ErrWorkerDied).
// What the parent does know, and has known since before the child existed, is
// which module it started the child for — and on a machine with one card behind
// two builds of one vendor's module five years apart (D-271, D-272) that is the
// only thing that makes the report actionable.
//
// The vendor and origin come with it, so a row can say "the configured path"
// rather than only a filename, which is the distinction F11 §3 draws between a
// person's own instruction failing and a known path simply being absent.
func (s Source) Failure(err error) pkcs11.Failure {
	c := s.candidate
	if c.Path == "" {
		// A Source built by NewSource rather than by Sources has no candidate.
		// Its module path is still known, and a row naming the path with an
		// unknown origin is worth more than a row naming nothing.
		c = pkcs11.Candidate{Path: s.ModulePath(), Origin: pkcs11.OriginKnown}
	}
	return pkcs11.Failure{Candidate: c, Err: err}
}

// ListAll asks every source for its certificates and returns everything that
// answered, with one Failure for every source that did not.
//
// **A source that fails does not cost the listing.** That is F11 §3's rule for
// a module that will not load, and this applies it to the case §3 could not
// reach: a module that loaded, answered discovery, and whose worker then died
// mid-listing. Either way the answer to "what can I sign with" is the
// certificates that were found, beside a list of what could not be asked and
// why — never an error where a list should be.
//
// # What it deliberately does not do
//
// It does not deduplicate. One card can appear through more than one module —
// this project's own machine has NetSeT at two paths in two builds five years
// apart, and both see the same card identically (D-271) — and collapsing that
// is F11 §4 step 3's job, on the thumbprint, across *all* backends including
// Windows CNG. Doing half of it here, over PKCS#11 only, would produce a list
// that is deduplicated in one dimension and not the other, which is harder to
// reason about than one that is not deduplicated at all.
//
// It does not stop at the first failure and it does not stop at the first
// success, because both would make which certificates a person is offered
// depend on the order modules happen to be discovered in.
func ListAll(ctx context.Context, sources []Source) ([]Listing, []pkcs11.Failure) {
	var listings []Listing
	var failures []pkcs11.Failure
	for _, s := range sources {
		// One cancelled context ends the whole listing rather than producing a
		// Failure per remaining source. A cancellation is the caller changing
		// its mind, not a module misbehaving, and recording it against modules
		// that were never asked would put this program's own decision into a
		// report about somebody's hardware.
		if err := ctx.Err(); err != nil {
			return listings, failures
		}
		found, err := s.List(ctx)
		if err != nil {
			failures = append(failures, s.Failure(err))
			continue
		}
		listings = append(listings, Listing{Source: s, Certificates: found})
	}
	return listings, failures
}

// Listing is what one module answered.
//
// The source is kept rather than the certificates being flattened together,
// and the reason is worth recording because the first draft did flatten them:
// the caller that turns this into a row has to be able to say *which module*
// saw a certificate — a machine can have one card behind two builds of one
// vendor's module that differ by twenty-seven times on one call (D-271,
// D-305). Flattening threw that away, and the caller then re-listed every
// module to get it back, which asks every card twice.
//
// A caller that does not care can flatten in one line. A caller that does care
// cannot recover what was dropped without paying for it again.
type Listing struct {
	Source       Source
	Certificates []keysource.Certificate
}

// CloseAll shuts every worker down and returns the first error, having tried
// all of them.
//
// All of them, rather than stopping at the first: these are child processes,
// and one that would not close is not a reason to leave the others running. The
// error is returned rather than swallowed because ErrWorkerAbandoned — a child
// that was killed and did not go — has never been produced by this package
// (D-306) and the first time it happens will be somewhere nobody can attach a
// debugger to.
func CloseAll(ctx context.Context, sources []Source) error {
	var first error
	for _, s := range sources {
		if err := s.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// IsWorkerFailure reports whether err is one of the ways a worker stops being
// useful, as against an answer the module gave.
//
// It exists because the two need telling apart by callers that have no business
// knowing this package's sentinels: "the worker died" is something to log with
// a module path and carry past, where "the card says CKR_PIN_INCORRECT" is an
// answer to show a person. Both arrive at the same call site as an error.
func IsWorkerFailure(err error) bool {
	return errors.Is(err, ErrWorkerDied) ||
		errors.Is(err, ErrWorkerCannotStart) ||
		errors.Is(err, ErrWorkerAbandoned) ||
		errors.Is(err, ErrUnexpectedPINRequest)
}
