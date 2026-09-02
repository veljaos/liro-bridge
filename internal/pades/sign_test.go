package pades

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
	"github.com/veljaos/liro-bridge/internal/pades/verify"
)

// fakeSession is a minimal keysource.Session backed by a real RSA key
// and a self-signed test certificate — this file's stand-in for real
// hardware/the soft token, exactly like every other package's own
// tests in this phase.
type fakeSession struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func newFakeSession(t *testing.T) *fakeSession {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(555),
		Subject:      pkix.Name{CommonName: "pades.SignDocument test signer"},
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
	return &fakeSession{cert: cert, key: key}
}

func (s *fakeSession) SignDigest(_ context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if alg != keysource.DigestSHA256 || len(digest) != 32 {
		return nil, fmt.Errorf("unexpected digest")
	}
	return rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest)
}

func (s *fakeSession) Certificate() keysource.Certificate {
	return keysource.Certificate{Thumbprint: "TEST", DER: s.cert.Raw}
}

func (s *fakeSession) Chain() [][]byte { return nil }
func (s *fakeSession) Close() error    { return nil }

func buildMinimalPDF(t *testing.T) []byte {
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
	return buf.Bytes()
}

// verifyResult independently verifies out with internal/pades/verify —
// the same package CI runs against every signature this project
// produces (F3 §8/§16.4).
func verifyResult(t *testing.T, out []byte) *verify.Result {
	t.Helper()
	slots, err := verify.FindSignatures(out)
	if err != nil {
		t.Fatalf("verify.FindSignatures: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("no signature found by the independent verifier")
	}
	return verify.VerifySignature(out, slots[len(slots)-1])
}

func TestSignDocumentBBWithNoTSA(t *testing.T) {
	sess := newFakeSession(t)
	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBB {
		t.Fatalf("AchievedLevel = %s, want B-B", result.AchievedLevel)
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
		t.Fatalf("independent verification failed: %+v errors=%v", r, r.Errors)
	}
	if r.HasTimestamp {
		t.Fatal("HasTimestamp = true for a B-B result")
	}
}

func TestSignDocumentBTWithRealTSA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	sess := newFakeSession(t)
	client := tsa.NewClient("https://test-tsa.ca.posta.rs/timestamp1", tsa.Auth{BasicUsername: "Test.Korisnik", BasicPassword: "123456"})

	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel: LevelBT,
		TSA:            client,
	})
	if err != nil {
		t.Skipf("Pošta test TSA unreachable, skipping: %v", err)
	}
	if result.AchievedLevel != LevelBT {
		t.Fatalf("AchievedLevel = %s, want B-T (notes: %v)", result.AchievedLevel, result.Notes)
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
		t.Fatalf("independent verification failed: %+v errors=%v", r, r.Errors)
	}
	if !r.HasTimestamp || !r.TimestampOK {
		t.Fatalf("Result = %+v, want a validated timestamp", r)
	}
}

// brokenTSAClient always fails, simulating an unreachable TSA without
// needing the network.
func brokenTSAClient() *tsa.Client {
	return tsa.NewClient("http://127.0.0.1:1/unreachable", tsa.Auth{})
}

func TestSignDocumentAbortsOnTSAFailureByDefault(t *testing.T) {
	sess := newFakeSession(t)
	_, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel:    LevelBT,
		TSA:               brokenTSAClient(),
		OnTSAFailureAbort: true,
	})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeTSAUnavailable {
		t.Fatalf("err = %v, want CodeTSAUnavailable", err)
	}
}

func TestSignDocumentFallsBackToBBOnTSAFailure(t *testing.T) {
	sess := newFakeSession(t)
	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel:    LevelBT,
		TSA:               brokenTSAClient(),
		OnTSAFailureAbort: false,
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBB {
		t.Fatalf("AchievedLevel = %s, want B-B (never silently claim more)", result.AchievedLevel)
	}
	if len(result.Notes) == 0 {
		t.Fatal("no note explaining the degradation to B-B")
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK {
		t.Fatalf("the B-B fallback signature itself must still verify: %+v", r)
	}
}

// TestSignDocumentRejectedTSAMapsToCodeTSARejected is Task 6: an HTTP
// 4xx from the TSA is a deterministic refusal (bad credentials, a
// malformed request), not a "did not respond" condition, and must map to
// TSA_REJECTED, not TSA_UNAVAILABLE — the two need different user
// actions (check credentials, versus wait and retry).
func TestSignDocumentRejectedTSAMapsToCodeTSARejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	sess := newFakeSession(t)
	_, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel:    LevelBT,
		TSA:               tsa.NewClient(server.URL, tsa.Auth{}),
		OnTSAFailureAbort: true,
	})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeTSARejected {
		t.Fatalf("err = %v, want CodeTSARejected", err)
	}
}

