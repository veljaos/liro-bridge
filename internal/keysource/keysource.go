// Package keysource abstracts over sources of signing keys (SPEC §5.1).
//
// A Source enumerates the signing certificates available on this machine
// and opens Sessions that produce raw signatures over pre-computed
// digests. Nothing in this package, or in anything built on top of it,
// ever sees a document — that is the whole reason SignDigest takes a
// digest and not a document (SPEC §5.1's own "why").
package keysource

import (
	"context"
	"fmt"
)

// Thumbprint is a certificate's SHA-1 thumbprint, uppercase hex — the
// identifier used everywhere in the agent (SPEC §11, F1 §3.1).
type Thumbprint string

// DigestAlgorithm identifies a hash algorithm a Session can sign a
// digest of. F2 supports exactly one; more may be added as new
// constants, never by repurposing this one (SPEC §7's "new codes, never
// repurposed" discipline applies equally well here).
type DigestAlgorithm int

const (
	DigestAlgorithmUnknown DigestAlgorithm = iota
	// DigestSHA256 is the only algorithm this phase signs. SPEC §12.4:
	// SHA-256 is the document digest algorithm everywhere in this
	// project; SHA-1 must never be produced (SPEC §18.8).
	DigestSHA256
)

// Size returns the expected digest length in bytes for alg, or 0 for an
// unrecognised algorithm — callers use this to validate a digest's
// length before it ever reaches a signing backend (F2 §2.2).
func (a DigestAlgorithm) Size() int {
	switch a {
	case DigestSHA256:
		return 32
	default:
		return 0
	}
}

// String implements fmt.Stringer so error messages naming an algorithm
// are readable without a type switch at every call site.
func (a DigestAlgorithm) String() string {
	switch a {
	case DigestSHA256:
		return "SHA-256"
	default:
		return fmt.Sprintf("DigestAlgorithm(%d)", int(a))
	}
}

// Certificate is one candidate signing certificate a Source can open a
// Session for.
type Certificate struct {
	Thumbprint Thumbprint

	// DER is the raw X.509 certificate.
	DER []byte

	// IsTestKey is true when this certificate comes from the soft token
	// (F2 §3.1) rather than real hardware. SPEC §16.6 requires every
	// signature produced with such a certificate to be visibly marked as
	// a test signature everywhere downstream — this field is the origin
	// of that mark.
	IsTestKey bool
}

// Source enumerates certificates and opens signing sessions. Both the
// Windows CNG backend (internal/keysource/windowscng) and the soft
// token (internal/keysource/softtoken) implement this identically, so
// nothing above this layer knows or cares which one it is talking to.
type Source interface {
	// Name identifies the backend, e.g. "windows-cng" or "softtoken".
	Name() string

	// List returns every certificate this source can sign with,
	// including ones that are currently unusable. The caller decides
	// what to show and what to disable.
	List(ctx context.Context) ([]Certificate, error)

	// Open begins a signing session for one certificate. Opening may
	// prompt the user for a PIN — the agent never sees it (SPEC §6.5,
	// F2 §2.3). The session must be closed.
	Open(ctx context.Context, thumbprint Thumbprint) (Session, error)
}

// Session signs digests with one certificate. Not safe for concurrent
// use (SPEC §8.5) — internal/signing owns serialisation.
type Session interface {
	// SignDigest signs a pre-computed digest. The digest must already be
	// the correct length for alg; implementations validate this before
	// making any backend call (F2 §2.2).
	SignDigest(ctx context.Context, alg DigestAlgorithm, digest []byte) ([]byte, error)

	// Certificate returns the signer certificate.
	Certificate() Certificate

	// Chain returns the issuing chain if the source can supply it. May
	// be empty; the caller is responsible for completing the chain
	// (SPEC §11.6) — in this phase, nobody does, because there is no
	// document to embed it into yet (that arrives in F3).
	Chain() [][]byte

	Close() error
}
