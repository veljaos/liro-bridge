package worker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// The tests in this file are the parent's half of the PIN exchange, against
// real spawned processes and real pipes. The child's half is in
// pinexchange_test.go, in process, where the byte-level properties can be
// decided.
//
// What none of them establish is that the PIN reaches a card, because there is
// no card here. What they establish is everything between the screen and the
// pipe: that the PIN arrives intact and once, that it is not collected when it
// would be refused, that the buffer is overwritten, and that the parent refuses
// a question it did not ask for. The card's own half is D-268's territory and
// costs one of three attempts to measure.

// closing bounds a test's own teardown.
//
// Worker.Close deliberately chooses no duration — the caller's context is the
// bound, which is D-297's instruction — so context.Background() means "wait for
// ever". That is the right answer for a caller who wants one and the wrong one
// for a test whose child is deliberately misbehaving, and the difference was
// measured rather than imagined: under a mutation that made the parent accept a
// PIN question it had not asked for, the rogue child never answered the
// shutdown and teardown wedged until `go test`'s own ten-minute timeout, once
// per mutation.
//
// It is not a deadline on a property (D-201) and is not asserted on. It is a
// test choosing how long to wait for a process it spawned, which a test may do
// and the product may not.
func closing(t *testing.T, w *Worker) func() {
	t.Helper()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = w.Close(ctx)
	}
}

// typing returns a PINEntry that writes one PIN and counts how many times it
// was asked.
//
// The counter is the point rather than the PIN: SPEC §6.5.1 clause 5 says
// nothing retries a PIN automatically, ever, for any reason, and a screen shown
// twice is the observable form of a retry.
func typing(pin string, calls *int) pkcs11.PINEntry {
	return func(dst []byte, _ pkcs11.PINRequest) (int, error) {
		*calls++
		return copy(dst, pin), nil
	}
}

// askingCard is a canned child that wants a PIN, and the PIN it wants.
const askingCardPIN = "12345"

func askingCard(a cannedAnswers) cannedAnswers {
	a.Needs = LoginNeeds{
		MinPINLength:     4,
		MaxPINLength:     8,
		TokenLabel:       "SAVKA ODŽIĆ 200100123",
		TokenSerial:      "0123456789",
		CertificateLabel: "Sertifikat za potpis",
	}
	a.WantPIN = askingCardPIN
	return a
}

