package tsa

import (
	"crypto/rand"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
)

// oidSHA256 is the same digest OID used everywhere else in this project
// (SPEC §12.4).
var oidSHA256 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}

// messageImprint mirrors RFC 3161's MessageImprint. We always request
// with SHA-256 (F3 §6.1: "SHA-256 or better, never SHA-1").
type messageImprint struct {
	HashAlgorithm pkix.AlgorithmIdentifier
	HashedMessage []byte
}

// timeStampReq mirrors RFC 3161's TimeStampReq. This project's own
// request is always well-formed DER — encoding/asn1 is used directly
// here, unlike the BER-tolerant response parsing above, because we
// control every byte of what we send.
type timeStampReq struct {
	Version        int
	MessageImprint messageImprint
	ReqPolicy      asn1.ObjectIdentifier `asn1:"optional"`
	Nonce          *big.Int              `asn1:"optional"`
	CertReq        bool                  `asn1:"optional,default:false"`
}

// nonceSize is F3 §6.1's explicit number: 8 random bytes.
const nonceSize = 8

// buildRequest builds a DER-encoded TimeStampReq over digest (SHA-256),
// with certReq true (so the token carries the TSA certificate, F3 §6.1)
// and a random 8-byte nonce that the caller must verify comes back
// unchanged.
func buildRequest(digest []byte) (der []byte, nonce *big.Int, err error) {
	if len(digest) != 32 {
		return nil, nil, fmt.Errorf("tsa: digest is %d bytes, want 32 (SHA-256)", len(digest))
	}
	nonceBytes := make([]byte, nonceSize)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, nil, fmt.Errorf("tsa: generating nonce: %w", err)
	}
	// A DER INTEGER is signed; force a positive value by treating the
	// random bytes as an unsigned magnitude, exactly what big.Int's
	// SetBytes already does — encoding/asn1 then adds a leading 0x00
	// byte itself if the high bit is set, keeping the value positive.
	nonce = new(big.Int).SetBytes(nonceBytes)

	req := timeStampReq{
		Version: 1,
		MessageImprint: messageImprint{
			HashAlgorithm: pkix.AlgorithmIdentifier{
				Algorithm:  oidSHA256,
				Parameters: asn1.RawValue{FullBytes: []byte{0x05, 0x00}},
			},
			HashedMessage: digest,
		},
		Nonce:   nonce,
		CertReq: true,
	}
	der, err = asn1.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("tsa: marshalling TimeStampReq: %w", err)
	}
	return der, nonce, nil
}
