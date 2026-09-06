//go:build windows

package main

// Task 1 (F5 second-real-run review): SPEC §12.8's choice, offered in
// the consent window instead of the batch simply failing with
// TSA_UNAVAILABLE. These are the Go-side rules and the real-window
// round trip; docs/decisions.md records what was established about the
// two authorities the settings window offers as presets.
import (
	"errors"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/ui"
)

func TestIsTSAFailureRecognisesBothTimestampCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"unavailable", errs.New(errs.CodeTSAUnavailable, errors.New("no answer")), true},
		{"rejected", errs.New(errs.CodeTSARejected, errors.New("refused")), true},
		{"card not present", errs.New(errs.CodeCardNotPresent, errors.New("no card")), false},
		{"a plain error", errors.New("output file already exists"), false},
		{"no error", nil, false},
	}
	for _, tc := range cases {
		if got := isTSAFailure(tc.err); got != tc.want {
			t.Errorf("%s: isTSAFailure = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestLowerLevelReportsTheWeakestLevelReached: a batch where one
// document lost its timestamp is a B-B batch as far as the report and
// the audit entry are concerned. Claiming the strongest level any
// document reached would be exactly the overclaim SPEC §18.11 forbids.
func TestLowerLevelReportsTheWeakestLevelReached(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"", string(pades.LevelBLT), string(pades.LevelBLT)},
		{string(pades.LevelBLT), "", string(pades.LevelBLT)},
		{string(pades.LevelBLT), string(pades.LevelBB), string(pades.LevelBB)},
		{string(pades.LevelBB), string(pades.LevelBLT), string(pades.LevelBB)},
		{string(pades.LevelBLT), string(pades.LevelBT), string(pades.LevelBT)},
		{string(pades.LevelBT), string(pades.LevelBT), string(pades.LevelBT)},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := lowerLevel(tc.a, tc.b); got != tc.want {
			t.Errorf("lowerLevel(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestAchievedLevelMarksBB proves B-B is never left to read as just
// another level: it is coloured in the warning intent family (never a
// colour chosen here - D-093) and carries its own sentence beside it.
func TestAchievedLevelMarksBB(t *testing.T) {
	c := i18n.Load("en")

	if got := achievedLevelIntent(string(pades.LevelBB)); got != string(ui.IntentWarning) {
		t.Errorf("B-B intent = %q, want %q", got, ui.IntentWarning)
	}
	if got := achievedLevelNote(c, string(pades.LevelBB)); got != c.T("consent.level_bb") {
		t.Errorf("B-B note = %q, want the catalogue's own %q", got, c.T("consent.level_bb"))
	}

	for _, level := range []pades.Level{pades.LevelBT, pades.LevelBLT} {
		if got := achievedLevelIntent(string(level)); got != string(ui.IntentPositive) {
			t.Errorf("%s intent = %q, want %q", level, got, ui.IntentPositive)
		}
		if got := achievedLevelNote(c, string(level)); got != "" {
			t.Errorf("%s carried a warning of its own: %q", level, got)
		}
	}

	if got := achievedLevelIntent(""); got != "" {
		t.Errorf("no level rendered intent %q, want none rather than a guess", got)
	}
}

// TestAuditEntryRecordsTheAchievedLevel proves the level survives a
// round trip through the store, and that the chain still verifies with
// it — the audit log is where a user finds out weeks later that a
// document was saved without a timestamp.
func TestAuditEntryRecordsTheAchievedLevel(t *testing.T) {
	store, err := audit.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	recordInteractiveAudit(store, nil, "AABB", 2, audit.OutcomeApproved, nil, false, string(pades.LevelBB))
	recordInteractiveAudit(store, nil, "AABB", 1, audit.OutcomeDenied, nil, false, "")

	entries, err := store.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("store holds %d entries, want 2", len(entries))
	}
	if entries[0].AchievedLevel != string(pades.LevelBB) {
		t.Errorf("first entry AchievedLevel = %q, want %q", entries[0].AchievedLevel, pades.LevelBB)
	}
	if entries[1].AchievedLevel != "" {
		t.Errorf("a denied batch recorded level %q, want none", entries[1].AchievedLevel)
	}
	if result := audit.Verify(entries); !result.OK {
		t.Fatalf("chain verification failed at entry %d", result.BrokenAt)
	}
}

// TestTSAChoiceScreenRoundTrips is the real-window half: the choice
// screen renders its three actions with real text, and each of the two
// proceeding actions reaches Go as an approve whose recorded choice
// Window.Eval reads back — the page->Go message surface staying at
// exactly three types (D-083).
//
// It is asked on the page that carries the progress and the report,
// because that is where the flow is by the time it is asked: after the
// approval and before the card.
func TestTSAChoiceScreenRoundTrips(t *testing.T) {
	c := i18n.Load("sr-Latn")
	m, messages := testMainWindow(t, "sr-Latn", config.Default(), nil)
	win := m.win

	if err := win.PostJSON(askTSAChoicePayload(consent.TSAReasonNotConfigured, c)); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if hidden := evalString(t, win, "String(document.getElementById('state-tsachoice').hidden)"); hidden != "false" {
		t.Fatalf("the timestamp choice screen is still hidden after being posted")
	}
	if got := evalString(t, win, "document.getElementById('tsa-reason').textContent"); got != c.T("consent.tsa_reason_not_configured") {
		t.Errorf("reason rendered %q, want %q", got, c.T("consent.tsa_reason_not_configured"))
	}
	for _, id := range []string{"tsa-without-btn", "tsa-configure-btn", "tsa-cancel-btn"} {
		if got := evalString(t, win, "document.getElementById('"+id+"').textContent"); got == "" {
			t.Errorf("%s rendered no label at all", id)
		}
	}

	for _, tc := range []struct{ id, want string }{
		{"tsa-without-btn", "withoutTimestamp"},
		{"tsa-configure-btn", "configure"},
	} {
		if _, err := win.Eval("document.getElementById('" + tc.id + "').click()"); err != nil {
			t.Fatalf("Eval(click %s): %v", tc.id, err)
		}
		if msg := recvMessage(t, messages, 5*time.Second); msg.Type != ui.MessageTypeApprove {
			t.Fatalf("clicking %s sent %v, want approve", tc.id, msg.Type)
		}
		if got := readTSAChoice(win); got != tc.want {
			t.Errorf("after clicking %s, readTSAChoice = %q, want %q", tc.id, got, tc.want)
		}
	}

	if _, err := win.Eval("document.getElementById('tsa-cancel-btn').click()"); err != nil {
		t.Fatalf("Eval(click tsa-cancel-btn): %v", err)
	}
	if msg := recvMessage(t, messages, 5*time.Second); msg.Type != ui.MessageTypeCancel {
		t.Fatalf("clicking Cancel sent %v, want cancel", msg.Type)
	}

	// Re-entering the screen clears the previous answer: an approve that
	// arrives with no fresh choice must never replay the last one.
	if err := win.PostJSON(askTSAChoicePayload(consent.TSAReasonUnreachable, c)); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if got := readTSAChoice(win); got != "" {
		t.Errorf("a freshly shown choice screen already reports %q", got)
	}
	if got := evalString(t, win, "document.getElementById('tsa-reason').textContent"); got != c.T("consent.tsa_reason_unreachable") {
		t.Errorf("reason rendered %q, want the unreachable one", got)
	}
}

// TestTheReportStatesTheAchievedLevel proves the level a batch reached
// is on screen, in words, not merely absent from the claims — SPEC
// §12.8's "visibly marked as such". The report is where a finished
// batch says it now; there is no separate done screen on a separate
// window any more.
func TestTheReportStatesTheAchievedLevel(t *testing.T) {
	c := i18n.Load("sr-Latn")
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)

	m.postReport(jobs.Report{Succeeded: 1, OutputDir: `C:\docs`, AchievedLevel: string(pades.LevelBB)})
	if got := evalString(t, m.win, "document.getElementById('report-level').textContent"); got != string(pades.LevelBB) {
		t.Errorf("report level = %q, want %q", got, pades.LevelBB)
	}
	if got := evalString(t, m.win, "document.getElementById('report-level').className"); got != "liro-text-small liro-outcome liro-outcome-warning" {
		t.Errorf("report level class = %q, want the warning intent", got)
	}
	if got := evalString(t, m.win, "document.getElementById('report-level-note').textContent"); got != c.T("consent.level_bb") {
		t.Errorf("report level note = %q, want %q", got, c.T("consent.level_bb"))
	}

	// A timestamped batch says its level positively and needs no
	// sentence of its own.
	m.postReport(jobs.Report{Succeeded: 1, OutputDir: `C:\docs`, AchievedLevel: string(pades.LevelBLT)})
	if got := evalString(t, m.win, "String(document.getElementById('report-level-note').hidden)"); got != "true" {
		t.Error("a timestamped batch still carried the no-timestamp warning")
	}
}
