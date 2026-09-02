package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
)

// This file builds small, synthetic PDF fixtures in Go, with every byte
// offset computed at build time rather than hand-typed. No real signed
// PDF is committed to this repository — real Serbian qualified
// signatures embed a real person's name, national ID number and email
// (SPEC §6.7/§11.6), so real fixtures live only in testdata/pdfs/local/
// (gitignored, see its README), mirroring the split D-021 already
// established for certificates. These synthetic fixtures exist to prove
// the *shape* of the parser and writer is right: one exercises the
// classic xref-table mechanism, the other cross-reference streams with
// an object stream, matching the two mechanisms F3 §2.1 measured across
// the three real fixtures.

// fixtureBuilder assembles a PDF file byte-by-byte, recording each
// object's starting offset so xref sections can be built accurately.
type fixtureBuilder struct {
	buf     bytes.Buffer
	offsets map[int]int
}

func newFixtureBuilder() *fixtureBuilder {
	b := &fixtureBuilder{offsets: map[int]int{}}
	b.buf.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	return b
}

func (b *fixtureBuilder) offset() int { return b.buf.Len() }

// writeObj appends "N 0 obj <body> endobj".
func (b *fixtureBuilder) writeObj(num int, body string) {
	b.offsets[num] = b.buf.Len()
	fmt.Fprintf(&b.buf, "%d 0 obj\n%s\nendobj\n", num, body)
}

// writeStreamObj appends a direct (uncompressed) stream object.
func (b *fixtureBuilder) writeStreamObj(num int, dictInner string, data []byte) {
	b.offsets[num] = b.buf.Len()
	fmt.Fprintf(&b.buf, "%d 0 obj\n<< %s /Length %d >>\nstream\n", num, dictInner, len(data))
	b.buf.Write(data)
	b.buf.WriteString("\nendstream\nendobj\n")
}

// writeClassicXref appends a classic xref table covering object numbers
// 0..size-1 (all assumed in-file except 0, which is the standard free
// head entry) plus a trailer, and returns the xref section's own
// offset (for startxref).
func (b *fixtureBuilder) writeClassicXref(size, root int, extraTrailer string) int {
	start := b.offset()
	fmt.Fprintf(&b.buf, "xref\n0 %d\n", size)
	b.buf.WriteString("0000000000 65535 f \n")
	for i := 1; i < size; i++ {
		off := b.offsets[i]
		fmt.Fprintf(&b.buf, "%010d %05d n \n", off, 0)
	}
	fmt.Fprintf(&b.buf, "trailer\n<< /Size %d /Root %d 0 R%s >>\nstartxref\n%d\n%%%%EOF", size, root, extraTrailer, start)
	return start
}

// bytesOut returns the assembled file.
func (b *fixtureBuilder) bytesOut() []byte { return b.buf.Bytes() }

// classicFixtureBody is the shared page-tree content for both fixtures:
// a one-page document with a content stream, object numbers 1-4.
func writeClassicBody(b *fixtureBuilder) {
	b.writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	b.writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	b.writeObj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>")
	content := []byte("BT /F1 12 Tf 72 712 Td (Hello) Tj ET")
	b.writeStreamObj(4, "", content)
}

// buildClassicFixture returns a minimal, single-revision PDF using a
// classic xref table (the mechanism the Halcom fixture used, per F3
// §2.1's measurement table).
func buildClassicFixture() []byte {
	b := newFixtureBuilder()
	writeClassicBody(b)
	b.writeClassicXref(5, 1, "")
	return b.bytesOut()
}

// buildClassicFixtureWithDirectAcroForm returns a minimal, single-
// revision PDF whose catalog carries /AcroForm as a direct dictionary —
// not an indirect reference — with one pre-existing field, matching the
// shape measured directly in halcom.pdf, mup.pdf and posta.pdf (see
// docs/decisions.md): all three real fixtures embed /AcroForm inline in
// the catalog. This synthetic fixture exercises that exact shape without
// depending on the gitignored real fixtures being present.
func buildClassicFixtureWithDirectAcroForm() []byte {
	b := newFixtureBuilder()
	b.writeObj(1, "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] /DA (/Helv 0 Tf 0 g ) >> >>")
	b.writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	b.writeObj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>")
	content := []byte("BT /F1 12 Tf 72 712 Td (Hello) Tj ET")
	b.writeStreamObj(4, "", content)
	b.writeObj(5, "<< /FT /Tx /T (ExistingField) >>")
	b.writeClassicXref(6, 1, "")
	return b.bytesOut()
}

