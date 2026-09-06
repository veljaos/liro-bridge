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

	"golang.org/x/crypto/ocsp"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades/pdf"
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

// chainedSession is like fakeSession but its certificate is issued by a
// separate CA rather than self-signed, and Chain() returns that CA — the
// shape internal/pades/dss.CollectRevocation needs (signer, then its
// issuer) to actually attempt revocation collection at all (a self-signed
// certificate is its own excluded root and is never checked, see
// CollectRevocation's own doc comment).
type chainedSession struct {
	cert  *x509.Certificate
	key   *rsa.PrivateKey
	caDER []byte
}

func (s *chainedSession) SignDigest(_ context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if alg != keysource.DigestSHA256 || len(digest) != 32 {
		return nil, fmt.Errorf("unexpected digest")
	}
	return rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest)
}

func (s *chainedSession) Certificate() keysource.Certificate {
	return keysource.Certificate{Thumbprint: "TEST", DER: s.cert.Raw}
}

func (s *chainedSession) Chain() [][]byte { return [][]byte{s.caDER} }
func (s *chainedSession) Close() error    { return nil }

// newTestCA builds a self-signed CA certificate usable to sign both a
// leaf certificate and a CRL.
func newTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pades.SignDocument test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(CA): %v", err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(CA): %v", err)
	}
	return ca, caKey
}

// newChainedSession issues a leaf certificate from ca/caKey, pointing its
// AIA/CRL DP extensions at ocspURLs/crlURLs, and wraps it in a
// chainedSession.
func newChainedSession(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, ocspURLs, crlURLs []string) *chainedSession {
	t.Helper()
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "pades.SignDocument test signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageContentCommitment,
		OCSPServer:            ocspURLs,
		CRLDistributionPoints: crlURLs,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(leaf): %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(leaf): %v", err)
	}
	return &chainedSession{cert: leaf, key: leafKey, caDER: ca.Raw}
}

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

