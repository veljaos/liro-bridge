package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedChain(t *testing.T, s *Store, n int) {
	t.Helper()
	now := time.Now()
	for i := 0; i < n; i++ {
		if _, err := s.Append(Entry{
			Timestamp:     now.Add(time.Duration(i) * time.Second),
			Thumbprint:    "AA11BB22",
			Application:   ApplicationLocalForTest,
			DocumentCount: i + 1,
			Outcome:       OutcomeApproved,
			AchievedLevel: "B-T",
		}); err != nil {
			t.Fatalf("seeding entry %d: %v", i, err)
		}
	}
}

func onlyFile(t *testing.T, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one log file, found %v", matches)
	}
	return matches[0]
}

// TestATruncatedChainIsLeftAloneAndContinuedInANewFile is Task 5's own
// scenario, end to end: truncate a chain's last line mid-entry, sign
// again, and require that the old file is byte-identical to what it was,
// that a new chain exists, that its first entry names the break, that
// verification reports both chains and the discontinuity, and that the
// export contains both.
func TestATruncatedChainIsLeftAloneAndContinuedInANewFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	seedChain(t, s, 6)

	original := onlyFile(t, dir)
	raw, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	// A power cut mid-write: the last line stops part way through.
	truncated := raw[:len(raw)-40]
	if err := os.WriteFile(original, truncated, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}

	// Sign again.
	entry, err := s.Append(Entry{
		Timestamp:     time.Now(),
		Thumbprint:    "CC33DD44",
		Application:   ApplicationLocalForTest,
		DocumentCount: 2,
		Outcome:       OutcomeApproved,
		AchievedLevel: "B-LT",
	})
	if err != nil {
		t.Fatalf("Append onto a truncated chain: %v — it must start a new chain, not refuse forever", err)
	}

	// The old file is byte-identical to what it was.
	after, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the broken chain's file was modified; it must never be overwritten, truncated or deleted")
	}

	// A new chain exists, beside it.
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected the broken file plus one new chain file, found %v", matches)
	}

	// Its first entry names the break.
	if entry.Discontinuity == nil {
		t.Fatal("the new chain's first entry carries no record of the break")
	}
	d := entry.Discontinuity
	if d.PreviousFile != filepath.Base(original) {
		t.Errorf("PreviousFile = %q, want %q", d.PreviousFile, filepath.Base(original))
	}
	if d.PreviousChain != 1 {
		t.Errorf("PreviousChain = %d, want 1", d.PreviousChain)
	}
	if d.Reason != BreakUnparseable {
		t.Errorf("Reason = %q, want %q", d.Reason, BreakUnparseable)
	}
	if d.Line != 6 {
		t.Errorf("Line = %d, want 6 — the truncated line is the sixth", d.Line)
	}
	if !d.HasLastSequence || d.LastSequence != 4 {
		t.Errorf("LastSequence = %d (has=%v), want 4 — the last entry that could still be read", d.LastSequence, d.HasLastSequence)
	}
	// A new chain starts its own sequence, and has nothing to chain to.
	if entry.Sequence != 0 {
		t.Errorf("the new chain's first entry has Sequence %d, want 0", entry.Sequence)
	}
	if len(entry.PrevHash) != 0 {
		t.Error("the new chain's first entry carries a PrevHash; there is nothing for it to point at")
	}

	// Verification reports both chains and the discontinuity.
	v, err := s.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(v.Chains) != 2 {
		t.Fatalf("Verify reports %d chains, want 2", len(v.Chains))
	}
	if v.Chains[0].Chain != 1 || v.Chains[1].Chain != 2 {
		t.Errorf("chains are numbered %d and %d, want 1 and 2", v.Chains[0].Chain, v.Chains[1].Chain)
	}
	if !v.Chains[0].Result.OK {
		t.Error("the readable part of the broken chain does not verify; the break is at its end, not inside it")
	}
	if v.Chains[0].TruncatedReason != BreakUnparseable || v.Chains[0].TruncatedAtLine != 6 {
		t.Errorf("chain 1 reports truncation %q at line %d, want %q at 6",
			v.Chains[0].TruncatedReason, v.Chains[0].TruncatedAtLine, BreakUnparseable)
	}
	if !v.Chains[1].Result.OK {
		t.Error("the new chain does not verify")
	}
	if v.Chains[1].Discontinuity == nil {
		t.Error("the verification does not name the discontinuity")
	}
	if v.OK {
		t.Error("a store holding a chain that cannot be read to its end reports OK")
	}
	if got := v.Discontinuities(); len(got) != 1 {
		t.Errorf("Discontinuities() = %d, want 1", len(got))
	}

	// Export contains both.
	entriesPath := filepath.Join(t.TempDir(), "export.jsonl")
	reportPath := filepath.Join(t.TempDir(), "export-report.json")
	report, exportErr := s.Export(entriesPath, reportPath)
	if exportErr == nil {
		t.Error("Export reported no problem with a log that cannot be read to its end")
	}
	if report.EntryCount != 6 {
		t.Errorf("Export wrote %d entries, want 6 — five readable from the broken chain plus the new one", report.EntryCount)
	}
	if len(report.Chains) != 2 {
		t.Errorf("the export report names %d chains, want 2", len(report.Chains))
	}
	if len(report.Discontinuities()) != 1 {
		t.Error("the export report does not name the discontinuity")
	}
	exported, err := os.ReadFile(entriesPath)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(strings.TrimSpace(string(exported)), "\n") + 1; lines != 6 {
		t.Errorf("the exported log has %d lines, want 6", lines)
	}
	if !strings.Contains(string(exported), `"discontinuity"`) {
		t.Error("the exported log does not carry the discontinuity record, so it is not self-describing")
	}
	if !strings.Contains(string(exported), `"thumbprint":"CC33DD44"`) {
		t.Error("the exported log does not contain the new chain's entry")
	}
	if !strings.Contains(string(exported), `"thumbprint":"AA11BB22"`) {
		t.Error("the exported log does not contain the broken chain's surviving entries")
	}
}

