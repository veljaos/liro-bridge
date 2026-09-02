package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// realPDFsDir mirrors internal/pades/pdf's identically-purposed constant
// (real signed PDFs, never committed — see testdata/pdfs/local/README.md).
// Duplicated rather than imported: this package shares no code with
// anything that produces or parses a signature (F3 §8), including test
// helpers that touch the same real files for a different purpose.
const realPDFsDir = "../../../testdata/pdfs/local"

// bundledSeedPath is the real, government-published Trusted List F1
// bundles into the binary (internal/trust/tsl's seedXML, [[D-018]]),
// read here directly rather than through any exported loader — none
// exists, and adding one only for this test would be unrequested scope.
const bundledSeedPath = "../../../internal/trust/tsl/seed/TSL-RS.xml"

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

// mainSignatureSlot runs VerifySignature over every slot FindSignatures
// found and returns the one that is a genuine PAdES signature over this
// document — ByteRangeDigestOK, SignatureOK and SigningCertificateOK all
// true. A real signed document also carries a separate document-
// timestamp revision (/SubFilter /ETSI.RFC3161, SPEC §12.2) whose own
// CMS messageDigest attribute covers its embedded TSTInfo, not this
// document's /ByteRange — VerifySignature correctly reports
// ByteRangeDigestOK=false for that slot, which is why this helper picks
// the one slot that passes all three checks rather than assuming
// position or count.
func mainSignatureSlot(t *testing.T, data []byte, slots []SignatureSlot) (SignatureSlot, *Result) {
	t.Helper()
	for _, slot := range slots {
		r := VerifySignature(data, slot)
		if r.ByteRangeDigestOK && r.SignatureOK && r.SigningCertificateOK {
			return slot, r
		}
	}
	t.Fatal("no signature slot in this document is a fully-verifying PAdES signature")
	return SignatureSlot{}, nil
}

// TestRealFixturesExtractSignatureAndClassifyQualified is F3 §2.5/§8's
// extraction requirement: from each real document, extract the signature
// dictionary's /ByteRange, the CMS blob and the signer certificate, and
// confirm the signer classifies as qualified against the bundled Trusted
// List (SPEC §11.1) — not against a synthetic seed built to already
// agree with this project's own understanding of the TSL (contrast
// [[D-021]]'s synthetic certificates, which only reuse real CA issuer
// bytes, not a real signer).
func TestRealFixturesExtractSignatureAndClassifyQualified(t *testing.T) {
	seed, err := os.ReadFile(bundledSeedPath)
	if err != nil {
		t.Fatalf("reading bundled Trusted List seed: %v", err)
	}
	list, err := tsl.Parse(seed)
	if err != nil {
		t.Fatalf("tsl.Parse(bundled seed): %v", err)
	}

	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			data := realFixture(t, name)
			slots, err := FindSignatures(data)
			if err != nil {
				t.Fatalf("FindSignatures: %v", err)
			}
			if len(slots) == 0 {
				t.Fatal("no signature dictionary found in a real signed document")
			}

			slot, r := mainSignatureSlot(t, data, slots)
			if slot.ByteRange[0] != 0 || slot.ByteRange[2] <= slot.ByteRange[1] || slot.ByteRange[3] <= 0 {
				t.Fatalf("/ByteRange = %v, does not look like [0 b c d] with b < c and d > 0", slot.ByteRange)
			}
			if len(slot.CMS) == 0 {
				t.Fatal("extracted CMS blob is empty")
			}
			if r.SignerCertificate == nil {
				t.Fatal("no signer certificate extracted from the CMS")
			}

			// SPEC §11.1: qualification is decided at the time of signing,
			// since a service granted then may be withdrawn by the time
			// this test runs. The certificate's own NotBefore is the best
			// available stand-in for "when this document was signed" —
			// this project does not otherwise learn the signing time
			// independently of the (SHA-1, per SPEC §12.4's documented
			// defect) unsigned timestamp embedded in the same CMS.
			refTime := r.SignerCertificate.NotBefore.Add(time.Hour)
			info := classify.Classify(r.SignerCertificate, list, false, false, refTime)
			if info.Qualification != classify.QualificationQualified {
				t.Errorf("Qualification = %v, want QualificationQualified for a real %s signer against the bundled Trusted List", info.Qualification, name)
			}
		})
	}
}

