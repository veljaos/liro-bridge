package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// pageContent walks Root -> Pages -> Kids[0] -> content stream and
// returns the decoded content bytes, proving every indirect reference
// along the chain resolves (F3 §2.5).
func pageContent(t *testing.T, doc *Document) []byte {
	t.Helper()
	root, ok := doc.ResolveDict(doc.Trailer().Get(Name("Root")))
	if !ok {
		t.Fatal("/Root did not resolve to a dictionary")
	}
	pages, ok := doc.ResolveDict(root.Get(Name("Pages")))
	if !ok {
		t.Fatal("/Pages did not resolve to a dictionary")
	}
	kids, ok := doc.Resolve(pages.Get(Name("Kids"))).(Array)
	if !ok || len(kids) != 1 {
		t.Fatalf("/Kids = %#v, want a one-element array", pages.Get(Name("Kids")))
	}
	page, ok := doc.ResolveDict(kids[0])
	if !ok {
		t.Fatal("page did not resolve to a dictionary")
	}
	ref, ok := page.Get(Name("Contents")).(Reference)
	if !ok {
		t.Fatalf("/Contents = %#v, want a Reference", page.Get(Name("Contents")))
	}
	stream, ok := doc.Get(ref.Num).(*Stream)
	if !ok {
		t.Fatalf("content object did not resolve to a stream")
	}
	return stream.Raw
}

func TestParseClassicFixture(t *testing.T) {
	data := buildClassicFixture()
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.UsesXrefStreams() {
		t.Fatal("UsesXrefStreams() = true for a classic-table fixture")
	}
	got := pageContent(t, doc)
	if !bytes.Contains(got, []byte("Hello")) {
		t.Fatalf("content stream = %q, want it to contain %q", got, "Hello")
	}
}

func TestParseStreamFixture(t *testing.T) {
	data := buildStreamFixture()
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !doc.UsesXrefStreams() {
		t.Fatal("UsesXrefStreams() = false for a cross-reference-stream fixture")
	}
	got := pageContent(t, doc)
	if !bytes.Contains(got, []byte("Hello")) {
		t.Fatalf("content stream = %q, want it to contain %q", got, "Hello")
	}
}

// TestObjectStreamExtractionMatchesDirectObject proves an object
// compressed inside an ObjStm resolves identically to how a direct
// object would (F3 §2.5): the Catalog is object 1, compressed at index
// 0 of object stream 6 in the stream fixture.
func TestObjectStreamExtractionMatchesDirectObject(t *testing.T) {
	doc, err := Parse(buildStreamFixture())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	catalog, ok := doc.Get(1).(Dict)
	if !ok {
		t.Fatalf("Get(1) = %#v, want Dict", doc.Get(1))
	}
	if catalog.GetName(Name("Type")) != "Catalog" {
		t.Fatalf("catalog /Type = %q, want Catalog", catalog.GetName(Name("Type")))
	}
	if ref, ok := catalog.Get(Name("Pages")).(Reference); !ok || ref.Num != 2 {
		t.Fatalf("catalog /Pages = %#v, want a reference to object 2", catalog.Get(Name("Pages")))
	}
}

// TestMultiRevisionMergeNewestWins builds a second classic revision on
// top of buildClassicFixture that redefines object 1 (the Catalog),
// adding a marker key. Parsing the combined file must return the
// revision-2 definition (F3 §2.1/§2.5), not revision 1's.
func TestMultiRevisionMergeNewestWins(t *testing.T) {
	base := buildClassicFixture()
	prevStart, ok := findStartxref(base)
	if !ok {
		t.Fatal("findStartxref on base fixture failed")
	}

	b := &fixtureBuilder{offsets: map[int]int{}}
	b.buf.Write(base)
	b.writeObj(1, "<< /Type /Catalog /Pages 2 0 R /Marker /RevisionTwo >>")
	start := b.offset()
	fmt.Fprintf(&b.buf, "xref\n1 1\n%010d %05d n \n", b.offsets[1], 0)
	fmt.Fprintf(&b.buf, "trailer\n<< /Size 5 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF", prevStart, start)

	doc, err := Parse(b.bytesOut())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	catalog, ok := doc.Get(1).(Dict)
	if !ok {
		t.Fatalf("Get(1) = %#v, want Dict", doc.Get(1))
	}
	if catalog.GetName(Name("Marker")) != "RevisionTwo" {
		t.Fatalf("/Marker = %q, want RevisionTwo (newest revision must win)", catalog.GetName(Name("Marker")))
	}
}

// TestCorruptStartxrefTriggersRebuild mangles startxref to an offset
// that is not an xref section at all; Parse must still succeed via the
// scan-and-rebuild fallback (F3 §2.2/§2.5).
func TestCorruptStartxrefTriggersRebuild(t *testing.T) {
	data := buildClassicFixture()
	idx := bytes.LastIndex(data, []byte("startxref\n"))
	if idx < 0 {
		t.Fatal("fixture has no startxref")
	}
	numStart := idx + len("startxref\n")
	numEnd := bytes.IndexByte(data[numStart:], '\n')
	if numEnd < 0 {
		t.Fatal("malformed fixture")
	}
	corrupted := append([]byte{}, data...)
	replacement := []byte("999999999")
	if len(replacement) != numEnd {
		// pad/truncate to keep the file byte length stable and simple
		if len(replacement) > numEnd {
			replacement = replacement[:numEnd]
		} else {
			for len(replacement) < numEnd {
				replacement = append(replacement, '9')
			}
		}
	}
	copy(corrupted[numStart:numStart+numEnd], replacement)

	doc, err := Parse(corrupted)
	if err != nil {
		t.Fatalf("Parse with corrupt startxref: %v", err)
	}
	got := pageContent(t, doc)
	if !bytes.Contains(got, []byte("Hello")) {
		t.Fatalf("rebuilt document content = %q, want it to contain %q", got, "Hello")
	}
}

