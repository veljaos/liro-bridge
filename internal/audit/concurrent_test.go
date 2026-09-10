package audit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// D-223's reproduction, committed.
//
// It was four lines in a throwaway harness: two audit.NewStore calls on
// one directory, an Append on each from two goroutines released
// together, then Verify. It is here because it fails against the tree
// this phase started from — measured, not assumed:
//
//	entries=2 chains=1 storeOK=false brokenAt=1
//	  seq=0 app=store-1  prev=(none) hash=3b0995ab
//	  seq=0 app=store-0  prev=(none) hash=8000e566
//
// Two sequence-0 entries, both with an empty PrevHash, and a log that
// reports itself tampered with at line 2 for the rest of its life.
//
// Two Stores in one process rather than two processes on purpose. It is
// the shape that isolates the defect: the guard that was there was a
// sync.Mutex on the Store *value*, so two Stores over one directory
// defeat it exactly as two processes do, with no scheduling to arrange
// and nothing to wait for. What it does not prove is that the lock
// reaches across processes — that is
// TestASecondProcessCannotAppendWhileThisOneHoldsTheLog's job, in the
// Windows file beside this one, and a sync.Mutex would sail through
// this test while failing that one.
func TestTwoStoresOnOneDirectoryDoNotForkTheChain(t *testing.T) {
	dir := t.TempDir()

	stores := make([]*Store, 2)
	for i := range stores {
		s, err := NewStore(dir)
		if err != nil {
			t.Fatalf("NewStore: %v", err)
		}
		stores[i] = s
	}

	start := make(chan struct{})
	failures := make([]error, len(stores))
	var wg sync.WaitGroup
	for i, s := range stores {
		wg.Add(1)
		go func(i int, s *Store) {
			defer wg.Done()
			<-start
			_, failures[i] = s.Append(Entry{
				Timestamp:     time.Now(),
				Thumbprint:    "AABB",
				Application:   fmt.Sprintf("store-%d", i),
				DocumentCount: 1,
				Outcome:       OutcomeApproved,
			})
		}(i, s)
	}
	close(start)
	wg.Wait()

	for i, err := range failures {
		if err != nil {
			t.Fatalf("store-%d Append: %v", i, err)
		}
	}

	entries, err := stores[0].All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Sequence != 0 || entries[1].Sequence != 1 {
		t.Errorf("sequences = %d, %d; want 0, 1 — two entries sharing a sequence number is the fork itself",
			entries[0].Sequence, entries[1].Sequence)
	}

	report, err := stores[0].Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !report.OK {
		t.Errorf("the log verifies as tampered with at entry %d after two concurrent appends", report.BrokenAt)
	}
	if len(report.Chains) != 1 {
		t.Errorf("chains = %d, want 1: neither append had any reason to start a new chain", len(report.Chains))
	}
}

// TestManyAppendsFromManyStoresStayOneUnbrokenChain is the same
// property at F6's own scale. Explorer starts one process per selected
// file (D-119), so twenty of them arriving at once is the ordinary case
// rather than the exotic one, and a lock that holds for two callers and
// not for twenty would pass the test above and fail in a person's hands.
func TestManyAppendsFromManyStoresStayOneUnbrokenChain(t *testing.T) {
	const writers = 20

	dir := t.TempDir()
	start := make(chan struct{})
	failures := make([]error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		s, err := NewStore(dir)
		if err != nil {
			t.Fatalf("NewStore: %v", err)
		}
		wg.Add(1)
		go func(i int, s *Store) {
			defer wg.Done()
			<-start
			_, failures[i] = s.Append(Entry{
				Timestamp:     time.Now(),
				Thumbprint:    "AABB",
				Application:   fmt.Sprintf("store-%d", i),
				DocumentCount: 1,
				Outcome:       OutcomeApproved,
			})
		}(i, s)
	}
	close(start)
	wg.Wait()

	for i, err := range failures {
		if err != nil {
			t.Fatalf("store-%d Append: %v", i, err)
		}
	}

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	entries, err := s.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(entries) != writers {
		t.Fatalf("entries = %d, want %d", len(entries), writers)
	}
	for i, e := range entries {
		if e.Sequence != uint64(i) {
			t.Fatalf("entry %d has sequence %d", i, e.Sequence)
		}
	}
	report, err := s.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !report.OK || len(report.Chains) != 1 {
		t.Errorf("after %d concurrent appends: ok=%v chains=%d brokenAt=%d",
			writers, report.OK, len(report.Chains), report.BrokenAt)
	}
}
