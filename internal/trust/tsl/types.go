// Package tsl fetches, verifies, caches and parses the Republic of
// Serbia's ETSI TS 119 612 Trusted List (SPEC §11.1, F1 §4).
//
// internal/trust must not import anything else under internal/ (SPEC
// §4.2 rule 3, enforced by scripts/checkdeps): trust evaluation is pure
// and independently testable. This package therefore has no dependency
// on internal/config, internal/errs or internal/platform — callers
// higher up the stack (the CLI, later the API) own configuration and
// error-code translation.
package tsl

import (
	"strings"
	"time"
)

// List is a parsed Trusted List. It answers only "what does the list
// say"; whether the list itself is trustworthy (signature verified,
// pinned signer, not a rollback) is decided before a List is ever handed
// out — see Verify and Store.
type List struct {
	Sequence  int
	IssuedAt  time.Time
	Providers []Provider
}

// Provider is one TrustServiceProvider entry.
type Provider struct {
	Name     string
	Services []Service
}

// Service is one TSPService entry: a single trust service (a CA, a TSA,
// ...) and the certificate that identifies it.
type Service struct {
	// Type is the full ServiceTypeIdentifier URI, e.g.
	// ".../Svctype/CA/QC". Use IsCA to test it.
	Type string

	Name string

	// Certificate is the DER-encoded ServiceDigitalIdentity certificate,
	// nil if the service has none (some TSA/QTST entries do not carry
	// one directly usable this way).
	Certificate []byte

	// Status is the full ServiceStatus URI, e.g. ".../Svcstatus/granted".
	// Use Granted to test it. StatusStart is when this status began.
	Status      string
	StatusStart time.Time

	// Qualifiers are the full Qualifier URIs found in
	// ServiceInformationExtensions, e.g. ".../SvcInfoExt/QCWithQSCD".
	Qualifiers []string

	// History holds status changes older than the current Status —
	// SPEC/F1 §4.3: "a service that is withdrawn today may have been
	// granted when a document was signed two years ago."
	History []HistoryEntry
}

// HistoryEntry is one ServiceHistoryInstance: a status the service held
// before its current one.
type HistoryEntry struct {
	Status      string
	StatusStart time.Time
}

// caQCSuffix and grantedSuffix are matched with strings.HasSuffix, not
// equality: F1 §4.3 measured that the Serbian list uses its own URI
// prefix (http://www.mit.gov.rs/TrstSvc/...) rather than the generic ETSI
// one, so only the final path segment is stable across lists.
const (
	caQCSuffix    = "/Svctype/CA/QC"
	grantedSuffix = "/Svcstatus/granted"
)

// IsCA reports whether this service is a qualified-certificate CA
// service (ServiceTypeIdentifier ending in /CA/QC).
func (s Service) IsCA() bool {
	return strings.HasSuffix(s.Type, caQCSuffix)
}

// Granted reports whether this service's current status is "granted".
func (s Service) Granted() bool {
	return StatusIsGranted(s.Status)
}

// StatusIsGranted reports whether a raw ServiceStatus URI (current or
// historical, e.g. from Service.StatusAt) means "granted". Exported so
// internal/trust/classify can apply the same test to a historical status
// without duplicating the URI suffix (F1 §5.4: qualification is decided
// against the status in effect at the reference time, not only the
// current one).
func StatusIsGranted(status string) bool {
	return strings.HasSuffix(status, grantedSuffix)
}

// HasQualifier reports whether one of this service's qualifier URIs ends
// with the given suffix, e.g. HasQualifier("QCWithQSCD").
func (s Service) HasQualifier(suffix string) bool {
	for _, q := range s.Qualifiers {
		if strings.HasSuffix(q, "/"+suffix) {
			return true
		}
	}
	return false
}

// StatusAt resolves the service's status as of time t, using History for
// dates before the current status began (F1 §4.9). It returns the empty
// string if t predates every known status change.
func (s Service) StatusAt(t time.Time) string {
	best := ""
	var bestStart time.Time
	consider := func(status string, start time.Time) {
		if start.After(t) {
			return
		}
		if best == "" || start.After(bestStart) {
			best = status
			bestStart = start
		}
	}
	consider(s.Status, s.StatusStart)
	for _, h := range s.History {
		consider(h.Status, h.StatusStart)
	}
	return best
}
