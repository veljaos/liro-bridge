package main

import (
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
)

// TestAWindowNoWebApplicationCanReachSaysSo is D27's option 3 sentence: a
// window whose agent's protocol did not start, or that has no agent at
// all, says above the document list that web applications cannot reach
// the program — and a window with a serving agent says nothing of the kind.
func TestAWindowNoWebApplicationCanReachSaysSo(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		m := newMainWindow(config.Default(), locale)
		if got := m.noticeTexts(); len(got) != 0 {
			t.Errorf("%s: a window with an agent behind it shows %v", locale, got)
		}

		m.unreachable = true
		got := m.noticeTexts()
		if len(got) != 1 || !got[0].Problem {
			t.Fatalf("%s: an unreachable window's notices are %+v, want one problem", locale, got)
		}
		if got[0].Text == "" || got[0].Text == "main.agent_unreachable" {
			t.Errorf("%s: the sentence is %q — missing from the catalogue", locale, got[0].Text)
		}
	}
}
