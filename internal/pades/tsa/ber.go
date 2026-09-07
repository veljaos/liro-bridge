// Package tsa implements an RFC 3161 time-stamping client (F3 §6).
package tsa

import (
	"fmt"
	"math"
)

// berNode is one parsed BER/DER tag-length-value. content holds the
// value bytes: for a definite-length value, exactly what the length
// byte(s) said; for an indefinite-length constructed value, everything
// between the header and the matching End-of-Contents (00 00) marker,
// still in its original TLV-encoded child form (see octetStringValue
// for reassembling a fragmented OCTET STRING's actual bytes).
//
// This parser exists because SPEC §12.5 states plainly, from real
// signed documents: timestamp tokens produced by the state's own
// signing tool are BER with indefinite length ("30 80 ..."), which
// Go's standard library encoding/asn1 refuses outright (it implements
// DER only). A parser that cannot read these documents breaks on a
// large share of real-world Serbian PDFs.
type berNode struct {
	class       byte // 0 universal, 1 application, 2 context-specific, 3 private
	constructed bool
	tag         int
	indefinite  bool
	content     []byte
	raw         []byte // the complete TLV exactly as received, EOC markers included — used when this project must re-embed a value byte-for-byte, whatever encoding (BER or DER) it originally arrived in
}

// parseBERValue parses exactly one TLV from the start of data, and
// returns the remaining bytes after it.
func parseBERValue(data []byte) (berNode, []byte, error) {
	class, constructed, tag, length, indefinite, headerLen, err := readTagAndLength(data)
	if err != nil {
		return berNode{}, nil, err
	}
	body := data[headerLen:]
	n := berNode{class: class, constructed: constructed, tag: tag, indefinite: indefinite}

	if indefinite {
		if !constructed {
			return berNode{}, nil, fmt.Errorf("tsa: indefinite length on a primitive value")
		}
		content, consumed, err := readUntilEOC(body)
		if err != nil {
			return berNode{}, nil, err
		}
		n.content = content
		n.raw = data[:headerLen+consumed]
		return n, body[consumed:], nil
	}
	if length > len(body) {
		return berNode{}, nil, fmt.Errorf("tsa: declared length %d exceeds %d available bytes", length, len(body))
	}
	n.content = body[:length]
	n.raw = data[:headerLen+length]
	return n, body[length:], nil
}

// readTagAndLength decodes a BER identifier and length octet sequence,
// supporting the high-tag-number form and both definite and indefinite
// (0x80) length forms.
func readTagAndLength(data []byte) (class byte, constructed bool, tag int, length int, indefinite bool, headerLen int, err error) {
	if len(data) < 2 {
		return 0, false, 0, 0, false, 0, fmt.Errorf("tsa: truncated BER header")
	}
	b0 := data[0]
	class = b0 >> 6
	constructed = b0&0x20 != 0
	tag = int(b0 & 0x1F)
	pos := 1
	if tag == 0x1F {
		tag = 0
		for {
			if pos >= len(data) {
				return 0, false, 0, 0, false, 0, fmt.Errorf("tsa: truncated high-tag-number form")
			}
			c := data[pos]
			tag = tag<<7 | int(c&0x7F)
			pos++
			if c&0x80 == 0 {
				break
			}
		}
	}
	if pos >= len(data) {
		return 0, false, 0, 0, false, 0, fmt.Errorf("tsa: truncated BER length")
	}
	lb := data[pos]
	pos++
	switch {
	case lb == 0x80:
		indefinite = true
	case lb&0x80 != 0:
		n := int(lb &^ 0x80)
		if n > 8 || pos+n > len(data) {
			return 0, false, 0, 0, false, 0, fmt.Errorf("tsa: malformed long-form length")
		}
		var v uint64
		for i := 0; i < n; i++ {
			v = v<<8 | uint64(data[pos+i])
		}
		// Eight length octets are enough to set the sign bit of an int,
		// and a negative length is not a length — it is a slice bound
		// that panics rather than a value any check further down can
		// reject. Found by fuzzing: "30 88 30 30 30 30 30 30 30 30"
		// produced a slice of [:-8633347502144212944] before this
		// check existed. The same shape was in this project's second,
		// independently written reader (internal/pades/verify), which
		// is worth saying out loud: two implementations do not catch a
		// mistake both of them make.
		if v > uint64(math.MaxInt) {
			return 0, false, 0, 0, false, 0, fmt.Errorf("tsa: long-form length %d is larger than this machine can address", v)
		}
		length = int(v)
		pos += n
	default:
		length = int(lb)
	}
	return class, constructed, tag, length, indefinite, pos, nil
}

// readUntilEOC scans a sequence of TLVs starting at data until it finds
// the End-of-Contents marker (00 00) at this nesting level, returning
// everything before it and how many bytes (including the marker) were
// consumed.
func readUntilEOC(data []byte) (content []byte, consumed int, err error) {
	pos := 0
	for {
		if pos+2 > len(data) {
			return nil, 0, fmt.Errorf("tsa: unterminated indefinite-length value")
		}
		if data[pos] == 0x00 && data[pos+1] == 0x00 {
			return data[:pos], pos + 2, nil
		}
		_, rest, err := parseBERValue(data[pos:])
		if err != nil {
			return nil, 0, err
		}
		pos = len(data) - len(rest)
	}
}

// children parses n's content as a sequence of sibling TLVs — valid for
// any constructed value.
func (n berNode) children() ([]berNode, error) {
	var out []berNode
	data := n.content
	for len(data) > 0 {
		child, rest, err := parseBERValue(data)
		if err != nil {
			return nil, err
		}
		out = append(out, child)
		data = rest
	}
	return out, nil
}

// octetStringValue returns the actual OCTET STRING bytes n encodes:
// n.content directly for a primitive encoding, or the concatenation of
// each fragment's own value for BER's constructed (fragmented) form —
// which is how an indefinite-length OCTET STRING (F3 §12.5) is built.
func (n berNode) octetStringValue() ([]byte, error) {
	if !n.constructed {
		return n.content, nil
	}
	parts, err := n.children()
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
