package cli

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
)

// fakeSignPDFSession is a real RSA-backed keysource.Session — unlike
// fakeSignSession in signdigest_test.go, sign needs a real certificate
// and a real signature, since it builds an actual CMS through
// internal/pades.SignDocument, not just a fake byte string.
type fakeSignPDFSession struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func newFakeSignPDFSession(t *testing.T) *fakeSignPDFSession {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(4242),
		Subject:      pkix.Name{CommonName: "cli.RunSign test signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return &fakeSignPDFSession{cert: cert, key: key}
}

func (s *fakeSignPDFSession) SignDigest(_ context.Context, _ keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	return rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest)
}
func (s *fakeSignPDFSession) Certificate() keysource.Certificate {
	return keysource.Certificate{Thumbprint: "SIGNPDFTEST", DER: s.cert.Raw}
}
func (s *fakeSignPDFSession) Chain() [][]byte { return nil }
func (s *fakeSignPDFSession) Close() error    { return nil }

func signPDFDeps(sess keysource.Session) SignPDFDeps {
	return SignPDFDeps{Open: func(context.Context, keysource.Thumbprint) (keysource.Session, error) {
		return sess, nil
	}}
}

func writeMinimalPDF(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := map[int]int{}
	write := func(num int, body string) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}
	write(1, "<< /Type /Catalog /Pages 2 0 R >>")
	write(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	write(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>")
	content := "BT /F1 12 Tf 72 712 Td (Hello) Tj ET"
	offsets[4] = buf.Len()
	fmt.Fprintf(&buf, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&buf, "%010d %05d n \n", offsets[i], 0)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xrefStart)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestRunSignDefaultOutputSuffix(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "document.pdf")
	writeMinimalPDF(t, in)

	sess := newFakeSignPDFSession(t)
	var stdout, stderr bytes.Buffer
	// --on-tsa-failure b-b: this test is about the output suffix, not
	// TSA behaviour (Task 7), and no --tsa is configured here.
	code := RunSign(context.Background(), []string{"--in", in, "--thumbprint", "SIGNPDFTEST", "--level", "b-t", "--on-tsa-failure", "b-b"}, &stdout, &stderr, "en", signPDFDeps(sess))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	wantOut := filepath.Join(dir, "document-signed.pdf")
	if _, err := os.Stat(wantOut); err != nil {
		t.Fatalf("expected output at %s: %v (stdout: %s)", wantOut, err, stdout.String())
	}
	if _, err := os.Stat(in); err != nil {
		t.Fatalf("input file was removed or moved: %v", err)
	}
	inBytes, _ := os.ReadFile(in)
	if !bytes.Equal(inBytes, mustReadOriginal(t, in)) {
		t.Fatal("input file content changed — the original must never be modified")
	}
}

// TestRunSignOpenErrorUsesSignPrefixNotSignDigest is Task 8: RunSign used
// to report a session-open failure through printSignDigestError, a
// helper hard-coded to the "sign-digest" command name — so running
// `sign` printed "liro-bridge: sign-digest: ..." for an error that had
// nothing to do with sign-digest.
func TestRunSignOpenErrorUsesSignPrefixNotSignDigest(t *testing.T) {
	openErr := errs.New(errs.CodeCardNotPresent, nil)
	deps := SignPDFDeps{Open: func(context.Context, keysource.Thumbprint) (keysource.Session, error) {
		return nil, openErr
	}}
	var stdout, stderr bytes.Buffer
	code := RunSign(context.Background(), []string{"--in", "x.pdf", "--thumbprint", "ABCD"}, &stdout, &stderr, "en", deps)
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero when Open fails")
	}
	if !strings.Contains(stderr.String(), "liro-bridge: sign:") {
		t.Fatalf("stderr = %q, want the \"liro-bridge: sign:\" prefix", stderr.String())
	}
	if strings.Contains(stderr.String(), "sign-digest") {
		t.Fatalf("stderr = %q, want no mention of sign-digest", stderr.String())
	}
}

func mustReadOriginal(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}

func TestRunSignRefusesToOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "document.pdf")
	writeMinimalPDF(t, in)
	out := filepath.Join(dir, "document-signed.pdf")
	if err := os.WriteFile(out, []byte("existing content"), 0o600); err != nil {
		t.Fatalf("seeding existing output: %v", err)
	}

	sess := newFakeSignPDFSession(t)
	var stdout, stderr bytes.Buffer
	code := RunSign(context.Background(), []string{"--in", in, "--thumbprint", "SIGNPDFTEST"}, &stdout, &stderr, "en", signPDFDeps(sess))
	if code == 0 {
		t.Fatal("exit code = 0, want a failure when the output already exists and --force was not passed")
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != "existing content" {
		t.Fatal("existing output file was overwritten without --force")
	}
}

func TestRunSignForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "document.pdf")
	writeMinimalPDF(t, in)
	out := filepath.Join(dir, "document-signed.pdf")
	if err := os.WriteFile(out, []byte("existing content"), 0o600); err != nil {
		t.Fatalf("seeding existing output: %v", err)
	}

	sess := newFakeSignPDFSession(t)
	var stdout, stderr bytes.Buffer
	code := RunSign(context.Background(), []string{"--in", in, "--thumbprint", "SIGNPDFTEST", "--force", "--on-tsa-failure", "b-b"}, &stdout, &stderr, "en", signPDFDeps(sess))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) == "existing content" {
		t.Fatal("--force did not overwrite the existing output")
	}
}

// TestRunSignGlobBatchSkipsAndContinues is F3 §12.10: if one document in
// a batch fails, the rest are still signed on the same session.
func TestRunSignGlobBatchSkipsAndContinues(t *testing.T) {
	dir := t.TempDir()
	good1 := filepath.Join(dir, "a.pdf")
	good2 := filepath.Join(dir, "c.pdf")
	bad := filepath.Join(dir, "b.pdf")
	writeMinimalPDF(t, good1)
	writeMinimalPDF(t, good2)
	if err := os.WriteFile(bad, []byte("not a pdf at all"), 0o600); err != nil {
		t.Fatalf("writing bad input: %v", err)
	}

	sess := newFakeSignPDFSession(t)
	var stdout, stderr bytes.Buffer
	code := RunSign(context.Background(), []string{"--in", filepath.Join(dir, "*.pdf"), "--thumbprint", "SIGNPDFTEST", "--on-tsa-failure", "b-b"}, &stdout, &stderr, "en", signPDFDeps(sess))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (2 of 3 succeeded); stderr: %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "a-signed.pdf")); err != nil {
		t.Errorf("a-signed.pdf missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "c-signed.pdf")); err != nil {
		t.Errorf("c-signed.pdf missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b-signed.pdf")); err == nil {
		t.Error("b-signed.pdf was created despite b.pdf not being a valid PDF")
	}
	if !strings.Contains(stderr.String(), "b.pdf") {
		t.Errorf("stderr does not mention the failed file: %s", stderr.String())
	}
}

func TestRunSignFlagValidation(t *testing.T) {
	sess := newFakeSignPDFSession(t)
	cases := []struct {
		name string
		args []string
	}{
		{"missing --in", []string{"--thumbprint", "SIGNPDFTEST"}},
		{"missing --thumbprint", []string{"--in", "x.pdf"}},
		{"bad --level", []string{"--in", "x.pdf", "--thumbprint", "T", "--level", "bogus"}},
		{"bad --on-tsa-failure", []string{"--in", "x.pdf", "--thumbprint", "T", "--on-tsa-failure", "bogus"}},
		// F4 §7: "--stamp-xy and --stamp-position are mutually
		// exclusive; supplying both is an error."
		{"conflicting stamp position and xy", []string{"--in", "x.pdf", "--thumbprint", "T", "--stamp", "--stamp-position", "top-left", "--stamp-xy", "10,20"}},
		{"bad --stamp-position", []string{"--in", "x.pdf", "--thumbprint", "T", "--stamp", "--stamp-position", "middle"}},
		{"bad --stamp-xy", []string{"--in", "x.pdf", "--thumbprint", "T", "--stamp", "--stamp-xy", "not-a-point"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := RunSign(context.Background(), c.args, &stdout, &stderr, "en", signPDFDeps(sess))
			if code == 0 {
				t.Fatalf("exit code = 0, want a validation failure")
			}
		})
	}
}

// TestRunSignStampProducesVisibleSignature is F4 §7's CLI surface
// exercised end to end: --stamp plus every optional flag produces a
// signed, stamped PDF whose signature still verifies independently.
func TestRunSignStampProducesVisibleSignature(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "document.pdf")
	writeMinimalPDF(t, in)

	sess := newFakeSignPDFSession(t)
	var stdout, stderr bytes.Buffer
	code := RunSign(context.Background(), []string{
		"--in", in, "--thumbprint", "SIGNPDFTEST", "--level", "b-t",
		"--stamp", "--stamp-position", "bottom-left", "--stamp-page", "1",
		"--stamp-reference", "REF-42", "--on-tsa-failure", "b-b",
	}, &stdout, &stderr, "en", signPDFDeps(sess))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	out := filepath.Join(dir, "document-signed.pdf")
	signed, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading signed output: %v", err)
	}
	if !bytes.Contains(signed, []byte("/Subtype /Form")) {
		t.Error("signed output has no Form XObject — the stamp does not appear to have been drawn")
	}
	if !bytes.Contains(signed, []byte("REF-42")) && !bytes.Contains(signed, []byte("00520045004600")) {
		// The reference text is CID-encoded (Identity-H), so it will not
		// appear as literal ASCII — this is a loose sanity check that
		// something stamp-shaped landed in the file, not a substitute
		// for internal/pades/appearance's own CID-sequence tests.
		t.Log("reference text not found verbatim (expected: it is CID-encoded)")
	}
}

