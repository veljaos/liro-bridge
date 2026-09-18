package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
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
	if err := Serve(context.Background(), os.Stdin, os.Stdout, &cannedHandler{answers: answers}); err != nil {
		return 1
	}
	return 0
}

// cannedHandler answers without a module, and ends the process on request.
type cannedHandler struct {
	answers cannedAnswers
	served  int
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

func (c *cannedHandler) List(context.Context) ([]CertificatePayload, error) {
	c.count()
	return []CertificatePayload{{DER: []byte{0x30, 0x02}}}, nil
}

func (c *cannedHandler) ChainFor(_ context.Context, tp string) ([][]byte, error) {
	c.count()
	return [][]byte{[]byte(tp)}, nil
}

func (c *cannedHandler) Close() error { return nil }
