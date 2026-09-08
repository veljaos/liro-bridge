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

	// CodeOutputInUse means the signed document could not replace the
	// file already at the output path because another program has that
	// file open. It is distinct from OUTPUT_WRITE_FAILED, which is
	// every other reason a write did not happen (a full disk, a
	// read-only folder), because the two need different things from the
	// person: this one needs a file closed, and nothing else will do.
	//
	// It exists because the signed document is now written to a
	// temporary file in the destination's own directory and renamed over
	// the target (J-8), so that the destination is never observed
	// half-written and a failed write never destroys a previously good
	// signed file. Measured: os.Rename over a destination any other
	// program has open — even only for reading — fails on Windows with
	// "Access is denied", where the old os.WriteFile succeeded. Refusing
	// is the accepted trade; reporting the refusal as "access denied"
	// would not be.
	CodeOutputInUse Code = "OUTPUT_IN_USE"

	// CodeTSAClientCertUnreadable means the configured TSA client
	// certificate file could not be read at all — the path is wrong or
	// unreadable. Distinct from CodeTSAClientCertInvalid below because
	// the two need different things from the user: a corrected path
	// versus a corrected password (D-104).
	CodeTSAClientCertUnreadable Code = "TSA_CLIENT_CERT_UNREADABLE"

	// CodeInputUnreadable means the document to sign could not be read:
	// it is open exclusively in another program (Acrobat holds a lock
	// while a file is open for editing), it was deleted or moved after
	// being added to the list, or it lives on a network drive that has
	// gone away mid-batch. F6 §7 asks each of these to be recognised and
	// named rather than surfacing as an unclassified failure — and none
	// of them is a problem with the card, which is where SIGN_FAILED's
	// own message would send the user.
	CodeInputUnreadable Code = "INPUT_UNREADABLE"

	// CodeTSAClientCertInvalid means the file was read but could not be
	// opened as a PKCS#12 key pair — almost always a wrong password.
	CodeTSAClientCertInvalid Code = "TSA_CLIENT_CERT_INVALID"

	// CodeRequestInvalid means the request itself is not well formed:
	// the wrong HTTP method, a body that is not JSON, a required field
	// missing, a field whose value is not one this endpoint accepts.
	// Details may carry {"field": "..."} naming the offending field —
	// a structured fact, never prose (SPEC §7).
	//
	// It is deliberately one code rather than one per malformed field:
	// every one of them needs the same thing from the caller, which is
	// to fix its request, and a code per field would be a second
	// vocabulary growing without limit alongside the first.
	CodeRequestInvalid Code = "REQUEST_INVALID"

	// CodeRateLimited means the caller asked for something more often
	// than the agent will serve it (F7 §10). Details carry
	// {"retryAfterSeconds": N}. Distinct from PAIRING_IN_PROGRESS,
	// which is not about how often this caller asked but about somebody
	// else's window already being open.
	CodeRateLimited Code = "RATE_LIMITED"

	// CodePairingInProgress means a pairing window is already open for
	// some application, and the agent shows exactly one at a time (F7
	// §10). The caller should try again shortly; nothing is wrong with
	// its request.
	CodePairingInProgress Code = "PAIRING_IN_PROGRESS"

	// CodePairingExpired means the pairing request named cannot be used
	// any more: its five minutes are up, five wrong codes voided it, it
	// was already confirmed, or there is no such request at all. All
	// four need the same thing — start a new pairing request — and
	// folding "no such request" in with the rest is deliberate, so that
	// guessing request identifiers tells a caller nothing about which
	// ones exist.
	CodePairingExpired Code = "PAIRING_EXPIRED"

	// CodePairingCodeIncorrect means the six-digit code did not match
	// and the request is still live. Details carry
	// {"attemptsRemaining": N} so a caller can tell the person how many
	// tries are left before the request is voided (F7 §2.1).
	CodePairingCodeIncorrect Code = "PAIRING_CODE_INCORRECT"

	// CodePairingOriginMismatch means confirm arrived from a different
	// origin than the one that requested the pairing (F7 §2.1). It is
	// its own code rather than folded into PAIRING_EXPIRED because it
	// is almost always an integration mistake — two calls made with two
	// different declared origins — and it reveals nothing an attacker
	// does not already have, since the caller supplied the request
	// identifier it is being told about.
	CodePairingOriginMismatch Code = "PAIRING_ORIGIN_MISMATCH"

	// CodePairingDenied means the person refused the pairing at the
	// agent's own window, or closed it. Distinct from PAIRING_EXPIRED
	// because the reaction differs: an expired request is worth
	// retrying, a refused one is an answer.
	CodePairingDenied Code = "PAIRING_DENIED"

	// CodeJobInProgress means the application already has a signing job
	// that has not finished, and the agent serves one at a time per
	// application (F7 §10). Nothing is wrong with the request; it is
	// too early. Distinct from RATE_LIMITED, which is about how often
	// the caller asked rather than about what it is already doing, and
	// from PAIRING_IN_PROGRESS, which is about somebody else's window.
	CodeJobInProgress Code = "JOB_IN_PROGRESS"

	// CodeJobNotFound means there is no job with that identifier: it
	// never existed, its result has already been collected — the result
	// is delivered exactly once (F7 §7.3) — or it finished and nobody
	// came back for it inside the ten minutes. All three need the same
	// thing from the caller, which is to submit again, and folding them
	// together means guessing identifiers reveals nothing about which
	// jobs exist.
	CodeJobNotFound Code = "JOB_NOT_FOUND"

	// CodeDocumentSigningDisabled means POST /v2/sign/pdf is switched
	// off on this machine (F7 §6: the whole-document path is optional,
	// and a deployment that only serves Liro's own applications does not
	// need it). It is its own code rather than NOT_PAIRED or
	// REQUEST_INVALID because neither of those is true and neither
	// suggests the one thing that would help: asking the person at the
	// machine to turn it on.
	CodeDocumentSigningDisabled Code = "DOCUMENT_SIGNING_DISABLED"

	// CodeCertificateListingDisabled means GET /v2/certificates is
	// switched off on this machine. The listing says who is at the
	// machine and which qualified certificates they hold, and on a
	// bookkeeper's machine that is several clients rather than one
	// (SPEC §14.1) — so the person at it can decline to answer, and an
	// application that asks is told so rather than being told it is
	// unpaired, which is not true and would send it to re-pair for no
	// gain.
	CodeCertificateListingDisabled Code = "CERTIFICATE_LISTING_DISABLED"

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
		CodeOutputInUse,
		CodeInputUnreadable,
		CodeTSAClientCertUnreadable,
		CodeTSAClientCertInvalid,
		CodeRequestInvalid,
		CodeRateLimited,
		CodePairingInProgress,
		CodePairingExpired,
		CodePairingCodeIncorrect,
		CodePairingOriginMismatch,
		CodePairingDenied,
		CodeJobInProgress,
		CodeJobNotFound,
		CodeDocumentSigningDisabled,
		CodeCertificateListingDisabled,
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
