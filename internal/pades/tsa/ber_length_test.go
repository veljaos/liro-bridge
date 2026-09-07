package tsa

import (
	"math"
	"strings"
	"testing"
)

// A BER long-form length may carry up to eight octets, and eight octets
// are enough to set the sign bit of an int. Every check downstream of
// the length compares it against a buffer size, which a negative value
// passes — and the next thing that happens is a slice bound.
//
// Found by FuzzParseResponse on its second minute: "30 88 30 30 30 30 30
// 30 30 30" — a SEQUENCE whose length is eight ASCII zeros — produced
//
//	panic: runtime error: slice bounds out of range [:-8633347502144212944]
//
// The identical defect was in internal/pades/verify's separate, from-
// scratch reader (D-044). Both are fixed; both carry a test.
func TestALengthTooLargeForAnIntIsRefusedRatherThanSliced(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{
			// The exact input the fuzzer found.
			name: "eight octets of 0x30",
			in:   []byte("0\x88\x880000000"),
		},
		{
			// The sign bit set on its own, which is the smallest value
			// that goes negative.
			name: "sign bit set",
			in:   []byte{0x30, 0x88, 0x80, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			// Every bit set: -1 as an int.
			name: "all ones",
			in: []byte{0x30, 0x88, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff},
		},
		{
			// The largest value that is still a positive int. This one
			// is refused by the "declared length exceeds available
			// bytes" check rather than by the new one, and must not
			// panic either.
			name: "max positive int",
			in: []byte{0x30, 0x88, 0x7f, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			node, rest, err := parseBERValue(c.in)
			if err == nil {
				t.Fatalf("parseBERValue accepted a length of %d bytes from %d bytes of input: node %+v, %d bytes left",
					len(node.content), len(c.in), node, len(rest))
			}
			// The message is for a developer reading a log, so it only
			// has to name what happened.
			if !strings.Contains(err.Error(), "length") {
				t.Fatalf("error does not say the length was the problem: %v", err)
			}
		})
	}
}

// A length that genuinely fits is still read, so the fix is a rejection
// of the impossible rather than a narrowing of the possible.
func TestALengthThatFitsIsStillRead(t *testing.T) {
	// 0x30 0x88 with six leading zero octets and 0x00 0x05: five bytes.
	in := []byte{0x30, 0x88, 0, 0, 0, 0, 0, 0, 0, 0x05, 'h', 'e', 'l', 'l', 'o'}
	node, rest, err := parseBERValue(in)
	if err != nil {
		t.Fatalf("parseBERValue: %v", err)
	}
	if string(node.content) != "hello" {
		t.Fatalf("content = %q, want %q", node.content, "hello")
	}
	if len(rest) != 0 {
		t.Fatalf("%d bytes left over, want 0", len(rest))
	}
	if uint64(math.MaxInt) < 5 {
		t.Fatal("unreachable; keeps the math import honest about what the guard compares against")
	}
}
