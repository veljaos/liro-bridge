package signing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

// errFingerprintMismatch is the cause wrapped into the CodeInternal
// error Sign returns when the batch fingerprint does not match what was
// computed at approval time (F2 §5.2).
var errFingerprintMismatch = errors.New("signing: batch contents changed between approval and execution")

// Batch is a set of digests approved together and signed with a single
// PIN entry (F2 §5.1).
type Batch struct {
	ID         string
	Thumbprint string
	Items      []BatchItem

	// Fingerprint is SHA-256 over the concatenated digests, in order,
	// computed once at construction — "before approval" (F2 §5.2). It
	// is shown on the consent screen (SPEC §6.6, arriving in F5) so a
	// technical user can compare what was approved against what the
	// calling application claims it sent.
	Fingerprint []byte
}

// BatchItem is one digest to sign within a Batch.
type BatchItem struct {
	// Digest is a pre-computed hash. Length must match Algorithm.
	Digest    []byte
	Algorithm keysource.DigestAlgorithm

	// Label is a caller-supplied display string. Untrusted input (SPEC
	// §6.6) — never used as a path, never logged.
	Label string
}

// ItemFailure records why one BatchItem did not produce a signature.
type ItemFailure struct {
	Index int
	Code  errs.Code
}

// BatchResult is the outcome of signing a Batch.
type BatchResult struct {
	// Signatures has exactly len(Batch.Items) entries; Signatures[i] is
	// nil where Items[i] failed (F2 §5.1/§5.3).
	Signatures [][]byte
	Failures   []ItemFailure
	Timing     TimingReport
}

// computeFingerprint implements F2 §5.2: SHA-256 over the concatenation
// of every digest, in order.
func computeFingerprint(items []BatchItem) []byte {
	h := sha256.New()
	for _, it := range items {
		h.Write(it.Digest)
	}
	return h.Sum(nil)
}

// NewBatch builds a Batch and computes its fingerprint immediately —
// "before approval" (F2 §5.2). Sign re-verifies the fingerprint
// immediately before signing, so any mutation of items between
// approval and execution is caught.
func NewBatch(id, thumbprint string, items []BatchItem) *Batch {
	return &Batch{
		ID:          id,
		Thumbprint:  thumbprint,
		Items:       items,
		Fingerprint: computeFingerprint(items),
	}
}

// abortsBatch reports whether code means continuing the batch is
// pointless (F2 §5.3): the card was removed, or it is now blocked.
// Everything else is skipped and reported.
func abortsBatch(code errs.Code) bool {
	return code == errs.CodeCardNotPresent || code == errs.CodePINLocked
}

// codeOf extracts an errs.Code from err, defaulting to CodeSignFailed
// for an error a backend did not classify (F2 §2.4's own fallback rule).
func codeOf(err error) errs.Code {
	var e *errs.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return errs.CodeSignFailed
}

// Sign signs every item in batch using sess, in order — sequentially,
// never in parallel (F2 §4.2): the card is a single serial device, so
// concurrent requests would only queue in the driver anyway, and
// concurrent access to smart card APIs is a known source of
// driver-level failures.
//
// The fingerprint is re-verified immediately before the first
// signature (F2 §5.2). A mismatch means the batch changed between
// approval and execution — this aborts immediately with CodeInternal
// and is logged loudly, since it is exactly the kind of gap this check
// exists to close cheaply.
func Sign(ctx context.Context, batch *Batch, sess *Session) (*BatchResult, error) {
	if got := computeFingerprint(batch.Items); !bytes.Equal(got, batch.Fingerprint) {
		slog.Error("signing: batch fingerprint mismatch between approval and execution — aborting",
			"batchID", batch.ID, "itemCount", len(batch.Items))
		return nil, errs.New(errs.CodeInternal, errFingerprintMismatch)
	}

	result := &BatchResult{Signatures: make([][]byte, len(batch.Items))}
	var durations []time.Duration
	// Measured through sess's own clock, not time.Now() directly: in
	// production that clock IS time.Now (Manager.now), but reusing it
	// lets tests substitute a fake clock and get deterministic,
	// millisecond-fast timing measurements instead of real sleeps.
	start := sess.now()

	for i, item := range batch.Items {
		itemStart := sess.now()
		sig, err := sess.SignDigest(ctx, item.Algorithm, item.Digest)
		d := sess.now().Sub(itemStart)
		durations = append(durations, d)

		if i == 0 && d > firstSignatureWarnThreshold {
			slog.Warn("signing: first signature took longer than expected",
				"batchID", batch.ID, "duration", d.String(), "threshold", firstSignatureWarnThreshold.String())
		}

		if err != nil {
			code := codeOf(err)
			result.Failures = append(result.Failures, ItemFailure{Index: i, Code: code})
			if abortsBatch(code) {
				result.Timing = buildTimingReport(durations)
				result.Timing.Total = sess.now().Sub(start)
				return result, nil
			}
			continue
		}
		result.Signatures[i] = sig
	}

	result.Timing = buildTimingReport(durations)
	result.Timing.Total = sess.now().Sub(start)
	return result, nil
}
