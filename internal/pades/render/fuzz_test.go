package render

import (
	"os"
	"path/filepath"
	"testing"
)

// The renderer is the newest and least-exercised code that reads foreign
// input in this project: a content-stream interpreter, a scanline
// rasteriser, colour spaces, PDF functions, image XObjects and their
// masks, a TrueType glyph reader and a CCITT decoder — none of which had
// ever been fuzzed. Every byte it looks at comes out of a document
// somebody else produced, so SPEC §16.5's rule for the parser ("never
// panic, never allocate unboundedly, never loop forever") applies to it
// with exactly the same force.
//
// Seeds come from three places, in order of how much they are worth:
// documents built here (small, committed, always available), the
// project's own committed fixtures, and — when they are present, which
// is never in CI — the real signed documents in testdata/pdfs/local and
// whatever FTEST corpus LIRO_FUZZ_SEED_DIR names.

// fuzzSeedDirEnv names a directory of PDFs to seed a fuzz target with,
// so a local run can start from the FTEST corpus
// (scripts/gencorpus/gencorpus.py) without those documents being
// committed. Unset — which is CI — means the built-in seeds only.
const fuzzSeedDirEnv = "LIRO_FUZZ_SEED_DIR"

// maxSeedBytes keeps a seed small enough to be worth mutating. A 280 KB
// document is a fine thing to render once and a poor thing to flip bits
// in: libFuzzer-style mutation spends its budget in the middle of a
// content stream that no longer has a valid xref pointing at it.
const maxSeedBytes = 256 * 1024

// addFileSeeds adds every *.pdf under dir that is small enough, ignoring
// a directory that is not there.
func addFileSeeds(f *testing.F, dir string, add func([]byte)) {
	names, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
	if err != nil {
		return
	}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil || len(data) > maxSeedBytes {
			continue
		}
		add(data)
	}
}

// seedPDFs adds the built-in synthetic documents, the committed
// fixtures, the real ones when present, and the FTEST corpus when
// LIRO_FUZZ_SEED_DIR points at it.
func seedPDFs(f *testing.F, add func([]byte)) {
	f.Helper()

	add(buildTestPDF([]testPage{{
		box:     [4]float64{0, 0, 200, 200},
		content: "0 0 0 rg 20 20 100 100 re f BT /F1 12 Tf 30 150 Td (hello) Tj ET",
	}}))
	add(buildTestPDF([]testPage{
		{box: [4]float64{0, 0, 200, 300}, rotate: 90, content: "1 0 0 RG 5 w 10 10 m 190 290 l S"},
		{box: [4]float64{0, 0, 300, 200}, rotate: 270, content: "q 2 0 0 2 10 10 cm 0 0 50 50 re f Q"},
	}))
	add(buildTestPDF([]testPage{{
		box:     [4]float64{0, 0, 150, 150},
		content: "q 100 0 0 60 20 40 cm BI /W 2 /H 2 /CS /RGB /BPC 8 /F /AHx ID\nff0000 00ff00\n0000ff ffffff>\nEI Q",
	}}))
	add([]byte("%PDF-1.7\n"))
	add([]byte(""))
	add([]byte("not a pdf"))

	for _, p := range []string{
		"../../../testdata/pdfs/blank.pdf",
		"../../../testdata/golden/minimal-signed-bb.pdf",
	} {
		if data, err := os.ReadFile(p); err == nil && len(data) <= maxSeedBytes {
			add(data)
		}
	}
	addFileSeeds(f, "../../../testdata/pdfs/local", add)
	if dir := os.Getenv(fuzzSeedDirEnv); dir != "" {
		addFileSeeds(f, dir, add)
	}
}

// fuzzRenderPixelBudget caps how big a page a fuzz execution will
// actually paint. MaxRenderPixels (40 M, 160 MB of RGBA) is the
// product's own guard and has its own test; a fuzzer that reaches it on
// every other execution spends its whole budget in memset rather than in
// the interpreter this target exists to exercise.
const fuzzRenderPixelBudget = 1 << 21 // 2 M pixels, 8 MB

// FuzzRenderPage renders one page of an arbitrary document at an
// arbitrary scale. Opening, page-box resolution, the content-stream
// interpreter, every colour space and function, images and their masks,
// glyph outlines and annotation appearance streams are all downstream of
// this one call.
func FuzzRenderPage(f *testing.F) {
	add := func(b []byte) { f.Add(b, uint16(0), uint8(15)) }
	seedPDFs(f, add)
	f.Add(buildTestPDF([]testPage{{box: [4]float64{0, 0, 100, 100}, content: "0 0 50 50 re f"}}), uint16(1), uint8(1))

	f.Fuzz(func(t *testing.T, data []byte, page uint16, scaleTenths uint8) {
		doc, err := Open(data)
		if err != nil || doc == nil {
			return
		}
		n := doc.PageCount()
		if n <= 0 {
			return
		}
		num := int(page)%n + 1

		info, err := doc.PageInfo(num)
		if err != nil {
			return
		}
		// 0.1 to 25.6 px/pt, so both a thumbnail and the placement
		// window's own 400 per cent step are reachable.
		scale := float64(scaleTenths%128+1) / 10
		if info.WidthPt*scale*info.HeightPt*scale > fuzzRenderPixelBudget {
			scale = 0.1
		}
		if info.WidthPt*scale*info.HeightPt*scale > fuzzRenderPixelBudget {
			return
		}

		res, err := doc.RenderPage(num, scale)
		if err != nil {
			return
		}
		if res.Image == nil {
			t.Fatalf("RenderPage returned no image and no error for page %d at %.2f px/pt", num, scale)
		}
		b := res.Image.Bounds()
		if b.Dx() < 1 || b.Dy() < 1 {
			t.Fatalf("RenderPage produced a %dx%d image", b.Dx(), b.Dy())
		}
	})
}
