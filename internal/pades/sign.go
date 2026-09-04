// Package pades orchestrates one document signing operation (F3 §9):
// placeholder reservation, CMS construction, an optional timestamp, and
// optional DSS embedding — tying together internal/pades/pdf,
// internal/pades/cms, internal/pades/tsa and internal/pades/dss, none
// of which know about each other. internal/pades/verify is deliberately
// never imported here (F3 §8): this package produces signatures, it
// never checks its own work.
package pades

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/pades/cms"
	"github.com/veljaos/liro-bridge/internal/pades/dss"
	"github.com/veljaos/liro-bridge/internal/pades/pdf"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
)

// Level is the PAdES conformance level actually achieved by one
// signing operation (F3 §12.6). Callers must report this, never the
// level that was merely requested (F3 §7.3/§18.11: no silent
// downgrade).
type Level string

const (
	LevelBB  Level = "B-B"
	LevelBT  Level = "B-T"
	LevelBLT Level = "B-LT"
)

// clockDriftWarnThreshold is Task 3's threshold for warning that the
// signer's machine clock (the /M value) disagrees with the qualified
// timestamp's genTime. This is deliberately below the TSA client's own
// ±10-minute hard skew window (internal/pades/tsa.maxSkew): that check
// protects the timestamp's own trustworthiness and aborts the whole
// operation when tripped; this one exists purely to tell the user their
// clock looks wrong before it embarrasses them on more documents, so it
// fires earlier and never blocks anything.
const clockDriftWarnThreshold = 5 * time.Minute

// Options configures SignDocument (F3 §9).
type Options struct {
	// ReservedBytes is the /Contents reservation. Zero means
	// pdf.DefaultReservedBytes (F3 §4.3).
	ReservedBytes int

	// RequestedLevel is B-T or B-LT (F3 §12.6: B-B is a fallback this
	// package reaches on TSA failure, never something requested
	// directly through this field — the CLI's own --level flag only
	// offers b-t/b-lt for exactly that reason).
	RequestedLevel Level

	// OnTSAFailureAbort selects F3 §6.4/§12.8's required choice: true
	// (the default) aborts the whole operation on TSA failure; false
	// saves at B-B instead, with Result.Notes recording why.
	OnTSAFailureAbort bool

	// TSA is the timestamp client. Nil means no timestamp is attempted
	// at all — the result is B-B unconditionally, matching
	// --on-tsa-failure=b-b's own degraded outcome, but without ever
	// contacting a TSA.
	TSA *tsa.Client

	// TrustStore is the candidate issuer certificates chain completion
	// searches first (F3 §5.4) — in production, the CA/QC service
	// certificates from the F1 Trusted List.
	TrustStore []*x509.Certificate

	// AIAFetch overrides chain completion's AIA fetcher. Nil means
	// cms.HTTPAIAFetcher.
	AIAFetch cms.AIAFetcher

	// Now overrides the signature dictionary's /M date and, indirectly
	// (via being held fixed across a run), makes output reproducible
	// for the golden-file test. Zero means time.Now().
	Now time.Time

	// Stamp requests a visible signature stamp (F4). Nil — the zero
	// value — means no stamp at all: the invisible-signature path below
	// is completely unchanged in that case, byte for byte (F4 §6),
	// because every line that reads opts.Stamp is skipped entirely.
	Stamp *StampOptions

	// MaxRevocationArtefactSize caps how large a single OCSP response or
	// CRL embedded into /DSS may be (Task 1b). Zero or negative means
	// dss.DefaultMaxArtefactSize (5 MB) — see that constant's own
	// comment for the measurement (a real MUP CRL: 30,136,214 bytes)
	// that motivates the default.
	MaxRevocationArtefactSize int64
}

