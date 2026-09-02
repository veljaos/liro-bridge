package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// realPDFsDir holds real signed PDFs extracted from actual Serbian
// qualified signatures — never committed, because they carry personal
// data (SPEC §6.7/§11.6). See testdata/pdfs/local/README.md. These tests
// exist for the same reason internal/trust/classify's real-certificate
// tests do (see that package's classify_real_test.go): synthetic
// fixtures (fixtures_test.go, used by parser_test.go/incremental_test.go)
// encode the same understanding of the PDF producers' quirks that this
// parser does, so a shared wrong assumption between the two still
// produces a passing test. Only a real document from a real issuer can
// contradict that assumption.
const realPDFsDir = "../../../testdata/pdfs/local"

// realFixtureNames lists the three real signed PDFs this project's real-
// fixture tests are built against (F3 §2.1, one per Serbian CA).
var realFixtureNames = []string{"halcom.pdf", "mup.pdf", "posta.pdf"}

// realFixture reads name from realPDFsDir, skipping the calling test with
// a clear message when the file is absent — the condition that keeps
// `go test ./...` green with no real documents on hand.
func realFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(realPDFsDir, name))
	if os.IsNotExist(err) {
		t.Skipf("%s not found: no real PDF fixtures available for this test — see %s/README.md", name, realPDFsDir)
	}
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return data
}

// TestRealFixturesParseCompletely is F3 §2.5's first real-fixture
// requirement: parse all three fixtures, enumerate every object, resolve
// every indirect reference — including objects compressed inside object
// streams — across the full /Prev chain.
func TestRealFixturesParseCompletely(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			data := realFixture(t, name)
			doc, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if doc.Trailer().Get(Name("Root")) == nil {
				t.Fatal("merged trailer has no /Root")
			}
			root, ok := doc.ResolveDict(doc.Trailer().Get(Name("Root")))
			if !ok {
				t.Fatal("/Root did not resolve to a dictionary")
			}
			if root.Get(Name("Pages")) == nil {
				t.Fatal("/Root has no /Pages")
			}

			var total, fromStream int
			for num, entry := range doc.xref {
				if entry.typ == xrefFree {
					continue
				}
				total++
				if entry.typ == xrefInStream {
					fromStream++
				}
				if doc.Get(num) == nil {
					t.Errorf("object %d (xref type %v) did not resolve", num, entry.typ)
				}
			}
			if total == 0 {
				t.Fatal("merged xref table is empty")
			}
			t.Logf("%s: %d objects resolved (%d compressed inside object streams)", name, total, fromStream)
		})
	}
}

// xrefMechanism is an independent measurement of which cross-reference
// mechanisms a document's /Prev chain actually uses, walked directly
// here rather than read off Document's own merged state — the same
// independence discipline internal/pades/verify applies to signature
// verification (F3 §8): a bug in Document's own bookkeeping should not
// also hide from the test that is supposed to catch it.
type xrefMechanism struct {
	classicSections     int
	xrefStreamSections  int
	objectStreamNumbers map[int64]bool
	lastIsStream        bool // mechanism of the section startxref itself points at
}

// measureXrefMechanism follows startxref and /Prev (and /XRefStm for a
// hybrid classic+stream section) exactly as parseXrefChain does, but
// tallies what kind of section it finds at each link instead of just
// merging entries — this is the "measured, not assumed" cross-reference
// mechanism inventory SPEC §0 and F3 §2.1 call for.
func measureXrefMechanism(t *testing.T, data []byte) xrefMechanism {
	t.Helper()
	m := xrefMechanism{objectStreamNumbers: map[int64]bool{}}

	startxref, ok := findStartxref(data)
	if !ok {
		t.Fatal("no startxref found")
	}
	visited := map[int64]bool{}
	offset := startxref
	first := true
	for offset != 0 && !visited[offset] {
		visited[offset] = true
		p := newParser(data, nil)
		p.pos = int(offset)
		p.skipWhitespaceAndComments()
		isClassic := p.peekKeyword("xref")
		if first {
			m.lastIsStream = !isClassic
			first = false
		}

		entries, _, prev, xrefStm, err := parseXrefSectionAt(data, offset)
		if err != nil {
			t.Fatalf("section at %d: %v", offset, err)
		}
		if isClassic {
			m.classicSections++
		} else {
			m.xrefStreamSections++
		}
		for _, e := range entries {
			if e.typ == xrefInStream {
				m.objectStreamNumbers[e.offset] = true
			}
		}

		if xrefStm != 0 && !visited[xrefStm] {
			visited[xrefStm] = true
			hEntries, _, _, _, err := parseXrefSectionAt(data, xrefStm)
			if err != nil {
				t.Fatalf("hybrid /XRefStm section at %d: %v", xrefStm, err)
			}
			m.xrefStreamSections++
			for _, e := range hEntries {
				if e.typ == xrefInStream {
					m.objectStreamNumbers[e.offset] = true
				}
			}
		}
		offset = prev
	}
	return m
}

