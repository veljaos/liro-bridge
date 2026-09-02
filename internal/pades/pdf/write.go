package pdf

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
)

// writeIndirectObject serialises "N 0 obj\n<value>\nendobj\n" for num.
// Generation is always 0: nothing in this project reuses a freed object
// slot with a bumped generation (F3 never requires it).
func writeIndirectObject(w *bytes.Buffer, num int, obj Object) {
	fmt.Fprintf(w, "%d 0 obj\n", num)
	writeObject(w, obj)
	w.WriteString("\nendobj\n")
}

// writeObject serialises obj in canonical form. Strings are written as
// literal (...) strings by default — with (, ) and \ backslash-escaped
// and any byte outside printable ASCII (plus \n, \r, \t) as a \ddd
// octal escape — matching what every real PDF producer emits for text
// fields such as /DA, /T and /Lang (Task 2 fix — see docs/decisions.md).
// A string is written as a hex (<...>) string only when isBinaryString
// says its bytes do not look like text at all, which is what this
// project's own /ID values are: PDF 32000-1 has no requirement either
// way, but every reference implementation reserves hex for that kind of
// genuinely binary content and writes everything else literally, and
// Adobe Acrobat's own signature-widget renderer executes /DA as a
// content-stream fragment and reads /T while assembling the field
// tree — both are exactly the two keys this project used to hex-encode.
func writeObject(w *bytes.Buffer, obj Object) {
	switch v := obj.(type) {
	case nil:
		w.WriteString("null")
	case bool:
		if v {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	case int64:
		fmt.Fprintf(w, "%d", v)
	case float64:
		w.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
	case Name:
		w.WriteByte('/')
		writeNameEscaped(w, v)
	case String:
		if isBinaryString(v) {
			w.WriteByte('<')
			w.WriteString(hex.EncodeToString(v))
			w.WriteByte('>')
		} else {
			writeLiteralString(w, v)
		}
	case Array:
		w.WriteByte('[')
		for i, e := range v {
			if i > 0 {
				w.WriteByte(' ')
			}
			writeObject(w, e)
		}
		w.WriteByte(']')
	case Dict:
		writeDict(w, v)
	case Reference:
		fmt.Fprintf(w, "%d %d R", v.Num, v.Gen)
	case Raw:
		w.Write(v)
	case *Stream:
		d := make(Dict, len(v.Dict)+1)
		for k, val := range v.Dict {
			d[k] = val
		}
		d[Name("Length")] = int64(len(v.Raw))
		writeDict(w, d)
		w.WriteString("\nstream\n")
		w.Write(v.Raw)
		w.WriteString("\nendstream")
	default:
		// Should not happen: Object is a closed set (object.go), and
		// every case above covers one member of it.
		panic(fmt.Sprintf("pdf: writeObject: unhandled type %T", obj))
	}
}

// isBinaryString reports whether v should be written as a hex string
// rather than a literal one: true when v contains any byte outside
// printable ASCII (0x20-0x7E) and the three whitespace bytes literal
// strings escape cleanly (\n \r \t). An /ID value — a raw digest — is
// overwhelmingly likely to contain such a byte; /DA, /T, /Lang and
// every other text-like string this project reads or writes are not.
// Deciding from content, rather than threading a per-key "this one is
// binary" flag through every caller, means the choice is made once,
// here, for both strings this project constructs itself and strings
// copied unmodified from a parsed document (F3 §3.3).
func isBinaryString(v String) bool {
	for _, b := range v {
		if b >= 0x20 && b <= 0x7E {
			continue
		}
		switch b {
		case '\n', '\r', '\t':
			continue
		}
		return true
	}
	return false
}

// writeLiteralString writes v as a "(...)" string, backslash-escaping
// '(', ')' and '\\' and octal-escaping any byte a literal string cannot
// carry unescaped (PDF 32000-1 §7.3.4.2). isBinaryString already keeps
// this function off the byte ranges that would produce a long run of
// \ddd escapes, but it handles the full byte range correctly regardless
// — the two are independent, testable properties.
func writeLiteralString(w *bytes.Buffer, v String) {
	w.WriteByte('(')
	for _, b := range v {
		switch b {
		case '(', ')', '\\':
			w.WriteByte('\\')
			w.WriteByte(b)
		case '\n':
			w.WriteString(`\n`)
		case '\r':
			w.WriteString(`\r`)
		case '\t':
			w.WriteString(`\t`)
		default:
			if b < 0x20 || b > 0x7E {
				fmt.Fprintf(w, "\\%03o", b)
			} else {
				w.WriteByte(b)
			}
		}
	}
	w.WriteByte(')')
}

// writeDict writes keys in sorted order so output is deterministic —
// required for the byte-exact golden-file test (F3 §4.5) and harmless
// everywhere else, since PDF dictionary key order carries no meaning.
func writeDict(w *bytes.Buffer, d Dict) {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	w.WriteString("<<")
	for _, k := range keys {
		w.WriteByte('/')
		writeNameEscaped(w, Name(k))
		w.WriteByte(' ')
		writeObject(w, d[Name(k)])
	}
	w.WriteString(">>")
}

// writeNameEscaped writes a Name with "#xx" escapes for whitespace,
// delimiters and bytes outside the printable ASCII range (PDF 32000-1
// §7.3.5) — the exact inverse of parseName's decoding.
func writeNameEscaped(w *bytes.Buffer, n Name) {
	for i := 0; i < len(n); i++ {
		b := n[i]
		if isWhitespace(b) || isDelimiter(b) || b < 0x21 || b > 0x7E || b == '#' {
			fmt.Fprintf(w, "#%02X", b)
			continue
		}
		w.WriteByte(b)
	}
}
