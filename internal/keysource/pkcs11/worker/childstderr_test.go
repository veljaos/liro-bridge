package worker

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
)

// TestWhatAChildSaidCanBeReadWhileItIsStillSayingIt is the regression test for
// D-303, and it is written to keep doing the thing that raced rather than to
// avoid it.
//
// The race was never in the supervisor's own goroutines. It was that Worker
// hands the caller's io.Writer to os/exec, which copies the child's standard
// error into it on a goroutine that lives as long as the child — so a caller
// who read that writer was reading memory another goroutine was writing.
// Every test in this package did exactly that, and the reasoning that let it
// through was that a copier blocked in Read has not written anything yet.
// bytes.Buffer.ReadFrom grows the buffer before each read, so it has.
//
// What must now be true is that reading what a child said is safe at any time,
// including the two moments that are hardest: while a child is alive and has
// written nothing, and while several goroutines ask at once. Under -race, this
// test is the check; without it, it is a test that a string comes back.
//
// It is deliberately not a test of the buffer's contents. The contents are
// asserted by the tests that count deaths across respawns; this one is about
// whether asking is allowed.
func TestWhatAChildSaidCanBeReadWhileItIsStillSayingIt(t *testing.T) {
	// A child that dies on its second request, so that by the end of this test
	// one child has written to its standard error and a second is alive and
	// has not. Both states are read from, concurrently, on purpose.
	w := New(cannedPath(cannedAnswers{Label: "noisy", DieOnRequest: 2}), io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	ctx := context.Background()

	// A live child that has said nothing. This is the state the old arrangement
	// raced on with no bytes involved at all: os/exec's copier had already
	// written the destination before its first read returned.
	if _, err := w.Enumerate(ctx); err != nil {
		t.Fatalf("the first request: %v\nchild stderr:\n%s", err, w.ChildStderr())
	}
	if said := w.ChildStderr(); said != "" {
		t.Errorf("a child that has said nothing reports %q", said)
	}

	// Several readers while a request is in flight, a death and a respawn all
	// overlap below. Eight rather than one because a single reader interleaves
	// with the copier at one point and eight interleave at many.
	//
	// It does not establish that ChildStderr avoids the mutex that serialises
	// the pipe. It would pass either way — taking that mutex would make these
	// readers block until the request came back rather than fail — and the
	// reason they are two mutexes is written on childStderr, not checked here.
	// Saying so because the first version of this comment claimed otherwise.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = w.ChildStderr()
			}
		}()
	}

	// The death and the respawn happen underneath the readers above.
	if _, err := w.Enumerate(ctx); err != nil {
		t.Fatalf("the request after the worker died: %v\nchild stderr:\n%s",
			err, w.ChildStderr())
	}
	wg.Wait()

	if n := strings.Count(w.ChildStderr(), cannedDeathLine); n != 1 {
		t.Errorf("%d deaths were recorded for one death:\n%s", n, w.ChildStderr())
	}
}

// TestTheRecordOfWhatChildrenSaidIsBounded is the other half of keeping it.
//
// A worker is long-lived and a vendor module is not required to be quiet, so an
// unbounded record is a leak with a module's name on it. The tail is what is
// kept, because what a child says on its way out is why it went — a fixed
// prefix of a chatty module's start-up noise answers nothing.
//
// It is tested directly rather than through a child, because producing more
// than maxChildStderr bytes of real output means a child that writes eight
// kilobytes, and what that would measure is how fast a pipe is.
func TestTheRecordOfWhatChildrenSaidIsBounded(t *testing.T) {
	var c childStderr

	// Twice the bound, in chunks that do not divide it, so the trimming is
	// exercised at offsets rather than on a boundary.
	for i := 0; i < (2*maxChildStderr)/300+1; i++ {
		if _, err := c.Write([]byte(strings.Repeat("x", 300))); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if got := len(c.String()); got > maxChildStderr {
		t.Errorf("the record grew to %d bytes against a bound of %d", got, maxChildStderr)
	}

	// The tail, not the head: the last thing written must survive.
	if _, err := c.Write([]byte("the last thing it said")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.HasSuffix(c.String(), "the last thing it said") {
		t.Error("the most recent output was trimmed away, which is the half that " +
			"says why a child went")
	}
	if got := len(c.String()); got > maxChildStderr {
		t.Errorf("the record is %d bytes against a bound of %d", got, maxChildStderr)
	}
}

// TestAWriteIsReportedWholeEvenWhenItIsTrimmed pins the io.Writer contract,
// which trimming is the obvious way to break.
//
// io.Writer requires Write to return len(p) on success. Returning the number of
// bytes *kept* instead would be a short write, and io.Copy turns a short write
// into io.ErrShortWrite — which would end os/exec's copier and take the child's
// standard error with it, at exactly the point the record started mattering.
func TestAWriteIsReportedWholeEvenWhenItIsTrimmed(t *testing.T) {
	var c childStderr
	big := []byte(strings.Repeat("y", maxChildStderr*2))
	n, err := c.Write(big)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(big) {
		t.Errorf("Write reported %d of %d bytes; a short write ends io.Copy with "+
			"io.ErrShortWrite and the child's standard error goes with it", n, len(big))
	}
}
