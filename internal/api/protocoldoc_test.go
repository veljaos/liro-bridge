package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docs/PROTOCOL.md is what an integrator implements against, and its
// worked example is the one thing in it they check their own code
// against before they check it against a running agent. A worked
// example that has drifted from the implementation is worse than none:
// it sends someone hunting for a bug in their own client that is not
// there.
//
// So the document's own numbers are read out of it and compared with
// what this package produces. The same discipline scripts/synctokens
// now applies to its generated CSS, for the same reason — a fact
// written down twice is a fact that can disagree with itself.
func TestTheProtocolDocumentsWorkedExampleIsTrue(t *testing.T) {
	doc := readProtocolDoc(t)

	for _, want := range []struct{ what, value string }{
		{"the empty-body hash", EmptyBodySHA256},
		{"the example device secret", exampleSecretBase64},
		{"the example timestamp", exampleTimestamp},
		{"the example nonce", exampleNonce},
		{"the example body", exampleBody},
		{"the example body hash", exampleBodyHash},
		{"the example signature", exampleSignature},
	} {
		if !strings.Contains(doc, want.value) {
			t.Errorf("docs/PROTOCOL.md does not carry %s (%s)", want.what, want.value)
		}
	}

	// The canonical string in the document is written with \n as a
	// literal escape, because a real newline in a fenced block would
	// look like five separate lines and hide the one thing this example
	// exists to pin down.
	escaped := strings.ReplaceAll(
		CanonicalString(exampleMethod, examplePath, exampleTimestamp, exampleNonce, []byte(exampleBody)),
		"\n", `\n`)
	if !strings.Contains(doc, escaped) {
		t.Errorf("docs/PROTOCOL.md does not carry the canonical string this package builds:\n  %s", escaped)
	}
}

// Every code this package can put on the wire has to be in the
// document's own table, or a caller receives something nothing tells
// them how to react to. F7 §8: "document every code a caller can
// receive."
func TestEveryCodeThisPackageReturnsIsDocumented(t *testing.T) {
	doc := readProtocolDoc(t)

	// The codes reachable through the endpoints and the authenticator
	// this part of the protocol has. Signing adds its own, with its own
	// endpoints; those are documented alongside them.
	for _, code := range []string{
		"REQUEST_INVALID", "NOT_PAIRED", "AUTH_FAILED",
		"PAIRING_CODE_INCORRECT", "PAIRING_DENIED", "PAIRING_ORIGIN_MISMATCH",
		"PAIRING_EXPIRED", "PAIRING_IN_PROGRESS", "RATE_LIMITED", "INTERNAL",
	} {
		if !strings.Contains(doc, "`"+code+"`") {
			t.Errorf("docs/PROTOCOL.md does not document the code %s", code)
		}
	}
}

func readProtocolDoc(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		path := filepath.Join(dir, "docs", "PROTOCOL.md")
		if b, err := os.ReadFile(path); err == nil {
			return string(b)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find docs/PROTOCOL.md above the working directory")
		}
		dir = parent
	}
}