// buildStreamFixture returns a minimal, single-revision PDF using a
// cross-reference stream and an object stream (the mechanism the MUP
// and Pošta fixtures used). Objects 1-3 (Catalog, Pages, Page) are
// compressed inside object stream 6; object 4 (the page content stream)
// cannot be — PDF forbids storing streams inside object streams — so it
// stays a direct object; object 6 is the ObjStm itself; object 7 is the
// xref stream.
func buildStreamFixture() []byte {
	b := newFixtureBuilder()

	compressed := []struct {
		num  int
		body string
	}{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>"},
	}
	var header bytes.Buffer
	var body bytes.Buffer
	objStmIndex := map[int]int{}
	for i, o := range compressed {
		fmt.Fprintf(&header, "%d %d ", o.num, body.Len())
		body.WriteString(o.body)
		body.WriteString(" ")
		objStmIndex[o.num] = i
	}
	first := header.Len()
	var payload bytes.Buffer
	payload.Write(header.Bytes())
	payload.Write(body.Bytes())

	var compressedStream bytes.Buffer
	zw := zlib.NewWriter(&compressedStream)
	_, _ = zw.Write(payload.Bytes())
	_ = zw.Close()

	content := []byte("BT /F1 12 Tf 72 712 Td (Hello) Tj ET")
	b.writeStreamObj(4, "", content)

	b.offsets[6] = b.offset()
	fmt.Fprintf(&b.buf, "6 0 obj\n<< /Type /ObjStm /N %d /First %d /Filter /FlateDecode /Length %d >>\nstream\n",
		len(compressed), first, compressedStream.Len())
	b.buf.Write(compressedStream.Bytes())
	b.buf.WriteString("\nendstream\nendobj\n")

	// xref stream: object 7, size 8 (numbers 0-7), W = [1 4 2].
	const size = 8
	xrefStart := b.offset()
	var rows bytes.Buffer
	writeXrefRow := func(typ int, f2 int64, f3 int) {
		rows.WriteByte(byte(typ))
		rows.WriteByte(byte(f2 >> 24))
		rows.WriteByte(byte(f2 >> 16))
		rows.WriteByte(byte(f2 >> 8))
		rows.WriteByte(byte(f2))
		rows.WriteByte(byte(f3 >> 8))
		rows.WriteByte(byte(f3))
	}
	writeXrefRow(0, 0, 65535) // object 0: free
	for _, o := range compressed {
		writeXrefRow(2, 6, objStmIndex[o.num]) // objects 1-3: inside objstm 6
	}
	writeXrefRow(1, int64(b.offsets[4]), 0) // object 4: content stream
	writeXrefRow(0, 0, 0)                   // object 5: unused, free
	writeXrefRow(1, int64(b.offsets[6]), 0) // object 6: the ObjStm
	writeXrefRow(1, int64(xrefStart), 0)    // object 7: this xref stream itself

	fmt.Fprintf(&b.buf, "7 0 obj\n<< /Type /XRef /Size %d /W [1 4 2] /Root 1 0 R /Length %d >>\nstream\n",
		size, rows.Len())
	b.buf.Write(rows.Bytes())
	b.buf.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&b.buf, "startxref\n%d\n%%%%EOF", xrefStart)

	return b.bytesOut()
}

// buildMixedHistoryFixture returns a synthetic PDF whose revision history
// deliberately mirrors mup.pdf's and posta.pdf's ([[D-075]]): a classic
// xref-table base revision, then a revision that upgrades to a genuine
// cross-reference stream, then a further classic-table revision appended
// on top of that. The document's *last* revision — the one its final
// startxref resolves to — is a classic table, even though a
// cross-reference stream appears earlier in its /Prev chain. This is the
// exact shape that exposed [[D-075]]'s bug (mechanism detection picking
// up the stream from the middle revision instead of the classic table
// the document actually ends with), and building it explicitly is the
// only way the fix stays pinned in CI, since the real fixtures that
// exposed it are gitignored (see this file's header).
func buildMixedHistoryFixture() []byte {
	b := newFixtureBuilder()

	// Revision 0: classic base, objects 1-4.
	writeClassicBody(b)
	rev0 := b.writeClassicXref(5, 1, "")

	// Revision 1: upgrade to a genuine cross-reference stream (object 6),
	// adding object 5 and chaining /Prev back to the classic base.
	b.writeObj(5, "<< /Type /Marker /Rev (stream) >>")
	const xrefStreamNum = 6
	var rows bytes.Buffer
	writeXrefRow := func(off int64) {
		rows.WriteByte(1) // type 1: in file
		rows.WriteByte(byte(off >> 24))
		rows.WriteByte(byte(off >> 16))
		rows.WriteByte(byte(off >> 8))
		rows.WriteByte(byte(off))
		rows.WriteByte(0) // generation, high byte
		rows.WriteByte(0) // generation, low byte
	}
	rev1 := b.offset() // where "6 0 obj" begins, and this section's own startxref target
	writeXrefRow(int64(b.offsets[5]))
	writeXrefRow(int64(rev1))
	b.offsets[xrefStreamNum] = rev1
	fmt.Fprintf(&b.buf, "%d 0 obj\n<< /Type /XRef /Size %d /W [1 4 2] /Index [5 2] /Root 1 0 R /Prev %d /Length %d >>\nstream\n",
		xrefStreamNum, xrefStreamNum+1, rev0, rows.Len())
	b.buf.Write(rows.Bytes())
	b.buf.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&b.buf, "startxref\n%d\n%%%%EOF", rev1)

	// Revision 2: a plain classic table on top — the document's actual
	// last revision — adding object 7 and chaining /Prev to rev1.
	b.writeObj(7, "<< /Type /Marker /Rev (classic-on-top) >>")
	rev2 := b.offset()
	fmt.Fprintf(&b.buf, "xref\n7 1\n%010d %05d n \n", b.offsets[7], 0)
	fmt.Fprintf(&b.buf, "trailer\n<< /Size 8 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF", rev1, rev2)

	return b.bytesOut()
}
