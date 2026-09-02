// Package classify turns a raw certificate into what the agent can
// display and the signing layer can trust (F1 §5). This is where SPEC
// §11's measured, issuer-specific facts about Serbian certificates
// become code.
//
// internal/trust must not import anything from internal/ except
// internal/errs (SPEC §4.2 rule 3, enforced by scripts/checkdeps):
// trust evaluation is pure and independently testable, and internal/errs
// is a dependency-free leaf carrying only the error-code vocabulary, so
// importing it does not compromise that property (see docs/decisions.md,
// the entry superseding D-020).
package classify

import (
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// Qualification is the result of checking a certificate's issuer against
// the Trusted List (SPEC §11.1).
type Qualification int

const (
	QualificationUnknown Qualification = iota
	// QualificationQualified means the certificate's issuer matches a
	// TSL service of type CA/QC that was granted at the reference time.
	QualificationQualified
	QualificationNotQualified
)

// Purpose is derived from KeyUsage alone (SPEC §11.4) — never from
// digitalSignature, which Halcom's signing certificates do not set.
type Purpose int

const (
	PurposeUnknown Purpose = iota
	PurposeSigning
	PurposeAuthentication
)

// Subject is the personal-data-scrubbed subject information extracted
// from a certificate's DN (SPEC §11.6/§11.7). Everything in it must be
// safe to display and to write to a visual stamp — see the "no 13 digits"
// and "no @" tests in classify_test.go.
type Subject struct {
	// DisplayName is built from GivenName + Surname, never from CN.
	DisplayName string

	GivenName string
	Surname   string

	// CommonName is kept raw for technical display only.
	CommonName string

	Organisation string
	TaxID        string // from "VATRS-..."
	CompanyID    string // from "MB:RS-..."

	// IssuerAssignedID is the CA's internal reference, from "CA:RS-...".
	// It is NOT the national identity number.
	IssuerAssignedID string

	Country  string
	Locality string
}

// Info is everything the agent knows about one certificate.
type Info struct {
	Thumbprint string
	Subject    Subject
	IssuerCN   string
	NotBefore  time.Time
	NotAfter   time.Time

	Qualification Qualification
	Purpose       Purpose

	// OnQSCD is true when the certificate asserts QcSSCD, or when the
	// matching TSL service carries a QCWithQSCD qualifier.
	OnQSCD bool

	// Usable is true when the certificate can be used to sign right now:
	// correct purpose, within validity, hardware present.
	Usable bool

	// NotUsableReason explains Usable == false. Empty when Usable is true.
	NotUsableReason errs.Code

	// IsTestKey is true when this certificate comes from the soft token
	// (F2 §3.1) rather than real hardware. Classify itself has no way to
	// know this from the certificate bytes alone — callers that build an
	// Info for a soft-token certificate set this field afterwards, the
	// same way internal/cli already overwrites Thumbprint with the
	// enumeration-supplied value. SPEC §16.6 requires every signature
	// produced with such a certificate to be visibly marked as a test
	// signature everywhere downstream; this field is the origin of that
	// mark for anything built on top of classify.Info.
	IsTestKey bool
}
