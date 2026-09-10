//go:build windows

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

// These are the two-process measurements from D-223, made deterministic
// — with one part of them that cannot be, and is not pretended to be.
//
// D-223 measured the defect by starting two real processes and
// releasing them "at the same instant". That collision cannot be made
// into a test: two independent schedulers cannot be made to enter the
// same critical section on the same tick, and a test built around
// arranging it would be measuring the machine's timing rather than this
// program's behaviour, which is exactly what D-201 forbids. What *can*
// be observed deterministically is the property the collision was
// evidence about — that one process holding the log stops another from
// reading and extending it — and that is what these tests observe, by
// having the second process hold the lock and never let go until it is
// told to, or until it is killed.
//
// Windows-only because the abandoned-holder signal is: a Windows mutex
// whose owner died reports WAIT_ABANDONED to the next waiter and grants
// it ownership. flock, the other platforms' lock, releases on process
// death and says nothing about it.

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

// TestALogReleasedByADeadWriterIsPickedUpAndContinued is the whole
// reason a named mutex was chosen over a byte-range file lock. The
// process holding the log is killed outright. The next append must not
// wait for a lock nobody will ever release, and — the tail being sound
// — must simply carry on the same chain.
func TestALogReleasedByADeadWriterIsPickedUpAndContinued(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	// This append is also what opens this process's own handle to the
	// named mutex, which is what keeps the object alive when the holder
	// dies — without a surviving handle the object would simply cease to
	// exist and the next waiter would be told nothing.
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

// TestADeadWriterOverAnUnsoundChainStartsANewOneRatherThanExtendingIt
// is the other half of what the abandoned signal is for.
//
// The directory holds exactly what D-223 measured a fork to leave
// behind: two entries with sequence 0, both with an empty PrevHash,
// both individually well-formed and perfectly readable. D-166's
// machinery cannot see it — nothing failed to parse — so the only
// moment anything looks is the moment Windows says the last writer died
// mid-append, which is this one.
func TestADeadWriterOverAnUnsoundChainStartsANewOneRatherThanExtendingIt(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s.Append(approved("first")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	forkTheChain(t, s)

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
		t.Fatalf("Append: %v", err)
	}
	if entry.Discontinuity == nil {
		t.Fatal("the entry was chained onto a forked chain, with no record that anything was wrong")
	}
	if entry.Discontinuity.Reason != BreakUnsound {
		t.Errorf("break reason = %q, want %q", entry.Discontinuity.Reason, BreakUnsound)
	}
	if entry.Discontinuity.PreviousChain != 1 {
		t.Errorf("previous chain = %d, want 1", entry.Discontinuity.PreviousChain)
	}

	chains, err := s.Chains()
	if err != nil {
		t.Fatalf("Chains: %v", err)
	}
	if len(chains) != 2 {
		t.Fatalf("chains = %d, want 2", len(chains))
	}
	if n := len(chains[0].Entries); n != 2 {
		t.Errorf("the damaged chain now has %d entries; it must be left exactly as it was", n)
	}
}

// TestAnUnsoundChainIsExtendedWhenNobodyDied is this file's control.
//
// Same directory, same fork, same append — and no dead writer. The
// entry goes onto the forked chain, because an ordinary acquisition
// says nothing about the data and a check that ran on every append
// would be answering Verify's question rather than Append's (see
// lastEntrySound). Without this, the test above would pass just as
// happily if the soundness check ran unconditionally, and the abandoned
// signal would be proving nothing.
func TestAnUnsoundChainIsExtendedWhenNobodyDied(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s.Append(approved("first")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	forkTheChain(t, s)

	entry, err := s.Append(approved("after"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if entry.Discontinuity != nil {
		t.Errorf("an ordinary append started a new chain: %+v", entry.Discontinuity)
	}
}

// forkTheChain writes a second sequence-0 entry into the store's
// current chain file, by hand — the artefact two unguarded appends used
// to leave behind, reproduced here without needing the race that
// produced it.
func forkTheChain(t *testing.T, s *Store) {
	t.Helper()

	base, err := s.LatestChainFile()
	if err != nil || base == "" {
		t.Fatalf("LatestChainFile: %q %v", base, err)
	}
	line, err := entryLine(AppendEntry(nil, approved("the other process")))
	if err != nil {
		t.Fatalf("entryLine: %v", err)
	}
	f, err := os.OpenFile(s.dir+string(os.PathSeparator)+base, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening the chain file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(line); err != nil {
		t.Fatalf("writing the forked line: %v", err)
	}
}
