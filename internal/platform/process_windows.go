//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
)

// ProcessRunning reports whether a process with this id exists and has
// not exited.
//
// It exists for one caller: deciding whether the "a batch is in
// flight" mark left in the registry belongs to a process that is still
// signing, or to one that died mid-batch (F10 §3.2). Both answers are
// acted on, and only one of them is safe to get wrong — a process id
// that has been reused by something unrelated reads as running, the
// mark is left alone, and an installer refuses. That is the direction
// this should fail in: refusing an upgrade because of a mark nobody
// owns costs a person one restart of the agent, and clearing a mark
// somebody does own costs them a signature.
//
// PROCESS_QUERY_LIMITED_INFORMATION rather than PROCESS_QUERY_INFORMATION
// because it is the access a standard user has to a process of their
// own in every case, including one running at a different integrity
// level.
func ProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// ERROR_INVALID_PARAMETER is "no such process". Anything else —
		// access denied, most of all — means a process is there and
		// this one may not look at it, which is still "running".
		return err != windows.ERROR_INVALID_PARAMETER
	}
	defer func() { _ = windows.CloseHandle(h) }()

	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	// STILL_ACTIVE (259) is what a process that has not exited reports.
	// A process that exited *with* 259 reads as running, which is the
	// documented ambiguity of this API and, again, the safe direction.
	return code == 259
}
