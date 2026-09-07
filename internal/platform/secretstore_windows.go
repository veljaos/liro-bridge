package platform

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// newSecretStore returns the Windows secret store: a file of DPAPI
// blobs beside the additional-entropy file they are bound to (SPEC
// §6.4's Windows row, "DPAPI, CryptProtectData with pOptionalEntropy").
func newSecretStore(dir string) (SecretStore, error) {
	return newFileSecretStore(dir, dpapiProtector{})
}

// dpapiProtector encrypts with CryptProtectData, which binds the blob
// to the current *user account* on the current machine — so a blob
// copied to another account, or to another machine, cannot be
// decrypted there even by someone holding the entropy file too.
//
// golang.org/x/sys/windows already declares both calls, so nothing here
// is hand-written syscall glue. That is D-082's reasoning for the
// registry, unchanged: hand-writing a syscall is worth it only where no
// maintained binding exists (D-080's WebView2 COM surface), and this
// project already depends on golang.org/x/sys, which SPEC §8.6 names as
// an expected acceptable dependency.
type dpapiProtector struct{}

// blobOf builds a windows.DataBlob pointing at b. b must not be empty:
// &b[0] on a zero-length slice panics, and both callers below guarantee
// non-empty input (Set rejects an empty value, the entropy file is
// always entropyLength bytes, and a stored blob that decoded to nothing
// is rejected before it gets here).
func blobOf(b []byte) windows.DataBlob {
	return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// copyOutBlob copies out's bytes into Go memory and frees the buffer
// DPAPI allocated. The copy is not optional: out.Data points at memory
// LocalFree is about to release.
func copyOutBlob(out windows.DataBlob) []byte {
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	if out.Size == 0 || out.Data == nil {
		return nil
	}
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
}

// errEmptyBlob guards the &b[0] precondition above rather than leaving
// it to a panic in a signing agent.
var errEmptyBlob = errors.New("platform: DPAPI was given an empty buffer")

// Protect implements protector.
func (dpapiProtector) Protect(plaintext, entropy []byte) ([]byte, error) {
	if len(plaintext) == 0 || len(entropy) == 0 {
		return nil, errEmptyBlob
	}
	in := blobOf(plaintext)
	ent := blobOf(entropy)
	var out windows.DataBlob
	// CRYPTPROTECT_UI_FORBIDDEN: this runs on a background goroutine
	// serving an HTTP request, where a modal credential prompt nobody
	// is watching would hang the pairing rather than fail it.
	err := windows.CryptProtectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	return copyOutBlob(out), nil
}

// Unprotect implements protector.
func (dpapiProtector) Unprotect(ciphertext, entropy []byte) ([]byte, error) {
	if len(ciphertext) == 0 || len(entropy) == 0 {
		return nil, errEmptyBlob
	}
	in := blobOf(ciphertext)
	ent := blobOf(entropy)
	var out windows.DataBlob
	err := windows.CryptUnprotectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	return copyOutBlob(out), nil
}
