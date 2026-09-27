//go:build linux

package ui

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

// The layout checks below read the page's own geometry after a payload has
// landed. Nothing is clicked or typed (D-094); they ask the page where it
// drew things. They load the real pages from Assets at the size the window
// is opened at, because the defect they exist for — a long output folder
// running the label, the path and the buttons into each other, and "Promeni…"
// cut short — was found by a person looking at a real window (D-372), as
// D-317's report screen was.

// longFolder is long enough to need several lines at 560 points, and made of
// one unbreakable run apart from the slashes, like a real deep path.
const longFolder = "/home/vboxuser/liro-bridge/cmd/liro-bridge/testdata/dokumenti-za-potpisivanje-2026/ugovori-i-aneksi/primljeno-od-klijenata"

// layoutProblems is evaluated in the page. It takes a list of element ids,
// ignores hidden ones, and reports every pair whose boxes overlap, a button
// whose content is clipped, anything reaching past the window's right edge,
// any of oneLine whose text wraps, and whether the document scrolls
// sideways.
const layoutProblems = `(function (ids, oneLine) {
  var out = [];
  var vw = document.documentElement.clientWidth;
  var els = ids.map(function (id) { return document.getElementById(id); })
               .filter(function (e) { return e && !e.hidden && e.getClientRects().length > 0; });
  els.forEach(function (e) {
    var r = e.getBoundingClientRect();
    if (e.scrollWidth > e.clientWidth + 1 || e.scrollHeight > e.clientHeight + 1) {
      if (e.tagName === "BUTTON") out.push(e.id + " is clipped (" + e.scrollWidth + "x" + e.scrollHeight + " in " + e.clientWidth + "x" + e.clientHeight + ")");
    }
    if (r.right > vw + 1) out.push(e.id + " reaches past the window: right " + Math.round(r.right) + " of " + vw);
    if (oneLine.indexOf(e.id) >= 0) {
      // Wrapped text fits its box, so nothing is clipped; what a person
      // reads is the words broken across lines in a squeezed column.
      var range = document.createRange();
      range.selectNodeContents(e);
      var tops = {};
      Array.prototype.forEach.call(range.getClientRects(), function (q) { if (q.width > 0) tops[Math.round(q.top)] = true; });
      var lines = Object.keys(tops).length;
      if (lines > 1) out.push(e.id + " wraps onto " + lines + " lines, " + Math.round(r.width) + " points wide");
    }
  });
  for (var i = 0; i < els.length; i++) {
    for (var j = i + 1; j < els.length; j++) {
      var a = els[i].getBoundingClientRect(), b = els[j].getBoundingClientRect();
      if (els[i].contains(els[j]) || els[j].contains(els[i])) continue;
      if (a.left < b.right - 1 && b.left < a.right - 1 && a.top < b.bottom - 1 && b.top < a.bottom - 1) {
        out.push(els[i].id + " overlaps " + els[j].id);
      }
    }
  }
  if (document.documentElement.scrollWidth > vw + 1) out.push("the page scrolls sideways: " + document.documentElement.scrollWidth + " in " + vw);
  return JSON.stringify(out);
})`

func evalLayoutProblems(t *testing.T, w Window, ids, oneLine []string) []string {
	t.Helper()
	arg, _ := json.Marshal(ids)
	one, _ := json.Marshal(oneLine)
	raw, err := w.Eval(layoutProblems + "(" + string(arg) + ", " + string(one) + ")")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	var inner string
	if err := json.Unmarshal([]byte(raw), &inner); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	var problems []string
	if err := json.Unmarshal([]byte(inner), &problems); err != nil {
		t.Fatalf("decoding %s: %v", inner, err)
	}
	return problems
}