// StampOptions configures the visual signature stamp (F4 §7's CLI
// flags, one field each). Label must already be localised by the
// caller (F4 §5.3) — this package has no access to internal/i18n
// (SPEC §4.2 draws no such rule, but internal/pades has never needed
// internal/i18n and F4 does not ask it to start).
type StampOptions struct {
	Label string

	Corner appearance.Corner
	UseXY  bool
	X, Y   float64

	// Page selects the target page (1-based; -1 means the last page).
	// Zero means the first page.
	Page int

	// Reference is the caller-supplied document reference line
	// (--stamp-reference), shown only when non-empty.
	Reference string

	// ShowDocumentID requests the identity-document-number line
	// (--stamp-show-document-id, F4 §5.2), populated from the signer
	// certificate's IDCRS-prefixed identifier if present. Off by
	// default: the zero value of StampOptions never shows it.
	ShowDocumentID bool
}

// Result reports what SignDocument actually produced (F3 §7.3): the
// achieved level, never the requested one, plus human-readable notes
// explaining any degradation. Notes are for local CLI/log output, not
// an API boundary — SPEC §7's "codes, not sentences" governs
// internal/api's future HTTP surface, which this phase does not build.
// Notes is English-only (SPEC §9.2: log content is never localised);
// RevocationTooLarge and ClockDriftWarning below exist alongside it
// specifically so a caller that *does* localise its own output (the
// CLI, SPEC §9.2) has the structured facts (a byte count, two times) to
// build that message in the user's locale, rather than displaying an
// English Notes sentence — this package has no access to internal/i18n
// (see StampOptions.Label's doc comment for the same reasoning).
type Result struct {
	Bytes         []byte
	AchievedLevel Level
	Notes         []string

	// RevocationTooLarge is true when B-LT was requested but at least
	// one certificate's OCSP response or CRL exceeded
	// Options.MaxRevocationArtefactSize and was therefore not embedded
	// (Task 1b/1c) — the achieved level is B-T, and this is why, as
	// opposed to no revocation endpoint answering at all.
	// LargestSkippedBytes is the biggest single skipped artefact's size.
	RevocationTooLarge  bool
	LargestSkippedBytes int64

	// ClockDriftWarning is true when the timestamp's genTime differed
	// from the signer's machine clock (the /M value written into the
	// signature dictionary, captured before the TSA round trip) by more
	// than clockDriftWarnThreshold (Task 3). It is a warning, never a
	// failure: the document is saved exactly as it would have been
	// otherwise. MachineTime and TimestampTime are the two values
	// compared, so a caller can name both.
	ClockDriftWarning bool
	MachineTime       time.Time
	TimestampTime     time.Time
}

