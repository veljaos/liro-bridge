//go:build windows

package platform

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetProcessHandleCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

// processHandles is this process's kernel handle count, the number Task
// Manager's Handles column shows.
func processHandles(t *testing.T) int {
	t.Helper()
	var n uint32
	r, _, err := procGetProcessHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		t.Fatalf("GetProcessHandleCount: %v", err)
	}
	return int(n)
}

// TestLockingTheSameDirectoryManyTimesKeepsAtMostOneHandle is C21 row 2
// ([[D-453]], [[D-457]]): the agent makes a new store, and so a new lock,
// for every audit record, and each lock opened the named mutex and never
// closed it — one kernel handle per record for the life of the agent.
//
// The count is the process's own, read by GetProcessHandleCount before
// and after, so what is asserted is what Task Manager would show, not
// this package's bookkeeping. The lock file's handle is closed by Unlock
// on every path; the mutex is what this test is about.
func TestLockingTheSameDirectoryManyTimesKeepsAtMostOneHandle(t *testing.T) {
	dir := t.TempDir()
	const cycles = 200

	// One cycle first, so that whatever a first lock costs once per
	// process — the mutex itself under the fix, the lazy DLL loads —
	// is not counted against the rest.
	first := NewDirLock(dir)
	if _, err := first.Lock(5 * time.Second); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	first.Unlock()
	if first.(*windowsDirLock).mutex == 0 {
		t.Skip("no named mutex on this machine, so there is no mutex handle to count")
	}

	runtime.GC()
	before := processHandles(t)
	for i := 0; i < cycles; i++ {
		l := NewDirLock(dir)
		if _, err := l.Lock(5 * time.Second); err != nil {
			t.Fatalf("Lock %d: %v", i, err)
		}
		l.Unlock()
	}
	runtime.GC()
	after := processHandles(t)

	// A few handles may come and go with the runtime's own threads; what
	// must not happen is one per cycle.
	const slack = 10
	t.Logf("%d lock cycles: %d handles before, %d after", cycles, before, after)
	if after-before > slack {
		t.Errorf("%d lock cycles left %d more kernel handles open (%d before, %d after): "+
			"one per lock, so one per audit record for the life of the agent",
			cycles, after-before, before, after)
	}
}
