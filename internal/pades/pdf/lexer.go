package pdf

import (
	"fmt"
	"strconv"
)

// isWhitespace reports whether b is PDF whitespace (spec table 1): NUL,
// TAB, LF, FF, CR, SP.
func isWhitespace(b byte) bool {
	switch b {
	case 0x00, 0x09, 0x0A, 0x0C, 0x0D, 0x20:
		return true
	}
	return false
}

// isDelimiter reports whether b is one of the nine PDF delimiter
// characters.
func isDelimiter(b byte) bool {
	switch b {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func isRegular(b byte) bool { return !isWhitespace(b) && !isDelimiter(b) }

// lengthResolver resolves an indirect /Length reference (num, gen) to its
// integer value, for a stream dictionary whose /Length is itself an
// indirect object (F3 §2.2). It returns false if the value cannot be
// resolved yet, in which case the caller falls back to scanning for
// "endstream".
type lengthResolver func(num, gen int) (int64, bool)

// parser is a cursor-based recursive-descent reader over a whole PDF
// file's bytes. Byte offsets it reports are absolute into that slice.
type parser struct {
	data          []byte
	pos           int
	resolveLength lengthResolver
	depth         int
}

// maxNestingDepth bounds recursive array/dictionary nesting. Real PDF
// objects nest a handful of levels deep; this exists so a fuzzed input
// of the form "[[[[[...." cannot recurse the Go stack into overflow
// (F3 §16.5: no panic, no unbounded allocation, no infinite loop).
const maxNestingDepth = 500

func newParser(data []byte, resolveLength lengthResolver) *parser {
	return &parser{data: data, resolveLength: resolveLength}
}

// eof also treats a negative position as end-of-input. p.pos can go
// negative when it is seeded from attacker-controlled arithmetic (an
// object-stream header's offset, an xref field) rather than advanced one
// byte at a time by this parser itself (F3 §16.5) — without this, every
// other method here that guards on eof() before indexing p.data would
// still panic on a negative index.
func (p *parser) eof() bool { return p.pos < 0 || p.pos >= len(p.data) }

// skipWhitespaceAndComments advances past whitespace and "%...EOL"
// comments (F3 §2.2 accepts \r, \n and \r\n as EOL everywhere).
func (p *parser) skipWhitespaceAndComments() {
	for !p.eof() {
		b := p.data[p.pos]
		if isWhitespace(b) {
			p.pos++
			continue
		}
		if b == '%' {
			for !p.eof() && p.data[p.pos] != '\n' && p.data[p.pos] != '\r' {
				p.pos++
			}
			continue
		}
		break
	}
}

// peekKeyword reports whether the given keyword appears at the current
// position (after skipping leading whitespace/comments), without
// consuming it.
func (p *parser) peekKeyword(kw string) bool {
	save := p.pos
	p.skipWhitespaceAndComments()
	ok := p.pos+len(kw) <= len(p.data) && string(p.data[p.pos:p.pos+len(kw)]) == kw
	p.pos = save
	return ok
}

// consumeKeyword skips whitespace, then requires kw to appear next.
func (p *parser) consumeKeyword(kw string) error {
	p.skipWhitespaceAndComments()
	if p.pos+len(kw) > len(p.data) || string(p.data[p.pos:p.pos+len(kw)]) != kw {
		return fmt.Errorf("pdf: expected keyword %q at offset %d", kw, p.pos)
	}
	p.pos += len(kw)
	return nil
}

// readRegularRun reads a maximal run of regular (non-whitespace,
// non-delimiter) bytes starting at the current position.
func (p *parser) readRegularRun() []byte {
	start := p.pos
	for !p.eof() && isRegular(p.data[p.pos]) {
		p.pos++
	}
	return p.data[start:p.pos]
}

// parseValue parses one PDF object at the current position: null,
// boolean, number, name, string (literal or hex), array, dictionary or
// stream, or an indirect reference "N G R". Numbers require lookahead to
// distinguish a plain number from the first two integers of a
// reference — see parseNumberOrReference.
func (p *parser) parseValue() (Object, error) {
	p.skipWhitespaceAndComments()
	if p.eof() {
		return nil, fmt.Errorf("pdf: unexpected end of file while parsing object")
	}
	b := p.data[p.pos]
	switch {
	case b == '/':
		return p.parseName()
	case b == '(':
		return p.parseLiteralString()
	case b == '<':
		if p.pos+1 < len(p.data) && p.data[p.pos+1] == '<' {
			return p.parseDictOrStream()
		}
		return p.parseHexString()
	case b == '[':
		return p.parseArray()
	case b == '+' || b == '-' || b == '.' || (b >= '0' && b <= '9'):
		return p.parseNumberOrReference()
	default:
		kw := string(p.readRegularRun())
		switch kw {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null":
			return nil, nil
		case "":
			return nil, fmt.Errorf("pdf: unrecognised byte 0x%02X at offset %d", b, p.pos)
		default:
			return nil, fmt.Errorf("pdf: unrecognised keyword %q at offset %d", kw, p.pos)
		}
	}
}

// parseNumberOrReference reads a number, then looks ahead for the
// pattern "<int> R" that turns two integers into an indirect reference.
// Backtracks cleanly when the lookahead does not match, so a plain
// number followed by an unrelated integer (e.g. inside an array) is left
// untouched.
func (p *parser) parseNumberOrReference() (Object, error) {
	first, isInt, err := p.parseNumber()
	if err != nil {
		return nil, err
	}
	if !isInt {
		return first, nil
	}
	save := p.pos
	p.skipWhitespaceAndComments()
	if p.eof() || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
		p.pos = save
		return first, nil
	}
	second, isInt2, err := p.parseNumber()
	if err != nil || !isInt2 {
		p.pos = save
		return first, nil
	}
	save2 := p.pos
	p.skipWhitespaceAndComments()
	if p.pos < len(p.data) && p.data[p.pos] == 'R' && (p.pos+1 >= len(p.data) || !isRegular(p.data[p.pos+1])) {
		p.pos++
		return Reference{Num: int(first.(int64)), Gen: int(second.(int64))}, nil
	}
	p.pos = save2
	_ = save
	// Not a reference: only the first number was ours to return; rewind
	// to just after it.
	p.pos = save
	return first, nil
}

// parseNumber reads a PDF integer or real number. isInt is true and the
// value is int64 when no '.' was present.
func (p *parser) parseNumber() (Object, bool, error) {
	start := p.pos
	if !p.eof() && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
		p.pos++
	}
	sawDigit := false
	sawDot := false
	for !p.eof() {
		b := p.data[p.pos]
		if b >= '0' && b <= '9' {
			sawDigit = true
			p.pos++
			continue
		}
		if b == '.' && !sawDot {
			sawDot = true
			p.pos++
			continue
		}
		break
	}
	text := string(p.data[start:p.pos])
	if !sawDigit {
		return nil, false, fmt.Errorf("pdf: malformed number %q at offset %d", text, start)
	}
	if !sawDot {
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("pdf: malformed integer %q: %w", text, err)
		}
		return n, true, nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, false, fmt.Errorf("pdf: malformed real %q: %w", text, err)
	}
	return f, false, nil
}

