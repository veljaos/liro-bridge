// Package errs implements the error model from SPEC §7: errors that cross
// an API boundary are stable machine-readable codes, never human-readable
// messages. Translation happens in the SDK or the UI, never here.
package errs

import "fmt"

// Code is a stable, machine-readable error identifier. Codes are never
// removed or repurposed; new situations get new codes.
type Code string

const (
	CodeNotPaired      Code = "NOT_PAIRED"
	CodeAuthFailed     Code = "AUTH_FAILED"
	CodeConsentDenied  Code = "CONSENT_DENIED"
	CodeConsentTimeout Code = "CONSENT_TIMEOUT"
	CodeNoReader       Code = "NO_READER"
	CodeCardNotPresent Code = "CARD_NOT_PRESENT"

	// CodeSmartCardServiceDown means the OS smart card service is not
	// running. This is different from NO_READER: the fix is to start a
	// Windows service, not to plug in hardware (F1 §2.3).
	CodeSmartCardServiceDown Code = "SMART_CARD_SERVICE_DOWN"
	CodePINRequired          Code = "PIN_REQUIRED"
	CodePINIncorrect         Code = "PIN_INCORRECT"
	CodePINLocked            Code = "PIN_LOCKED"
	CodeCertNotFound         Code = "CERT_NOT_FOUND"
	CodeCertExpired          Code = "CERT_EXPIRED"
	CodeCertNotUsable        Code = "CERT_NOT_USABLE"
	CodeCertRevoked          Code = "CERT_REVOKED"
	CodeTSAUnavailable       Code = "TSA_UNAVAILABLE"
	CodeTSARejected          Code = "TSA_REJECTED"
	CodePDFInvalid           Code = "PDF_INVALID"
	CodePDFEncrypted         Code = "PDF_ENCRYPTED"
	CodeSignFailed           Code = "SIGN_FAILED"

	// CodeStampGlyphMissing means the visual signature stamp needs a
	// character the embedded font subset does not contain. This is
	// unrelated to the card, the reader or the signing operation itself
	// (Task 2) — SIGN_FAILED's own message ("the card failed to produce
	// a signature") sends the user to check hardware for a problem that
	// has nothing to do with hardware.
	CodeStampGlyphMissing Code = "STAMP_GLYPH_MISSING"
	CodeVersionTooOld     Code = "VERSION_TOO_OLD"

	// CodeOutputExists means the output file the signature would be
	// written to is already there, and nothing was overwritten (SPEC
	// §12.11/§18.10: the original is never silently replaced). It is a
	// refusal with an obvious cause and an obvious remedy, not an
	// unclassified failure — reaching the user as INTERNAL's "an
	// unexpected error occurred" made a deliberate protection look like
	// a defect (D-104).
	CodeOutputExists Code = "OUTPUT_EXISTS"

	// CodeOutputWriteFailed means the signed bytes could not be written
	// to the output path: a full disk, a read-only folder, a file held
	// open by another program. The signature itself succeeded, which is
	// why this is not SIGN_FAILED (D-104).
	CodeOutputWriteFailed Code = "OUTPUT_WRITE_FAILED"

	// CodeTSAClientCertUnreadable means the configured TSA client
	// certificate file could not be read at all — the path is wrong or
	// unreadable. Distinct from CodeTSAClientCertInvalid below because
	// the two need different things from the user: a corrected path
	// versus a corrected password (D-104).
	CodeTSAClientCertUnreadable Code = "TSA_CLIENT_CERT_UNREADABLE"

	// CodeTSAClientCertInvalid means the file was read but could not be
	// opened as a PKCS#12 key pair — almost always a wrong password.
	CodeTSAClientCertInvalid Code = "TSA_CLIENT_CERT_INVALID"

	CodeInternal Code = "INTERNAL"
)

// AllCodes lists every code this package defines, in declaration
// order. It exists so a test can assert that each one has a message in
// every locale catalogue: a code with no message renders its own key
// ("error.pin_locked") to the user, which is the failure this list
// makes impossible to introduce silently (D-104).
func AllCodes() []Code {
	return []Code{
		CodeNotPaired,
		CodeAuthFailed,
		CodeConsentDenied,
		CodeConsentTimeout,
		CodeNoReader,
		CodeCardNotPresent,
		CodeSmartCardServiceDown,
		CodePINRequired,
		CodePINIncorrect,
		CodePINLocked,
		CodeCertNotFound,
		CodeCertExpired,
		CodeCertNotUsable,
		CodeCertRevoked,
		CodeTSAUnavailable,
		CodeTSARejected,
		CodePDFInvalid,
		CodePDFEncrypted,
		CodeSignFailed,
		CodeStampGlyphMissing,
		CodeVersionTooOld,
		CodeOutputExists,
		CodeOutputWriteFailed,
		CodeTSAClientCertUnreadable,
		CodeTSAClientCertInvalid,
		CodeInternal,
	}
}

// Error is the only error type that crosses an API boundary. It carries a
// code and optional structured details — never a human-readable message.
// Translation happens in the SDK or the UI, not here.
type Error struct {
	Code    Code           `json:"code"`
	Details map[string]any `json:"details,omitempty"`

	// cause is the internal error. It is never serialised and never sent
	// to a client; it exists for logging.
	cause error
}

// Error returns an English description for logs. It is never sent to a
// client — only the Code and Details fields are serialised.
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.cause)
	}
	return string(e.Code)
}

// Unwrap returns the internal cause, so callers can use errors.Is/As
// against it. The cause itself is never serialised.
func (e *Error) Unwrap() error { return e.cause }

// New returns an Error with the given code and cause, and no details.
func New(code Code, cause error) *Error {
	return &Error{Code: code, cause: cause}
}

// WithDetails returns an Error with the given code, cause and structured,
// machine-readable details. Details must never carry prose.
func WithDetails(code Code, cause error, details map[string]any) *Error {
	return &Error{Code: code, Details: details, cause: cause}
}
