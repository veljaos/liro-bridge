package tsl

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixedFetcher(data []byte, err error) Fetcher {
	return func(_ context.Context, _ string) ([]byte, error) {
		return data, err
	}
}

func TestNewFileStoreFallsBackToEmbeddedSeedWithNoCache(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(filepath.Join(dir, "tsl-cache.xml"), DefaultURL, fixedFetcher(nil, errors.New("unused")))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	list, prov, err := s.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if prov.Source != SourceEmbedded {
		t.Fatalf("Source = %v, want SourceEmbedded", prov.Source)
	}
	if list.Sequence != 36 {
		t.Fatalf("Sequence = %d, want 36", list.Sequence)
	}
}

func TestRefreshAcceptsNewerSequenceAndPersistsToCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "tsl-cache.xml")
	s, err := NewFileStore(cachePath, DefaultURL, fixedFetcher(seedXML, nil))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	_, prov, _ := s.Current(context.Background())
	if prov.Source != SourceNetwork {
		t.Fatalf("Source = %v, want SourceNetwork", prov.Source)
	}
	if prov.Sequence != 36 {
		t.Fatalf("Sequence = %d, want 36", prov.Sequence)
	}

	cached, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("cache file was not written: %v", err)
	}
	if !bytes.Equal(cached, seedXML) {
		t.Fatal("cache file content does not match the fetched list")
	}
}

// TestRefreshRejectsRollback is the test that makes the anti-rollback
// rule in F1 §4.8 fail if it is ever silently disabled — the deliberately
// violating input here is a well-formed, correctly *signed* list (so a
// naive implementation that only checked the signature would accept it)
// whose sequence number is nonetheless lower than what is already
// current.
func TestRefreshRejectsRollback(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(filepath.Join(dir, "tsl-cache.xml"), DefaultURL, fixedFetcher(seedXML, nil))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	// Seed the store's "current" above the real list's sequence (36) so
	// the real (validly signed) list now reads as a rollback attempt.
	s.current.Sequence = 37

	err = s.Refresh(context.Background())
	if !errors.Is(err, ErrRollback) {
		t.Fatalf("Refresh(lower sequence) = %v, want ErrRollback", err)
	}

	list, prov, _ := s.Current(context.Background())
	if list.Sequence != 37 {
		t.Fatalf("current list was replaced despite the rollback rejection: Sequence = %d", list.Sequence)
	}
	if prov.Source == SourceNetwork {
		t.Fatal("provenance was updated despite the rollback rejection")
	}
}

func TestRefreshRejectsUnverifiableList(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(filepath.Join(dir, "tsl-cache.xml"), DefaultURL, fixedFetcher([]byte("<garbage/>"), nil))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	if err := s.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh(unsigned garbage) = nil, want an error")
	}
	_, prov, _ := s.Current(context.Background())
	if prov.Source != SourceEmbedded {
		t.Fatalf("Source = %v, want the embedded seed to remain current", prov.Source)
	}
}

// TestNetworkFailureLeavesCurrentListIntact is the direct proof of F1
// §4.8's "network failure is never fatal": Current must keep returning
// the same list and report the same provenance after a failed Refresh.
func TestNetworkFailureLeavesCurrentListIntact(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(filepath.Join(dir, "tsl-cache.xml"), DefaultURL, fixedFetcher(nil, errors.New("connection refused")))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	before, prevProv, _ := s.Current(context.Background())

	if err := s.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh(network failure) = nil, want an error")
	}

	after, prov, err := s.Current(context.Background())
	if err != nil {
		t.Fatalf("Current after failed refresh returned an error: %v (F1 §4.8: staleness is reported, not treated as failure)", err)
	}
	if after != before {
		t.Fatal("current list pointer changed despite a failed refresh")
	}
	if prov != prevProv {
		t.Fatalf("provenance changed despite a failed refresh: %+v -> %+v", prevProv, prov)
	}
}

func TestNewFileStoreUsesValidCacheOverEmbeddedSeed(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "tsl-cache.xml")
	if err := os.WriteFile(cachePath, seedXML, 0o644); err != nil {
		t.Fatalf("writing cache fixture: %v", err)
	}

	s, err := NewFileStore(cachePath, DefaultURL, fixedFetcher(nil, errors.New("unused")))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	_, prov, _ := s.Current(context.Background())
	if prov.Source != SourceCache {
		t.Fatalf("Source = %v, want SourceCache", prov.Source)
	}
}

// waitForRefresh is how long a single refresh is allowed to take before
// this test calls it a hang. It is deliberately enormous relative to the
// work involved (one verify-and-parse of the embedded list, measured at
// ~28 ms on the development machine): the test is checking that Run
// refreshes immediately and then keeps ticking, which is a question
// about ordering, not about speed. A tight budget here does not make the
// test stricter, only flakier — see D-112.
const waitForRefresh = 30 * time.Second

func TestRunRefreshesImmediatelyThenOnInterval(t *testing.T) {
	dir := t.TempDir()

	// Each fetch announces itself on a buffered channel rather than
	// incrementing a counter the test reads after a fixed wall-clock
	// budget. Run is on its own goroutine here, so a plain counter would
	// also be a data race; the channel answers both problems at once.
	fetched := make(chan struct{}, 8)
	fetch := func(_ context.Context, _ string) ([]byte, error) {
		select {
		case fetched <- struct{}{}:
		default: // never block Run once the test has seen enough
		}
		return seedXML, nil
	}
	s, err := NewFileStore(filepath.Join(dir, "tsl-cache.xml"), DefaultURL, fetch)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		s.Run(ctx, time.Millisecond)
	}()

	// Two refreshes: the immediate one Run does before its first tick,
	// then at least one the ticker drives. Waiting for the events proves
	// the same thing the old fixed budget was trying to, without
	// depending on how fast the machine happens to be.
	for i := 1; i <= 2; i++ {
		select {
		case <-fetched:
		case <-time.After(waitForRefresh):
			t.Fatalf("refresh %d of 2 never happened within %v", i, waitForRefresh)
		}
	}

	// Cancelling must actually stop Run. The old test never checked
	// this — it let a context deadline expire and assumed the rest.
	cancel()
	select {
	case <-returned:
	case <-time.After(waitForRefresh):
		t.Fatalf("Run did not return within %v of its context being cancelled", waitForRefresh)
	}
}