// TestThePINReachesTheWorkerIntactAndTheSessionSigns is the seam end to end,
// across a process boundary, which is the only place it exists at all.
//
// The child checks the PIN against what the test put on its canned card, so a
// success here means the exact bytes the screen produced arrived — not that a
// login happened, which is a much weaker thing and is what a test that only
// looked at the error would establish.
func TestThePINReachesTheWorkerIntactAndTheSessionSigns(t *testing.T) {
	var stderr bytes.Buffer
	w := New(cannedPath(askingCard(cannedAnswers{Label: "card"})), &stderr)
	t.Cleanup(closing(t, w))

	ctx := context.Background()
	screens := 0

	sess, err := w.Open(ctx, "ABCDEF", typing(askingCardPIN, &screens))
	if err != nil {
		t.Fatalf("Open: %v\nchild stderr:\n%s", err, stderr.String())
	}
	if screens != 1 {
		t.Errorf("the PIN screen was shown %d times, want 1", screens)
	}
	if got := sess.Certificate().Thumbprint; got != "ABCDEF" {
		t.Errorf("the session's certificate is %q, want the one that was asked for", got)
	}
	if len(sess.Certificate().DER) == 0 {
		t.Error("the session's certificate has no bytes")
	}
	if sess.Certificate().IsTestKey {
		t.Error("a worker's certificate is marked as a test key, which would " +
			"un-mark a signature SPEC §16.6 requires be marked")
	}

	digest := bytes.Repeat([]byte{7}, 32)
	sig, err := sess.SignDigest(ctx, keysource.DigestSHA256, digest)
	if err != nil {
		t.Fatalf("SignDigest: %v\nchild stderr:\n%s", err, stderr.String())
	}
	// The canned child answers with the algorithm byte followed by the digest,
	// so this says the digest crossed intact and the algorithm with it.
	if len(sig) != 1+len(digest) || sig[0] != byte(keysource.DigestSHA256) {
		t.Errorf("the signature came back as %d bytes beginning %#x", len(sig), sig[0])
	}
	if !bytes.Equal(sig[1:], digest) {
		t.Error("the digest did not cross intact")
	}

	if err := sess.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// The module is still loaded: closing a session logs the card out and
	// nothing more, which is the whole reason a worker is held open (D-297).
	if _, err := w.List(ctx); err != nil {
		t.Errorf("the worker stopped answering after its session closed: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("the child wrote to its standard error:\n%s", stderr.String())
	}
}

// TestATokenWithNoPINRequirementIsOpenedWithoutAScreen is SPEC §6.5.1 clause 1
// from the parent's side, and it is the reason the exchange has two phases.
//
// The parent cannot know in advance whether a PIN is needed: only the child can
// see CKF_PROTECTED_AUTHENTICATION_PATH, per token, every time. A parent that
// collected one first would have drawn a screen for a card that was never going
// to be asked — and this program's PIN screen deliberately looks like a system
// dialog (D-277), so a spurious one is worse than an inconvenience.
func TestATokenWithNoPINRequirementIsOpenedWithoutAScreen(t *testing.T) {
	var stderr bytes.Buffer
	// No Needs, so the canned child never calls ask.
	w := New(cannedPath(cannedAnswers{Label: "pinpad"}), &stderr)
	t.Cleanup(closing(t, w))

	screens := 0
	sess, err := w.Open(context.Background(), "ABCDEF", typing("unused", &screens))
	if err != nil {
		t.Fatalf("Open: %v\nchild stderr:\n%s", err, stderr.String())
	}
	t.Cleanup(func() { _ = sess.Close() })

	if screens != 0 {
		t.Errorf("a PIN screen was shown %d times for a token that never asked", screens)
	}
}

// TestAWrongPINIsOneAttemptAndIsNotRetried is clause 5, which is the clause
// with a consequence measured in visits to a police station.
//
// "Nothing retries a PIN automatically, ever, for any reason. One wrong PIN is
// one attempt. Three block the card."
//
// The canned child refuses the PIN the way a card refuses one — an answer, not
// an ending — and what must follow is one screen and one failure. A supervisor
// that treated a refused login like a failed listing would show three.
func TestAWrongPINIsOneAttemptAndIsNotRetried(t *testing.T) {
	var stderr bytes.Buffer
	w := New(cannedPath(askingCard(cannedAnswers{Label: "card"})), &stderr)
	t.Cleanup(closing(t, w))

	screens := 0
	_, err := w.Open(context.Background(), "ABCDEF", typing("00000", &screens))
	if err == nil {
		t.Fatal("a wrong PIN opened a session")
	}
	if !strings.Contains(err.Error(), "CKR_PIN_INCORRECT") {
		t.Errorf("the failure does not carry the card's own reason: %v", err)
	}
	if screens != 1 {
		t.Errorf("the PIN screen was shown %d times, want exactly 1.\n\n"+
			"SPEC §6.5.1 clause 5: nothing retries a PIN automatically, ever, for "+
			"any reason. One wrong PIN is one of three attempts.", screens)
	}
}

// TestALoginIsNeverRetriedAgainstAFreshWorker is the same clause one level up,
// where it would be easiest to break by accident.
//
// The read operations are retried against a respawned worker, and that is right:
// they ask the same question of a freshly opened module and get the same answer.
// A login does not, and the reason it cannot is not squeamishness — a parent
// whose worker died during a login cannot tell whether C_Login ran, because the
// process that knows is gone. Retrying might cost nothing or might cost the
// second of three attempts, and there is no way to find out which.
func TestALoginIsNeverRetriedAgainstAFreshWorker(t *testing.T) {
	var stderr bytes.Buffer
	// The child ends instead of answering its first request, which is the login.
	w := New(cannedPath(askingCard(cannedAnswers{Label: "card", DieOnRequest: 1})), &stderr)
	t.Cleanup(closing(t, w))

	screens := 0
	_, err := w.Open(context.Background(), "ABCDEF", typing(askingCardPIN, &screens))
	if err == nil {
		t.Fatal("Open succeeded against a child that ended without answering")
	}
	if !errors.Is(err, ErrWorkerDied) {
		t.Errorf("Open: %v, want ErrWorkerDied", err)
	}
	if screens != 0 {
		t.Errorf("the PIN screen was shown %d times against a child that died "+
			"before it could ask", screens)
	}
	if got := strings.Count(stderr.String(), cannedDeathLine); got != 1 {
		t.Errorf("%d children died, want 1: a login is not retried against a "+
			"respawned worker", got)
	}
	if retryable[OpLogin] {
		t.Error("OpLogin is in the retryable allow-list")
	}
}

// TestACancelledPINScreenIsNotAFailure is the distinction the folder chooser
// already draws and that D-145 had to make for a window closed before its first
// payload: a person who closed the screen did not fail at anything, and
// reporting it as a failure is how a program tells somebody off for changing
// their mind.
func TestACancelledPINScreenIsNotAFailure(t *testing.T) {
	var stderr bytes.Buffer
	w := New(cannedPath(askingCard(cannedAnswers{Label: "card"})), &stderr)
	t.Cleanup(closing(t, w))

	cancelled := pkcs11.PINEntry(func([]byte, pkcs11.PINRequest) (int, error) {
		return 0, pkcs11.ErrPINCancelled
	})

	_, err := w.Open(context.Background(), "ABCDEF", cancelled)
	if !errors.Is(err, pkcs11.ErrPINCancelled) {
		t.Fatalf("Open: %v, want it to carry ErrPINCancelled", err)
	}

	// And the worker was ended rather than left waiting for a PIN that is not
	// coming. The next request starts a fresh one and is answered.
	if _, err := w.List(context.Background()); err != nil {
		t.Errorf("the worker did not recover after a cancelled screen: %v", err)
	}
}

// TestAPINThatCannotBeRightNeverReachesThePipe is SPEC §6.5.1 clause 7, and
// D-268 is what it cost to learn that it is nobody else's job.
//
// One authorised C_Login with a NULL PIN on a MUP token: the module passed it to
// the card as an empty PIN, the card rejected it as a wrong PIN, and one of
// three attempts was gone. The token had declared minPin=4 the whole time and
// the module range-checked nothing. A declared limit describes what the card
// accepts, not what the module enforces.
//
// So the check is not "the login failed" — it is that nothing was written. This
// is measured against a buffer rather than a process, because "how many bytes
// left this parent" is the question and a pipe does not answer it.
func TestAPINThatCannotBeRightNeverReachesThePipe(t *testing.T) {
	needs := LoginNeeds{MinPINLength: 4, MaxPINLength: 8, Exchange: "EX1"}

	for _, tc := range []struct {
		name string
		pin  string
	}{
		{"nothing at all, which is D-268's own case", ""},
		{"shorter than the token accepts", "123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pipe bytes.Buffer
			calls := 0
			err := sendPIN(&pipe, "EX1", needs, typing(tc.pin, &calls), "C:\\module.dll")

			var lengthErr *pkcs11.PINLengthError
			if !errors.As(err, &lengthErr) {
				t.Fatalf("sendPIN: %v, want a PINLengthError", err)
			}
			if lengthErr.Got != len(tc.pin) || lengthErr.Min != 4 || lengthErr.Max != 8 {
				t.Errorf("the refusal says %d against %d..%d", lengthErr.Got, lengthErr.Min, lengthErr.Max)
			}
			if pipe.Len() != 0 {
				t.Errorf("%d bytes were written for a PIN that cannot be right.\n\n"+
					"SPEC §6.5.1 clause 7: a PIN that cannot possibly be right must not "+
					"reach the card, because reaching it costs one of three attempts "+
					"(D-268).", pipe.Len())
			}
			if calls != 1 {
				t.Errorf("the screen was shown %d times, want 1", calls)
			}
		})
	}
}