// TestSignDocumentClockDriftWarningWhenTimestampDisagrees is Task 3: a
// genTime that disagrees with the /M value (signingDate, here forced via
// Options.Now so the test controls both sides of the comparison) by more
// than five minutes must produce a warning — but never fail the
// operation or alter which bytes get written beyond the timestamp itself
// arriving normally.
func TestSignDocumentClockDriftWarningWhenTimestampDisagrees(t *testing.T) {
	sess := newFakeSession(t)
	// Both times are anchored to the real time.Now() (not an arbitrary
	// fixed date) because internal/pades/tsa.Client's own ±10-minute
	// hard skew check (internal/pades/tsa/client.go's validateResponse)
	// compares genTime against the real current time independently of
	// this test — a fixed-date machineTime far from actual "now" would
	// make that unrelated check fail first.
	machineTime := time.Now()
	tsaTime := machineTime.Add(7 * time.Minute) // beyond the 5-minute threshold, within the TSA client's own 10-minute skew window

	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel: LevelBT,
		TSA:            fakeTSAClientAt(t, tsaTime),
		Now:            machineTime,
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBT {
		t.Fatalf("AchievedLevel = %s, want B-T", result.AchievedLevel)
	}
	if !result.ClockDriftWarning {
		t.Fatal("ClockDriftWarning = false, want true (7 minutes of drift exceeds the 5-minute threshold)")
	}
	if !result.MachineTime.Equal(machineTime) {
		t.Fatalf("MachineTime = %s, want %s", result.MachineTime, machineTime)
	}
	// TimestampTime round-trips through RFC 3161's GeneralizedTime
	// encoding (whole seconds, UTC), so it is compared with a
	// sub-second tolerance rather than exact equality.
	if d := result.TimestampTime.Sub(tsaTime); d < -time.Second || d > time.Second {
		t.Fatalf("TimestampTime = %s, want approximately %s", result.TimestampTime, tsaTime)
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK || !r.HasTimestamp || !r.TimestampOK {
		t.Fatalf("a clock-drift warning must never affect the signature itself: %+v errors=%v", r, r.Errors)
	}
}

// TestSignDocumentNoClockDriftWarningWithinThreshold proves the warning
// does not fire for ordinary, sub-threshold skew — a few seconds of
// difference between requesting the timestamp and receiving it is
// completely normal and must not nag the user.
func TestSignDocumentNoClockDriftWarningWithinThreshold(t *testing.T) {
	sess := newFakeSession(t)
	now := time.Now()

	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel: LevelBT,
		TSA:            fakeTSAClientAt(t, now),
		Now:            now,
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.ClockDriftWarning {
		t.Fatal("ClockDriftWarning = true, want false (genTime matches the machine clock exactly)")
	}
}

// TestSignDocumentBLTDegradesToBTWhenRevocationTooLarge is Task 1b/1c's
// end-to-end case: a CRL that is fetched and valid, but larger than
// Options.MaxRevocationArtefactSize, must not be embedded — the achieved
// level stays B-T (never a silently-overclaimed B-LT, SPEC §18.11), and
// Result carries the specific reason (as opposed to plain OCSP/CRL
// unavailability) so a caller can report it honestly (Task 1c).
func TestSignDocumentBLTDegradesToBTWhenRevocationTooLarge(t *testing.T) {
	ca, caKey := newTestCA(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tmpl := &x509.RevocationList{
			Number:     big.NewInt(1),
			ThisUpdate: time.Now().Add(-time.Minute),
			NextUpdate: time.Now().Add(time.Hour),
		}
		der, err := x509.CreateRevocationList(rand.Reader, tmpl, ca, caKey)
		if err != nil {
			t.Fatalf("CreateRevocationList: %v", err)
		}
		_, _ = w.Write(der)
	}))
	defer server.Close()

	sess := newChainedSession(t, ca, caKey, nil, []string{server.URL})

	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		RequestedLevel:            LevelBLT,
		TSA:                       fakeTSAClientAt(t, time.Now()),
		MaxRevocationArtefactSize: 16, // any real CRL exceeds this
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBT {
		t.Fatalf("AchievedLevel = %s, want B-T (B-LT must never be claimed when revocation data was skipped)", result.AchievedLevel)
	}
	if !result.RevocationTooLarge {
		t.Fatal("RevocationTooLarge = false, want true")
	}
	if result.LargestSkippedBytes <= 16 {
		t.Fatalf("LargestSkippedBytes = %d, want the CRL's real (larger than 16) size", result.LargestSkippedBytes)
	}
	if len(result.Notes) == 0 {
		t.Fatal("no note explaining the B-T degradation")
	}
	// D-079: a skipped-for-size CRL means no evidence was actually
	// obtained for embedding, so no /DSS revision is written at all —
	// not one containing only /Certs. The reported level and reason
	// above are unchanged by that fix; only the writing became
	// conditional.
	if bytes.Contains(result.Bytes, []byte("/DSS")) {
		t.Fatal("output contains /DSS, want no DSS revision written when the only evidence was too large to embed")
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK {
		t.Fatalf("the B-T signature itself must still verify despite the skipped /DSS: %+v", r)
	}
}

