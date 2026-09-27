//go:build !windows

package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// A SIGTERM sent to this very process reaches quit instead of ending the
// test binary: the handler is registered before quitOnTerminate returns, so
// there is no window in which the signal would take Go's default (D-393, D20).
func TestQuitOnTerminateCallsQuitOnSIGTERM(t *testing.T) {
	called := make(chan struct{})
	stop := quitOnTerminate(func() { close(called) })
	defer stop()

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("sending SIGTERM to this process: %v", err)
	}
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not reach quit within 5 s")
	}
}

// Stopping without a signal must not quit: runTray's defer runs this on every
// ordinary exit, and a quit from there would be a second close of the channel
// Quit already closed (sync.OnceFunc guards it in runTray, but this function
// should not rely on that).
func TestQuitOnTerminateStopDoesNotQuit(t *testing.T) {
	called := make(chan struct{}, 1)
	stop := quitOnTerminate(func() { called <- struct{}{} })
	stop()
	select {
	case <-called:
		t.Fatal("quit was called with no signal")
	case <-time.After(200 * time.Millisecond):
	}
}