// SignDocument signs pdfBytes with session, on the document's first
// page unless opts.Stamp says otherwise. session supplies the raw RSA
// signature over a pre-computed digest and, when it can, the signer's
// certificate and chain (F2/SPEC §5.1) — nothing here ever sees a PIN.
//
// With opts.Stamp == nil (the default), this function's behaviour is
// exactly F3's: one incremental revision, an invisible signature field.
// F4 §6 requires that path to stay byte-for-byte unchanged, which is
// why every stamp-specific step below is inside an `if opts.Stamp !=
// nil` block rather than something this function always does and
// discards the result of.
func SignDocument(ctx context.Context, pdfBytes []byte, session keysource.Session, opts Options) (*Result, error) {
	signerCert, err := x509.ParseCertificate(session.Certificate().DER)
	if err != nil {
		return nil, fmt.Errorf("pades: parsing signer certificate: %w", err)
	}

	signingDate := opts.Now
	if signingDate.IsZero() {
		signingDate = time.Now()
	}

	docBytes := pdfBytes
	stampPage := 0
	var stampAppearance *pdf.Appearance
	if opts.Stamp != nil {
		docBytes, stampPage, stampAppearance, err = applyStamp(docBytes, signerCert, signingDate, opts.Stamp)
		if err != nil {
			return nil, err
		}
	}

	doc, err := pdf.Parse(docBytes)
	if err != nil {
		return nil, err
	}
	ph, err := pdf.BuildPlaceholder(doc, pdf.PlaceholderOptions{
		ReservedBytes: opts.ReservedBytes,
		SubFilter:     pdf.Name("ETSI.CAdES.detached"),
		SigningDate:   signingDate,
		PageNumber:    stampPage,
		Appearance:    stampAppearance,
	})
	if err != nil {
		return nil, err
	}
	digest := ph.Digest()

	chain := parseChain(session.Chain())
	if len(chain) == 0 {
		fetch := opts.AIAFetch
		if fetch == nil {
			fetch = cms.HTTPAIAFetcher
		}
		chain = cms.CompleteChain(ctx, signerCert, opts.TrustStore, fetch)
	}

	builder := cms.NewBuilder(signerCert, chain, digest[:])
	attrsDigest := builder.SignedAttrsDigest()
	sig, err := session.SignDigest(ctx, keysource.DigestSHA256, attrsDigest[:])
	if err != nil {
		return nil, err // already an *errs.Error from the key source (F2 §2.4)
	}
	builder.SetSignature(sig)

	result := &Result{AchievedLevel: LevelBB}

	// A level was requested (the CLI's --level always sets one; only
	// this package's own tests pass Options{} to mean "no particular
	// level, whatever B-B gives you"). A requested level of B-B is the
	// caller stating that outcome deliberately — Settings' own B-B
	// option (D-105) — so no timestamp is attempted at all and none is
	// reported as missing; the result is B-B because that is what was
	// asked for, not because something failed. Reaching B-T requires a TSA — if
	// none is configured, that is treated exactly like a TSA that was
	// contacted and failed (Task 7/SPEC §12.8/§18.11: the achieved level
	// must never fall below the requested one without the caller being
	// told why, and "nobody configured a TSA" is not an exception to
	// that rule).
	if opts.RequestedLevel != "" && opts.RequestedLevel != LevelBB {
		var resp *tsa.Response
		var tsaErr error
		if opts.TSA == nil {
			tsaErr = fmt.Errorf("pades: level %s requested but no TSA is configured", opts.RequestedLevel)
		} else {
			sigDigest := sha256.Sum256(sig)
			resp, tsaErr = opts.TSA.Timestamp(ctx, sigDigest[:])
		}
		switch {
		case tsaErr == nil:
			builder.AddUnsignedAttribute(cms.OIDSignatureTimeStampToken, resp.TokenDER)
			result.AchievedLevel = LevelBT
			// Task 3: the /M value (signingDate) was fixed before this
			// round trip even began — it is the signer's own machine
			// clock, unverified. Now that a qualified timestamp exists,
			// compare the two: a document whose visible stamp date and
			// cryptographically attested time disagree by more than a
			// few minutes means the machine clock is wrong, and the
			// user should find out now, not after a hundred more
			// documents carry the same wrong date. This never fails the
			// operation — it is a warning attached to an otherwise
			// successful result (SPEC's stamp-clock decision, docs/decisions.md).
			drift := resp.GenTime.Sub(signingDate)
			if drift < 0 {
				drift = -drift
			}
			if drift > clockDriftWarnThreshold {
				result.ClockDriftWarning = true
				result.MachineTime = signingDate
				result.TimestampTime = resp.GenTime
				result.Notes = append(result.Notes, fmt.Sprintf(
					"machine clock reads %s but the timestamp reads %s (drift %s)",
					signingDate.Format(time.RFC3339), resp.GenTime.Format(time.RFC3339), drift.Round(time.Second)))
			}
		case opts.OnTSAFailureAbort:
			return nil, errs.New(classifyTSAError(tsaErr), tsaErr)
		default:
			result.Notes = append(result.Notes, fmt.Sprintf("no timestamp (saved at B-B): %v", tsaErr))
		}
	}

	cmsDER, err := builder.Finish()
	if err != nil {
		return nil, err
	}
	if err := ph.InjectSignature(cmsDER); err != nil {
		return nil, err
	}
	result.Bytes = ph.Bytes

	if result.AchievedLevel == LevelBT && opts.RequestedLevel == LevelBLT {
		applyDSS(ctx, result, cmsDER, signerCert, chain, opts.MaxRevocationArtefactSize)
	}

	return result, nil
}

