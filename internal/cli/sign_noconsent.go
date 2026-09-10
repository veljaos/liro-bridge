//go:build softtoken

// This file is the signing path that asks nobody, and it exists only in
// a binary built with the "softtoken" tag.
//
// SPEC §18.2 is unconditional — "No signature without human approval.
// No flag, no configuration, no header bypasses the consent screen" —
// and SPEC §4.3 says all four entry points go through the same consent
// screen. This code predates the consent screen (F5) and was never
// reconciled with either. It was reachable as `liro-bridge sign` by any
// process on the machine, with no pairing, no origin binding and no
// human.
//
// It survives at all for one reason: CI signs a PDF and verifies it
// with OpenSSL and this project's own independent verifier on every
// push (SPEC §16.4, F3's exit condition), and must keep doing that with
// no human present. That is a test build's need, so it lives behind the
// tag that already means "this is a test build" (SPEC §16.6), reached by
// its own command name — `sign-no-consent` — rather than by `sign`,
// which shows the window in every build. A release binary contains
// neither the command nor this function; CI proves it by inspecting the
// binary.
package cli

import (
	"context"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/pades/dss"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/signing"
)

// TSACredentials are the timestamp authority's credentials. They come
// from configuration and never from a flag: on Windows a full command
// line is readable by every process on the machine — Task Manager's
// "Command line" column, Get-CimInstance Win32_Process, any process
// lister — so a password passed as an argument is a password published
// to the machine for as long as the process runs.
//
// D-091 already moved these four values into configuration; the flags
// stayed beside them until this phase removed them. A URL and a file
// path are not credentials and remain flags.
type TSACredentials struct {
	BasicUsername      string
	BasicPassword      string
	ClientCertPassword string
}

// SignPDFDeps supplies sign-no-consent with everything it needs, as
// functions and data rather than concrete types — the same shape
// SignDeps already uses for sign-digest (F2), so main.go remains the
// only place a concrete keysource.Source is chosen.
type SignPDFDeps struct {
	Open func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error)

	// TrustStore is the candidate issuer certificates chain completion
	// searches first (F3 §5.4) — in production, the F1 Trusted List's
	// CA/QC service certificates.
	TrustStore []*x509.Certificate

	// TSACredentials come from config.json. See TSACredentials.
	TSACredentials TSACredentials
}

// signOutputSuffix is F3 §12.11's default.
const signOutputSuffix = "-signed"

