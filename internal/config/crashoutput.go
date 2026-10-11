package config

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

// crashFileName is where the Go runtime's own report of a fatal error or
// an unrecovered panic goes, beside bridge.log.
//
// A file of its own rather than bridge.log. The rotating writer renames
// bridge.log when it fills, and a second handle held for the life of the
// process would keep writing to the renamed file — or, on Windows, stand
// in the way of the rename. And a crash report is many lines of plain
// text, which would break every reader that takes bridge.log as one JSON
// object per line.
const crashFileName = "crash.log"

// SetupCrashOutput sends the runtime's report of a crash to crash.log in
// logDir, in addition to stderr.
//
// Why this exists. A fatal error ends the process inside the runtime: no
// deferred call runs, and nothing this program logs says it stopped. The
// runtime writes its report to stderr, and the agent has usually given
// its console back by then (detachAllocatedConsole on Windows) — so the
// report reached bridge.log only by what looks like an accident, a
// reused handle, which nothing guarantees and which would go silent the
// day it stopped happening ([[D-454]]). This makes the record a choice.
//
// The file is opened for appending and never truncated: a crash is rare,
// and the reports before the last one are evidence too. It is closed here
// because SetCrashOutput keeps a duplicate of its own.
func SetupCrashOutput(logDir string) error {
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logDir, crashFileName), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}