// TestRealFixturesIndependentVerificationAcceptsExistingSignature is F3
// §8/§10.2/SPEC §16.4's independent-verification requirement: recompute
// the /ByteRange digest and compare it to messageDigest, and verify the
// RSA signature over the re-tagged SET OF signed attributes, for each
// real document's own, pre-existing signature. All three must verify —
// this exact procedure was applied to all three during specification and
// all three verified (F3 §8), so a failure here is a bug in this
// package, not in the documents.
func TestRealFixturesIndependentVerificationAcceptsExistingSignature(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			data := realFixture(t, name)
			slots, err := FindSignatures(data)
			if err != nil {
				t.Fatalf("FindSignatures: %v", err)
			}
			// mainSignatureSlot already asserts these three; re-asserting
			// them here (rather than trusting the helper silently) is
			// this test's actual point — the F3 §8/§10.2 requirement.
			_, r := mainSignatureSlot(t, data, slots)
			if !r.ByteRangeDigestOK {
				t.Error("ByteRangeDigestOK = false, want the /ByteRange digest to match messageDigest")
			}
			if !r.SignatureOK {
				t.Error("SignatureOK = false, want the RSA signature to verify over the re-tagged SET OF signed attributes")
			}
			if !r.SigningCertificateOK {
				t.Error("SigningCertificateOK = false, want signingCertificateV2 to match the signer certificate")
			}

			// SPEC §12.4: the reference documents' own embedded signature
			// timestamp (RFC 5652 unsignedAttrs.signatureTimeStampToken,
			// distinct from the separate document-timestamp revision the
			// BER test below covers) uses a SHA-1 imprint — "a defect to
			// be aware of, not a pattern to copy." This project's
			// verifier is correct to reject it; asserting the exact
			// reason here pins that this failure is the known, documented
			// one and not some other, unexpected regression.
			if r.HasTimestamp && !r.TimestampOK {
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e, "hash algorithm other than SHA-256") {
						found = true
					}
				}
				if !found {
					t.Errorf("main signature's embedded timestamp failed for an unexpected reason: %v", r.Errors)
				}
			}
		})
	}
}

// TestRealFixturesDocumentTimestampIsBERIndefiniteLength is SPEC
// §12.5/F3 §5.5's BER-tolerance requirement, exercised against a real
// token rather than only the hand-built indefinite-length vector
// [[D-048]] already covers: each real document carries a document
// timestamp (SPEC §12.2, /SubFilter /ETSI.RFC3161) encoded as BER with
// indefinite length ("30 80 ..."). FindSignatures' own decoding
// (decodeSignedCMS -> readDER) silently drops a slot it cannot parse
// (a malformed match "resumes scanning past it" rather than failing the
// whole call), so the real assertion here is the exact slot count: if
// this package's BER reader could not handle indefinite length, the
// timestamp slot would simply be missing rather than producing a loud
// failure, which is exactly the class of silent bug F3 §4.1 warns about
// for /ByteRange-adjacent code. A DER-only parser fails on this input,
// which is why SPEC §12.5 requires BER tolerance in the first place.
func TestRealFixturesDocumentTimestampIsBERIndefiniteLength(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			data := realFixture(t, name)
			slots, err := FindSignatures(data)
			if err != nil {
				t.Fatalf("FindSignatures: %v", err)
			}
			if len(slots) != 2 {
				t.Fatalf("found %d signature slots, want 2 (the PAdES signature and the document timestamp) — a slot may have been silently dropped by a BER decoding failure", len(slots))
			}

			var indefiniteLength int
			for _, slot := range slots {
				if len(slot.CMS) >= 2 && slot.CMS[0] == 0x30 && slot.CMS[1] == 0x80 {
					indefiniteLength++
				}
			}
			if indefiniteLength == 0 {
				t.Fatal("no slot's CMS begins with the BER indefinite-length SEQUENCE marker (30 80) — expected the document timestamp to be encoded this way")
			}
		})
	}
}
