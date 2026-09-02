package pades

import (
	"context"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// allFieldNames walks data's /AcroForm /Fields tree — the top-level
// array plus every /Kids subtree beneath it, since a field name can be
// hierarchical (Task 1) — and returns every /T value found, in the
// order encountered. Deliberately independent of
// internal/pades/pdf.uniqueFieldName's own identically-shaped walk (an
// unexported helper this test file cannot call anyway): a bug shared
// between the code that names a field and the code that checks the name
// would pass both ways.
func allFieldNames(t *testing.T, data []byte) []string {
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
	acroForm, ok := doc.ResolveDict(catalog.Get(pdf.Name("AcroForm")))
	if !ok {
		t.Fatal("/AcroForm does not resolve to a dictionary")
	}
	fields, _ := doc.Resolve(acroForm.Get(pdf.Name("Fields"))).(pdf.Array)

	var names []string
	visited := map[int]bool{}
	var walk func(pdf.Array)
	walk = func(arr pdf.Array) {
		for _, item := range arr {
			if ref, ok := item.(pdf.Reference); ok {
				if visited[ref.Num] {
					continue
				}
				visited[ref.Num] = true
			}
			dict, ok := doc.ResolveDict(item)
			if !ok {
				continue
			}
			if tv, ok := dict.Get(pdf.Name("T")).(pdf.String); ok {
				names = append(names, string(tv))
			}
			if kids, ok := doc.Resolve(dict.Get(pdf.Name("Kids"))).(pdf.Array); ok {
				walk(kids)
			}
		}
	}
	walk(fields)
	return names
}

// TestRealFixturesSignatureFieldNameIsUnique is Task 1's closing test.
// Adobe reports "At least one signature is invalid" and an empty
// Signature Panel for any real document this project signs, because the
// new signature field it adds is always named "Signature1" — the same
// name a real MUP-signed PDF's own original CAdES signature field
// already carries. Two fields sharing a /T are, per the PDF spec, one
// field with two conflicting /V values to any reader that assembles the
// AcroForm tree; the cryptography is unaffected, which is exactly why no
// existing test (all of which check only cryptographic properties)
// caught it. This test signs each real fixture and asserts every /T in
// the resulting /Fields tree is distinct.
func TestRealFixturesSignatureFieldNameIsUnique(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			in := realFixture(t, name)

			sess := newFakeSession(t)
			result, err := SignDocument(context.Background(), in, sess, Options{})
			if err != nil {
				t.Fatalf("SignDocument: %v", err)
			}

			names := allFieldNames(t, result.Bytes)
			if len(names) == 0 {
				t.Fatal("no /T values found in the signed document's /Fields tree")
			}
			seen := make(map[string]int, len(names))
			for _, n := range names {
				seen[n]++
			}
			for n, count := range seen {
				if count > 1 {
					t.Errorf("field name %q appears %d times in /Fields — /T must be unique in the AcroForm field tree, or Acrobat sees one field with conflicting /V values and reports the signature invalid", n, count)
				}
			}
		})
	}
}
