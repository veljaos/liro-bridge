package main

import (
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
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
// and whenever a level was asked for (a calling program's, or since D-476
// one chosen in Settings), the question offers
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

// TestALevelSomebodyChoseIsARequest is C26 with D-476 (worklist 0.7):
// a level a calling program named, or a person chose in Settings, is a
// request, reached or refused with LEVEL_NOT_REACHED, and the timestamp
// question does not offer less. Only the best available, the default,
// keeps B-T where B-LT's evidence is missing, and may offer signing
// without a timestamp when no authority is configured. The cfg here is
// the batch's: protocolserver.go has already put a caller's named level
// into it.
func TestALevelSomebodyChoseIsARequest(t *testing.T) {
	for _, tc := range []struct {
		why     string
		level   string
		request bool
	}{
		{"the default, the best available", "", false},
		{"B-LT chosen in Settings, or named by a caller", "b-lt", true},
		{"B-T chosen in Settings, or named by a caller", "b-t", true},
		{"B-B chosen in Settings", "b-b", true},
	} {
		cfg := config.Default()
		cfg.SignatureLevel = tc.level
		if got := levelIsARequest(cfg); got != tc.request {
			t.Errorf("%s: a request %v, want %v", tc.why, got, tc.request)
		}
	}
}
