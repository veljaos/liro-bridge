package cms

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

// This test file is intentionally self-contained: it parses CMS bytes
// with a small hand-rolled TLV walker (below), not with the DER helpers
// in der.go that built them, and verifies the RSA signature with the
// standard library's crypto/rsa — never with anything from this
// package. Reusing this package's own encoder to check its own output
// would prove nothing; F3 §8 makes the same point about the real
// independent verifier this project ships (internal/pades/verify, not
// yet built), and this test earns the same property on a smaller scale.

// readTLV reads one DER tag-length-value from data and returns its
// content, and the remaining bytes after it.
func readTLV(t *testing.T, data []byte) (tag byte, content, rest []byte) {
	t.Helper()
	if len(data) < 2 {
		t.Fatalf("readTLV: too short: % X", data)
	}
	tag = data[0]
	n := int(data[1])
	off := 2
	if n&0x80 != 0 {
		numBytes := n &^ 0x80
		if numBytes == 0 || len(data) < 2+numBytes {
			t.Fatalf("readTLV: malformed long-form length")
		}
		n = 0
		for i := 0; i < numBytes; i++ {
			n = n<<8 | int(data[2+i])
		}
		off = 2 + numBytes
	}
	if len(data) < off+n {
		t.Fatalf("readTLV: content shorter than declared length (tag %#x)", tag)
	}
	return tag, data[off : off+n], data[off+n:]
}

// splitTLVs reads every top-level TLV in data, in order, until data is
// exhausted.
func splitTLVs(t *testing.T, data []byte) [][]byte {
	t.Helper()
	var children [][]byte
	for len(data) > 0 {
		_, _, rest := readTLV(t, data)
		full := data[:len(data)-len(rest)]
		children = append(children, full)
		data = rest
	}
	return children
}

// generateTestCert returns a throwaway self-signed RSA certificate and
// key, used only to build and verify CMS structures in this package's
// own tests.
func generateTestCert(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(12345),
		Subject:      pkix.Name{CommonName: "Test Signer"},
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

// buildTestCMS runs the full Builder flow with a real RSA signature,
// optionally attaching a fake timestamp-token unsigned attribute.
func buildTestCMS(t *testing.T, withTimestamp bool) (der []byte, cert *x509.Certificate, messageDigest []byte) {
	t.Helper()
	cert, key := generateTestCert(t)
	messageDigest = sha256.New().Sum([]byte("fake byte-range digest"))

	b := NewBuilder(cert, nil, messageDigest)
	digest := b.SignedAttrsDigest()
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15: %v", err)
	}
	b.SetSignature(sig)
	if withTimestamp {
		b.AddUnsignedAttribute(oidSignatureTimeStampToken, derOctetString([]byte("fake TSA token")))
	}
	out, err := b.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	return out, cert, messageDigest
}

// parsedSignerInfo holds the pieces this test needs out of the first
// (only) SignerInfo, read with the local TLV walker.
type parsedSignerInfo struct {
	signedAttrsFull []byte // includes the 0xA0 tag+length
	signature       []byte
	unsignedAttrs   [][]byte // Attribute SEQUENCEs, if any
}

