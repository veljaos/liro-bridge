package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeHandler stands in for one open module. It counts what it was asked and
// answers whatever it was told to, so that the loop can be exercised on a
// machine with no card, no module and no reader — which is every machine CI
// runs on.
type fakeHandler struct {
	enumerate []CertificatePayload
	list      []CertificatePayload
	chain     [][]byte
	fail      error

	enumerates int
	lists      int
	chainFors  int
	closes     int
	wantedTP   string

	// onCall runs at the start of every answering method, so a test can observe
	// what the loop had done at the moment it handed work over.
	onCall func()

	// The login half. needs is what the fake token asks for; a zero MaxPINLength
	// means this token has a protected authentication path and ask is never
	// called at all, which is the branch SPEC §6.5.1 clause 1 is about and the
	// reason the exchange has two phases.
	needs     LoginNeeds
	loginCert CertificatePayload
	loginFail error

	logins      int
	askedFor    string
	collected   int   // how many bytes ask reported, or -1 if ask was not called
	askErr      error // what ask answered with, kept so a test can see it was not swallowed
	signature   []byte
	signAlg     int
	signDigest  []byte
	signs       int
	sessionShut int
}

func (f *fakeHandler) note() {
	if f.onCall != nil {
		f.onCall()
	}
}

func (f *fakeHandler) Enumerate(context.Context) ([]CertificatePayload, error) {
	f.note()
	f.enumerates++
	return f.enumerate, f.fail
}

func (f *fakeHandler) List(context.Context) ([]CertificatePayload, error) {
	f.note()
	f.lists++
	return f.list, f.fail
}

func (f *fakeHandler) ChainFor(_ context.Context, tp string) ([][]byte, error) {
	f.note()
	f.chainFors++
	f.wantedTP = tp
	return f.chain, f.fail
}

func (f *fakeHandler) Login(_ context.Context, thumbprint string, ask PINExchange) (CertificatePayload, [][]byte, error) {
	f.note()
	f.logins++
	f.askedFor = thumbprint
	f.collected = -1

	if f.needs.MaxPINLength > 0 {
		// The buffer is the token's own maximum, as the real login allocates it.
		dst := make([]byte, f.needs.MaxPINLength)
		n, err := ask(dst, f.needs)
		f.collected, f.askErr = n, err
		if err != nil {
			return CertificatePayload{}, nil, err
		}
	}
	if f.loginFail != nil {
		return CertificatePayload{}, nil, f.loginFail
	}
	return f.loginCert, f.chain, nil
}

func (f *fakeHandler) SignDigest(_ context.Context, alg int, digest []byte) ([]byte, error) {
	f.note()
	f.signs++
	f.signAlg, f.signDigest = alg, digest
	return f.signature, f.fail
}

func (f *fakeHandler) CloseSession() error {
	f.sessionShut++
	return nil
}

func (f *fakeHandler) Close() error {
	f.closes++
	return nil
}

// requests encodes reqs as the parent would write them.
func requests(t *testing.T, reqs ...Request) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	for _, r := range reqs {
		if err := WriteFrame(&buf, r); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
	}
	return &buf
}

// responses decodes everything the loop wrote.
func responses(t *testing.T, r io.Reader) []Response {
	t.Helper()
	var out []Response
	for {
		var resp Response
		err := ReadFrame(r, &resp)
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("reading a response: %v", err)
		}
		out = append(out, resp)
	}
}

// TestEveryRequestGetsExactlyOneAnswerInOrder is the loop's whole contract, and
// the ordering half is why it is asserted rather than assumed: the parent has
// no request identifiers to match answers against, deliberately, because with
// one pipe and one loop there is nothing for them to disambiguate. That is only
// true while every request gets exactly one response, in order.
func TestEveryRequestGetsExactlyOneAnswerInOrder(t *testing.T) {
	h := &fakeHandler{
		enumerate: []CertificatePayload{{DER: []byte{1}, Label: "one"}},
		list:      []CertificatePayload{{DER: []byte{2}}},
		chain:     [][]byte{{3}},
	}
	in := requests(t,
		Request{Op: OpEnumerate},
		Request{Op: OpList},
		Request{Op: OpChainFor, Thumbprint: "ABCD"},
	)

	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	got := responses(t, &out)
	if len(got) != 3 {
		t.Fatalf("got %d responses for 3 requests", len(got))
	}
	for i, r := range got {
		if r.Err != "" {
			t.Fatalf("response %d carried an error: %s", i, r.Err)
		}
	}
	if len(got[0].Certificates) != 1 || got[0].Certificates[0].Label != "one" {
		t.Errorf("the first answer is not the enumerate answer: %+v", got[0])
	}
	if len(got[1].Certificates) != 1 || got[1].Certificates[0].Label != "" {
		t.Errorf("the second answer is not the list answer: %+v", got[1])
	}
	if len(got[2].Chain) != 1 {
		t.Errorf("the third answer is not the chain answer: %+v", got[2])
	}
	if h.wantedTP != "ABCD" {
		t.Errorf("the thumbprint reached the handler as %q, want %q", h.wantedTP, "ABCD")
	}

	// One module, one C_Initialize, three requests. If the loop ever closed and
	// reopened between requests this would be three.
	if h.closes != 0 {
		t.Errorf("the handler was closed %d times while serving; a held-open "+
			"module is the whole reason this process exists (D-297)", h.closes)
	}
}

