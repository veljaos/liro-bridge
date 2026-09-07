package tsa

import (
	"crypto/sha256"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// An RFC 3161 timestamp token is read from two places, and neither of
// them is under this agent's control: an HTTP response from a timestamp
// authority, and the unsigned attribute inside the CMS of a document
// somebody else signed. SPEC §12.5 requires this reader to accept BER
// with indefinite lengths and fragmented OCTET STRINGs — precisely the
// shapes where a length is a promise the encoder makes and the decoder
// has to survive being lied to about.
//
// D-048 records that no genuinely BER-encoded real-world token was
// available when this reader was written, so its indefinite-length path
// rests on one hand-built vector. Fuzzing is what that gap asks for.

const fuzzSeedDirEnv = "LIRO_FUZZ_SEED_DIR"

const maxSeedBytes = 256 * 1024

// seedTokens adds real timestamp responses and tokens: ones this
// package's own test helper builds, and the ones embedded in whatever
// signed documents are on this machine.
func seedTokens(f *testing.F, add func([]byte)) {
	f.Helper()

	add([]byte{})
	add([]byte{0x30, 0x00})
	add([]byte{0x30, 0x80, 0x00, 0x00})                  // an empty indefinite-length SEQUENCE
	add([]byte{0x30, 0x80, 0x30, 0x80, 0x00, 0x00})      // one that never ends
	add([]byte{0x30, 0x82, 0xff, 0xff, 0x02, 0x01})      // a length far past the buffer
	add([]byte{0x24, 0x80, 0x04, 0x01, 'a', 0x00, 0x00}) // a fragmented OCTET STRING

	// A well-formed response built by this package's own test helper,
	// so the fuzzer starts from something that parses all the way to a
	// genTime rather than from garbage.
	digest := sha256.Sum256([]byte("liro fuzz seed"))
	if resp := buildFuzzSeedResponse(f, digest[:]); resp != nil {
		add(resp)
	}

	// Every embedded token in every signed document reachable from here.
	dirs := []string{"../../../testdata/pdfs/local"}
	if d := os.Getenv(fuzzSeedDirEnv); d != "" {
		dirs = append(dirs, d)
	}
	files := []string{"../../../testdata/golden/minimal-signed-bb.pdf"}
	for _, dir := range dirs {
		names, _ := filepath.Glob(filepath.Join(dir, "*.pdf"))
		files = append(files, names...)
	}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, tok := range embeddedTokens(data) {
			if len(tok) > 0 && len(tok) <= maxSeedBytes {
				add(tok)
			}
		}
	}
}

// embeddedTokens finds every "30 80" and "30 82" TLV that follows the
// signatureTimeStampToken OID in a file's raw bytes. It is deliberately
// crude — a seed does not have to be correct, only interesting — and it
// avoids importing internal/pades/verify, which would make this
// package's tests depend on the one package written to be independent
// of it.
func embeddedTokens(data []byte) [][]byte {
	const oid = "\x2a\x86\x48\x86\xf7\x0d\x01\x09\x10\x02\x0e" // 1.2.840.113549.1.9.16.2.14
	var out [][]byte
	for i := 0; i+len(oid) < len(data); i++ {
		if string(data[i:i+len(oid)]) != oid {
			continue
		}
		// The attribute value follows within a short distance; take a
		// generous window and let the parser decide what it is.
		end := i + 64*1024
		if end > len(data) {
			end = len(data)
		}
		out = append(out, data[i+len(oid):end])
	}
	return out
}

// buildFuzzSeedResponse produces one valid TimeStampResp. It returns nil
// rather than failing the fuzz target if the helper cannot build one:
// a missing seed is a weaker fuzz run, not a broken test.
func buildFuzzSeedResponse(f *testing.F, digest []byte) (resp []byte) {
	defer func() {
		if recover() != nil {
			resp = nil
		}
	}()
	t := &testing.T{}
	return buildTestResponse(t, digest, big.NewInt(1), time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC))
}

// FuzzParseResponse drives the whole RFC 3161 reader: the BER value
// reader, the status/failInfo decoding, the TimeStampToken's CMS
// envelope, and TSTInfo with its GeneralizedTime and its nonce.
func FuzzParseResponse(f *testing.F) {
	seedTokens(f, func(b []byte) { f.Add(b) })

	f.Fuzz(func(t *testing.T, data []byte) {
		resp, err := parseResponse(data)
		if err != nil {
			return
		}
		if resp == nil {
			t.Fatal("parseResponse returned no error and no response")
		}
		// A parsed response is used for these three things, so they are
		// what has to survive whatever the parser decided it read.
		_ = resp.GenTime.Unix()
		_ = len(resp.TokenDER)
		_ = len(resp.MessageImprintHash)
	})
}

// FuzzParseBERValue drives the reader one layer down, where a length, a
// tag and an end-of-contents marker are the whole attack surface.
func FuzzParseBERValue(f *testing.F) {
	seedTokens(f, func(b []byte) { f.Add(b) })

	f.Fuzz(func(t *testing.T, data []byte) {
		node, rest, err := parseBERValue(data)
		if err != nil {
			return
		}
		if len(rest) > len(data) {
			t.Fatalf("parseBERValue returned %d bytes of remainder for %d bytes of input", len(rest), len(data))
		}
		children, err := node.children()
		if err != nil {
			return
		}
		for _, c := range children {
			_, _ = c.octetStringValue()
			_, _ = c.children()
		}
	})
}
