package platform

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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
// 15 per cent. This is that measurement as a test.
func TestARenameSurvivesSomethingGlancingAtTheDestination(t *testing.T) {
	// Enough attempts that a 15 per cent failure rate is not something
	// this can pass by luck: the odds of twenty clean runs at that rate
	// are about one in twenty-five thousand.
	const attempts = 20

	for i := 0; i < attempts; i++ {
		dir := t.TempDir()
		path := filepath.Join(dir, "document-signed.pdf")
		if err := os.WriteFile(path, []byte(strings.Repeat("o", 300*1024)), 0o600); err != nil {
			t.Fatalf("seeding: %v", err)
		}

		stop := make(chan struct{})
		var wg sync.WaitGroup
		// Two lookers rather than one, because what is being reproduced
		// is a coincidence and two of them find it sooner.
		for w := 0; w < 2; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					_, _ = os.Stat(path)
				}
			}()
		}

		err := WriteFileAtomic(path, []byte(strings.Repeat("n", 1024*1024)), 0o600)
		close(stop)
		wg.Wait()

		if err != nil {
			t.Fatalf("attempt %d of %d: something reading the destination made the write fail: %v\n"+
				"a file another program merely looked at must not cost a signature",
				i+1, attempts, err)
		}
		if got, err := os.ReadFile(path); err != nil || len(got) != 1024*1024 {
			t.Fatalf("attempt %d: the destination holds %d bytes (err %v), want the whole new file",
				i+1, len(got), err)
		}
	}
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

	release, ok := holdFileOpen(t, path)
	if !ok {
		t.Skip("this platform's rename is not refused by a reader holding the destination")
	}
	defer release()

	start := time.Now()
	err := WriteFileAtomic(path, []byte("the new one"), 0o600)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("WriteFileAtomic replaced a destination held open throughout")
	}
	if elapsed > 4*renameRetryBudget {
		t.Errorf("refusing took %s, more than four times the %s budget: the retry is not bounded",
			elapsed.Round(time.Millisecond), renameRetryBudget)
	}
	if got, readErr := os.ReadFile(path); readErr != nil || string(got) != string(original) {
		t.Fatalf("the original was disturbed: %q (err %v)", got, readErr)
	}
	assertNoTemporaryFilesLeft(t, dir)
}
