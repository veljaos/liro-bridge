//go:build windows

package main

// The watcher that keeps an open window collecting stragglers (F6 §2).
//
// This is the half that makes a multiple selection whole, and the half
// no fake clock could have told us we needed: measured against the real
// binary, twenty Explorer invocations spread over 2390ms with a 1070ms
// stall in the middle, and the coalescing window alone left ten
// documents in the inbox with nobody to open them.

import (
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

func TestWatchInboxDeliversLateArrivals(t *testing.T) {
	box := jobs.NewInbox(t.TempDir())
	dropped := make(chan []string, 4)
	stop := make(chan struct{})
	defer close(stop)

	go watchInbox(box, dropped, stop)

	// Nothing yet: the watcher must not invent an empty batch.
	select {
	case got := <-dropped:
		t.Fatalf("the watcher delivered %v from an empty inbox", got)
	case <-time.After(3 * inboxPollInterval):
	}

	if err := box.Append(`C:\docs\kasni.pdf`); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-dropped:
		if len(got) != 1 || got[0] != `C:\docs\kasni.pdf` {
			t.Fatalf("the watcher delivered %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a document appended after the window opened was never delivered")
	}

	// And the inbox is emptied, so it is not delivered twice.
	if n, err := box.Count(); err != nil || n != 0 {
		t.Fatalf("inbox holds %d after delivery (err %v), want 0", n, err)
	}

	// A second arrival is delivered too: the watcher keeps watching.
	if err := box.Append(`C:\docs\jos-kasnije.pdf`); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-dropped:
		if len(got) != 1 || got[0] != `C:\docs\jos-kasnije.pdf` {
			t.Fatalf("the watcher delivered %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher stopped after one delivery")
	}
}

func TestWatchInboxStopsWhenAsked(t *testing.T) {
	box := jobs.NewInbox(t.TempDir())
	dropped := make(chan []string, 4)
	stop := make(chan struct{})

	done := make(chan struct{})
	go func() { defer close(done); watchInbox(box, dropped, stop) }()

	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher did not stop when its window closed")
	}
}

// TestLateArrivalsReachTheQueue is the property the watcher exists for,
// through the window's own event loop: a straggler joins the list that
// is already on screen, exactly as a dropped file does.
func TestLateArrivalsReachTheQueue(t *testing.T) {
	dir := t.TempDir()
	first := writeTestPDF(t, dir, "prvi.pdf", 10)
	late := writeTestPDF(t, dir, "kasni.pdf", 10)

	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{first})
	if m.queue.Len() != 1 {
		t.Fatalf("the queue starts with %d documents", m.queue.Len())
	}

	// What the watcher does when it finds something.
	m.addPaths([]string{late})

	if m.queue.Len() != 2 {
		t.Fatalf("a late arrival did not join the queue: %d documents", m.queue.Len())
	}
	rows := evalNumber(t, m.win, "document.querySelectorAll('#file-list .file-row').length")
	if rows != 2 {
		t.Fatalf("the list shows %v rows after a late arrival, want 2", rows)
	}
}
