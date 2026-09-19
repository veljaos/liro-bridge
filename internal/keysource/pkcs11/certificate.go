package pkcs11

import (
	"errors"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// ErrLoginNotBuilt was the wall F11 §2 stopped at: Open returned it while
// SafeSign had not been asked whether its token offers a protected
// authentication path, because building §6.5.1's fallback before that answer
// would have been building for a question nobody had closed (D-269, D-271).
//
// The answer is measured — it does not (D-273) — and Open is built, so nothing
// returns this any more. It is kept rather than deleted because it is
// exported: something outside this package may branch on it, and a sentinel
// that disappears turns a handled condition into an unhandled one at compile
// time in the best case and at run time in the worst. It may go when F11 §4
// has wired this backend up and nothing is left that could have held it.
//
// It lives here, unconstrained, so that anything above this layer branches on
// one sentinel whatever it is compiled for.
//
// Deprecated: Open no longer returns this. A token needing a PIN with nothing
// wired up to collect one returns ErrNoPINEntry instead.
var ErrLoginNotBuilt = errors.New("pkcs11: signing needs a logged-in session and the login step is not built yet (F11 §5)")

// ErrModuleClosed is a LiveModule used after Close.
//
// It is a distinct error rather than a panic because the worker's loop reaches
// it only through a request that arrived after an OpShutdown, which is
// something to answer rather than a fault in this process — a worker that
// panicked on a late request would look to its parent exactly like a module
// that killed it, which is the one thing the out-of-process arrangement exists
// to be able to tell apart.
//
// It lives here for the same reason ErrLoginNotBuilt does: a caller branches on
// one sentinel whatever platform it was compiled for.
var ErrModuleClosed = errors.New("pkcs11: this module has been closed")

// ErrCertificateNotFound is a thumbprint no token this module can see carries.
//
// # Why it is a sentinel and not a sentence
//
// It is the one answer a caller with several backends must be able to act on
// without reading English. A machine can have Windows CNG and two or three
// PKCS#11 modules on it, and choosing where to sign means asking each in turn
// until one says yes — which only works if "that certificate is not here" can
// be told from "this module is broken". Without it the two arrive at the call
// site as the same thing: an error. D-033 already ruled that masking a real
// failure behind a second attempt against an unrelated backend produces a
// confusing failure about the wrong thing, and that rule cannot be applied by
// a caller that cannot tell the cases apart.
//
// Until F11 §4 nothing above this package had more than one place to look, so
// a sentence was enough and a sentence is what both sites returned.
//
// It lives here for the same reason ErrModuleClosed does: one sentinel,
// whatever platform the caller was compiled for. It also crosses the worker's
// pipe, where a string cannot carry it — see worker.Response.NotFound.
var ErrCertificateNotFound = errors.New("pkcs11: no certificate with that thumbprint on any token this module sees")

// CertificateInfo is one certificate found on one token, with enough about
// where it came from to explain a duplicate.
//
// A machine can have several modules — this project's development machine has
// NetSeT at two paths in two builds five years apart, plus SafeSign and Nexus
// — and one card seen through two of them is two sightings of one certificate.
// Thumbprint is what collapses them; the rest is what says which sighting a
// row came from (F11 §4).
type CertificateInfo struct {
	Thumbprint   string
	DER          []byte
	Label        string
	SlotID       uint32
	TokenLabel   string
	TokenSerial  string
	ModulePath   string
	ProtectedPIN bool
}

// dedupe collapses sightings of one certificate to one row, first sighting
// winning, and drops everything a keysource.Certificate does not carry.
//
// One module can report one card in more than one slot, and a token can carry
// one certificate as more than one object. The thumbprint is the certificate,
// and it is byte-identical to what internal/keysource/windowscng computes for
// the same bytes — which is what lets the layer above collapse one card seen
// through CNG *and* through PKCS#11 into a single row.
//
// Order is never relied on. Measured: the two NetSeT builds return this card's
// two certificates in opposite order — Sign-then-Auth from TrustEdgeID
// 1.1.3.3, Auth-then-Sign from MUP RS\Celik 1.1.0.0.
func dedupe(found []CertificateInfo) []keysource.Certificate {
	seen := make(map[string]bool, len(found))
	out := make([]keysource.Certificate, 0, len(found))
	for _, c := range found {
		if c.Thumbprint == "" || seen[c.Thumbprint] {
			continue
		}
		seen[c.Thumbprint] = true
		out = append(out, keysource.Certificate{
			Thumbprint: keysource.Thumbprint(c.Thumbprint),
			DER:        c.DER,
		})
	}
	return out
}
