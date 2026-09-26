//go:build windows

package audit

import (
	"os"
	"testing"
	"time"
)

// The Windows-only half of D-223's two-process measurements. The halves
// that hold on every platform, and the helper process all of them use, are
// in crossprocess_test.go.
//
// Windows-only because the abandoned-holder signal is: a Windows mutex
// whose owner died reports WAIT_ABANDONED to the next waiter and grants it
// ownership, and that moment is when the chain's soundness is examined.
// flock, the other platforms' lock, releases on process death and says
// nothing about it.

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
