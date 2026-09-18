package worker

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// TestOneWorkerServesManyRequests is the property the whole arrangement exists
// for, and it is checked against real processes and a real pipe rather than
// against the loop with a fake handler.
//
// D-297: discovery pays C_Initialize per candidate because a crash there is the
// expected outcome; a session must not, because paying it per call rolls
// D-272's dice every time. The observable form of "it does not pay per call" is
// that three requests reach one process — which, on a child that counts what it
// has served, is visible as the third answer still coming from a module that
// was opened once.
func TestOneWorkerServesManyRequests(t *testing.T) {
	w := New(cannedPath(cannedAnswers{Label: "one module"}), io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	ctx := context.Background()

	certs, err := w.Enumerate(ctx)
	if err != nil {
		t.Fatalf("Enumerate: %v\nchild stderr:\n%s", err, w.ChildStderr())
	}
	if len(certs) != 1 || certs[0].Label != "one module" {
		t.Fatalf("Enumerate came back as %+v", certs)
	}

	if _, err := w.List(ctx); err != nil {
		t.Fatalf("List: %v", err)
	}

	chain, err := w.ChainFor(ctx, "ABCDEF")
	if err != nil {
		t.Fatalf("ChainFor: %v", err)
	}
	if len(chain) != 1 || string(chain[0]) != "ABCDEF" {
		t.Fatalf("ChainFor came back as %q", chain)
	}

	// One child served all three. A worker that respawned between requests
	// would have written a start-up line for each, and a worker that died would
	// have written a reason.
	if w.ChildStderr() != "" {
		t.Errorf("the child wrote to its standard error while serving three "+
			"requests, which means it was not one child:\n%s", w.ChildStderr())
	}
}

// TestADeadWorkerIsRespawnedAndTheRequestIsAnswered is F12 §2's exit property in
// the half that can be demonstrated on demand.
//
// D-294 is explicit that the module's half cannot be: "no number of clean runs
// demonstrates 'when it dies, the parent survives'; only a death does", and a
// death cannot be scheduled. What a child that ends on purpose demonstrates is
// the parent's own handling, which D-296 established is a real measurement
// because the parent reads a status from a process that genuinely ended.
//
// The child here answers once and then stops. The parent must notice, spawn
// another, and answer the second request — and this process must still be
// running at the end, which is the thing F12 §2 is actually about.
func TestADeadWorkerIsRespawnedAndTheRequestIsAnswered(t *testing.T) {
	w := New(cannedPath(cannedAnswers{Label: "answered", DieOnRequest: 2}), io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	ctx := context.Background()

	if _, err := w.Enumerate(ctx); err != nil {
		t.Fatalf("the first request: %v", err)
	}

	// The second request reaches a child that ends rather than answering. The
	// respawned one is a fresh child, so this is its own first request and it
	// answers it.
	certs, err := w.Enumerate(ctx)
	if err != nil {
		t.Fatalf("the request after the worker died: %v\nchild stderr:\n%s",
			err, w.ChildStderr())
	}
	if len(certs) != 1 || certs[0].Label != "answered" {
		t.Fatalf("the respawned worker answered with %+v", certs)
	}

	// Exactly one child died, so exactly one respawn happened. More would mean
	// the parent is respawning over something that is not a death.
	if n := strings.Count(w.ChildStderr(), cannedDeathLine); n != 1 {
		t.Errorf("%d children died for one death:\n%s", n, w.ChildStderr())
	}
}

// TestAWorkerThatKeepsDyingIsGivenUpOnRatherThanRespawnedForEver is the bound.
//
// It is a count and not an interval, which is D-297's own instruction: the only
// duration this project has measured around a dying child is D-296's 341–376 ms
// reap, and D-296 says in as many words that what those milliseconds are spent
// on is not established. A backoff sized from an unexplained number is how a
// magic constant gets into a supervisor and stays there.
//
// The child here dies before answering anything, so every attempt fails. The
// parent must stop after maxAttempts and say so, rather than spawning processes
// until something changes.
func TestAWorkerThatKeepsDyingIsGivenUpOnRatherThanRespawnedForEver(t *testing.T) {
	// Every child ends on its own first request, so no attempt can succeed.
	w := New(cannedPath(cannedAnswers{DieOnRequest: 1}), io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	_, err := w.Enumerate(context.Background())
	if !errors.Is(err, ErrWorkerDied) {
		t.Fatalf("a worker that never answers produced %v, want ErrWorkerDied", err)
	}
	if n := strings.Count(w.ChildStderr(), cannedDeathLine); n != maxAttempts {
		t.Errorf("the parent tried %d workers, want %d:\n%s",
			n, maxAttempts, w.ChildStderr())
	}
	if !strings.Contains(err.Error(), "attempts") {
		t.Errorf("the error does not say it gave up after trying: %v", err)
	}
}

// TestTheRecursionGuardStopsAWorkerSpawningAWorker is D-293's guard, at the
// second spawner.
//
// The guard is one rule read by both, and this is the half that would otherwise
// only be checked for the probe. A worker child that spawned a worker would
// recurse exactly as the probe did — os.Executable() is the same binary, and a
// binary that ignores the subcommand runs its suite, which spawns again.
//
// Both directions, because a guard that has never been seen to refuse is not
// one (D-031): without the marker a worker starts, and with it nothing is
// spawned at all.
func TestTheRecursionGuardStopsAWorkerSpawningAWorker(t *testing.T) {
	// The control first. If this process is already marked, the treatment below
	// proves nothing.
	if pkcs11.IsChild() {
		t.Fatalf("%s is already set in this process's environment, so the "+
			"treatment below would pass without the guard doing anything",
			pkcs11.ChildMarker)
	}

	live := New(cannedPath(cannedAnswers{Label: "started"}), io.Discard)
	if _, err := live.Enumerate(context.Background()); err != nil {
		t.Fatalf("unmarked, a worker should start and answer: %v", err)
	}
	_ = live.Close(context.Background())

	// Treatment: marked, exactly as pkcs11.ChildEnv marks the children this
	// package spawns. Nothing else about the call changes.
	t.Setenv(pkcs11.ChildMarker, "1")

	refused := New(cannedPath(cannedAnswers{}), io.Discard)
	_, err := refused.Enumerate(context.Background())
	if !errors.Is(err, pkcs11.ErrChildRecursion) {
		t.Fatalf("a marked process spawned a worker: %v\n\n"+
			"That marker is the only thing that stops a child which does not "+
			"dispatch the subcommand from spawning children of its own. "+
			"Measured without it, for the probe: 254 processes to 827.", err)
	}
	if !errors.Is(err, ErrWorkerCannotStart) {
		t.Errorf("the refusal is not reported as a worker that could not "+
			"start: %v", err)
	}
}

// TestAModuleThatWillNotLoadIsAnErrorAndNotACrash is F11 §3's rule at the
// worker: "A module that fails to load is not a crash. Say which path, say the
// module did not load, carry on."
//
// This goes through the real Run rather than the canned server, so the child
// really does try to load the path it was given and really does fail. The path
// is one that cannot exist, so it says nothing about anybody's installed
// middleware and runs anywhere.
func TestAModuleThatWillNotLoadIsAnErrorAndNotACrash(t *testing.T) {
	const notAModule = `C:\this\path\does\not\exist\nothing.dll`

	w := New(notAModule, io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	_, err := w.Enumerate(context.Background())
	if err == nil {
		t.Fatal("a module that cannot be loaded produced no error")
	}
	if !errors.Is(err, ErrWorkerDied) {
		t.Errorf("a child that exited without answering is reported as %v, "+
			"want ErrWorkerDied", err)
	}
	if !strings.Contains(err.Error(), notAModule) {
		t.Errorf("the error does not name the module: %v", err)
	}
	// F11 §3 asks for a readable reason, and the child's own is on the standard
	// error it inherited. Three attempts, three reasons.
	if n := strings.Count(w.ChildStderr(), "pkcs11 worker:"); n != maxAttempts {
		t.Errorf("the child wrote %d reasons for %d attempts:\n%s",
			n, maxAttempts, w.ChildStderr())
	}

	// And this process is still running, which is the whole of F12 §2.
	if os.Getpid() == 0 {
		t.Fatal("unreachable")
	}
}

// TestANonRetryableOperationIsTriedOnce is the other half of the retry rule,
// and it is the half that matters most for what comes next.
//
// SPEC §6.5.1 clause 5: "Nothing retries a PIN automatically, ever, for any
// reason. One wrong PIN is one attempt. Three block the card." A login is not
// in the retryable set and must never be added to it — and because the set is
// an allow-list, an operation added later is not retried until somebody says
// so, which is the direction that cannot go wrong quietly (D-259).
//
// Shutdown stands in for it here because it is the one non-retryable operation
// that exists. What is measured is the mechanism: a request outside the set
// reaches one worker and not three.
func TestANonRetryableOperationIsTriedOnce(t *testing.T) {
	if retryable[OpShutdown] {
		t.Fatal("shutdown is in the retryable set, so this test measures nothing")
	}

	w := New(`C:\this\path\does\not\exist\nothing.dll`, io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	if _, err := w.do(context.Background(), Request{Op: OpShutdown}); err == nil {
		t.Fatal("a shutdown to a worker that will not start produced no error")
	}
	if n := strings.Count(w.ChildStderr(), "pkcs11 worker:"); n != 1 {
		t.Errorf("a non-retryable operation reached %d workers, want 1:\n%s",
			n, w.ChildStderr())
	}
}

// TestCloseIsSafeTwiceAndWithNothingRunning covers the two ways Close is
// reached that are not "there is a worker and it is answering": the ordinary
// second call, and a Worker nothing has asked anything of.
func TestCloseIsSafeTwiceAndWithNothingRunning(t *testing.T) {
	ctx := context.Background()

	never := New(cannedPath(cannedAnswers{}), nil)
	if err := never.Close(ctx); err != nil {
		t.Errorf("Close with nothing spawned: %v, want nil", err)
	}

	w := New(cannedPath(cannedAnswers{Label: "x"}), io.Discard)
	if _, err := w.Enumerate(ctx); err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if err := w.Close(ctx); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := w.Close(ctx); err != nil {
		t.Errorf("Close a second time: %v, want nil", err)
	}
}

// TestAWorkerAnsweringOneCallerAtATime is why the mutex is on this type rather
// than left to the orchestration layer.
//
// One pipe carries one request at a time. Two callers interleaving frames would
// corrupt both, and the corruption would look like a protocol fault rather than
// like a race — which is the kind of thing that gets diagnosed as "the worker is
// flaky" for a week.
func TestAWorkerAnsweringOneCallerAtATime(t *testing.T) {
	w := New(cannedPath(cannedAnswers{Label: "shared"}), io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			certs, err := w.Enumerate(context.Background())
			if err != nil {
				errs <- err
				return
			}
			if len(certs) != 1 || certs[0].Label != "shared" {
				errs <- errors.New("an answer came back malformed, which is what " +
					"interleaved frames look like")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent request: %v", err)
	}
}
