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
	CodePINRequired    Code = "PIN_REQUIRED"
	CodePINIncorrect   Code = "PIN_INCORRECT"
	CodePINLocked      Code = "PIN_LOCKED"
	CodeCertNotFound   Code = "CERT_NOT_FOUND"
	CodeCertExpired    Code = "CERT_EXPIRED"
	CodeCertNotUsable  Code = "CERT_NOT_USABLE"
	CodeCertRevoked    Code = "CERT_REVOKED"
	CodeTSAUnavailable Code = "TSA_UNAVAILABLE"
	CodeTSARejected    Code = "TSA_REJECTED"
	CodePDFInvalid     Code = "PDF_INVALID"
	CodePDFEncrypted   Code = "PDF_ENCRYPTED"
	CodeSignFailed     Code = "SIGN_FAILED"
	CodeVersionTooOld  Code = "VERSION_TOO_OLD"
	CodeInternal       Code = "INTERNAL"
)

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
