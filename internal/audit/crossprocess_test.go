package audit

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// The two-process measurements from D-223, made deterministic, that hold on
// every platform: one process holding the log stops another from extending
// it, and a holder that is killed does not strand the log. They were
// Windows-only until F12 found the Linux half never crossed a process
// boundary — internal/platform's flock tests contend two descriptors inside
// one process (open item B18). What stays Windows-only is in
// crossprocess_windows_test.go: the abandoned-mutex signal, which flock does
// not have.
//
// D-223's own collision — two processes released "at the same instant" —
// cannot be a test: two schedulers cannot be made to enter one critical
// section on one tick, and arranging it would measure the machine rather
// than this program (D-201). What these observe is the property the
// collision was evidence about, by having the second process hold the lock
// until it is told to let go, or is killed.

// lockHolderEnv names the directory the helper process should hold the
// lock over. Its presence is also what tells the helper it is the
// helper.
const lockHolderEnv = "LIRO_AUDIT_LOCK_HELPER_DIR"

// TestAuditLockHelperProcess is not a test of anything. It is the body
// of the second process the tests below start: it takes the audit
// directory's lock, says so on stdout, and holds it until its stdin is
// closed — or until it is killed, which is the other thing the tests
// need it for.
//
// Re-executing the test binary is the standard way to get a second
// process out of a Go test without a second program to build and keep
// in step with this one.
func TestAuditLockHelperProcess(t *testing.T) {
	dir := os.Getenv(lockHolderEnv)
	if dir == "" {
		t.Skip("not the helper process")
	}
	lock := platform.NewDirLock(dir)
	if _, err := lock.Lock(30 * time.Second); err != nil {
		fmt.Println("helper: could not take the lock:", err)
		os.Exit(3)
	}
	fmt.Println("helper: holding the lock")
	// Wait for the parent to close our stdin, which is the release
	// signal — a real end-of-file rather than a sleep, so nothing here
	// depends on how fast the machine is.
	_, _ = io.Copy(io.Discard, os.Stdin)
	lock.Unlock()
	fmt.Println("helper: released the lock")
}

// lockHolder starts the helper process and returns once it has actually
// taken the lock. Closing the returned WriteCloser releases it;
// cmd.Process.Kill() ends the process while it still holds it, which is
// how the abandoned path is reached.
func lockHolder(t *testing.T, dir string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestAuditLockHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), lockHolderEnv+"="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the helper process: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "holding the lock") {
			// Drain the rest in the background so the pipe never fills.
			go func() { _, _ = io.Copy(io.Discard, stdout) }()
			return cmd, stdin
		}
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	t.Fatal("the helper process never reported that it had the lock")
	return nil, nil
}

// approved is one ordinary batch outcome, so the tests below say what
// they are about rather than restating an Entry each time.
func approved(app string) Entry {
	return Entry{
		Timestamp:     time.Now(),
		Thumbprint:    "AABB",
		Application:   app,
		DocumentCount: 1,
		Outcome:       OutcomeApproved,
	}
}

// TestASecondProcessCannotAppendWhileThisOneHoldsTheLog is the
// cross-process half of the property: the lock is one lock, shared by
// every process that resolves the same directory, not a mutex inside
// one of them.
//
// It is what makes the in-process reproduction test mean something. A
// sync.Mutex — or a lock scoped to this process by any other means —
// passes TestTwoStoresOnOneDirectoryDoNotForkTheChain and fails here.
func TestASecondProcessCannotAppendWhileThisOneHoldsTheLog(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s.Append(approved("first")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	holder, release := lockHolder(t, dir)
	t.Cleanup(func() {
		_ = release.Close()
		_ = holder.Wait()
	})

	// A short wait, because the point is the outcome of the expiry and
	// not how long it takes to arrive; AppendLockTimeout is ten seconds
	// in the product for the reason its own comment gives.
	s.lockTimeout = 150 * time.Millisecond

	entry, err := s.Append(approved("blocked"))
	if err != nil {
		t.Fatalf("Append while the log was held: %v", err)
	}
	if entry.Discontinuity == nil {
		t.Fatal("the entry went into the chain the other process was holding, with no record that the log could not be locked")
	}
	if entry.Discontinuity.Reason != BreakUnguarded {
		t.Errorf("break reason = %q, want %q", entry.Discontinuity.Reason, BreakUnguarded)
	}
	if entry.Discontinuity.PreviousChain != 1 {
		t.Errorf("previous chain = %d, want 1", entry.Discontinuity.PreviousChain)
	}
	if entry.Sequence != 0 || len(entry.PrevHash) != 0 {
		t.Errorf("the entry that opens a new chain has sequence %d and a %d-byte PrevHash; want 0 and none",
			entry.Sequence, len(entry.PrevHash))
	}

	report, err := s.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !report.OK {
		t.Errorf("the log does not verify after the expiry: brokenAt=%d", report.BrokenAt)
	}
	if len(report.Chains) != 2 {
		t.Fatalf("chains = %d, want 2 — the entry belongs in a chain of its own", len(report.Chains))
	}
	if n := report.Chains[0].EntryCount; n != 1 {
		t.Errorf("the chain the other process was holding now has %d entries; it must be untouched", n)
	}
	if len(report.Discontinuities()) != 1 {
		t.Errorf("discontinuities = %d, want 1", len(report.Discontinuities()))
	}
}

// TestALogReleasedByADeadWriterIsPickedUpAndContinued: the process holding
// the log is killed outright. The next append must not wait for a lock
// nobody will ever release, and — the tail being sound — must simply carry
// on the same chain.
//
// On Windows this is the whole reason a named mutex was chosen over a
// byte-range file lock: the next waiter is granted it as abandoned. On
// Linux flock is released with the dying process's file description, and
// says nothing about it; the outcome asked for here is the same, and this
// test is what shows it on both (open item B18).
func TestALogReleasedByADeadWriterIsPickedUpAndContinued(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	// On Windows this append is also what opens this process's own handle
	// to the named mutex, which is what keeps the object alive when the
	// holder dies — without a surviving handle the object would simply
	// cease to exist and the next waiter would be told nothing.
	first, err := s.Append(approved("before"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	holder, release := lockHolder(t, dir)
	// Killed, not asked. Closing its stdin first would let it release
	// the lock the ordinary way, which is the opposite of what this
	// test is about — measured: with the close before the kill, the
	// helper won the race and the acquisition below was an ordinary
	// one.
	t.Cleanup(func() { _ = release.Close() })
	if err := holder.Process.Kill(); err != nil {
		t.Fatalf("killing the helper process: %v", err)
	}
	_ = holder.Wait()

	s.lockTimeout = 5 * time.Second
	entry, err := s.Append(approved("after"))
	if err != nil {
		t.Fatalf("Append after the holder died: %v", err)
	}
	if entry.Discontinuity != nil {
		t.Errorf("a new chain was started over a log whose last entry is perfectly sound: %+v", entry.Discontinuity)
	}
	if entry.Sequence != 1 {
		t.Errorf("sequence = %d, want 1", entry.Sequence)
	}
	if string(entry.PrevHash) != string(first.Hash) {
		t.Error("the entry does not chain from the one before it")
	}

	report, err := s.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !report.OK || len(report.Chains) != 1 || report.EntryCount != 2 {
		t.Errorf("after a dead writer: ok=%v chains=%d entries=%d", report.OK, len(report.Chains), report.EntryCount)
	}
}
