package tsa

import "testing"

// TestParseBERIndefiniteLength builds, by hand, a constructed SEQUENCE
// with indefinite length (F3 §12.5's "30 80 ..." shape) containing an
// INTEGER and a fragmented (constructed) OCTET STRING, terminated by
// End-of-Contents markers, and checks it parses identically to the
// equivalent definite-length DER encoding would.
func TestParseBERIndefiniteLength(t *testing.T) {
	// INTEGER 5
	integer := []byte{0x02, 0x01, 0x05}
	// Constructed, indefinite-length OCTET STRING made of two chunks
	// ("hel" + "lo"), terminated by 00 00.
	fragmentedOctetString := []byte{
		0x24, 0x80, // [UNIVERSAL 4] constructed, indefinite
		0x04, 0x03, 'h', 'e', 'l', // chunk 1
		0x04, 0x02, 'l', 'o', // chunk 2
		0x00, 0x00, // EOC
	}
	seq := append([]byte{0x30, 0x80}, integer...)
	seq = append(seq, fragmentedOctetString...)
	seq = append(seq, 0x00, 0x00) // EOC for the outer SEQUENCE

	node, rest, err := parseBERValue(seq)
	if err != nil {
		t.Fatalf("parseBERValue: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("leftover bytes after parsing: % X", rest)
	}
	if node.tag != 16 || !node.constructed || !node.indefinite {
		t.Fatalf("node = %#v, want an indefinite constructed SEQUENCE (tag 16)", node)
	}

	kids, err := node.children()
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(kids) != 2 {
		t.Fatalf("got %d children, want 2", len(kids))
	}
	if kids[0].tag != 2 || string(kids[0].content) != "\x05" {
		t.Fatalf("children[0] = %#v, want INTEGER 5", kids[0])
	}
	if kids[1].tag != 4 || !kids[1].indefinite {
		t.Fatalf("children[1] = %#v, want an indefinite OCTET STRING", kids[1])
	}
	value, err := kids[1].octetStringValue()
	if err != nil {
		t.Fatalf("octetStringValue: %v", err)
	}
	if string(value) != "hello" {
		t.Fatalf("octetStringValue() = %q, want %q", value, "hello")
	}
}

// TestParseBERDefiniteLengthLongForm exercises the long-form length
// encoding on an ordinary definite-length value (the common case for
// everything this project generates itself).
func TestParseBERDefiniteLengthLongForm(t *testing.T) {
	content := make([]byte, 200)
	for i := range content {
		content[i] = byte(i)
	}
	data := append([]byte{0x04, 0x81, 0xC8}, content...) // OCTET STRING, length 200 (0xC8)
	node, rest, err := parseBERValue(data)
	if err != nil {
		t.Fatalf("parseBERValue: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("leftover bytes: % X", rest)
	}
	if len(node.content) != 200 {
		t.Fatalf("content length = %d, want 200", len(node.content))
	}
}

func TestParseBERTruncatedInputReturnsError(t *testing.T) {
	cases := [][]byte{
		{},
		{0x30},
		{0x30, 0x80},             // indefinite with no content and no EOC
		{0x30, 0x05, 0x01, 0x02}, // declared length exceeds available bytes
	}
	for _, c := range cases {
		if _, _, err := parseBERValue(c); err == nil {
			t.Errorf("parseBERValue(% X) succeeded, want an error", c)
		}
	}
}
