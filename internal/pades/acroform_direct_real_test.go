package pades

import (
	"bytes"
	"context"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// acroFormDirect parses data, resolves the catalog, and reports whether
// its /AcroForm entry is a direct dictionary (true, dict returned
// directly) or an indirect reference (false, dict resolved through it).
// Fails the test outright if /AcroForm is present but neither shape, or
// absent entirely.
func acroFormDirect(t *testing.T, data []byte) (direct bool, dict pdf.Dict) {
	t.Helper()
	doc, err := pdf.Parse(data)
	if err != nil {
		t.Fatalf("pdf.Parse: %v", err)
	}
	rootRef, ok := doc.Trailer().Get(pdf.Name("Root")).(pdf.Reference)
	if !ok {
		t.Fatal("trailer /Root is not an indirect reference")
	}
	catalog, ok := doc.ResolveDict(rootRef)
	if !ok {
		t.Fatal("/Root does not resolve to a dictionary")
	}
	switch af := catalog.Get(pdf.Name("AcroForm")).(type) {
	case pdf.Dict:
		return true, af
	case pdf.Reference:
		resolved, ok := doc.ResolveDict(af)
		if !ok {
			t.Fatal("catalog /AcroForm reference does not resolve to a dictionary")
		}
		return false, resolved
	default:
		t.Fatalf("catalog /AcroForm is neither a direct dictionary nor an indirect reference: %#v", af)
		return false, nil
	}
}

// TestRealFixturesAcroFormDirectnessMatchesInput is the finding this fix
// addresses (see docs/decisions.md): Adobe Acrobat showed an empty
// Signature Panel for every real signed document this project produced,
// while accepting a signed blank page. The exact pattern, measured
// directly:
//
//	halcom.pdf   original /AcroForm = DIRECT dictionary in the catalog
//	mup.pdf      original /AcroForm = DIRECT dictionary in the catalog
//	posta.pdf    original /AcroForm = DIRECT dictionary in the catalog
//	blank.pdf    no /AcroForm at all
//
// All three real documents carried /AcroForm inline; this project's
// incremental update used to lift it out into a new indirect object and
// repoint the catalog at it, changing /AcroForm's identity between
// revisions — exactly what Acrobat's incremental-update analysis reads
// as the whole form being replaced, rather than one field being added.
// blank.pdf has nothing to lift and was unaffected, which is why it was
// the only one of the four Acrobat accepted.
//
// This test signs all three real fixtures and asserts direct/indirect
// structure is preserved; TestSignBlankFixtureAcroFormIsIndirect (in
// blank_test.go) covers the fourth case.
func TestRealFixturesAcroFormDirectnessMatchesInput(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			in := realFixture(t, name)

			inDirect, inDict := acroFormDirect(t, in)
			if !inDirect {
				t.Fatalf("%s: input /AcroForm is not a direct dictionary — this test's premise (measured directly against the real fixtures) does not hold for this file; investigate before trusting the rest of this test", name)
			}

			sess := newFakeSession(t)
			result, err := SignDocument(context.Background(), in, sess, Options{})
			if err != nil {
				t.Fatalf("SignDocument: %v", err)
			}
			if !bytes.HasPrefix(result.Bytes, in) {
				t.Fatal("signing did not append to the real document's exact original bytes — the original must remain a literal prefix")
			}

			outDirect, outDict := acroFormDirect(t, result.Bytes)
			if outDirect != inDirect {
				t.Fatalf("input /AcroForm direct=%v, output /AcroForm direct=%v — an incremental update must not change whether /AcroForm is direct or indirect", inDirect, outDirect)
			}

			inFields, _ := inDict.Get(pdf.Name("Fields")).(pdf.Array)
			outFields, _ := outDict.Get(pdf.Name("Fields")).(pdf.Array)
			if len(outFields) != len(inFields)+1 {
				t.Fatalf("len(/Fields) = %d, want %d (the original %d field(s) plus the one new signature field)", len(outFields), len(inFields)+1, len(inFields))
			}

			// The new revision's own bytes — not merely the resolved
			// final state — must carry /AcroForm inline in the rewritten
			// catalog. Scoped to the newly appended bytes, not the whole
			// file: the file's unmodified original prefix is not proof
			// of anything this project's own writer just did.
			newRevision := result.Bytes[len(in):]
			if !bytes.Contains(newRevision, []byte("/AcroForm <<")) {
				t.Errorf("new revision does not contain an inline /AcroForm dictionary")
			}
			if bytes.Contains(newRevision, []byte("/Type /Catalog")) {
				// only when this file's own catalog carries /Type
				// /Catalog explicitly — some producers omit it — verify
				// the catalog object itself is the one carrying the
				// inline AcroForm, not some unrelated dictionary.
				catalogObj := dumpObject(newRevision, "/Type /Catalog")
				if !bytes.Contains([]byte(catalogObj), []byte("/AcroForm <<")) {
					t.Errorf("the rewritten catalog object does not itself contain the inline /AcroForm dictionary:\n%s", catalogObj)
				}
			}

			after := countFullyVerifyingSignatures(t, result.Bytes)
			before := countFullyVerifyingSignatures(t, in)
			if after != before+1 {
				t.Fatalf("%d signatures fully verify after signing, want %d (the original CA's signature plus this project's new one)", after, before+1)
			}

			t.Logf("%s: new revision's catalog object:\n%s", name, dumpObject(newRevision, "/AcroForm <<"))
		})
	}
}