// TestTheParentsPINBufferIsOverwrittenBeforeSendPINReturns is clause 2's
// parent-side half: the PIN exists for the duration of the write and is
// overwritten immediately afterwards.
//
// The buffer is sendPIN's own local, so the only way to see it from outside is
// to keep the slice the entry was handed — which aliases the same backing array
// rather than copying it, so what is read back afterwards is the array the wipe
// ran over. That is the same trick wipe_test.go uses one package over, and for
// the same reason: a loop that was elided and a loop that ran look identical
// through anything else.
//
// # What it does not establish
//
// That the bytes are unreachable — a copy the compiler made on the stack, or a
// page the operating system has since written to swap, is not visible from
// here and never will be. What it establishes is that the one buffer this layer
// owns is zero by the time the function returns, on the success path and on
// every failure path.
func TestTheParentsPINBufferIsOverwrittenBeforeSendPINReturns(t *testing.T) {
	needs := LoginNeeds{MinPINLength: 4, MaxPINLength: 8, Exchange: "EX1"}

	for _, tc := range []struct {
		name string
		pin  string
		fail error
	}{
		{name: "after a PIN that was written", pin: "12345"},
		{name: "after a length that was refused", pin: "1"},
		{name: "after a screen that failed", pin: "12345", fail: errors.New("the screen broke")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var held []byte
			entry := pkcs11.PINEntry(func(dst []byte, _ pkcs11.PINRequest) (int, error) {
				held = dst // aliased, not copied
				if tc.fail != nil {
					return 0, tc.fail
				}
				return copy(dst, tc.pin), nil
			})

			var pipe bytes.Buffer
			_ = sendPIN(&pipe, "EX1", needs, entry, "C:\\module.dll")

			if held == nil {
				t.Fatal("the entry was never called, so there is no buffer to look at")
			}
			for i, b := range held {
				if b != 0 {
					t.Fatalf("byte %d of the PIN buffer is %#x after sendPIN returned; "+
						"SPEC §6.5.1 clause 2 requires it be overwritten immediately", i, b)
				}
			}
		})
	}
}

