//go:build windows

package main

import (
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

// longOutputDir is a path of the shape that found this defect: a temporary
// directory with a session identifier in it, from a run started by hand.
//
// It is longer than a path most people see, and that is the point rather than a
// weakness — it is the shape of the *value*, not its frequency, that decides
// whether a layout holds, and a person signing from a deep folder on a network
// share gets the same thing. It is also unbreakable in the places that matter:
// a Windows separator is not a wrapping opportunity, so nothing short of
// overflow-wrap will keep it inside its box.
const longOutputDir = `C:\Users\Veljko\AppData\Local\Temp\claude\` +
	`C--Users-Veljko-Desktop-liro-bridge\9aef24e7-05d3-4cec-ac6e-a03ec7ace959\` +
	`scratchpad\look\potpisani-dokumenti`

// TestTheReportsLabelsAndValuesDoNotOverlapWithALongPath is the defect the
// owner photographed, measured.
//
// Two separate failures were visible on one screen and this asserts both,
// because they have one cause between them — .liro-row is built for a row with
// a control on its right, and these rows hold two pieces of one sentence:
//
//   - **The overlap.** The path drew on top of "Sačuvano u", appearing to start
//     underneath the label rather than after it. min-width:0 let the value's box
//     shrink to nothing and nothing told the text to break, so it overflowed.
//   - **The chasm.** "Nivo potpisa" and "B-B" sat at opposite ends of the
//     window, where they stop reading as a label and its value at all.
//
// # Why no layout test saw it
//
// This screen had never been rendered with a long path in it. The suite's own
// long-value test (TestMainWindowLongNameDoesNotWidenTheWindow) exercises a
// long document *name* on the documents step, and its comment says outright
// that the long *path* case "is exercised where paths are handled, in
// internal/jobs" — which is true and is about jobs, not about a screen. The
// report screen takes a path, displays it, and nothing had ever given it one
// that was long.
//
// It is D-279's family: every test passed and a photograph caught it.
func TestTheReportsLabelsAndValuesDoNotOverlapWithALongPath(t *testing.T) {
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})

	// B-B on purpose: the level whose value is short, which is where the
	// opposite-ends failure is most visible, and which also puts the
	// explanatory note underneath (SPEC §12.8) so the row is measured in the
	// company it actually keeps.
	m.postReport(jobs.Report{Succeeded: 3, OutputDir: longOutputDir, AchievedLevel: "B-B"})

	if out := evalText(t, m.win, "document.getElementById('report-output').textContent"); !strings.Contains(out, "potpisani-dokumenti") {
		t.Fatalf("the report is not showing the long path at all, so this test measures nothing: %q", out)
	}

	for _, row := range []struct{ label, value string }{
		{"main.report_output_label", "report-output"},
		{"main.report_level_label", "report-level"},
	} {
		labelSel := "document.querySelector('[data-i18n=\"" + row.label + "\"]')"
		valueSel := "document.getElementById('" + row.value + "')"

		// One round trip for every number, because a page that relaid out
		// between two calls answers the second question about a different page
		// (D-201, and the comment on evalNumbers).
		n := evalNumbers(t, m.win,
			labelSel+".getBoundingClientRect().left",
			labelSel+".getBoundingClientRect().right",
			valueSel+".getBoundingClientRect().left",
			valueSel+".getBoundingClientRect().right",
			valueSel+".parentElement.getBoundingClientRect().left",
			valueSel+".parentElement.getBoundingClientRect().right",
		)
		labelLeft, labelRight, valueLeft, valueRight := n[0], n[1], n[2], n[3]
		rowLeft, rowRight := n[4], n[5]
		rowWidth := rowRight - rowLeft

		// Content wider than the box it is in, which is what text drawn on
		// top of a neighbour looks like from here. It has never been seen to
		// fire on this screen — #report-output and #report-level have carried
		// overflow-wrap:anywhere since before this test — and it is kept
		// because the rule it guards is one line away from being deleted by
		// somebody tidying up, and because the owner photographed something
		// this has not yet explained (see the entry).
		if evalBool(t, m.win, valueSel+".scrollWidth > "+valueSel+".clientWidth + 1") {
			over := evalNumbers(t, m.win, valueSel+".scrollWidth", valueSel+".clientWidth")
			t.Errorf("%s: its content is %.0f wide in a box %.0f wide, so it is drawn "+
				"outside its own box — which is the path appearing on top of the label",
				row.value, over[0], over[1])
		}

		// Sub-pixel tolerance: these are fractional CSS pixels and two boxes
		// that abut can differ in the last decimal.
		if valueLeft < labelRight-0.5 {
			t.Errorf("%s: the value starts at %.1f, before the label ends at %.1f — "+
				"they overlap, which is what the path drawing on top of the label looks like",
				row.value, valueLeft, labelRight)
		}
		if gap := valueLeft - labelRight; gap > rowWidth/4 {
			t.Errorf("%s: %.1f points of empty space between the label and its value, "+
				"in a row %.1f wide. They are at opposite ends and do not read as a "+
				"label and its value.", row.value, gap, rowWidth)
		}
		// Against the row's own edges, not its width: these are viewport
		// coordinates, and comparing one against the other is how the first
		// version of this assertion failed on a layout that was correct.
		if labelLeft < rowLeft-0.5 || valueRight > rowRight+0.5 {
			t.Errorf("%s: the row's contents are outside it — label starts at %.1f, "+
				"value ends at %.1f, row spans %.1f to %.1f",
				row.value, labelLeft, valueRight, rowLeft, rowRight)
		}
	}

	// And the whole reason a long value is dangerous: it must wrap rather than
	// push the window wider (D-096).
	assertNoPageScroll(t, m.win)
}
