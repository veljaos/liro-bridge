//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A test that points LOCALAPPDATA at a temporary directory has to delete
// it again, and this one could not: a WebView2 window created while
// LOCALAPPDATA points there puts its user-data folder underneath it, and
// the browser process group holding those files open was measured still
// running two hours after the test binary that started it had exited.
// The cleanup's failure was discarded — `_ = os.RemoveAll(dir)` — so
// nothing said so, and `go test ./...` left about six megabytes in
// %TEMP% every time it ran. Measured after this pass's suite runs: 196
// directories, 3 365 files, 1.26 GB (FTEST Group 3, C-6).
//
// That is the same family as B-1 and B-10 — a test that changes the
// machine and does not put it back — and the same answer: put it back,
// and prove the putting back.
//
// The proof is deliberately about the mechanism rather than about what
// is in %TEMP% right now. A test that asserted the latter would go red
// for a directory some earlier run left stuck, which is a verdict about
// history rather than about this run — the trap D-112 records and
// D-171 hit again in this same pass.

func TestASweptConfigHomeIsActuallyGone(t *testing.T) {
	tmp := os.TempDir()

	old := filepath.Join(tmp, configHomePrefix+"sweep-old-"+t.Name())
	fresh := filepath.Join(tmp, configHomePrefix+"sweep-fresh-"+t.Name())
	stranger := filepath.Join(tmp, "not-a-liro-config-"+t.Name())
	for _, d := range []string{old, fresh, stranger} {
		if err := os.MkdirAll(filepath.Join(d, "webview2-profile"), 0o755); err != nil {
			t.Fatalf("seeding %s: %v", d, err)
		}
		if err := os.WriteFile(filepath.Join(d, "config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatalf("seeding %s: %v", d, err)
		}
	}
	t.Cleanup(func() {
		for _, d := range []string{old, fresh, stranger} {
			_ = os.RemoveAll(d)
		}
	})

	// Two hours back, so the one-hour cutoff is cleared with room
	// rather than raced.
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	for _, d := range []string{old, stranger} {
		if err := os.Chtimes(d, twoHoursAgo, twoHoursAgo); err != nil {
			t.Fatalf("ageing %s: %v", d, err)
		}
	}

	sweepStaleConfigHomes()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("a config home older than the cutoff survived the sweep: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("a config home younger than the cutoff was swept: %v", err)
	}
	if _, err := os.Stat(stranger); err != nil {
		t.Errorf("a directory that is not a config home was swept: %v", err)
	}
}

// removeWithRetry is what tempConfigHome's own cleanup calls, and it has
// to actually delete a directory nothing is holding — the sweep is the
// fallback for the case where something is, not the first line.
func TestAConfigHomeNothingIsHoldingIsDeletedAtOnce(t *testing.T) {
	dir := filepath.Join(os.TempDir(), configHomePrefix+"immediate-"+t.Name())
	if err := os.MkdirAll(filepath.Join(dir, "webview2-profile", "EBWebView"), 0o755); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	start := time.Now()
	removeWithRetry(dir)
	elapsed := time.Since(start)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		_ = os.RemoveAll(dir)
		t.Fatalf("the config home is still there after removeWithRetry: %v", err)
	}
	// It must not have spent the retry budget on a directory that was
	// free from the first attempt.
	if elapsed > time.Second {
		t.Errorf("removing a directory nothing was holding took %s: the first attempt should have succeeded",
			elapsed.Round(time.Millisecond))
	}
}
