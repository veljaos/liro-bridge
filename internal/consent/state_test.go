package consent

import (
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/signing"
)

// TestProgressForTimingIsPreparingCardBeforeFirstSignature is F5 §5.4:
// the first ~4.9s window (SPEC §12.9) is an indeterminate
// "Preparing card..." state, not a progress bar sitting at 0%.
func TestProgressForTimingIsPreparingCardBeforeFirstSignature(t *testing.T) {
	p := ProgressForTiming(0, 100, 0, 0, signing.PINPolicyUnknown)
	if p.State != StatePreparingCard {
		t.Fatalf("got state %q, want %q", p.State, StatePreparingCard)
	}
	if p.ETAKnown {
		t.Fatal("ETAKnown must be false before the first signature completes")
	}
}

func TestProgressForTimingIsSigningOnceFirstSignatureKnown(t *testing.T) {
	p := ProgressForTiming(1, 100, 5*time.Second, 400*time.Millisecond, signing.PINPolicyPerBatch)
	if p.State != StateSigning {
		t.Fatalf("got state %q, want %q", p.State, StateSigning)
	}
	if !p.ETAKnown {
		t.Fatal("ETAKnown must be true once the first signature is measured")
	}
	if p.PerSignaturePIN {
		t.Fatal("PerSignaturePIN must be false for PINPolicyPerBatch")
	}
}

// TestProgressForTimingWarnsOnPerSignaturePIN is F5 §5.4: "If the PIN
// policy is detected as per-signature, change the message to warn that
// the PIN will be requested repeatedly."
func TestProgressForTimingWarnsOnPerSignaturePIN(t *testing.T) {
	p := ProgressForTiming(2, 10, 5*time.Second, 3*time.Second, signing.PINPolicyPerSignature)
	if !p.PerSignaturePIN {
		t.Fatal("PerSignaturePIN must be true once PINPolicyPerSignature is detected")
	}
}

// TestProgressForTimingETAIsNeverAConstant is a direct check on this
// package's own call site of the F2 §5.6 rule (D-030): two different
// measured medians must produce two different ETAs, proving the value
// actually depends on the measurement rather than being hard-coded.
func TestProgressForTimingETAIsNeverAConstant(t *testing.T) {
	fast := ProgressForTiming(1, 10, 1*time.Second, 100*time.Millisecond, signing.PINPolicyPerBatch)
	slow := ProgressForTiming(1, 10, 1*time.Second, 2*time.Second, signing.PINPolicyPerBatch)
	if fast.ETA == slow.ETA {
		t.Fatalf("ETA did not change with the measured median: fast=%v slow=%v", fast.ETA, slow.ETA)
	}
}

func TestNeedsTSAChoice(t *testing.T) {
	if !NeedsTSAChoice(errs.CodeTSAUnavailable) {
		t.Fatal("CodeTSAUnavailable must need the SPEC §12.8 choice")
	}
	if NeedsTSAChoice(errs.CodeCardNotPresent) {
		t.Fatal("CodeCardNotPresent must not need the TSA choice")
	}
}
