package signing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// fakeClock lets tests advance time deterministically instead of
// sleeping for real seconds.
type fakeClock struct {
	t time.Time
}

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestSession(inner keysource.Session, clock *fakeClock) *Session {
	return newSession(inner, clock.now)
}

func TestSignDigestSucceedsWithinAllWindows(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	sess := newTestSession(&fakeKeySession{}, clock)
	if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err != nil {
		t.Fatalf("SignDigest: %v", err)
	}
}

// TestIdleTimeoutClosesSession is the failing test for F2 §4.1: a
// session that has been idle for more than 90s must refuse to sign
// again, and the underlying handle must be released.
func TestIdleTimeoutClosesSession(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	inner := &fakeKeySession{}
	sess := newTestSession(inner, clock)

	if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err != nil {
		t.Fatalf("first SignDigest: %v", err)
	}

	clock.advance(IdleTimeout + time.Second)
	_, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32))
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("error = %v, want ErrSessionExpired", err)
	}
	if !inner.closed {
		t.Fatal("the underlying session must be closed on idle timeout")
	}
}

func TestIdleTimeoutDoesNotFireWithinWindow(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	sess := newTestSession(&fakeKeySession{}, clock)
	if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err != nil {
		t.Fatalf("first SignDigest: %v", err)
	}
	clock.advance(IdleTimeout - time.Second)
	if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err != nil {
		t.Fatalf("SignDigest just under the idle timeout: %v", err)
	}
}

// TestMaxLifetimeClosesSessionEvenUnderContinuousActivity is the
// failing test for F2 §4.1's other half: unlike idle timeout, this
// fires regardless of how recently the session was used.
func TestMaxLifetimeClosesSessionEvenUnderContinuousActivity(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	inner := &fakeKeySession{}
	sess := newTestSession(inner, clock)

	// Keep the session "continuously active" — never idle — but let
	// wall-clock time exceed MaxLifetime.
	for i := 0; i < 5; i++ {
		if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err != nil {
			t.Fatalf("SignDigest #%d: %v", i, err)
		}
		clock.advance(time.Second)
	}
	clock.advance(MaxLifetime)

	_, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32))
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("error = %v, want ErrSessionExpired (maximum lifetime), even though the session was kept continuously active", err)
	}
}

// TestApprovalWindowExpiresBeforeFirstSignature is F2 §4.1's third
// number: if signing has not begun within 120s of the session opening,
// something is wrong.
func TestApprovalWindowExpiresBeforeFirstSignature(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	sess := newTestSession(&fakeKeySession{}, clock)

	clock.advance(ApprovalWindow + time.Second)
	_, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32))
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("error = %v, want ErrSessionExpired (approval window)", err)
	}
}

func TestApprovalWindowDoesNotApplyAfterFirstSignature(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	sess := newTestSession(&fakeKeySession{}, clock)

	if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err != nil {
		t.Fatalf("first SignDigest: %v", err)
	}
	// Advance past ApprovalWindow (120s) in steps that each stay under
	// IdleTimeout (90s), so idle timeout never fires and the only
	// question is whether ApprovalWindow wrongly still applies after
	// the first signature already happened. It must not.
	for i := 0; i < 3; i++ {
		clock.advance(60 * time.Second)
		if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err != nil {
			t.Fatalf("SignDigest #%d, well past the approval window but never idle: %v", i, err)
		}
	}
}

func TestManagerOpenReusesLiveSession(t *testing.T) {
	src := &fakeKeySource{session: &fakeKeySession{cert: keysource.Certificate{Thumbprint: "ABC"}}}
	mgr := NewManager()
	mgr.now = func() time.Time { return time.Now() }

	s1, err := mgr.Open(context.Background(), src, "ABC")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s2, err := mgr.Open(context.Background(), src, "ABC")
	if err != nil {
		t.Fatalf("Open (second): %v", err)
	}
	if s1 != s2 {
		t.Fatal("Open must reuse a live session for the same thumbprint")
	}
	if src.opens != 1 {
		t.Fatalf("source.Open called %d times, want 1", src.opens)
	}
}

func TestManagerOpenReplacesExpiredSession(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	src := &fakeKeySource{session: &fakeKeySession{cert: keysource.Certificate{Thumbprint: "ABC"}}}
	mgr := NewManager()
	mgr.now = clock.now

	if _, err := mgr.Open(context.Background(), src, "ABC"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	clock.advance(MaxLifetime + time.Second)
	if _, err := mgr.Open(context.Background(), src, "ABC"); err != nil {
		t.Fatalf("Open (after expiry): %v", err)
	}
	if src.opens != 2 {
		t.Fatalf("source.Open called %d times, want 2 (expired session must be replaced)", src.opens)
	}
}

func TestManagerOpenPropagatesSourceError(t *testing.T) {
	src := &fakeKeySource{session: &fakeKeySession{}, openErr: errFakeOpen}
	mgr := NewManager()
	if _, err := mgr.Open(context.Background(), src, "ABC"); !errors.Is(err, errFakeOpen) {
		t.Fatalf("error = %v, want errFakeOpen", err)
	}
}

func TestManagerSupportsMultipleThumbprintsConcurrently(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		thumb := keysource.Thumbprint(fmt.Sprintf("T%d", i%5))
		src := &fakeKeySource{session: &fakeKeySession{cert: keysource.Certificate{Thumbprint: thumb}}}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := mgr.Open(context.Background(), src, thumb); err != nil {
				t.Errorf("Open: %v", err)
			}
		}()
	}
	wg.Wait()
	if mgr.Len() == 0 {
		t.Fatal("expected sessions to be registered")
	}
}

func TestManagerCloseForgetsSession(t *testing.T) {
	src := &fakeKeySource{session: &fakeKeySession{cert: keysource.Certificate{Thumbprint: "ABC"}}}
	mgr := NewManager()
	if _, err := mgr.Open(context.Background(), src, "ABC"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := mgr.Close("ABC"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if mgr.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 after Close", mgr.Len())
	}
	if !src.session.closed {
		t.Fatal("underlying session must be closed")
	}
}
