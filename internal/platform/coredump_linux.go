//go:build linux

package platform

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// ForbidCoreDumps stops this process — and, for the limit, everything it
// starts — from leaving a core image of its memory on disk (SPEC §6.5.1
// clause 3; D-376).
//
// D-355 measured that a crash here can produce a full core with a secret in
// it, written by apport, and that the only thing preventing one by default is
// the desktop's soft limit of 0: somebody else's setting, which a person or a
// distribution can change without knowing what it protects. So the program
// sets its own, and sets two things because they fail differently:
//
//   - RLIMIT_CORE to zero, soft and hard. The hard limit too, so that nothing
//     loaded later — a vendor module — can raise the soft one again. It is
//     inherited by every child, including the web processes and the PKCS#11
//     worker.
//   - PR_SET_DUMPABLE to 0. This one is not inherited across execve, which
//     is why every process of this binary calls this function itself.
//
// Which of the two keeps a core off disk on a given machine depends on the
// kernel's core_pattern and whoever it pipes to; D-376 measured each alone
// and both together against apport on Ubuntu 24.04.
func ForbidCoreDumps() error {
	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return fmt.Errorf("platform: setting RLIMIT_CORE to zero: %w", err)
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("platform: clearing the dumpable flag: %w", err)
	}
	return nil
}
