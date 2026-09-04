package audit

// Store persists the audit chain to disk (F5 §8.4):
// %LOCALAPPDATA%\Liro\audit\, one file per month, rotated by size at
// 5 MB, never uploaded. The chain itself spans every file in the
// directory — a file boundary is purely a storage detail, not a break
// in what Verify checks, since tampering with an earlier month's file
// must still be detectable.
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MaxFileSize is F5 §8.4's rotation threshold.
const MaxFileSize = 5 * 1024 * 1024

// jsonEntry is Entry's on-disk JSON shape. Hash and PrevHash are
// stored hex-encoded — JSON has no native byte-string type, and hex
// keeps the file human-inspectable, which matters for an artefact
// SPEC's export requirement expects a person to be able to look at.
type jsonEntry struct {
	Sequence      uint64    `json:"sequence"`
	Timestamp     time.Time `json:"timestamp"`
	Thumbprint    string    `json:"thumbprint"`
	Application   string    `json:"application"`
	DocumentCount int       `json:"documentCount"`
	Outcome       Outcome   `json:"outcome"`
	FailureCode   string    `json:"failureCode,omitempty"`
	IsTestKey     bool      `json:"isTestKey"`
	AchievedLevel string    `json:"achievedLevel,omitempty"`
	PrevHash      string    `json:"prevHash"`
	Hash          string    `json:"hash"`
}

func toJSONEntry(e Entry) jsonEntry {
	return jsonEntry{
		Sequence:      e.Sequence,
		Timestamp:     e.Timestamp.Truncate(time.Second).UTC(),
		Thumbprint:    e.Thumbprint,
		Application:   e.Application,
		DocumentCount: e.DocumentCount,
		Outcome:       e.Outcome,
		FailureCode:   string(e.FailureCode),
		IsTestKey:     e.IsTestKey,
		AchievedLevel: e.AchievedLevel,
		PrevHash:      hexEncode(e.PrevHash),
		Hash:          hexEncode(e.Hash),
	}
}

func fromJSONEntry(j jsonEntry) (Entry, error) {
	prevHash, err := hexDecode(j.PrevHash)
	if err != nil {
		return Entry{}, fmt.Errorf("audit: decoding prevHash: %w", err)
	}
	hash, err := hexDecode(j.Hash)
	if err != nil {
		return Entry{}, fmt.Errorf("audit: decoding hash: %w", err)
	}
	return Entry{
		Sequence:      j.Sequence,
		Timestamp:     j.Timestamp,
		Thumbprint:    j.Thumbprint,
		Application:   j.Application,
		DocumentCount: j.DocumentCount,
		Outcome:       j.Outcome,
		FailureCode:   errCode(j.FailureCode),
		IsTestKey:     j.IsTestKey,
		AchievedLevel: j.AchievedLevel,
		PrevHash:      prevHash,
		Hash:          hash,
	}, nil
}

// Store guards its own directory with a mutex: Append must read the
// last entry and write the new one as one atomic-from-this-process
// operation, or two concurrent batches finishing at once could both
// compute the same PrevHash and silently fork the chain.
type Store struct {
	dir string
	mu  sync.Mutex
}

// NewStore returns a Store rooted at dir, creating it if necessary.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// listFiles returns every *.jsonl file in the store's directory, in
// chain order. The zero-padded rotation suffix (fileName) makes plain
// lexicographic sort the correct order both within a month and across
// months/years — see fileName's own doc comment.
func (s *Store) listFiles() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// fileName returns the path for month t's file at the given rotation
// (starting at 1). The rotation is zero-padded to 3 digits so that
// "2026-09-002.jsonl" sorts before "2026-09-010.jsonl" — an unpadded
// scheme would sort "10" before "2".
func (s *Store) fileName(t time.Time, rotation int) string {
	return filepath.Join(s.dir, fmt.Sprintf("%04d-%02d-%03d.jsonl", t.Year(), t.Month(), rotation))
}

// currentFile returns the file Append should write to for entries
// timestamped at t: the highest-rotation file for t's month, or a new
// rotation-1 file if none exists yet, or the next rotation if the
// current one has reached MaxFileSize.
func (s *Store) currentFile(t time.Time) (string, error) {
	prefix := fmt.Sprintf("%04d-%02d-", t.Year(), t.Month())
	files, err := s.listFiles()
	if err != nil {
		return "", err
	}

	rotation := 0
	var latest string
	for _, f := range files {
		if strings.HasPrefix(filepath.Base(f), prefix) {
			latest = f
			rotation++
		}
	}
	if latest == "" {
		return s.fileName(t, 1), nil
	}
	info, err := os.Stat(latest)
	if err != nil {
		return "", err
	}
	if info.Size() >= MaxFileSize {
		return s.fileName(t, rotation+1), nil
	}
	return latest, nil
}

// lastEntry reads the last line of the last file in chain order,
// returning (Entry{}, false, nil) if the store is empty.
func (s *Store) lastEntry() (Entry, bool, error) {
	files, err := s.listFiles()
	if err != nil {
		return Entry{}, false, err
	}
	for i := len(files) - 1; i >= 0; i-- {
		entries, err := readEntries(files[i])
		if err != nil {
			return Entry{}, false, err
		}
		if len(entries) > 0 {
			return entries[len(entries)-1], true, nil
		}
	}
	return Entry{}, false, nil
}

func readEntries(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var j jsonEntry
		if err := json.Unmarshal(line, &j); err != nil {
			return nil, fmt.Errorf("audit: parsing %s: %w", path, err)
		}
		e, err := fromJSONEntry(j)
		if err != nil {
			return nil, fmt.Errorf("audit: parsing %s: %w", path, err)
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Append computes next's Sequence/PrevHash/Hash against the last entry
// currently in the store and writes it (F5 §8.1/§8.2). It never
// modifies an existing file's earlier lines — one line is opened,
// written and closed per call.
func (s *Store) Append(next Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	last, ok, err := s.lastEntry()
	if err != nil {
		return Entry{}, err
	}
	var prev *Entry
	if ok {
		prev = &last
	}
	completed := AppendEntry(prev, next)

	path, err := s.currentFile(completed.Timestamp)
	if err != nil {
		return Entry{}, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Entry{}, err
	}
	defer func() { _ = f.Close() }()

	b, err := json.Marshal(toJSONEntry(completed))
	if err != nil {
		return Entry{}, err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return Entry{}, err
	}
	return completed, nil
}

// All reads every entry in the store, in chain order.
func (s *Store) All() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.listFiles()
	if err != nil {
		return nil, err
	}
	var all []Entry
	for _, f := range files {
		entries, err := readEntries(f)
		if err != nil {
			return nil, err
		}
		all = append(all, entries...)
	}
	return all, nil
}

// Verify reads the whole store and reports the first break, if any
// (F5 §8.2).
func (s *Store) Verify() (VerifyResult, error) {
	entries, err := s.All()
	if err != nil {
		return VerifyResult{}, err
	}
	return Verify(entries), nil
}
