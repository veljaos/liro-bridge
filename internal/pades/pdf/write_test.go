package pdf

import (
	"bytes"
	"testing"
)

// TestWriteObjectStringIsLiteralByDefault is Task 2's own test: every
// real PDF producer, and the original document this project rewrites
// via incremental update, write /DA, /T and /Lang as literal (...)
// strings — never hex. Acrobat executes /DA as a content-stream
// fragment and reads /T while assembling the AcroForm field tree; if
// either fails in Acrobat's own parser, it renders no fields at all
// (empty Signature Panel, "At least one signature is invalid"), which
// is the exact symptom this fixes.
func TestWriteObjectStringIsLiteralByDefault(t *testing.T) {
	cases := []struct {
		name string
		in   String
		want string
	}{
		{"DA", String("/Helv 0 Tf 0 g "), "(/Helv 0 Tf 0 g )"},
		{"T", String("Signature1"), "(Signature1)"},
		{"Lang", String("en"), "(en)"},
		{"empty", String(""), "()"},
		{"parens and backslash", String(`a(b)c\d`), `(a\(b\)c\\d)`},
		{"newline and tab", String("a\nb\tc"), `(a\nb\tc)`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeObject(&buf, tc.in)
			if got := buf.String(); got != tc.want {
				t.Errorf("writeObject(%q) = %s, want %s", tc.in, got, tc.want)
			}
			if bytes.Contains(buf.Bytes(), []byte("<")) {
				t.Errorf("writeObject(%q) = %s, gratuitously hex-encoded", tc.in, buf.String())
			}
		})
	}
}

// TestWriteObjectStringIsHexForBinaryContent is the other half of Task
// 2: a genuinely binary string (an /ID value — a raw digest, not text)
// is still written as a hex string, not forced through literal-string
// octal escaping that no real producer uses for that kind of content.
func TestWriteObjectStringIsHexForBinaryContent(t *testing.T) {
	id := String{0x4c, 0x69, 0x00, 0xff, 0x80, 0x01, 0x02, 0x03}
	var buf bytes.Buffer
	writeObject(&buf, id)
	got := buf.String()
	if len(got) == 0 || got[0] != '<' || got[len(got)-1] != '>' {
		t.Fatalf("writeObject(%v) = %s, want a hex string", []byte(id), got)
	}
}

// TestWriteObjectLiteralStringRoundTripsThroughParser proves the
// literal-string writer's escaping is actually correct, not just
// visually plausible: every byte value, fed through writeObject then
// parseLiteralString, comes back unchanged — including bytes that need
// octal escaping and the handful of controls whitespace-related
// escaping covers.
func TestWriteObjectLiteralStringRoundTripsThroughParser(t *testing.T) {
	var all []byte
	for b := 0; b < 256; b++ {
		all = append(all, byte(b))
	}
	in := String(all)
	var buf bytes.Buffer
	writeObject(&buf, in)

	p := newParser(buf.Bytes(), nil)
	got, err := p.parseValue()
	if err != nil {
		t.Fatalf("re-parsing writeObject's own output: %v", err)
	}
	gotStr, ok := got.(String)
	if !ok {
		t.Fatalf("re-parsed value is %T, want String", got)
	}
	if !bytes.Equal(gotStr, in) {
		t.Fatalf("round trip changed the bytes:\n got  %v\n want %v", []byte(gotStr), []byte(in))
	}
}

// TestIsBinaryString pins the content-based decision itself, since
// writeObject's choice of literal vs. hex depends entirely on it.
func TestIsBinaryString(t *testing.T) {
	cases := []struct {
		name string
		in   String
		want bool
	}{
		{"printable ASCII", String("/Helv 0 Tf 0 g "), false},
		{"empty", String(""), false},
		{"tab newline CR", String("a\tb\nc\rd"), false},
		{"high bit set", String([]byte{0x80}), true},
		{"NUL", String([]byte{0x00}), true},
		{"DEL", String([]byte{0x7F}), true},
		{"random digest bytes", String([]byte{0x4c, 0x69, 0x00, 0xff}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBinaryString(tc.in); got != tc.want {
				t.Errorf("isBinaryString(%v) = %v, want %v", []byte(tc.in), got, tc.want)
			}
		})
	}
}
