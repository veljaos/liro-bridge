//go:build softtoken

// sign-digest signs a bare digest with a certificate already on this
// machine, and shows nobody anything before it does. It exists only in
// a build made with the "softtoken" tag, beside the other signing path
// with no consent window (sign_noconsent.go).
//
// Why it is not in a release binary (D-227). A digest is not a lesser
// thing than a document: the digest a PAdES signature is computed over
// is a SHA-256 of a /ByteRange, so a signature over an attacker-chosen
// digest is a signature over an attacker-chosen document. SPEC §6.5
// forecloses the obvious reassurance — the card caches the PIN in its
// own state independently of which process is talking to it, so a
// second process signs silently for as long as the session lives. That
// is the whole reason the consent window exists, and SPEC §18.2 admits
// no exception to it.
//
// And a consent screen in front of a bare digest cannot be built:
// SPEC §6.6 says what the screen shows — a document count, a batch
// fingerprint and the list of file names — and a digest has none of the
// three. The hash-only use case already has a better front door in
// POST /v2/sign, with pairing, origin binding and a real consent
// screen; SPEC §4.3 does not list a CLI digest command among the four
// entry points at all. A release binary loses nothing.
//
// What it keeps is the developer's build check it was written for: F2
// §6.1's recipe, where a signature over a digest is verified with
// OpenSSL, a tool sharing no code with this project. That is a test
// build's need, so it lives behind the tag that already means "this is
// a test build" (SPEC §16.6). The tag adds the soft token; it does not
// take CNG away, so the same recipe still runs against a real card.

package cli

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/signing"
)

// maxRepeat caps --repeat (F2 §6): well past anything a human batch
// realistically reaches in one sitting, and it bounds how much a single
// invocation can be made to allocate.
const maxRepeat = 1000

// digestHexLen is 32 bytes of SHA-256, hex-encoded (F2 §6).
const digestHexLen = 64

// SignDeps supplies sign-digest with a way to open a signing session,
// as a function rather than a concrete type — mirroring Deps in
// report.go. main.go is the only place a concrete keysource.Source (or,
// with the softtoken build tag, more than one) is chosen; this package
// never imports internal/keysource/windowscng or
// internal/keysource/softtoken directly.
type SignDeps struct {
	Open func(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error)
}

