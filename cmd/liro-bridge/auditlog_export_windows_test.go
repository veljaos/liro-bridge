//go:build windows

package main

// Task 5 of the first-use fix pass: the audit log window can export
// what it is showing.
//
// Settings has had an Export since F5, and it is the same function; the
// point is where the button is. A person looking at the log and
// deciding they want to keep it should not have to close the window,
// open Settings, and find the same action there.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestAuditLogWindowOffersExportBesideClose checks the window a person
// actually looks at: the button is there, in their language, beside
// Close, and pressing it reaches Go.
//
// What it deliberately does not do is press OK in the folder chooser.
// That is a native modal dialog and D-094 forbids simulating the click,
// so the last step — files on disk, destination named on screen — is
// the owner's to confirm. What is testable is everything up to it, plus
// the status line that reports the result, which is exercised directly
// below.
func TestAuditLogWindowOffersExportBesideClose(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		win, messages := sharedAuditLogWindowWithMessages(t)
		if err := win.PostJSON(buildAuditLogInit(c, twentyAuditEntries())); err != nil {
			t.Fatalf("%s: PostJSON: %v", locale, err)
		}

		label := evalText(t, win, "document.getElementById('export-btn').textContent")
		if want := c.T("auditwindow.export"); label != want {
			t.Fatalf("%s: the export button reads %q, want %q", locale, label, want)
		}
		if label == "auditwindow.export" {
			t.Fatalf("%s: the export button is showing its own key", locale)
		}
		// Beside Close, in the same action row, and both on screen.
		row := evalNumber(t, win,
			"document.getElementById('export-btn').parentElement === document.getElementById('close-btn').parentElement ? 1 : 0")
		if row != 1 {
			t.Fatalf("%s: Export is not in the same action row as Close", locale)
		}
		for _, id := range []string{"export-btn", "close-btn"} {
			bottom := evalNumber(t, win, "document.getElementById('"+id+"').getBoundingClientRect().bottom")
			if height := evalNumber(t, win, "window.innerHeight"); bottom > height {
				t.Fatalf("%s: %s sits at %v, past the window's %v", locale, id, bottom, height)
			}
		}

		if _, err := win.Eval("document.getElementById('export-btn').click()"); err != nil {
			t.Fatalf("%s: clicking Export: %v", locale, err)
		}
		select {
		case msg := <-messages:
			// One action in this window, so approve is unambiguous and
			// the three-message surface is untouched (D-083).
			if msg.Type != ui.MessageTypeApprove {
				t.Fatalf("%s: Export sent %v, want approve", locale, msg.Type)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: Export never reached Go", locale)
		}
	}
}

// TestAuditLogWindowSaysWhereTheExportWent: the same status line
// Settings uses, in the window that now shares its Export. A button
// that runs and says nothing is the defect D-097 fixed once already.
func TestAuditLogWindowSaysWhereTheExportWent(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := sharedAuditLogWindowWithMessages(t)
	if err := win.PostJSON(buildAuditLogInit(c, twentyAuditEntries())); err != nil {
		t.Fatal(err)
	}
	if !evalBool(t, win, "document.getElementById('action-status').hidden") {
		t.Fatal("a freshly opened window is already reporting something")
	}

	const target = `C:\Users\Veljko\Desktop\Izvoz`
	postWindowStatus(win, fmt.Sprintf(c.T("settings.export_done"), target), ui.IntentPositive)

	if evalBool(t, win, "document.getElementById('action-status').hidden") {
		t.Fatal("the status line is still hidden after an export reported itself")
	}
	text := evalText(t, win, "document.getElementById('action-status').textContent")
	if !strings.Contains(text, target) {
		t.Fatalf("the window does not say where the files went: %q", text)
	}
	if h := evalNumber(t, win, "document.getElementById('action-status').getBoundingClientRect().height"); h <= 0 {
		t.Fatalf("the status line has no height on screen")
	}
	class := evalText(t, win, "document.getElementById('action-status').className")
	if !strings.Contains(class, "liro-outcome-"+string(ui.IntentPositive)) {
		t.Fatalf("the status line takes no colour from its intent family: class %q", class)
	}
	assertNoPageScroll(t, win)
}
