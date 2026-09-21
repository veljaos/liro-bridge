package main

import (
	"context"
	"sync"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// countingWindow is a window that does nothing and remembers that it
// was asked to. It is not a fake WebView2 or a fake GTK window: this
// test is about how many windows a run creates and what becomes of
// them, which is a question about cmd/liro-bridge and not about either
// host.
type countingWindow struct {
	mu     sync.Mutex
	closed int
}

func (w *countingWindow) PostJSON(any) error          { return nil }
func (w *countingWindow) Eval(string) (string, error) { return "", nil }
func (w *countingWindow) Navigate(string) error       { return nil }
func (w *countingWindow) Resize(int, int) error       { return nil }
func (w *countingWindow) Handle() uintptr             { return 0 }

func (w *countingWindow) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed++
	return nil
}

func (w *countingWindow) closedTimes() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

// TestEveryRunGetsItsOwnWindowAndNoneIsKept enforces SPEC §6.5.2's
// first clause.
//
// **"A new window for every request. Never a hidden window shown again,
// never one re-used between requests."** The reason that clause exists
// is measured (D-337): on the compositor this was written for, a *new*
// window is given focus even by a process that has been idle for
// forty-five seconds, while an existing window cannot be raised at all
// — gtk_window_present on an unfocused window does nothing, and a
// clicked notification does not raise it either. So a re-used window is
// not a small economy on that platform. It is a consent request the
// person may never see, for as long as the window it would have been
// shown in is behind something else.
//
// The property was true before the clause existed, by accident of how
// open was written. This is what would notice if somebody made the
// natural optimisation — hold the window between requests, it is
// expensive to create — which on Windows would be invisible and correct
// and on Wayland would be a signature nobody is asked about.
func TestEveryRunGetsItsOwnWindowAndNoneIsKept(t *testing.T) {
	var created []*countingWindow

	original := newUIWindow
	t.Cleanup(func() { newUIWindow = original })
	newUIWindow = func(ui.Options) (ui.Window, error) {
		w := &countingWindow{}
		created = append(created, w)
		return w, nil
	}

	// Cancelled before the first run: open puts the window up, posts
	// the first step and then finds its loop already over, which is the
	// shortest complete run this flow has.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	const runs = 3
	for i := 0; i < runs; i++ {
		m := newSigningFlow(config.Default(), "en", flowRequest{})
		m.open(ctx, nil, stepCertificate)
	}

	if len(created) != runs {
		t.Fatalf("%d runs created %d windows, want one each: a window is being re-used "+
			"between requests, which SPEC §6.5.2 forbids", runs, len(created))
	}
	for i, w := range created {
		for j, other := range created {
			if i != j && w == other {
				t.Fatalf("runs %d and %d were handed the same window", i, j)
			}
		}
		if got := w.closedTimes(); got != 1 {
			t.Errorf("window %d was closed %d times, want exactly once: a window that "+
				"outlives its run is a window available to be shown again", i, got)
		}
	}
}
