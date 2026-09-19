// Package cli implements the agent's command-line interface. In F1 this
// is exactly one command, "certs" (F1 §6): it enumerates certificates,
// classifies them against the Trusted List, and prints the result. It
// never signs anything and never opens a session that could prompt for
// a PIN.
package cli

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
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

	// ModuleCertificates supplies certificates read through PKCS#11 modules,
	// each carrying which module it was seen through, together with one
	// failure per module that could not be asked. Nil on a build with no
	// PKCS#11 path, in which case Gather behaves exactly as it did before.
	//
	// It is separate from ExtraCertificates rather than folded into it because
	// the two differ on the one fact that decides whether a certificate is
	// usable: an extra certificate is never on hardware by definition (the
	// soft token, F2 §3), and a certificate read off a token always is.
	//
	// The failures are returned rather than logged because F11 §3 asks for a
	// module that could not be read to be a row in the report, not a silence.
	ModuleCertificates func(ctx context.Context) ([]ModuleCertificate, []ModuleFailure, error)
}

// The backend names a row reports, which are keysource.Source.Name()'s values.
// Constants here rather than literals at four call sites, because the whole
// value of the field is that a reader can compare two rows with it.
const (
	backendCNG       = "windows-cng"
	backendPKCS11    = "pkcs11"
	backendSoftToken = "softtoken"
)

// ModuleCertificate is one certificate read through one PKCS#11 module.
//
// The type is declared here, in neutral terms, rather than reusing
// internal/keysource/pkcs11's: this package classifies and reports, and it has
// no business importing a backend in order to describe what a backend found.
type ModuleCertificate struct {
	Thumbprint string
	DER        []byte

	// ModulePath is which module saw it. Two builds of one vendor's module can
	// be installed at once and see the same card identically (D-271), so this
	// is the only thing that tells two sightings of one certificate apart.
	ModulePath string
}

// ModuleFailure is one module that could not be asked, and why.
type ModuleFailure struct {
	Path   string
	Origin string
	Reason string
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

	// Backends are the backends that offered this certificate, in the order
	// they were asked: "windows-cng" first, then "pkcs11", then "softtoken".
	//
	// One card can be visible through more than one of them at once — measured
	// on this project's own machine, where a Pošta certificate read through
	// aetpkss1.dll and the same certificate read out of the Windows store give
	// the identical thumbprint (D-310). **That is one certificate and it is one
	// row**, which is F11 §4 step 3; this field is what stops the collapse
	// losing the fact that there were two sightings.
	//
	// It matters beyond bookkeeping: D-311 decides that CNG signs when it
	// offers the certificate, so a row listing both backends is a row that will
	// be signed through the first of them, and a person asking why their
	// PKCS#11 module is not being used has this to read.
	Backends []string

	// Modules are the PKCS#11 module paths that offered this certificate, in
	// discovery order. Empty for a certificate no module saw.
	Modules []string
}

