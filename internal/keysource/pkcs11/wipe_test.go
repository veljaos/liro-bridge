package pkcs11

import (
	"runtime"
	"testing"
	"unsafe"
)

// TestWipeZeroesTheBytesAtThePinnedAddress is the owner's own instruction, and
// it is the reason wipe is a function rather than a loop at the call site:
// verify by reading the bytes back through the pinned address, not by trusting
// a loop the compiler is allowed to notice.
//
// Reading through the slice would prove nothing useful. A slice header and the
// memory behind it are two different things, and if the stores were elided the
// slice would still read as whatever the compiler decided — the check has to
// go to the address the foreign call was given, which is the address the
// pinner fixed. That is what `unsafe.Add` over the pinned pointer does here.
func TestWipeZeroesTheBytesAtThePinnedAddress(t *testing.T) {
	const n = 15 // the Pošta token's own ulMaxPinLen (D-273)

	buf := make([]byte, n)
	var p runtime.Pinner
	p.Pin(&buf[0])
	defer p.Unpin()

	// The address C_Login would be handed.
	base := unsafe.Pointer(&buf[0])

	for i := range buf {
		buf[i] = byte('0' + i%10)
	}
	// Confirm the fixture is capable of failing: if the bytes were already
	// zero, a wipe that did nothing would pass.
	nonZero := 0
	for i := 0; i < n; i++ {
		if *(*byte)(unsafe.Add(base, i)) != 0 {
			nonZero++
		}
	}
	if nonZero != n {
		t.Fatalf("the fixture wrote %d of %d bytes; a wipe that did nothing would pass this test", nonZero, n)
	}

	wipe(buf)

	for i := 0; i < n; i++ {
		if got := *(*byte)(unsafe.Add(base, i)); got != 0 {
			t.Errorf("byte %d at the pinned address is 0x%02X after wipe, want 0", i, got)
		}
	}
}
