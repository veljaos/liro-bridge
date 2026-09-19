package worker

import (
	"context"
	"fmt"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// Source presents one Worker as a keysource.Source, so the agent reaches a
// PKCS#11 module the same way it reaches Windows CNG and the soft token: a
// backend that lists certificates and opens sessions, with nothing above this
// layer knowing which it is.
//
// # This is F11 §4 step 1, and it is what makes §2's argument true of the agent
//
// Everything below this file was built and measured against tests and against
// scripts/p11worker. The agent itself has never used it: internal/keysource/
// pkcs11 loads modules in-process and nothing outside cmd/liro-bridge's main.go
// imports it. **That is D-247's shape waiting to happen** — a worker, a
// protocol, a PIN seam, a lifecycle and nine entries of measurement, with no
// caller in the product. This type is the caller.
//
// # Why it is a wrapper rather than methods on Worker
//
// Worker.Open takes the PIN entry as a parameter, and source_windows.go's
// comment explains why that is right: in the worker the PIN does not come from
// a screen this process drew, and making it a parameter keeps the seam visible
// at the call. Satisfying keysource.Source means Open(ctx, thumbprint) with no
// entry, so something has to hold one. This does, and Worker keeps its
// parameter.
//
// What it holds is a way to obtain a PIN and never a PIN — the same line
// pkcs11.Source draws, for the same reason (SPEC §6.5.1 clause 2), and
// pin_test.go in this package is what makes that structural rather than a
// promise.
//
// # What it deliberately does not do
//
// It does not own the Worker. A Worker is a child process with a lifetime the
// agent manages — started lazily, respawned within a bound, closed on a context
// the caller chooses (see Worker.Close). Handing that lifetime to a value that
// gets copied around as an interface would put "when does the child die" in the
// hands of whoever last held a copy. Close is here so the owner has one place
// to call, and it is the owner who calls it.
type Source struct {
	w *Worker

	// entry collects the PIN when the token needs one. Nil is allowed and is
	// not checked here: a token with a protected authentication path never asks
	// (SPEC §6.5.1 clause 1), and one that does ask fails with
	// pkcs11.ErrNoPINEntry at the moment it asks. Refusing nil up front would
	// break the clause-1 branch, which is the one the SPEC prefers.
	entry pkcs11.PINEntry

	// candidate is where this module came from — the path, the vendor, and
	// whether it was the person's own configured path or one measured off a
	// real machine. Zero unless Sources built this Source.
	//
	// It is carried so that a worker dying later can be turned into a row that
	// says which module and on whose instruction (see Failure). The parent has
	// always known this and did not learn it from the child, which matters
	// because a dead child is precisely the case where nothing can be asked.
	candidate pkcs11.Candidate
}

// Source is a keysource.Source. The assertion is here rather than left to the
// call site because the whole point of this type is that the agent can hold it
// without knowing what it is, and a compile error is the right place to find
// out that it cannot.
var _ keysource.Source = Source{}

// NewSource presents w as a keysource.Source, collecting PINs with entry.
func NewSource(w *Worker, entry pkcs11.PINEntry) Source {
	return Source{w: w, entry: entry}
}

// WithPINEntry returns a copy of this Source that collects PINs with entry.
//
// A copy rather than a mutation, and for a sharper reason than pkcs11.Source's:
// the Worker behind this value is a child process that is expensive to start
// and is meant to be shared, while the screen that collects the PIN belongs to
// one window and one moment. The owner window of a PIN dialog is the window a
// person is looking at, which is not a property of a module.
//
// So the agent discovers modules once, keeps the Workers, and makes one of
// these per signature with the right screen attached. Copying a Source copies
// the pointer to the Worker and nothing else — the child is shared, the screen
// is not.
func (s Source) WithPINEntry(entry pkcs11.PINEntry) Source {
	s.entry = entry
	return s
}

// Name implements keysource.Source.
//
// It is "pkcs11" and not "pkcs11-worker" on purpose. The backend is the
// issuer's PKCS#11 module; that this program talks to it through a child
// process is how this program is built, not what the user's certificate came
// from, and a name that changed when the implementation did would put an
// implementation detail into audit records that outlive it.
//
// pkcs11.Source.Name() returns the same string, and source_test.go requires
// them to stay equal.
func (Source) Name() string { return "pkcs11" }

// ModulePath is which module this source speaks to. Two sources over two
// modules are told apart by this and by nothing else — a machine can have one
// card visible through two modules at two versions (D-271, D-272) — so it is
// what a Failure, a log line or an audit entry names.
func (s Source) ModulePath() string {
	if s.w == nil {
		return ""
	}
	return s.w.ModulePath()
}

// Origin says whether this module's path was the person's own configured one
// or one of the installation paths this project has measured off real machines.
//
// It is here because what an audit entry may record about a module depends on
// it: a known path is under Program Files or System32 and carries no personal
// name by construction, where a configured path is a person's own installation
// and can be anywhere, including under their user profile. SPEC §6.7 says the
// audit log never contains personal names, and this is the only thing that
// tells the caller which case it has.
//
// A Source built by NewSource rather than by Sources has no candidate, and
// answers OriginKnown — the conservative direction here is the one that admits
// less, and a caller that cannot establish a path was configured must not
// record it as though it had been checked.
func (s Source) Origin() pkcs11.Origin {
	if s.candidate.Path == "" {
		return pkcs11.OriginKnown
	}
	return s.candidate.Origin
}

// List implements keysource.Source.
//
// The worker answers with raw DER and an optional label, because the child
// sends nothing the parent could not derive and nothing the parent cannot use
// (protocol.go). The thumbprint is derived here, with pkcs11.Thumbprint, which
// is the same function the in-process backend uses and computes the same value
// windowscng does for the same bytes — measured on a real card in D-310, and
// the premise F11 §4 step 3 rests on.
func (s Source) List(ctx context.Context) ([]keysource.Certificate, error) {
	if s.w == nil {
		return nil, errNoWorker
	}
	payloads, err := s.w.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.convert(payloads), nil
}

// convert is List's body, separated so the skipping rule below can be tested
// without a child. No correct child sends an empty certificate and the canned
// test child cannot be asked to — adding a switch to it for that would be a
// seam whose only user is a test (D-100).
func (Source) convert(payloads []CertificatePayload) []keysource.Certificate {
	out := make([]keysource.Certificate, 0, len(payloads))
	for _, p := range payloads {
		if len(p.DER) == 0 {
			// A certificate with no bytes has no thumbprint, and a row whose
			// identifier is the SHA-1 of nothing would collide with every other
			// such row rather than being merely useless. Skipped rather than
			// returned, and not an error: one unreadable object must not cost
			// the listing, which is the same judgement enumerate makes about a
			// slot SafeSign does not recognise.
			continue
		}
		out = append(out, keysource.Certificate{
			Thumbprint: pkcs11.Thumbprint(p.DER),
			DER:        p.DER,
			// IsTestKey is false and is not a decision made here: it marks the
			// soft token (SPEC §16.6), and a real module reading a real card is
			// not one. If a PKCS#11 soft token is ever added it will need to
			// say so through the protocol rather than be guessed at from here.
			IsTestKey: false,
		})
	}
	return out
}

// Open implements keysource.Source.
//
// Opening may ask for a PIN, and when it does the exchange is the one SPEC
// §6.5.1 clause 2 permits: the screen belongs to the agent, the C_Login belongs
// to the worker, and the PIN crosses one inherited pipe in one write. Nothing
// here retries anything (clause 5), and nothing here can: a login that is not
// an answer ends the worker, which is Worker.Open's own behaviour and not
// something this wrapper adds or could take away.
func (s Source) Open(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
	if s.w == nil {
		return nil, errNoWorker
	}
	return s.w.Open(ctx, thumbprint, s.entry)
}

// Close shuts the worker down. See Worker.Close for why it takes a context and
// why context.Background() here is a choice rather than a default.
//
// It is not part of keysource.Source. A caller holding the interface cannot
// reach it, which is deliberate: the child's lifetime belongs to whoever
// started it.
func (s Source) Close(ctx context.Context) error {
	if s.w == nil {
		return nil
	}
	return s.w.Close(ctx)
}

// errNoWorker is what every method answers when this Source was built around
// nothing. The zero value of a struct is reachable in Go whatever the
// constructor does, and a nil-pointer panic inside a keysource.Source would
// reach the agent as a crash rather than as a Failure with a module path on it
// (F11 §3).
var errNoWorker = fmt.Errorf("pkcs11 worker: this source has no worker")