// TestRealFixturesCrossReferenceMechanismMatchesMeasurement asserts the
// cross-reference mechanism this parser actually detects in each real
// fixture's /Prev chain matches direct measurement of the files
// themselves.
//
// This corrects testdata/pdfs/local/README.md's structural description
// (also cited by [[D-039]]): halcom.pdf uses three classic xref-table
// revisions and never a cross-reference stream or object stream anywhere
// in its chain. mup.pdf and posta.pdf are not "purely stream-based" —
// each is a classic-table base revision, one revision that upgrades to a
// genuine cross-reference stream (reached via a backward-compatible
// hybrid /XRefStm pointer from a same-revision classic "xref 0 0" stub,
// PDF 32000-1 §7.5.8.4), and two further plain classic-table revisions
// appended on top of that. Only mup's and posta's one stream revision
// carries object streams: four distinct ones for mup, one for posta —
// this part of the original measurement was correct. See docs/decisions.md
// for the full account, including a parser bug this measurement exposed.
//
// All three fixtures' *last* revision — the one their final startxref
// resolves to — is a classic table, mup's and posta's mixed history
// notwithstanding ([[D-075]]). UsesXrefStreams() must track that last
// revision, not whether a stream appears anywhere in the /Prev chain.
func TestRealFixturesCrossReferenceMechanismMatchesMeasurement(t *testing.T) {
	cases := []struct {
		name              string
		wantClassic       int
		wantXrefStreams   int
		wantObjectStreams int
		wantLastIsStream  bool
	}{
		{"halcom.pdf", 3, 0, 0, false},
		{"mup.pdf", 4, 1, 4, false},
		{"posta.pdf", 4, 1, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := realFixture(t, c.name)
			m := measureXrefMechanism(t, data)
			if m.classicSections != c.wantClassic {
				t.Errorf("classic xref sections = %d, want %d", m.classicSections, c.wantClassic)
			}
			if m.xrefStreamSections != c.wantXrefStreams {
				t.Errorf("cross-reference stream sections = %d, want %d", m.xrefStreamSections, c.wantXrefStreams)
			}
			if len(m.objectStreamNumbers) != c.wantObjectStreams {
				t.Errorf("distinct object streams referenced = %d, want %d", len(m.objectStreamNumbers), c.wantObjectStreams)
			}
			if m.lastIsStream != c.wantLastIsStream {
				t.Errorf("last revision is a stream = %v, want %v", m.lastIsStream, c.wantLastIsStream)
			}

			doc, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if doc.UsesXrefStreams() != c.wantLastIsStream {
				t.Errorf("UsesXrefStreams() = %v, want %v (the last revision's mechanism)", doc.UsesXrefStreams(), c.wantLastIsStream)
			}
		})
	}
}

// TestRealFixturesIncrementalUpdatePreservesOriginalBytes is F3 §3.4's
// most direct test, applied to all three real fixtures rather than only
// the synthetic ones incremental_test.go already covers: appending a
// trivial revision must leave the original bytes byte-for-byte
// untouched, since that is what lets the documents' own existing
// signatures (verified separately in internal/pades/verify's real-
// fixture tests) survive our incremental update.
//
// It also pins [[D-075]]'s fix directly: the mechanism of the newly
// appended revision must match the mechanism of the *input's* last
// revision (independently measured, not read back through
// Document.UsesXrefStreams() — the code under test). The check is scoped
// to appendedStartxref, which by construction lies in the bytes this
// update wrote, never in the untouched prefix — the same scoping
// discipline [[D-072]] and [[D-074]] already record, since a check that
// tolerated a match anywhere in the whole file would also pass with the
// bug still present for mup.pdf and posta.pdf.
func TestRealFixturesIncrementalUpdatePreservesOriginalBytes(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			in := realFixture(t, name)
			wantStream := measureXrefMechanism(t, in).lastIsStream

			doc, err := Parse(in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			u := NewUpdate(doc)
			marker := u.NewObjectNumber()
			u.Set(marker, Dict{Name("Type"): Name("Marker")})
			out, err := u.Apply()
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if !bytes.HasPrefix(out, in) {
				t.Fatal("output is not a byte-for-byte prefix extension of the real document")
			}
			if _, err := Parse(out); err != nil {
				t.Fatalf("re-parsing the updated document: %v", err)
			}

			appendedStartxref, ok := findStartxref(out)
			if !ok {
				t.Fatal("no startxref in the updated document")
			}
			if appendedStartxref < int64(len(in)) {
				t.Fatalf("appended startxref %d falls inside the original %d bytes, not the newly written revision", appendedStartxref, len(in))
			}
			gotStream, ok := xrefMechanismAtStartxref(out)
			if !ok {
				t.Fatal("could not resolve the appended revision's own startxref")
			}
			if gotStream != wantStream {
				t.Errorf("appended revision uses a stream = %v, want %v (the input's last revision, measured independently)", gotStream, wantStream)
			}
		})
	}
}
