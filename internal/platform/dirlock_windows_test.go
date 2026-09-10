//go:build windows

package platform

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestTheLockNameComesFromTheDirectoryAndNotTheSession is the property
// SPEC §14.1 and the audit directory's own location force. The name
// must be a function of the path — so a console session and an RDP
// session of one user share it — and nothing else.
func TestTheLockNameComesFromTheDirectoryAndNotTheSession(t *testing.T) {
	const dir = `C:\Users\somebody\AppData\Local\Liro\audit`

	if a, b := dirLockID(dir), dirLockID(dir); a != b {
		t.Fatalf("the same directory produced two names: %q and %q", a, b)
	}
	if a, b := dirLockID(dir), dirLockID(dir+`\..\audit`); a != b {
		t.Errorf("a directory and the same directory spelled differently produced %q and %q", a, b)
	}
	if a, b := dirLockID(dir), dirLockID(`C:\Users\somebody-else\AppData\Local\Liro\audit`); a == b {
		t.Errorf("two users' audit directories share the lock name %q", a)
	}
}

// TestTheNameNeverCarriesThePath: SPEC §18.3 forbids a file name — and
// a fortiori a path under a user profile, which carries the person's
// name — in any log file, and audit.Append logs this name when a wait
// expires.
func TestTheNameNeverCarriesThePath(t *testing.T) {
	dir := t.TempDir()
	name := NewDirLock(dir).Name()
	for _, part := range []string{dir, filepath.Base(dir), "\\", "/", ":"} {
		if contains(name, part) {
			t.Errorf("the lock name %q carries %q from its directory", name, part)
		}
	}
}

func contains(s, sub string) bool {
	if sub == "" {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// The two tests below are the cross-session case, reproduced by its
// mechanism rather than by two logon sessions — which cannot be created
// on a machine without administrative rights, and which is exactly why
// the guarantee must not rest on a namespace.
//
// What two sessions do to a Local\ mutex is make it two different
// objects over one directory. So: two locks over one directory whose
// mutexes cannot possibly be the same object. If the exclusion holds
// there, it holds across sessions, because nothing else about a session
// reaches a file lock.

// unsharedPair returns two locks over one directory whose mutexes are
// different objects — the state two logon sessions would produce with a
// session-scoped name.
func unsharedPair(t *testing.T, dir string) (a, b *windowsDirLock) {
	t.Helper()
	lockPath := filepath.Join(dir, dirLockFileName)
	id := dirLockID(dir)
	a = &windowsDirLock{
		globalName: `Global\` + dirLockPrefix + id + ".sessionA",
		localName:  `Local\` + dirLockPrefix + id + ".sessionA",
		lockPath:   lockPath,
		mutexName:  "flock+" + id,
	}
	b = &windowsDirLock{
		globalName: `Global\` + dirLockPrefix + id + ".sessionB",
		localName:  `Local\` + dirLockPrefix + id + ".sessionB",
		lockPath:   lockPath,
		mutexName:  "flock+" + id,
	}
	return a, b
}

func TestTwoLocksThatCannotShareAMutexStillExcludeEachOther(t *testing.T) {
	dir := t.TempDir()
	a, b := unsharedPair(t, dir)

	// Prove the premise rather than assume it: the two mutexes really
	// are different objects.
	a.openMutex()
	b.openMutex()
	if a.mutex == 0 || b.mutex == 0 {
		t.Skip("no named mutex on this machine, so there is nothing to be unshared")
	}
	if a.globalName == b.globalName {
		t.Fatal("the two locks share a mutex name, so this test proves nothing")
	}

	assertExcludes(t, a, b)
}

// TestALockWithNoMutexAtAllStillExcludes is the fallback path,
// measured rather than reasoned about: a machine whose policy refuses a
// named object of any kind. The mutex handle is left at zero, which is
// what openMutex produces there, and the guard must be untouched.
func TestALockWithNoMutexAtAllStillExcludes(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, dirLockFileName)

	noMutex := func() *windowsDirLock {
		return &windowsDirLock{
			lockPath:   lockPath,
			mutexName:  "flock+none",
			mutexTried: true, // openMutex is a no-op; mutex stays 0
		}
	}
	a, b := noMutex(), noMutex()
	assertExcludes(t, a, b)

	if a.mutex != 0 || b.mutex != 0 {
		t.Fatal("a mutex was opened after all, so this test measured the wrong thing")
	}
}

// assertExcludes: while one holds the lock the other cannot take it,
// and once the first lets go the second gets it. Observed by what each
// side was able to do and in which order, never by how long either took
// (D-201).
func assertExcludes(t *testing.T, first, second *windowsDirLock) {
	t.Helper()

	if _, err := first.Lock(5 * time.Second); err != nil {
		t.Fatalf("first Lock: %v", err)
	}

	// While it is held, a bounded wait on the other side must expire.
	// Waited on to completion before anything is released — an earlier
	// version of this helper released the first lock without waiting,
	// which made the second side's attempt race the release and be
	// granted for a perfectly good reason.
	expired := make(chan error, 1)
	go func() {
		_, err := second.Lock(50 * time.Millisecond)
		expired <- err
	}()
	if err := <-expired; !errors.Is(err, ErrDirLockTimeout) {
		if err == nil {
			second.Unlock()
		}
		first.Unlock()
		t.Fatalf("the second lock was granted while the first held it (err %v)", err)
	}

	// And once the first lets go, the second gets it. Observed by what
	// each side was able to do and in which order, never by how long
	// either took (D-201).
	var mu sync.Mutex
	var order []string

	taken := make(chan error, 1)
	go func() {
		_, err := second.Lock(10 * time.Second)
		if err == nil {
			mu.Lock()
			order = append(order, "second")
			mu.Unlock()
			second.Unlock()
		}
		taken <- err
	}()

	mu.Lock()
	order = append(order, "first")
	mu.Unlock()
	first.Unlock()

	if err := <-taken; err != nil {
		t.Fatalf("second Lock: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("order = %v, want [first second]", order)
	}
}

// TestGlobalNamedObjectsAreAvailableToThisAccount records, as a check
// rather than a comment, the measurement the fallback exists for.
//
// It does not fail where a machine's policy refuses: that is a
// supported configuration and the point of the fallback. It fails only
// if CreateMutexW starts returning something neither expected.
func TestGlobalNamedObjectsAreAvailableToThisAccount(t *testing.T) {
	h, err := createMutex(`Global\LiroBridge.Test.` + dirLockID(t.TempDir()))
	if err == nil {
		_ = windows.CloseHandle(h)
		return
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Log("this machine refuses Global\\ names; the session-scoped signal and the file lock are what run here")
		return
	}
	t.Fatalf("CreateMutex on a Global name failed with something unexpected: %v", err)
}
