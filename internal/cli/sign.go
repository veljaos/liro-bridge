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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
	"github.com/veljaos/liro-bridge/internal/signing"
)

// SignPDFDeps supplies sign with everything it needs, as functions and
// data rather than concrete types — the same shape SignDeps already
// uses for sign-digest (F2), so main.go remains the only place a
// concrete keysource.Source is chosen.
type SignPDFDeps struct {
	Open func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error)

	// TrustStore is the candidate issuer certificates chain completion
	// searches first (F3 §5.4) — in production, the F1 Trusted List's
	// CA/QC service certificates.
	TrustStore []*x509.Certificate
}

// signOutputSuffix is F3 §12.11's default.
const signOutputSuffix = "-signed"

// RunSign implements "liro-bridge sign" (F3 §9).
func RunSign(ctx context.Context, args []string, stdout, stderr io.Writer, locale string, deps SignPDFDeps) int {
	c := i18n.Load(locale)

	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inPattern := fs.String("in", "", "input PDF, or a glob matching several")
	outPath := fs.String("out", "", "output path (only valid for a single input file)")
	thumbprintHex := fs.String("thumbprint", "", "certificate SHA-1 thumbprint, hex")
	level := fs.String("level", "b-lt", "b-t or b-lt")
	tsaURL := fs.String("tsa", "", "RFC 3161 TSA URL")
	tsaUser := fs.String("tsa-user", "", "TSA HTTP Basic auth username")
	tsaPassword := fs.String("tsa-password", "", "TSA HTTP Basic auth password")
	tsaP12Path := fs.String("tsa-client-cert", "", "TSA TLS client certificate, PKCS#12 file")
	tsaP12Password := fs.String("tsa-client-cert-password", "", "TSA TLS client certificate password")
	onTSAFailure := fs.String("on-tsa-failure", "abort", "abort or b-b")
	reserve := fs.Int("reserve", 0, "bytes reserved for /Contents (default 32768)")
	maxRevocationSize := fs.Int64("max-revocation-size", 0, "largest CRL/OCSP response embedded into /DSS, in bytes (default 5242880 = 5 MB; Task 1b)")
	force := fs.Bool("force", false, "overwrite an existing output file")
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
			fprintln(stderr, "liro-bridge: sign:", c.T("sign.stamp_position_xy_conflict"))
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
				fprintln(stderr, "liro-bridge: sign:", c.T("sign.stamp_xy_invalid"))
				return 2
			}
			opts.UseXY, opts.X, opts.Y = true, x, y
		} else {
			corner, ok := parseStampPosition(*stampPosition)
			if !ok {
				fprintln(stderr, "liro-bridge: sign:", c.T("sign.stamp_position_invalid"))
				return 2
			}
			opts.Corner = corner
		}
		stampOpts = opts
	}

	if *inPattern == "" {
		fprintln(stderr, "liro-bridge: sign:", c.T("sign.in_required"))
		return 2
	}
	if *thumbprintHex == "" {
		fprintln(stderr, "liro-bridge: sign:", c.T("sign.thumbprint_required"))
		return 2
	}
	requestedLevel, ok := parseLevel(*level)
	if !ok {
		fprintln(stderr, "liro-bridge: sign:", c.T("sign.level_invalid"))
		return 2
	}
	abortOnTSAFailure, ok := parseOnTSAFailure(*onTSAFailure)
	if !ok {
		fprintln(stderr, "liro-bridge: sign:", c.T("sign.on_tsa_failure_invalid"))
		return 2
	}

	files, err := expandInput(*inPattern)
	if err != nil || len(files) == 0 {
		fprintln(stderr, "liro-bridge: sign:", c.T("sign.no_input_files"))
		return 1
	}

	var client *tsa.Client
	if *tsaURL != "" {
		auth := tsa.Auth{BasicUsername: *tsaUser, BasicPassword: *tsaPassword}
		if *tsaP12Path != "" {
			p12, err := os.ReadFile(*tsaP12Path)
			if err != nil {
				fprintln(stderr, "liro-bridge: sign: reading --tsa-client-cert:", err)
				return 2
			}
			cert, err := tsa.LoadPKCS12ClientCert(p12, *tsaP12Password)
			if err != nil {
				fprintln(stderr, "liro-bridge: sign: loading --tsa-client-cert:", err)
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
		printCommandError(stderr, "sign", err, c)
		return 1
	}
	defer func() { _ = session.Close() }()

	if len(files) > 1 && *outPath != "" {
		// F2 §6/D-037's precedent: a single --out cannot serve N files;
		// ignore it rather than invent a naming scheme (F3 never
		// specifies one for this case).
		*outPath = ""
	}

	failures := 0
	for _, in := range files {
		out := *outPath
		if out == "" {
			out = defaultOutputPath(in)
		}
		if err := signOneFile(ctx, in, out, *force, session, client, requestedLevel, abortOnTSAFailure, *reserve, *maxRevocationSize, deps.TrustStore, stampOpts, stdout, stderr, c); err != nil {
			failures++
			fprintln(stderr, "liro-bridge: sign:", in+":", errMessage(err, c))
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

func signOneFile(ctx context.Context, in, out string, force bool, session keysource.Session, client *tsa.Client, level pades.Level, abortOnTSAFailure bool, reserve int, maxRevocationSize int64, trustStore []*x509.Certificate, stamp *pades.StampOptions, stdout, stderr io.Writer, c *i18n.Catalogue) error {
	if !force {
		if _, err := os.Stat(out); err == nil {
			return errors.New(c.T("sign.output_exists"))
		}
	}
	pdfBytes, err := os.ReadFile(in)
	if err != nil {
		return err
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
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(out, result.Bytes, 0o600); err != nil {
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

// errMessage renders err for the CLI's own local, English-or-localised
// output — not an API boundary (SPEC §7 governs internal/api's future
// JSON responses; this phase builds no such surface). A structured
// *errs.Error's Details are appended, since they carry exactly the kind
// of fact a person debugging a failed signing run needs — the character
// and code point of a glyph missing from the stamp's font subset
// (F4 §3.3), for instance, which the localised code alone
// ("SIGN_FAILED") would not convey.
func errMessage(err error, c *i18n.Catalogue) string {
	var e *errs.Error
	if errors.As(err, &e) {
		if e.Code == errs.CodeStampGlyphMissing {
			// Task 2: the catalogue message itself names the character
			// and its code point ("...: %s (%s)."), rather than having
			// them appended generically the way other codes' Details are
			// — this is the one error whose whole point is to be read as
			// a sentence, not a code plus a debugging fragment.
			return fmt.Sprintf(c.T(i18n.CodeKey(e.Code)), e.Details["character"], e.Details["codePoint"])
		}
		msg := c.T(i18n.CodeKey(e.Code))
		if len(e.Details) > 0 {
			msg += " (" + formatDetails(e.Details) + ")"
		}
		return msg
	}
	return err.Error()
}

func formatDetails(details map[string]any) string {
	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%v", k, details[k])
	}
	return strings.Join(parts, ", ")
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
