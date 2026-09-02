package signing

import (
	"sort"
	"time"
)

// alwaysAuthenticateThreshold is the boundary F2 §5.5 uses to detect
// PINPolicyPerSignature: the measured subsequent-signature time on a
// real e-ID card is ~413ms (F2 §5.4). A PIN dialog cannot be shown,
// filled and dismissed by a human in under two seconds, so 2000ms sits
// roughly 5x above normal card latency and far below any plausible
// human interaction — neither a slow card nor a fast typist lands in
// the wrong bucket.
const alwaysAuthenticateThreshold = 2000 * time.Millisecond

// firstSignatureWarnThreshold: if the first signature takes longer than
// this, something is probably wrong with the reader or middleware, even
// though the operation may still succeed (F2 §5.6).
const firstSignatureWarnThreshold = 30 * time.Second

// PINPolicy describes how often a card asks for the PIN within one
// batch (F2 §5.4). There is no reliable property to query for this —
// it is detected from timing.
type PINPolicy int

const (
	// PINPolicyUnknown is the state before enough signatures have
	// completed to decide, and permanently for a batch of one (F2 §5.5).
	PINPolicyUnknown PINPolicy = iota
	// PINPolicyPerBatch means one PIN entry unlocks the whole batch.
	PINPolicyPerBatch
	// PINPolicyPerSignature means the card requires a PIN for every
	// signature (ALWAYS_AUTHENTICATE).
	PINPolicyPerSignature
)

// String implements fmt.Stringer for readable logs and CLI output.
func (p PINPolicy) String() string {
	switch p {
	case PINPolicyPerBatch:
		return "per-batch"
	case PINPolicyPerSignature:
		return "per-signature"
	default:
		return "unknown"
	}
}

// TimingReport is what a completed (or in-progress) batch knows about
// its own timing (F2 §5.4). Every duration comes from an actual
// measurement — never a hard-coded constant (F2 §5.6): Pošta is
// migrating to RSA-4096, which will be slower, and a hard-coded
// estimate would silently become a lie.
type TimingReport struct {
	FirstSignature   time.Duration
	MedianSubsequent time.Duration
	Total            time.Duration
	PINPolicy        PINPolicy
}

// detectPINPolicy implements F2 §5.5's exact procedure: sig2 alone
// gives a provisional read; sig2 and sig3 together, by their median,
// give the final one. A batch that never reaches a second signature
// stays PINPolicyUnknown — permanently, per F2 §5.5's own rule that a
// batch of one never determines a policy.
//
// durations holds one entry per signature attempt actually made, in
// order, regardless of whether that attempt succeeded — a slow failure
// still reflects whether a PIN dialog appeared, which is the signal
// being measured.
func detectPINPolicy(durations []time.Duration) PINPolicy {
	if len(durations) < 2 {
		return PINPolicyUnknown
	}
	sig2 := durations[1]
	if len(durations) < 3 {
		return classifyByThreshold(sig2)
	}
	sig3 := durations[2]
	return classifyByThreshold(median2(sig2, sig3))
}

func classifyByThreshold(d time.Duration) PINPolicy {
	if d > alwaysAuthenticateThreshold {
		return PINPolicyPerSignature
	}
	return PINPolicyPerBatch
}

func median2(a, b time.Duration) time.Duration {
	return (a + b) / 2
}

// buildTimingReport summarises a completed batch's per-item durations.
// durations includes an entry for every signature attempt, successful
// or not — a failed attempt still took real wall-clock time. Items
// never attempted (batch aborted early) have no entry.
func buildTimingReport(durations []time.Duration) TimingReport {
	report := TimingReport{PINPolicy: detectPINPolicy(durations)}
	if len(durations) == 0 {
		return report
	}
	report.FirstSignature = durations[0]
	if len(durations) > 1 {
		report.MedianSubsequent = medianOf(durations[1:])
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	report.Total = total
	return report
}

// medianOf returns the median of a non-empty slice of durations,
// without mutating the caller's slice.
func medianOf(durations []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// EstimatedTotal implements F2 §5.6's ETA formula: once the first,
// slower signature is measured, the whole batch's total time is
// estimated as that first signature plus every remaining item at the
// measured median. It returns ok == false — the indeterminate
// "Preparing card…" state — until first is actually known (first <= 0),
// which is exactly the window before the first signature completes.
func EstimatedTotal(first time.Duration, totalItems int, median time.Duration) (estimate time.Duration, ok bool) {
	if first <= 0 {
		return 0, false
	}
	if totalItems <= 1 {
		return first, true
	}
	return first + time.Duration(totalItems-1)*median, true
}