func TestEncryptedDocumentIsRejected(t *testing.T) {
	b := newFixtureBuilder()
	writeClassicBody(b)
	b.writeClassicXref(5, 1, " /Encrypt 6 0 R")
	_, err := Parse(b.bytesOut())
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodePDFEncrypted {
		t.Fatalf("Parse(encrypted) error = %v, want CodePDFEncrypted", err)
	}
}

func TestOversizedInputIsRejected(t *testing.T) {
	data := make([]byte, MaxInputSize+1)
	_, err := Parse(data)
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodePDFInvalid {
		t.Fatalf("Parse(oversized) error = %v, want CodePDFInvalid", err)
	}
}

// --- lexer table tests (F3 §2.5) ---

func parseOneValue(t *testing.T, src string) Object {
	t.Helper()
	p := newParser([]byte(src), nil)
	v, err := p.parseValue()
	if err != nil {
		t.Fatalf("parseValue(%q): %v", src, err)
	}
	return v
}

func TestLexerNameHashEscape(t *testing.T) {
	got := parseOneValue(t, "/A#20B")
	if got != Name("A B") {
		t.Fatalf("got %q, want %q", got, "A B")
	}
}

func TestLexerLiteralStringNestedParens(t *testing.T) {
	got := parseOneValue(t, `(a (nested) string)`)
	if string(got.(String)) != "a (nested) string" {
		t.Fatalf("got %q", got)
	}
}

func TestLexerLiteralStringEscapes(t *testing.T) {
	got := parseOneValue(t, `(line1\nline2\)paren\\slash\101)`)
	want := "line1\nline2)paren\\slashA"
	if string(got.(String)) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLexerHexStringOddDigits(t *testing.T) {
	got := parseOneValue(t, "<48656C6C6F1>") // odd trailing digit, treated as trailing 0
	want := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x10}
	if !bytes.Equal(got.(String), want) {
		t.Fatalf("got % X, want % X", got.(String), want)
	}
}

func TestLexerRealNumberNotations(t *testing.T) {
	cases := map[string]float64{
		"4.":    4.0,
		"-.002": -0.002,
		"0.0":   0.0,
		"34.5":  34.5,
		"-3.62": -3.62,
	}
	for src, want := range cases {
		got := parseOneValue(t, src)
		f, ok := got.(float64)
		if !ok || f != want {
			t.Errorf("parseValue(%q) = %#v, want float64(%v)", src, got, want)
		}
	}
}

func TestLexerIntegerVsReference(t *testing.T) {
	got := parseOneValue(t, "3 0 R")
	if got != (Reference{Num: 3, Gen: 0}) {
		t.Fatalf("got %#v, want Reference{3,0}", got)
	}
	// Two adjacent integers with no "R" must not be mistaken for a
	// reference — the array test also exercises this in practice.
	arr := parseOneValue(t, "[1 2]").(Array)
	if len(arr) != 2 || arr[0] != int64(1) || arr[1] != int64(2) {
		t.Fatalf("got %#v, want [1 2]", arr)
	}
}

func TestIndirectLengthResolved(t *testing.T) {
	// Object 2 holds the integer length used by object 1's stream.
	b := newFixtureBuilder()
	b.offsets[2] = b.offset()
	b.buf.WriteString("2 0 obj\n11\nendobj\n")
	b.offsets[1] = b.offset()
	b.buf.WriteString("1 0 obj\n<< /Length 2 0 R >>\nstream\nHello World\nendstream\nendobj\n")
	b.offsets[3] = b.offset()
	b.buf.WriteString("3 0 obj\n<< /Type /Catalog >>\nendobj\n")
	b.writeClassicXref(4, 3, "")

	doc, err := Parse(b.bytesOut())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s, ok := doc.Get(1).(*Stream)
	if !ok {
		t.Fatalf("Get(1) = %#v, want *Stream", doc.Get(1))
	}
	if string(s.Raw) != "Hello World" {
		t.Fatalf("stream raw = %q, want %q", s.Raw, "Hello World")
	}
}

func TestLineEndingsAllAccepted(t *testing.T) {
	for _, eol := range []string{"\n", "\r", "\r\n"} {
		src := "1 0 obj" + eol + "<< /Type /Catalog >>" + eol + "endobj" + eol
		p := newParser([]byte(src), nil)
		_, _, obj, err := p.parseIndirectAt(0)
		if err != nil {
			t.Fatalf("eol %q: parseIndirectAt: %v", eol, err)
		}
		d, ok := obj.(Dict)
		if !ok || d.GetName(Name("Type")) != "Catalog" {
			t.Fatalf("eol %q: got %#v", eol, obj)
		}
	}
}
