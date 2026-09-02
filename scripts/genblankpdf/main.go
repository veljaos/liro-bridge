// Command genblankpdf writes testdata/pdfs/blank.pdf: a single blank A4
// page, no AcroForm, no existing signature.
//
// This is the smallest possible discriminator for the writer-level bugs
// Adobe Acrobat's raw parser is strict about (see docs/decisions.md,
// the entry recorded for this fixture): signing a document with no
// pre-existing form isolates this project's own emitted structures —
// the AcroForm it creates, the widget it adds, the strings it writes —
// from its incremental-update rewriting of someone else's. Every real
// fixture this project also tests against (testdata/pdfs/local/, not
// committed — see that directory's README) already carries its own
// AcroForm, /DA and /Lang, so a bug in code that only runs when this
// project creates those keys itself has nowhere else to show up.
//
// The file is generated, not hand-typed, so its bytes are reproducible
// from this program alone: run `go run ./scripts/genblankpdf` from the
// repository root to regenerate it.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// a4Width and a4Height are ISO 216 A4 in PDF points (1/72 inch),
// rounded to whole points the way PDF producers conventionally do —
// 595 x 842pt, not the fractional 595.28 x 841.89 the millimetre
// conversion gives exactly.
const (
	a4Width  = 595
	a4Height = 842
)

func main() {
	data := buildBlankPDF()
	const outPath = "testdata/pdfs/blank.pdf"
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "genblankpdf: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", outPath, len(data))
}

// buildBlankPDF hand-builds a minimal, classic-xref PDF: a Catalog, one
// Pages node, one blank Page (A4 MediaBox, empty Resources, no
// /Contents — a page needs none to be valid), and a trailer with a
// fixed, deterministic /ID so re-running this program byte-for-byte
// reproduces the committed fixture.
func buildBlankPDF() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	// A binary comment (PDF 32000-1 §7.5.2) tells naive readers this is
	// an 8-bit file, matching what real producers emit right after the
	// header — harmless bytes above 0x7F, never interpreted.
	buf.Write([]byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'})

	offsets := map[int]int{}
	write := func(num int, body string) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}

	write(1, "<< /Type /Catalog /Pages 2 0 R >>")
	write(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	write(3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << >> >>", a4Width, a4Height))

	xrefStart := buf.Len()
	buf.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&buf, "%010d %05d n \n", offsets[i], 0)
	}
	// /ID: two copies of the same 16-byte digest, as real producers do
	// for a freshly created (not yet updated) document (PDF 32000-1
	// §14.4). Derived from a fixed seed string with SHA-256 (truncated
	// to 16 bytes) rather than typed out as hex directly, so it is both
	// deterministic — this program's output matches the committed
	// fixture byte-for-byte on every run — and genuinely binary, the
	// same way a real producer's own digest-derived /ID is: unlike
	// readable text hex-decodes to, a hash's output bytes are not
	// themselves printable ASCII, which is what lets this project's PDF
	// writer's content-based hex-vs-literal choice (Task 2) reserve hex
	// for this value on its own, with no special-casing for the /ID key.
	digest := sha256.Sum256([]byte("liro-bridge-blank-fixture-v1"))
	id := hex.EncodeToString(digest[:16])
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R /ID [<%s> <%s>] >>\nstartxref\n%d\n%%%%EOF", id, id, xrefStart)
	return buf.Bytes()
}
