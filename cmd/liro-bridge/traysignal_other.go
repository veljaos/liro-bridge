//go:build !windows

package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// quitOnTerminate makes SIGTERM end the tray the way its Quit item does, and
// returns the function that stops listening.
//
// SIGTERM is how a logout ends the agent. With Go's default the process died
// of it on the spot and none of runTray's deferred calls ran: the discovery
// file stayed behind naming a port nobody answers, and the log recorded no
// ending (D-393). A next start dials the port rather than believing the file
// (D-344), but a stale file is exactly the case dialling exists for, and a
// caller reading it meets a dead port first — or, under linger, meets it the
// next day. Routed through quit, the file is removed by protocol.stop and the
// workers are shut down by run's own deferred close, in the order Quit uses.
//
// Only the first SIGTERM is taken. After it the default is restored, so a
// second one — a session manager that has stopped waiting — ends the process
// at once rather than being ignored while the shutdown runs.
func quitOnTerminate(quit func()) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case s := <-ch:
			signal.Stop(ch)
			slog.Info("tray: asked to terminate, so stopping the way Quit does", "signal", s.String())
			quit()
		case <-done:
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}
