package tsl

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestVerifyBundledSeedSucceeds is the core proof required by F1 §4.9:
// signature verification against the bundled list must succeed. This is
// checked against the real, government-published document (see
// seed/README.md), not a synthetic fixture.
func TestVerifyBundledSeedSucceeds(t *testing.T) {
	if err := Verify(seedXML); err != nil {
		t.Fatalf("Verify(seed) = %v, want nil", err)
	}
}

// TestVerifyFailsOnOneByteModification is the tamper-evidence proof
// required by F1 §4.9. Flipping a byte inside the document content (not
// inside the signature block, which would just corrupt the signature
// bytes themselves in a different way) must still be caught, because the
// document no longer matches the signed digest.
func TestVerifyFailsOnOneByteModification(t *testing.T) {
	tampered := make([]byte, len(seedXML))
	copy(tampered, seedXML)

	// Flip one byte inside <TSLSequenceNumber>36</TSLSequenceNumber>,
	// well inside the signed content and far from the signature block.
	needle := []byte("<TSLSequenceNumber>36</TSLSequenceNumber>")
	idx := bytes.Index(tampered, needle)
	if idx < 0 {
		t.Fatal("test fixture assumption broken: TSLSequenceNumber not found in seed")
	}
	tampered[idx+20] ^= 0x01 // one of the digits of "36"

	err := Verify(tampered)
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Verify(tampered) = %v, want ErrSignatureInvalid", err)
	}
}

// TestVerifyRejectsUnpinnedSigner proves the pinning check actually
// fires: swap the embedded KeyInfo certificate's bytes for a
// self-signed certificate that is not one of the two Ministry
// fingerprints, re-sign nothing (the RSA signature will now also fail,
// but the pinning check must reject it on its own terms first, which
// this test observes indirectly via ErrSignatureInvalid).
func TestVerifyRejectsUnpinnedSigner(t *testing.T) {
	// A different, syntactically valid base64 DER blob (an arbitrary
	// small self-signed certificate) swapped in for the real signer
	// certificate's X509Certificate content. It cannot be a pinned
	// fingerprint by construction.
	other := unpinnedTestCertBase64
	doc := string(seedXML)
	marker := "<ds:X509Certificate>"
	start := strings.Index(doc, marker)
	if start < 0 {
		t.Fatal("test fixture assumption broken: no ds:X509Certificate in seed")
	}
	start += len(marker)
	end := strings.Index(doc[start:], "</ds:X509Certificate>")
	if end < 0 {
		t.Fatal("test fixture assumption broken: unterminated ds:X509Certificate")
	}
	end += start

	swapped := doc[:start] + other + doc[end:]

	err := Verify([]byte(swapped))
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Verify(unpinned signer) = %v, want ErrSignatureInvalid", err)
	}
	if !strings.Contains(err.Error(), "not a pinned") {
		t.Fatalf("Verify(unpinned signer) = %v, want the pinning check to be what fired", err)
	}
}

// unpinnedTestCertBase64 is a throwaway self-signed RSA certificate
// generated for this test only. It is not derived from, and does not
// resemble, any real certificate — it exists purely to have a
// syntactically valid X509Certificate value whose SHA-256 fingerprint is
// certain not to be in PinnedSigners.
const unpinnedTestCertBase64 = "MIICtTCCAZ2gAwIBAgIBATANBgkqhkiG9w0BAQsFADAeMRwwGgYDVQQDExNOb3QgQSBQaW5uZWQgU2lnbmVyMB4XDTI2MDgzMTEzMTQxMloXDTM2MDgzMTEzMTQxMlowHjEcMBoGA1UEAxMTTm90IEEgUGlubmVkIFNpZ25lcjCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAOL/x4LRPL5kNA9KkSydkkh+0gYFjVNvYPh64ZuW1ZDH7e2UrrMYkR+Zo0ij8Cf1Gzt7kXtVn6nLSP4BIs+Uuw/oDtgWpK2jyzZZ78WZ6t0LAWXWnPCVc7Def5s4V0zSziJRbfMQ8phJWe+VX1VePSYufuVp6xi3fmXX20jKbUgVIIhM0qmv8STh2HK3KxfNmRQPdbKeZRM2UB2ZXOhAzENSiSfcotG+YzWefOzplfEyNhsqCwaWxkjKZ6ztWdA4tGfJHbSrNs5ituu1H9KI8SEE4QqG3ggXoCVRhBnniXCtbNYvDMBX6jyYLkhfmrXPerfvUKKhnM+4uroHVYMHTnsCAwEAATANBgkqhkiG9w0BAQsFAAOCAQEA0RY6sQZ9IS3QEv4Eib3CehZQ6IrBLQy0fr1W3TdElRvR96/zklu8fDTkJZoUVI4rKy4Znripew4TQVFFIzEVNE8Xq0xcmaXkAWuWd3pyd+RNYRyH5oQiBldcH+BmwZ4gHeZNdzfa6uvOkVZKNko9OX249e4SFDfygoAfirl0ZRfdLRy9U8u7U4muAcG+z3Wg6vDptlL6ZeEOl71hFg8Pdb638i4JyaNIqN1RI7BgFEAqLcWCM4eekzRyk2JbiyJKrXp2lzdUaQfD72aU7KgrAvTOR5v79a1w+LO47J+lFMZVRQOfu108v1E3ZdzDJyab6dNIgFu22/+HSk0EtA5gkw=="

func TestVerifyRejectsGarbage(t *testing.T) {
	err := Verify([]byte("<not-a-trusted-list/>"))
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Verify(garbage) = %v, want ErrSignatureInvalid", err)
	}
}
