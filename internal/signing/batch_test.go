package signing

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

func digestItems(n int) []BatchItem {
	items := make([]BatchItem, n)
	for i := range items {
		d := make([]byte, 32)
		d[0] = byte(i)
		items[i] = BatchItem{Digest: d, Algorithm: keysource.DigestSHA256, Label: "file.pdf"}
	}
	return items
}

func TestNewBatchComputesFingerprint(t *testing.T) {
	items := digestItems(3)
	b := NewBatch("batch-1", "ABC", items)
	if len(b.Fingerprint) != 32 {
		t.Fatalf("Fingerprint length = %d, want 32 (SHA-256)", len(b.Fingerprint))
	}
	// Re-deriving it independently must agree.
	if got := computeFingerprint(items); string(got) != string(b.Fingerprint) {
		t.Fatal("Fingerprint does not match an independent recomputation")
	}
}

func TestSignProducesTenSignaturesInOrder(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	inner := &fakeKeySession{}
	sess := newTestSession(inner, clock)
	batch := NewBatch("b", "ABC", digestItems(10))

	result, err := Sign(context.Background(), batch, sess)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(result.Signatures) != 10 {
		t.Fatalf("len(Signatures) = %d, want 10", len(result.Signatures))
	}
	for i, sig := range result.Signatures {
		if sig == nil {
			t.Fatalf("Signatures[%d] is nil, want a signature", i)
		}
	}
	if len(result.Failures) != 0 {
		t.Fatalf("Failures = %v, want none", result.Failures)
	}
}

// TestSignSkipsFailedItemAndContinues is the failing test for F2 §5.3:
// item 5 (index 4) fails with an ordinary error, and items 6-10 must
// still be attempted.
func TestSignSkipsFailedItemAndContinues(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	inner := &fakeKeySession{signResults: []signResult{
		{sig: []byte("s1")}, {sig: []byte("s2")}, {sig: []byte("s3")}, {sig: []byte("s4")},
		{err: codeErr(errs.CodeSignFailed)}, // item 5 (index 4) fails
	}}
	sess := newTestSession(inner, clock)
	batch := NewBatch("b", "ABC", digestItems(10))

	result, err := Sign(context.Background(), batch, sess)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(result.Failures) != 1 || result.Failures[0].Index != 4 {
		t.Fatalf("Failures = %v, want exactly one at index 4", result.Failures)
	}
	if result.Signatures[4] != nil {
		t.Fatal("Signatures[4] must be nil for the failed item")
	}
	successCount := 0
	for i, sig := range result.Signatures {
		if i == 4 {
			continue
		}
		if sig == nil {
			t.Fatalf("Signatures[%d] is nil, want items 6-10 (and 1-4) to still be signed", i)
		}
		successCount++
	}
	if successCount != 9 {
		t.Fatalf("successCount = %d, want 9", successCount)
	}
}

// TestSignAbortsOnCardNotPresent is one of F2 §5.3's two abort
// exceptions.
func TestSignAbortsOnCardNotPresent(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	inner := &fakeKeySession{signResults: []signResult{
		{sig: []byte("s1")}, {sig: []byte("s2")},
		{err: codeErr(errs.CodeCardNotPresent)},
	}}
	sess := newTestSession(inner, clock)
	batch := NewBatch("b", "ABC", digestItems(10))

	result, err := Sign(context.Background(), batch, sess)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if inner.calls != 3 {
		t.Fatalf("SignDigest called %d times, want exactly 3 (abort after item 3)", inner.calls)
	}
	if len(result.Failures) != 1 || result.Failures[0].Code != errs.CodeCardNotPresent {
		t.Fatalf("Failures = %v, want one CARD_NOT_PRESENT", result.Failures)
	}
}

// TestSignDoesNotAbortOnPlainSignFailed proves SIGN_FAILED alone (not
// CARD_NOT_PRESENT or PIN_LOCKED) does not abort the batch (F2 §5.3).
func TestSignDoesNotAbortOnPlainSignFailed(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	inner := &fakeKeySession{signResults: []signResult{
		{sig: []byte("s1")},
		{err: codeErr(errs.CodeSignFailed)},
	}}
	sess := newTestSession(inner, clock)
	batch := NewBatch("b", "ABC", digestItems(5))

	result, err := Sign(context.Background(), batch, sess)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if inner.calls != 5 {
		t.Fatalf("SignDigest called %d times, want 5 (SIGN_FAILED must not abort)", inner.calls)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("Failures = %v, want exactly one", result.Failures)
	}
}

