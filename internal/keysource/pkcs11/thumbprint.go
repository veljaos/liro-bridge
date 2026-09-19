package pkcs11

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// thumbprint computes the certificate identifier used throughout the agent:
// the SHA-1 hash of the DER-encoded certificate, uppercase hex (F1 §3.1).
//
// This is a legacy X.509 identification convention — the same one Windows' own
// certificate UI shows, and the one internal/keysource/windowscng computes for
// the same certificates — not a cryptographic use of SHA-1, so it does not
// conflict with SPEC §12.4's and §18.8's prohibition on producing SHA-1 in a
// signature. It has to be byte-identical to the CNG backend's answer, because
// deduplicating one card seen through two backends is done on this value and
// nothing else (F11 §4).
func thumbprint(der []byte) string {
	sum := sha1.Sum(der)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// Thumbprint is thumbprint for callers outside this package, typed as the
// identifier the rest of the agent uses.
//
// It is exported because the worker (F12 §2) turns a certificate the child sent
// as raw DER into a keysource.Certificate, and computing the identifier there
// would make a third private copy of four lines whose whole requirement is that
// every copy agree. Two already exist — this one and
// internal/keysource/windowscng's — and F11 §4 step 3 collapses one card seen
// through both into one row on exactly this value.
//
// D-310 measured that the two agree on a real card: a Pošta certificate read
// through aetpkss1.dll and the same certificate read out of the Windows store
// both give AF5063BB74378BD503AB46DD08AEAD205BA2AA54. That is a measurement of
// two implementations, and the way to keep it true is to stop adding them.
func Thumbprint(der []byte) keysource.Thumbprint {
	return keysource.Thumbprint(thumbprint(der))
}