// Report is everything "liro-bridge certs" prints.
type Report struct {
	Readers      []platform.ReaderState
	Certificates []CertRow
	TSL          tsl.Provenance

	// ModuleFailures are the PKCS#11 modules that could not be asked. Empty on
	// every machine where all of them answered, and on every build with no
	// PKCS#11 path.
	//
	// They are in the report rather than in a log because the person who needs
	// them is the person looking at a list that does not contain their
	// certificate, and the reason it does not is that a module would not load
	// (F11 §3).
	ModuleFailures []ModuleFailure
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
		return Report{}, readerListingError(err)
	}

	certs, err := deps.Enumerate(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("enumerating certificates: %w", err)
	}

	list, provenance, err := deps.Store.Current(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("reading trusted list: %w", err)
	}

	// One memory of probe answers for this listing and no longer (J-7).
	// A certificate does not appear and disappear while a single list is
	// being enumerated, so asking about the same one twice is waste — and
	// it is expensive waste: the probe measured 457 ms for a certificate
	// whose card is present and 855 ms for one whose card is not, per
	// call. It is deliberately not carried across listings: a card really
	// can be inserted between one `certs` and the next, and SPEC §11.10
	// exists because reporting a certificate as available when it is not
	// produces the worst outcome in this product.
	probes := newPresenceMemo()

	// Modules that could not be asked. F11 §3: a module that will not load is
	// a row to show and carry past, never a reason for a listing to fail.
	var failures []ModuleFailure

	rows := make([]CertRow, 0, len(certs))
	for _, c := range certs {
		x, err := x509.ParseCertificate(c.DER)
		if err != nil {
			continue // not this phase's concern to explain a malformed store entry
		}
		info := classify.Classify(x, list, c.OnHardware, probes.presence(ctx, deps, c), now)
		info.Thumbprint = c.Thumbprint // identical to classify's own computation; use the source value
		rows = append(rows, CertRow{
			Info: info, OnHardware: c.OnHardware, DER: c.DER,
			Backends: []string{backendCNG},
		})
	}

	// One card seen through two backends is one certificate and one row, and
	// the thumbprint is what makes that possible: it is the SHA-1 of the same
	// DER bytes whichever backend read them, which D-310 measured on real
	// hardware rather than leaving as an argument about how SHA-1 works.
	//
	// The collapse happens here, above both backends, rather than inside
	// either: doing it in the PKCS#11 layer would deduplicate one dimension
	// and not the other, which is harder to reason about than not
	// deduplicating at all.
	if deps.ModuleCertificates != nil {
		moduleCerts, moduleFailures, err := deps.ModuleCertificates(ctx)
		if err != nil {
			return Report{}, fmt.Errorf("enumerating certificates through PKCS#11 modules: %w", err)
		}
		failures = moduleFailures
		byThumbprint := make(map[string]int, len(rows))
		for i, r := range rows {
			byThumbprint[r.Info.Thumbprint] = i
		}
		for _, c := range moduleCerts {
			if i, seen := byThumbprint[c.Thumbprint]; seen {
				rows[i].Backends = append(rows[i].Backends, backendPKCS11)
				rows[i].Modules = append(rows[i].Modules, c.ModulePath)
				// A module enumerated this certificate off a token, so the card
				// is in the reader: PKCS#11 enumeration only ever looks at
				// slots with a token present. That is a better answer than the
				// presence probe's, and it is evidence rather than an opinion —
				// the bytes came off the card.
				//
				// Re-classified rather than patched: Usable and
				// NotUsableReason are computed together from purpose, dates and
				// presence, and reaching in to set one of them would leave a
				// row whose reason contradicted its verdict. Only when the
				// existing row is itself on hardware — a software copy of the
				// same certificate is a different thing about which a card in a
				// reader says nothing.
				if rows[i].OnHardware {
					if x, err := x509.ParseCertificate(c.DER); err == nil {
						reclassified := classify.Classify(x, list, true, true, now)
						reclassified.Thumbprint = rows[i].Info.Thumbprint
						reclassified.IsTestKey = rows[i].Info.IsTestKey
						rows[i].Info = reclassified
					}
				}
				continue
			}
			x, err := x509.ParseCertificate(c.DER)
			if err != nil {
				continue // not this layer's job to explain a malformed object on a card
			}
			info := classify.Classify(x, list, true, true, now)
			info.Thumbprint = c.Thumbprint
			byThumbprint[c.Thumbprint] = len(rows)
			rows = append(rows, CertRow{
				Info:       info,
				OnHardware: true,
				DER:        c.DER,
				Backends:   []string{backendPKCS11},
				Modules:    []string{c.ModulePath},
			})
		}
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
			rows = append(rows, CertRow{
				Info: info, OnHardware: false, DER: c.DER,
				Backends: []string{backendSoftToken},
			})
		}
	}

	return Report{Readers: readers, Certificates: rows, TSL: provenance, ModuleFailures: failures}, nil
}

// readerListingError gives a failed reader listing the code SPEC §7
// already has for it.
//
// SMART_CARD_SERVICE_DOWN was defined in F1 and never once produced:
// platform.ErrSmartCardServiceDown travelled as a bare wrapped error and
// every caller above treated it as an unclassified failure. That is not
// a cosmetic gap. It is the state windows-latest is permanently in —
// there is no reader on a CI runner and the service is not running —
// and it is the state of any machine where the agent is installed before
// the reader is plugged in. Measured there: `liro-bridge sign` gave up
// 1.011s in with no window and nothing on stdout, because the only thing
// that knew why was an error string nobody could branch on (D-236).
func readerListingError(err error) error {
	code := errs.CodeInternal
	if errors.Is(err, platform.ErrSmartCardServiceDown) {
		code = errs.CodeSmartCardServiceDown
	}
	return errs.New(code, fmt.Errorf("listing smart card readers: %w", err))
}