// TestThePINIsWrittenOnceBehindTheFrameThatNamesIt is the wire format of clause
// 2: the frame says how many bytes follow, and exactly that many follow, with
// nothing between them and nothing after.
func TestThePINIsWrittenOnceBehindTheFrameThatNamesIt(t *testing.T) {
	const pin = "123456"
	needs := LoginNeeds{MinPINLength: 4, MaxPINLength: 8, Exchange: "EX1"}

	var pipe bytes.Buffer
	calls := 0
	if err := sendPIN(&pipe, "EX1", needs, typing(pin, &calls), "C:\\module.dll"); err != nil {
		t.Fatalf("sendPIN: %v", err)
	}

	var frame Request
	if err := ReadFrame(&pipe, &frame); err != nil {
		t.Fatalf("reading the PIN frame back: %v", err)
	}
	if frame.Op != OpLoginPIN {
		t.Errorf("the frame is %q, want %q", frame.Op, OpLoginPIN)
	}
	if frame.Exchange != "EX1" {
		t.Errorf("the frame carries exchange %q", frame.Exchange)
	}
	if frame.PINLength != len(pin) {
		t.Fatalf("the frame names %d bytes, want %d", frame.PINLength, len(pin))
	}

	// Everything left is the PIN and only the PIN.
	rest := pipe.Bytes()
	if string(rest) != pin {
		t.Errorf("what followed the frame is %q, want %q", rest, pin)
	}
}

// TestAPINRequestWithNothingPendingIsRefusedLoudly is the owner's second
// condition, and it is deliberately produced by a child that does not use
// Serve: no correct child can do this, which is the point.
//
// "A PIN request arriving when nothing is pending is a defect rather than a
// no-op. Say so loudly — it means either the worker is confused or something
// else is talking to the parent, and both are worth knowing about."
//
// A listing is asked for. The answer is a PIN question. There is no approved
// operation behind it and no entry to collect one with, and the parent must
// refuse rather than find a way to answer.
func TestAPINRequestWithNothingPendingIsRefusedLoudly(t *testing.T) {
	var stderr bytes.Buffer
	w := New(cannedPath(askingCard(cannedAnswers{AskUnbidden: true})), &stderr)
	t.Cleanup(closing(t, w))

	_, err := w.List(context.Background())
	if !errors.Is(err, ErrUnexpectedPINRequest) {
		t.Fatalf("List against a worker that asked for a PIN: %v, want ErrUnexpectedPINRequest", err)
	}
	if !strings.Contains(err.Error(), string(OpList)) {
		t.Errorf("the refusal does not say what was being asked at the time: %v", err)
	}

	// It is not retried against a respawned worker either. A listing is
	// retryable, so without the refusal ending the attempt this would be three
	// PIN questions rather than one.
	if !retryable[OpList] {
		t.Fatal("OpList is not retryable, so this test is not measuring what it says")
	}
}