// applyStamp appends one incremental revision (F4 §6) containing the
// visual stamp's font, logo and Form XObject, and returns the updated
// bytes, the (already-normalised, 1-based-or-last) target page number,
// and the widget placement pdf.BuildPlaceholder needs to make that page's
// signature widget visible. It never touches the page object or adds
// the widget itself — that happens in BuildPlaceholder's own, later
// revision, the same "add a further revision on top" shape applyDSS
// already uses once a signature exists.
//
// The stamp's time line shows signingDate (the /M date), not an RFC
// 3161 timestamp time, even though F4 §5's content table names
// "timestamp time": the stamp's content stream bytes are fixed here,
// before the TSA round-trip in SignDocument happens (and, on B-B, no
// timestamp may exist at all). SPEC §12.3's "no signingTime in the CMS"
// rule governs the cryptographic signed attributes and is unaffected —
// nothing about this line changes what the CMS asserts or what an
// independent verifier checks. See docs/decisions.md.
func applyStamp(pdfBytes []byte, signerCert *x509.Certificate, signingDate time.Time, stamp *StampOptions) ([]byte, int, *pdf.Appearance, error) {
	doc, err := pdf.Parse(pdfBytes)
	if err != nil {
		return nil, 0, nil, err
	}
	rootRef, ok := doc.Trailer().Get(pdf.Name("Root")).(pdf.Reference)
	if !ok {
		return nil, 0, nil, fmt.Errorf("pades: trailer /Root is not an indirect reference")
	}
	catalog, ok := doc.ResolveDict(rootRef)
	if !ok {
		return nil, 0, nil, fmt.Errorf("pades: /Root does not resolve to a dictionary")
	}
	page := stamp.Page
	if page == 0 {
		page = 1
	}
	pageNum, err := pdf.FindPage(doc, catalog, page)
	if err != nil {
		return nil, 0, nil, err
	}
	pageDict, ok := doc.ResolveDict(pdf.Reference{Num: pageNum})
	if !ok {
		return nil, 0, nil, fmt.Errorf("pades: page %d does not resolve to a dictionary", pageNum)
	}

	u := pdf.NewUpdate(doc)
	result, err := appearance.Render(doc, pageDict, u, buildAppearanceOptions(signerCert, signingDate, stamp))
	if err != nil {
		return nil, 0, nil, wrapStampError(err)
	}
	out, err := u.Apply()
	if err != nil {
		return nil, 0, nil, err
	}
	return out, page, &result, nil
}

// buildAppearanceOptions assembles appearance.Options from the signer
// certificate, the signing date and the caller's StampOptions — kept
// separate from applyStamp's PDF plumbing so it can be tested on its
// own (see TestBuildAppearanceOptionsNeverContainsThirteenConsecutiveDigits)
// without needing a real pdf.Document or pdf.Update.
func buildAppearanceOptions(signerCert *x509.Certificate, signingDate time.Time, stamp *StampOptions) appearance.Options {
	var documentID string
	if stamp.ShowDocumentID {
		// F4 §5.2: opt-in, and even then sourced only from the
		// IDCRS-prefixed identifier — see documentIDFromCertificate's
		// own doc comment for why PNORS can never reach this call.
		documentID = documentIDFromCertificate(signerCert)
	}
	return appearance.Options{
		Label:       stamp.Label,
		SignerName:  signerDisplayName(signerCert),
		Reference:   stamp.Reference,
		DocumentID:  documentID,
		SerialHex:   strings.ToUpper(signerCert.SerialNumber.Text(16)),
		SigningTime: signingDate.Format("2006-01-02 15:04 MST"),
		Corner:      stamp.Corner,
		UseXY:       stamp.UseXY,
		X:           stamp.X,
		Y:           stamp.Y,
	}
}