// NothingUsableReason answers the one question a screen offering no
// signing certificate has to answer: why not. It returns "" when at
// least one offered certificate can sign right now, and otherwise the
// SPEC §7 code for the reason, in the order a person can act on:
//
//	NO_READER          nothing to put a card into
//	CARD_NOT_PRESENT   a reader, and no card in it
//	CERT_NOT_FOUND     a card, and nothing on it this agent can offer
//	<the row's own>     certificates offered, none of them usable, all
//	                    for the same reason — CERT_EXPIRED, say
//	CERT_NOT_USABLE    offered, unusable, and not all for one reason
//
// It is a method on the report rather than a function in the window
// because everything it needs is here — the reader states and Hidden(),
// which is the same rule the window's own list is built from — and
// because the next screen that has to explain an empty list should
// answer this question the same way rather than a second way (D-108,
// D-124, D-138). Hidden rows are not offered to anybody, so they are not
// evidence that there is something to sign with.
func (r Report) NothingUsableReason() errs.Code {
	offered := make([]CertRow, 0, len(r.Certificates))
	for _, row := range r.Certificates {
		if row.Hidden() {
			continue
		}
		if row.Info.Usable {
			return ""
		}
		offered = append(offered, row)
	}

	if len(offered) > 0 {
		reason := offered[0].Info.NotUsableReason
		for _, row := range offered[1:] {
			if row.Info.NotUsableReason != reason {
				return errs.CodeCertNotUsable
			}
		}
		if reason == "" {
			return errs.CodeCertNotUsable
		}
		return reason
	}

	if len(r.Readers) == 0 {
		return errs.CodeNoReader
	}
	for _, reader := range r.Readers {
		if reader.CardPresent {
			return errs.CodeCertNotFound
		}
	}
	return errs.CodeCardNotPresent
}

// presenceMemo is one listing's answers to the presence question, keyed
// by thumbprint (J-7).
//
// It is created inside Gather and thrown away with it, which is the
// whole of its scope. Two separate listings ask twice, on purpose: a
// card can be inserted or removed between them, and a stale "present"
// is the failure SPEC §11.10 is written to prevent — the agent offers
// to sign, the user clicks, enters a PIN, and fails five seconds later.
//
// Probing is never done concurrently. D-027 rejected concurrent
// smart-card access for signing, on the grounds that the card is a
// single serial device whose driver queues requests anyway and that
// concurrent access is a known source of driver-level failures; a probe
// goes through the same middleware and the same card, so the same
// reasoning applies. This memo removes repeated work; it does not
// overlap the work that remains.
type presenceMemo struct {
	answers map[string]bool
}

func newPresenceMemo() *presenceMemo {
	return &presenceMemo{answers: make(map[string]bool)}
}

// presence answers Task 2's per-certificate presence question: for a
// hardware-backed certificate, it calls deps.PresenceCheck for that
// certificate alone, rather than reusing one machine-wide answer for
// every certificate (the bug D-077 fixes — one card inserted used to
// mark every hardware-backed certificate as available). A software-backed
// certificate is never checked: computeUsable ignores this value
// entirely when onHardware is false, so probing it would be pure waste.
//
// The answer is remembered for the rest of this listing, so a store
// holding the same certificate under more than one entry costs one
// probe rather than one per entry.
//
// A PresenceCheck failure (as opposed to a clean "false" result) is
// treated conservatively as "not present" rather than propagated as a
// fatal Gather error — a single certificate's probe failing (e.g. the
// certificate having vanished from the store between enumeration and the
// probe) must not make the whole `certs` command fail; it is logged so
// the cause is not silently lost. That answer is remembered too: a probe
// that failed once in this listing will fail again in it, and asking
// twice only pays the cost twice.
func (m *presenceMemo) presence(ctx context.Context, deps Deps, c windowscng.Certificate) bool {
	if !c.OnHardware {
		return false
	}
	if answer, ok := m.answers[c.Thumbprint]; ok {
		return answer
	}
	present, err := deps.PresenceCheck(ctx, keysource.Thumbprint(c.Thumbprint))
	if err != nil {
		slog.Warn("cli: per-certificate presence check failed, treating as not present",
			"thumbprint", c.Thumbprint, "error", err)
		present = false
	}
	m.answers[c.Thumbprint] = present
	return present
}
