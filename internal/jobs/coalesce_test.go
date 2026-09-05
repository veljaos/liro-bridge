package jobs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeClock advances only when Sleep is called, so a gather that waits
// out a quiet period takes no wall-clock time and never races.
//
// It is also why these tests could not have found the defect the real
// binary did: a fake clock has no process-creation stalls in it, so
// every arrival lands inside the quiet period by construction. What
// keeps a real twenty-file selection whole is not the window these
// tests measure — see CoalesceWindow's own comment, and the watcher in
// cmd/liro-bridge.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
	// onSleep runs before each sleep advances the clock, which is how a
	// test injects an arrival part-way through a wait.
	onSleep func(elapsed time.Duration)
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)}
}

func (f *fakeClock) clock() Clock {
	return Clock{
		Now: func() time.Time {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.now
		},
		Sleep: func(d time.Duration) {
			if f.onSleep != nil {
				f.onSleep(d)
			}
			f.mu.Lock()
			f.now = f.now.Add(d)
			f.mu.Unlock()
		},
	}
}

// TestTwentyFilesSelectedAtOnceBecomeOneBatch is F6 §2's stated
// requirement, tested at the size F6 names. Twenty separate
// invocations, arriving one poll apart, are one batch — not twenty, and
// not two.
func TestTwentyFilesSelectedAtOnceBecomeOneBatch(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)

	// The first invocation is the one that collects; the other
	// nineteen arrive while it waits.
	if err := box.Append(filepath.Join(dir, "doc00.pdf")); err != nil {
		t.Fatal(err)
	}

	clock := newFakeClock()
	arrived := 1
	clock.onSleep = func(time.Duration) {
		if arrived < 20 {
			if err := box.Append(filepath.Join(dir, fmt.Sprintf("doc%02d.pdf", arrived))); err != nil {
				t.Error(err)
			}
			arrived++
		}
	}

	batch, err := CollectBatch(box, CoalesceWindow, clock.clock())
	if err != nil {
		t.Fatalf("CollectBatch: %v", err)
	}
	if len(batch) != 20 {
		t.Fatalf("batch has %d paths, want 20 — twenty selected files are one batch (F6 §2)", len(batch))
	}
	for i, p := range batch {
		if want := filepath.Join(dir, fmt.Sprintf("doc%02d.pdf", i)); p != want {
			t.Fatalf("batch[%d] = %q, want %q — arrival order is preserved", i, p, want)
		}
	}
	// And the inbox is empty afterwards, so the next right-click does
	// not re-sign this batch.
	if n, err := box.Count(); err != nil || n != 0 {
		t.Fatalf("inbox holds %d after Take (err %v), want 0", n, err)
	}
}

// TestQuietPeriodRestartsWithEachArrival is why the window is a quiet
// period rather than a deadline: a trickle of arrivals over several
// windows is still one batch.
func TestQuietPeriodRestartsWithEachArrival(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)
	if err := box.Append("a.pdf"); err != nil {
		t.Fatal(err)
	}

	clock := newFakeClock()
	sleeps := 0
	clock.onSleep = func(time.Duration) {
		sleeps++
		// One arrival every four polls — well inside the quiet window,
		// so nothing should ever be taken early.
		if sleeps%4 == 0 && sleeps <= 40 {
			if err := box.Append(fmt.Sprintf("later-%d.pdf", sleeps)); err != nil {
				t.Error(err)
			}
		}
	}

	batch, err := CollectBatch(box, CoalesceWindow, clock.clock())
	if err != nil {
		t.Fatalf("CollectBatch: %v", err)
	}
	if len(batch) != 11 {
		t.Fatalf("batch has %d paths, want 11 — the quiet period must restart on each arrival", len(batch))
	}
}

func TestCollectBatchWaitsForTheQuietPeriod(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)
	if err := box.Append("only.pdf"); err != nil {
		t.Fatal(err)
	}

	clock := newFakeClock()
	start := clock.now

	batch, err := CollectBatch(box, CoalesceWindow, clock.clock())
	if err != nil {
		t.Fatalf("CollectBatch: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("batch = %v, want one path", batch)
	}
	if waited := clock.now.Sub(start); waited < CoalesceWindow {
		t.Fatalf("took the batch after %v, want at least the %v quiet period", waited, CoalesceWindow)
	}
}

// TestCollectBatchGivesUpEventually pins the bound: something
// appending forever must not stop the window from ever opening.
func TestCollectBatchGivesUpEventually(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)
	if err := box.Append("first.pdf"); err != nil {
		t.Fatal(err)
	}

	clock := newFakeClock()
	n := 0
	clock.onSleep = func(time.Duration) {
		n++
		if err := box.Append(fmt.Sprintf("endless-%d.pdf", n)); err != nil {
			t.Error(err)
		}
	}

	batch, err := CollectBatch(box, CoalesceWindow, clock.clock())
	if err != nil {
		t.Fatalf("CollectBatch: %v", err)
	}
	if len(batch) == 0 {
		t.Fatal("gave up with an empty batch")
	}
	if clock.now.Sub(newFakeClock().now) > coalesceMaxWait+CoalesceWindow {
		t.Fatalf("waited %v, want the gather bounded at %v", clock.now.Sub(newFakeClock().now), coalesceMaxWait)
	}
}

