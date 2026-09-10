//go:build windows

package platform

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// dirLockPrefix names the mutex so that a person looking at the
// machine's named objects can tell what it belongs to. The directory's
// hash follows it; see dirLockID.
const dirLockPrefix = "LiroBridge.Dir."

// waitTimeout is WAIT_TIMEOUT. golang.org/x/sys/windows defines
// WAIT_ABANDONED, WAIT_OBJECT_0 and WAIT_FAILED but not this one.
const waitTimeout = 0x00000102

// windowsDirLock is LockFileEx over a lock file in the directory, with
// a named mutex in front of it that carries the abandoned-writer
// signal and nothing else. See DirLock for why it is those two and not
// one of them.
//
// Order matters and is fixed: the mutex first, then the file. Fixed so
// that no process can hold one while waiting for the other in the
// opposite order, and mutex-first specifically because that is what
// makes the abandoned signal reach anybody. A waiter blocked on the
// mutex holds a handle to it, which keeps the object alive when its
// owner dies, and is then the thread told about it. A waiter that took
// the file first would find the mutex uncontended and learn nothing.
//
// Spending the whole budget on the mutex costs nothing: whoever holds
// it holds the file lock too, or is a moment from taking it, so the
// file wait would have expired as well.
type windowsDirLock struct {
	globalName string
	localName  string
	lockPath   string

	// mutex is best-effort. A zero handle means this machine would not
	// give one, which loses the abandoned signal and nothing else.
	mutex      windows.Handle
	mutexName  string
	mutexTried bool
	warnOnce   sync.Once

	// held for the duration of one Lock/Unlock pair.
	mutexHeld bool
	file      windows.Handle
	overlap   windows.Overlapped
}

