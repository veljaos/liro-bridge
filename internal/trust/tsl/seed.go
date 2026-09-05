package tsl

import (
	"crypto/sha256"
	_ "embed" // for go:embed below
	"encoding/hex"
	"fmt"
)

// seedXML is the Trusted List bundled in the binary so a first run with no
// network access can still make a truthful (if aged) qualification
// decision (F1 §4.7). It is sequence 36, issued 2026-05-20, fetched from
// the Ministry's own publication endpoint.
//
//go:embed seed/TSL-RS.xml
var seedXML []byte

// seedSHA256 is the expected digest of seed/TSL-RS.xml, checked by
// TestSeedDigestMatchesExpected so a change to the embedded file is
// never silent (F1 §4.7). It is the digest of the Ministry's own
// published bytes with the leading UTF-8 BOM stripped and the CRLF line
// endings the server sends left exactly as they are — see D-107 for why
// that second half needs saying, and .gitattributes for what keeps it
// true. Regenerate with `go run ./scripts/genseed -write`, which
// verifies the candidate's signature against PinnedSigners before it
// will write anything.
const seedSHA256 = "3f5744843bcfaba0698c6b5f7121ee9f133151249808c9de0ebb8e351d9ddb8b"

// seedSequence is the TSL sequence number the embedded seed carries.
// Stating it here, as a constant a human read off the document, is what
// lets TestSeedSequenceAgreesWithWhatTheCodeReports check that the file,
// the parser and the Store all say the same number — three things that
// were reported as disagreeing (F6 §0a) and turned out not to.
const seedSequence = 36

// verifySeedDigest recomputes the seed's SHA-256 and compares it against
// seedSHA256, returning an error on mismatch.
func verifySeedDigest() error {
	sum := sha256.Sum256(seedXML)
	got := hex.EncodeToString(sum[:])
	if got != seedSHA256 {
		return fmt.Errorf("embedded seed digest mismatch: got %s, want %s", got, seedSHA256)
	}
	return nil
}
