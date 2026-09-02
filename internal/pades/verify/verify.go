package verify

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"math/big"
)

// Object identifiers this package needs to recognise, defined
// independently of internal/pades/cms's identically-valued but separate
// constants (F3 §8: no shared code with the signer).
var (
	oidSignedData              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidMessageDigest           = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningCertificateV2    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}
	oidSignatureTimeStampToken = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}
)

// Result is everything this package could establish about one
// signature. Each check is reported independently rather than folded
// into a single pass/fail: a test-key signature that is otherwise
// perfectly valid will never chain to the real Trusted List, and that
// is a fact about the certificate, not about whether the cryptography
// is correct.
type Result struct {
	ByteRange [4]int64

	// ByteRangeDigestOK: recomputed SHA-256 over [0,b)+[c,c+d), read
	// from the file itself, matches the CMS messageDigest attribute.
	ByteRangeDigestOK bool

	// SignatureOK: the RSA signature verifies over the signed
	// attributes, re-tagged from implicit [0] to a genuine SET OF
	// (RFC 5652 §5.4) before hashing.
	SignatureOK bool

	// SigningCertificateOK: the signingCertificateV2 attribute's
	// certHash matches SHA-256 of the signer certificate actually used.
	SigningCertificateOK bool

	SignerCertificate *x509.Certificate

	// SignerChainTrusted: the signer certificate's issuer matches a
	// granted CA/QC service in the Trusted List supplied to
	// CheckChainTrust (F3 §8 point 6). False, not merely unset, when
	// CheckChainTrust was never called — a test-key signature (every
	// signature this project's own CI produces) will always be false
	// here, which is correct: it is not on the real Trusted List.
	SignerChainTrusted bool

	// HasTimestamp is false for a B-B signature (no signatureTimeStampToken).
	HasTimestamp bool
	// TimestampOK: the timestamp token parses (BER tolerated), and its
	// messageImprint matches SHA-256 of the RSA signature it covers.
	TimestampOK          bool
	TimestampGenTime     string // RFC 3339, empty if HasTimestamp is false
	TimestampSerial      *big.Int
	TimestampCertificate *x509.Certificate
	// TimestampChainTrusted mirrors SignerChainTrusted, for the TSA's
	// own certificate against a granted TSA/QTST service.
	TimestampChainTrusted bool

	Errors []string
}

func (r *Result) fail(format string, args ...any) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
}

// VerifySignature independently verifies one signature slot found by
// FindSignatures against pdfBytes (F3 §8, SPEC §16.4).
func VerifySignature(pdfBytes []byte, slot SignatureSlot) *Result {
	r := &Result{ByteRange: slot.ByteRange}

	b, c, d := slot.ByteRange[1], slot.ByteRange[2], slot.ByteRange[3]
	digest := sha256.New()
	digest.Write(pdfBytes[:b])
	digest.Write(pdfBytes[c : c+d])
	byteRangeDigest := digest.Sum(nil)

	ci, err := parseCMS(slot.CMS)
	if err != nil {
		r.fail("parsing CMS: %v", err)
		return r
	}

	if !bytes.Equal(ci.messageDigest, byteRangeDigest) {
		r.fail("messageDigest attribute does not match SHA-256 of the /ByteRange span")
	} else {
		r.ByteRangeDigestOK = true
	}

	signerCert, err := findCertificate(ci.certificates, ci.issuer, ci.serialNumber)
	if err != nil {
		r.fail("locating signer certificate: %v", err)
		return r
	}
	r.SignerCertificate = signerCert

	// RFC 5652 §5.4: the signature covers the DER encoding of the
	// complete SET OF SignedAttributes, not the implicit [0] form the
	// attributes carry inside SignerInfo — only the leading tag byte
	// differs (0xA0 -> 0x31); length and content are identical. Skipping
	// this re-tagging is the exact mistake F3 §5.2 names, which is why
	// this file re-derives it from the RFC on its own rather than
	// calling into internal/pades/cms.
	retagged := append([]byte{}, ci.signedAttrsRaw...)
	if len(retagged) == 0 || retagged[0] != 0xA0 {
		r.fail("signedAttrs is not tagged as implicit [0]")
		return r
	}
	retagged[0] = 0x31
	signedAttrsDigest := sha256.Sum256(retagged)

	pub, ok := signerCert.PublicKey.(*rsa.PublicKey)
	if !ok {
		r.fail("signer certificate public key is %T, not RSA", signerCert.PublicKey)
		return r
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, signedAttrsDigest[:], ci.signature); err != nil {
		r.fail("RSA signature does not verify over the re-tagged SET OF signed attributes: %v", err)
	} else {
		r.SignatureOK = true
	}

	certHash := sha256.Sum256(signerCert.Raw)
	if !bytes.Equal(ci.signingCertHash, certHash[:]) {
		r.fail("signingCertificateV2 certHash does not match the signer certificate actually used")
	} else {
		r.SigningCertificateOK = true
	}

	if len(ci.timestampTokenDER) > 0 {
		r.HasTimestamp = true
		verifyTimestamp(r, ci.timestampTokenDER, ci.signature)
	}

	return r
}