// NewDirLock returns a lock over dir. The directory must exist;
// audit.NewStore creates it before anything asks for the lock.
func NewDirLock(dir string) DirLock {
	id := dirLockID(dir)
	return &windowsDirLock{
		globalName: `Global\` + dirLockPrefix + id,
		localName:  `Local\` + dirLockPrefix + id,
		lockPath:   filepath.Join(dir, dirLockFileName),
		mutexName:  "flock+" + id,
	}
}

func (l *windowsDirLock) Name() string { return l.mutexName }

// openMutex creates or opens the named mutex, once. A failure is
// recorded and not retried: it is a property of the machine, not of
// this moment, and the lock works without it.
//
// Global\ rather than Local\ because a signal about a per-user
// directory should reach every session that can resolve it. Creating an
// object in the global namespace is documented as requiring
// SeCreateGlobalPrivilege; measured on Windows 11 Pro 26200 it does
// not, even from a child process whose primary token has every
// privilege deleted and BUILTIN\Administrators turned deny-only — a
// token strictly weaker than a standard user's. Where a machine's
// policy does refuse, this falls back to the session-scoped name, which
// narrows the signal and not the guard: the guard is the lock file
// below, and it has no namespace at all.
func (l *windowsDirLock) openMutex() {
	if l.mutexTried {
		return
	}
	l.mutexTried = true

	h, err := createMutex(l.globalName)
	if err == nil {
		l.mutex = h
		return
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		l.warnOnce.Do(func() {
			slog.Warn("audit: the log's abandoned-writer signal is unavailable on this machine; the log's own lock is unaffected",
				"lock", l.mutexName, "error", err)
		})
		return
	}
	if h, err = createMutex(l.localName); err != nil {
		l.warnOnce.Do(func() {
			slog.Warn("audit: the log's abandoned-writer signal is unavailable on this machine; the log's own lock is unaffected",
				"lock", l.mutexName, "error", err)
		})
		return
	}
	l.mutex = h
	l.warnOnce.Do(func() {
		slog.Warn("audit: this machine does not allow a machine-wide object name, so the log's abandoned-writer signal covers this logon session only; the log's own lock is unaffected",
			"lock", l.mutexName)
	})
}

// createMutex returns a handle to the named mutex, creating it if it
// does not exist. ERROR_ALREADY_EXISTS is success with a handle
// attached — CreateMutexW returns one to every caller and uses the last
// error only to say who created it, which is not a question this lock
// asks.
func createMutex(name string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateMutex(nil, false, p)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return 0, err
	}
	if h == 0 {
		return 0, fmt.Errorf("platform: CreateMutex(%s) returned no handle", name)
	}
	return h, nil
}

// Lock waits up to timeout for the file lock, taking the mutex on the
// way for what it says about the last writer.
//
// The goroutine is pinned to its OS thread first. A Windows mutex is
// owned by the thread that waited on it, and a goroutine may be moved
// between threads at any function call — so without this, ReleaseMutex
// could run on a thread that does not own the mutex and fail with
// ERROR_NOT_OWNER, leaving it held until the process exits.
func (l *windowsDirLock) Lock(timeout time.Duration) (DirLockState, error) {
	runtime.LockOSThread()
	deadline := time.Now().Add(timeout)

	state := l.takeMutex(deadline)

	if err := l.takeFile(deadline); err != nil {
		l.releaseMutex()
		runtime.UnlockOSThread()
		return DirLockHeld, err
	}
	return state, nil
}

// takeMutex waits for the mutex until deadline and reports what the
// wait said. Every failure is silent and returns DirLockHeld: this is
// the signal, not the guard.
func (l *windowsDirLock) takeMutex(deadline time.Time) DirLockState {
	l.openMutex()
	if l.mutex == 0 {
		return DirLockHeld
	}

	event, err := windows.WaitForSingleObject(l.mutex, millisecondsUntil(deadline))
	if err != nil {
		slog.Warn("audit: waiting for the log's abandoned-writer signal failed; the log's own lock is unaffected",
			"lock", l.mutexName, "error", err)
		return DirLockHeld
	}
	switch event {
	case windows.WAIT_OBJECT_0:
		l.mutexHeld = true
		return DirLockHeld
	case windows.WAIT_ABANDONED:
		l.mutexHeld = true
		return DirLockAbandoned
	case waitTimeout:
		return DirLockHeld
	default:
		slog.Warn("audit: waiting for the log's abandoned-writer signal returned an unexpected result",
			"lock", l.mutexName, "event", event)
		return DirLockHeld
	}
}

// takeFile is the guard: an exclusive whole-file lock on the lock file,
// retried until deadline. One attempt is always made, even with no time
// left, because an expired budget and a busy lock are different things
// and only the second is a timeout.
func (l *windowsDirLock) takeFile(deadline time.Time) error {
	p, err := windows.UTF16PtrFromString(l.lockPath)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}

	for {
		l.overlap = windows.Overlapped{}
		err = windows.LockFileEx(h,
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &l.overlap)
		if err == nil {
			l.file = h
			return nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			_ = windows.CloseHandle(h)
			return err
		}
		if !time.Now().Before(deadline) {
			_ = windows.CloseHandle(h)
			return ErrDirLockTimeout
		}
		time.Sleep(dirLockPoll)
	}
}

// Unlock releases the file lock, then the mutex, then the thread.
func (l *windowsDirLock) Unlock() {
	if l.file != 0 {
		if err := windows.UnlockFileEx(l.file, 0, 1, 0, &l.overlap); err != nil {
			slog.Error("audit: releasing the log's lock failed", "lock", l.mutexName, "error", err)
		}
		_ = windows.CloseHandle(l.file)
		l.file = 0
	}
	l.releaseMutex()
	runtime.UnlockOSThread()
}

func (l *windowsDirLock) releaseMutex() {
	if !l.mutexHeld {
		return
	}
	if err := windows.ReleaseMutex(l.mutex); err != nil {
		slog.Error("audit: releasing the log's abandoned-writer signal failed", "lock", l.mutexName, "error", err)
	}
	l.mutexHeld = false
}

// millisecondsUntil is what is left of a budget, never negative.
func millisecondsUntil(deadline time.Time) uint32 {
	ms := time.Until(deadline).Milliseconds()
	if ms < 0 {
		return 0
	}
	return uint32(ms)
}
