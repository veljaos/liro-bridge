package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// maxAttempts is how many workers one request may be tried against: the one
// that was running, and two respawns.
//
// It is a **count and not an interval**, which is the instruction D-297
// recorded and the reason is worth keeping: D-296 measured a worker dying on
// purpose being reaped in 341–376 ms, and also measured that what those
// milliseconds are spent on is not established — pinning it needs the
// per-process error-reporting disable D-292 found could not be demonstrated.
// A backoff sized from an unexplained number is how a magic constant gets into
// a supervisor and stays there for years.
//
// Three, rather than one or ten. D-272 measured the crash at roughly one call
// in a hundred and bursty rather than steady, so one attempt would turn a
// one-per-cent fault into a one-per-cent listing failure — the defect D-297
// says people learn to re-run instead of read. Three consecutive deaths is far
// past what that rate produces even allowing for bursts, so exhausting the
// budget means the module is not usable now rather than that this request was
// unlucky, and saying so is more useful than trying again.
const maxAttempts = 3

// retryable is the allow-list of operations a request may be re-sent for after
// the worker serving it died.
//
// An allow-list rather than a deny-list, for D-259's reason: a deny-list is one
// unconsidered addition away from being wrong, and the addition that is coming
// is the one that must never be retried. SPEC §6.5.1 clause 5: "Nothing retries
// a PIN automatically, ever, for any reason. One wrong PIN is one attempt.
// Three block the card, and for a national identity card unblocking means a
// visit to a police station."
//
// A login is not in this set and must never be added to it, and neither is
// anything else that has an effect on the card. What is here is three reads,
// each of which asks the same question of a freshly opened module and gets the
// same answer.
//
// OpShutdown is absent for a different reason: a worker that died is already
// shut down, so there is nothing to retry against.
var retryable = map[Op]bool{
	OpEnumerate: true,
	OpList:      true,
	OpChainFor:  true,
}

var (
	// ErrWorkerDied is a worker that stopped answering: it exited, or the pipe
	// ended mid-frame, which is what a process dying mid-answer looks like from
	// here.
	//
	// It deliberately does not say the module killed it. D-272 measured that it
	// can, and the child exits non-zero for other reasons too — a module that
	// would not load at all, a frame that would not encode. From this side they
	// are the same event and there is nothing to tell them apart with, so the
	// name says what was observed rather than what caused it. The reason, when
	// there was one to give, is what the child wrote to its standard error —
	// read it with Worker.ChildStderr, never by reading back the writer handed
	// to New, which a copier goroutine is writing to (D-303).
	ErrWorkerDied = errors.New("pkcs11 worker: the worker process stopped answering")

	// ErrWorkerCannotStart is a worker that never ran: this binary could not be
	// found, or this process is itself a child and must not spawn one.
	ErrWorkerCannotStart = errors.New("pkcs11 worker: the worker process could not be started")

	// ErrUnexpectedPINRequest is a worker asking for a PIN when nothing is
	// pending, or asking about an exchange this parent did not open.
	//
	// It is a defect and is reported as one, loudly, rather than being answered
	// or ignored. The owner's words: a PIN request arriving when nothing is
	// pending means either the worker is confused or something else is talking
	// to the parent, and both are worth knowing about.
	//
	// What is at stake is not an extra round trip. The PIN screen is the one
	// window in this program that deliberately looks like a system dialog
	// (D-277, SPEC §10), and honouring a request merely because a worker is
	// alive would make "can make a PIN dialog appear" a thing the child side
	// can do. The binding is to the operation a person approved — the Exchange
	// identifier minted by Open, which is the call the consent screen leads to —
	// and not to the worker being alive.
	//
	// It kills the worker, because a child that asked a question this parent
	// will not answer is a child waiting for bytes that will never come.
	//
	// The explicit `error` is load-bearing rather than a style choice, and this
	// is the second place in the tree to need it — login_windows.go carries the
	// first two and the full argument. In short: this is named after a PIN and
	// cannot hold one, pin_test.go asks what a declaration can carry rather than
	// what it is called (D-270), and a package-level var with no type expression
	// and a function call for an initialiser is one the checker cannot see
	// through and is right to refuse. Naming the type answers its question
	// truthfully. Renaming the var to dodge the matcher is what D-270 rejected.
	ErrUnexpectedPINRequest error = errors.New("pkcs11 worker: the worker asked for a PIN outside an approved operation")
)

