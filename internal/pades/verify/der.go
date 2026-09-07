// Package verify is the independent PAdES/CMS verifier F3 §8 and
// SPEC §16.4 require: it recomputes the /ByteRange digest, verifies the
// RSA signature over the re-tagged SignedAttrs, validates
// signingCertificateV2, parses and validates the timestamp token
// (BER-tolerant), and checks the chain against the Trusted List.
//
// It is written from RFC 5652 and the PAdES specification directly, and
// deliberately shares no code with internal/pades/pdf,
// internal/pades/cms or internal/pades/tsa — the packages that produce
// a signature in the first place. F3 §8: "a bug in a shared helper
// passes in both directions and the test proves nothing." Every ASN.1
// reader, byte-offset calculation and re-tagging step below is its own
// implementation, not a call into the signer's. The only things this
// package does reuse are the Go standard library's crypto primitives
// (crypto/rsa, crypto/x509, crypto/sha256) — the trusted oracle, not a
// helper this project wrote — and internal/trust/tsl, which is F1's
// independently developed and tested Trusted List client, not part of
// the CMS/ByteRange signing path this rule protects.
package verify

import (
	"fmt"
	"math"
)

// derNode is one parsed BER/DER tag-length-value, read independently of
// internal/pades/tsa's identically-purposed but separate berNode.
type derNode struct {
	class       byte
	constructed bool
	tag         int
	indefinite  bool
	content     []byte
	raw         []byte // the complete TLV as it appears in the source, EOC markers included
}

// readDER reads exactly one TLV starting at data and returns it plus
// whatever bytes follow it.
func readDER(data []byte) (derNode, []byte, error) {
	if len(data) < 2 {
		return derNode{}, nil, fmt.Errorf("verify: truncated DER/BER header")
	}
	b0 := data[0]
	n := derNode{class: b0 >> 6, constructed: b0&0x20 != 0, tag: int(b0 & 0x1F)}
	pos := 1
	if n.tag == 0x1F {
		n.tag = 0
		for {
			if pos >= len(data) {
				return derNode{}, nil, fmt.Errorf("verify: truncated high-tag-number form")
			}
			c := data[pos]
			n.tag = n.tag<<7 | int(c&0x7F)
			pos++
			if c&0x80 == 0 {
				break
			}
		}
	}
	if pos >= len(data) {
		return derNode{}, nil, fmt.Errorf("verify: truncated length")
	}
	lb := data[pos]
	pos++
	var length int
	switch {
	case lb == 0x80:
		n.indefinite = true
	case lb&0x80 != 0:
		numBytes := int(lb &^ 0x80)
		if numBytes == 0 || numBytes > 8 || pos+numBytes > len(data) {
			return derNode{}, nil, fmt.Errorf("verify: malformed long-form length")
		}
		var v uint64
		for i := 0; i < numBytes; i++ {
			v = v<<8 | uint64(data[pos+i])
		}
		// Eight length octets are enough to set the sign bit of an int,
		// and every check below this one compares length against a
		// buffer size — which a negative value passes, straight into a
		// slice bound that panics. Found by fuzzing the sibling reader
		// in internal/pades/tsa, which had the identical defect: this
		// package is deliberately an independent second implementation
		// (D-044), and independence is no help against a mistake both
		// implementations make.
		if v > uint64(math.MaxInt) {
			return derNode{}, nil, fmt.Errorf("verify: long-form length %d is larger than this machine can address", v)
		}
		length = int(v)
		pos += numBytes
	default:
		length = int(lb)
	}

	body := data[pos:]
	if n.indefinite {
		if !n.constructed {
			return derNode{}, nil, fmt.Errorf("verify: indefinite length on a primitive value")
		}
		content, consumed, err := readUntilEndOfContents(body)
		if err != nil {
			return derNode{}, nil, err
		}
		n.content = content
		n.raw = data[:pos+consumed]
		return n, body[consumed:], nil
	}
	if length > len(body) {
		return derNode{}, nil, fmt.Errorf("verify: declared length %d exceeds %d available bytes", length, len(body))
	}
	n.content = body[:length]
	n.raw = data[:pos+length]
	return n, body[length:], nil
}

func readUntilEndOfContents(data []byte) (content []byte, consumed int, err error) {
	pos := 0
	for {
		if pos+2 > len(data) {
			return nil, 0, fmt.Errorf("verify: unterminated indefinite-length value")
		}
		if data[pos] == 0 && data[pos+1] == 0 {
			return data[:pos], pos + 2, nil
		}
		_, rest, err := readDER(data[pos:])
		if err != nil {
			return nil, 0, err
		}
		pos = len(data) - len(rest)
	}
}

// sequence parses n.content as a run of sibling TLVs.
func (n derNode) sequence() ([]derNode, error) {
	var out []derNode
	data := n.content
	for len(data) > 0 {
		child, rest, err := readDER(data)
		if err != nil {
			return nil, err
		}
		out = append(out, child)
		data = rest
	}
	return out, nil
}

// octetStringValue returns the actual bytes an OCTET STRING encodes,
// reassembling BER's fragmented (constructed) form when present.
func (n derNode) octetStringValue() ([]byte, error) {
	if !n.constructed {
		return n.content, nil
	}
	parts, err := n.sequence()
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, p := range parts {
		v, err := p.octetStringValue()
		if err != nil {
			return nil, err
		}
		out = append(out, v...)
	}
	return out, nil
}
