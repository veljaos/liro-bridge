package worker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// The tests in this file exercise the child's half of the PIN exchange against
// a reader and a writer, with no process, no module and no card — which is
// every machine CI runs on, and which is where the properties SPEC §6.5.1
// clause 2 is about can actually be decided.
//
// The parent's half is in login_test.go, against real spawned children.

// conversation builds what a parent writes: frames, and after a PIN frame, the
// raw bytes that follow it.
//
// It exists because the PIN is the one thing on this pipe that is not a frame,
// and a test helper that could only write frames would be unable to express the
// case the whole protocol is shaped around.
func conversation(t *testing.T, parts ...any) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	for _, part := range parts {
		switch p := part.(type) {
		case Request:
			if err := WriteFrame(&buf, p); err != nil {
				t.Fatalf("WriteFrame: %v", err)
			}
		case []byte:
			buf.Write(p)
		case string:
			buf.WriteString(p)
		default:
			t.Fatalf("conversation: cannot write a %T", part)
		}
	}
	return &buf
}

// askingToken is a fake token that wants a PIN of between 4 and 8 characters,
// which is what a MUP card declares (D-268).
var askingToken = LoginNeeds{
	MinPINLength:     4,
	MaxPINLength:     8,
	TokenLabel:       "SAVKA ODŽIĆ 200100123",
	TokenSerial:      "0123456789",
	CertificateLabel: "Sertifikat za potpis",
}

// TestATokenWithAProtectedAuthenticationPathIsNeverAskedForAPIN is SPEC §6.5.1
// clause 1, and it is the reason the exchange has two phases at all.
//
// Where a module advertises a protected authentication path, the module or the
// reader collects the PIN and this program passes NULL. The parent cannot know
// that in advance — only the child can see the flag, it is a property of a
// token through a module read per token every time (D-273, D-276), and a reader
// with a pinpad answers differently through the same DLL. So the login either
// asks or does not, and the parent finds out by being asked.
//
// Here the handler never calls ask, and what must follow is that no question
// reaches the pipe at all: one request, one answer, and the answer is the
// finished login.
func TestATokenWithAProtectedAuthenticationPathIsNeverAskedForAPIN(t *testing.T) {
	h := &fakeHandler{loginCert: CertificatePayload{DER: []byte{0x30, 0x01}}}
	// needs is left zero, so fakeHandler.Login does not call ask — which is
	// what a protected authentication path looks like from this side.

	in := conversation(t, Request{Op: OpLogin, Thumbprint: "ABCD", Exchange: "EX1"})
	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	got := responses(t, &out)
	if len(got) != 1 {
		t.Fatalf("got %d responses for one login, want 1", len(got))
	}
	if got[0].Login != nil {
		t.Error("a PIN question was written for a token that never asked for one")
	}
	if got[0].Err != "" {
		t.Fatalf("the login was refused: %s", got[0].Err)
	}
	if got[0].Certificate == nil || len(got[0].Certificate.DER) == 0 {
		t.Error("the login answered with no certificate")
	}
	if h.collected != -1 {
		t.Errorf("the handler collected %d bytes from a token with a protected "+
			"authentication path", h.collected)
	}
}

// TestThePINGoesStraightIntoTheBufferTheLoginWillUse is clause 2's child half.
//
// The bytes behind the OpLoginPIN frame are read by one io.ReadFull into the
// buffer the caller handed in — which in the real child is the buffer login
// allocated, pinned, and will pass to C_Login and then overwrite. Nothing
// between the pipe and that buffer holds a copy, and there is nowhere for one
// to be: ask has no intermediate slice.
func TestThePINGoesStraightIntoTheBufferTheLoginWillUse(t *testing.T) {
	const pin = "1234"
	h := &fakeHandler{needs: askingToken, loginCert: CertificatePayload{DER: []byte{0x30, 0x02}}}

	// What the handler was given, captured by aliasing rather than copying, so
	// the test reads the same backing array the read wrote into.
	var given []byte
	h.onCall = nil
	h.loginFail = nil
	in := conversation(t,
		Request{Op: OpLogin, Thumbprint: "ABCD", Exchange: "EX1"},
		Request{Op: OpLoginPIN, Exchange: "EX1", PINLength: len(pin)},
		[]byte(pin),
	)

	var out bytes.Buffer
	watcher := &recordingHandler{fakeHandler: h, sawBuffer: func(b []byte) { given = b }}
	if err := Serve(context.Background(), in, &out, watcher); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	got := responses(t, &out)
	if len(got) != 2 {
		t.Fatalf("got %d frames, want 2: the question and the answer", len(got))
	}
	if got[0].Login == nil {
		t.Fatal("the first frame is not a PIN question")
	}
	if got[0].Login.Exchange != "EX1" {
		t.Errorf("the question carries exchange %q, want the one the request did", got[0].Login.Exchange)
	}
	if got[0].Login.MinPINLength != askingToken.MinPINLength || got[0].Login.MaxPINLength != askingToken.MaxPINLength {
		t.Errorf("the question carries %d..%d, want the token's own %d..%d",
			got[0].Login.MinPINLength, got[0].Login.MaxPINLength,
			askingToken.MinPINLength, askingToken.MaxPINLength)
	}
	if got[0].Login.TokenLabel != askingToken.TokenLabel {
		t.Errorf("the question does not name the card: %q", got[0].Login.TokenLabel)
	}

	if got[1].Err != "" {
		t.Fatalf("the login was refused: %s", got[1].Err)
	}
	if h.collected != len(pin) {
		t.Fatalf("the handler was told %d bytes arrived, want %d", h.collected, len(pin))
	}
	if string(given[:h.collected]) != pin {
		t.Errorf("the buffer holds %q, want %q", given[:h.collected], pin)
	}
	// The rest of the buffer is untouched: ReadFull was given dst[:n] and not
	// dst, so a shorter PIN does not consume bytes that are not there.
	for i := h.collected; i < len(given); i++ {
		if given[i] != 0 {
			t.Errorf("byte %d of the buffer is %#x; the read went past the PIN", i, given[i])
			break
		}
	}
}

