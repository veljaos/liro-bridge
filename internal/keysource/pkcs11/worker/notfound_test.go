package worker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// TestNotFoundSurvivesThePipeAsASentinel is what makes D-311's fallback
// possible at all.
//
// The agent asks CNG, then each PKCS#11 module, until one says yes. That only
// works if "that certificate is not here" can be told from "this module is
// broken" — and between the parent and the module there is a process boundary
// and a JSON frame, where an error is a string. errors.Is does not survive
// json.Marshal.
//
// Both directions are asserted in one test, because the mistake to guard
// against is a parent that answers "not found" to everything: that would make
// the agent walk past a card with a real problem and end up reporting the last
// backend's failure for a certificate the first one had.
func TestNotFoundSurvivesThePipeAsASentinel(t *testing.T) {
	t.Run("a missing certificate arrives as ErrCertificateNotFound", func(t *testing.T) {
		w := New(cannedPath(cannedAnswers{NoSuchCertificate: true}), io.Discard)
		t.Cleanup(func() { _ = w.Close(context.Background()) })

		_, err := NewSource(w, nil).Open(context.Background(), "ABCD")
		if !errors.Is(err, pkcs11.ErrCertificateNotFound) {
			t.Fatalf("Open = %v\nwant an error that is pkcs11.ErrCertificateNotFound", err)
		}
		// The module path has to survive too: it is the only thing that tells
		// two builds of one vendor's module apart (D-271), and a report that
		// loses it cannot be acted on.
		if got := err.Error(); !strings.Contains(got, "canned") {
			t.Errorf("the error does not name the module: %q", got)
		}
	})

	t.Run("any other refusal is not", func(t *testing.T) {
		// A wrong PIN: the card answering, which is emphatically not "this
		// certificate is somewhere else". If this came back as not-found the
		// agent would try the next module and ask for another PIN, which
		// SPEC §6.5.1 clause 5 exists to prevent.
		w := New(cannedPath(askingCard(cannedAnswers{WantPIN: "the right one"})), io.Discard)
		t.Cleanup(func() { _ = w.Close(context.Background()) })

		entry := func(dst []byte, _ pkcs11.PINRequest) (int, error) { return copy(dst, "the wrong one"), nil }
		_, err := NewSource(w, entry).Open(context.Background(), "ABCD")
		if err == nil {
			t.Fatal("a wrong PIN was accepted")
		}
		if errors.Is(err, pkcs11.ErrCertificateNotFound) {
			t.Fatalf("a refused PIN arrived as ErrCertificateNotFound: %v\n"+
				"That would make the agent walk on to the next module and ask for "+
				"another PIN for the same signature.", err)
		}
	})

	t.Run("a chain request misses the same way", func(t *testing.T) {
		w := New(cannedPath(cannedAnswers{NoSuchCertificate: true}), io.Discard)
		t.Cleanup(func() { _ = w.Close(context.Background()) })

		_, err := w.ChainFor(context.Background(), "ABCD")
		if !errors.Is(err, pkcs11.ErrCertificateNotFound) {
			t.Fatalf("ChainFor = %v, want ErrCertificateNotFound", err)
		}
	})
}

// TestWithPINEntrySharesTheChildAndNotTheScreen. The Worker is a child process
// that is expensive to start and is meant to be shared between signatures; the
// screen belongs to one window and one moment. Copying must separate them.
func TestWithPINEntrySharesTheChildAndNotTheScreen(t *testing.T) {
	w := New(cannedPath(cannedAnswers{}), io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	base := NewSource(w, nil)
	withScreen := base.WithPINEntry(func([]byte, pkcs11.PINRequest) (int, error) { return 0, nil })

	if base.entry != nil {
		t.Error("WithPINEntry mutated the source it was called on")
	}
	if withScreen.entry == nil {
		t.Error("the copy has no PIN entry")
	}
	if base.w != withScreen.w {
		t.Error("the copy has a different worker; the child must be shared, " +
			"or every signature pays for a new process")
	}
}