// TestAPINRequestAboutAnotherExchangeIsRefused is the owner's first condition:
// the binding is to the operation a person approved, not to the worker being
// alive.
//
// The child here is in the middle of a login this parent did ask for, and asks
// its question about a different exchange. That is not a question about the
// approved operation, and honouring it would make "can make a PIN dialog
// appear" a thing the far end can do at will — with a dialog that deliberately
// looks like a system one (D-277, SPEC §10).
func TestAPINRequestAboutAnotherExchangeIsRefused(t *testing.T) {
	var stderr bytes.Buffer
	w := New(cannedPath(askingCard(cannedAnswers{AskWithExchange: "NOT-THE-ONE"})), &stderr)
	t.Cleanup(closing(t, w))

	screens := 0
	_, err := w.Open(context.Background(), "ABCDEF", typing(askingCardPIN, &screens))
	if !errors.Is(err, ErrUnexpectedPINRequest) {
		t.Fatalf("Open: %v, want ErrUnexpectedPINRequest", err)
	}
	if screens != 0 {
		t.Errorf("the PIN screen was shown %d times for an exchange this parent "+
			"did not open", screens)
	}
}

// TestASecondPINQuestionIsRefused is clause 5 at the protocol level. A worker
// that could ask twice could ask three times, and three is the number that ends
// with a visit to a police station.
func TestASecondPINQuestionIsRefused(t *testing.T) {
	var stderr bytes.Buffer
	w := New(cannedPath(askingCard(cannedAnswers{Label: "card", AskTwice: true})), &stderr)
	t.Cleanup(closing(t, w))

	screens := 0
	_, err := w.Open(context.Background(), "ABCDEF", typing(askingCardPIN, &screens))
	if !errors.Is(err, ErrUnexpectedPINRequest) {
		t.Fatalf("Open: %v, want ErrUnexpectedPINRequest", err)
	}
	if screens != 1 {
		t.Errorf("the PIN screen was shown %d times, want exactly 1", screens)
	}
}

// TestALoginWithNoPINEntryIsRefusedRatherThanAttempted covers the wiring
// mistake: a token that needs a PIN and a parent with nothing to collect one
// with. It is a sentinel rather than a generic failure because the layer above
// has to tell it from a card refusing a PIN, which is an entirely different
// thing to tell a person.
func TestALoginWithNoPINEntryIsRefusedRatherThanAttempted(t *testing.T) {
	var stderr bytes.Buffer
	w := New(cannedPath(askingCard(cannedAnswers{Label: "card"})), &stderr)
	t.Cleanup(closing(t, w))

	_, err := w.Open(context.Background(), "ABCDEF", nil)
	if !errors.Is(err, pkcs11.ErrNoPINEntry) {
		t.Fatalf("Open with no PIN entry: %v, want ErrNoPINEntry", err)
	}
}

// TestEveryExchangeIdentifierIsDifferent is what makes the binding bind to an
// operation rather than to the conversation. Two logins in a row must not be
// able to answer each other's questions.
func TestEveryExchangeIdentifierIsDifferent(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		id, err := newExchangeID()
		if err != nil {
			t.Fatalf("newExchangeID: %v", err)
		}
		if len(id) != exchangeIDBytes*2 {
			t.Fatalf("an identifier is %d characters, want %d", len(id), exchangeIDBytes*2)
		}
		if seen[id] {
			t.Fatalf("%q came up twice in %d mints", id, i+1)
		}
		seen[id] = true
	}
}
