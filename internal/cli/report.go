// Package cli implements the agent's command-line interface. In F1 this
// is exactly one command, "certs" (F1 §6): it enumerates certificates,
// classifies them against the Trusted List, and prints the result. It
// never signs anything and never opens a session that could prompt for
// a PIN.
package cli

import (
	"context"
	"crypto/x509"
	"fmt"
	"log/slog"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// Deps supplies certs with everything it needs, as functions rather than
// concrete types, so tests can exercise the reporting and rendering
// logic with fakes instead of real hardware or a real network fetch —
// the Windows APIs and TSL network fetch cannot be meaningfully unit
// tested (F1 §2.5/§3.6), but everything built on top of them can be.
type Deps struct {
	// Readers answers "is there a reader at all, and does it currently
	// hold a card" (F1 §6, unchanged by Task 2) — shown as its own
	// section of the report, independent of any certificate.
	Readers func(ctx context.Context) ([]platform.ReaderState, error)

	// PresenceCheck answers a different, narrower question per
	// certificate: is *this* certificate's own card currently present
	// (Task 2 / SPEC §11.10 updated). Unlike a single machine-wide
	// answer, this is called once per hardware-backed certificate, so
	// one card inserted does not mark every hardware-backed certificate
	// on the machine as available — the bug this task fixes. It must
	// never prompt for a PIN (windowscng.Source.Presence's own
	// contract: opening a key handle is silent).
	PresenceCheck func(ctx context.Context, thumbprint keysource.Thumbprint) (bool, error)

	Enumerate func(ctx context.Context) ([]windowscng.Certificate, error)
	Store     tsl.Store

	// ExtraCertificates supplies certificates from a source other than
	// the Windows CNG store — in this phase, the soft token (F2 §3),
	// wired in only when it is configured (LIRO_SOFTTOKEN_P12 set and
	// the binary built with the "softtoken" tag). Nil otherwise, in
	// which case Gather behaves exactly as it did in F1. Certificates it
	// returns are never on hardware; ExtraCertificate.IsTestKey flows
	// into the resulting row's classify.Info.IsTestKey, which is how
	// "certs --json" (F2 §6.1's OpenSSL recipe) can list and export the
	// soft token's public certificate exactly like a real one.
	ExtraCertificates func(ctx context.Context) ([]ExtraCertificate, error)
}

// ExtraCertificate is a certificate from a source other than the
// Windows CNG store (F2 §3) — currently only the soft token.
type ExtraCertificate struct {
	Thumbprint string
	DER        []byte
	IsTestKey  bool
}

// CertRow is one certificate, classified. OnHardware and DER are
// carried alongside Info (rather than inside it — Info is the
// caller-facing shape F1 §5.1 defines) because the CLI's "Storage" row
// and --json's PEM export (F2 §6.1) need them and Classify does not
// store its own input back onto the result.
type CertRow struct {
	Info       classify.Info
	OnHardware bool
	DER        []byte
}

// Report is everything "liro-bridge certs" prints.
type Report struct {
	Readers      []platform.ReaderState
	Certificates []CertRow
	TSL          tsl.Provenance
}

// Hidden reports whether row is hidden from the default (non --all)
// view: a Windows-internal artefact (self-signed, GUID subject,
// software KSP, unknown to the Trusted List), or a certificate whose
// purpose is not signing — the authentication certificate every
// Serbian card carries beside the signing one, which shows the person
// their own name a second time and is not a choice they can make.
//
// The rule itself lives in classify.Info, not here, so that this
// command, the consent window and the Certificates window share one
// implementation rather than three copies that can drift (F6 §0b).
func (r CertRow) Hidden() bool {
	return r.Info.HiddenByDefault()
}

// Gather assembles a Report: readers, enumerated certificates
// classified against the Trusted List's current state. It attempts one
// best-effort Trusted List refresh first (F1 §4.8: "refresh at
// startup"); a failed refresh is not fatal and is not returned as an
// error — the existing (embedded or cached) list is used instead, with
// its own staleness visible in Report.TSL.
//
// No PIN is ever requested: nothing here opens a signing session.
func Gather(ctx context.Context, deps Deps, now time.Time) (Report, error) {
	_ = deps.Store.Refresh(ctx) // best-effort; failure is never fatal (F1 §4.8)

	readers, err := deps.Readers(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("listing smart card readers: %w", err)
	}

	certs, err := deps.Enumerate(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("enumerating certificates: %w", err)
	}

	list, provenance, err := deps.Store.Current(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("reading trusted list: %w", err)
	}

	rows := make([]CertRow, 0, len(certs))
	for _, c := range certs {
		x, err := x509.ParseCertificate(c.DER)
		if err != nil {
			continue // not this phase's concern to explain a malformed store entry
		}
		info := classify.Classify(x, list, c.OnHardware, certPresence(ctx, deps, c), now)
		info.Thumbprint = c.Thumbprint // identical to classify's own computation; use the source value
		rows = append(rows, CertRow{Info: info, OnHardware: c.OnHardware, DER: c.DER})
	}

	if deps.ExtraCertificates != nil {
		extra, err := deps.ExtraCertificates(ctx)
		if err != nil {
			return Report{}, fmt.Errorf("enumerating extra certificates: %w", err)
		}
		for _, c := range extra {
			x, err := x509.ParseCertificate(c.DER)
			if err != nil {
				continue
			}
			// onHardware is always false for an extra certificate — the
			// soft token is never on hardware by definition (F2 §3) —
			// so hardwarePresent is irrelevant to computeUsable's result
			// and no presence probe is made.
			info := classify.Classify(x, list, false, false, now)
			info.Thumbprint = c.Thumbprint
			info.IsTestKey = c.IsTestKey
			rows = append(rows, CertRow{Info: info, OnHardware: false, DER: c.DER})
		}
	}

	return Report{Readers: readers, Certificates: rows, TSL: provenance}, nil
}

// certPresence answers Task 2's per-certificate presence question: for a
// hardware-backed certificate, it calls deps.PresenceCheck for that
// certificate alone, rather than reusing one machine-wide answer for
// every certificate (the bug this task fixes — one card inserted used to
// mark every hardware-backed certificate as available). A software-backed
// certificate is never checked: computeUsable ignores this value
// entirely when onHardware is false, so probing it would be pure waste.
//
// A PresenceCheck failure (as opposed to a clean "false" result) is
// treated conservatively as "not present" rather than propagated as a
// fatal Gather error — a single certificate's probe failing (e.g. the
// certificate having vanished from the store between enumeration and the
// probe) must not make the whole `certs` command fail; it is logged so
// the cause is not silently lost.
func certPresence(ctx context.Context, deps Deps, c windowscng.Certificate) bool {
	if !c.OnHardware {
		return false
	}
	present, err := deps.PresenceCheck(ctx, keysource.Thumbprint(c.Thumbprint))
	if err != nil {
		slog.Warn("cli: per-certificate presence check failed, treating as not present",
			"thumbprint", c.Thumbprint, "error", err)
		return false
	}
	return present
}