// TestTheNextSignatureAfterABreakSaysNothing is the "tell the user once"
// half: only the entry that opened the new chain carries the record, so
// nothing has to remember to stop saying it.
func TestTheNextSignatureAfterABreakSaysNothing(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedChain(t, s, 3)
	original := onlyFile(t, dir)
	raw, _ := os.ReadFile(original)
	if err := os.WriteFile(original, raw[:len(raw)-40], 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := s.Append(Entry{Timestamp: time.Now(), Thumbprint: "AA", Application: ApplicationLocalForTest, DocumentCount: 1, Outcome: OutcomeApproved})
	if err != nil {
		t.Fatal(err)
	}
	if first.Discontinuity == nil {
		t.Fatal("the first entry after the break carries no record")
	}
	second, err := s.Append(Entry{Timestamp: time.Now(), Thumbprint: "AA", Application: ApplicationLocalForTest, DocumentCount: 1, Outcome: OutcomeApproved})
	if err != nil {
		t.Fatal(err)
	}
	if second.Discontinuity != nil {
		t.Fatal("the second entry after the break carries the record too; the person would be told again")
	}
	if second.Sequence != 1 || !bytes.Equal(second.PrevHash, first.Hash) {
		t.Fatal("the second entry did not continue the new chain")
	}
}

// TestAChainThatCannotBeReadAtAllIsContinuedToo covers the other reason
// a chain ends: not a corrupt line but a file that cannot be read.
// Refusing to sign over a log is worse than recording that the log
// moved.
func TestAChainThatCannotBeReadAtAllIsContinuedToo(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedChain(t, s, 2)
	original := onlyFile(t, dir)

	release, held := holdFileUnreadable(t, original)
	if !held {
		t.Skip("this platform cannot make a file unreadable to its own owner")
	}
	defer release()

	entry, err := s.Append(Entry{Timestamp: time.Now(), Thumbprint: "AA", Application: ApplicationLocalForTest, DocumentCount: 1, Outcome: OutcomeApproved})
	if err != nil {
		t.Fatalf("Append with the previous chain unreadable: %v", err)
	}
	if entry.Discontinuity == nil {
		t.Fatal("no record was made of a previous chain that could not be read")
	}
	if entry.Discontinuity.Reason != BreakUnreachable {
		t.Errorf("Reason = %q, want %q", entry.Discontinuity.Reason, BreakUnreachable)
	}
	if entry.Discontinuity.HasLastSequence {
		t.Error("a chain that could not be read at all reported a last sequence")
	}
}

// TestAnOrdinaryLogStillHasOneChain is the shape of every log that has
// never been interrupted, and the compatibility guarantee: the files are
// named exactly as they always were, and nothing reports a chain nobody
// started.
func TestAnOrdinaryLogStillHasOneChain(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedChain(t, s, 4)

	base := filepath.Base(onlyFile(t, dir))
	if strings.Contains(base, ".c") {
		t.Fatalf("an uninterrupted log's file is named %q; it must keep the name it always had", base)
	}
	v, err := s.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Chains) != 1 || !v.OK || v.BrokenAt != -1 {
		t.Fatalf("an intact log verified as %+v", v)
	}
	if v.Chains[0].Discontinuity != nil {
		t.Error("an uninterrupted chain reports a discontinuity")
	}
}

