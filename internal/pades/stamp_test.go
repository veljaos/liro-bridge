package pades

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
	"regexp"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades/appearance"
)

// TestStampDoesNotAffectNoStampGoldenOutput is F4 §6/§8's central
// separation test, spelled out exactly as required: "a test that signs
// the same document with and without the stamp flag, using a fixed key
// and timestamp, and asserts the no-stamp output still matches the F3
// golden file byte for byte." If Options.Stamp being nil ever stops
// being a complete no-op for every line SignDocument executes, this is
// the test that catches it.
func TestStampDoesNotAffectNoStampGoldenOutput(t *testing.T) {
	fixedDate := time.Date(2026, 3, 24, 15, 55, 52, 0, time.FixedZone("CET", 3600))
	doc := goldenMinimalPDF()

	sessNoStamp := newDeterministicSession(t)
	noStamp, err := SignDocument(context.Background(), doc, sessNoStamp, Options{
		ReservedBytes: 4096,
		Now:           fixedDate,
	})
	if err != nil {
		t.Fatalf("SignDocument (no stamp): %v", err)
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden file: %v", err)
	}
	if !bytes.Equal(noStamp.Bytes, want) {
		t.Fatalf("no-stamp output does not match the F3 golden file byte for byte (got %d bytes, want %d)", len(noStamp.Bytes), len(want))
	}

	// The companion half of the same test: signing *with* a stamp, same
	// key and date, must actually produce something — proving the two
	// paths are genuinely different pieces of code, not that the stamp
	// flag was silently ignored both times.
	sessStamp := newDeterministicSession(t)
	withStamp, err := SignDocument(context.Background(), doc, sessStamp, Options{
		ReservedBytes: 8192,
		Now:           fixedDate,
		Stamp: &StampOptions{
			Label:  "Digitally signed by",
			Corner: appearance.BottomRight,
		},
	})
	if err != nil {
		t.Fatalf("SignDocument (with stamp): %v", err)
	}
	if bytes.Equal(withStamp.Bytes, want) {
		t.Fatal("with-stamp output is byte-identical to the no-stamp golden file — the stamp was not actually applied")
	}
}

// TestSignDocumentWithStampStillVerifies proves a stamped signature is
// still a real, independently verifiable PAdES signature (F4 §8: "each
// output parses and both signatures verify" — applied here at the
// synthetic-fixture level; TestRealFixturesStampAndVerify below does
// the same against all three real documents).
func TestSignDocumentWithStampStillVerifies(t *testing.T) {
	sess := newFakeSession(t)
	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		Stamp: &StampOptions{
			Label:     "Digitally signed by",
			Reference: "INV-2026-0042",
			Corner:    appearance.BottomLeft,
		},
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
		t.Fatalf("independent verification failed for a stamped signature: %+v errors=%v", r, r.Errors)
	}
}

// pnorsSession is a fakeSession whose certificate carries a JMBG-shaped
// PNORS value in the same multi-valued RDN as an IDCRS value — the
// worst case for a national-identity-number leak (SPEC §11.6 Trap 1).
type pnorsSession struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func newPNORSSession(t *testing.T) *pnorsSession {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(0x1234567890),
		Subject: pkix.Name{
			CommonName: "ВЕЉКО СТАНОЈЕВИЋ 011445479 Sign",
			ExtraNames: []pkix.AttributeTypeAndValue{
				{Type: oidGivenNameStamp, Value: "ВЕЉКО"},
				{Type: oidSurnameStamp, Value: "СТАНОЈЕВИЋ"},
				{Type: oidSerialNumberStamp, Value: "PNORS-0114454791234"},
				{Type: oidSerialNumberStamp, Value: "IDCRS-998877"},
			},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &pnorsSession{cert: cert, key: key}
}

func (s *pnorsSession) SignDigest(_ context.Context, _ keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	return rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest)
}
func (s *pnorsSession) Certificate() keysource.Certificate {
	return keysource.Certificate{Thumbprint: "PNORS-TEST", DER: s.cert.Raw}
}
func (s *pnorsSession) Chain() [][]byte { return nil }
func (s *pnorsSession) Close() error    { return nil }

var thirteenConsecutiveDigits = regexp.MustCompile(`\d{13}`)

