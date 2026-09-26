package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// cannedPrefix marks a module path that is not one: it carries, instead of a
// path, what a test wants its own child to answer with.
//
// Keying on the argument rather than on a second subcommand is the pattern
// crash_windows_test.go already uses in the pkcs11 package — the crash a child
// performs on request is selected by its path, inside TestMain, so nothing in
// the shipped code knows about it. The same reasoning applies with more force
// here: Worker.start builds its command line from Subcommand and the module
// path, and giving it a settable subcommand so that tests could reach a fake
// child would be a seam in the product whose only user is a test (D-100).
//
// A real module path cannot begin with this, and if one somehow did, the child
// would answer canned data to a test rather than doing anything to a card.
const cannedPrefix = "liro-worker-test-canned://"

// cannedAnswers is what a test tells its own child to answer with.
type cannedAnswers struct {
	Label string `json:"label"`
	// Slots is what the canned module says about its readers and cards.
	Slots *SlotCounts `json:"slots,omitempty"`
	// DieOnRequest, when positive, makes the child exit instead of answering
	// that request, counting from one. Zero means it never dies.
	//
	// It is how a worker dying mid-conversation is produced at all here.
	// D-272's module dies about once in a hundred calls and a demonstration
	// cannot be scheduled (D-294: "no number of clean runs demonstrates 'when
	// it dies, the parent survives'; only a death does"), so what the parent's
	// own handling can be measured against is a child that ends on purpose —
	// which D-296 ruled is a real death for this purpose, because the parent
	// reads a status from a process that genuinely ended rather than a value
	// somebody handed it.
	//
	// One-based, and deliberately not "die after N": the zero value has to mean
	// "never", and "after 0" and "never" are the same number. Getting that
	// wrong is not hypothetical — the first version of this was DieAfter, and
	// the test meant to prove the respawn bound passed against a child that
	// never died, which is a test that could not fail.
	DieOnRequest int `json:"dieOnRequest"`

	// Needs, when its maximum is positive, makes this child ask for a PIN on a
	// login. Zero means a token with a protected authentication path, which is
	// SPEC §6.5.1 clause 1's branch and completes without asking.
	Needs LoginNeeds `json:"needs"`

	// WantPIN is what the child requires the collected bytes to be. A login
	// whose PIN does not match is refused the way a card refuses one, so a test
	// can tell a PIN that arrived intact from one that did not arrive at all.
	WantPIN string `json:"wantPin"`

	// AskUnbidden makes the child send a PIN question in answer to an operation
	// that is not a login. It is the only way to produce the owner's second
	// condition — a PIN request arriving when nothing is pending — from a real
	// process, because no correct child does it.
	AskUnbidden bool `json:"askUnbidden"`

	// AskWithExchange, when set, is the exchange identifier the child echoes
	// instead of the one it was given. It produces the first condition: a
	// question about an operation this parent did not approve.
	AskWithExchange string `json:"askWithExchange"`

	// AskTwice makes the child ask a second time after a complete answer, which
	// is what a retry would look like from the parent's side.
	AskTwice bool `json:"askTwice"`

	// NoSuchCertificate makes this child answer a login or a chain request the
	// way a module answers for a thumbprint none of its tokens carries.
	//
	// It is here rather than being provoked with a wrong thumbprint because the
	// canned child has no certificate store to miss in: what is being measured
	// is whether one particular refusal survives the pipe as a sentinel, and
	// that is a property of the protocol rather than of any card.
	NoSuchCertificate bool `json:"noSuchCertificate"`
}

// cannedPath encodes answers as the argument a spawned child will be given.
func cannedPath(a cannedAnswers) string {
	b, err := json.Marshal(a)
	if err != nil {
		panic(err) // a struct of a string and an int
	}
	return cannedPrefix + string(b)
}