// TestSignAbortsOnPINLocked is F2 §5.3's other abort exception.
func TestSignAbortsOnPINLocked(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	inner := &fakeKeySession{signResults: []signResult{{err: codeErr(errs.CodePINLocked)}}}
	sess := newTestSession(inner, clock)
	batch := NewBatch("b", "ABC", digestItems(5))

	result, err := Sign(context.Background(), batch, sess)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("SignDigest called %d times, want exactly 1", inner.calls)
	}
	if len(result.Failures) != 1 || result.Failures[0].Code != errs.CodePINLocked {
		t.Fatalf("Failures = %v, want one PIN_LOCKED", result.Failures)
	}
}

// TestSignAbortsOnFingerprintMismatch is the failing test for F2 §5.2:
// mutating a batch's items after construction (simulating a change
// between approval and execution) must abort with CodeInternal before
// any signature is attempted.
func TestSignAbortsOnFingerprintMismatch(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	inner := &fakeKeySession{}
	sess := newTestSession(inner, clock)

	batch := NewBatch("b", "ABC", digestItems(3))
	batch.Items[0].Digest[0] ^= 0xFF // mutate after the fingerprint was computed

	_, err := Sign(context.Background(), batch, sess)
	if err == nil {
		t.Fatal("Sign must fail when the fingerprint no longer matches the items")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInternal {
		t.Fatalf("error = %v, want INTERNAL", err)
	}
	if inner.calls != 0 {
		t.Fatalf("SignDigest called %d times, want 0 — a fingerprint mismatch must abort before signing", inner.calls)
	}
}

func TestSignReportsTimingAndPINPolicy(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	// Each SignDigest call advances the fake clock so buildTimingReport
	// sees a realistic shape: a slow first signature, fast rest.
	delays := []time.Duration{4900 * time.Millisecond, 410 * time.Millisecond, 405 * time.Millisecond}
	inner := &clockAdvancingSession{clock: clock, delays: delays}
	sess := newTestSession(inner, clock)
	batch := NewBatch("b", "ABC", digestItems(3))

	result, err := Sign(context.Background(), batch, sess)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if result.Timing.FirstSignature != delays[0] {
		t.Fatalf("FirstSignature = %v, want %v", result.Timing.FirstSignature, delays[0])
	}
	if result.Timing.PINPolicy != PINPolicyPerBatch {
		t.Fatalf("PINPolicy = %v, want PINPolicyPerBatch", result.Timing.PINPolicy)
	}
}

// TestSignWarnsWhenFirstSignatureExceedsThirtySeconds is the failing
// test for F2 §5.6's warning: something is probably wrong with the
// reader or middleware if the first signature takes this long, even
// though the operation may still succeed.
func TestSignWarnsWhenFirstSignatureExceedsThirtySeconds(t *testing.T) {
	old := slog.Default()
	defer slog.SetDefault(old)
	var logBuf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))

	clock := &fakeClock{t: time.Now()}
	inner := &clockAdvancingSession{clock: clock, delays: []time.Duration{31 * time.Second}}
	sess := newTestSession(inner, clock)
	batch := NewBatch("b", "ABC", digestItems(1))

	if _, err := Sign(context.Background(), batch, sess); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !strings.Contains(logBuf.String(), "first signature took longer than expected") {
		t.Fatalf("expected a warning log for a 31s first signature, got: %s", logBuf.String())
	}
}

func TestSignDoesNotWarnForNormalFirstSignature(t *testing.T) {
	old := slog.Default()
	defer slog.SetDefault(old)
	var logBuf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))

	clock := &fakeClock{t: time.Now()}
	inner := &clockAdvancingSession{clock: clock, delays: []time.Duration{5 * time.Second}}
	sess := newTestSession(inner, clock)
	batch := NewBatch("b", "ABC", digestItems(1))

	if _, err := Sign(context.Background(), batch, sess); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if strings.Contains(logBuf.String(), "first signature took longer than expected") {
		t.Fatalf("must not warn for a normal 5s first signature, got: %s", logBuf.String())
	}
}

// clockAdvancingSession is a keysource.Session that advances a
// fakeClock by a scripted delay on each SignDigest call, so
// time.Since-based measurements in batch.go see deterministic
// durations without a real sleep.
type clockAdvancingSession struct {
	clock  *fakeClock
	delays []time.Duration
	calls  int
	closed bool
}

func (c *clockAdvancingSession) SignDigest(context.Context, keysource.DigestAlgorithm, []byte) ([]byte, error) {
	if c.calls < len(c.delays) {
		c.clock.advance(c.delays[c.calls])
	}
	c.calls++
	return []byte("sig"), nil
}
func (c *clockAdvancingSession) Certificate() keysource.Certificate { return keysource.Certificate{} }
func (c *clockAdvancingSession) Chain() [][]byte                    { return nil }
func (c *clockAdvancingSession) Close() error                       { c.closed = true; return nil }
