package ui

import (
	"bytes"
	"testing"
	"unicode/utf16"
)

// These run on every platform, which is the reason encodePINInto is a function
// in a neutral file rather than a loop inside accept: the one decision in the
// PIN dialog that can be got wrong without a window is how many bytes a string
// of characters needs, and whether that fits the token's own buffer. Everything
// else in that dialog is Win32 and cannot be tested at all.

func TestAPINThatFitsIsWrittenWhole(t *testing.T) {
	// The shape both Serbian cards this project has measured actually use: a
	// numeric PIN, well inside the token's own maximum (MUP declares 8, Pošta
	// 15 — D-276).
	dst := make([]byte, 15)
	n := encodePINInto(dst, []rune("1234"))
	if n != 4 {
		t.Fatalf("encodePINInto wrote %d bytes for a four-character PIN, want 4", n)
	}
	if got := string(dst[:n]); got != "1234" {
		t.Errorf("the bytes written are %q, want %q", got, "1234")
	}
}

// TestAPINExactlyFillingTheBufferIsAccepted is the boundary on the side that
// must not be refused. A token declaring 15 accepts a PIN of 15 bytes, and a
// layer that refused it would refuse a legitimate PIN and cost the person the
// attempt they were about to make correctly.
func TestAPINExactlyFillingTheBufferIsAccepted(t *testing.T) {
	dst := make([]byte, 4)
	if n := encodePINInto(dst, []rune("1234")); n != 4 {
		t.Errorf("a PIN exactly filling the buffer was reported as %d, want 4", n)
	}
}

// TestAPINLongerThanTheTokensBufferIsRefusedAndNothingIsWritten is the case
// ErrPINTooLong exists for, and the reason it is not a cancellation.
//
// EM_SETLIMITTEXT bounds the control in characters and ulMaxPinLen is in
// bytes, so on a Pošta token fifteen Cyrillic characters are fifteen the
// control accepts and thirty bytes the token will not take.
func TestAPINLongerThanTheTokensBufferIsRefusedAndNothingIsWritten(t *testing.T) {
	dst := make([]byte, 15)
	for i := range dst {
		dst[i] = 0xEE // so that "nothing was written" is distinguishable from zeroes
	}
	runes := []rune("ШИФРАШИФРАШИФРА") // 15 characters, 30 bytes
	if len(runes) != 15 {
		t.Fatalf("the fixture is %d characters, not the 15 this test is about", len(runes))
	}
	n := encodePINInto(dst, runes)
	if n != -1 {
		t.Fatalf("encodePINInto accepted %d bytes into a 15-byte buffer, want -1", n)
	}
	if bytes.ContainsRune(dst, 0) || !bytes.Equal(dst, bytes.Repeat([]byte{0xEE}, 15)) {
		t.Errorf("the buffer was written to on the refused path: %x\n"+
			"Nothing may be written unless all of it fits — a prefix of what was "+
			"typed, left in the caller's buffer on a path reported as producing "+
			"nothing, is a partial PIN nobody asked for", dst)
	}
}

// TestOneByteOverIsRefused pins the other side of the boundary, so that a
// rule written as >= rather than > would be caught.
func TestOneByteOverIsRefused(t *testing.T) {
	dst := make([]byte, 4)
	if n := encodePINInto(dst, []rune("12345")); n != -1 {
		t.Errorf("five characters into a four-byte buffer was reported as %d, want -1", n)
	}
}

// TestWhatTheDialogWouldActuallyHandIt runs the real decode the dialog does,
// so that the fixtures above are known to be the shape encodePINInto is given
// rather than the shape this test found convenient.
//
// utf16.Decode is what accept calls on the bytes WM_GETTEXT wrote, and it
// replaces an unpaired surrogate with U+FFFD rather than producing a rune
// utf8.RuneLen refuses — which is why the refusal inside encodePINInto for a
// negative RuneLen is unreachable through the dialog and is there anyway.
func TestWhatTheDialogWouldActuallyHandIt(t *testing.T) {
	// A lone high surrogate, which is what a torn UTF-16 pair looks like.
	runes := utf16.Decode([]uint16{0xD800})
	if len(runes) != 1 || runes[0] != 0xFFFD {
		t.Fatalf("utf16.Decode gave %U, and this test's premise was that it gives U+FFFD", runes)
	}
	dst := make([]byte, 15)
	if n := encodePINInto(dst, runes); n != 3 {
		t.Errorf("U+FFFD encoded to %d bytes, want 3", n)
	}
}
