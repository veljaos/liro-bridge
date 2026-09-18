package pkcs11

import (
	"context"
	"errors"
	"testing"
)

// TestAClosedHolderAnswersRatherThanPanicking covers the two things about a
// LiveModule that are decided here rather than by a card.
//
// # Close is idempotent, and that is a decision rather than caution
//
// Twice is the ordinary case: the worker closes on an OpShutdown request and
// again from the defer that covers every other way its loop can end. The
// alternative was to make every caller remember, or to rest on what a second
// runtime.Pinner.Unpin does — which this code has not measured and does not
// need to, because nilling the module makes the question moot rather than
// answered.
//
// # A request after shutdown is answered, not fatal
//
// A worker that panicked on a late request would look to its parent exactly
// like a module that killed it: a non-zero exit with nothing to say which. The
// whole point of the out-of-process arrangement is being able to tell those
// apart (D-272, D-275), so a use-after-close is an error value.
//
// It needs no module and no card: a zero LiveModule is precisely a closed one,
// which is what lets this be checked at all rather than only on a machine with
// a reader.
func TestAClosedHolderAnswersRatherThanPanicking(t *testing.T) {
	var closed LiveModule // the zero value is a holder with nothing open

	if err := closed.Close(); err != nil {
		t.Errorf("Close on a holder with nothing open: %v, want nil", err)
	}
	if err := closed.Close(); err != nil {
		t.Errorf("Close a second time: %v, want nil — the worker closes on "+
			"shutdown and again from its defer", err)
	}

	ctx := context.Background()

	if _, err := closed.Enumerate(ctx); !errors.Is(err, ErrModuleClosed) {
		t.Errorf("Enumerate after Close: %v, want ErrModuleClosed", err)
	}
	if _, err := closed.List(ctx); !errors.Is(err, ErrModuleClosed) {
		t.Errorf("List after Close: %v, want ErrModuleClosed", err)
	}
	if _, err := closed.ChainFor(ctx, "abc"); !errors.Is(err, ErrModuleClosed) {
		t.Errorf("ChainFor after Close: %v, want ErrModuleClosed", err)
	}
}

// TestHoldRefusesAPathThatIsNotAModule is the same distinction discovery
// already draws, at the holder: a file is a module because it exports
// C_GetFunctionList, never because its name looks promising.
//
// It uses a path that cannot exist rather than the IAIK wrapper D-271 names,
// so it says nothing about anybody's installed middleware and runs anywhere.
func TestHoldRefusesAPathThatIsNotAModule(t *testing.T) {
	if _, err := NewSource("").Hold(); err == nil {
		t.Error("Hold with no module path returned no error")
	}
	if _, err := NewSource(`C:\this\path\does\not\exist\nothing.dll`).Hold(); err == nil {
		t.Error("Hold on a path that does not exist returned no error")
	}
}
