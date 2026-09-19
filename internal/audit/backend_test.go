package audit

import (
	"bytes"
	"testing"
	"time"
)

// base is an entry with every optional field unset: what every entry in every
// audit log on disk today is, as far as these two new fields are concerned.
func base() Entry {
	return Entry{
		Sequence:      7,
		Timestamp:     time.Unix(1_700_000_000, 0),
		Thumbprint:    "AF5063BB74378BD503AB46DD08AEAD205BA2AA54",
		Application:   "local",
		DocumentCount: 3,
		Outcome:       OutcomeApproved,
		IsTestKey:     false,
		AchievedLevel: "B-T",
		PrevHash:      []byte{1, 2, 3, 4},
	}
}

// TestAnEntryWithNoBackendCanonicalisesToExactlyWhatItAlwaysDid is the property
// the whole design of these two fields turns on.
//
// The log is hash-chained: an entry's stored hash is over its canonical bytes,
// and every entry after it hashes that. If adding a field changed the bytes of
// an entry written before the field existed, every audit log on every machine
// would stop verifying at its first entry — including the owner's own, which
// holds 316 of them.
//
// The guarantee is structural: the marker is emitted only when one of the two
// fields is set, so an entry with neither produces a buffer that ends where it
// always ended.
func TestAnEntryWithNoBackendCanonicalisesToExactlyWhatItAlwaysDid(t *testing.T) {
	got := base().CanonicalBytes()
	if len(got) == 0 {
		t.Fatal("no canonical bytes at all")
	}
	if got[len(got)-1] == backendMarker {
		t.Errorf("an entry with no backend ends with the backend marker")
	}
	// The decisive form: setting a backend must lengthen the buffer, and
	// clearing it again must give back exactly the original bytes.
	with := base()
	with.Backend = "pkcs11"
	if bytes.Equal(got, with.CanonicalBytes()) {
		t.Fatal("setting a backend did not change the canonical bytes, so the field " +
			"is not hashed and could be altered without breaking the chain")
	}
	with.Backend = ""
	if !bytes.Equal(got, with.CanonicalBytes()) {
		t.Fatal("clearing the backend did not give back the original bytes")
	}
}

// TestTheBackendAndTheModuleCannotCollide. Both are written whenever either is
// set, and both are length-prefixed, so the two halves cannot be confused with
// each other or with an entry that has neither.
func TestTheBackendAndTheModuleCannotCollide(t *testing.T) {
	onlyBackend := base()
	onlyBackend.Backend = "pkcs11"

	onlyModule := base()
	onlyModule.Module = "pkcs11"

	both := base()
	both.Backend = "pkcs11"
	both.Module = "netsetpkcs11_x64.dll"

	seen := map[string]string{}
	for name, e := range map[string]Entry{
		"neither":      base(),
		"only backend": onlyBackend,
		"only module":  onlyModule,
		"both":         both,
	} {
		key := string(e.CanonicalBytes())
		if other, dup := seen[key]; dup {
			t.Errorf("%q and %q canonicalise to the same bytes", name, other)
		}
		seen[key] = name
	}
}

// TestASignatureThroughOneModuleIsNotTheSameRecordAsThroughAnother is the
// reason the module is hashed rather than merely stored.
//
// Two builds of one vendor's module, five years apart, differ by twenty-seven
// times on one call (D-305) and are identical in everything CK_INFO reports.
// The path is the only thing that tells them apart, so an audit entry that
// recorded the path without hashing it would let the one fact that
// distinguishes them be edited without breaking the chain.
func TestASignatureThroughOneModuleIsNotTheSameRecordAsThroughAnother(t *testing.T) {
	a := base()
	a.Backend = "pkcs11"
	a.Module = `C:\Program Files\TrustEdgeID\netsetpkcs11_x64.dll`

	b := base()
	b.Backend = "pkcs11"
	b.Module = `C:\Program Files\MUP RS\Celik\netsetpkcs11_x64.dll`

	if bytes.Equal(a.CanonicalBytes(), b.CanonicalBytes()) {
		t.Fatal("two signatures through different modules hash identically")
	}
}

// TestTheNewFieldsSurviveTheRoundTripToDisk. A field that is hashed but not
// stored would make every entry fail to verify the moment it was read back.
func TestTheNewFieldsSurviveTheRoundTripToDisk(t *testing.T) {
	e := base()
	e.Backend = "pkcs11"
	e.Module = "netsetpkcs11_x64.dll"
	e.Hash = e.ComputeHash()

	back, err := fromJSONEntry(toJSONEntry(e))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if back.Backend != e.Backend || back.Module != e.Module {
		t.Fatalf("round trip lost the backend or the module: %q / %q", back.Backend, back.Module)
	}
	if !bytes.Equal(back.ComputeHash(), e.Hash) {
		t.Fatal("the entry read back does not hash to what was written")
	}
}