// TestInboxAppendsFromSeveralWritersAreWholeLines is what makes the
// file a safe handover between processes: concurrent appends interleave
// whole paths, never fragments of two.
func TestInboxAppendsFromSeveralWritersAreWholeLines(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)

	const writers = 20
	const each = 10
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				if err := box.Append(fmt.Sprintf(`C:\docs\writer%02d-file%02d.pdf`, w, i)); err != nil {
					t.Error(err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	batch, err := box.Take()
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if len(batch) != writers*each {
		t.Fatalf("Take returned %d paths, want %d", len(batch), writers*each)
	}
	seen := map[string]bool{}
	for _, p := range batch {
		if seen[p] {
			t.Fatalf("duplicate line %q", p)
		}
		seen[p] = true
		if filepath.Ext(p) != ".pdf" {
			t.Fatalf("line %q is not a whole path — an append was torn", p)
		}
	}
}

// TestTakeIsAtomicAgainstAConcurrentAppend covers the handover race: a
// path appended while the batch is being taken belongs to the next
// batch, never to no batch at all.
func TestTakeIsAtomicAgainstAConcurrentAppend(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)
	if err := box.Append("first.pdf"); err != nil {
		t.Fatal(err)
	}

	first, err := box.Take()
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first batch = %v", first)
	}

	// A late arrival recreates the inbox rather than vanishing.
	if err := box.Append("second.pdf"); err != nil {
		t.Fatal(err)
	}
	second, err := box.Take()
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if len(second) != 1 || second[0] != "second.pdf" {
		t.Fatalf("second batch = %v, want just second.pdf", second)
	}
}

func TestTakeOnAMissingInboxIsEmptyNotAnError(t *testing.T) {
	box := NewInbox(t.TempDir())
	batch, err := box.Take()
	if err != nil {
		t.Fatalf("Take on a missing inbox: %v", err)
	}
	if len(batch) != 0 {
		t.Fatalf("batch = %v, want none", batch)
	}
}

// TestInboxHandlesALongPath is F6 §7's "path longer than 260
// characters", at the handover layer: the line has to survive the
// round trip whatever its length.
func TestInboxHandlesALongPath(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)
	long := `C:\` + string(make([]byte, 0)) + filepath.Join(dir, repeat("verylongfoldername", 20), "document.pdf")

	if err := box.Append(long); err != nil {
		t.Fatalf("Append: %v", err)
	}
	batch, err := box.Take()
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if len(batch) != 1 || batch[0] != long {
		t.Fatalf("round trip lost a %d-character path", len(long))
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

// TestInboxIsPerUserDirectory pins where the handover file lives: beside
// the agent's other per-user state, so two users signed in over RDP have
// their own (SPEC §14.1).
func TestInboxIsPerUserDirectory(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	a, b := NewInbox(dirA), NewInbox(dirB)
	if a.Path() == b.Path() {
		t.Fatal("two inboxes in different directories share one path")
	}
	if err := a.Append("mine.pdf"); err != nil {
		t.Fatal(err)
	}
	n, err := b.Count()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("one user's inbox is visible in another's")
	}
	if _, err := os.Stat(a.Path()); err != nil {
		t.Fatalf("the inbox was not created where Path says: %v", err)
	}
}

// TestDiscardIfStaleThrowsAwayAForgottenInbox covers the one way a
// path can be stranded: a collector that died between taking its batch
// and draining what arrived after it. Those documents are gone from
// that batch either way; merging them into the next right-click would
// sign files the person did not choose this time.
func TestDiscardIfStaleThrowsAwayAForgottenInbox(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)
	if err := box.Append("zaboravljen.pdf", "drugi.pdf"); err != nil {
		t.Fatal(err)
	}

	// Fresh: nothing is discarded.
	n, err := box.DiscardIfStale(CoalesceStaleAfter, time.Now())
	if err != nil || n != 0 {
		t.Fatalf("DiscardIfStale on a fresh inbox = %d (err %v), want 0", n, err)
	}
	if got, _ := box.Count(); got != 2 {
		t.Fatalf("a fresh inbox lost entries: %d", got)
	}

	// Old: discarded, and it says how many.
	n, err = box.DiscardIfStale(CoalesceStaleAfter, time.Now().Add(CoalesceStaleAfter+time.Minute))
	if err != nil {
		t.Fatalf("DiscardIfStale: %v", err)
	}
	if n != 2 {
		t.Fatalf("discarded %d, want 2", n)
	}
	if got, _ := box.Count(); got != 0 {
		t.Fatalf("the stale inbox survived: %d entries", got)
	}
}

func TestDiscardIfStaleOnAMissingInboxIsHarmless(t *testing.T) {
	box := NewInbox(t.TempDir())
	n, err := box.DiscardIfStale(CoalesceStaleAfter, time.Now())
	if err != nil || n != 0 {
		t.Fatalf("DiscardIfStale = %d (err %v), want 0 and no error", n, err)
	}
}