// wrapStampError maps an appearance.MissingGlyphError to
// errs.CodeStampGlyphMissing with structured Details naming the
// character and its code point (F4 §3.3/Task 2). This used to reuse
// errs.CodeSignFailed — see docs/decisions.md, which supersedes that
// choice: a missing glyph has nothing to do with the card, the reader or
// the signing operation, and reporting it as SIGN_FAILED sent users to
// check hardware for a problem that was in the stamp's own text. Any
// other error from the stamp path is returned unchanged.
func wrapStampError(err error) error {
	var mg *appearance.MissingGlyphError
	if errors.As(err, &mg) {
		return errs.WithDetails(errs.CodeStampGlyphMissing, err, map[string]any{
			"character": string(mg.Rune),
			"codePoint": fmt.Sprintf("U+%04X", mg.Rune),
		})
	}
	return err
}

// classifyTSAError maps a TSA failure to TSA_REJECTED when the TSA (or
// the HTTP layer in front of it) affirmatively refused the request — an
// HTTP 4xx (*tsa.HTTPStatusError) or an RFC 3161-level rejection
// (*tsa.RejectionError) — and to TSA_UNAVAILABLE for everything else: a
// network error, a timeout, repeated 5xxs, or no TSA being configured at
// all (Task 6). The two codes call for different user actions — wait and
// retry, versus check credentials or configuration — so collapsing them
// into one, as this package did before, reported "did not respond" for a
// request the TSA had, in fact, actively refused.
func classifyTSAError(err error) errs.Code {
	var rej *tsa.RejectionError
	var status *tsa.HTTPStatusError
	if errors.As(err, &rej) || errors.As(err, &status) {
		return errs.CodeTSARejected
	}
	return errs.CodeTSAUnavailable
}

// applyDSS attempts F3 §7's B-LT upgrade in place on result, degrading
// to B-T with a note on any failure rather than returning an error —
// DSS embedding is best-effort by design (F3 §7.3). maxArtefactSize is
// Options.MaxRevocationArtefactSize, forwarded to
// dss.CollectRevocation (Task 1b); zero or negative means
// dss.DefaultMaxArtefactSize.
func applyDSS(ctx context.Context, result *Result, cmsDER []byte, signerCert *x509.Certificate, chain []*x509.Certificate, maxArtefactSize int64) {
	doc, err := pdf.Parse(result.Bytes)
	if err != nil {
		result.Notes = append(result.Notes, fmt.Sprintf("B-LT requested; re-parsing for DSS failed: %v", err))
		return
	}
	allCerts := append([]*x509.Certificate{signerCert}, chain...)
	entries := dss.CollectRevocation(ctx, allCerts, maxArtefactSize)
	dssResult, err := dss.Apply(doc, cmsDER, allCerts, entries)
	if err != nil {
		result.Notes = append(result.Notes, fmt.Sprintf("B-LT requested; embedding /DSS failed: %v", err))
		return
	}
	result.Bytes = dssResult.Bytes
	if dssResult.Complete {
		result.AchievedLevel = LevelBLT
		return
	}
	if dssResult.TooLarge {
		// Task 1c: the achieved level is B-T, and the reason must be
		// reported honestly, not folded into the generic "unavailable"
		// message below — this Note stays English-only (SPEC §9.2); the
		// structured RevocationTooLarge/LargestSkippedBytes fields on
		// Result are what a localising caller (the CLI) actually uses.
		result.RevocationTooLarge = true
		result.LargestSkippedBytes = dssResult.LargestSkippedBytes
		result.Notes = append(result.Notes, fmt.Sprintf(
			"revocation data too large to embed (%d bytes); saved at B-T", dssResult.LargestSkippedBytes))
		return
	}
	result.Notes = append(result.Notes, "B-LT requested; OCSP/CRL unavailable for at least one certificate")
}

func parseChain(der [][]byte) []*x509.Certificate {
	var out []*x509.Certificate
	for _, d := range der {
		if c, err := x509.ParseCertificate(d); err == nil {
			out = append(out, c)
		}
	}
	return out
}