// TestShutdownFinalisesBeforeItAnswers pins the ordering, not just the effect.
//
// F12 §2: "C_Finalize only on shutdown or a deliberate reset." A parent that
// reads the shutdown answer and immediately kills the child would, if the
// answer came first, be killing a process that had not finalised — which is the
// state a vendor module is least likely to have been tested in.
func TestShutdownFinalisesBeforeItAnswers(t *testing.T) {
	closedBeforeAnswer := false
	h := &fakeHandler{}
	var out bytes.Buffer

	in := requests(t, Request{Op: OpShutdown}, Request{Op: OpEnumerate})

	// The only way to see the ordering from outside is to look at the buffer at
	// the moment Close runs. onCall is not called by Close, so this is wired
	// directly.
	h.onCall = nil
	err := Serve(context.Background(), in, &writeWatcher{w: &out, before: func() {
		closedBeforeAnswer = h.closes > 0
	}}, h)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}

	if h.closes != 1 {
		t.Errorf("the handler was closed %d times, want 1", h.closes)
	}
	if !closedBeforeAnswer {
		t.Error("the shutdown answer was written before C_Finalize had happened")
	}
	if got := responses(t, &out); len(got) != 1 {
		t.Errorf("got %d responses, want 1: the request after shutdown must not "+
			"be served", len(got))
	}
	if h.enumerates != 0 {
		t.Error("a request after shutdown reached the module")
	}
}

// writeWatcher runs before, once, on the first write.
type writeWatcher struct {
	w      io.Writer
	before func()
	fired  bool
}

func (x *writeWatcher) Write(p []byte) (int, error) {
	if !x.fired {
		x.fired = true
		x.before()
	}
	return x.w.Write(p)
}

// TestTheParentLettingGoOfThePipeIsNotAnError covers the other ordinary
// ending. A parent that exits, or that closes its end without a shutdown,
// leaves the child reading an empty pipe — which is a close rather than a
// fault, and must not be reported as one, because the caller turns an error
// into a non-zero exit and a non-zero exit is what the supervisor reads as a
// worker that did not survive.
func TestTheParentLettingGoOfThePipeIsNotAnError(t *testing.T) {
	h := &fakeHandler{}
	var out bytes.Buffer
	if err := Serve(context.Background(), bytes.NewReader(nil), &out, h); err != nil {
		t.Errorf("Serve on an already-ended pipe: %v, want nil", err)
	}
	if out.Len() != 0 {
		t.Errorf("Serve wrote %d bytes with nothing to answer", out.Len())
	}
}

// TestAnUnknownOperationIsRefusedRatherThanIgnored is the newer-parent case.
//
// A worker that silently did nothing for an operation it did not recognise
// would answer with an empty success, and an empty success is exactly what a
// listing with no certificates on it looks like. The failure would then arrive
// as "this card has nothing on it" rather than as "these two do not agree about
// the protocol", which is the sort of misreading that costs an afternoon.
func TestAnUnknownOperationIsRefusedRatherThanIgnored(t *testing.T) {
	h := &fakeHandler{}
	in := requests(t, Request{Op: Op("sign")}, Request{Op: OpEnumerate})

	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	got := responses(t, &out)
	if len(got) != 2 {
		t.Fatalf("got %d responses for 2 requests", len(got))
	}
	if got[0].Err == "" {
		t.Error("an unknown operation was answered with a success")
	}
	if !strings.Contains(got[0].Err, "sign") {
		t.Errorf("the refusal does not name what was asked for: %q", got[0].Err)
	}
	if len(got[0].Certificates) != 0 || len(got[0].Chain) != 0 {
		t.Error("a refusal carried a payload")
	}
	// And the loop carried on: one bad request does not end a worker.
	if got[1].Err != "" || h.enumerates != 1 {
		t.Error("an unknown operation ended the loop instead of being refused")
	}
}

// TestAnUnknownOperationCannotOverflowTheAnswer covers the one way a request
// could stop a worker without killing it.
//
// A frame may claim up to maxFrame, so an operation name of a megabyte is
// representable. Quoted whole into a response, that response would itself
// exceed maxFrame, WriteFrame would refuse it, and the loop would end — a
// worker killed by a string. The pipe is inherited and only this program's own
// parent writes it, so this is not a defence against an attacker; it is a
// defence against a length nobody thought about.
func TestAnUnknownOperationCannotOverflowTheAnswer(t *testing.T) {
	h := &fakeHandler{}
	in := requests(t, Request{Op: Op(strings.Repeat("x", maxFrame/2))})

	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v — a long operation name ended the loop", err)
	}
	got := responses(t, &out)
	if len(got) != 1 {
		t.Fatalf("got %d responses, want 1", len(got))
	}
	if got[0].Err == "" {
		t.Fatal("a very long operation name was answered with a success")
	}
	if len(got[0].Err) > maxOpInMessage+256 {
		t.Errorf("the refusal is %d bytes; it quotes the operation whole", len(got[0].Err))
	}
}

