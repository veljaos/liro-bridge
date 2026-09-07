package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// J-8 weighed one kind of held-open destination: a person has the
// previous signed document open in a PDF reader. That is a state which
// persists, and OUTPUT_IN_USE tells them what to do about it.
//
// There is another kind, and on Windows it is far more common: something
// opens the file for a moment and lets go. An antivirus scanner reading
// a file that has just appeared in a folder, a search indexer, a backup
// agent, Explorer's own preview pane, a folder-watching sync client.
// Against that, a rename that gives up at the first refusal turns a
// passing glance into a refused signature — and one the person cannot
// act on, because by the time they read "close it and try again" it is
// already closed.
//
// Measured before the retry existed: with a tight os.Stat loop running
// against the destination, os.Rename over it failed 30 times in 200 —
// 15 per cent.

// glanceHold is how long the reader below keeps the destination open.
// Comfortably inside renameRetryBudget, and long enough that the first
// rename attempt is certain to be refused, so this cannot pass for the
// wrong reason on a fast machine.
const glanceHold = 300 * time.Millisecond

// TestARenameSurvivesSomethingGlancingAtTheDestination models the glance
// as what it is: a reader that holds the destination and then lets go,
// while the write is under way.
//
// The first version of this test span two `os.Stat` loops instead, which
// is how the defect was originally measured — and it failed once in a
// twenty-run suite pass, on a machine with six fuzzers on it. That
// failure was correct and the test was wrong: two unthrottled stat loops
// on a saturated machine do not model a glance, they model a file that
// is open essentially all the time, and no bounded retry can or should
// survive that. A test whose verdict depends on how busy the machine is
// measures the machine (D-112), so it was replaced by this one, which
// says exactly how long the file is held and asserts against a budget
// that is stated rather than raced for.
func TestARenameSurvivesSomethingGlancingAtTheDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document-signed.pdf")
	original := []byte("the previously signed document")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	release, held := holdFileOpen(t, path)
	if !held {
		t.Skip("this platform's rename is not refused by a reader holding the destination")
	}

	// Let go partway through the retry budget, which is what a scanner
	// that has finished reading does.
	releasedAt := make(chan time.Time, 1)
	go func() {
		time.Sleep(glanceHold)
		release()
		releasedAt <- time.Now()
	}()

	newData := []byte("the freshly signed one")
	start := time.Now()
	err := WriteFileAtomic(path, newData, 0o600)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("a reader that held the destination for %s and let go made the write fail after %s: %v\n"+
			"a file another program merely looked at must not cost a signature",
			glanceHold, elapsed.Round(time.Millisecond), err)
	}
	if elapsed < glanceHold {
		t.Fatalf("the write finished in %s, before the reader let go at %s: "+
			"the rename was never actually refused, so this proves nothing",
			elapsed.Round(time.Millisecond), glanceHold)
	}
	<-releasedAt

	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(newData) {
		t.Fatalf("the destination holds %q (err %v), want %q", got, err, newData)
	}
	assertNoTemporaryFilesLeft(t, dir)
}

// The other half: a destination something is genuinely holding is still
// refused, and within a bounded time rather than eventually. The retry
// changes how long "held open" has to last, not whether it counts.
func TestARenameStillRefusesADestinationThatStaysOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document-signed.pdf")
	original := []byte("the previously signed document")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	release, held := holdFileOpen(t, path)
	if !held {
		t.Skip("this platform's rename is not refused by a reader holding the destination")
	}
	defer release()

	start := time.Now()
	err := WriteFileAtomic(path, []byte("the new one"), 0o600)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("WriteFileAtomic replaced a destination held open throughout")
	}
	// Bounded, not instant: the retry must give up rather than keep
	// going. Generous, because this runs alongside everything else in
	// the suite and the assertion is about the shape of the wait, not
	// about how fast this machine is.
	if elapsed > 10*renameRetryBudget {
		t.Errorf("refusing took %s, more than ten times the %s budget: the retry is not bounded",
			elapsed.Round(time.Millisecond), renameRetryBudget)
	}
	if got, readErr := os.ReadFile(path); readErr != nil || string(got) != string(original) {
		t.Fatalf("the original was disturbed: %q (err %v)", got, readErr)
	}
	assertNoTemporaryFilesLeft(t, dir)
}
