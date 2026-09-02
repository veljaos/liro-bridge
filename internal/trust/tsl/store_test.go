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

func TestRunRefreshesImmediatelyThenOnInterval(t *testing.T) {
	dir := t.TempDir()
	var calls int
	fetch := func(_ context.Context, _ string) ([]byte, error) {
		calls++
		return seedXML, nil
	}
	s, err := NewFileStore(filepath.Join(dir, "tsl-cache.xml"), DefaultURL, fetch)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	s.Run(ctx, 40*time.Millisecond)

	if calls < 2 {
		t.Fatalf("Refresh called %d times in 250ms with a 40ms interval, want at least 2 (immediate + at least one tick)", calls)
	}
}