// Worker is the parent's end of one worker process: one module, held open in a
// child, answering a pipe.
//
// # What it is for, against the probe, which looks the same and is not
//
// Discovery spawns a throwaway child per candidate and pays C_Initialize per
// candidate. That is correct there: it is asking unknown files what they are, a
// crash is the *expected* outcome, and it costs one Failure and about 30 ms.
// This holds C_Initialize open, because a session has to survive many calls and
// paying it per call rolls D-272's dice every time — which turns a startup
// problem into a listing problem, and a listing that fails one time in a
// hundred is the kind of defect people learn to re-run instead of read.
//
// D-297 is the entry, and it is worth reading before merging the two. The tell
// that such a consolidation is wrong: the merged version has to take a
// parameter to decide which of the two it is.
//
// # It starts lazily
//
// Nothing is spawned until something is asked. A machine with four candidate
// modules should not hold four vendor DLLs open in four processes because a
// listing might happen, and the first request pays exactly what a respawn pays,
// so there is one code path rather than two.
//
// # Safe for concurrent use, which the module underneath is not
//
// One pipe carries one request at a time, so two callers interleaving frames
// would corrupt both. The mutex is here rather than left to the orchestration
// layer because the thing it protects is this type's own pipe; what the
// orchestration layer serialises (D-027) is the card, which is one level down
// and is serialised by the child being single-threaded (D-298).
type Worker struct {
	modulePath string
	stderr     io.Writer

	// said is what children have written to their own standard error, kept by
	// this Worker so the parent can quote it. See ChildStderr.
	said childStderr

	mu  sync.Mutex
	cmd *exec.Cmd
	in  io.WriteCloser
	out io.ReadCloser
}

// New returns a Worker over the module at path. Nothing is spawned yet.
//
// stderr is where the child's own standard error is forwarded, live. It is a
// parameter rather than always os.Stderr because a vendor module writes to it
// during DllMain — measured: Nexus's personal64.dll does — and that output is
// the caller's to place. Nil means os.Stderr, which is where the probe puts it.
//
// # It is written from another goroutine, for as long as a child lives
//
// os/exec copies a child's standard error into a non-*os.File writer on a
// goroutine of its own, which runs from Start until the child is reaped. So
// whatever is passed here is written to concurrently with everything the
// caller does next, and a caller that also *reads* it — a bytes.Buffer, a
// strings.Builder — has a data race.
//
// That is not a caution invented for this comment. It was measured: every test
// in this package passed a bytes.Buffer and read it to find out what a child
// had said, and CI's race detector reported it against three of them. The
// reasoning that let it through was that a copier blocked in Read has not
// written anything yet, and that is false — bytes.Buffer.ReadFrom grows the
// buffer *before* each read, so a copier that has never received a byte has
// already written the slice header (D-303).
//
// Callers who want to know what a child said use ChildStderr, which is this
// Worker's own record and is safe to read at any time. This parameter is for
// placing the noise, not for reading it back.
func New(modulePath string, stderr io.Writer) *Worker {
	if stderr == nil {
		stderr = os.Stderr
	}
	return &Worker{modulePath: modulePath, stderr: stderr}
}

// maxChildStderr bounds what one Worker keeps of what its children said.
//
// A worker is long-lived and a vendor module is not required to be quiet, so an
// unbounded record is a leak with a module's name on it. The *tail* is kept
// rather than the head: what a child says on its way out is why it went, which
// is the question ErrWorkerDied exists to answer, and a fixed prefix of a
// chatty module's start-up noise answers nothing.
const maxChildStderr = 8 << 10

