package signing

import (
	"testing"
	"time"
)

// TestDetectPINPolicyTable is F2 §7.2's exact table.
func TestDetectPINPolicyTable(t *testing.T) {
	ms := time.Millisecond
	cases := []struct {
		name string
		durs []time.Duration
		want PINPolicy
	}{
		{"sig2=400 sig3=410", []time.Duration{5000 * ms, 400 * ms, 410 * ms}, PINPolicyPerBatch},
		{"sig2=5000 sig3=4800", []time.Duration{5000 * ms, 5000 * ms, 4800 * ms}, PINPolicyPerSignature},
		{"sig2=1900 sig3=1950 (below threshold)", []time.Duration{5000 * ms, 1900 * ms, 1950 * ms}, PINPolicyPerBatch},
		{"sig2=2100 sig3=400 (median decides)", []time.Duration{5000 * ms, 2100 * ms, 400 * ms}, PINPolicyPerBatch},
		{"batch of one", []time.Duration{5000 * ms}, PINPolicyUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectPINPolicy(c.durs); got != c.want {
				t.Fatalf("detectPINPolicy(%v) = %v, want %v", c.durs, got, c.want)
			}
		})
	}
}

// TestDetectPINPolicyProvisionalAfterSecondSignature proves the
// provisional read after exactly two signatures (no third yet) uses
// sig2 alone, per F2 §5.5's two-step procedure.
func TestDetectPINPolicyProvisionalAfterSecondSignature(t *testing.T) {
	ms := time.Millisecond
	if got := detectPINPolicy([]time.Duration{5000 * ms, 3000 * ms}); got != PINPolicyPerSignature {
		t.Fatalf("provisional read = %v, want PINPolicyPerSignature", got)
	}
	if got := detectPINPolicy([]time.Duration{5000 * ms, 300 * ms}); got != PINPolicyPerBatch {
		t.Fatalf("provisional read = %v, want PINPolicyPerBatch", got)
	}
}

func TestBuildTimingReportComputesFirstAndMedian(t *testing.T) {
	ms := time.Millisecond
	report := buildTimingReport([]time.Duration{4900 * ms, 410 * ms, 415 * ms, 405 * ms})
	if report.FirstSignature != 4900*ms {
		t.Fatalf("FirstSignature = %v, want 4900ms", report.FirstSignature)
	}
	if report.MedianSubsequent != 410*ms {
		t.Fatalf("MedianSubsequent = %v, want 410ms", report.MedianSubsequent)
	}
	wantTotal := 4900*ms + 410*ms + 415*ms + 405*ms
	if report.Total != wantTotal {
		t.Fatalf("Total = %v, want %v", report.Total, wantTotal)
	}
}

func TestBuildTimingReportHandlesEmpty(t *testing.T) {
	report := buildTimingReport(nil)
	if report.FirstSignature != 0 || report.MedianSubsequent != 0 || report.PINPolicy != PINPolicyUnknown {
		t.Fatalf("empty report = %+v, want all zero/unknown", report)
	}
}

func TestEstimatedTotalIsIndeterminateBeforeFirstSignature(t *testing.T) {
	if _, ok := EstimatedTotal(0, 10, 0); ok {
		t.Fatal("EstimatedTotal with first == 0 must be indeterminate (F2 §5.6: \"Preparing card…\")")
	}
}

func TestEstimatedTotalUsesMeasuredValuesOnly(t *testing.T) {
	first := 4900 * time.Millisecond
	median := 410 * time.Millisecond
	got, ok := EstimatedTotal(first, 100, median)
	if !ok {
		t.Fatal("expected a determinate estimate once the first signature is known")
	}
	want := first + 99*median
	if got != want {
		t.Fatalf("EstimatedTotal = %v, want %v", got, want)
	}
}

func TestEstimatedTotalSingleItemBatch(t *testing.T) {
	first := 4900 * time.Millisecond
	got, ok := EstimatedTotal(first, 1, 0)
	if !ok || got != first {
		t.Fatalf("EstimatedTotal(single item) = (%v, %v), want (%v, true)", got, ok, first)
	}
}