// TestBuildAppearanceOptionsNeverContainsThirteenConsecutiveDigits is
// the other security-relevant test this project's engagement rules
// require explicitly: no stamp output may contain 13 consecutive digits
// (a JMBG's shape, SPEC §3's glossary). This certificate is deliberately
// the worst case — a real 13-digit PNORS value sitting in the same
// multi-valued RDN as the IDCRS value --stamp-show-document-id is
// allowed to display — with every optional stamp line turned on.
//
// This checks the plain-text strings actually fed to
// internal/pades/appearance.Render — what a human or an accessibility
// tool reading the stamp would see (confirmed lossless by
// internal/pades/appearance's own TestToUnicodeCoversEveryGID and
// TestEncodeCIDs* tests) — rather than scanning the finished PDF's raw
// bytes. Scanning raw bytes was tried first and produces false
// failures unrelated to personal-data leakage: Identity-H text is
// stored as 2-byte glyph indices written in hexadecimal (F4 §3.3), and
// small glyph indices routinely hex-format using only the digit
// characters 0-9 (e.g. CID 84 is the four hex characters "0054"),
// so consecutive glyphs in ordinary stamp text can produce a run of 13
// digit *characters* with no connection to any 13-digit *number*
// appearing anywhere. Checking the source strings before CID encoding
// is both the more precise check and the one that actually matches
// what "no stamp output may contain 13 consecutive digits" is
// protecting against.
func TestBuildAppearanceOptionsNeverContainsThirteenConsecutiveDigits(t *testing.T) {
	sess := newPNORSSession(t)
	opts := buildAppearanceOptions(sess.cert, time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), &StampOptions{
		Label:          "Digitally signed by",
		Reference:      "INV-2026-0042-AB99887766",
		ShowDocumentID: true,
	})

	fields := map[string]string{
		"Label":       opts.Label,
		"SignerName":  opts.SignerName,
		"Reference":   opts.Reference,
		"DocumentID":  opts.DocumentID,
		"SerialHex":   opts.SerialHex,
		"SigningTime": opts.SigningTime,
	}
	for name, value := range fields {
		if loc := thirteenConsecutiveDigits.FindStringIndex(value); loc != nil {
			t.Errorf("appearance.Options.%s = %q contains 13 consecutive digits at offset %d", name, value, loc[0])
		}
	}
	// documentIDFromCertificate must have returned the IDCRS value, not
	// the PNORS one — otherwise this test would trivially pass by
	// omission rather than by the JMBG genuinely being absent.
	if opts.DocumentID != "998877" {
		t.Fatalf("opts.DocumentID = %q, want the IDCRS value %q — test setup did not exercise the PNORS/IDCRS trap", opts.DocumentID, "998877")
	}
}

// TestSignDocumentWithPNORSCertificateStillSignsAndVerifies is the
// end-to-end companion to the field-level check above: signing with the
// worst-case PNORS/IDCRS certificate must still succeed and produce a
// signature that verifies — the safety net is that the number never
// appears in the *stamp text*, not that signing refuses to proceed.
func TestSignDocumentWithPNORSCertificateStillSignsAndVerifies(t *testing.T) {
	sess := newPNORSSession(t)
	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		Stamp: &StampOptions{
			Label:          "Digitally signed by",
			Reference:      "INV-2026-0042-AB99887766",
			ShowDocumentID: true,
			Corner:         appearance.BottomRight,
		},
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK {
		t.Fatalf("independent verification failed: %+v errors=%v", r, r.Errors)
	}
}

// TestSignDocumentStampTargetsRequestedPage is F4 §2.1/§8: "works on any
// page, not only the first." A three-page document, stamped on page 2,
// must place the widget on page 2's object, not page 1's.
func TestSignDocumentStampTargetsRequestedPage(t *testing.T) {
	sess := newFakeSession(t)
	result, err := SignDocument(context.Background(), buildThreePagePDF(t), sess, Options{
		Stamp: &StampOptions{
			Label:  "Digitally signed by",
			Page:   2,
			Corner: appearance.BottomRight,
		},
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK {
		t.Fatalf("stamping page 2 broke signature verification: %+v", r)
	}
	// The widget's /P entry must point at page 2's object number (5, in
	// buildThreePagePDF's fixed numbering) — checked textually since
	// this package deliberately never imports internal/pades/pdf's
	// object model into its own tests beyond what sign_test.go already
	// does.
	if !bytes.Contains(result.Bytes, []byte("/P 5 0 R")) {
		t.Fatalf("signature widget does not reference page object 5 (page 2)")
	}
}

func buildThreePagePDF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := map[int]int{}
	write := func(num int, body string) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}
	write(1, "<< /Type /Catalog /Pages 2 0 R >>")
	write(2, "<< /Type /Pages /Kids [3 0 R 5 0 R 6 0 R] /Count 3 >>")
	write(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>")
	content := "BT /F1 12 Tf 72 712 Td (One) Tj ET"
	offsets[4] = buf.Len()
	fmt.Fprintf(&buf, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)
	write(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 7 0 R >>")
	write(6, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 8 0 R >>")
	content2 := "BT /F1 12 Tf 72 712 Td (Two) Tj ET"
	offsets[7] = buf.Len()
	fmt.Fprintf(&buf, "7 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content2), content2)
	content3 := "BT /F1 12 Tf 72 712 Td (Three) Tj ET"
	offsets[8] = buf.Len()
	fmt.Fprintf(&buf, "8 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content3), content3)

	xrefStart := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 9\n0000000000 65535 f \n")
	for i := 1; i <= 8; i++ {
		fmt.Fprintf(&buf, "%010d %05d n \n", offsets[i], 0)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 9 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xrefStart)
	return buf.Bytes()
}
