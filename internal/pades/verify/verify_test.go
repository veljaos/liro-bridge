package verify

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/pades/cms"
	"github.com/veljaos/liro-bridge/internal/pades/pdf"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
)

// testTSATokenAttrOID is used only by this test file's harness, to
// attach a real TSA response's token to a CMS built for the test — this
// is not the same declaration this package's own non-test code uses to
// recognise the attribute when verifying (see cms_parse.go's
// oidSignatureTimeStampToken), which is the whole point of the
// independence F3 §8 asks for.
var testTSATokenAttrOID = []int{1, 2, 840, 113549, 1, 9, 16, 2, 14}

// This file builds complete signed PDFs using the actual signing
// pipeline (internal/pades/pdf, internal/pades/cms, optionally
// internal/pades/tsa) so this package's own tests exercise the real
// end-to-end path, then verifies them with nothing but this package's
// independently-written code. That direction of dependency — tests
// depending on the signer, never the other way — is exactly what F3 §8
// asks for.

func generateTestCert(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(98765),
		Subject:      pkix.Name{CommonName: "Verify Test Signer"},
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
	return cert, key
}

// buildMinimalPDF is a tiny, single-object classic-xref PDF with one
// page and content stream — enough for BuildPlaceholder.
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

// buildSignedPDF runs the real pipeline: BuildPlaceholder, sign with a
// real RSA key, inject the CMS. withTimestamp attaches a real timestamp
// from Pošta's public test TSA when true (network required; the caller
// decides whether to skip based on the result).
func buildSignedPDF(t *testing.T, withTimestamp bool) (signed []byte, cert *x509.Certificate, tsaErr error) {
	t.Helper()
	cert, key := generateTestCert(t)

	doc, err := pdf.Parse(buildMinimalPDF(t))
	if err != nil {
		t.Fatalf("pdf.Parse: %v", err)
	}
	ph, err := pdf.BuildPlaceholder(doc, pdf.PlaceholderOptions{
		SubFilter:   pdf.Name("ETSI.CAdES.detached"),
		SigningDate: time.Now(),
	})
	if err != nil {
		t.Fatalf("BuildPlaceholder: %v", err)
	}

	digest := ph.Digest()
	b := cms.NewBuilder(cert, nil, digest[:])
	attrsDigest := b.SignedAttrsDigest()
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, attrsDigest[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15: %v", err)
	}
	b.SetSignature(sig)

	if withTimestamp {
		sigDigest := sha256.Sum256(sig)
		c := tsa.NewClient("https://test-tsa.ca.posta.rs/timestamp1", tsa.Auth{BasicUsername: "Test.Korisnik", BasicPassword: "123456"})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := c.Timestamp(ctx, sigDigest[:])
		if err != nil {
			tsaErr = err
		} else {
			b.AddUnsignedAttribute(testTSATokenAttrOID, resp.TokenDER)
		}
	}

	cmsDER, err := b.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := ph.InjectSignature(cmsDER); err != nil {
		t.Fatalf("InjectSignature: %v", err)
	}
	return ph.Bytes, cert, tsaErr
}

func TestVerifySignatureAccepts(t *testing.T) {
	signed, cert, _ := buildSignedPDF(t, false)

	slots, err := FindSignatures(signed)
	if err != nil {
		t.Fatalf("FindSignatures: %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("found %d signature slots, want 1", len(slots))
	}

	r := VerifySignature(signed, slots[0])
	if len(r.Errors) > 0 {
		t.Fatalf("verification errors: %v", r.Errors)
	}
	if !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
		t.Fatalf("Result = %+v, want all three core checks true", r)
	}
	if r.SignerCertificate == nil || r.SignerCertificate.SerialNumber.Cmp(cert.SerialNumber) != 0 {
		t.Fatalf("SignerCertificate = %v, want serial %v", r.SignerCertificate, cert.SerialNumber)
	}
	if r.HasTimestamp {
		t.Fatal("HasTimestamp = true for a B-B signature")
	}
}

