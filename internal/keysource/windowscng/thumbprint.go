package windowscng

import (
	"crypto/sha1" //nolint:gosec // SHA-1 here is a certificate thumbprint identifier, not a signature (SPEC §11: "Thumbprint is the SHA-1 hash of the DER certificate"). This is not the SHA-1-in-signatures prohibition of SPEC §12.4/§18.8.
	"encoding/hex"
	"strings"
)

// thumbprint computes the certificate identifier used throughout the
// agent: the SHA-1 hash of the DER-encoded certificate, uppercase hex
// (F1 §3.1). This is a legacy X.509 identification convention (the same
// one Windows' own certificate UI shows), not a cryptographic use of
// SHA-1, so it does not conflict with SPEC §12.4's "no SHA-1 in
// signatures" rule.
func thumbprint(der []byte) string {
	sum := sha1.Sum(der) //nolint:gosec
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
