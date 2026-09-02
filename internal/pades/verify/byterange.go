package verify

import (
	"bytes"
	"fmt"
)

// SignatureSlot is one signature dictionary found by scanning the raw
// file for "/ByteRange", with the CMS bytes already extracted from the
// hex-encoded /Contents span the ByteRange itself names.
type SignatureSlot struct {
	ByteRange [4]int64
	CMS       []byte
}

// FindSignatures scans pdfBytes directly — not via any PDF object
// parser — for every "/ByteRange" occurrence, reading the four integers
// that follow it (F3 §4.2's "[0 b c d]") and taking pdfBytes[b:c] as the
// signature's delimited "<...>" hex string. This is deliberately
// independent of internal/pades/pdf: it recomputes b and c from the
// bytes actually on disk, which is the F3 §4.5/§8 requirement that this
// computation never trust the signing code's own offset variables.
//
// Because /ByteRange's own [0,b) + [c,c+d) span already identifies
// exactly where the hex string is, this needs no separate "/Contents"
// text search or dictionary-boundary parsing — the excluded span *is*
// the delimited hex string, by construction (F3 §4.2 step 4).
func FindSignatures(pdfBytes []byte) ([]SignatureSlot, error) {
	var out []SignatureSlot
	search := pdfBytes
	base := 0
	for {
		i := bytes.Index(search, []byte("/ByteRange"))
		if i < 0 {
			break
		}
		pos := base + i + len("/ByteRange")
		br, next, err := parseByteRangeArray(pdfBytes, pos)
		if err != nil {
			// Not a real match (e.g. the literal text appears inside a
			// content stream) — resume scanning just past it.
			base = pos
			search = pdfBytes[base:]
			continue
		}
		base = next
		search = pdfBytes[base:]

		b, c, d := br[1], br[2], br[3]
		if b < 0 || c < b || d < 0 || c+d > int64(len(pdfBytes)) {
			continue
		}
		span := pdfBytes[b:c]
		if len(span) < 2 || span[0] != '<' || span[len(span)-1] != '>' {
			continue
		}
		cms, err := decodeSignedCMS(span[1 : len(span)-1])
		if err != nil {
			continue
		}
		out = append(out, SignatureSlot{ByteRange: br, CMS: cms})
	}
	return out, nil
}

// parseByteRangeArray parses "[ n n n n ]" starting at or after pos,
// tolerating the whitespace this project's own writer uses for
// space-padded fixed-width fields (F3 §4.4), and returns the four
// values plus the position just after the closing ']'.
func parseByteRangeArray(data []byte, pos int) ([4]int64, int, error) {
	var out [4]int64
	pos = skipPDFWhitespace(data, pos)
	if pos >= len(data) || data[pos] != '[' {
		return out, 0, fmt.Errorf("verify: no '[' after /ByteRange")
	}
	pos++
	for i := 0; i < 4; i++ {
		pos = skipPDFWhitespace(data, pos)
		start := pos
		for pos < len(data) && data[pos] >= '0' && data[pos] <= '9' {
			pos++
		}
		if pos == start {
			return out, 0, fmt.Errorf("verify: expected a digit in /ByteRange array")
		}
		var v int64
		for _, c := range data[start:pos] {
			v = v*10 + int64(c-'0')
		}
		out[i] = v
	}
	pos = skipPDFWhitespace(data, pos)
	if pos >= len(data) || data[pos] != ']' {
		return out, 0, fmt.Errorf("verify: no ']' closing /ByteRange array")
	}
	return out, pos + 1, nil
}

func skipPDFWhitespace(data []byte, pos int) int {
	for pos < len(data) {
		switch data[pos] {
		case 0x00, 0x09, 0x0A, 0x0C, 0x0D, 0x20:
			pos++
			continue
		}
		return pos
	}
	return pos
}

// decodeSignedCMS hex-decodes a /Contents span (ignoring embedded
// whitespace, the way PDF hex strings always do) and returns exactly
// the leading DER TLV it contains — trusting the CMS SEQUENCE's own
// self-describing length to discard the zero-padding this project's
// signer (and most others) reserve after it, rather than needing to
// know the "real" length from anywhere else.
func decodeSignedCMS(hexSpan []byte) ([]byte, error) {
	var digits []byte
	for _, b := range hexSpan {
		switch {
		case b == 0x00 || b == 0x09 || b == 0x0A || b == 0x0C || b == 0x0D || b == 0x20:
			continue
		case b >= '0' && b <= '9', b >= 'a' && b <= 'f', b >= 'A' && b <= 'F':
			digits = append(digits, b)
		default:
			return nil, fmt.Errorf("verify: invalid hex digit 0x%02X in /Contents", b)
		}
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	raw := make([]byte, len(digits)/2)
	for i := range raw {
		raw[i] = hexVal(digits[2*i])<<4 | hexVal(digits[2*i+1])
	}
	node, _, err := readDER(raw)
	if err != nil {
		return nil, fmt.Errorf("verify: /Contents is not a valid DER value: %w", err)
	}
	return node.raw, nil
}

func hexVal(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	default:
		return b - 'A' + 10
	}
}
