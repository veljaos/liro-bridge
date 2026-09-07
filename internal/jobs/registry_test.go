package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// testClock is a clock a test moves by hand, so a ten-minute deadline
// is reached in microseconds and nothing here measures the machine
// (D-112).
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestOneJobPerApplicationAtATime(t *testing.T) {
	clock := newTestClock()
	r := NewRegistry(clock.Now)

	first, err := r.Submit("app-1", 3, "fp")
	if err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	if _, err := r.Submit("app-1", 1, "fp"); !errors.Is(err, ErrJobInProgress) {
		t.Fatalf("second Submit for the same application: %v, want ErrJobInProgress", err)
	}
	// A different application is unaffected: the limit is per
	// application, not per agent.
	if _, err := r.Submit("app-2", 1, "fp"); err != nil {
		t.Fatalf("Submit for a second application: %v", err)
	}

	// The slot frees when the job finishes, not when its result is
	// collected — otherwise a caller that never collects is locked out
	// for the whole ten minutes.
	first.Complete(clock.Now(), []byte(`{"ok":true}`), Update{Completed: 3})
	if _, err := r.Submit("app-1", 1, "fp"); err != nil {
		t.Fatalf("Submit after the first job finished: %v", err)
	}
}

func TestTheResultIsDeliveredOnceAndThenForgotten(t *testing.T) {
	clock := newTestClock()
	r := NewRegistry(clock.Now)
	job, err := r.Submit("app", 1, "fp")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	job.Complete(clock.Now(), []byte(`{"signatures":[]}`), Update{Completed: 1})

	body, ok := job.TakeResult()
	if !ok || string(body) != `{"signatures":[]}` {
		t.Fatalf("first TakeResult: %q, ok %v", body, ok)
	}
	if _, ok := job.TakeResult(); ok {
		t.Fatal("the result was handed over twice")
	}

	r.Forget(job.ID)
	if _, ok := r.Get(job.ID); ok {
		t.Fatal("the job is still in the registry after being forgotten")
	}
}

func TestAJobNobodyCollectedIsDiscardedAfterTheDeadline(t *testing.T) {
	clock := newTestClock()
	r := NewRegistry(clock.Now).WithTTL(DefaultJobTTL)
	job, err := r.Submit("app", 1, "fp")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Unfinished jobs are never discarded, however long they take: a
	// signature in flight has its own timeouts, and forgetting it from
	// underneath the person signing would leave the caller with no way
	// to find out what happened.
	clock.advance(DefaultJobTTL * 3)
	r.Sweep()
	if _, ok := r.Get(job.ID); !ok {
		t.Fatal("an unfinished job was discarded")
	}

	job.Complete(clock.Now(), []byte(`{}`), Update{Completed: 1})
	clock.advance(DefaultJobTTL - time.Second)
	r.Sweep()
	if _, ok := r.Get(job.ID); !ok {
		t.Fatal("a finished job was discarded a second before its deadline")
	}
	clock.advance(2 * time.Second)
	r.Sweep()
	if _, ok := r.Get(job.ID); ok {
		t.Fatal("a finished job survived its deadline")
	}
}

func TestFollowSeesEveryStateAndAlwaysEndsOnTheTerminalOne(t *testing.T) {
	clock := newTestClock()
	r := NewRegistry(clock.Now)
	job, err := r.Submit("app", 2, "fp")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	seen := make(chan JobState, 16)
	done := make(chan error, 1)
	go func() {
		done <- job.Follow(context.Background(), func(u Update) error {
			seen <- u.State
			return nil
		})
	}()

	expect := func(want JobState) {
		t.Helper()
		select {
		case got := <-seen:
			if got != want {
				t.Fatalf("saw %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("waiting for %q", want)
		}
	}

	expect(JobQueued)
	job.Publish(Update{State: JobAwaitingConsent, RemainingConsent: 120 * time.Second})
	expect(JobAwaitingConsent)
	job.Publish(Update{State: JobPreparingCard})
	expect(JobPreparingCard)
	job.Complete(clock.Now(), []byte(`{}`), Update{Completed: 2})
	expect(JobCompleted)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Follow returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Follow did not return after the terminal state")
	}
}

// TestFollowIsNotRacedByAFastRun is the property a buffered channel of
// updates would not have: a run that publishes every state before the
// follower has read any of them must still leave the follower on the
// terminal state rather than dropping it.
func TestFollowIsNotRacedByAFastRun(t *testing.T) {
	clock := newTestClock()
	r := NewRegistry(clock.Now)
	job, err := r.Submit("app", 100, "fp")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	for i := 0; i < 100; i++ {
		job.Publish(Update{State: JobSigning, Completed: i + 1})
	}
	job.Complete(clock.Now(), []byte(`{}`), Update{Completed: 100})

	var last Update
	if err := job.Follow(context.Background(), func(u Update) error {
		last = u
		return nil
	}); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if last.State != JobCompleted {
		t.Fatalf("the follower ended on %q, want %q", last.State, JobCompleted)
	}
}

func TestATerminalStateIsNeverPublishedOver(t *testing.T) {
	clock := newTestClock()
	r := NewRegistry(clock.Now)
	job, err := r.Submit("app", 1, "fp")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	job.Fail(clock.Now(), errs.CodeConsentDenied, Update{})
	// A late progress hook from a run that has already ended must not
	// bring the job back to life.
	job.Publish(Update{State: JobSigning, Completed: 1})
	job.Complete(clock.Now(), []byte(`{}`), Update{Completed: 1})

	got := job.Snapshot()
	if got.State != JobFailed || got.Code != errs.CodeConsentDenied {
		t.Fatalf("state is %q/%q, want failed/CONSENT_DENIED", got.State, got.Code)
	}
	if _, ok := job.TakeResult(); ok {
		t.Fatal("a failed job handed over a result")
	}
}

func TestFollowStopsWhenTheClientGoesAway(t *testing.T) {
	clock := newTestClock()
	r := NewRegistry(clock.Now)
	job, err := r.Submit("app", 1, "fp")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- job.Follow(ctx, func(Update) error { return nil })
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Follow returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Follow did not return when its context was cancelled")
	}
}
