package consent

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestBuildViewModelFingerprintMatchesSPEC(t *testing.T) {
	d1 := sha256.Sum256([]byte("doc1"))
	d2 := sha256.Sum256([]byte("doc2"))
	digests := [][]byte{d1[:], d2[:]}

	want := sha256.New()
	want.Write(d1[:])
	want.Write(d2[:])
	wantHex := hex.EncodeToString(want.Sum(nil))

	vm := BuildViewModel(ApplicationLocal, digests, []string{"a.pdf", "b.pdf"}, nil)
	if vm.Fingerprint != wantHex {
		t.Fatalf("got fingerprint %s, want %s", vm.Fingerprint, wantHex)
	}
	if vm.DocumentCount != 2 {
		t.Fatalf("got DocumentCount %d, want 2", vm.DocumentCount)
	}
}

// TestBuildViewModelSanitisesFiles proves the ViewModel construction
// path itself — not just CapFileNames in isolation — never lets an
// unsanitised name through.
func TestBuildViewModelSanitisesFiles(t *testing.T) {
	vm := BuildViewModel(ApplicationLocal, [][]byte{{1}}, []string{"racun" + string(rune(0x202E)) + "fdp.exe"}, nil)
	if vm.Files[0] != "racunfdp.exe" {
		t.Fatalf("got %q, want sanitised name", vm.Files[0])
	}
}

// TestShortFingerprintElidesToSixteenCharacters is Task 2's Go-side
// half (F5 second-real-run review): the value handed to the page is
// already elided, so the page has no opportunity to render 64
// unbreakable characters even if its own CSS changed.
func TestShortFingerprintElidesToSixteenCharacters(t *testing.T) {
	full := "d54aeba8571c16922cb7cd1f6b758824bc7b26e6865daa4bba47be47c906135e"
	got := ShortFingerprint(full)
	if got != "d54aeba8571c1692..." {
		t.Fatalf("ShortFingerprint(%s) = %q", full, got)
	}
	if len(got) != FingerprintPrefixLength+3 {
		t.Fatalf("ShortFingerprint returned %d characters, want %d", len(got), FingerprintPrefixLength+3)
	}
}

// TestShortFingerprintLeavesShortValuesAlone: a value that already fits
// gains no ellipsis, which would otherwise claim there is more of it.
func TestShortFingerprintLeavesShortValuesAlone(t *testing.T) {
	for _, in := range []string{"", "abc", "0123456789abcdef"} {
		if got := ShortFingerprint(in); got != in {
			t.Errorf("ShortFingerprint(%q) = %q, want it unchanged", in, got)
		}
	}
}

// TestBuildViewModelCarriesBothFingerprintForms proves the full value
// is still available (the Copy action needs it) alongside the elided
// one the window renders.
func TestBuildViewModelCarriesBothFingerprintForms(t *testing.T) {
	vm := BuildViewModel(ApplicationLocal, [][]byte{{1}}, []string{"a.pdf"}, nil)
	if len(vm.Fingerprint) != 64 {
		t.Fatalf("Fingerprint is %d characters, want the full 64", len(vm.Fingerprint))
	}
	if vm.FingerprintShort != ShortFingerprint(vm.Fingerprint) {
		t.Fatalf("FingerprintShort = %q, want %q", vm.FingerprintShort, ShortFingerprint(vm.Fingerprint))
	}
}
