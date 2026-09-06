package render

import (
	"strconv"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// clexer reads a content stream: a flat sequence of operands (ordinary
// PDF objects) and operators (bare keywords).
//
// It is a second, separate reader from internal/pades/pdf's own lexer,
// which parses the object *file* — indirect references, streams,
// cross-reference tables, none of which occur inside a content stream.
// Keeping them apart means nothing in the signing path can be reached,
// let alone changed, by anything this preview renderer does.
type clexer struct {
	data []byte
	pos  int
}

type tokKind int

const (
	tokEOF tokKind = iota
	tokObject
	tokOperator
)

type token struct {
	kind tokKind
	obj  pdf.Object
	op   string
}

func isWhite(c byte) bool {
	return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (l *clexer) skipSpace() {
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isWhite(c) {
			l.pos++
			continue
		}
		if c == '%' {
			for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
			continue
		}
		return
	}
}

func (l *clexer) next() token {
	l.skipSpace()
	if l.pos >= len(l.data) {
		return token{kind: tokEOF}
	}
	c := l.data[l.pos]
	switch {
	case c == '/':
		return token{kind: tokObject, obj: l.readName()}
	case c == '(':
		return token{kind: tokObject, obj: l.readLiteralString()}
	case c == '<':
		if l.pos+1 < len(l.data) && l.data[l.pos+1] == '<' {
			return token{kind: tokObject, obj: l.readDict()}
		}
		return token{kind: tokObject, obj: l.readHexString()}
	case c == '[':
		return token{kind: tokObject, obj: l.readArray()}
	case c == ']' || c == '>' || c == ')' || c == '}' || c == '{':
		l.pos++
		return l.next()
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return token{kind: tokObject, obj: l.readNumber()}
	}
	// A bare keyword: an operator, or one of the three literals.
	start := l.pos
	for l.pos < len(l.data) && !isWhite(l.data[l.pos]) && !isDelim(l.data[l.pos]) {
		l.pos++
	}
	word := string(l.data[start:l.pos])
	switch word {
	case "true":
		return token{kind: tokObject, obj: true}
	case "false":
		return token{kind: tokObject, obj: false}
	case "null":
		return token{kind: tokObject, obj: nil}
	case "":
		l.pos++
		return l.next()
	}
	return token{kind: tokOperator, op: word}
}

func (l *clexer) readName() pdf.Name {
	l.pos++ // '/'
	var out []byte
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isWhite(c) || isDelim(c) {
			break
		}
		if c == '#' && l.pos+2 < len(l.data) {
			if v, err := strconv.ParseUint(string(l.data[l.pos+1:l.pos+3]), 16, 8); err == nil {
				out = append(out, byte(v))
				l.pos += 3
				continue
			}
		}
		out = append(out, c)
		l.pos++
	}
	return pdf.Name(out)
}

func (l *clexer) readNumber() pdf.Object {
	start := l.pos
	real := false
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if c == '.' {
			real = true
		} else if c != '+' && c != '-' && (c < '0' || c > '9') {
			break
		}
		l.pos++
	}
	s := string(l.data[start:l.pos])
	if !real {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	}
	// Producers write ".5", "-.5" and even "4." — ParseFloat takes all
	// three; anything it refuses is treated as zero rather than as a
	// reason to stop reading the page.
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return int64(0)
	}
	return v
}

func (l *clexer) readLiteralString() pdf.String {
	l.pos++ // '('
	depth := 1
	var out []byte
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		l.pos++
		switch c {
		case '\\':
			if l.pos >= len(l.data) {
				return out
			}
			e := l.data[l.pos]
			l.pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\n':
			case '\r':
				if l.pos < len(l.data) && l.data[l.pos] == '\n' {
					l.pos++
				}
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for k := 0; k < 2 && l.pos < len(l.data) && l.data[l.pos] >= '0' && l.data[l.pos] <= '7'; k++ {
						v = v*8 + int(l.data[l.pos]-'0')
						l.pos++
					}
					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return out
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

func (l *clexer) readHexString() pdf.String {
	l.pos++ // '<'
	var out []byte
	var hi byte
	half := false
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		l.pos++
		if c == '>' {
			break
		}
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			continue
		}
		if half {
			out = append(out, hi<<4|v)
			half = false
		} else {
			hi = v
			half = true
		}
	}
	if half {
		out = append(out, hi<<4)
	}
	return out
}

func (l *clexer) readArray() pdf.Array {
	l.pos++ // '['
	var out pdf.Array
	for {
		l.skipSpace()
		if l.pos >= len(l.data) {
			return out
		}
		if l.data[l.pos] == ']' {
			l.pos++
			return out
		}
		t := l.next()
		switch t.kind {
		case tokEOF:
			return out
		case tokObject:
			out = append(out, t.obj)
		default:
			// An operator inside an array is malformed; skip it rather
			// than abandoning the rest of the page.
		}
	}
}

func (l *clexer) readDict() pdf.Dict {
	l.pos += 2 // '<<'
	out := pdf.Dict{}
	for {
		l.skipSpace()
		if l.pos >= len(l.data) {
			return out
		}
		if l.data[l.pos] == '>' {
			l.pos++
			if l.pos < len(l.data) && l.data[l.pos] == '>' {
				l.pos++
			}
			return out
		}
		if l.data[l.pos] != '/' {
			l.pos++
			continue
		}
		key := l.readName()
		t := l.next()
		if t.kind != tokObject {
			return out
		}
		out[key] = t.obj
	}
}
