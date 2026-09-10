package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// DirLock is a lock over a directory, shared by every process on this
// machine that resolves the same directory — whatever session that
// process is running in.
//
// It exists for the audit log (SPEC §6.7). A hash chain's defining
// property is that an entry cannot be altered without breaking every
// entry after it, and appending to one is a read-then-write: the last
// entry is read, the next Sequence and PrevHash are computed from it,
// and one line is written. Two processes doing that at the same moment
// both read the same last entry and both write a successor to it, which
// forks the chain — and a forked chain reports itself as tampered with
// from the fork onwards, for ever (measured, D-223).
//
// # Two mechanisms, each doing the thing it is good at
//
// The exclusion is a whole-file lock on a lock file inside the
// directory (LockFileEx on Windows, flock elsewhere). The file *is* the
// name: two processes that resolve the same directory open the same
// file and therefore contend, in any session, under any account, with
// no privilege and no namespace to get wrong. That is the guarantee
// this type makes, and nothing conditional stands behind it.
//
// The abandoned-writer signal is a named mutex, and only that. When the
// process holding a Windows mutex dies, the next waiter's wait returns
// WAIT_ABANDONED and it is granted ownership — which does not merely
// avoid a deadlock, it says that the previous writer died in the middle
// of a write. A file lock releases on process death too, but silently:
// the next writer gets it and cannot tell whether the last line on disk
// is whole. So the mutex is taken first, purely to learn that, and
// failing to get it is fatal to nothing.
//
// The split is why the scope claim needs no footnote. An earlier
// arrangement had the mutex carrying the exclusion, with a Local\ name
// as the fallback where a Global\ one could not be created — and Local\
// is per logon session while the audit directory is per *user*
// (ConfigDir), so on such a machine one user's console session and RDP
// session, or an interactive agent and a scheduled task in session 0,
// would silently have stopped sharing a lock over one directory. Two
// spellings of one directory (a substituted drive, an 8.3 short name)
// would have done the same thing on any machine, since a mutex name is
// derived from the path. Neither is possible now: the file is the same
// file however it was reached, and the mutex is only ever a hint.
//
// The contract:
//
//   - Lock and Unlock must be called from the same goroutine, and
//     Unlock only after a Lock that returned no error. On Windows a
//     mutex is owned by a thread, not a process, so the implementation
//     pins the goroutine to its OS thread for the duration of the hold.
//   - Lock never waits longer than the timeout it is given. Waiting
//     for ever over a log is not an option: the alternative to
//     appending is not "sign later", it is "do not sign".
type DirLock interface {
	// Lock waits up to timeout for the lock. It returns DirLockHeld for
	// an ordinary acquisition, DirLockAbandoned when the previous
	// holder died while holding it, and ErrDirLockTimeout when the wait
	// expired without the lock being taken.
	Lock(timeout time.Duration) (DirLockState, error)

	// Unlock releases the lock. It is only valid after a Lock that
	// returned no error.
	Unlock()

	// Name identifies the lock on this machine, for diagnostics. It
	// never contains the directory's path: a path under a user profile
	// carries the person's name, and SPEC §18.3 forbids one in any log
	// file.
	Name() string
}

// DirLockState says how the lock was obtained.
type DirLockState int

const (
	// DirLockHeld is an ordinary acquisition: the previous holder
	// released it, or nobody held it.
	DirLockHeld DirLockState = iota

	// DirLockAbandoned means the process that held the lock exited
	// without releasing it, and Windows said so. It is a signal about
	// the *data*, not about the lock: what a caller does with it is
	// verify what it is about to append to.
	//
	// Best-effort by construction. A named object exists only while
	// some handle to it is open, so the signal reaches a waiter that
	// already held one when the writer died — the contended case — and
	// a long-lived agent holding one is what makes it reach further.
	// Nothing depends on it: a torn last line does not parse, which is
	// caught on every read regardless (D-166).
	DirLockAbandoned
)

// ErrDirLockTimeout is returned by Lock when the wait expired.
var ErrDirLockTimeout = errors.New("platform: the directory lock could not be taken within the time allowed")

// dirLockFileName is the lock file, inside the directory it locks. The
// name begins with a dot and does not end in ".jsonl", so nothing that
// reads the audit directory sees it: internal/audit globs "*.jsonl" and
// nothing else.
const dirLockFileName = ".lock"

// dirLockPoll is how often a waiting process retries. Neither flock(2)
// nor LockFileEx has a timed wait, and a blocking one cannot be
// cancelled — so the wait is a bounded retry, which is what makes
// Lock's timeout mean anything.
const dirLockPoll = 2 * time.Millisecond

// dirLockID identifies a directory in one short string, for a mutex
// name and for diagnostics.
//
// Hashed rather than embedded, twice over: a Windows object name cannot
// contain a backslash and a path is mostly backslashes, and a path
// under a user profile carries the person's name, which SPEC §18.3
// forbids in any log file. Lower-cased and cleaned because Windows
// paths are case-insensitive and "C:\a\b" and "C:\a\.\b" are one
// directory.
//
// Two spellings of one directory that survive that — an 8.3 short name,
// a substituted drive — hash differently and do not share a mutex. That
// costs nothing: the mutex carries no exclusion, only the
// abandoned-writer signal, and the lock file they both open is the same
// file whichever way it was named.
func dirLockID(dir string) string {
	clean := strings.ToLower(filepath.Clean(dir))
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:8])
}