// childStderr is one Worker's record of what its children have written to
// their own standard error.
//
// It is a writer rather than a buffer the Worker reads, because the writing is
// done by a goroutine inside os/exec and the reading by whoever holds the
// Worker. One mutex, held across both, is the whole of it.
//
// It is deliberately *not* w.mu. That mutex serialises requests on the pipe and
// is held for the length of a round trip — so quoting what a child said while
// one was in flight would block until the module answered, which is exactly
// when a caller most wants to know what the child has been saying.
type childStderr struct {
	mu  sync.Mutex
	buf []byte
}

func (c *childStderr) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	if len(c.buf) > maxChildStderr {
		c.buf = c.buf[len(c.buf)-maxChildStderr:]
	}
	return len(p), nil
}

func (c *childStderr) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.buf)
}

// ChildStderr is what this Worker's children have written to their standard
// error, most recent last, up to maxChildStderr bytes.
//
// It is safe to call at any time, including while a child is running and while
// another goroutine has a request in flight, and that is the entire reason it
// exists: ErrWorkerDied's own comment says the reason a worker went is "on the
// standard error the child inherited", and until D-303 the only way to act on
// that advice was to read memory an os/exec copier goroutine was writing.
//
// It spans respawns on purpose. A worker that died and was replaced has two
// children's output in here, in order, which is what makes "how many died"
// answerable at all.
func (w *Worker) ChildStderr() string { return w.said.String() }

// ModulePath is which module this worker was started for. It is what a Failure
// or a log line names, and it is the only thing that tells two workers apart.
func (w *Worker) ModulePath() string { return w.modulePath }

// Enumerate reads every certificate object the module can see.
func (w *Worker) Enumerate(ctx context.Context) ([]CertificatePayload, error) {
	resp, err := w.do(ctx, Request{Op: OpEnumerate})
	return resp.Certificates, err
}

// List is the same reading, deduplicated for the agent's listing.
func (w *Worker) List(ctx context.Context) ([]CertificatePayload, error) {
	resp, err := w.do(ctx, Request{Op: OpList})
	return resp.Certificates, err
}

// ChainFor returns the issuer chain the token itself carries for one
// certificate, which is empty for both Serbian cards (D-274).
func (w *Worker) ChainFor(ctx context.Context, thumbprint string) ([][]byte, error) {
	resp, err := w.do(ctx, Request{Op: OpChainFor, Thumbprint: thumbprint})
	return resp.Chain, err
}

// Close asks the worker to finalise, then makes sure it is gone.
//
// # Why it takes a context
//
// Because nothing here may choose how long to wait. The shutdown request
// reaches a module that has to run C_Finalize, and D-272 measured a module
// dying inside C_Initialize without ever measuring one that *hangs* — nothing
// says one cannot. A worker that will not finalise has to be given up on, and
// the number of seconds that takes is not this function's to invent: D-297
// leaves the per-request deadline open deliberately, as a question with its own
// justification and its own owner.
//
// So the caller's context is the bound, and every step is an event rather than
// a duration: the shutdown answer, the child's own exit, and — only if the
// context ends first — a kill.
//
// Which means context.Background() here is a choice and not a default: it says
// "wait for ever", and against a child that never answers a shutdown it does
// exactly that. Measured, not supposed — a deliberately misbehaving child in
// this package's own tests wedged teardown until `go test`'s ten-minute timeout,
// which is the correct behaviour arriving somewhere nobody wanted it. A caller
// who cannot wait for ever passes a context that says so.
//
// Closing stdin is what makes this terminate even when the shutdown request
// never got through: the child's next read ends, Serve returns nil, the process
// exits. It is safe to call Close twice.
func (w *Worker) Close(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cmd == nil {
		return nil
	}

	// Ask, but do not depend on the answer: a worker that has already died has
	// nothing to say and that is not an error here.
	_, _ = w.roundTrip(ctx, Request{Op: OpShutdown})

	_ = w.in.Close()
	return w.reap(ctx)
}