func TestVerifySignatureWithRealTimestamp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	signed, _, tsaErr := buildSignedPDF(t, true)
	if tsaErr != nil {
		t.Skipf("Pošta test TSA unreachable, skipping: %v", tsaErr)
	}

	slots, err := FindSignatures(signed)
	if err != nil || len(slots) != 1 {
		t.Fatalf("FindSignatures: %v (slots=%d)", err, len(slots))
	}
	r := VerifySignature(signed, slots[0])
	if len(r.Errors) > 0 {
		t.Fatalf("verification errors: %v", r.Errors)
	}
	if !r.HasTimestamp || !r.TimestampOK {
		t.Fatalf("Result = %+v, want a validated timestamp", r)
	}
	if r.TimestampGenTime == "" {
		t.Fatal("TimestampGenTime is empty")
	}
	t.Logf("verified real timestamp: genTime=%s serial=%s", r.TimestampGenTime, r.TimestampSerial)
}

func TestVerifySignatureTamperInsideSignedRangeIsDetected(t *testing.T) {
	signed, _, _ := buildSignedPDF(t, false)
	tampered := append([]byte{}, signed...)
	tampered[10] ^= 0xFF // well inside the document body

	slots, err := FindSignatures(tampered)
	if err != nil || len(slots) != 1 {
		t.Fatalf("FindSignatures: %v (slots=%d)", err, len(slots))
	}
	r := VerifySignature(tampered, slots[0])
	if r.ByteRangeDigestOK {
		t.Fatal("ByteRangeDigestOK = true after tampering inside the signed range")
	}
	// The CMS bytes themselves are untouched, so the signature over
	// them is still internally self-consistent — a caller must require
	// ByteRangeDigestOK together with SignatureOK, never either alone.
	if !r.SignatureOK {
		t.Fatal("SignatureOK = false, want true (tampering the document must not touch the CMS bytes' own internal validity)")
	}
}

func TestVerifySignatureTamperInsideContentsIsDetected(t *testing.T) {
	signed, _, _ := buildSignedPDF(t, false)
	slots, err := FindSignatures(signed)
	if err != nil || len(slots) != 1 {
		t.Fatalf("FindSignatures: %v (slots=%d)", err, len(slots))
	}
	b, c := slots[0].ByteRange[1], slots[0].ByteRange[2]
	// Flip a hex digit a few characters before the end of the genuine
	// (non-padding) CMS content — inside the actual signature bytes,
	// not the zero padding that fills the rest of the reservation.
	hexContentLen := int64(2 * len(slots[0].CMS))
	flipAt := b + 1 + hexContentLen - 4
	tampered := append([]byte{}, signed...)
	if flipAt >= c-1 {
		t.Fatalf("computed flip offset %d is outside the Contents span [%d,%d)", flipAt, b, c)
	}
	if tampered[flipAt] == 'F' {
		tampered[flipAt] = '0'
	} else {
		tampered[flipAt] = 'F'
	}

	slots2, err := FindSignatures(tampered)
	if err != nil || len(slots2) != 1 {
		t.Fatalf("FindSignatures after tamper: %v (slots=%d)", err, len(slots2))
	}
	r := VerifySignature(tampered, slots2[0])
	if r.SignatureOK && r.ByteRangeDigestOK {
		t.Fatal("verification accepted a document tampered inside /Contents")
	}
}

func TestVerifySignatureSecondRevisionSurvives(t *testing.T) {
	// The already-signed-document property, at this package's own
	// level: appending an unrelated incremental revision after signing
	// must not disturb the earlier signature's verifiability.
	signed, _, _ := buildSignedPDF(t, false)
	doc, err := pdf.Parse(signed)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	u := pdf.NewUpdate(doc)
	marker := u.NewObjectNumber()
	u.Set(marker, pdf.Dict{pdf.Name("Type"): pdf.Name("Marker")})
	out, err := u.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	slots, err := FindSignatures(out)
	if err != nil || len(slots) != 1 {
		t.Fatalf("FindSignatures: %v (slots=%d)", err, len(slots))
	}
	r := VerifySignature(out, slots[0])
	if len(r.Errors) > 0 || !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
		t.Fatalf("Result = %+v, errors=%v — the earlier signature must still verify after an unrelated update", r, r.Errors)
	}
}