func openRealPage(t *testing.T, page string, width, height int) Window {
	t.Helper()
	requireWebKitCanStart(t)
	assets, err := fs.Sub(Assets, "assets")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWindow(Options{
		Assets:      assets,
		VirtualHost: "liro.invalid",
		StartPage:   page,
		Width:       width,
		Height:      height,
	})
	if err != nil {
		t.Fatalf("NewWindow(%s): %v", page, err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// The documents step with a chosen output folder whose path is long: the
// label, the path and both buttons each keep their own room: nothing
// overlaps, and the label and the buttons each stay on one line — the path
// is the one thing that may wrap. 560x690 is stepDocumentsWidth/Height in
// cmd/liro-bridge. The labels are the Serbian ones, the longest the page
// is given; they are inputs here, not what is asserted.
//
// Three paths: a short one, the owner's (which collided in the owner's
// window), and a long one.
func TestALongOutputFolderDoesNotRunIntoItsButtons(t *testing.T) {
	for _, folder := range []string{"/home/vboxuser/Desktop", "/home/vboxuser/liro-bridge/cmd/liro-bridge", longFolder} {
		w := openRealPage(t, "/pages/main.html", 560, 690)
		if err := w.PostJSON(map[string]any{
			"type": "init",
			"strings": map[string]string{
				"main.output_label":  "Sačuvaj potpisane u",
				"main.output_beside": "Pored svakog dokumenta",
				"main.output_change": "Promeni...",
			},
			"files":              []any{},
			"outputFolderText":   folder,
			"outputFolderChosen": true,
			"primaryLabel":       "Dalje",
		}); err != nil {
			t.Fatalf("PostJSON: %v", err)
		}
		got, err := w.Eval("document.getElementById('output-folder').textContent")
		if err != nil || got != `"`+folder+`"` {
			t.Fatalf("the payload did not land: %s (%v)", got, err)
		}
		row := []string{"output-label", "output-folder", "output-beside-btn", "output-change-btn"}
		oneLine := []string{"output-label", "output-beside-btn", "output-change-btn"}
		if problems := evalLayoutProblems(t, w, row, oneLine); len(problems) > 0 {
			t.Errorf("the output-folder row with %s:\n  %s", folder, strings.Join(problems, "\n  "))
		}
		_ = w.Close()
	}
}

// What an export says, with a long folder, in both windows that have an
// Export button (Settings, 520x880; the audit log, 460x520 — settingswindow.go
// and auditlog.go): the sentence carries the whole folder, and the grid of
// written files sits under it. The owner asked for this to be checked
// rather than assumed after the output folder broke (D-372).
func TestALongExportFolderFitsInBothExportWindows(t *testing.T) {
	status := map[string]any{
		"type": "status",
		"status": map[string]any{
			"text":   "Dnevnik revizije je izvezen u " + longFolder,
			"intent": "positive",
			"files": []map[string]string{
				{"name": "liro-audit-20260927-141216.jsonl", "detail": "5 unosa, provera ispravnosti: u redu"},
				{"name": "liro-audit-20260927-141216-report.json", "detail": "izveštaj o proveri"},
			},
		},
	}
	for _, page := range []struct {
		path          string
		width, height int
	}{{"/pages/settings.html", 520, 880}, {"/pages/auditlog.html", 460, 520}} {
		w := openRealPage(t, page.path, page.width, page.height)
		if err := w.PostJSON(status); err != nil {
			t.Fatalf("%s: PostJSON: %v", page.path, err)
		}
		// The grid's cells have no ids of their own; give them some so the
		// same check can name them.
		got, err := w.Eval(`(function () {
  var cells = document.querySelectorAll("#action-status-files > span");
  cells.forEach(function (c, i) { c.id = "status-cell-" + i; });
  return cells.length;
})()`)
		if err != nil || got != "4" {
			t.Fatalf("%s: the status did not land: %s cells (%v)", page.path, got, err)
		}
		ids := []string{"action-status-text", "status-cell-0", "status-cell-1", "status-cell-2", "status-cell-3"}
		if problems := evalLayoutProblems(t, w, ids, []string{}); len(problems) > 0 {
			t.Errorf("%s with a long export folder:\n  %s", page.path, strings.Join(problems, "\n  "))
		}
		_ = w.Close()
	}
}