func parseCMS(t *testing.T, der []byte) parsedSignerInfo {
	t.Helper()
	tag, ciContent, rest := readTLV(t, der)
	if tag != 0x30 || len(rest) != 0 {
		t.Fatalf("top level is not a single SEQUENCE (ContentInfo)")
	}
	ciParts := splitTLVs(t, ciContent)
	if len(ciParts) != 2 {
		t.Fatalf("ContentInfo has %d children, want 2", len(ciParts))
	}
	explicitTag, explicitContent, _ := readTLV(t, ciParts[1])
	if explicitTag != 0xA0 {
		t.Fatalf("ContentInfo.content tag = %#x, want explicit [0] (0xA0)", explicitTag)
	}
	sdTag, sdContent, _ := readTLV(t, explicitContent)
	if sdTag != 0x30 {
		t.Fatalf("SignedData is not a SEQUENCE")
	}
	sdParts := splitTLVs(t, sdContent)
	if len(sdParts) != 5 {
		t.Fatalf("SignedData has %d children, want 5 (version, digestAlgorithms, encapContentInfo, certificates, signerInfos)", len(sdParts))
	}
	siSetTag, siSetContent, _ := readTLV(t, sdParts[4])
	if siSetTag != 0x31 {
		t.Fatalf("signerInfos tag = %#x, want SET OF (0x31)", siSetTag)
	}
	siList := splitTLVs(t, siSetContent)
	if len(siList) != 1 {
		t.Fatalf("signerInfos has %d entries, want 1", len(siList))
	}
	siTag, siContent, _ := readTLV(t, siList[0])
	if siTag != 0x30 {
		t.Fatalf("SignerInfo is not a SEQUENCE")
	}
	siParts := splitTLVs(t, siContent)
	if len(siParts) < 6 {
		t.Fatalf("SignerInfo has %d children, want at least 6", len(siParts))
	}
	// version, sid, digestAlgorithm, signedAttrs, signatureAlgorithm, signature, [unsignedAttrs]
	sigTag, sigContent, _ := readTLV(t, siParts[5])
	if sigTag != 0x04 {
		t.Fatalf("signature field tag = %#x, want OCTET STRING (0x04)", sigTag)
	}
	result := parsedSignerInfo{signedAttrsFull: siParts[3], signature: sigContent}
	if len(siParts) > 6 {
		uaTag, uaContent, _ := readTLV(t, siParts[6])
		if uaTag != 0xA1 {
			t.Fatalf("unsignedAttrs tag = %#x, want implicit [1] (0xA1)", uaTag)
		}
		result.unsignedAttrs = splitTLVs(t, uaContent)
	}
	return result
}

func TestCMSSignatureVerifiesWithStandardLibrary(t *testing.T) {
	der, cert, _ := buildTestCMS(t, false)
	parsed := parseCMS(t, der)

	retagged := append([]byte{}, parsed.signedAttrsFull...)
	if retagged[0] != 0xA0 {
		t.Fatalf("signedAttrs stored tag = %#x, want implicit [0] (0xA0)", retagged[0])
	}
	retagged[0] = 0x31
	digest := sha256.Sum256(retagged)

	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("cert.PublicKey = %T, want *rsa.PublicKey", cert.PublicKey)
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], parsed.signature); err != nil {
		t.Fatalf("rsa.VerifyPKCS1v15 (retagged SET OF): %v", err)
	}
}

// TestCMSVerificationFailsWithoutRetagging is the direct demonstration
// of why F3 §5.2's re-tagging step matters: hashing the signedAttrs
// bytes exactly as they sit in SignerInfo (implicit [0], tag 0xA0)
// instead of re-tagged as SET OF (0x31) must NOT verify.
func TestCMSVerificationFailsWithoutRetagging(t *testing.T) {
	der, cert, _ := buildTestCMS(t, false)
	parsed := parseCMS(t, der)

	digest := sha256.Sum256(parsed.signedAttrsFull) // no re-tagging
	pub := cert.PublicKey.(*rsa.PublicKey)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], parsed.signature); err == nil {
		t.Fatal("signature verified over the un-retagged implicit [0] bytes; it must only verify over the re-tagged SET OF form")
	}
}

func TestCMSNoSigningTimeAttribute(t *testing.T) {
	der, _, _ := buildTestCMS(t, false)
	parsed := parseCMS(t, der)

	attrsTag, attrsContent, _ := readTLV(t, parsed.signedAttrsFull)
	if attrsTag != 0xA0 {
		t.Fatalf("signedAttrs tag = %#x", attrsTag)
	}
	attrs := splitTLVs(t, attrsContent)
	if len(attrs) != 3 {
		t.Fatalf("signedAttrs has %d attributes, want exactly 3 (contentType, messageDigest, signingCertificateV2)", len(attrs))
	}
	oidSigningTime := asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	seen := map[string]bool{}
	for _, a := range attrs {
		_, content, _ := readTLV(t, a)
		parts := splitTLVs(t, content)
		var oid asn1.ObjectIdentifier
		if _, err := asn1.Unmarshal(parts[0], &oid); err != nil {
			t.Fatalf("parsing attribute OID: %v", err)
		}
		if oid.Equal(oidSigningTime) {
			t.Fatal("found a signingTime attribute; F3 §5.2 forbids it outright")
		}
		seen[oid.String()] = true
	}
	for _, want := range []asn1.ObjectIdentifier{oidContentType, oidMessageDigest, oidSigningCertificateV2} {
		if !seen[want.String()] {
			t.Fatalf("missing expected signed attribute %v", want)
		}
	}
}

