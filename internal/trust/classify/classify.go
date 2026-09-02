package classify

import (
	"crypto/sha1" //nolint:gosec // certificate thumbprint identifier, not a signature — see the identical note in internal/keysource/windowscng/thumbprint.go
	"crypto/x509"
	"encoding/hex"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// thumbprint computes the SHA-1 hash of the DER certificate, uppercase
// hex — the identifier used everywhere in the agent (SPEC §11, F1 §3.1).
// Duplicated from internal/keysource/windowscng's identical helper
// rather than imported: internal/trust must not import anything from
// internal/ (SPEC §4.2 rule 3), including internal/keysource.
func thumbprint(der []byte) string {
	sum := sha1.Sum(der) //nolint:gosec
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// computeUsable answers "can this certificate sign right now" (F1 §5.5).
// Revocation checking is not part of F1: it requires network calls and
// arrives in F3, where the result must be embedded in the document.
func computeUsable(purpose Purpose, notBefore, notAfter, now time.Time, onHardware, hardwarePresent bool) (bool, errs.Code) {
	if purpose != PurposeSigning {
		return false, errs.CodeCertNotUsable
	}
	if now.Before(notBefore) || now.After(notAfter) {
		return false, errs.CodeCertExpired
	}
	if onHardware && !hardwarePresent {
		return false, errs.CodeCardNotPresent
	}
	return true, ""
}

// Classify turns a parsed certificate into an Info. list may be nil
// (no Trusted List available yet), in which case Qualification is
// QualificationUnknown rather than a guess. now is the reference time
// for both certificate validity and TSL status (F1 §4.3: a service
// withdrawn today may have been granted when a document was signed
// years ago) — pass time.Now() in production, a fixed time in tests.
func Classify(cert *x509.Certificate, list *tsl.List, onHardware, hardwarePresent bool, now time.Time) Info {
	subject := parseSubject(cert)
	purpose := purposeFromKeyUsage(cert.KeyUsage)

	qualification, onQSCDFromTSL := qualify(cert, list, now)
	onQSCD := onQSCDFromTSL || hasQcSSCD(cert)

	usable, reason := computeUsable(purpose, cert.NotBefore, cert.NotAfter, now, onHardware, hardwarePresent)

	return Info{
		Thumbprint:      thumbprint(cert.Raw),
		Subject:         subject,
		IssuerCN:        cert.Issuer.CommonName,
		NotBefore:       cert.NotBefore,
		NotAfter:        cert.NotAfter,
		Qualification:   qualification,
		Purpose:         purpose,
		OnQSCD:          onQSCD,
		Usable:          usable,
		NotUsableReason: reason,
	}
}