// recordingHandler hands a test the buffer ask was called with, by aliasing it
// rather than copying it — a copy would be a second PIN, which is the thing
// this whole file is about.
type recordingHandler struct {
	*fakeHandler
	sawBuffer func([]byte)
}

func (r *recordingHandler) Login(ctx context.Context, thumbprint string, ask PINExchange) (CertificatePayload, [][]byte, error) {
	wrapped := PINExchange(func(dst []byte, needs LoginNeeds) (int, error) {
		r.sawBuffer(dst)
		return ask(dst, needs)
	})
	return r.fakeHandler.Login(ctx, thumbprint, wrapped)
}

// TestThePINIsNotReadUntilTheLoginAsksForIt is the property that makes one pipe
// sufficient, measured at the moment it matters.
//
// The PIN is already in the pipe when the login request is handed to the
// handler — that is what "one write, read immediately" means from the parent's
// side, and it is why a buffered reader would be fatal here rather than merely
// untidy. What must be true is that not one byte of it has been consumed until
// ask does the consuming: anything the loop reads ahead into is a copy nothing
// will ever overwrite, and it would be invisible to pin_test.go because it
// would be somebody else's byte slice.
//
// TestTheLoopReadsNoFurtherThanTheRequestItIsServing asserts this with a second
// request standing in for the PIN. This asserts it with the PIN itself, which
// is the case the stand-in was standing in for.
func TestThePINIsNotReadUntilTheLoginAsksForIt(t *testing.T) {
	const pin = "12345"

	var loginFrame bytes.Buffer
	if err := WriteFrame(&loginFrame, Request{Op: OpLogin, Thumbprint: "ABCD", Exchange: "EX1"}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	loginLen := loginFrame.Len()

	in := conversation(t,
		Request{Op: OpLogin, Thumbprint: "ABCD", Exchange: "EX1"},
		Request{Op: OpLoginPIN, Exchange: "EX1", PINLength: len(pin)},
		[]byte(pin),
	)
	counted := &countingReader{r: in}

	readWhenLoginBegan := -1
	h := &fakeHandler{needs: askingToken, loginCert: CertificatePayload{DER: []byte{0x30, 0x03}}}
	h.onCall = func() { readWhenLoginBegan = counted.n }

	var out bytes.Buffer
	if err := Serve(context.Background(), counted, &out, h); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	if readWhenLoginBegan < 0 {
		t.Fatal("the handler was never called, so nothing was observed")
	}
	if readWhenLoginBegan != loginLen {
		t.Errorf("at the moment the login began, %d bytes had been read; the login "+
			"frame is %d and the PIN was already in the pipe behind it.\n\n"+
			"Anything read ahead of the question is a copy of the PIN that nothing "+
			"will overwrite (SPEC §6.5.1 clause 2).", readWhenLoginBegan, loginLen)
	}
	if h.collected != len(pin) {
		t.Errorf("the PIN did not arrive: %d bytes, want %d", h.collected, len(pin))
	}
}

// TestTheReadStopsExactlyAtTheEndOfThePIN is the boundary, from the other side.
//
// A read that took one byte too few would leave a stray byte to be read as the
// next frame's header; one byte too many would eat the frame behind it. Either
// way the pipe is out of step, and the bytes it is out of step about are a PIN.
// The check is that an ordinary request written immediately behind the PIN is
// served normally.
func TestTheReadStopsExactlyAtTheEndOfThePIN(t *testing.T) {
	const pin = "1234567"
	h := &fakeHandler{
		needs:     askingToken,
		loginCert: CertificatePayload{DER: []byte{0x30, 0x04}},
		list:      []CertificatePayload{{DER: []byte{0x30, 0x05}}},
	}

	in := conversation(t,
		Request{Op: OpLogin, Thumbprint: "ABCD", Exchange: "EX1"},
		Request{Op: OpLoginPIN, Exchange: "EX1", PINLength: len(pin)},
		[]byte(pin),
		Request{Op: OpList},
	)

	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v — the pipe was out of step after the PIN", err)
	}
	got := responses(t, &out)
	if len(got) != 3 {
		t.Fatalf("got %d frames, want 3: question, login answer, list answer", len(got))
	}
	if got[2].Err != "" || len(got[2].Certificates) != 1 {
		t.Errorf("the request behind the PIN was not served: %+v", got[2])
	}
	if h.lists != 1 {
		t.Errorf("the module was asked to list %d times, want 1", h.lists)
	}
}