// TestMain makes this package's test binary answer the subcommand a spawned
// child of these tests is given.
//
// # What it is for, and what it is not for
//
// Worker.start spawns os.Executable(). Under `go test` that is worker.test, and
// a test binary that has never heard of the subcommand ignores the positional
// arguments and runs the whole suite. D-293 measured what that costs when
// nothing stops it recursing: `go test ./internal/keysource/pkcs11/...` took
// that machine from 254 processes to 827.
//
// It is not what stops that happening. pkcs11.ChildMarker is — on the parent
// side, read from the parent's own environment, holding for every binary
// whether or not the binary dispatches anything. A guard that depends on each
// future test binary remembering a TestMain is not a guard.
//
// What this is for is that the tests be about the thing: a child that runs its
// own suite answers with test output, which is not a response, and a parent
// test would then be reading its own noise rather than the protocol.
func TestMain(m *testing.M) {
	// Before testing.M parses anything: these arguments are not test flags and
	// the flag package would reject them. Mirrors run() in cmd/liro-bridge.
	if len(os.Args) > 2 && os.Args[1] == Subcommand {
		if encoded, ok := strings.CutPrefix(os.Args[2], cannedPrefix); ok {
			os.Exit(runCannedServer(encoded))
		}
		// Anything else is the real thing, which will try to load a module.
		os.Exit(Run(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func runCannedServer(encoded string) int {
	var answers cannedAnswers
	if err := json.Unmarshal([]byte(encoded), &answers); err != nil {
		return 2
	}
	if answers.AskUnbidden || answers.AskWithExchange != "" {
		return runRogueServer(answers)
	}
	if err := Serve(context.Background(), os.Stdin, os.Stdout, &cannedHandler{answers: answers}); err != nil {
		return 1
	}
	return 0
}

// runRogueServer is a child that does not use Serve.
//
// Two of the parent's conditions cannot be produced through the Handler
// interface, and that is not an accident — it is the interface being the right
// shape. A Handler cannot ask a PIN question outside a login because it has no
// writer, and it cannot echo the wrong exchange identifier because serveLogin
// stamps the one the request arrived with. Both are exactly what the parent's
// refusals are for: the owner's words were that a PIN request arriving when
// nothing is pending means either the worker is confused or something else is
// talking to the parent. This is that something else.
//
// It is here rather than in the shipped code for the reason D-100 gives: a seam
// in the product whose only user is a test is a seam. The child is selected by
// its module path, inside TestMain, and nothing in the release binary knows it
// exists.
func runRogueServer(a cannedAnswers) int {
	for {
		var req Request
		if err := ReadFrame(os.Stdin, &req); err != nil {
			return 0 // the parent let go, which is how this child ends
		}

		needs := a.Needs
		needs.Exchange = req.Exchange
		if a.AskWithExchange != "" {
			needs.Exchange = a.AskWithExchange
		}
		if err := WriteFrame(os.Stdout, Response{Login: &needs}); err != nil {
			return 1
		}

		// Wait for whatever comes next, which for a parent that is doing its job
		// is nothing at all: it kills this process instead of answering.
		var ignored Request
		if err := ReadFrame(os.Stdin, &ignored); err != nil {
			return 0
		}
	}
}

// cannedHandler answers without a module, and ends the process on request.
type cannedHandler struct {
	answers  cannedAnswers
	served   int
	loggedIn bool
}

// count records one request and, if this child was told to, ends the process
// rather than answering it. Not a panic and not an exception: what the parent
// reads of a death is a non-zero exit and a pipe that ended, and that is what
// this produces.
//
// It says so on the standard error it inherited before it goes, which is what
// makes the number of attempts a parent made observable from outside — the same
// channel the real child puts its own reason on.
func (c *cannedHandler) count() {
	c.served++
	if c.answers.DieOnRequest > 0 && c.served == c.answers.DieOnRequest {
		fmt.Fprintln(os.Stderr, cannedDeathLine)
		os.Exit(3)
	}
}

// cannedDeathLine is what a child prints on its way out, so a test can count
// how many of them there were.
const cannedDeathLine = "canned child: ending without answering, as asked"

func (c *cannedHandler) Enumerate(context.Context) ([]CertificatePayload, error) {
	c.count()
	return []CertificatePayload{{DER: []byte{0x30, 0x01}, Label: c.answers.Label}}, nil
}

func (c *cannedHandler) List(context.Context) ([]CertificatePayload, *SlotCounts, error) {
	c.count()
	return []CertificatePayload{{DER: []byte{0x30, 0x02}}}, c.answers.Slots, nil
}

func (c *cannedHandler) ChainFor(_ context.Context, tp string) ([][]byte, error) {
	c.count()
	if c.answers.NoSuchCertificate {
		return nil, fmt.Errorf("%w: %s", pkcs11.ErrCertificateNotFound, tp)
	}
	return [][]byte{[]byte(tp)}, nil
}

// Login collects a PIN when this canned token was told to want one, and checks
// it against what the test put on the card.
//
// Checking it is what makes "the PIN arrived intact" different from "a login
// happened", which is the whole thing a test of this seam has to be able to
// tell apart. A wrong one is refused the way a card refuses one: an answer, not
// an ending.
func (c *cannedHandler) Login(_ context.Context, thumbprint string, ask PINExchange) (CertificatePayload, [][]byte, error) {
	c.count()
	if c.answers.NoSuchCertificate {
		// Before the PIN is asked for, which is what a real module does: it
		// cannot ask for a PIN for a certificate it does not have.
		return CertificatePayload{}, nil, fmt.Errorf("%w: %s", pkcs11.ErrCertificateNotFound, thumbprint)
	}
	if c.answers.Needs.MaxPINLength > 0 {
		dst := make([]byte, c.answers.Needs.MaxPINLength)
		n, err := ask(dst, c.answers.Needs)
		if err != nil {
			return CertificatePayload{}, nil, err
		}
		if c.answers.AskTwice {
			// A second question after a complete answer, which is what a retry
			// would look like from the parent's side and which clause 5 says
			// must never happen. The parent's job is to refuse it.
			_, _ = ask(dst, c.answers.Needs)
		}
		if string(dst[:n]) != c.answers.WantPIN {
			return CertificatePayload{}, nil, fmt.Errorf("CKR_PIN_INCORRECT")
		}
	}
	c.loggedIn = true
	return CertificatePayload{DER: []byte{0x30, 0x03}, Label: c.answers.Label},
		[][]byte{[]byte(thumbprint)}, nil
}

func (c *cannedHandler) SignDigest(_ context.Context, alg int, digest []byte) ([]byte, error) {
	c.count()
	if !c.loggedIn {
		return nil, errNoSession
	}
	return append([]byte{byte(alg)}, digest...), nil
}

func (c *cannedHandler) CloseSession() error {
	c.loggedIn = false
	return nil
}

func (c *cannedHandler) Close() error { return nil }
