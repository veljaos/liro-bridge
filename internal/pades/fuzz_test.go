package pades

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/pades/verify"
)

// SPEC §16.3 calls signing an already-signed document "the single most
// important test in the project". Every test that does it signs a
// document this project or a real CA produced. This target signs
// whatever the fuzzer hands it: the whole pipeline — parse, page-tree
// walk, catalog and AcroForm rewrite, /Contents reservation, /ByteRange
// arithmetic, CMS construction and injection — over an existing document
// that may be malformed in any way at all.
//
// It runs two orders of magnitude slower than
// pdf.FuzzIncrementalUpdate, because every execution does a real
// RSA-2048 signature. That is the point of having both: the pdf-level
// target explores the writer, this one proves that what comes out the
// far end of the whole pipeline still verifies.
//
// No TSA is configured and no network is touched: opts.TSA is nil, which
// SignDocument treats as B-B unconditionally, and the certificate chain
// is the one the session carries.

const fuzzSeedDirEnv = "LIRO_FUZZ_SEED_DIR"

const maxSeedBytes = 128 * 1024

// FuzzSignMalformedDocument signs an arbitrary document and, when
// signing succeeds, requires the output to satisfy the three properties
// that make an incremental update a signature rather than a rewrite: the
// input is still a literal prefix, the new signature verifies against
// its own /ByteRange, and every signature the document already had is
// still there and still says what it said.
func FuzzSignMalformedDocument(f *testing.F) {
	f.Add(goldenMinimalPDF())
	f.Add([]byte("%PDF-1.7\n%%EOF\n"))
	f.Add([]byte(""))

	for _, p := range []string{
		"testdata/pdfs/blank.pdf",
		"testdata/golden/minimal-signed-bb.pdf",
	} {
		if data, err := os.ReadFile(filepath.Join("../..", p)); err == nil {
			f.Add(data)
		}
	}
	dirs := []string{"../../testdata/pdfs/local"}
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
			f.Add(data)
		}
	}

	session := newDeterministicSession(f)
	fixedNow := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Signatures already present before this one, so the check
		// below is about what signing did rather than about what the
		// input happened to contain.
		before, _ := verify.FindSignatures(data)

		res, err := SignDocument(context.Background(), data, session, Options{
			RequestedLevel:    LevelBT,
			OnTSAFailureAbort: false,
			Now:               fixedNow,
			ReservedBytes:     4096,
		})
		if err != nil {
			return
		}
		if res == nil || len(res.Bytes) == 0 {
			t.Fatal("SignDocument returned no error and no document")
		}

		if len(res.Bytes) < len(data) || string(res.Bytes[:len(data)]) != string(data) {
			t.Fatalf("the input's %d bytes are no longer a prefix of the %d-byte signed output",
				len(data), len(res.Bytes))
		}

		after, err := verify.FindSignatures(res.Bytes)
		if err != nil {
			t.Fatalf("the signed output has no readable signature slots: %v", err)
		}
		if len(after) <= len(before) {
			t.Fatalf("signing added no signature slot: %d before, %d after", len(before), len(after))
		}

		// The slot this call added is the last one, and it must verify
		// against its own /ByteRange — the property SPEC §12.1 says a
		// bug here breaks invisibly.
		last := after[len(after)-1]
		r := verify.VerifySignature(res.Bytes, last)
		if r == nil {
			t.Fatal("VerifySignature returned nil for a signature this call just made")
		}
		if !r.ByteRangeDigestOK || !r.SignatureOK {
			t.Fatalf("a signature this call just made does not verify: byteRangeDigestOK=%v signatureOK=%v errors=%v",
				r.ByteRangeDigestOK, r.SignatureOK, r.Errors)
		}
	})
}