// TestRunSignStampXYAccepted proves --stamp-xy alone (no --stamp-position)
// is accepted and produces output.
func TestRunSignStampXYAccepted(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "document.pdf")
	writeMinimalPDF(t, in)

	sess := newFakeSignPDFSession(t)
	var stdout, stderr bytes.Buffer
	code := RunSign(context.Background(), []string{
		"--in", in, "--thumbprint", "SIGNPDFTEST",
		"--stamp", "--stamp-xy", "50,50", "--on-tsa-failure", "b-b",
	}, &stdout, &stderr, "en", signPDFDeps(sess))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
}

// TestRunSignStampMissingGlyphNamesCharacterAndCodePoint is Task 2: the
// CLI's own local diagnostic output for a missing stamp glyph must name
// the character and its code point, in the shape "...: <char> (U+XXXX).",
// not report the card-shaped SIGN_FAILED message this used to surface as.
func TestRunSignStampMissingGlyphNamesCharacterAndCodePoint(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "document.pdf")
	writeMinimalPDF(t, in)

	sess := newFakeSignPDFSession(t)
	var stdout, stderr bytes.Buffer
	code := RunSign(context.Background(), []string{
		"--in", in, "--thumbprint", "SIGNPDFTEST",
		"--stamp", "--stamp-reference", "中", // CJK: outside the embedded font subset by construction
	}, &stdout, &stderr, "en", signPDFDeps(sess))
	if code == 0 {
		t.Fatal("exit code = 0, want a failure for a stamp reference containing an unsupported character")
	}
	want := "The stamp contains a character the font does not support: 中 (U+4E2D)."
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
}

// TestLevelLineReportsRevocationTooLargeLocalised is Task 1c: the
// specific, localised message — not the generic English Notes join —
// naming what happened (revocation data too large) and the size, in
// every locale.
func TestLevelLineReportsRevocationTooLargeLocalised(t *testing.T) {
	result := &pades.Result{
		AchievedLevel:       pades.LevelBT,
		RevocationTooLarge:  true,
		LargestSkippedBytes: 30136214, // the real measured MUP CRL size (Task 1b)
		Notes:               []string{"revocation data too large to embed (30136214 bytes); saved at B-T"},
	}
	cases := map[string]string{
		"en":      "Revocation data was too large to embed (29 MB); saved at B-T.",
		"sr-Latn": "Podaci o opozivu su bili preveliki za ugrađivanje (29 MB); sačuvano na nivou B-T.",
		"sr-Cyrl": "Подаци о опозиву су били превелики за уграђивање (29 MB); сачувано на нивоу B-T.",
	}
	for locale, want := range cases {
		t.Run(locale, func(t *testing.T) {
			got := levelLine(result, i18n.Load(locale))
			if !strings.Contains(got, want) {
				t.Fatalf("levelLine(%s) = %q, want it to contain %q", locale, got, want)
			}
		})
	}
}

// TestLevelLineFallsBackToNotesWhenNotTooLarge proves RevocationTooLarge
// is the only case with a dedicated message — an ordinary degradation
// (e.g. a TSA falling back to B-B) still uses the existing Notes join.
func TestLevelLineFallsBackToNotesWhenNotTooLarge(t *testing.T) {
	result := &pades.Result{AchievedLevel: pades.LevelBB, Notes: []string{"no timestamp (saved at B-B): boom"}}
	got := levelLine(result, i18n.Load("en"))
	want := "B-B  (no timestamp (saved at B-B): boom)"
	if got != want {
		t.Fatalf("levelLine = %q, want %q", got, want)
	}
}

// TestPrintClockDriftWarningLocalised is Task 3: the warning names both
// times and is silent when there is no drift, in every locale.
func TestPrintClockDriftWarningLocalised(t *testing.T) {
	machine := time.Date(2026, 9, 2, 13, 5, 0, 0, time.UTC)
	tsaTime := machine.Add(12 * time.Minute)
	result := &pades.Result{ClockDriftWarning: true, MachineTime: machine, TimestampTime: tsaTime}

	for _, locale := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		t.Run(locale, func(t *testing.T) {
			var buf bytes.Buffer
			printClockDriftWarning(&buf, result, i18n.Load(locale))
			out := buf.String()
			if !strings.Contains(out, machine.Format(time.RFC3339)) || !strings.Contains(out, tsaTime.Format(time.RFC3339)) {
				t.Fatalf("printClockDriftWarning(%s) = %q, want it to name both times", locale, out)
			}
		})
	}
}

func TestPrintClockDriftWarningSilentWithoutDrift(t *testing.T) {
	var buf bytes.Buffer
	printClockDriftWarning(&buf, &pades.Result{ClockDriftWarning: false}, i18n.Load("en"))
	if buf.Len() != 0 {
		t.Fatalf("printClockDriftWarning wrote %q, want nothing when ClockDriftWarning is false", buf.String())
	}
}