// TestTamperingIsStillDetectedWithinAChain: starting a new chain after a
// break must not have made tampering inside a chain invisible, which is
// the whole property SPEC §6.7 buys with the hash chain.
func TestTamperingIsStillDetectedWithinAChain(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedChain(t, s, 5)
	file := onlyFile(t, dir)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), `"documentCount":3`, `"documentCount":999`, 1)
	if tampered == string(raw) {
		t.Fatal("the fixture did not contain the entry this test alters")
	}
	if err := os.WriteFile(file, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}

	v, err := s.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if v.OK {
		t.Fatal("an altered entry was not detected")
	}
	if v.Chains[0].Result.BrokenAt != 2 {
		t.Errorf("BrokenAt = %d, want 2", v.Chains[0].Result.BrokenAt)
	}
	if v.BrokenAt != 2 {
		t.Errorf("the store-wide BrokenAt = %d, want 2", v.BrokenAt)
	}
}

// TestTheDiscontinuityIsPartOfWhatIsHashed: the record of a break is
// content, and content that could be edited without breaking the chain
// it starts would be worth nothing.
func TestTheDiscontinuityIsPartOfWhatIsHashed(t *testing.T) {
	base := Entry{
		Timestamp:     time.Unix(1700000000, 0).UTC(),
		Thumbprint:    "AA",
		Application:   ApplicationLocalForTest,
		DocumentCount: 1,
		Outcome:       OutcomeApproved,
	}
	withRecord := base
	withRecord.Discontinuity = &Discontinuity{
		PreviousChain: 1, PreviousFile: "2026-09-001.jsonl",
		LastSequence: 4, HasLastSequence: true, Line: 6, Reason: BreakUnparseable,
	}
	altered := base
	altered.Discontinuity = &Discontinuity{
		PreviousChain: 1, PreviousFile: "2026-09-001.jsonl",
		LastSequence: 4, HasLastSequence: true, Line: 7, Reason: BreakUnparseable,
	}

	if bytes.Equal(base.CanonicalBytes(), withRecord.CanonicalBytes()) {
		t.Error("an entry with a discontinuity canonicalises to the same bytes as one without")
	}
	if bytes.Equal(withRecord.CanonicalBytes(), altered.CanonicalBytes()) {
		t.Error("changing the line a break was recorded at does not change the entry's hash")
	}
	// An entry written before this field existed canonicalises to
	// exactly what it always did, so a log that predates the change
	// still verifies.
	if !bytes.Equal(base.CanonicalBytes(), Entry{
		Timestamp:     base.Timestamp,
		Thumbprint:    base.Thumbprint,
		Application:   base.Application,
		DocumentCount: base.DocumentCount,
		Outcome:       base.Outcome,
	}.CanonicalBytes()) {
		t.Error("an entry with no discontinuity no longer canonicalises to the bytes it always did")
	}
}

// TestChainNumberOfReadsTheFileName pins the naming, including the rule
// that keeps an existing audit directory working unchanged.
func TestChainNumberOfReadsTheFileName(t *testing.T) {
	cases := map[string]int{
		"2026-09-001.jsonl":     1,
		"2026-09-014.jsonl":     1,
		"2026-09-001.c2.jsonl":  2,
		"2026-09-003.c17.jsonl": 17,
		// Anything this package did not write is chain 1, which is what
		// makes a directory written before chains existed readable
		// exactly as it was.
		"something-else.jsonl": 1,
		"2026-09-001.cx.jsonl": 1,
	}
	for name, want := range cases {
		if got := chainNumberOf(name); got != want {
			t.Errorf("chainNumberOf(%q) = %d, want %d", name, got, want)
		}
	}
}

// TestARotatedNewChainKeepsItsOwnNumbering: rotation and chains are
// independent, and a chain's second file must not be read as a different
// chain.
func TestARotatedNewChainKeepsItsOwnNumbering(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedChain(t, s, 2)
	first := onlyFile(t, dir)
	raw, _ := os.ReadFile(first)
	if err := os.WriteFile(first, raw[:len(raw)-40], 0o600); err != nil {
		t.Fatal(err)
	}
	e0, err := s.Append(Entry{Timestamp: time.Now(), Thumbprint: "AA", Application: ApplicationLocalForTest, DocumentCount: 1, Outcome: OutcomeApproved})
	if err != nil {
		t.Fatal(err)
	}

	// Write chain 2's second rotation by hand, the way
	// TestStoreVerifyDetectsTamperingAcrossFiles does for chain 1.
	now := time.Now()
	path2 := s.chainFileName(2, now.Year(), int(now.Month()), 2)
	e1 := AppendEntry(&e0, Entry{Timestamp: now, Thumbprint: "BB", Application: ApplicationLocalForTest, DocumentCount: 1, Outcome: OutcomeApproved})
	b, err := json.Marshal(toJSONEntry(e1))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path2, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	v, err := s.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Chains) != 2 {
		t.Fatalf("Verify reports %d chains, want 2 — a rotation is not a new chain", len(v.Chains))
	}
	if v.Chains[1].EntryCount != 2 {
		t.Errorf("chain 2 holds %d entries, want 2 across its two files", v.Chains[1].EntryCount)
	}
	if !v.Chains[1].Result.OK {
		t.Error("chain 2 does not verify across its own two files")
	}
}