// parseName reads "/Name" with "#xx" hex escapes resolved (F3 §2.2: e.g.
// "/A#20B" is the name "A B").
func (p *parser) parseName() (Object, error) {
	p.pos++ // '/'
	var buf []byte
	for !p.eof() && isRegular(p.data[p.pos]) {
		b := p.data[p.pos]
		if b == '#' && p.pos+2 < len(p.data) && isHexDigit(p.data[p.pos+1]) && isHexDigit(p.data[p.pos+2]) {
			buf = append(buf, hexVal(p.data[p.pos+1])<<4|hexVal(p.data[p.pos+2]))
			p.pos += 3
			continue
		}
		buf = append(buf, b)
		p.pos++
	}
	return Name(buf), nil
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
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

// parseLiteralString reads "(...)", tracking parenthesis depth so
// balanced, unescaped parentheses do not terminate the string early
// (F3 §2.2), and resolving \n \r \t \b \f \( \) \\ \\ddd escapes and
// backslash-newline line continuations per PDF 32000-1 §7.3.4.2.
func (p *parser) parseLiteralString() (Object, error) {
	p.pos++ // '('
	depth := 1
	var buf []byte
	for {
		if p.eof() {
			return nil, fmt.Errorf("pdf: unterminated literal string")
		}
		b := p.data[p.pos]
		switch b {
		case '(':
			depth++
			buf = append(buf, b)
			p.pos++
		case ')':
			depth--
			p.pos++
			if depth == 0 {
				return String(buf), nil
			}
			buf = append(buf, b)
		case '\\':
			p.pos++
			if p.eof() {
				return String(buf), nil
			}
			e := p.data[p.pos]
			switch e {
			case 'n':
				buf = append(buf, '\n')
				p.pos++
			case 'r':
				buf = append(buf, '\r')
				p.pos++
			case 't':
				buf = append(buf, '\t')
				p.pos++
			case 'b':
				buf = append(buf, '\b')
				p.pos++
			case 'f':
				buf = append(buf, '\f')
				p.pos++
			case '(', ')', '\\':
				buf = append(buf, e)
				p.pos++
			case '\r':
				p.pos++
				if !p.eof() && p.data[p.pos] == '\n' {
					p.pos++
				}
			case '\n':
				p.pos++
			default:
				if e >= '0' && e <= '7' {
					val := 0
					n := 0
					for n < 3 && !p.eof() && p.data[p.pos] >= '0' && p.data[p.pos] <= '7' {
						val = val*8 + int(p.data[p.pos]-'0')
						p.pos++
						n++
					}
					buf = append(buf, byte(val))
				} else {
					// Backslash followed by anything else: the
					// backslash is ignored (spec §7.3.4.2).
					buf = append(buf, e)
					p.pos++
				}
			}
		default:
			buf = append(buf, b)
			p.pos++
		}
	}
}

// parseHexString reads "<...>", ignoring embedded whitespace. An odd
// number of hex digits is treated as if a trailing '0' were present
// (F3 §2.2/§2.5).
func (p *parser) parseHexString() (Object, error) {
	p.pos++ // '<'
	var digits []byte
	for {
		if p.eof() {
			return nil, fmt.Errorf("pdf: unterminated hex string")
		}
		b := p.data[p.pos]
		if b == '>' {
			p.pos++
			break
		}
		if isWhitespace(b) {
			p.pos++
			continue
		}
		if !isHexDigit(b) {
			return nil, fmt.Errorf("pdf: invalid hex digit 0x%02X in hex string at offset %d", b, p.pos)
		}
		digits = append(digits, b)
		p.pos++
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	out := make([]byte, len(digits)/2)
	for i := range out {
		out[i] = hexVal(digits[2*i])<<4 | hexVal(digits[2*i+1])
	}
	return String(out), nil
}

// parseArray reads "[...]".
func (p *parser) parseArray() (Object, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxNestingDepth {
		return nil, fmt.Errorf("pdf: array nesting exceeds %d levels at offset %d", maxNestingDepth, p.pos)
	}
	p.pos++ // '['
	var arr Array
	for {
		p.skipWhitespaceAndComments()
		if p.eof() {
			return nil, fmt.Errorf("pdf: unterminated array")
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
	}
}

// parseDictOrStream reads "<<...>>" and, if immediately followed by the
// "stream" keyword, the stream body as well (F3 §2.2/§4.2).
func (p *parser) parseDictOrStream() (Object, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxNestingDepth {
		return nil, fmt.Errorf("pdf: dictionary nesting exceeds %d levels at offset %d", maxNestingDepth, p.pos)
	}
	p.pos += 2 // '<<'
	dict := Dict{}
	for {
		p.skipWhitespaceAndComments()
		if p.eof() {
			return nil, fmt.Errorf("pdf: unterminated dictionary")
		}
		if p.data[p.pos] == '>' && p.pos+1 < len(p.data) && p.data[p.pos+1] == '>' {
			p.pos += 2
			break
		}
		if p.data[p.pos] != '/' {
			return nil, fmt.Errorf("pdf: expected name key at offset %d", p.pos)
		}
		keyObj, err := p.parseName()
		if err != nil {
			return nil, err
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		dict[keyObj.(Name)] = val
	}

	if !p.peekKeyword("stream") {
		return dict, nil
	}
	if err := p.consumeKeyword("stream"); err != nil {
		return nil, err
	}
	// The "stream" keyword must be followed by CRLF or a lone LF (spec
	// §7.3.8.1); a lone CR is nonconforming but tolerated here per
	// F3 §2.2's general leniency on line endings.
	if !p.eof() && p.data[p.pos] == '\r' {
		p.pos++
	}
	if !p.eof() && p.data[p.pos] == '\n' {
		p.pos++
	}
	bodyStart := p.pos

	length, ok := p.streamLength(dict)
	if ok && bodyStart+int(length) <= len(p.data) {
		bodyEnd := bodyStart + int(length)
		trailingOK := p.peekEndstreamAt(bodyEnd)
		if trailingOK {
			p.pos = bodyEnd
			p.skipWhitespaceAndComments()
			if err := p.consumeKeyword("endstream"); err != nil {
				return nil, err
			}
			return &Stream{Dict: dict, Raw: p.data[bodyStart:bodyEnd]}, nil
		}
	}

	// /Length was wrong, unresolved, or absent: fall back to scanning
	// for "endstream" (F3 §2.2's explicit fallback for an indirect
	// /Length that cannot yet be resolved).
	idx := indexOf(p.data, p.pos, "endstream")
	if idx < 0 {
		return nil, fmt.Errorf("pdf: stream at offset %d has no endstream", bodyStart)
	}
	bodyEnd := idx
	// Trim exactly one trailing EOL that precedes "endstream" and is not
	// part of the stream data.
	if bodyEnd > bodyStart && p.data[bodyEnd-1] == '\n' {
		bodyEnd--
		if bodyEnd > bodyStart && p.data[bodyEnd-1] == '\r' {
			bodyEnd--
		}
	} else if bodyEnd > bodyStart && p.data[bodyEnd-1] == '\r' {
		bodyEnd--
	}
	p.pos = idx
	if err := p.consumeKeyword("endstream"); err != nil {
		return nil, err
	}
	return &Stream{Dict: dict, Raw: p.data[bodyStart:bodyEnd]}, nil
}

// peekEndstreamAt reports whether "endstream" appears at or shortly
// after offset (allowing the optional EOL between stream data and the
// keyword), confirming a resolved /Length was actually correct.
func (p *parser) peekEndstreamAt(offset int) bool {
	i := offset
	for i < len(p.data) && (p.data[i] == '\r' || p.data[i] == '\n' || p.data[i] == ' ' || p.data[i] == '\t') {
		i++
	}
	return i+len("endstream") <= len(p.data) && string(p.data[i:i+len("endstream")]) == "endstream"
}

// streamLength resolves a stream dictionary's /Length: a direct integer,
// or an indirect reference resolved via p.resolveLength.
func (p *parser) streamLength(dict Dict) (int64, bool) {
	switch v := dict[Name("Length")].(type) {
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case Reference:
		if p.resolveLength == nil {
			return 0, false
		}
		return p.resolveLength(v.Num, v.Gen)
	default:
		return 0, false
	}
}

// parseIndirectAt parses "N G obj <value> [endobj]" at an absolute byte
// offset, used both for ordinary object resolution and for reading the
// indirect stream object that carries a cross-reference stream.
func (p *parser) parseIndirectAt(offset int) (num, gen int, obj Object, err error) {
	// offset comes straight from an xref entry — a classic table's
	// 10-digit field, a stream's binary row, or the "N G obj" rebuild
	// scan — and in a malformed or fuzzed file can be arbitrary (F3
	// §16.5). Bounds-check before using it as a slice index.
	if offset < 0 || offset >= len(p.data) {
		return 0, 0, nil, fmt.Errorf("pdf: object offset %d out of range [0,%d)", offset, len(p.data))
	}
	p.pos = offset
	p.skipWhitespaceAndComments()
	n1, isInt1, err := p.parseNumber()
	if err != nil || !isInt1 {
		return 0, 0, nil, fmt.Errorf("pdf: expected object number at offset %d", offset)
	}
	p.skipWhitespaceAndComments()
	n2, isInt2, err := p.parseNumber()
	if err != nil || !isInt2 {
		return 0, 0, nil, fmt.Errorf("pdf: expected generation number at offset %d", offset)
	}
	if err := p.consumeKeyword("obj"); err != nil {
		return 0, 0, nil, err
	}
	obj, err = p.parseValue()
	if err != nil {
		return 0, 0, nil, fmt.Errorf("pdf: parsing object %d %d at offset %d: %w", n1, n2, offset, err)
	}
	p.skipWhitespaceAndComments()
	if p.peekKeyword("endobj") {
		_ = p.consumeKeyword("endobj")
	}
	return int(n1.(int64)), int(n2.(int64)), obj, nil
}

// indexOf finds the first occurrence of needle in data at or after
// start, without allocating a substring for every candidate offset.
func indexOf(data []byte, start int, needle string) int {
	if start < 0 {
		start = 0
	}
	n := len(needle)
	for i := start; i+n <= len(data); i++ {
		if string(data[i:i+n]) == needle {
			return i
		}
	}
	return -1
}
