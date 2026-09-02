package tsl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultURL is the Ministry's own publication endpoint for the Trusted
// List. F1 §4.8 requires this to be configurable; this constant is only
// the default a caller may use.
const DefaultURL = "https://www.mit.gov.rs/TrustedList/TSL-RS.xml"

// RefreshInterval is how often Run refreshes the list once started, per
// F1 §4.8 ("every 24 hours").
const RefreshInterval = 24 * time.Hour

// SourceKind identifies where the currently active List came from.
type SourceKind int

const (
	SourceEmbedded SourceKind = iota
	SourceCache
	SourceNetwork
)

func (k SourceKind) String() string {
	switch k {
	case SourceEmbedded:
		return "embedded"
	case SourceCache:
		return "cache"
	case SourceNetwork:
		return "network"
	default:
		return "unknown"
	}
}

// Provenance describes where the list currently in use came from and how
// old it is.
type Provenance struct {
	Source    SourceKind
	IssuedAt  time.Time
	FetchedAt time.Time
	Sequence  int
}

// Store holds the current Trusted List and keeps it fresh.
type Store interface {
	// Current returns the list in use, its issue date, and where it came
	// from. It never returns an error for a stale list — staleness is
	// reported, not treated as failure.
	Current(ctx context.Context) (*List, Provenance, error)

	// Refresh attempts to fetch a newer list. Failure is logged and
	// returned, but leaves the existing list in place.
	Refresh(ctx context.Context) error
}

// ErrRollback means a fetched list's sequence number is lower than the
// one currently in use — a rollback attempt (F1 §4.8) — and was
// rejected.
var ErrRollback = errors.New("trusted list sequence number went backwards")

// Fetcher retrieves the raw Trusted List XML from a URL. The default,
// HTTPFetcher, uses net/http; tests supply a fake so Store's rollback,
// caching and failure-handling logic never depends on the network.
type Fetcher func(ctx context.Context, url string) ([]byte, error)

// HTTPFetcher is the production Fetcher.
func HTTPFetcher(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching trusted list: unexpected status %s", resp.Status)
	}
	// The published list is under 1 MB; refuse anything wildly larger
	// rather than reading an unbounded body from a network endpoint.
	const maxSize = 32 << 20
	return io.ReadAll(io.LimitReader(resp.Body, maxSize))
}

// FileStore is the Store implementation used by the agent: it starts
// from an embedded seed, persists fetched lists to a local cache file,
// and never fails closed just because the network is unavailable
// (F1 §4.8; SPEC §11.1).
type FileStore struct {
	url       string
	cachePath string
	fetch     Fetcher

	// mu guards current and provenance, which Refresh replaces and
	// Current reads. A FileStore may be shared by a background refresh
	// goroutine (Run) and a caller reading Current concurrently.
	mu         sync.Mutex
	current    *List
	provenance Provenance
}

// NewFileStore constructs a FileStore. cachePath is where the last
// successfully fetched list is persisted; the caller decides where that
// is (internal/platform, not this package — SPEC §4.2 rule 3 keeps
// internal/trust free of any internal/ dependency). fetch is normally
// HTTPFetcher; tests pass a fake.
//
// Construction loads the best list already available: the on-disk cache
// if it is present, verifiable and at least as new as the embedded seed,
// otherwise the embedded seed itself. This never touches the network.
func NewFileStore(cachePath, url string, fetch Fetcher) (*FileStore, error) {
	s := &FileStore{
		url:       url,
		cachePath: cachePath,
		fetch:     fetch,
	}

	if raw, mtime, err := readCache(cachePath); err == nil {
		list, perr := verifyAndParse(raw)
		if perr == nil {
			s.current = list
			s.provenance = Provenance{Source: SourceCache, IssuedAt: list.IssuedAt, FetchedAt: mtime, Sequence: list.Sequence}
			return s, nil
		}
		slog.Warn("tsl: cached list failed verification, falling back to embedded seed", "error", perr)
	}

	seedList, err := verifyAndParse(seedXML)
	if err != nil {
		// The embedded seed is checked at build time (TestVerifyBundledSeedSucceeds);
		// reaching this in production would mean the binary itself is broken.
		return nil, fmt.Errorf("tsl: embedded seed failed verification: %w", err)
	}
	s.current = seedList
	s.provenance = Provenance{Source: SourceEmbedded, IssuedAt: seedList.IssuedAt, Sequence: seedList.Sequence}
	return s, nil
}

func verifyAndParse(raw []byte) (*List, error) {
	if err := Verify(raw); err != nil {
		return nil, err
	}
	return Parse(raw)
}

func readCache(path string) ([]byte, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	return b, info.ModTime(), nil
}

// Current implements Store.
func (s *FileStore) Current(_ context.Context) (*List, Provenance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current, s.provenance, nil
}

// Refresh implements Store. A fetch or verification failure, or a
// rollback attempt, is logged and returned, but never replaces the list
// already in use (F1 §4.8: "network failure is never fatal").
func (s *FileStore) Refresh(ctx context.Context) error {
	raw, err := s.fetch(ctx, s.url)
	if err != nil {
		slog.Warn("tsl: refresh failed, keeping current list", "error", err)
		return fmt.Errorf("fetching trusted list: %w", err)
	}

	list, err := verifyAndParse(raw)
	if err != nil {
		slog.Warn("tsl: fetched list failed verification, keeping current list", "error", err)
		return err
	}

	s.mu.Lock()
	currentSeq := s.current.Sequence
	s.mu.Unlock()

	if list.Sequence < currentSeq {
		slog.Warn("tsl: fetched list has a lower sequence number than the current one, rejecting as a rollback",
			"fetchedSequence", list.Sequence, "currentSequence", currentSeq)
		return fmt.Errorf("%w: fetched %d, have %d", ErrRollback, list.Sequence, currentSeq)
	}

	if err := writeCacheAtomically(s.cachePath, raw); err != nil {
		// The fetch and verification succeeded; a cache-write failure
		// (e.g. read-only disk) should not discard a perfectly good,
		// freshly verified list from this run's memory.
		slog.Warn("tsl: failed to persist refreshed list to cache", "error", err)
	}

	s.mu.Lock()
	s.current = list
	s.provenance = Provenance{Source: SourceNetwork, IssuedAt: list.IssuedAt, FetchedAt: time.Now(), Sequence: list.Sequence}
	s.mu.Unlock()
	return nil
}

// writeCacheAtomically writes data to path via a temp file plus rename,
// so a crash mid-write never leaves a truncated cache (same pattern as
// internal/config.Save, applied independently here since this package
// may not depend on internal/config).
func writeCacheAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tsl-cache-*.xml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Run refreshes immediately, then again every interval, until ctx is
// done. Callers that want the "at startup and every 24 hours" behaviour
// of F1 §4.8 run this in a goroutine with interval set to
// RefreshInterval; tests use a short interval instead of waiting a day.
func (s *FileStore) Run(ctx context.Context, interval time.Duration) {
	if err := s.Refresh(ctx); err != nil {
		slog.Warn("tsl: startup refresh failed", "error", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Refresh(ctx); err != nil {
				slog.Warn("tsl: scheduled refresh failed", "error", err)
			}
		}
	}
}
