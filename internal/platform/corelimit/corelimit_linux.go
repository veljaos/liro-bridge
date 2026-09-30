//go:build linux

// Package corelimit sets this process's core-file limit to zero, and does
// nothing else.
//
// It is its own package so that the file-chooser helper (internal/chooser)
// can set the limit without importing internal/platform, which also holds the
// Secret Service client and the keyring fallback. The helper's import guard
// allows it this package and no other of this module's, and a leaf with one
// function is something that guard can allow without allowing anything else.
package corelimit

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Set sets RLIMIT_CORE to zero, soft and hard.
//
// The hard limit too, so that nothing loaded later can raise the soft one
// again. It is inherited by every child. It does not touch the dumpable
// flag: platform.ForbidCoreDumps does that on top of this, and the chooser
// helper deliberately does not (D-408, D-409).
func Set() error {
	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return fmt.Errorf("setting RLIMIT_CORE to zero: %w", err)
	}
	return nil
}
