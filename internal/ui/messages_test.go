package ui

import "testing"

// TestParseMessage covers F5 §2.4's three-message surface and its
// "everything else is dropped" rule — the page can request exactly
// approve, cancel, and select a certificate by thumbprint; anything
// else, including malformed JSON, is rejected.
func TestParseMessage(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Message
		ok   bool
	}{
		{"approve", `{"type":"approve"}`, Message{Type: MessageTypeApprove}, true},
		{"cancel", `{"type":"cancel"}`, Message{Type: MessageTypeCancel}, true},
		{
			"select certificate",
			`{"type":"selectCertificate","thumbprint":"ABCDEF0123456789"}`,
			Message{Type: MessageTypeSelectCertificate, Thumbprint: "ABCDEF0123456789"},
			true,
		},
		{"select certificate without a thumbprint is rejected", `{"type":"selectCertificate"}`, Message{}, false},
		{"select certificate with an empty thumbprint is rejected", `{"type":"selectCertificate","thumbprint":""}`, Message{}, false},
		{"unknown type is dropped", `{"type":"navigate","url":"https://evil.example"}`, Message{}, false},
		{"empty type is dropped", `{"type":""}`, Message{}, false},
		{"malformed JSON is dropped", `{"type":`, Message{}, false},
		{"not an object is dropped", `"approve"`, Message{}, false},
		{"empty body is dropped", ``, Message{}, false},
		// A message carrying extra, unexpected fields alongside a valid
		// type must still be accepted for that type, and the extra
		// field must never leak into the parsed Message — the page-side
		// surface is exactly three requests, nothing more (F5 §2.4).
		{"approve with unexpected extra fields still parses, extras dropped", `{"type":"approve","extra":"ignored","thumbprint":"should-not-appear"}`, Message{Type: MessageTypeApprove}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseMessage([]byte(tc.raw))
			if ok != tc.ok {
				t.Fatalf("ParseMessage(%q) ok = %v, want %v", tc.raw, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("ParseMessage(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestDispatchMessageDropsRejectedMessages proves the "dropped and
// logged" half of F5 §2.4: dispatchMessage must never call onMessage
// for input ParseMessage rejects.
func TestDispatchMessageDropsRejectedMessages(t *testing.T) {
	var called bool
	dispatchMessage(func(Message) { called = true }, []byte(`{"type":"deleteEverything"}`))
	if called {
		t.Fatal("dispatchMessage invoked onMessage for a message type outside the three-message surface")
	}
}

// TestDispatchMessageDeliversAcceptedMessages is the mirror check: a
// well-formed message of a recognised type must actually reach
// onMessage, scoped to exactly the one call this test makes (not
// asserting anything about the whole package's behaviour).
func TestDispatchMessageDeliversAcceptedMessages(t *testing.T) {
	var got Message
	var called bool
	dispatchMessage(func(m Message) { got, called = m, true }, []byte(`{"type":"cancel"}`))
	if !called {
		t.Fatal("dispatchMessage did not invoke onMessage for a valid cancel message")
	}
	if got.Type != MessageTypeCancel {
		t.Fatalf("got Type %q, want %q", got.Type, MessageTypeCancel)
	}
}
