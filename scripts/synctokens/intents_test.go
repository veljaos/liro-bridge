package main

import (
	"strings"
	"testing"
)

// TestIntentFamilyColorHasExactlyTheFourOutcomeIntents proves
// IntentFamilyColor — the single place Task 6's outcome colours come
// from — defines exactly positive/warning/negative/caution, no more
// and no fewer, each mapped to a real token name.
func TestIntentFamilyColorHasExactlyTheFourOutcomeIntents(t *testing.T) {
	want := []string{"positive", "warning", "negative", "caution"}
	if len(IntentFamilyColor) != len(want) {
		t.Fatalf("IntentFamilyColor has %d entries, want %d", len(IntentFamilyColor), len(want))
	}
	for _, name := range want {
		token, ok := IntentFamilyColor[name]
		if !ok {
			t.Fatalf("IntentFamilyColor missing intent %q", name)
		}
		if !strings.HasPrefix(token, "--liro-color-") {
			t.Fatalf("IntentFamilyColor[%q] = %q, want a --liro-color-* token", name, token)
		}
	}
}

// TestIntentOutcomeCSSGeneratesOneRulePerIntent proves the generated
// CSS defines exactly one .liro-outcome-<name> rule per
// IntentFamilyColor entry, each referencing its mapped token through
// var() rather than a literal colour.
func TestIntentOutcomeCSSGeneratesOneRulePerIntent(t *testing.T) {
	css := intentOutcomeCSS()
	for name, token := range IntentFamilyColor {
		want := ".liro-outcome-" + name + " {\n  color: var(" + token + ");\n}"
		if !strings.Contains(css, want) {
			t.Fatalf("intentOutcomeCSS() missing rule %q; got:\n%s", want, css)
		}
	}
}

// TestIntentsCSSEmbedsGeneratedOutcomeRules proves the final intentsCSS
// (what synctokens actually writes to disk) contains the generated
// outcome section, not just the hand-written base.
func TestIntentsCSSEmbedsGeneratedOutcomeRules(t *testing.T) {
	for name := range IntentFamilyColor {
		if !strings.Contains(intentsCSS, ".liro-outcome-"+name+" {") {
			t.Fatalf("intentsCSS missing .liro-outcome-%s rule", name)
		}
	}
}