// do sends one request, respawning a dead worker for the operations that may be
// re-sent.
func (w *Worker) do(ctx context.Context, req Request) (Response, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	attempts := 1
	if retryable[req.Op] {
		attempts = maxAttempts
	}

	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := w.start(); err != nil {
			// A worker that cannot be started is not a worker that died, and
			// retrying it is retrying whatever made os.Executable or the fork
			// guard refuse — which will refuse again.
			return Response{}, err
		}

		resp, err := w.roundTrip(ctx, req)
		if err == nil {
			if resp.Err != "" {
				// The module answered and the answer was a refusal. That is not
				// a death: F11 §3 asks for a readable reason rather than a
				// number, and respawning over it would hide a card that is
				// simply not there behind three process spawns.
				return resp, fmt.Errorf("pkcs11 worker: %s: %s", w.modulePath, resp.Err)
			}
			return resp, nil
		}

		last = err
		if !errors.Is(err, ErrWorkerDied) {
			// A frame that would not parse is this program disagreeing with
			// itself, not a module killing a process. Respawning would turn a
			// protocol fault into three of them.
			w.endLocked(ctx)
			return Response{}, err
		}
		_ = w.reap(ctx)
	}

	return Response{}, fmt.Errorf("%w after %d attempts: %s: %w",
		ErrWorkerDied, attempts, w.modulePath, last)
}

// start spawns a worker if there is not one running. The caller holds the mutex.
func (w *Worker) start() error {
	if w.cmd != nil {
		return nil
	}

	// The same guard the probe uses, reading this process's own environment: a
	// process that was itself spawned to load a module does not spawn another.
	// D-293 measured what its absence costs — 254 processes to 827 — and the
	// reason it is on the parent side is that it then holds for every binary,
	// including one that ignores the subcommand and runs a test suite instead.
	if pkcs11.IsChild() {
		return fmt.Errorf("%w: %w", ErrWorkerCannotStart, pkcs11.ErrChildRecursion)
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: finding this program's own binary: %w", ErrWorkerCannotStart, err)
	}

	// No CommandContext: the child's life is this Worker's, not one request's.
	// A request that runs out of context kills the child through roundTrip,
	// which is the same thing said at the level that knows about it.
	cmd := exec.Command(self, Subcommand, w.modulePath)
	cmd.Env = pkcs11.ChildEnv()
	// The Worker's own record first, then the caller's writer. The order
	// matters on the failure path: io.MultiWriter stops at the first error, and
	// what this Worker keeps is what it can still quote when the caller's writer
	// is the thing that broke.
	//
	// A consequence worth knowing: cmd.Stderr is now never an *os.File, so
	// os/exec always pipes and copies rather than letting the child inherit
	// fd 2 directly. Nothing is lost when a module kills the child — the bytes
	// are already in the pipe and the copier drains it before seeing EOF — but
	// it is a goroutine and a pipe per child where a caller passing os.Stderr
	// used to have neither.
	cmd.Stderr = io.MultiWriter(&w.said, w.stderr)

	in, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrWorkerCannotStart, err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return fmt.Errorf("%w: %w", ErrWorkerCannotStart, err)
	}
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		return fmt.Errorf("%w: %w", ErrWorkerCannotStart, err)
	}

	w.cmd, w.in, w.out = cmd, in, out
	return nil
}

// roundTrip writes one request and reads one response. The caller holds the
// mutex.
//
// # Nothing here chooses a duration
//
// The only way out other than an answer is the caller's own context. D-297
// leaves the per-request deadline open on purpose — "a per-request deadline is
// a separate question justified by the cost of the request it bounds; bring the
// owner the number rather than choosing" — and inventing one here would be
// choosing it in the place hardest to find later.
//
// What it does do is make the context effective. A pipe read does not observe a
// context, so the read runs on its own goroutine and the context kills the
// child instead: the pipe then ends, the read returns, and the goroutine goes
// with it. The child is unusable after that either way, which is why it is
// killed rather than left.
func (w *Worker) roundTrip(ctx context.Context, req Request) (Response, error) {
	return w.exchange(ctx, req, nil)
}

