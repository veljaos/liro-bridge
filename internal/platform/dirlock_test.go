package platform

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestALockIsHeldAgainstASecondCaller: the second caller waits, and
// gets it once the first lets go. Observed by what the two callers were
// able to do and in which order, never by how long either of them took
// (D-201).
func TestALockIsHeldAgainstASecondCaller(t *testing.T) {
	dir := t.TempDir()

	first := NewDirLock(dir)
	if _, err := first.Lock(5 * time.Second); err != nil {
		t.Fatalf("first Lock: %v", err)
	}

	// Ordered by the lock alone: the second caller appends to order
	// only after it has the lock, and the first appends before it
	// releases. If the lock did nothing, "second" could land first.
	var mu sync.Mutex
	var order []string

	taken := make(chan error, 1)
	go func() {
		second := NewDirLock(dir)
		_, err := second.Lock(10 * time.Second)
		if err == nil {
			mu.Lock()
			order = append(order, "second")
			mu.Unlock()
			second.Unlock()
		}
		taken <- err
	}()

	// The goroutine above may not have reached its Lock call yet, which
	// is exactly why nothing here waits for a duration: this side
	// records its own step and releases, and the assertion is on the
	// order the two steps could possibly have happened in.
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

// TestAWaitForAHeldLockExpiresRatherThanBlockingForEver pins the one
// promise the audit log depends on when everything else has gone wrong:
// Lock comes back.
func TestAWaitForAHeldLockExpiresRatherThanBlockingForEver(t *testing.T) {
	dir := t.TempDir()

	held := NewDirLock(dir)
	if _, err := held.Lock(5 * time.Second); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer held.Unlock()

	expired := make(chan error, 1)
	go func() {
		other := NewDirLock(dir)
		_, err := other.Lock(50 * time.Millisecond)
		expired <- err
	}()

	select {
	case err := <-expired:
		if !errors.Is(err, ErrDirLockTimeout) {
			t.Fatalf("Lock on a held lock returned %v, want ErrDirLockTimeout", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Lock never returned at all")
	}
}

// TestTwoDirectoriesAreTwoLocks: one user signing must not be able to
// stop another, and one directory's lock must not be another's. SPEC
// §14.1 makes several users on one machine a supported configuration.
func TestTwoDirectoriesAreTwoLocks(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	first := NewDirLock(a)
	if _, err := first.Lock(5 * time.Second); err != nil {
		t.Fatalf("Lock(a): %v", err)
	}
	defer first.Unlock()

	second := NewDirLock(b)
	if _, err := second.Lock(2 * time.Second); err != nil {
		t.Fatalf("Lock(b) while a was held: %v", err)
	}
	second.Unlock()
}
