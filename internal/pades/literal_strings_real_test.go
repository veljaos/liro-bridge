package pades

import (
	"bytes"
	"context"
	"regexp"
	"testing"
)

// containsLiteralString reports whether data contains "/key (value)" —
// key's value written as a literal PDF string, not a hex one. value is
// used verbatim (every value this file checks — a field name, a
// default-appearance operator string, a two-letter language tag — is
// plain ASCII with no '(' ')' or '\\' of its own, so it needs no
// escaping to appear as a literal PDF byte sequence).
func containsLiteralString(t *testing.T, data []byte, key, value string) bool {
	t.Helper()
	return bytes.Contains(data, []byte("/"+key+" ("+value+")"))
}

// containsHexString reports whether data contains "/key <...>" — key's
// value written as a hex string, or "/key [<...>" for an array whose
// first element is one (as /ID's two-element array is).
func containsHexString(t *testing.T, data []byte, key string) bool {
	t.Helper()
	re := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + ` \[?<[0-9A-Fa-f]*>`)
	return re.Match(data)
}

// dumpObject returns the exact "N 0 obj\n...\nendobj" bytes for the
// last object in data whose body contains needle — used only to print
// an object's real bytes for inspection (Task 2's reporting
// requirement), not to assert anything itself. The *last* match matters
// when data is a whole signed document: an incremental update can
// shadow an object number a real document's own pre-existing revision
// already used, and the later (this project's own) copy is the one that
// actually governs how a reader assembles the document.
func dumpObject(data []byte, needle string) string {
	re := regexp.MustCompile(`(?s)\d+ 0 obj\n.*?\nendobj`)
	matches := re.FindAllString(string(data), -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if bytes.Contains([]byte(matches[i]), []byte(needle)) {
			return matches[i]
		}
	}
	return "(not found)"
}

// TestRealFixturesFormStringsAreLiteral is Task 2's verification against
// a genuine document: mup.pdf's own AcroForm already carries /DA (a
// content-stream fragment Acrobat executes when building field
// appearances) and the catalog carries /Lang, both parsed from the real
// document and copied forward unmodified by BuildPlaceholder (F3 §3.3:
// "copy each object being modified in full"). Before this fix, copying
// them forward meant re-serialising them — and this project's writer
// hex-encoded every string it serialised, regardless of where it came
// from. This test signs the real document and confirms /DA, /Lang and
// this project's own /T all come out as literal strings in the newly
// appended revision — scoped to result.Bytes[len(in):] specifically,
// since the *original* document's own bytes (still present, unmodified,
// as the file's prefix — F3 §3.1) already write these literally on
// their own, which would let a check against the whole file pass even
// if this project's own rewriting still hex-encoded them — and logs the
// exact AcroForm and widget dictionary bytes so they can be diffed
// against the original document's own bytes by hand.
func TestRealFixturesFormStringsAreLiteral(t *testing.T) {
	for _, name := range realFixtureNames {
		t.Run(name, func(t *testing.T) {
			in := realFixture(t, name)

			sess := newFakeSession(t)
			result, err := SignDocument(context.Background(), in, sess, Options{})
			if err != nil {
				t.Fatalf("SignDocument: %v", err)
			}
			if !bytes.HasPrefix(result.Bytes, in) {
				t.Fatal("signing did not append to the real document's exact original bytes")
			}
			newRevision := result.Bytes[len(in):]

			names := allFieldNames(t, result.Bytes)
			if len(names) == 0 {
				t.Fatal("no /T values found in the signed document's /Fields tree")
			}
			newField := names[len(names)-1]

			t.Logf("new widget dictionary:\n%s", dumpObject(newRevision, "/FT /Sig"))
			t.Logf("new AcroForm dictionary:\n%s", dumpObject(newRevision, "/SigFlags"))

			if !containsLiteralString(t, newRevision, "T", newField) {
				t.Errorf("/T %q is not written as a literal string in the new revision", newField)
			}
			if containsHexString(t, newRevision, "T") {
				t.Errorf("/T is hex-encoded in the new revision")
			}

			if bytes.Contains(in, []byte("/DA (")) {
				if !bytes.Contains(newRevision, []byte("/DA (")) {
					t.Errorf("/DA is not written as a literal string in the new revision")
				}
				if containsHexString(t, newRevision, "DA") {
					t.Errorf("/DA is hex-encoded in the new revision — Acrobat executes it as a content-stream fragment")
				}
			}

			if bytes.Contains(in, []byte("/Lang(en)")) || bytes.Contains(in, []byte("/Lang (en)")) {
				if !containsLiteralString(t, newRevision, "Lang", "en") {
					t.Errorf("/Lang is not written as a literal string in the new revision")
				}
				if containsHexString(t, newRevision, "Lang") {
					t.Errorf("/Lang is hex-encoded in the new revision")
				}
			}

			// The catalog's own new copy carries an /ID only when this
			// project's incremental-update writer emits one (F3 §3.2's
			// trailer, updateTrailer) — genuinely binary content, which
			// must stay hex even though /DA, /T and /Lang no longer do.
			if containsHexString(t, newRevision, "ID") == false && bytes.Contains(newRevision, []byte("/ID")) {
				t.Errorf("/ID is present but not hex-encoded in the new revision")
			}
		})
	}
}