// TestSignDocumentUnavailableTSAStillMapsToCodeTSAUnavailable is Task 6's
// other half: a genuinely unreachable TSA (exhausted retries against a
// closed port) must still map to TSA_UNAVAILABLE, not TSA_REJECTED —
// classifyTSAError must not conflate the two in either direction.
func TestSignDocumentUnavailableTSAStillMapsToCodeTSAUnavailable(t *testing.T) {
	sess := newFakeSession(t)
	_, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel:    LevelBT,
		TSA:               brokenTSAClient(),
		OnTSAFailureAbort: true,
	})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeTSAUnavailable {
		t.Fatalf("err = %v, want CodeTSAUnavailable", err)
	}
}

// TestSignDocumentFailsWhenLevelRequestedButNoTSAConfigured is Task 7:
// requesting B-T/B-LT with no TSA at all previously skipped the whole
// timestamp step silently and returned a B-B result with no error and no
// note — SPEC §12.8/§18.11's "never silently downgrade" applies just as
// much to "nobody configured a TSA" as it does to a TSA that was
// contacted and failed.
func TestSignDocumentFailsWhenLevelRequestedButNoTSAConfigured(t *testing.T) {
	sess := newFakeSession(t)
	_, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel:    LevelBT,
		OnTSAFailureAbort: true,
	})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeTSAUnavailable {
		t.Fatalf("err = %v, want CodeTSAUnavailable (no TSA configured)", err)
	}
}

// TestSignDocumentNoTSAConfiguredFallsBackToBBWithNote mirrors
// TestSignDocumentFallsBackToBBOnTSAFailure for the "no TSA at all" case:
// with OnTSAFailureAbort false, the result is still B-B (never claims
// more than was reached), but now carries a note explaining why —
// exactly what was missing before Task 7's fix.
func TestSignDocumentNoTSAConfiguredFallsBackToBBWithNote(t *testing.T) {
	sess := newFakeSession(t)
	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel:    LevelBT,
		OnTSAFailureAbort: false,
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBB {
		t.Fatalf("AchievedLevel = %s, want B-B", result.AchievedLevel)
	}
	if len(result.Notes) == 0 {
		t.Fatal("no note explaining why B-T was requested but not reached")
	}
}

// TestSignDocumentMissingGlyphMapsToCodeStampGlyphMissing is Task 2: a
// character the stamp's font subset cannot draw used to surface as
// SIGN_FAILED, indistinguishable from a card or reader problem, sending
// the user to check hardware for a problem that was in the stamp's own
// text — see docs/decisions.md for what superseded that mapping.
func TestSignDocumentMissingGlyphMapsToCodeStampGlyphMissing(t *testing.T) {
	sess := newFakeSession(t)
	_, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		Stamp: &StampOptions{
			Label:     "Digitally signed by",
			Reference: "中", // CJK: outside the subset by construction (charset.go never includes CJK)
		},
	})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeStampGlyphMissing {
		t.Fatalf("err = %v, want CodeStampGlyphMissing", err)
	}
	if e.Details["character"] != "中" {
		t.Errorf("Details[character] = %v, want %q", e.Details["character"], "中")
	}
	if e.Details["codePoint"] != "U+4E2D" {
		t.Errorf("Details[codePoint] = %v, want U+4E2D", e.Details["codePoint"])
	}
}

func TestSignDocumentOversizedReservationFailsLoudly(t *testing.T) {
	sess := newFakeSession(t)
	_, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{ReservedBytes: 16})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeSignFailed {
		t.Fatalf("err = %v, want CodeSignFailed for an undersized reservation", err)
	}
}

func TestSignDocumentAlreadySignedDocumentBothVerify(t *testing.T) {
	// The single most important test in F3 (§10.2), exercised at this
	// package's own level: sign, then sign again (a second revision),
	// and confirm both signatures still verify independently.
	sess1 := newFakeSession(t)
	result1, err := SignDocument(context.Background(), buildMinimalPDF(t), sess1, Options{})
	if err != nil {
		t.Fatalf("first SignDocument: %v", err)
	}

	sess2 := newFakeSession(t)
	result2, err := SignDocument(context.Background(), result1.Bytes, sess2, Options{})
	if err != nil {
		t.Fatalf("second SignDocument: %v", err)
	}
	if !bytes.HasPrefix(result2.Bytes, result1.Bytes) {
		t.Fatal("the second signing operation did not append to the first's exact bytes")
	}

	slots, err := verify.FindSignatures(result2.Bytes)
	if err != nil || len(slots) != 2 {
		t.Fatalf("FindSignatures: %v (slots=%d, want 2)", err, len(slots))
	}
	for i, slot := range slots {
		r := verify.VerifySignature(result2.Bytes, slot)
		if len(r.Errors) > 0 || !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
			t.Fatalf("signature %d failed independent verification: %+v errors=%v", i, r, r.Errors)
		}
	}
}