func TestCMSMessageDigestAttributeMatchesInput(t *testing.T) {
	der, _, messageDigest := buildTestCMS(t, false)
	parsed := parseCMS(t, der)
	_, attrsContent, _ := readTLV(t, parsed.signedAttrsFull)
	for _, a := range splitTLVs(t, attrsContent) {
		_, content, _ := readTLV(t, a)
		parts := splitTLVs(t, content)
		var oid asn1.ObjectIdentifier
		if _, err := asn1.Unmarshal(parts[0], &oid); err != nil {
			t.Fatalf("parsing attribute OID: %v", err)
		}
		if !oid.Equal(oidMessageDigest) {
			continue
		}
		setTag, setContent, _ := readTLV(t, parts[1])
		if setTag != 0x31 {
			t.Fatalf("attrValues tag = %#x, want SET OF", setTag)
		}
		octetTag, octetContent, _ := readTLV(t, setContent)
		if octetTag != 0x04 {
			t.Fatalf("messageDigest value tag = %#x, want OCTET STRING", octetTag)
		}
		if !bytes.Equal(octetContent, messageDigest) {
			t.Fatalf("messageDigest attribute = % X, want % X", octetContent, messageDigest)
		}
		return
	}
	t.Fatal("messageDigest attribute not found")
}

func TestCMSUnsignedAttrsAbsentWithoutTimestamp(t *testing.T) {
	der, _, _ := buildTestCMS(t, false)
	parsed := parseCMS(t, der)
	if len(parsed.unsignedAttrs) != 0 {
		t.Fatalf("unsignedAttrs present with no timestamp attached: %d entries", len(parsed.unsignedAttrs))
	}
}

func TestCMSUnsignedAttrsPresentWithTimestamp(t *testing.T) {
	der, _, _ := buildTestCMS(t, true)
	parsed := parseCMS(t, der)
	if len(parsed.unsignedAttrs) != 1 {
		t.Fatalf("unsignedAttrs has %d entries, want 1", len(parsed.unsignedAttrs))
	}
	_, content, _ := readTLV(t, parsed.unsignedAttrs[0])
	parts := splitTLVs(t, content)
	var oid asn1.ObjectIdentifier
	if _, err := asn1.Unmarshal(parts[0], &oid); err != nil {
		t.Fatalf("parsing attribute OID: %v", err)
	}
	if !oid.Equal(oidSignatureTimeStampToken) {
		t.Fatalf("unsigned attribute OID = %v, want signatureTimeStampToken", oid)
	}
}

// TestCMSCertificatesFieldContainsSignerCert proves the signer
// certificate this project embeds is byte-identical to the one the
// signature was computed against — not a coincidence of construction,
// but checked directly.
func TestCMSCertificatesFieldContainsSignerCert(t *testing.T) {
	der, cert, _ := buildTestCMS(t, false)
	_, ciContent, _ := readTLV(t, der)
	ciParts := splitTLVs(t, ciContent)
	_, explicitContent, _ := readTLV(t, ciParts[1])
	_, sdContent, _ := readTLV(t, explicitContent)
	sdParts := splitTLVs(t, sdContent)
	certsTag, certsContent, _ := readTLV(t, sdParts[3])
	if certsTag != 0xA0 {
		t.Fatalf("certificates tag = %#x, want implicit [0] (0xA0)", certsTag)
	}
	certs := splitTLVs(t, certsContent)
	if len(certs) != 1 {
		t.Fatalf("certificates has %d entries, want 1 (no chain supplied)", len(certs))
	}
	if !bytes.Equal(certs[0], cert.Raw) {
		t.Fatal("embedded certificate does not match the signer certificate byte-for-byte")
	}
}
