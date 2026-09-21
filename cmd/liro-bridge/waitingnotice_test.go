package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

type fakeNotifier struct {
	posted  int
	closed  int
	summary string
	body    string
	err     error
}

type fakeNotification struct{ n *fakeNotifier }

func (f fakeNotification) Close() { f.n.closed++ }

func (n *fakeNotifier) Notify(summary, body string) (platform.Notification, error) {
	if n.err != nil {
		return nil, n.err
	}
	n.posted++
	n.summary, n.body = summary, body
	return fakeNotification{n: n}, nil
}

func useFakeNotifier(t *testing.T) *fakeNotifier {
	t.Helper()
	f := &fakeNotifier{}
	original := newNotifier
	t.Cleanup(func() { newNotifier = original })
	newNotifier = func() platform.Notifier { return f }
	return f
}

// remoteFlow is a run that came from a caller rather than from the
// person at the keyboard, with nothing of the protocol server around
// it: the only thing these tests need from "remote" is that it is what
// the window uses to tell the two apart.
func remoteFlow(app string) *mainWindow {
	m := newSigningFlow(config.Default(), "en", flowRequest{})
	m.remote = &remoteBatch{
		req:           api.SignRequest{Application: app},
		job:           &jobs.Job{ID: "job-for-a-test", Owner: app, Total: 1},
		stopCountdown: make(chan struct{}),
		expired:       make(chan struct{}),
	}
	// No audit store: these tests are about a notification, and a
	// refusal recorded on the way out must not reach anybody's disk.
	m.auditStore = func() (*audit.Store, error) { return nil, errors.New("no audit store in this test") }
	return m
}

// TestARequestFromACallerIsAnnouncedAndWithdrawn covers SPEC §6.5.2's
// second clause and the sentence about taking it down again.
func TestARequestFromACallerIsAnnouncedAndWithdrawn(t *testing.T) {
	f := useFakeNotifier(t)
	m := remoteFlow("My ERP")

	m.announceWaiting()
	if f.posted != 1 {
		t.Fatalf("a request from a caller posted %d notifications, want 1", f.posted)
	}
	if !strings.Contains(f.body, "My ERP") {
		t.Errorf("the notification body is %q, which does not name the application that asked", f.body)
	}
	if f.summary == "" {
		t.Error("the notification has no summary, so a desktop would show an empty banner")
	}

	m.withdrawWaiting()
	if f.closed != 1 {
		t.Fatalf("the notification was withdrawn %d times, want 1: one still saying a signature "+
			"is waiting, for a batch answered ten minutes ago, is worse than never posting it", f.closed)
	}
	// However a run ends, it may reach this twice.
	m.withdrawWaiting()
	if f.closed != 1 {
		t.Errorf("withdrawing twice closed %d times, want 1", f.closed)
	}
}

// TestADesktopWithNoNotificationServiceStopsNothing is the best-effort
// clause: "its absence is never a refusal".
func TestADesktopWithNoNotificationServiceStopsNothing(t *testing.T) {
	f := useFakeNotifier(t)
	f.err = errors.New("no session bus")
	m := remoteFlow("My ERP")

	m.announceWaiting()
	if m.waiting != nil {
		t.Error("a notification that was never posted is being held as though it had been")
	}
	// The interesting assertion is that this returns at all: nothing
	// above panicked, nothing refused, and the run carries on to its
	// window.
	m.withdrawWaiting()
}

// TestAPersonsOwnRunIsNotAnnounced is the other half of the second
// clause, and it is the direction that would be noise rather than
// silence: somebody who has just typed `liro-bridge sign` is looking at
// the screen they asked for.
func TestAPersonsOwnRunIsNotAnnounced(t *testing.T) {
	f := useFakeNotifier(t)

	original := newUIWindow
	t.Cleanup(func() { newUIWindow = original })
	newUIWindow = func(ui.Options) (ui.Window, error) { return &countingWindow{}, nil }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := newSigningFlow(config.Default(), "en", flowRequest{})
	m.open(ctx, nil, stepCertificate)

	if f.posted != 0 {
		t.Fatalf("a local run posted %d notifications, want none: the person opened this window", f.posted)
	}
}
