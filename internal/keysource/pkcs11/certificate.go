package pkcs11

import (
	"errors"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// ErrLoginNotBuilt is returned by Open. It is the wall F11 stops at.
//
// C_Sign on a token's private key needs a logged-in session, and the login
// step is deliberately not built: SPEC §6.5.1 requires the protected
// authentication path wherever a module offers one and only falls back to
// asking for the PIN where it does not, and SafeSign — the module F11's own
// exit condition turns on — has not been asked which it is. That reading costs
// nothing and spends no PIN attempt; building the fallback before it exists
// would be building for a question nobody has closed (D-269).
//
// It lives here, unconstrained, so that anything above this layer branches on
// one sentinel whatever it is compiled for.
var ErrLoginNotBuilt = errors.New("pkcs11: signing needs a logged-in session and the login step is not built yet (F11 §5)")

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
