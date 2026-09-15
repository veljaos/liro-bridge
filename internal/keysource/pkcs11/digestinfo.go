package pkcs11

import (
	"fmt"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// digestInfoPrefix is the DER encoding of an RFC 8017 DigestInfo with an empty
// digest, per algorithm — everything up to and including the OCTET STRING's
// tag and length, so that appending the digest completes the structure.
//
// # Why this exists at all, and why it is the trap of this phase
//
// CKM_RSA_PKCS performs EMSA-PKCS1-v1_5 padding over **the bytes it is given**
// and signs them. It does not know what a hash is and does not add a
// DigestInfo. So the caller must hand it a complete DigestInfo, and this is
// that structure:
//
//	DigestInfo ::= SEQUENCE {
//	    digestAlgorithm AlgorithmIdentifier,
//	    digest          OCTET STRING
//	}
//
// The Windows CNG backend does not need this and has no equivalent code,
// because BCRYPT_PAD_PKCS1 takes the algorithm's own identifier in
// BCRYPT_PKCS1_PADDING_INFO and builds the DigestInfo inside Windows
// (windowscng/conn_windows.go). That difference is the reason this file is
// easy to forget and expensive to get wrong: hand CKM_RSA_PKCS a bare digest
// and it signs a bare digest, every layer reports success, and the signature
// verifies against nothing (F11 §2.1).
//
// The alternative — CKM_SHA256_RSA_PKCS — is wrong for a different reason: it
// expects the *data* and hashes it itself, so handing it a digest signs a hash
// of a hash. Both mistakes look identical from here. Neither is detectable
// without an independent verifier, which is why F11 §2.1 forbids accepting
// "bytes came back" as evidence.
//
// # How these bytes are known to be right
//
// Not by transcription. `TestDigestInfoMatchesTheStandardLibrary` signs the
// same digest twice with one RSA key — once through
// `rsa.SignPKCS1v15(…, crypto.SHA256, digest)`, which builds the DigestInfo
// itself, and once through `rsa.SignPKCS1v15(…, crypto.Hash(0), digestInfo)`,
// which treats its input as an already-built DigestInfo — and requires the two
// signatures to be byte-identical. They can only be identical if this prefix
// is exactly what the standard library builds, and the check shares no code
// with this file.
var digestInfoPrefix = map[keysource.DigestAlgorithm][]byte{
	// SHA-256: SEQUENCE { SEQUENCE { OID 2.16.840.1.101.3.4.2.1, NULL },
	// OCTET STRING (32) }
	keysource.DigestSHA256: {
		0x30, 0x31,
		0x30, 0x0d,
		0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01,
		0x05, 0x00,
		0x04, 0x20,
	},
}

// digestInfo returns the DER DigestInfo that CKM_RSA_PKCS is to sign.
//
// The digest's length is checked against the algorithm's own, because the
// prefix declares that length in two places — the outer SEQUENCE's and the
// OCTET STRING's — and a digest of the wrong size would produce a structure
// that is internally inconsistent rather than one that fails to parse. That is
// the same class of silent wrongness the rest of this file is about.
func digestInfo(alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	prefix, ok := digestInfoPrefix[alg]
	if !ok {
		return nil, fmt.Errorf("pkcs11: no DigestInfo prefix for %s; this layer signs %s and nothing else",
			alg, keysource.DigestSHA256)
	}
	if want := alg.Size(); len(digest) != want {
		return nil, fmt.Errorf("pkcs11: a %s digest is %d bytes, got %d", alg, want, len(digest))
	}
	out := make([]byte, 0, len(prefix)+len(digest))
	out = append(out, prefix...)
	out = append(out, digest...)
	return out, nil
}
