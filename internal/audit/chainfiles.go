package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A store holds one or more chains. Chain 1 is the original — every
// audit directory written before chains existed is chain 1, and stays
// chain 1, under its own unchanged file names ("2026-09-001.jsonl").
// A later chain's files carry their number: "2026-09-001.c2.jsonl".
//
// Naming rather than a subdirectory, and a suffix rather than a prefix,
// for one reason each: an existing directory must keep working with the
// file names it already has, and a broken chain's own file must never be
// moved, renamed or touched at all. Grouping is done by reading the
// names, not by moving the files.

// chainSuffixPattern matches the ".cN" a chain after the first carries
// before its .jsonl extension.
var chainSuffixPattern = regexp.MustCompile(`\.c([0-9]+)$`)

// chainNumberOf reads the chain number out of a file's base name. Any
// *.jsonl name without a ".cN" part is chain 1, which is what keeps an
// audit directory written before chains existed readable exactly as it
// was.
func chainNumberOf(base string) int {
	name := strings.TrimSuffix(base, ".jsonl")
	m := chainSuffixPattern.FindStringSubmatch(name)
	if m == nil {
		return 1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// chainFileName returns the path for chain's file for month t at the
// given rotation (starting at 1). Chain 1 keeps the original shape.
func (s *Store) chainFileName(chain int, year int, month int, rotation int) string {
	base := fmt.Sprintf("%04d-%02d-%03d", year, month, rotation)
	if chain > 1 {
		base += fmt.Sprintf(".c%d", chain)
	}
	return filepath.Join(s.dir, base+".jsonl")
}

// Chain is one hash chain in the store: which files it spans, what is in
// them, and — when it exists — the record its own first entry carries
// saying why the chain was started at all.
type Chain struct {
	// Number is 1 for the original chain and rises by one for each
	// chain started after a break.
	Number int

	// Files are the chain's files' base names, in chain order.
	Files []string

	// Entries are every entry that could be read, in chain order.
	Entries []Entry

	// Discontinuity is the chain's first entry's own record of why this
	// chain exists. Nil for chain 1, and for any chain whose first entry
	// predates this mechanism.
	Discontinuity *Discontinuity

	// TruncatedFile, TruncatedAtLine and TruncatedReason describe where
	// reading this chain stopped, when it could not be read to the end.
	// A chain in that state can never be appended to again — the last
	// entry cannot be read, so the next PrevHash cannot be computed —
	// which is exactly what makes the next Append start a new chain.
	TruncatedFile   string
	TruncatedAtLine int
	TruncatedReason BreakReason
}

// Truncated reports whether this chain could not be read to the end.
func (c Chain) Truncated() bool { return c.TruncatedReason != "" }

// readStop says where and why reading a file stopped short.
type readStop struct {
	line   int
	reason BreakReason
	err    error
}

// readEntriesTolerant reads every entry it can from path and reports
// where it had to stop.
//
// Tolerant on purpose. The old behaviour — one bad line, and the whole
// file is an error — is what made a broken log unreadable rather than
// merely unextendable: the audit window showed nothing, the export
// wrote nothing, and the evidence that did survive was unreachable. A
// chain that breaks at line 400 still has 399 entries worth keeping, and
// the break itself is a fact to report, not a reason to report nothing.
func readEntriesTolerant(path string) ([]Entry, *readStop) {
	f, err := os.Open(path)
	if err != nil {
		return nil, &readStop{line: 0, reason: BreakUnreachable, err: err}
	}
	defer func() { _ = f.Close() }()

	var out []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var j jsonEntry
		if err := json.Unmarshal(raw, &j); err != nil {
			return out, &readStop{line: line, reason: BreakUnparseable, err: fmt.Errorf("audit: parsing %s line %d: %w", filepath.Base(path), line, err)}
		}
		e, err := fromJSONEntry(j)
		if err != nil {
			return out, &readStop{line: line, reason: BreakUnparseable, err: fmt.Errorf("audit: parsing %s line %d: %w", filepath.Base(path), line, err)}
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return out, &readStop{line: line + 1, reason: BreakUnreachable, err: fmt.Errorf("audit: reading %s: %w", filepath.Base(path), err)}
	}
	return out, nil
}

// chains groups the store's files into chains, in chain order, reading
// each one as far as it can.
//
// Only a failure to list the directory at all is returned as an error: a
// chain that cannot be read is a chain with a break recorded on it, not
// a store that cannot be used. That distinction is the whole of Task 5 —
// blocking a bookkeeper's afternoon over a log is worse than recording
// that the log moved.
func (s *Store) chains() ([]Chain, error) {
	matches, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	byNumber := map[int][]string{}
	for _, m := range matches {
		base := filepath.Base(m)
		n := chainNumberOf(base)
		byNumber[n] = append(byNumber[n], base)
	}

	numbers := make([]int, 0, len(byNumber))
	for n := range byNumber {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	out := make([]Chain, 0, len(numbers))
	for _, n := range numbers {
		files := byNumber[n]
		// Within one chain the rotation suffix is zero-padded, so plain
		// lexicographic order is chain order both within a month and
		// across months and years (fileName's own reasoning).
		sort.Strings(files)

		c := Chain{Number: n, Files: files}
		for _, base := range files {
			entries, stop := readEntriesTolerant(filepath.Join(s.dir, base))
			c.Entries = append(c.Entries, entries...)
			if stop != nil {
				c.TruncatedFile = base
				c.TruncatedAtLine = stop.line
				c.TruncatedReason = stop.reason
				break
			}
		}
		if len(c.Entries) > 0 {
			c.Discontinuity = c.Entries[0].Discontinuity
		}
		out = append(out, c)
	}
	return out, nil
}

// Chains returns every chain in the store, in order, with each one read
// as far as it can be. It is what verification and export are built on,
// and what a caller asking "how many chains are there and where did they
// break" wants.
func (s *Store) Chains() ([]Chain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chains()
}