// exchange writes one request and reads until the worker has answered it,
// answering one mid-exchange question along the way if mid says how. The caller
// holds the mutex.
//
// # Why every request comes through here, including the ones with no question
//
// mid is nil for all but one operation, and that is the point. A Response
// carrying a PIN question is refused wherever there is no mid to answer it,
// which makes "a PIN request arriving when nothing is pending" a checked
// condition on every single request rather than a thing the login path happens
// to get right. The owner's condition was that the request be bound to the
// approved operation and not to the worker being alive; nil is what "no
// operation was approved" looks like from here.
//
// A second question after the first is refused for the same reason and would be
// worse: a worker that could ask twice could ask three times, and clause 5 says
// nothing retries a PIN, ever, for any reason.
func (w *Worker) exchange(ctx context.Context, req Request, mid func(in io.Writer, needs LoginNeeds) error) (Response, error) {
	// Captured rather than read inside the goroutine: a respawn replaces these,
	// and a goroutine abandoned by a cancelled context would otherwise read the
	// fields of a worker that is no longer the one it was talking to.
	in, out := w.in, w.out

	type outcome struct {
		resp Response
		err  error
	}
	done := make(chan outcome, 1)

	go func() {
		if err := WriteFrame(in, req); err != nil {
			// A write to a pipe whose far end is gone is how a death that
			// happened between two requests is noticed.
			done <- outcome{err: fmt.Errorf("%w: writing a request: %w", ErrWorkerDied, err)}
			return
		}

		resp, err := readAnswer(out)
		if err != nil {
			done <- outcome{err: err}
			return
		}

		if resp.Login != nil {
			if mid == nil {
				done <- outcome{err: fmt.Errorf("%w: in answer to %q", ErrUnexpectedPINRequest, req.Op)}
				return
			}
			if err := mid(in, *resp.Login); err != nil {
				done <- outcome{err: err}
				return
			}
			resp, err = readAnswer(out)
			if err != nil {
				done <- outcome{err: err}
				return
			}
			if resp.Login != nil {
				done <- outcome{err: fmt.Errorf("%w: it asked twice, and clause 5 says nothing retries a PIN", ErrUnexpectedPINRequest)}
				return
			}
		}

		done <- outcome{resp: resp}
	}()

	select {
	case got := <-done:
		return got.resp, got.err
	case <-ctx.Done():
		w.killLocked()
		return Response{}, ctx.Err()
	}
}

// readAnswer reads one response frame, naming a pipe that ended mid-frame as a
// death rather than as a protocol fault — which is what it is, and what the
// supervisor above needs to tell them apart.
func readAnswer(out io.Reader) (Response, error) {
	var resp Response
	if err := ReadFrame(out, &resp); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, ErrShortFrame) {
			return Response{}, fmt.Errorf("%w: %w", ErrWorkerDied, err)
		}
		return Response{}, fmt.Errorf("pkcs11 worker: reading a response: %w", err)
	}
	return resp, nil
}

// reap makes sure the child is gone and forgets it, returning why it stopped.
// The caller holds the mutex.
func (w *Worker) reap(ctx context.Context) error {
	if w.cmd == nil {
		return nil
	}

	waited := make(chan error, 1)
	cmd := w.cmd
	go func() { waited <- cmd.Wait() }()

	var err error
	select {
	case err = <-waited:
	case <-ctx.Done():
		// A child that will not exit is killed rather than waited for. Nothing
		// here chooses how long to wait; the caller's context did.
		w.killLocked()
		err = <-waited
	}

	_ = w.in.Close()
	_ = w.out.Close()
	w.cmd, w.in, w.out = nil, nil, nil
	return err
}

// endLocked kills the child and forgets it. The caller holds the mutex.
//
// It is kill-then-reap rather than reap alone, and the order is the whole
// reason it exists. reap waits for the process to exit and only then closes the
// pipes, which is right when the child is on its way out — but a child that
// asked a question this parent refused, or that is waiting for a PIN that will
// never arrive, is blocked on a read and will wait as long as the caller's
// context allows. It is already unusable: something in this conversation went
// wrong in a way that means neither end knows where the next frame starts.
func (w *Worker) endLocked(ctx context.Context) {
	w.killLocked()
	_ = w.reap(ctx)
}

// killLocked ends the child now. The caller holds the mutex.
//
// An already-exited process answers with an error, which is ignored: the
// question this asks is "is it gone", and both answers are yes.
func (w *Worker) killLocked() {
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}
