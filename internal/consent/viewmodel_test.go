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
