package pkcs11

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// TestDigestInfoMatchesTheStandardLibrary is the check the whole of
// digestinfo.go rests on, and the reason it is worth having is F11 §2.1: hand
// CKM_RSA_PKCS the wrong bytes and the signature verifies against nothing
// while every layer reports success.
//
// The trick is that crypto/rsa can be asked the same question two ways.
// SignPKCS1v15 with crypto.SHA256 builds the DigestInfo itself from the
// algorithm; SignPKCS1v15 with crypto.Hash(0) treats whatever it is given as
// an already-built DigestInfo and pads it unchanged. If this package's prefix
// is exactly what the standard library builds, the two signatures are
// byte-identical — and they can be identical for no other reason, because
// PKCS#1 v1.5 padding is deterministic and the two inputs differ only in what
// this file supplies.
//
// It shares no code with digestinfo.go, which is what makes it evidence rather
// than a restatement (D-044's rule for the CMS verifier, applied to a much
// smaller structure).
func TestDigestInfoMatchesTheStandardLibrary(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	digest := sha256.Sum256([]byte("the bytes a /ByteRange digest would be over"))

	// What Windows CNG does for us on the other backend, and what
	// CKM_RSA_PKCS will not do: build the DigestInfo from the algorithm.
	wantSig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("signing with crypto.SHA256: %v", err)
	}

	info, err := digestInfo(keysource.DigestSHA256, digest[:])
	if err != nil {
		t.Fatalf("digestInfo: %v", err)
	}

	// crypto.Hash(0) means "this is already a DigestInfo; pad it and sign it",
	// which is exactly what CKM_RSA_PKCS does with what we hand it.
	gotSig, err := rsa.SignPKCS1v15(nil, key, crypto.Hash(0), info)
	if err != nil {
		t.Fatalf("signing the DigestInfo: %v", err)
	}

	if !bytes.Equal(wantSig, gotSig) {
		t.Errorf("signing this package's DigestInfo does not produce the same signature as\n"+
			"signing the digest with crypto.SHA256. The prefix in digestinfo.go is wrong,\n"+
			"and a signature made with it would verify against nothing (F11 §2.1).\n"+
			"  DigestInfo: %x", info)
	}
}

// TestTheDigestInfoCheckWouldActuallyFire is the other half. A comparison that
// cannot fail proves nothing, and the failure mode being guarded against —
// handing CKM_RSA_PKCS a bare digest — is one byte-identical-looking mistake
// away from the correct code.
func TestTheDigestInfoCheckWouldActuallyFire(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	digest := sha256.Sum256([]byte("anything"))

	wantSig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	// The mistake this whole file exists to prevent: sign the bare digest as
	// though it were a DigestInfo.
	bareSig, err := rsa.SignPKCS1v15(nil, key, crypto.Hash(0), digest[:])
	if err != nil {
		t.Fatalf("signing the bare digest: %v", err)
	}
	if bytes.Equal(wantSig, bareSig) {
		t.Fatal("signing a bare digest produced the same signature as signing a DigestInfo, " +
			"so the check above could not tell the two apart and proves nothing")
	}
}

// TestDigestInfoParsesAsTheStructureItClaimsToBe reads the bytes back with the
// standard library's ASN.1 decoder — a third implementation, neither this
// package's nor crypto/rsa's internal table — and checks the OID and the
// digest are what went in.
//
// The signature comparison above already proves the bytes are right. This
// proves they are right *for the stated reason* rather than by coincidence,
// which is what makes the failure legible when somebody adds a second
// algorithm.
func TestDigestInfoParsesAsTheStructureItClaimsToBe(t *testing.T) {
	digest := sha256.Sum256([]byte("a document"))
	info, err := digestInfo(keysource.DigestSHA256, digest[:])
	if err != nil {
		t.Fatalf("digestInfo: %v", err)
	}

	var parsed struct {
		Algorithm struct {
			Algorithm  asn1.ObjectIdentifier
			Parameters asn1.RawValue `asn1:"optional"`
		}
		Digest []byte
	}
	rest, err := asn1.Unmarshal(info, &parsed)
	if err != nil {
		t.Fatalf("the DigestInfo does not parse as a DigestInfo: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("%d trailing bytes after the DigestInfo", len(rest))
	}
	// 2.16.840.1.101.3.4.2.1 is id-sha256 (RFC 8017 B.1).
	want := asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	if !parsed.Algorithm.Algorithm.Equal(want) {
		t.Errorf("the algorithm OID is %v, want %v", parsed.Algorithm.Algorithm, want)
	}
	if !bytes.Equal(parsed.Digest, digest[:]) {
		t.Errorf("the digest came back as %x, want %x", parsed.Digest, digest)
	}
}

// TestDigestInfoRefusesWhatItCannotSign covers the two ways a caller can hand
// this the wrong thing. Both are refused rather than encoded: a digest of the
// wrong length would produce a structure whose two declared lengths disagree
// with its content, which is the same silent wrongness one level down.
func TestDigestInfoRefusesWhatItCannotSign(t *testing.T) {
	digest := sha256.Sum256([]byte("x"))

	if _, err := digestInfo(keysource.DigestAlgorithmUnknown, digest[:]); err == nil {
		t.Error("an unknown algorithm was encoded rather than refused")
	}
	for _, n := range []int{0, 20, 31, 33, 64} {
		if _, err := digestInfo(keysource.DigestSHA256, make([]byte, n)); err == nil {
			t.Errorf("a %d-byte digest was accepted as SHA-256", n)
		}
	}
}

// TestSHA1IsNotSignable is SPEC §18.8, which is unconditional: no SHA-1
// anywhere, whatever a module offers. Both NetSeT builds and SafeSign offer
// CKM_SHA1_RSA_PKCS (D-271, D-273); this layer has no prefix for it, so
// nothing here can produce one even if a caller asks.
func TestSHA1IsNotSignable(t *testing.T) {
	for alg := range digestInfoPrefix {
		if alg != keysource.DigestSHA256 {
			t.Errorf("digestinfo.go carries a prefix for %s; SPEC §12.4 signs SHA-256 and §18.8 forbids SHA-1", alg)
		}
	}
	if len(digestInfoPrefix) != 1 {
		t.Errorf("digestinfo.go carries %d prefixes, want exactly the one", len(digestInfoPrefix))
	}
}
