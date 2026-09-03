package audit

import (
	"testing"
	"time"
)

func buildChain(n int) []Entry {
	entries := make([]Entry, 0, n)
	var prev *Entry
	for i := 0; i < n; i++ {
		e := AppendEntry(prev, Entry{
			Timestamp:     time.Unix(1000+int64(i), 0),
			Thumbprint:    "AABBCCDD",
			Application:   "local",
			DocumentCount: 1,
			Outcome:       OutcomeApproved,
		})
		entries = append(entries, e)
		prev = &entries[len(entries)-1]
	}
	return entries
}

func TestAppendEntryChainsSequenceAndPrevHash(t *testing.T) {
	entries := buildChain(3)
	for i, e := range entries {
		if e.Sequence != uint64(i) {
			t.Errorf("entry %d: Sequence = %d, want %d", i, e.Sequence, i)
		}
	}
	if len(entries[0].PrevHash) != 0 {
		t.Fatalf("the first entry's PrevHash must be empty, got % X", entries[0].PrevHash)
	}
	for i := 1; i < len(entries); i++ {
		if string(entries[i].PrevHash) != string(entries[i-1].Hash) {
			t.Fatalf("entry %d's PrevHash does not match entry %d's Hash", i, i-1)
		}
	}
}

func TestVerifyAcceptsAnUntamperedChain(t *testing.T) {
	entries := buildChain(100)
	result := Verify(entries)
	if !result.OK {
		t.Fatalf("Verify rejected an untampered 100-entry chain, BrokenAt=%d", result.BrokenAt)
	}
}

func TestVerifyEmptyChainIsOK(t *testing.T) {
	if result := Verify(nil); !result.OK {
		t.Fatal("Verify rejected an empty chain")
	}
}

// TestVerifyDetectsAlterationAtTheCorrectPosition is F5 §10's explicit
// requirement: "Append 100 entries, verify; alter entry 50, assert
// verification reports break at 50." Altering a field changes that
// entry's own recomputed Hash, so the break is detected AT the altered
// entry itself — before propagating to the entries chained after it.
func TestVerifyDetectsAlterationAtTheCorrectPosition(t *testing.T) {
	entries := buildChain(100)
	entries[50].DocumentCount = 999 // tamper, without recomputing Hash

	result := Verify(entries)
	if result.OK {
		t.Fatal("Verify accepted a chain with an altered entry")
	}
	if result.BrokenAt != 50 {
		t.Fatalf("Verify reported the break at %d, want 50", result.BrokenAt)
	}
}

// TestVerifyDetectsDeletionAtTheCorrectPosition is F5 §10's other
// explicit case: "delete entry 30, assert the same." Deleting an entry
// leaves entry 31 (now at index 30 in the shortened slice) with a
// PrevHash that no longer matches entry 29's Hash — the break surfaces
// at the deleted entry's position, the first place the chain no longer
// links up.
func TestVerifyDetectsDeletionAtTheCorrectPosition(t *testing.T) {
	entries := buildChain(100)
	entries = append(entries[:30], entries[31:]...)

	result := Verify(entries)
	if result.OK {
		t.Fatal("Verify accepted a chain with a deleted entry")
	}
	if result.BrokenAt != 30 {
		t.Fatalf("Verify reported the break at %d, want 30", result.BrokenAt)
	}
}

// TestVerifyDetectsHashTamperingEvenWhenPrevHashIsAlsoForged proves the
// second, independent check in Verify: an attacker who alters an entry
// AND recomputes its own Hash to match (but cannot recompute the next
// entry's PrevHash, since they would have to also forge every entry
// after it back to a real signature) is still caught — at the first
// point where the forged Hash and the next entry's real PrevHash
// disagree.
func TestVerifyDetectsHashTamperingEvenWhenPrevHashIsAlsoForged(t *testing.T) {
	entries := buildChain(10)
	entries[5].DocumentCount = 999
	entries[5].Hash = entries[5].ComputeHash() // attacker recomputes their own hash

	result := Verify(entries)
	if result.OK {
		t.Fatal("Verify accepted a chain where a tampered entry's own hash was recomputed to match")
	}
	if result.BrokenAt != 6 {
		t.Fatalf("Verify reported the break at %d, want 6 (the next entry, whose PrevHash no longer matches)", result.BrokenAt)
	}
}
