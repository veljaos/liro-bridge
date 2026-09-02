// Package cms builds RFC 5652 SignedData structures by hand (F3 §5,
// SPEC §12.1): standard-library encoding/asn1 is used for leaf values
// (OIDs, INTEGERs) where it fits, but the SignedData/SignerInfo
// structure itself — SEQUENCE, SET OF, and the [0]/[1] context tags —
// is assembled directly as DER bytes. That is deliberate, not a
// shortcut: RFC 5652 §5.4 requires the signature to be computed over
// the signed attributes re-tagged from their implicit [0] form to a
// genuine SET OF (tag 0x31) before hashing, which only makes sense to
// implement by owning the exact bytes end to end.
package cms

import (
	"bytes"
	"encoding/asn1"
	"math/big"
	"sort"
)

// derLength encodes a DER length in short or long form.
func derLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte(n)}, b...)
		n >>= 8
	}
	return append([]byte{0x80 | byte(len(b))}, b...)
}

// derTLV wraps content in a tag-length-value header.
func derTLV(tag byte, content []byte) []byte {
	out := make([]byte, 0, 2+len(content))
	out = append(out, tag)
	out = append(out, derLength(len(content))...)
	out = append(out, content...)
	return out
}

func concatBytes(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// derSequence builds a SEQUENCE (tag 0x30) from already-encoded TLV
// children, in the given order (SEQUENCE order is significant, unlike
// SET OF).
func derSequence(children ...[]byte) []byte {
	return derTLV(0x30, concatBytes(children...))
}

// derSetOf builds a SET OF (tag 0x31) from already-encoded TLV
// children, sorted into DER canonical order (X.690 §11.6: ascending by
// encoded octets) — required for a signature over it to be
// reproducible, and for strict validators to accept it.
func derSetOf(children [][]byte) []byte {
	sorted := append([][]byte{}, children...)
	sort.Slice(sorted, func(i, j int) bool { return bytes.Compare(sorted[i], sorted[j]) < 0 })
	return derTLV(0x31, concatBytes(sorted...))
}

// derImplicitSetOf builds a SET OF but with its tag replaced by a
// context-specific constructed tag (e.g. "[0] IMPLICIT SET OF
// Attribute" in SignerInfo) — the IMPLICIT tagging RFC 5652 uses
// throughout SignedData/SignerInfo. tag must be 0-30.
func derImplicitSetOf(tag byte, children [][]byte) []byte {
	sorted := append([][]byte{}, children...)
	sort.Slice(sorted, func(i, j int) bool { return bytes.Compare(sorted[i], sorted[j]) < 0 })
	return derTLV(0xA0|tag, concatBytes(sorted...))
}

// derExplicit wraps inner in an EXPLICIT context-specific constructed
// tag. tag must be 0-30.
func derExplicit(tag byte, inner []byte) []byte {
	return derTLV(0xA0|tag, inner)
}

// derOctetString builds an OCTET STRING (tag 0x04).
func derOctetString(b []byte) []byte { return derTLV(0x04, b) }

// derNull builds the ASN.1 NULL value, used as an AlgorithmIdentifier's
// (absent-parameter) parameters field by convention for SHA-256 and
// RSA.
func derNull() []byte { return []byte{0x05, 0x00} }

// derOID marshals an OID to its full TLV via the standard library,
// which already implements OID DER encoding correctly.
func derOID(oid asn1.ObjectIdentifier) []byte {
	b, err := asn1.Marshal(oid)
	if err != nil {
		// Every OID this package uses is a compile-time constant; a
		// marshal failure here would be a bug in this file, not
		// reachable from any external input.
		panic("cms: marshalling a constant OID failed: " + err.Error())
	}
	return b
}

// derInteger marshals a big.Int to its full INTEGER TLV via the
// standard library, which already implements the two's-complement DER
// encoding correctly.
func derInteger(v *big.Int) []byte {
	b, err := asn1.Marshal(v)
	if err != nil {
		panic("cms: marshalling an integer failed: " + err.Error())
	}
	return b
}

// algorithmIdentifier builds an AlgorithmIdentifier SEQUENCE with an
// explicit NULL parameters field — the conventional encoding for both
// id-sha256 and sha256WithRSAEncryption, and what the real fixtures
// this specification was written against use.
func algorithmIdentifier(oid asn1.ObjectIdentifier) []byte {
	return derSequence(derOID(oid), derNull())
}
