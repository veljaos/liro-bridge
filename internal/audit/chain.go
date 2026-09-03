package audit

import "bytes"

// AppendEntry fills in Sequence, PrevHash and Hash for a new entry
// following prev (nil for the very first entry in the whole chain) and
// returns the completed Entry. Every other field must already be set
// by the caller.
func AppendEntry(prev *Entry, next Entry) Entry {
	if prev == nil {
		next.Sequence = 0
		next.PrevHash = nil
	} else {
		next.Sequence = prev.Sequence + 1
		next.PrevHash = prev.Hash
	}
	next.Hash = next.ComputeHash()
	return next
}

// VerifyResult is the outcome of walking a chain.
type VerifyResult struct {
	// OK is true when every entry's Hash matches its own recomputed
	// CanonicalBytes hash and every entry's PrevHash matches the
	// previous entry's Hash.
	OK bool

	// BrokenAt is the index (into the slice passed to Verify) of the
	// first entry that fails either check, when OK is false. F5 §8.2
	// requires the break's position, not just whether one exists.
	BrokenAt int
}

// Verify walks entries in order and reports the first break (F5 §8.2:
// "Provide `audit verify` which walks the chain and reports the first
// break"). An empty chain is trivially OK.
//
// Two independent things are checked per entry, either of which can
// break the chain: its own Hash no longer matches its CanonicalBytes
// (the entry itself was altered), or its PrevHash no longer matches the
// previous entry's Hash (an entry was altered, inserted or removed
// somewhere earlier, which cascades — even if this entry's own Hash is
// internally self-consistent with its own now-different PrevHash, the
// PrevHash itself is wrong).
func Verify(entries []Entry) VerifyResult {
	var prevHash []byte
	for i, e := range entries {
		if !bytes.Equal(e.PrevHash, prevHash) {
			return VerifyResult{OK: false, BrokenAt: i}
		}
		if !bytes.Equal(e.Hash, e.ComputeHash()) {
			return VerifyResult{OK: false, BrokenAt: i}
		}
		prevHash = e.Hash
	}
	return VerifyResult{OK: true, BrokenAt: -1}
}