// TestSignDocumentBLTSkipsDSSWhenNoRevocationEvidence is D-079's core
// case: no OCSP responder and no CRL distribution point at all (the "no
// endpoint exists" branch of dss.CollectRevocation), so nothing is ever
// collected to embed. A /DSS dictionary carrying only /Certs asserts
// long-term validation evidence the document does not actually have —
// the certificates are already inside the CMS — so the fix is to skip
// the revision entirely rather than write an empty one. The achieved
// level and its reason are unchanged (still B-T, still "OCSP/CRL
// unavailable"); only whether a revision gets appended changes.
func TestSignDocumentBLTSkipsDSSWhenNoRevocationEvidence(t *testing.T) {
	ca, caKey := newTestCA(t)
	input := buildMinimalPDF(t)

	sess := newChainedSession(t, ca, caKey, nil, nil) // no OCSP, no CRL DP configured
	result, err := SignDocument(context.Background(), input, sess, Options{
		RequestedLevel: LevelBLT,
		TSA:            fakeTSAClientAt(t, time.Now()),
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBT {
		t.Fatalf("AchievedLevel = %s, want B-T (no revocation evidence exists to justify B-LT)", result.AchievedLevel)
	}
	if bytes.Contains(result.Bytes, []byte("/DSS")) {
		t.Fatal("output contains /DSS, want no DSS revision written when there is no revocation evidence to embed")
	}
	if !bytes.HasPrefix(result.Bytes, input) {
		t.Fatal("the original document bytes are not a literal prefix of the signed output")
	}

	// A plain B-T signing of the same input never attempts applyDSS at
	// all, so its revision count is the ground truth this result's
	// count must match exactly: the skipped-DSS case must not leave any
	// trace of an extra revision beyond the signature itself.
	btSess := newChainedSession(t, ca, caKey, nil, nil)
	btResult, err := SignDocument(context.Background(), input, btSess, Options{
		RequestedLevel: LevelBT,
		TSA:            fakeTSAClientAt(t, time.Now()),
	})
	if err != nil {
		t.Fatalf("baseline plain B-T SignDocument: %v", err)
	}
	if got, want := bytes.Count(result.Bytes, []byte("%%EOF")), bytes.Count(btResult.Bytes, []byte("%%EOF")); got != want {
		t.Fatalf("%%%%EOF count = %d, want %d (same as a plain B-T signing: no extra revision beyond the signature)", got, want)
	}

	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK || !r.HasTimestamp || !r.TimestampOK {
		t.Fatalf("independent verification failed: %+v errors=%v", r, r.Errors)
	}
}

// TestSignDocumentBLTAchievedWithOCSPEvidence is the complement of
// TestSignDocumentBLTSkipsDSSWhenNoRevocationEvidence: with a fake OCSP
// responder returning a small, valid "good" response, real revocation
// evidence exists, so the /DSS revision is written and B-LT is reached.
func TestSignDocumentBLTAchievedWithOCSPEvidence(t *testing.T) {
	ca, caKey := newTestCA(t)
	var leafCert *x509.Certificate // set below, read by the handler closure at request time
	ocspSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respTemplate := ocsp.Response{
			Status:       ocsp.Good,
			SerialNumber: leafCert.SerialNumber,
			ThisUpdate:   time.Now().Add(-time.Minute),
			NextUpdate:   time.Now().Add(time.Hour),
			Certificate:  ca,
		}
		der, err := ocsp.CreateResponse(ca, ca, respTemplate, caKey)
		if err != nil {
			t.Fatalf("ocsp.CreateResponse: %v", err)
		}
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(der)
	}))
	defer ocspSrv.Close()

	sess := newChainedSession(t, ca, caKey, []string{ocspSrv.URL}, nil)
	leafCert = sess.cert
	input := buildMinimalPDF(t)

	result, err := SignDocument(context.Background(), input, sess, Options{
		RequestedLevel: LevelBLT,
		TSA:            fakeTSAClientAt(t, time.Now()),
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBLT {
		t.Fatalf("AchievedLevel = %s, want B-LT (notes: %v)", result.AchievedLevel, result.Notes)
	}
	if !bytes.HasPrefix(result.Bytes, input) {
		t.Fatal("the original document bytes are not a literal prefix of the signed output")
	}

	doc, err := pdf.Parse(result.Bytes)
	if err != nil {
		t.Fatalf("re-parse signed output: %v", err)
	}
	root, ok := doc.ResolveDict(doc.Trailer().Get(pdf.Name("Root")))
	if !ok {
		t.Fatal("/Root did not resolve")
	}
	dssDict, ok := doc.ResolveDict(root.Get(pdf.Name("DSS")))
	if !ok {
		t.Fatal("/DSS did not resolve, want a written DSS revision")
	}
	ocsps, ok := doc.Resolve(dssDict.Get(pdf.Name("OCSPs"))).(pdf.Array)
	if !ok || len(ocsps) == 0 {
		t.Fatalf("/DSS/OCSPs = %#v, want at least one entry", dssDict.Get(pdf.Name("OCSPs")))
	}

	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK || !r.HasTimestamp || !r.TimestampOK {
		t.Fatalf("independent verification failed: %+v errors=%v", r, r.Errors)
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

// unchainedSession is a signer whose Chain() is empty: the certificate
// is all there is. That is not a contrivance — MUP embeds only the
// signer certificate in its CMS (SPEC §11.8), so this is what the
// pipeline holds whenever chain completion fails, which is the same
// network outage that stops OCSP answering. It is also exactly the
// soft token's own shape.
type unchainedSession struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func (s *unchainedSession) SignDigest(_ context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if alg != keysource.DigestSHA256 || len(digest) != 32 {
		return nil, fmt.Errorf("unexpected digest")
	}
	return rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest)
}

func (s *unchainedSession) Certificate() keysource.Certificate {
	return keysource.Certificate{Thumbprint: "TESTNOCHAIN", DER: s.cert.Raw}
}

func (s *unchainedSession) Chain() [][]byte { return nil }
func (s *unchainedSession) Close() error    { return nil }

func newUnchainedSession(t *testing.T) *unchainedSession {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(3),
		Subject:               pkix.Name{CommonName: "pades.SignDocument unchained signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageContentCommitment,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return &unchainedSession{cert: cert, key: key}
}

// TestBLTIsNeverClaimedForADocumentWithNoDSS is the regression test for
// what FTEST found by signing at every level through the shipped binary
// and then looking at the bytes rather than at the reported level.
//
// Measured, `liro-bridge sign --level b-lt` against the soft token:
//
//	Nivo: B-LT
//	66714 bytes  /DSS=False /OCSPs=False /CRLs=False /VRI=False
//
// B-LT is B-T plus a /DSS carrying revocation evidence (SPEC §12.6), so
// a document with no /DSS at all has not reached it, and reporting that
// it has is the level overclaim SPEC §18.11 and D-047 forbid outright.
//
// The reason the existing no-evidence test (above) did not catch it is
// worth stating: dss.Apply's `case i+1 < len(certs)` only expects
// evidence for a certificate whose issuer is also in the list, so a
// chain of exactly one certificate expects none and comes out
// "complete" having collected nothing. Every other test in this package
// uses chainedSession, whose chain is two certificates long.
func TestBLTIsNeverClaimedForADocumentWithNoDSS(t *testing.T) {
	sess := newUnchainedSession(t)
	input := buildMinimalPDF(t)

	result, err := SignDocument(context.Background(), input, sess, Options{
		RequestedLevel: LevelBLT,
		TSA:            fakeTSAClientAt(t, time.Now()),
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}

	hasDSS := bytes.Contains(result.Bytes, []byte("/DSS"))
	if hasDSS {
		t.Fatal("a /DSS was written for a signer with no OCSP and no CRL, which D-079 rules out")
	}
	if result.AchievedLevel == LevelBLT {
		t.Errorf("AchievedLevel = %s for output containing no /DSS, no /OCSPs and no /CRLs; "+
			"B-LT is B-T plus revocation evidence (SPEC §12.6) and claiming it here is the "+
			"overclaim SPEC §18.11 forbids", result.AchievedLevel)
	}
	if result.AchievedLevel != LevelBT {
		t.Errorf("AchievedLevel = %s, want B-T: the signature and its timestamp both succeeded", result.AchievedLevel)
	}
	if len(result.Notes) == 0 {
		t.Error("no note explains why B-LT was requested and not reached")
	}

	// The signature itself is untouched by any of this.
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK || !r.HasTimestamp || !r.TimestampOK {
		t.Fatalf("independent verification failed: %+v errors=%v", r, r.Errors)
	}
}
