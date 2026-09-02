package windowscng

import (
	"os"
	"testing"
)

// TestThumbprintMatchesKnownValue is the F1 §3.6 test: thumbprint
// computation from a known DER file, compared against a known value —
// independently computed here with `openssl dgst -sha1` rather than by
// re-deriving it through the same crypto/sha1 call the code under test
// uses, so the test cannot pass merely because both sides share a bug.
func TestThumbprintMatchesKnownValue(t *testing.T) {
	der, err := os.ReadFile("../../../testdata/certs/mup_signing.der")
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}

	// openssl dgst -sha1 testdata/certs/mup_signing.der
	const want = "41869CE698BE963AC96F2B94ADA19A370DC41293"
	if got := thumbprint(der); got != want {
		t.Fatalf("thumbprint() = %q, want %q", got, want)
	}
}
