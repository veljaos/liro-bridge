package consent

import "testing"

// TestDefaultStampChoiceIsVisibleBottomRight pins the answer step 3
// offers a first-time user, in the package that defines it: visible,
// bottom-right (SPEC §13.1's own default corner). D-103 records why it
// is visible — a signature the signer cannot see reads, to them, as one
// that was never applied.
func TestDefaultStampChoiceIsVisibleBottomRight(t *testing.T) {
	got := DefaultStampChoice()
	if !got.Visible {
		t.Error("DefaultStampChoice().Visible = false, want true")
	}
	if got.Position != StampPositionBottomRight {
		t.Errorf("DefaultStampChoice().Position = %q, want %q", got.Position, StampPositionBottomRight)
	}
}

// TestStampPositionsAreTheFourCornersInOrder: step 3 offers the
// default first, then the remaining three, and offers nothing else —
// a visual placement picker is a later phase (SPEC §13.1).
func TestStampPositionsAreTheFourCornersInOrder(t *testing.T) {
	want := []string{"bottom-right", "bottom-left", "top-right", "top-left"}
	got := StampPositions()
	if len(got) != len(want) {
		t.Fatalf("StampPositions() returned %d values, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("StampPositions()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestValidStampPosition(t *testing.T) {
	for _, p := range StampPositions() {
		if !ValidStampPosition(p) {
			t.Errorf("ValidStampPosition(%q) = false", p)
		}
	}
	for _, p := range []string{"", "middle", "Bottom-Right", "bottomright"} {
		if ValidStampPosition(p) {
			t.Errorf("ValidStampPosition(%q) = true, want false", p)
		}
	}
}

// TestNormalisedReplacesAnUnknownCorner: a stale configuration file or
// a tampered page cannot put a position nothing downstream understands
// into pades.StampOptions.
func TestNormalisedReplacesAnUnknownCorner(t *testing.T) {
	got := StampChoice{Visible: true, Position: "middle"}.Normalised()
	if got.Position != StampPositionBottomRight {
		t.Errorf("Normalised().Position = %q, want %q", got.Position, StampPositionBottomRight)
	}
	if !got.Visible {
		t.Error("Normalised() changed Visible")
	}
	kept := StampChoice{Visible: false, Position: StampPositionTopLeft}.Normalised()
	if kept.Position != StampPositionTopLeft || kept.Visible {
		t.Errorf("Normalised() changed a valid choice: %+v", kept)
	}
}
