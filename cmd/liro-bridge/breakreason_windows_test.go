//go:build windows

package main

import (
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/i18n"
)

// Every reason a chain can break becomes a sentence, in every locale,
// in both places one is rendered.
//
// Both switches end in `default: return string(r)`, which is the right
// fallback for a value written by a newer build than the one reading
// the log — and exactly the wrong thing to discover by reading it on a
// report screen. A reason added without its three catalogue entries
// would show a person the word "unguarded", in English, on a Serbian
// screen, and nothing would have failed.
func TestEveryBreakReasonReadsAsASentenceInEveryLocale(t *testing.T) {
	reasons := []audit.BreakReason{
		audit.BreakUnparseable,
		audit.BreakUnreachable,
		audit.BreakUnsound,
		audit.BreakUnguarded,
	}
	renderers := map[string]func(*i18n.Catalogue, audit.BreakReason) string{
		"the export summary": breakReasonText,
		"the audit window":   auditBreakReasonText,
	}

	// The rule can fail: a reason nothing knows about comes back as the
	// bare value, which is the first arm below. A check incapable of
	// failing passes forever (D-031).
	for where, render := range renderers {
		const unwired = audit.BreakReason("a-reason-nobody-wired-up")
		if got := render(i18n.Load("en"), unwired); got != string(unwired) {
			t.Fatalf("%s renders an unknown reason as %q, so this test cannot fail for the right reason", where, got)
		}
	}

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		for where, render := range renderers {
			for _, r := range reasons {
				got := render(c, r)
				switch {
				case got == string(r):
					t.Errorf("%s in %s renders %q as the bare enumerated value", where, locale, r)
				case strings.Contains(got, "settings.export_break"):
					t.Errorf("%s in %s renders %q as a missing catalogue key: %q", where, locale, r, got)
				case strings.TrimSpace(got) == "":
					t.Errorf("%s in %s renders %q as nothing at all", where, locale, r)
				}
			}
		}
	}
}
