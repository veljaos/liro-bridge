//go:build !windows

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// posixDirLock is an advisory whole-file lock (flock) on a file inside
// the directory. The file *is* the name: two processes resolving the
// same directory open the same inode and therefore contend, which is
// the scope DirLock requires, with no namespace question to answer.
//
// It never reports DirLockAbandoned. flock releases when the holding
// process dies — the file description goes away with it — but says
// nothing about whether it died mid-write, which is the difference
// between this and the Windows named mutex. That costs nothing today:
// the agent is Windows-only until phase 13, and this implementation
// exists so that internal/audit's behaviour is one behaviour on every
// platform it is tested on rather than two.
type posixDirLock struct {
	path string
	name string
	f    *os.File
}

// NewDirLock returns a lock over dir. The directory must exist;
// audit.NewStore creates it before anything asks for the lock.
func NewDirLock(dir string) DirLock {
	return &posixDirLock{
		path: filepath.Join(dir, dirLockFileName),
		// Never the path: it sits under a user's home directory, which
		// carries their name, and SPEC §18.3 forbids one in any log
		// file.
		name: "flock+" + dirLockID(dir),
	}
}

func (l *posixDirLock) Name() string { return l.name }

func (l *posixDirLock) Lock(timeout time.Duration) (DirLockState, error) {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return DirLockHeld, err
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			l.f = f
			return DirLockHeld, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return DirLockHeld, err
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return DirLockHeld, ErrDirLockTimeout
		}
		time.Sleep(dirLockPoll)
	}
}

func (l *posixDirLock) Unlock() {
	if l.f == nil {
		return
	}
	// Closing the file releases the lock; unlocking first keeps the two
	// facts separate for a reader, and costs one syscall.
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}
