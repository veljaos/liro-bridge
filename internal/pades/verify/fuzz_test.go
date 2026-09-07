package verify

import (
	"os"
	"path/filepath"
	"testing"
)

// The CMS blob in a signed PDF is bytes somebody else produced. This
// package reads them with its own from-scratch BER/DER reader (D-044,
// deliberately not shared with the signer), which is exactly the kind of
// code SPEC §16.5's rule was written for: an attacker-shaped length or
// tag must produce an error, never a panic, an unbounded allocation or a
// loop that does not end.
//
// Two targets, because the bytes arrive by two different routes:
//
//   - FuzzParseCMS hands the SignedData straight to the parser, so the
//     fuzzer's whole budget goes into DER shapes rather than into
//     producing a PDF whose /Contents happens to hold one.
//   - FuzzVerifySignature hands it a whole document, which is the route
//     it actually travels: the raw-byte /ByteRange scanner, the hex
//     decode of /Contents, the CMS parse, the certificate lookup, the
//     signature check and the embedded RFC 3161 token, in one call.

const fuzzSeedDirEnv = "LIRO_FUZZ_SEED_DIR"

const maxSeedBytes = 256 * 1024

// seedPDFs adds the committed fixtures, the real signed documents when
// they are present (never in CI — testdata/pdfs/local is gitignored),
// and whatever FTEST corpus LIRO_FUZZ_SEED_DIR names.
func seedPDFs(f *testing.F, add func([]byte)) {
	f.Helper()
	add([]byte(""))
	add([]byte("%PDF-1.7\n%%EOF\n"))
	add([]byte("/ByteRange [0 1 2 3] /Contents <00>"))
	add([]byte("/ByteRange[0 0 0 0]/Contents<>"))

	dirs := []string{"../../../testdata/pdfs/local"}
	if d := os.Getenv(fuzzSeedDirEnv); d != "" {
		dirs = append(dirs, d)
	}
	for _, p := range []string{
		"../../../testdata/golden/minimal-signed-bb.pdf",
		"../../../testdata/pdfs/blank.pdf",
	} {
		if data, err := os.ReadFile(p); err == nil && len(data) <= maxSeedBytes {
			add(data)
		}
	}
	for _, dir := range dirs {
		names, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
		if err != nil {
			continue
		}
		for _, name := range names {
			data, err := os.ReadFile(name)
			if err != nil || len(data) > maxSeedBytes {
				continue
			}
			add(data)
		}
	}
}

// seedCMS adds every CMS blob this package can find inside a document it
// can reach, so the DER seeds are real signatures rather than shapes
// invented here. A blob that cannot be extracted is simply not added.
func seedCMS(f *testing.F, add func([]byte)) {
	f.Helper()
	// A minimal SEQUENCE, an empty input, and two truncations, so the
	// reader's own length handling has somewhere to start.
	add([]byte{})
	add([]byte{0x30, 0x00})
	add([]byte{0x30, 0x80, 0x00, 0x00})
	add([]byte{0x30, 0x82, 0xff, 0xff, 0x02, 0x01, 0x01})

	seedPDFs(f, func(pdfBytes []byte) {
		slots, err := FindSignatures(pdfBytes)
		if err != nil {
			return
		}
		for _, s := range slots {
			if len(s.CMS) > 0 && len(s.CMS) <= maxSeedBytes {
				add(s.CMS)
			}
		}
	})
}

// FuzzParseCMS drives the SignedData parser directly.
func FuzzParseCMS(f *testing.F) {
	seedCMS(f, func(b []byte) { f.Add(b) })

	f.Fuzz(func(t *testing.T, der []byte) {
		c, err := parseCMS(der)
		if err != nil {
			return
		}
		if c == nil {
			t.Fatal("parseCMS returned no error and no result")
		}
		// A successful parse must survive being used the way
		// VerifySignature uses it, which is where a nil field or a
		// nonsensical length would actually be dereferenced.
		_, _ = findCertificate(c.certificates, c.issuer, c.serialNumber)
	})
}

// FuzzVerifySignature runs the whole verification path over an arbitrary
// document: the raw-byte /ByteRange scan, the /Contents hex decode, the
// CMS parse, the digest recomputation, the RSA check and the embedded
// timestamp token.
func FuzzVerifySignature(f *testing.F) {
	seedPDFs(f, func(b []byte) { f.Add(b) })

	f.Fuzz(func(t *testing.T, data []byte) {
		slots, err := FindSignatures(data)
		if err != nil {
			return
		}
		for _, s := range slots {
			r := VerifySignature(data, s)
			if r == nil {
				t.Fatal("VerifySignature returned nil")
			}
		}
	})
}
