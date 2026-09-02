package pades

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"os"
	"path/filepath"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/pades/verify"
)

// realPDFsDir mirrors internal/pades/pdf's and internal/pades/verify's
// identically-purposed constants (real signed PDFs, never committed —
// see testdata/pdfs/local/README.md).
const realPDFsDir = "../../testdata/pdfs/local"

var realFixtureNames = []string{"halcom.pdf", "mup.pdf", "posta.pdf"}

func realFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(realPDFsDir, name))
	if os.IsNotExist(err) {
		t.Skipf("%s not found: no real PDF fixtures available for this test — see %s/README.md", name, realPDFsDir)
	}
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return data
}

// mustFullyVerify runs internal/pades/verify's independent checks over
// every signature slot data contains and returns how many are a genuine,
// fully-verifying PAdES signature (ByteRangeDigestOK, SignatureOK and
// SigningCertificateOK all true) — the count that matters for "both the
// old and the new signature verify", since a real document also carries
// a document-timestamp revision (SPEC §12.2) that never satisfies
// ByteRangeDigestOK by construction (see internal/pades/verify's own
// real-fixture tests for why).
func countFullyVerifyingSignatures(t *testing.T, data []byte) int {
	t.Helper()
	slots, err := verify.FindSignatures(data)
	if err != nil {
		t.Fatalf("verify.FindSignatures: %v", err)
	}
	n := 0
	for _, slot := range slots {
		r := verify.VerifySignature(data, slot)
		if r.ByteRangeDigestOK && r.SignatureOK && r.SigningCertificateOK {
			n++
		}
	}
	return n
}

// TestRealFixturesAlreadySignedDocumentBothSignaturesVerify is F3
// §10.2's real-fixture instance of this project's single most important
// test: sign each real, already-signed document again — with a test key
// source standing in for the soft token, exactly as this package's own
// fakeSession already does for TestSignDocumentAlreadySignedDocumentBothVerify
// — and confirm both the original CA's signature and this project's new
// one verify afterwards. Until now this has only been proven against a
// prior signature this project's own code produced (see [[D-038]]); this
// is the first time it runs against a genuine prior signature from a
// real CA, which is exactly the gap F3 §10.2 flagged as unverifiable
// without real fixtures.
func TestRealFixturesAlreadySignedDocumentBothSignaturesVerify(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			in := realFixture(t, name)

			before := countFullyVerifyingSignatures(t, in)
			if before == 0 {
				t.Fatal("the real document's own signature does not verify before this project touches it")
			}

			sess := newFakeSession(t)
			result, err := SignDocument(context.Background(), in, sess, Options{})
			if err != nil {
				t.Fatalf("SignDocument: %v", err)
			}
			if !bytes.HasPrefix(result.Bytes, in) {
				t.Fatal("signing did not append to the real document's exact original bytes")
			}

			after := countFullyVerifyingSignatures(t, result.Bytes)
			if after != before+1 {
				t.Fatalf("%d signatures fully verify after signing, want %d (the original CA's signature plus this project's new one)", after, before+1)
			}

			slots, err := verify.FindSignatures(result.Bytes)
			if err != nil {
				t.Fatalf("verify.FindSignatures: %v", err)
			}
			r := verify.VerifySignature(result.Bytes, slots[len(slots)-1])
			if !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
				t.Fatalf("the newly appended signature does not independently verify: %+v errors=%v", r, r.Errors)
			}
		})
	}
}

// realMUPGivenName and realMUPSurname are SPEC §11.7's measured, real
// MUP certificate values — used here (not the real certificate itself,
// which this environment does not have a private key for) to exercise
// the exact Cyrillic string F4 §8's "Cyrillic name from the MUP
// certificate renders correctly" requirement names, through the full
// SignDocument -> appearance.Render pipeline against a real document's
// actual page geometry, not a synthetic fixture's.
const (
	realMUPGivenName = "ВЕЉКО"
	realMUPSurname   = "СТАНОЈЕВИЋ"
)

// mupNameSession is a fakeSession-shaped Session (see sign_test.go's
// fakeSession) whose certificate's givenName/surname are SPEC §11.7's
// real MUP values, so the stamp exercises real-shaped Cyrillic input.
type mupNameSession struct{ *fakeSession }

func newMUPNameSession(t *testing.T) *mupNameSession {
	t.Helper()
	s := newFakeSession(t)
	tmpl := &x509.Certificate{
		SerialNumber: s.cert.SerialNumber,
		Subject: pkix.Name{
			CommonName: "ВЕЉКО СТАНОЈЕВИЋ 011445479 Sign",
			ExtraNames: []pkix.AttributeTypeAndValue{
				{Type: oidGivenNameStamp, Value: realMUPGivenName},
				{Type: oidSurnameStamp, Value: realMUPSurname},
			},
		},
		NotBefore: s.cert.NotBefore,
		NotAfter:  s.cert.NotAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &s.key.PublicKey, s.key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	s.cert = cert
	return &mupNameSession{s}
}

// TestRealFixturesStampAndBothSignaturesVerify is F4 §8: "Stamp renders
// on all three real fixtures; each output parses and both signatures
// verify," with the MUP case specifically exercising SPEC §11.7's real
// Cyrillic givenName/surname through the full pipeline.
func TestRealFixturesStampAndBothSignaturesVerify(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			in := realFixture(t, name)
			before := countFullyVerifyingSignatures(t, in)

			sess := newMUPNameSession(t)
			result, err := SignDocument(context.Background(), in, sess, Options{
				Stamp: &StampOptions{
					Label:          "Digitally signed by",
					Reference:      "F4 real-fixture test",
					ShowDocumentID: true,
					Corner:         appearance.BottomRight,
				},
			})
			if err != nil {
				t.Fatalf("SignDocument with stamp: %v", err)
			}
			if !bytes.HasPrefix(result.Bytes, in) {
				t.Fatal("signing did not append to the real document's exact original bytes")
			}

			after := countFullyVerifyingSignatures(t, result.Bytes)
			if after != before+1 {
				t.Fatalf("%d signatures fully verify after stamping, want %d", after, before+1)
			}

			slots, err := verify.FindSignatures(result.Bytes)
			if err != nil {
				t.Fatalf("verify.FindSignatures: %v", err)
			}
			r := verify.VerifySignature(result.Bytes, slots[len(slots)-1])
			if !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
				t.Fatalf("the newly appended, stamped signature does not independently verify: %+v errors=%v", r, r.Errors)
			}
		})
	}
}
