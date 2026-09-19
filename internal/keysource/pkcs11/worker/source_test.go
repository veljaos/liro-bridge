package worker

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// TestSourceIsAKeySourceAndSaysTheSameNameAsTheInProcessBackend is the point of
// the type: the agent holds a keysource.Source and does not know which backend
// it has.
//
// The name equality is not decoration. If the worker-backed source called
// itself something else, every audit record, every log line and every report
// written while the agent used it would say a different backend from the ones
// written before, for the same card and the same module — and the difference
// would record which release the user was on rather than anything about their
// certificate.
func TestSourceIsAKeySourceAndSaysTheSameNameAsTheInProcessBackend(t *testing.T) {
	var s keysource.Source = NewSource(New(cannedPath(cannedAnswers{}), io.Discard), nil)
	if got, want := s.Name(), pkcs11.NewSource("").Name(); got != want {
		t.Fatalf("worker-backed source calls itself %q; the in-process backend calls itself %q.\n"+
			"They are the same backend and must answer the same name.", got, want)
	}
	if s.Name() != "pkcs11" {
		t.Errorf("Name() = %q, want %q", s.Name(), "pkcs11")
	}
}

// TestListDerivesTheThumbprintTheRestOfTheAgentUses runs a real child over a
// real pipe — the canned server, not a mock — and checks the one value F11 §4
// step 3 collapses rows on.
func TestListDerivesTheThumbprintTheRestOfTheAgentUses(t *testing.T) {
	w := New(cannedPath(cannedAnswers{}), io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	certs, err := NewSource(w, nil).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("List returned %d certificates, want 1", len(certs))
	}

	// Computed here from the bytes the canned child sends, rather than taken
	// from the thing under test. A test that asks the code what the answer is
	// and then agrees with it is D-296's first question answered badly.
	sum := sha1.Sum([]byte{0x30, 0x02})
	want := keysource.Thumbprint(strings.ToUpper(hex.EncodeToString(sum[:])))
	if certs[0].Thumbprint != want {
		t.Errorf("Thumbprint = %q, want %q", certs[0].Thumbprint, want)
	}
	if string(certs[0].DER) != string([]byte{0x30, 0x02}) {
		t.Errorf("DER = % x, want 30 02", certs[0].DER)
	}
	if certs[0].IsTestKey {
		t.Error("IsTestKey is true for a certificate read through a real module; " +
			"that flag marks the soft token (SPEC §16.6) and nothing else")
	}
}

// TestACertificateWithNoBytesIsSkippedRatherThanGivenTheThumbprintOfNothing is
// the positive control for the guard in List.
//
// Without the guard every empty payload would become a row identified by
// SHA-1("") — da39a3ee… — so two of them would be the same certificate and one
// of them might be openable. The failure is not "a useless row"; it is a
// collision between things that are not related.
func TestACertificateWithNoBytesIsSkippedRatherThanGivenTheThumbprintOfNothing(t *testing.T) {
	// Exercised against the conversion directly, because no correct child sends
	// an empty certificate and the canned one cannot be asked to: adding a
	// switch to the child for it would be a seam whose only user is this test
	// (D-100), which is the thing the canned harness exists to avoid.
	src := Source{}
	got := src.convert([]CertificatePayload{
		{DER: []byte{0x30, 0x02}},
		{DER: nil},
		{DER: []byte{}},
		{DER: []byte{0x30, 0x03}},
	})
	if len(got) != 2 {
		t.Fatalf("convert kept %d of 4 payloads, want 2 (the two with bytes)", len(got))
	}
	empty := sha1.Sum(nil)
	never := keysource.Thumbprint(strings.ToUpper(hex.EncodeToString(empty[:])))
	for _, c := range got {
		if c.Thumbprint == never {
			t.Fatalf("a row was identified by the SHA-1 of no bytes (%s), which is the "+
				"collision the guard exists to prevent", never)
		}
	}
}