// TestAPINAnswerAboutAnotherExchangeEndsTheWorker is the owner's first
// condition at the child's end.
//
// The binding is to the operation a person approved, and an answer carrying a
// different identifier is not an answer to anything this child asked. It ends
// the worker rather than becoming a refusal, because the bytes behind that
// frame are a PIN and there is no longer any way to know where the next frame
// starts.
func TestAPINAnswerAboutAnotherExchangeEndsTheWorker(t *testing.T) {
	h := &fakeHandler{needs: askingToken}
	in := conversation(t,
		Request{Op: OpLogin, Thumbprint: "ABCD", Exchange: "EX1"},
		Request{Op: OpLoginPIN, Exchange: "SOMETHING-ELSE", PINLength: 4},
		[]byte("1234"),
	)

	var out bytes.Buffer
	err := Serve(context.Background(), in, &out, h)
	if !errors.Is(err, ErrProtocolDesync) {
		t.Fatalf("Serve: %v, want ErrProtocolDesync", err)
	}
	if h.collected != 0 {
		t.Errorf("the handler was told %d bytes arrived; no PIN may be read for an "+
			"exchange this child did not open", h.collected)
	}

	// The question was written and nothing after it. A refusal written here
	// would be a frame the parent reads as the answer to its login.
	got := responses(t, &out)
	if len(got) != 1 || got[0].Login == nil {
		t.Errorf("got %d frames, want only the question", len(got))
	}
}

// TestAFrameThatIsNotAPINAnswerEndsTheWorker covers the rest of the shape: any
// other operation arriving where the PIN answer belongs.
func TestAFrameThatIsNotAPINAnswerEndsTheWorker(t *testing.T) {
	h := &fakeHandler{needs: askingToken}
	in := conversation(t,
		Request{Op: OpLogin, Thumbprint: "ABCD", Exchange: "EX1"},
		Request{Op: OpList},
	)

	var out bytes.Buffer
	err := Serve(context.Background(), in, &out, h)
	if !errors.Is(err, ErrProtocolDesync) {
		t.Fatalf("Serve: %v, want ErrProtocolDesync", err)
	}
	if !strings.Contains(err.Error(), string(OpList)) {
		t.Errorf("the reason does not say what arrived instead: %v", err)
	}
	if h.lists != 0 {
		t.Error("a frame arriving where the PIN belongs was served as an operation")
	}
}

// TestAPINOfferedWithNoLoginInProgressEndsTheWorker is the mirror of the
// parent's own refusal, and it ends the worker for a reason that is easy to get
// wrong.
//
// Nothing has been read past the frame, so the loop looks recoverable and a
// refusal in a Response looks like the polite answer. It is not: behind that
// frame are PINLength raw bytes that only ask ever reads, so carrying on means
// the next ReadFrame takes the first four bytes of a PIN as a length prefix and
// the rest into a payload buffer — where it sits in this process's heap, never
// overwritten, as somebody else's byte slice. That is exactly what D-297
// rejected bufio for, arrived at from the other direction.
func TestAPINOfferedWithNoLoginInProgressEndsTheWorker(t *testing.T) {
	h := &fakeHandler{}
	in := conversation(t,
		Request{Op: OpLoginPIN, Exchange: "EX1", PINLength: 4},
		[]byte("1234"),
		Request{Op: OpList},
	)

	var out bytes.Buffer
	err := Serve(context.Background(), in, &out, h)
	if !errors.Is(err, ErrProtocolDesync) {
		t.Fatalf("Serve: %v, want ErrProtocolDesync", err)
	}
	if out.Len() != 0 {
		t.Errorf("Serve wrote %d bytes; an offered PIN gets no answer", out.Len())
	}
	if h.lists != 0 {
		t.Error("the loop carried on past an offered PIN and served what followed it")
	}
}

