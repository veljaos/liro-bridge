package render

import (
	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// cmapEnc is a composite font's /Encoding: how a string of bytes splits
// into character codes, and what CID each code selects.
//
// Identity-H — one CID per two big-endian bytes — is what every
// producer this project has looked at emits, so it is the fast path and
// the fallback. An embedded CMap stream is read for the rest; a
// predefined CMap named by the document (UniGB-UCS2-H and its family)
// is treated as two-byte identity, which is the right code *width* even
// when it is not the right CID, so text stays on its own line rather
// than dissolving into single bytes.
type cmapEnc struct {
	identity   bool
	codespaces []codespace
	singles    map[uint32]int
	ranges     []cidRange
}

type codespace struct {
	nbytes    int
	low, high uint32
}

type cidRange struct {
	nbytes    int
	low, high uint32
	cid       int
}

func identityCMap() *cmapEnc {
	return &cmapEnc{
		identity:   true,
		codespaces: []codespace{{nbytes: 2, low: 0, high: 0xFFFF}},
	}
}

// next reads the next character code from s, returning it and how many
// bytes it took. A byte sequence matching no codespace is taken as one
// byte, which is what PDF 32000-1 §9.7.6.3 prescribes for the shortest
// unmatched case.
func (c *cmapEnc) next(s []byte) (uint32, int) {
	if len(s) == 0 {
		return 0, 1
	}
	if c == nil || len(c.codespaces) == 0 {
		if len(s) >= 2 {
			return uint32(s[0])<<8 | uint32(s[1]), 2
		}
		return uint32(s[0]), 1
	}
	for n := 1; n <= 4 && n <= len(s); n++ {
		var code uint32
		for i := 0; i < n; i++ {
			code = code<<8 | uint32(s[i])
		}
		for _, cs := range c.codespaces {
			if cs.nbytes == n && code >= cs.low && code <= cs.high {
				return code, n
			}
		}
	}
	// No codespace matched. Use the shortest declared code length, so a
	// two-byte font does not start reading its text one byte at a time.
	n := c.codespaces[0].nbytes
	if n < 1 || n > len(s) {
		n = 1
	}
	var code uint32
	for i := 0; i < n; i++ {
		code = code<<8 | uint32(s[i])
	}
	return code, n
}

func (c *cmapEnc) cid(code uint32, n int) int {
	if c == nil || c.identity {
		return int(code)
	}
	if v, ok := c.singles[code]; ok {
		return v
	}
	for _, r := range c.ranges {
		if r.nbytes == n && code >= r.low && code <= r.high {
			return r.cid + int(code-r.low)
		}
	}
	return int(code)
}

func parseCMapEncoding(doc *pdf.Document, obj pdf.Object) *cmapEnc {
	switch v := doc.Resolve(obj).(type) {
	case pdf.Name:
		// Identity-H/V by name, and every predefined CMap, are treated
		// as two-byte identity — see the type comment.
		return identityCMap()
	case *pdf.Stream:
		data, err := doc.DecodeStream(v)
		if err != nil {
			return identityCMap()
		}
		enc := parseCMapStream(data)
		if len(enc.codespaces) == 0 {
			enc.codespaces = []codespace{{nbytes: 2, low: 0, high: 0xFFFF}}
		}
		return enc
	default:
		_ = v
	}
	return identityCMap()
}

// parseCMapStream reads the codespace and CID mappings out of an
// embedded CMap. It is the same PostScript-flavoured syntax a content
// stream uses, so the content-stream lexer reads it.
func parseCMapStream(data []byte) *cmapEnc {
	enc := &cmapEnc{singles: map[uint32]int{}}
	l := &clexer{data: data}
	var stack []pdf.Object
	for {
		t := l.next()
		if t.kind == tokEOF {
			return enc
		}
		if t.kind == tokObject {
			if len(stack) < 512 {
				stack = append(stack, t.obj)
			}
			continue
		}
		switch t.op {
		case "endcodespacerange":
			for i := 0; i+1 < len(stack); i += 2 {
				lo, ok1 := stack[i].(pdf.String)
				hi, ok2 := stack[i+1].(pdf.String)
				if !ok1 || !ok2 || len(lo) == 0 {
					continue
				}
				enc.codespaces = append(enc.codespaces, codespace{
					nbytes: len(lo), low: beBytes(lo), high: beBytes(hi),
				})
			}
		case "endcidrange":
			for i := 0; i+2 < len(stack); i += 3 {
				lo, ok1 := stack[i].(pdf.String)
				hi, ok2 := stack[i+1].(pdf.String)
				cid, ok3 := asInt(stack[i+2])
				if !ok1 || !ok2 || !ok3 || len(lo) == 0 {
					continue
				}
				enc.ranges = append(enc.ranges, cidRange{
					nbytes: len(lo), low: beBytes(lo), high: beBytes(hi), cid: cid,
				})
			}
		case "endcidchar":
			for i := 0; i+1 < len(stack); i += 2 {
				code, ok1 := stack[i].(pdf.String)
				cid, ok2 := asInt(stack[i+1])
				if !ok1 || !ok2 {
					continue
				}
				enc.singles[beBytes(code)] = cid
			}
		}
		stack = stack[:0]
	}
}

func beBytes(s pdf.String) uint32 {
	var v uint32
	for i := 0; i < len(s) && i < 4; i++ {
		v = v<<8 | uint32(s[i])
	}
	return v
}

// parseCIDWidths reads a /W array: alternating "cid [w w w]" runs and
// "cidFirst cidLast w" spans (PDF 32000-1 §9.7.4.3).
func parseCIDWidths(doc *pdf.Document, obj pdf.Object) map[int]float64 {
	arr, ok := doc.Resolve(obj).(pdf.Array)
	if !ok {
		return nil
	}
	out := map[int]float64{}
	for i := 0; i < len(arr); {
		first, ok := asInt(doc.Resolve(arr[i]))
		if !ok {
			i++
			continue
		}
		if i+1 >= len(arr) {
			break
		}
		switch v := doc.Resolve(arr[i+1]).(type) {
		case pdf.Array:
			for j, e := range v {
				if w, ok := asFloat(doc.Resolve(e)); ok {
					out[first+j] = w
				}
			}
			i += 2
		default:
			last, _ := asInt(v)
			if i+2 >= len(arr) {
				return out
			}
			w, _ := asFloat(doc.Resolve(arr[i+2]))
			if last-first > 65535 {
				last = first + 65535
			}
			for c := first; c <= last; c++ {
				out[c] = w
			}
			i += 3
		}
	}
	return out
}

// parseToUnicode reads a /ToUnicode CMap, which is what tells this
// renderer what a code *means* when the font program's own shapes are
// not available. Only the first character of a multi-character mapping
// is kept: a ligature substituted from one font into another is drawn as
// its first letter rather than not at all.
func parseToUnicode(doc *pdf.Document, obj pdf.Object) map[uint32]rune {
	stream, ok := doc.Resolve(obj).(*pdf.Stream)
	if !ok {
		return nil
	}
	data, err := doc.DecodeStream(stream)
	if err != nil {
		return nil
	}
	out := map[uint32]rune{}
	l := &clexer{data: data}
	var stack []pdf.Object
	for {
		t := l.next()
		if t.kind == tokEOF {
			return out
		}
		if t.kind == tokObject {
			if len(stack) < 512 {
				stack = append(stack, t.obj)
			}
			continue
		}
		switch t.op {
		case "endbfchar":
			for i := 0; i+1 < len(stack); i += 2 {
				src, ok := stack[i].(pdf.String)
				if !ok {
					continue
				}
				if dst, ok := stack[i+1].(pdf.String); ok {
					if r := utf16First(dst); r != 0 {
						out[beBytes(src)] = r
					}
				}
			}
		case "endbfrange":
			for i := 0; i+2 < len(stack); i += 3 {
				lo, ok1 := stack[i].(pdf.String)
				hi, ok2 := stack[i+1].(pdf.String)
				if !ok1 || !ok2 {
					continue
				}
				l0, l1 := beBytes(lo), beBytes(hi)
				if l1 < l0 || l1-l0 > 65535 {
					continue
				}
				switch dst := stack[i+2].(type) {
				case pdf.String:
					base := utf16First(dst)
					if base == 0 {
						continue
					}
					for c := l0; c <= l1; c++ {
						out[c] = base + rune(c-l0)
					}
				case pdf.Array:
					for j, e := range dst {
						if s, ok := e.(pdf.String); ok {
							if r := utf16First(s); r != 0 {
								out[l0+uint32(j)] = r
							}
						}
					}
				}
			}
		}
		stack = stack[:0]
	}
}

// utf16First decodes the first character of a UTF-16BE string, which is
// how /ToUnicode stores its destinations.
func utf16First(s pdf.String) rune {
	if len(s) < 2 {
		if len(s) == 1 {
			return rune(s[0])
		}
		return 0
	}
	u := rune(s[0])<<8 | rune(s[1])
	if u >= 0xD800 && u <= 0xDBFF && len(s) >= 4 {
		lo := rune(s[2])<<8 | rune(s[3])
		if lo >= 0xDC00 && lo <= 0xDFFF {
			return 0x10000 + (u-0xD800)<<10 + (lo - 0xDC00)
		}
	}
	return u
}
