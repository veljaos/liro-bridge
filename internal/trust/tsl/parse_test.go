package tsl

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func mustParseSeed(t *testing.T) *List {
	t.Helper()
	list, err := Parse(seedXML)
	if err != nil {
		t.Fatalf("Parse(seed): %v", err)
	}
	return list
}

func TestSeedDigestMatchesExpected(t *testing.T) {
	if err := verifySeedDigest(); err != nil {
		t.Fatal(err)
	}
}

func TestParseSeedSequenceAndDate(t *testing.T) {
	list := mustParseSeed(t)
	if list.Sequence != 36 {
		t.Fatalf("Sequence = %d, want 36", list.Sequence)
	}
	want := time.Date(2026, 5, 20, 1, 0, 0, 0, time.UTC)
	if !list.IssuedAt.Equal(want) {
		t.Fatalf("IssuedAt = %v, want %v", list.IssuedAt, want)
	}
}

// TestParseSeedProviderCount asserts the real provider count measured
// directly from the bundled document. F1 §4.3 states "11 providers"; the
// actual sequence-36 file (whose SHA-256 matches F1 §4.7 exactly, byte
// for byte once the BOM is stripped) contains 10
// <TrustServiceProvider> elements. This test trusts direct measurement
// of the pinned artifact over the phase document's summary — see
// docs/decisions.md.
func TestParseSeedProviderCount(t *testing.T) {
	list := mustParseSeed(t)
	if len(list.Providers) != 10 {
		t.Fatalf("len(Providers) = %d, want 10", len(list.Providers))
	}
}

// fingerprintPrefix returns the first 8 bytes of a certificate's SHA-256,
// uppercase hex, matching the shortened fingerprints in F1 §4.4.
func fingerprintPrefix(der []byte) string {
	sum := sha256.Sum256(der)
	return strings.ToUpper(hex.EncodeToString(sum[:8]))
}

// TestSeedContainsAllSevenPinnedServiceCertificates is the parser
// correctness bar set by F1 §4.9: every trust anchor for the three
// supported issuers must be found, with status granted. Fingerprint
// prefixes are the F1 §4.4 table values with the colons removed.
func TestSeedContainsAllSevenPinnedServiceCertificates(t *testing.T) {
	list := mustParseSeed(t)

	want := map[string]string{
		"0E3EFEB4F77F9511": "MUP Gradjani CA 4",
		"923234E9382D7539": "MUP Stranci CA 4",
		"20EC0DB0BC171A06": "Posta Srbije CA 1",
		"9138F1CF70B0A19A": "Posta Srbije CA 2",
		"F004AB5F5023EFED": "PKS CA Class1",
		"8D56989132BC43F0": "Halcom BG CA FL e-signature",
		"9504AA9811FA6262": "Halcom BG CA FL e-signature 2",
	}

	found := make(map[string]bool, len(want))
	for _, p := range list.Providers {
		for _, s := range p.Services {
			if len(s.Certificate) == 0 {
				continue
			}
			fp := fingerprintPrefix(s.Certificate)
			if label, ok := want[fp]; ok {
				found[fp] = true
				if !s.IsCA() {
					t.Errorf("%s (%s): Type = %q, want a CA/QC service", label, fp, s.Type)
				}
				if !s.Granted() {
					t.Errorf("%s (%s): Status = %q, want granted", label, fp, s.Status)
				}
			}
		}
	}
	for fp, label := range want {
		if !found[fp] {
			t.Errorf("service certificate not found: %s (fingerprint %s...)", label, fp)
		}
	}
}

// TestServiceHistoryInstanceCount matches the measurement in F1 §4.3.
func TestServiceHistoryInstanceCount(t *testing.T) {
	list := mustParseSeed(t)
	n := 0
	for _, p := range list.Providers {
		for _, s := range p.Services {
			n += len(s.History)
		}
	}
	if n != 31 {
		t.Fatalf("total ServiceHistoryInstance = %d, want 31", n)
	}
}

func TestServiceStatusAtResolvesHistoricalStatus(t *testing.T) {
	svc := Service{
		Status:      "http://www.mit.gov.rs/TrstSvc/TrustedList/Svcstatus/withdrawn",
		StatusStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		History: []HistoryEntry{
			{Status: "http://www.mit.gov.rs/TrstSvc/TrustedList/Svcstatus/granted", StatusStart: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	}

	// Two years before the withdrawal, the service was granted (F1 §4.3
	// example: a service withdrawn today may have been granted when a
	// document was signed two years ago).
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := svc.StatusAt(at); !strings.HasSuffix(got, "granted") {
		t.Fatalf("StatusAt(%v) = %q, want granted", at, got)
	}

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if got := svc.StatusAt(now); !strings.HasSuffix(got, "withdrawn") {
		t.Fatalf("StatusAt(%v) = %q, want withdrawn", now, got)
	}

	before := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := svc.StatusAt(before); got != "" {
		t.Fatalf("StatusAt(%v) = %q, want empty (predates every known status)", before, got)
	}
}
