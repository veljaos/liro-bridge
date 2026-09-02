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
// never silent (F1 §4.7).
const seedSHA256 = "3f5744843bcfaba0698c6b5f7121ee9f133151249808c9de0ebb8e351d9ddb8b"

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