// TestAPINLengthTheTokenCannotAcceptEndsTheWorker is clause 7 at this end.
//
// The parent enforces the token's limits before it writes, so a length outside
// them cannot arrive from a parent that is in step. This is not that check
// repeated — it is what stops a length that does not fit the buffer from being
// read into it. It cannot recover, because the bytes the parent promised are
// already on their way.
func TestAPINLengthTheTokenCannotAcceptEndsTheWorker(t *testing.T) {
	for _, tc := range []struct {
		name   string
		length int
	}{
		{"shorter than the token accepts", askingToken.MinPINLength - 1},
		{"longer than the buffer", askingToken.MaxPINLength + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &fakeHandler{needs: askingToken}
			in := conversation(t,
				Request{Op: OpLogin, Thumbprint: "ABCD", Exchange: "EX1"},
				Request{Op: OpLoginPIN, Exchange: "EX1", PINLength: tc.length},
				bytes.Repeat([]byte{'1'}, max(tc.length, 0)),
			)

			var out bytes.Buffer
			err := Serve(context.Background(), in, &out, h)
			if !errors.Is(err, ErrProtocolDesync) {
				t.Fatalf("Serve: %v, want ErrProtocolDesync", err)
			}
			if h.collected != 0 {
				t.Errorf("the handler was told %d bytes arrived", h.collected)
			}
		})
	}
}

// TestALoginWithNoExchangeIsRefused is the owner's first condition as the child
// enforces it on arrival: a login with no binding is refused before a card is
// touched.
//
// It is a refusal rather than an ending because nothing has been read past it —
// a login frame has nothing behind it — and because a parent that sent one is
// disagreeing with itself in a way worth reading.
func TestALoginWithNoExchangeIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  Request
	}{
		{"no exchange", Request{Op: OpLogin, Thumbprint: "ABCD"}},
		{"no thumbprint", Request{Op: OpLogin, Exchange: "EX1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &fakeHandler{needs: askingToken}
			in := conversation(t, tc.req, Request{Op: OpList})

			var out bytes.Buffer
			if err := Serve(context.Background(), in, &out, h); err != nil {
				t.Fatalf("Serve: %v — a refusable login ended the loop", err)
			}
			got := responses(t, &out)
			if len(got) != 2 {
				t.Fatalf("got %d responses for 2 requests", len(got))
			}
			if got[0].Err == "" {
				t.Error("a login with no binding was accepted")
			}
			if h.logins != 0 {
				t.Error("a login with no binding reached the card")
			}
			if h.lists != 1 {
				t.Error("the loop did not carry on; a refusal is not an ending")
			}
		})
	}
}

// TestASignatureNeedsALoginFirst is the closed set doing its job: OpSignDigest
// is answerable only because a session exists, and the session exists only
// because a login succeeded, and the login needed an exchange identifier the
// parent minted for an operation a person approved.
func TestASignatureNeedsALoginFirst(t *testing.T) {
	h := &fakeHandler{signature: []byte{9, 9}}
	in := conversation(t,
		Request{Op: OpSignDigest, DigestAlgorithm: 1, Digest: bytes.Repeat([]byte{7}, 32)},
	)

	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	got := responses(t, &out)
	if len(got) != 1 {
		t.Fatalf("got %d responses, want 1", len(got))
	}
	// fakeHandler signs whatever it is asked to, so what this establishes is the
	// wiring: the algorithm and the digest arrive intact and the answer comes
	// back as a signature rather than as a certificate.
	if h.signs != 1 || h.signAlg != 1 || len(h.signDigest) != 32 {
		t.Errorf("the sign request arrived as alg=%d digest=%d bytes over %d calls",
			h.signAlg, len(h.signDigest), h.signs)
	}
	if len(got[0].Signature) != 2 {
		t.Errorf("the answer is not a signature: %+v", got[0])
	}
}

// TestClosingTheSessionLeavesTheModuleLoaded is the whole reason the worker is a
// different thing from the probe (D-297): the expensive part is C_Initialize and
// it stays, while the card's authenticated state goes as soon as the batch that
// needed it is done (SPEC §6.5).
func TestClosingTheSessionLeavesTheModuleLoaded(t *testing.T) {
	h := &fakeHandler{list: []CertificatePayload{{DER: []byte{0x30, 0x06}}}}
	in := conversation(t, Request{Op: OpCloseSession}, Request{Op: OpList})

	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if h.sessionShut != 1 {
		t.Errorf("the session was closed %d times, want 1", h.sessionShut)
	}
	if h.closes != 0 {
		t.Error("closing the session finalised the module, which is the one thing " +
			"holding a worker open is for")
	}
	if h.lists != 1 {
		t.Error("the module would not answer after the session was closed")
	}
}