// RunSignDigest implements "liro-bridge sign-digest" (F2 §6). stdout
// carries exactly one thing when --out is absent: the base64 signature,
// so a caller can capture it directly (`sig=$(liro-bridge sign-digest
// ...)`). Every other message — errors, the timing report, the
// soft-token warning — goes to stderr.
func RunSignDigest(ctx context.Context, args []string, stdout, stderr io.Writer, locale string, deps SignDeps) int {
	c := i18n.Load(locale)

	fs := flag.NewFlagSet("sign-digest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	thumbprintHex := fs.String("thumbprint", "", "certificate SHA-1 thumbprint, hex")
	digestHex := fs.String("digest", "", "SHA-256 digest, 64 hex characters")
	outPath := fs.String("out", "", "write raw signature bytes here (default: base64 to stdout)")
	repeat := fs.Int("repeat", 1, "sign the same digest this many times in one session, to exercise batching")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Reject a malformed digest before touching the card (F2 §6): a
	// wrong length or non-hex value can never succeed, so there is no
	// reason to open a session — and possibly trigger a PIN prompt —
	// first.
	digest, err := parseDigestHex(*digestHex)
	if err != nil {
		fprintln(stderr, "liro-bridge: sign-digest:", c.T("sign.invalid_digest"))
		return 2
	}
	if *thumbprintHex == "" {
		fprintln(stderr, "liro-bridge: sign-digest:", c.T("sign.thumbprint_required"))
		return 2
	}

	n := *repeat
	if n < 1 {
		n = 1
	}
	if n > maxRepeat {
		n = maxRepeat
	}

	innerSession, err := deps.Open(ctx, keysource.Thumbprint(*thumbprintHex))
	if err != nil {
		printCommandError(stderr, "sign-digest", err, c)
		return 1
	}
	defer func() { _ = innerSession.Close() }()

	sess := signing.WrapSession(innerSession)
	items := make([]signing.BatchItem, n)
	for i := range items {
		items[i] = signing.BatchItem{Digest: digest, Algorithm: keysource.DigestSHA256}
	}
	batch := signing.NewBatch(newBatchID(), *thumbprintHex, items)

	result, err := signing.Sign(ctx, batch, sess)
	if err != nil {
		printCommandError(stderr, "sign-digest", err, c)
		return 1
	}

	if n > 1 {
		// --repeat exists to measure timing, not to produce N usable
		// signatures (RSA-PKCS#1v1.5 over the same digest with the same
		// key is deterministic — every one of the N signatures is
		// byte-identical). --out is ignored in this mode.
		printTimingReport(stderr, result, c)
		printTestKeyWarning(stderr, innerSession, c)
		if len(result.Failures) == len(items) {
			return 1
		}
		return 0
	}

	if len(result.Failures) > 0 {
		fprintln(stderr, "liro-bridge: sign-digest:", c.T(i18n.CodeKey(result.Failures[0].Code)))
		return 1
	}

	sig := result.Signatures[0]
	if *outPath != "" {
		if err := os.WriteFile(*outPath, sig, 0o600); err != nil {
			fprintln(stderr, "liro-bridge: sign-digest: writing output:", err)
			return 1
		}
		printTestKeyWarning(stderr, innerSession, c)
		return 0
	}

	fprintln(stdout, base64.StdEncoding.EncodeToString(sig))
	printTestKeyWarning(stderr, innerSession, c)
	return 0
}

// parseDigestHex validates and decodes --digest (F2 §6): exactly 64 hex
// characters, i.e. a 32-byte SHA-256 digest.
func parseDigestHex(s string) ([]byte, error) {
	if len(s) != digestHexLen {
		return nil, fmt.Errorf("digest must be exactly %d hex characters, got %d", digestHexLen, len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("digest is not valid hex: %w", err)
	}
	return b, nil
}

// newBatchID returns a fresh identifier for a CLI-originated batch.
// There is no consent screen to correlate it with yet (F5) — it exists
// only so log lines about this batch can be grouped together.
func newBatchID() string {
	return fmt.Sprintf("cli-%d-%d", os.Getpid(), time.Now().UnixNano())
}

func printTestKeyWarning(w io.Writer, sess keysource.Session, c *i18n.Catalogue) {
	if sess.Certificate().IsTestKey {
		fprintln(w, c.T("sign.test_key_warning"))
	}
}

// printCommandError writes err's localised message with the given
// command's own "liro-bridge: <cmd>:" prefix (Task 8). It replaces what
// used to be printSignDigestError, hard-coded to "sign-digest" and
// reused, wrongly, by "sign" — which is why running `sign` printed
// "liro-bridge: sign-digest: ..." for an error that had nothing to do
// with sign-digest.
func printCommandError(w io.Writer, cmd string, err error, c *i18n.Catalogue) {
	var e *errs.Error
	if errors.As(err, &e) {
		fprintln(w, "liro-bridge:", cmd+":", c.T(i18n.CodeKey(e.Code)))
		return
	}
	fprintln(w, "liro-bridge:", cmd+":", err)
}

func printTimingReport(w io.Writer, result *signing.BatchResult, c *i18n.Catalogue) {
	signed := len(result.Signatures) - len(result.Failures)
	fprintf(w, c.T("sign.report_heading")+"\n", signed, len(result.Signatures))
	fprintf(w, "  %s %s\n", c.T("sign.first_signature_label"), result.Timing.FirstSignature)
	fprintf(w, "  %s %s\n", c.T("sign.median_subsequent_label"), result.Timing.MedianSubsequent)
	fprintf(w, "  %s %s\n", c.T("sign.pin_policy_label"), pinPolicyLabel(result.Timing.PINPolicy, c))
	fprintf(w, "  %s %s\n", c.T("sign.total_label"), result.Timing.Total)
}

func pinPolicyLabel(p signing.PINPolicy, c *i18n.Catalogue) string {
	switch p {
	case signing.PINPolicyPerBatch:
		return c.T("sign.pin_policy_per_batch")
	case signing.PINPolicyPerSignature:
		return c.T("sign.pin_policy_per_signature")
	default:
		return c.T("sign.pin_policy_unknown")
	}
}