// RunSignWithoutConsent implements "liro-bridge sign-no-consent": F3
// §9's batch signing with no consent screen and no human. See this
// file's own doc comment for why it exists and why it is behind a build
// tag.
func RunSignWithoutConsent(ctx context.Context, args []string, stdout, stderr io.Writer, locale string, deps SignPDFDeps) int {
	c := i18n.Load(locale)

	fs := flag.NewFlagSet("sign-no-consent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inPattern := fs.String("in", "", "input PDF, or a glob matching several")
	outPath := fs.String("out", "", "output path (only valid for a single input file)")
	thumbprintHex := fs.String("thumbprint", "", "certificate SHA-1 thumbprint, hex")
	level := fs.String("level", "b-lt", "b-t or b-lt")
	tsaURL := fs.String("tsa", "", "RFC 3161 TSA URL")
	tsaP12Path := fs.String("tsa-client-cert", "", "TSA TLS client certificate, PKCS#12 file")
	onTSAFailure := fs.String("on-tsa-failure", "abort", "abort or b-b")
	reserve := fs.Int("reserve", 0, "bytes reserved for /Contents (default 32768)")
	maxRevocationSize := fs.Int64("max-revocation-size", 0, "largest CRL/OCSP response embedded into /DSS, in bytes (default 5242880 = 5 MB; Task 1b)")
	force := fs.Bool("force", false, "overwrite an existing output file")
	resign := fs.Bool("resign", false, "sign inputs whose names already end in the output suffix; they are skipped and counted by default")
	stamp := fs.Bool("stamp", false, "add a visible signature stamp (F4); off by default")
	stampPosition := fs.String("stamp-position", "", "bottom-right (default) | bottom-left | top-right | top-left")
	stampXY := fs.String("stamp-xy", "", "explicit stamp position \"x,y\" in points; mutually exclusive with --stamp-position")
	stampPage := fs.Int("stamp-page", 1, "target page for the stamp; -1 means the last page")
	stampReference := fs.String("stamp-reference", "", "the stamp's reference line")
	stampShowDocumentID := fs.Bool("stamp-show-document-id", false, "show the signer's identity document number on the stamp; off by default")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var stampOpts *pades.StampOptions
	if *stamp {
		if *stampPosition != "" && *stampXY != "" {
			fprintln(stderr, "liro-bridge: sign-no-consent:", c.T("sign.stamp_position_xy_conflict"))
			return 2
		}
		opts := &pades.StampOptions{
			Label:          c.T("sign.stamp_label"),
			Reference:      *stampReference,
			Page:           *stampPage,
			ShowDocumentID: *stampShowDocumentID,
		}
		if *stampXY != "" {
			x, y, ok := parseStampXY(*stampXY)
			if !ok {
				fprintln(stderr, "liro-bridge: sign-no-consent:", c.T("sign.stamp_xy_invalid"))
				return 2
			}
			opts.UseXY, opts.X, opts.Y = true, x, y
		} else {
			corner, ok := parseStampPosition(*stampPosition)
			if !ok {
				fprintln(stderr, "liro-bridge: sign-no-consent:", c.T("sign.stamp_position_invalid"))
				return 2
			}
			opts.Corner = corner
		}
		stampOpts = opts
	}

	if *inPattern == "" {
		fprintln(stderr, "liro-bridge: sign-no-consent:", c.T("sign.in_required"))
		return 2
	}
	if *thumbprintHex == "" {
		fprintln(stderr, "liro-bridge: sign-no-consent:", c.T("sign.thumbprint_required"))
		return 2
	}
	requestedLevel, ok := parseLevel(*level)
	if !ok {
		fprintln(stderr, "liro-bridge: sign-no-consent:", c.T("sign.level_invalid"))
		return 2
	}
	abortOnTSAFailure, ok := parseOnTSAFailure(*onTSAFailure)
	if !ok {
		fprintln(stderr, "liro-bridge: sign-no-consent:", c.T("sign.on_tsa_failure_invalid"))
		return 2
	}

	files, err := expandInput(*inPattern)
	if err != nil || len(files) == 0 {
		fprintln(stderr, "liro-bridge: sign-no-consent:", c.T("sign.no_input_files"))
		return 1
	}

	// J-3: run against the same folder twice, --in's glob picks up the
	// first run's own output and signs it again — ugovor-signed.pdf
	// becomes ugovor-signed-signed.pdf, and a third run makes it
	// ugovor-signed-signed-signed.pdf. Which of the two a person meant
	// is not something to guess at: counter-signing a document called
	// ugovor-signed.pdf that arrived from somebody else is perfectly
	// ordinary. So this says how many were found and what would happen,
	// and does the safe thing unless told otherwise.
	files, alreadySigned := partitionAlreadySigned(files, signOutputSuffix, *resign)
	if alreadySigned > 0 {
		fprintln(stderr, "liro-bridge: sign-no-consent:", alreadySignedNotice(c, alreadySigned, signOutputSuffix, *resign))
	}
	if len(files) == 0 {
		fprintln(stderr, "liro-bridge: sign-no-consent:", fmt.Sprintf(c.T("sign.already_signed_nothing_left"), signOutputSuffix))
		return 1
	}

	var client *tsa.Client
	if *tsaURL != "" {
		// The credentials come from configuration, never from the
		// command line. See TSACredentials.
		auth := tsa.Auth{
			BasicUsername: deps.TSACredentials.BasicUsername,
			BasicPassword: deps.TSACredentials.BasicPassword,
		}
		if *tsaP12Path != "" {
			p12, err := os.ReadFile(*tsaP12Path)
			if err != nil {
				fprintln(stderr, "liro-bridge: sign-no-consent: reading --tsa-client-cert:", err)
				return 2
			}
			cert, err := tsa.LoadPKCS12ClientCert(p12, deps.TSACredentials.ClientCertPassword)
			if err != nil {
				fprintln(stderr, "liro-bridge: sign-no-consent: loading --tsa-client-cert:", err)
				return 2
			}
			auth.ClientCertificate = &cert
		}
		client = tsa.NewClient(*tsaURL, auth)
	}

	// F3 §12.10/one session, one PIN: the session is opened once and
	// reused across every document in the batch.
	session, err := deps.Open(ctx, keysource.Thumbprint(*thumbprintHex))
	if err != nil {
		printCommandError(stderr, "sign-no-consent", err, c)
		return 1
	}
	defer func() { _ = session.Close() }()

	if len(files) > 1 && *outPath != "" {
		// F2 §6/D-037's precedent: a single --out cannot serve N files;
		// ignore it rather than invent a naming scheme (F3 never
		// specifies one for this case).
		*outPath = ""
	}

	// One memory for this batch, and a fresh one for the next (J-10).
	// A revocation endpoint that does not answer for the first document
	// is not asked again for the ninety-ninth: measured, an OCSP
	// responder that accepts the request and never answers costs 20 s
	// per document, which is 33 minutes on a hundred-document batch.
	revocationMemory := dss.NewEndpointMemory()

	failures := 0
	for _, in := range files {
		out := *outPath
		if out == "" {
			out = defaultOutputPath(in)
		}
		if err := signOneFile(ctx, in, out, *force, session, client, requestedLevel, abortOnTSAFailure, *reserve, *maxRevocationSize, revocationMemory, deps.TrustStore, stampOpts, stdout, stderr, c); err != nil {
			failures++
			fprintln(stderr, "liro-bridge: sign-no-consent:", in+":", errMessage(err, c))
			if len(files) == 1 {
				return 1
			}
			// F3 §12.10: skip and continue for a batch, never abort the
			// rest over one document's failure.
			continue
		}
	}

	if len(files) > 1 {
		fprintf(stdout, c.T("sign.batch_summary")+"\n", len(files)-failures, len(files))
	}
	if failures == len(files) {
		return 1
	}
	return 0
}

// partitionAlreadySigned splits files into the ones to sign and a count
// of the ones whose names already end in suffix. With resign true
// nothing is removed — the count is still returned, because naming what
// is about to happen is the point of it.
func partitionAlreadySigned(files []string, suffix string, resign bool) (toSign []string, alreadySigned int) {
	toSign = make([]string, 0, len(files))
	for _, f := range files {
		if !jobs.LooksLikeOutput(f, suffix) {
			toSign = append(toSign, f)
			continue
		}
		alreadySigned++
		if resign {
			toSign = append(toSign, f)
		}
	}
	return toSign, alreadySigned
}

// alreadySignedNotice says how many inputs already carry the output
// suffix and what is being done about them — skipped, with the flag
// that would sign them, or signed because that flag was given.
func alreadySignedNotice(c *i18n.Catalogue, n int, suffix string, resign bool) string {
	key := "sign.already_signed_skipped_many"
	switch {
	case resign && n == 1:
		key = "sign.already_signed_resigning_one"
	case resign:
		key = "sign.already_signed_resigning_many"
	case n == 1:
		key = "sign.already_signed_skipped_one"
	}
	if n == 1 {
		return fmt.Sprintf(c.T(key), suffix)
	}
	return fmt.Sprintf(c.T(key), n, suffix)
}

func signOneFile(ctx context.Context, in, out string, force bool, session keysource.Session, client *tsa.Client, level pades.Level, abortOnTSAFailure bool, reserve int, maxRevocationSize int64, revocationMemory *dss.EndpointMemory, trustStore []*x509.Certificate, stamp *pades.StampOptions, stdout, stderr io.Writer, c *i18n.Catalogue) error {
	if !force {
		if _, err := os.Stat(out); err == nil {
			return errors.New(c.T("sign.output_exists"))
		}
	}
	// INPUT_UNREADABLE and OUTPUT_WRITE_FAILED exist for exactly these
	// two moments (D-118, D-104) and the window path has used them since
	// F6. This one did not: it returned the raw os error, so a Serbian
	// user signing from the command line was shown "open C:\...: Access
	// is denied." — English, from the operating system, for a situation
	// this project already has a localised sentence for. Two front doors
	// answering the same question differently is what D-108, D-124 and
	// D-138 each had to remove once already.
	pdfBytes, err := os.ReadFile(in)
	if err != nil {
		return errs.New(errs.CodeInputUnreadable, err)
	}

	result, err := pades.SignDocument(ctx, pdfBytes, session, pades.Options{
		ReservedBytes:             reserve,
		RequestedLevel:            level,
		OnTSAFailureAbort:         abortOnTSAFailure,
		TSA:                       client,
		TrustStore:                trustStore,
		Now:                       time.Now(),
		Stamp:                     stamp,
		MaxRevocationArtefactSize: maxRevocationSize,
		RevocationMemory:          revocationMemory,
	})
	if err != nil {
		return err
	}

	// Not os.WriteFile: that opens with O_CREATE|O_TRUNC and then
	// writes, so the destination exists at the wrong length in between
	// and a write that fails partway destroys whatever was there
	// (J-8). WriteFileAtomic writes beside the target and renames over
	// it, and already carries its own errs.Code — OUTPUT_IN_USE when
	// the destination is held open by another program, which is the one
	// case with an answer a person can act on.
	if err := platform.WriteFileAtomic(out, result.Bytes, 0o600); err != nil {
		return err
	}

	fprintln(stdout, c.T("sign.signed_label"), out)
	fprintln(stdout, " ", c.T("sign.level_label"), levelLine(result, c))
	fprintln(stdout, " ", c.T("sign.certificate_label"), thumbprintTail(session.Certificate().Thumbprint))
	printTestKeyWarning(stderr, session, c)
	printClockDriftWarning(stderr, result, c)
	return nil
}

// levelLine builds the text shown after "Level:" (Task 1c): the achieved
// level plus, when there is one, a parenthesised explanation.
// RevocationTooLarge gets its own localised, specific message — not the
// English Notes sentence (SPEC §9.2 reserves English for logs) — naming
// what happened and why, exactly as the task requires; every other
// degradation still uses the existing (English-only) Notes join.
func levelLine(result *pades.Result, c *i18n.Catalogue) string {
	line := string(result.AchievedLevel)
	switch {
	case result.RevocationTooLarge:
		line += "  (" + fmt.Sprintf(c.T("sign.revocation_too_large"), formatBytesApprox(result.LargestSkippedBytes)) + ")"
	case len(result.Notes) > 0:
		line += "  (" + strings.Join(result.Notes, "; ") + ")"
	}
	return line
}

// formatBytesApprox renders n as whole megabytes for Task 1c's message
// ("Revocation data was too large to embed (32 MB)") — precise enough to
// be useful, without a fractional MB a user has no reason to care about.
func formatBytesApprox(n int64) string {
	return fmt.Sprintf("%.0f MB", float64(n)/(1024*1024))
}

// printClockDriftWarning prints Task 3's localised warning when the
// timestamp's genTime and the machine clock (the /M value) disagreed by
// more than five minutes. Like printTestKeyWarning, this is advisory
// output on stderr — it never affects the exit code or the file written.
func printClockDriftWarning(w io.Writer, result *pades.Result, c *i18n.Catalogue) {
	if !result.ClockDriftWarning {
		return
	}
	fprintln(w, fmt.Sprintf(c.T("sign.clock_drift_warning"),
		result.MachineTime.Format(time.RFC3339), result.TimestampTime.Format(time.RFC3339)))
}

func parseLevel(s string) (pades.Level, bool) {
	switch strings.ToLower(s) {
	case "b-t":
		return pades.LevelBT, true
	case "b-lt":
		return pades.LevelBLT, true
	default:
		return "", false
	}
}

func parseOnTSAFailure(s string) (abort bool, ok bool) {
	switch strings.ToLower(s) {
	case "abort":
		return true, true
	case "b-b":
		return false, true
	default:
		return false, false
	}
}

// expandInput resolves --in: a glob pattern, or (when it matches no
// glob metacharacters, or the glob itself matches nothing but the path
// exists literally) a single file.
func expandInput(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err == nil && len(matches) > 0 {
		return matches, nil
	}
	if _, statErr := os.Stat(pattern); statErr == nil {
		return []string{pattern}, nil
	}
	return nil, fmt.Errorf("no files matched %q", pattern)
}

// defaultOutputPath implements F3 §12.11: document.pdf -> document-signed.pdf.
func defaultOutputPath(in string) string {
	ext := filepath.Ext(in)
	base := strings.TrimSuffix(in, ext)
	return base + signOutputSuffix + ext
}

func thumbprintTail(t keysource.Thumbprint) string {
	s := string(t)
	if len(s) <= 8 {
		return "…" + s
	}
	return "…" + s[len(s)-8:]
}

// parseStampPosition maps --stamp-position's four accepted values (F4
// §7) to a Corner, defaulting to bottom-right (SPEC §13.1) when empty.
func parseStampPosition(s string) (appearance.Corner, bool) {
	switch s {
	case "", "bottom-right":
		return appearance.BottomRight, true
	case "bottom-left":
		return appearance.BottomLeft, true
	case "top-right":
		return appearance.TopRight, true
	case "top-left":
		return appearance.TopLeft, true
	default:
		return 0, false
	}
}

// parseStampXY parses --stamp-xy's "x,y" form (F4 §7), both values in
// points.
func parseStampXY(s string) (x, y float64, ok bool) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	x, errX := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	y, errY := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errX != nil || errY != nil {
		return 0, 0, false
	}
	return x, y, true
}

// signingSessionAdapter is unused directly but documents that
// *signing.Session (F2's idle/lifetime-tracked wrapper) also satisfies
// keysource.Session, so main.go may pass either into SignPDFDeps.Open.
var _ keysource.Session = (*signing.Session)(nil)
