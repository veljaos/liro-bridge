package pdf

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// FuzzParse (fuzz_test.go) proves the parser survives foreign input.
// This target asks the harder question: a document that parsed is then
// *written to*. BuildPlaceholder rewrites the catalog, extends or
// creates the AcroForm, adds a widget to a page it found by walking the
// page tree, reserves /Contents, computes /ByteRange and appends a whole
// revision whose cross-reference mechanism it chose from the input's own
// last revision (D-075). Every one of those steps reads a value a
// malformed document supplied.
//
// This is the incremental-update half of "a malformed existing document
// that is then signed"; internal/pades.FuzzSignMalformedDocument is the
// other half, with the CMS and the RSA signature on top. The split is
// deliberate: this one runs two orders of magnitude more executions per
// second, so it is what actually explores the writer.

const fuzzSeedDirEnv = "LIRO_FUZZ_SEED_DIR"

const maxSeedBytes = 256 * 1024

// seedDocuments adds the synthetic fixtures this package already builds,
// the committed ones, the real signed documents when present, and
// whatever FTEST corpus LIRO_FUZZ_SEED_DIR names.
func seedDocuments(f *testing.F, add func([]byte)) {
	f.Helper()
	add(buildClassicFixture())
	add(buildStreamFixture())
	add(buildMixedHistoryFixture())
	add(buildClassicFixtureWithDirectAcroForm())
	add([]byte("%PDF-1.4\n"))
	add([]byte(""))

	for _, p := range []string{
		"../../../testdata/pdfs/blank.pdf",
		"../../../testdata/golden/minimal-signed-bb.pdf",
	} {
		if data, err := os.ReadFile(p); err == nil && len(data) <= maxSeedBytes {
			add(data)
		}
	}
	dirs := []string{"../../../testdata/pdfs/local"}
	if d := os.Getenv(fuzzSeedDirEnv); d != "" {
		dirs = append(dirs, d)
	}
	for _, dir := range dirs {
		names, _ := filepath.Glob(filepath.Join(dir, "*.pdf"))
		for _, name := range names {
			data, err := os.ReadFile(name)
			if err != nil || len(data) > maxSeedBytes {
				continue
			}
			add(data)
		}
	}
}

// FuzzIncrementalUpdate parses an arbitrary document and appends a
// signature placeholder revision to it, then parses the result and walks
// it. The second parse is the point: an incremental update this project
// wrote that its own parser cannot read again is a document no reader
// can read either, and nothing else in the suite checks that against
// input this project did not construct.
func FuzzIncrementalUpdate(f *testing.F) {
	seedDocuments(f, func(b []byte) { f.Add(b, int32(0), false) })
	f.Add(buildClassicFixture(), int32(-1), true)
	f.Add(buildStreamFixture(), int32(7), false)

	f.Fuzz(func(t *testing.T, data []byte, page int32, visible bool) {
		doc, err := Parse(data)
		if err != nil || doc == nil {
			return
		}

		opts := PlaceholderOptions{
			SubFilter:     Name("ETSI.CAdES.detached"),
			SigningDate:   time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
			PageNumber:    int(page % 64),
			ReservedBytes: 512, // small: the reservation's size is not what is being explored
		}
		if visible {
			opts.Appearance = &Appearance{Rect: [4]float64{10, 10, 200, 60}}
		}

		ph, err := BuildPlaceholder(doc, opts)
		if err != nil {
			return
		}
		if ph == nil {
			t.Fatal("BuildPlaceholder returned no placeholder and no error")
		}

		out := ph.Bytes
		if len(out) < len(data) || string(out[:len(data)]) != string(data) {
			t.Fatalf("the input's %d bytes are no longer a prefix of the %d-byte output: an incremental update must never rewrite what was there",
				len(data), len(out))
		}

		// /ByteRange must select the whole file except the /Contents
		// hex span, whatever the input looked like.
		br := ph.ByteRange
		if br[0] != 0 || br[1] < 0 || br[2] < br[1] || br[3] < 0 || br[2]+br[3] != int64(len(out)) {
			t.Fatalf("/ByteRange %v does not cover the %d-byte output", br, len(out))
		}

		again, err := Parse(out)
		if err != nil {
			t.Fatalf("this project's own incremental update produced a document it cannot parse: %v", err)
		}
		for num := range again.xref {
			_ = again.Get(num)
		}
	})
}