// TestOpenPassesThePINEntryThroughAndTheWholeExchangeRuns is the seam this type
// exists to join, measured end to end against a child that requires a
// particular PIN and refuses any other the way a card does.
func TestOpenPassesThePINEntryThroughAndTheWholeExchangeRuns(t *testing.T) {
	w := New(cannedPath(askingCard(cannedAnswers{Label: "card"})), io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	asked := 0
	entry := func(dst []byte, req pkcs11.PINRequest) (int, error) {
		asked++
		if req.TokenLabel != "SAVKA ODŽIĆ 200100123" {
			t.Errorf("the entry was asked about token %q, want the canned card's label", req.TokenLabel)
		}
		if req.ModulePath == "" {
			t.Error("the entry was asked with no module path; it is the only thing that " +
				"tells two modules over one card apart (D-271)")
		}
		return copy(dst, askingCardPIN), nil
	}

	sess, err := NewSource(w, entry).Open(context.Background(), keysource.Thumbprint("ABCD"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	if asked != 1 {
		t.Errorf("the PIN entry was called %d times, want exactly 1. "+
			"SPEC §6.5.1 clause 5: nothing retries a PIN, ever.", asked)
	}
	if len(sess.Certificate().DER) == 0 {
		t.Error("the session carries no certificate")
	}
}

// TestOpenWithNoPINEntryFailsOnlyWhenTheTokenAsks keeps clause 1's branch open.
//
// A token with a protected authentication path collects the PIN itself and this
// program never sees one, so a Source with no entry must work against it. The
// two halves are asserted together because the mistake this guards against is
// a nil check at the top of Open, which would look correct and would make
// clause 1 — the branch the SPEC *prefers* — unreachable.
func TestOpenWithNoPINEntryFailsOnlyWhenTheTokenAsks(t *testing.T) {
	t.Run("a token that does not ask opens with no entry", func(t *testing.T) {
		w := New(cannedPath(cannedAnswers{}), io.Discard) // Needs.MaxPINLength == 0: never asks
		t.Cleanup(func() { _ = w.Close(context.Background()) })

		sess, err := NewSource(w, nil).Open(context.Background(), keysource.Thumbprint("ABCD"))
		if err != nil {
			t.Fatalf("Open against a protected-authentication-path token with no PIN entry: %v\n"+
				"SPEC §6.5.1 clause 1 is the branch this must not break.", err)
		}
		_ = sess.Close()
	})

	t.Run("a token that asks fails with ErrNoPINEntry", func(t *testing.T) {
		w := New(cannedPath(askingCard(cannedAnswers{})), io.Discard)
		t.Cleanup(func() { _ = w.Close(context.Background()) })

		_, err := NewSource(w, nil).Open(context.Background(), keysource.Thumbprint("ABCD"))
		if !errors.Is(err, pkcs11.ErrNoPINEntry) {
			t.Fatalf("Open = %v, want an error that is pkcs11.ErrNoPINEntry", err)
		}
	})
}

// TestTheZeroSourceAnswersRatherThanPanics. Go makes Source{} reachable whatever
// NewSource does, and a nil-pointer panic inside a keysource.Source reaches the
// agent as a crash rather than as a Failure with a module path on it (F11 §3).
func TestTheZeroSourceAnswersRatherThanPanics(t *testing.T) {
	var s Source
	if _, err := s.List(context.Background()); err == nil {
		t.Error("List on the zero Source returned no error")
	}
	if _, err := s.Open(context.Background(), keysource.Thumbprint("ABCD")); err == nil {
		t.Error("Open on the zero Source returned no error")
	}
	if err := s.Close(context.Background()); err != nil {
		t.Errorf("Close on the zero Source: %v, want nil — there is nothing to close", err)
	}
	if s.ModulePath() != "" {
		t.Errorf("ModulePath on the zero Source = %q, want empty", s.ModulePath())
	}
	if s.Name() != "pkcs11" {
		t.Errorf("Name on the zero Source = %q; the name is a property of the backend, "+
			"not of whether this instance was built correctly", s.Name())
	}
}
