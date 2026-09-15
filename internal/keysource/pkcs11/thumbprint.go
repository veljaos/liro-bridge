package pkcs11

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
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
