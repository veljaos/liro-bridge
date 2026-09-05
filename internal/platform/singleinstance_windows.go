//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
)

// windowsLeader claims a named mutex in the Local namespace, which is
// per logon session — so two users signed in over RDP each have their
// own, matching SPEC §14.1's "one agent instance per user session,
// never per machine". A Global mutex would let one user's signing
// batch silence another's.
type windowsLeader struct {
	name   string
	handle windows.Handle
}

// NewLeader returns a Leader claiming the named mutex name.
func NewLeader(name string) Leader { return &windowsLeader{name: `Local\` + name} }

// Acquire creates or opens the mutex and reports whether this process
// created it.
//
// ERROR_ALREADY_EXISTS is the whole signal: CreateMutexW succeeds for
// every caller, and only the first one gets a last-error saying the
// object was new. Waiting on the mutex instead would make the other
// nineteen processes queue up and each open a window in turn, which is
// the opposite of what is wanted — they should hand their file over and
// leave.
func (l *windowsLeader) Acquire() (bool, error) {
	namePtr, err := windows.UTF16PtrFromString(l.name)
	if err != nil {
		return false, err
	}
	h, err := windows.CreateMutex(nil, false, namePtr)
	if h != 0 {
		l.handle = h
	}
	if err != nil {
		if err == windows.ERROR_ALREADY_EXISTS {
			// Somebody else is the collector. The handle is still ours
			// to close.
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Release closes the handle, which destroys the mutex once every
// process holding one has done the same.
func (l *windowsLeader) Release() {
	if l.handle != 0 {
		_ = windows.CloseHandle(l.handle)
		l.handle = 0
	}
}
