package verify

import (
	"strings"
	"testing"
)

// This package is deliberately a second, independent implementation of
// the reader in internal/pades/tsa (D-044): a bug in a shared helper
// would pass in both directions, so nothing is shared.
//
// It had the same bug anyway. A BER long-form length of eight octets
// sets the sign bit of an int; every check below the length compares it
// against a buffer size, which a negative value passes; the slice bound
// that follows panics. Found by fuzzing the *other* reader, then looked
// for here — which is the useful lesson to keep: two implementations do
// not catch a mistake both of them make, and the way to find that class
// of thing is to go and look at the sibling the moment one of them
// falls over.
func TestALengthTooLargeForAnIntIsRefusedRatherThanSliced(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"eight octets of 0x30", []byte("0\x88\x880000000")},
		{"sign bit set", []byte{0x30, 0x88, 0x80, 0, 0, 0, 0, 0, 0, 0}},
		{"all ones", []byte{0x30, 0x88, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{"max positive int", []byte{0x30, 0x88, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			node, rest, err := readDER(c.in)
			if err == nil {
				t.Fatalf("readDER accepted a length of %d bytes from %d bytes of input: node %+v, %d bytes left",
					len(node.content), len(c.in), node, len(rest))
			}
			if !strings.Contains(err.Error(), "length") {
				t.Fatalf("error does not say the length was the problem: %v", err)
			}
		})
	}
}

func TestALengthThatFitsIsStillRead(t *testing.T) {
	in := []byte{0x30, 0x88, 0, 0, 0, 0, 0, 0, 0, 0x05, 'h', 'e', 'l', 'l', 'o'}
	node, rest, err := readDER(in)
	if err != nil {
		t.Fatalf("readDER: %v", err)
	}
	if string(node.content) != "hello" {
		t.Fatalf("content = %q, want %q", node.content, "hello")
	}
	if len(rest) != 0 {
		t.Fatalf("%d bytes left over, want 0", len(rest))
	}
}