// TestAHandlerFailureIsAnAnswerRatherThanAnEnding is the distinction the
// supervisor rests on: a module that refuses a request has not died, and only a
// worker that has died should be respawned. F11 §3 asks for a readable reason
// rather than a number, and this is where the reason is put.
func TestAHandlerFailureIsAnAnswerRatherThanAnEnding(t *testing.T) {
	h := &fakeHandler{fail: errors.New("CKR_TOKEN_NOT_PRESENT")}
	in := requests(t, Request{Op: OpEnumerate}, Request{Op: OpList})

	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v — a handler failure ended the loop", err)
	}
	got := responses(t, &out)
	if len(got) != 2 {
		t.Fatalf("got %d responses for 2 requests", len(got))
	}
	for i, r := range got {
		if !strings.Contains(r.Err, "CKR_TOKEN_NOT_PRESENT") {
			t.Errorf("response %d does not carry the module's own reason: %q", i, r.Err)
		}
	}
}

// TestChainForNeedsAThumbprint refuses the request rather than asking the
// module for the chain of nothing. The empty string is not a certificate, and a
// module asked about it would answer with its own error, which would read as a
// card problem rather than as a request that was never complete.
func TestChainForNeedsAThumbprint(t *testing.T) {
	h := &fakeHandler{}
	in := requests(t, Request{Op: OpChainFor})

	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	got := responses(t, &out)
	if len(got) != 1 || got[0].Err == "" {
		t.Fatalf("chainfor with no thumbprint was not refused: %+v", got)
	}
	if h.chainFors != 0 {
		t.Error("chainfor with no thumbprint reached the module")
	}
}

// countingReader records how much of a stream has actually been consumed.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// TestTheLoopReadsNoFurtherThanTheRequestItIsServing is SPEC §6.5.1 clause 2 as
// a property of the loop, rather than as the import rule that keeps bufio out.
//
// The clause bounds the PIN to "one write, read immediately, never buffered",
// and the whole reason the framing is length-prefixed is that the PIN arrives
// on this same reader immediately behind a request (D-297). If the loop read
// ahead — a buffered reader, a larger read, a decoder that grabs what it can —
// the PIN would already be sitting in this process's heap before anything asked
// for it, never overwritten, and invisible to pin_test.go because it would be
// somebody else's byte slice rather than a field, a parameter or a named
// result.
//
// TestReadFrameTakesExactlyItsOwnBytesAndNotOneMore asserts this of one frame.
// This asserts it of the loop, which is the layer a future buffered reader
// would be introduced at and the layer the PIN will be read from.
//
// The second frame stands in for the PIN: at the moment the loop hands the
// first request to the module, not one byte of it may have been consumed.
func TestTheLoopReadsNoFurtherThanTheRequestItIsServing(t *testing.T) {
	var first bytes.Buffer
	if err := WriteFrame(&first, Request{Op: OpEnumerate}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	firstLen := first.Len()

	in := requests(t, Request{Op: OpEnumerate}, Request{Op: OpShutdown})
	counted := &countingReader{r: in}

	readWhenCalled := -1
	h := &fakeHandler{}
	h.onCall = func() { readWhenCalled = counted.n }

	var out bytes.Buffer
	if err := Serve(context.Background(), counted, &out, h); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	if readWhenCalled < 0 {
		t.Fatal("the handler was never called, so nothing was observed")
	}
	if readWhenCalled != firstLen {
		t.Errorf("at the moment the first request was served, %d bytes had been "+
			"read; its frame is %d.\n\n"+
			"The loop read past the request it was serving. SPEC §6.5.1 clause 2 "+
			"bounds the PIN to one write, read immediately, never buffered — and "+
			"the PIN arrives on this reader immediately behind a request. Anything "+
			"the loop reads ahead into is a copy of it nothing will overwrite.",
			readWhenCalled, firstLen)
	}
}

// TestACancelledContextEndsTheLoop covers the third ending. It is an error
// rather than a clean stop, because a worker whose context was cancelled has
// not finalised its module and the caller should not report that as a tidy end.
func TestACancelledContextEndsTheLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := &fakeHandler{}
	in := requests(t, Request{Op: OpEnumerate})

	var out bytes.Buffer
	if err := Serve(ctx, in, &out, h); !errors.Is(err, context.Canceled) {
		t.Errorf("Serve with a cancelled context: %v, want context.Canceled", err)
	}
	if h.enumerates != 0 {
		t.Error("a cancelled context still reached the module")
	}
}
