package main

import (
	"testing"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestALevelBelowTheOneSetIsNeverShownAsPositive is C26's first fix
// (D-469, the owner: "fix that first"): B-T reached where B-LT was set
// was coloured positive with nothing beside it, which reads as the
// signature asked for. Every Serbian card stops at B-T today (D-466).
func TestALevelBelowTheOneSetIsNeverShownAsPositive(t *testing.T) {
	c := i18n.Load("en")
	bt, blt, bb := string(pades.LevelBT), string(pades.LevelBLT), string(pades.LevelBB)

	if got := achievedLevelIntent(bt, blt); got != string(ui.IntentWarning) {
		t.Errorf("B-T where B-LT was set: intent %q, want the warning", got)
	}
	if got := achievedLevelNote(c, bt, blt); got != c.T("consent.level_blt_short") {
		t.Errorf("B-T where B-LT was set: note %q, want the shortfall said", got)
	}

	// The controls: B-T where B-T was set, and B-LT, are what was asked
	// for and stay positive and silent; B-B keeps its own sentence.
	for _, tc := range []struct{ level, requested string }{{bt, bt}, {blt, blt}} {
		if got := achievedLevelIntent(tc.level, tc.requested); got != string(ui.IntentPositive) {
			t.Errorf("%s where %s was set: intent %q, want positive", tc.level, tc.requested, got)
		}
		if got := achievedLevelNote(c, tc.level, tc.requested); got != "" {
			t.Errorf("%s where %s was set: note %q, want none", tc.level, tc.requested, got)
		}
	}
	if got := achievedLevelNote(c, bb, blt); got != c.T("consent.level_bb") {
		t.Errorf("B-B: note %q, want its own sentence", got)
	}
}

// TestSigningWithoutATimestampIsOfferedOnlyWhenNobodyAskedForMore is the
// other half of C26 (D-469): after a configured authority has failed,
// and whenever a calling program named its level, the question offers
// fixing it or stopping — never taking less. And an answer the page
// sent for a button it was not showing is not a decision.
func TestSigningWithoutATimestampIsOfferedOnlyWhenNobodyAskedForMore(t *testing.T) {
	for _, tc := range []struct {
		reason      consent.TSAReason
		callerNamed bool
		want        bool
	}{
		{consent.TSAReasonNotConfigured, false, true},
		{consent.TSAReasonNotConfigured, true, false},
		{consent.TSAReasonUnreachable, false, false},
		{consent.TSAReasonUnreachable, true, false},
	} {
		if got := offersWithoutTimestamp(tc.reason, tc.callerNamed); got != tc.want {
			t.Errorf("offersWithoutTimestamp(%s, caller named a level: %v) = %v, want %v", tc.reason, tc.callerNamed, got, tc.want)
		}
	}

	if got := acceptedTSAChoice("withoutTimestamp", false); got != "" {
		t.Errorf("an answer for a button not on screen was taken as %q", got)
	}
	if got := acceptedTSAChoice("withoutTimestamp", true); got != "withoutTimestamp" {
		t.Errorf("the offered answer was refused: %q", got)
	}
	if got := acceptedTSAChoice("configure", false); got != "configure" {
		t.Errorf("configure was refused: %q", got)
	}
}

// TestACallerThatNamedALevelIsRefusedBelowIt is the protocol's half of
// C26 (D-469): only a request that named no level may come back below
// B-LT; one that named B-LT gets LEVEL_NOT_REACHED from the library,
// whose refusal internal/pades tests.
func TestACallerThatNamedALevelIsRefusedBelowIt(t *testing.T) {
	if keepsBTWhenBLTFallsShort(api.SignRequest{Level: "b-lt"}) {
		t.Error("a caller that asked for b-lt would be handed B-T")
	}
	if !keepsBTWhenBLTFallsShort(api.SignRequest{}) {
		t.Error("a caller that named no level would be refused where the window keeps B-T")
	}
}
