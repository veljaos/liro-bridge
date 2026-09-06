//go:build windows

package main

// The remembered position: whose it is, where it is said, and what
// pressing the picker's three endings comes to.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
)

// TestTheRememberedPositionBelongsToTheFirstMethod is F6b §4 as the
// three-outcome screen says it: placing by looking at the page is one
// of the three methods rather than a fifth entry in a list of corners,
// and from Settings — which has no step after this one — that method is
// where a remembered position is changed or given back.
func TestTheRememberedPositionBelongsToTheFirstMethod(t *testing.T) {
	c := i18n.Load("sr-Latn")

	// A corner placement: the first method's own block is not on screen.
	cfg := config.Default()
	cfg.VisibleStamp = true
	win, _ := sharedStampWindow(t, c, cfg, stampRoleSettings)
	if got := evalString(t, win, `getComputedStyle(document.getElementById("stamp-placed")).display`); got != "none" {
		t.Errorf("the placed block is shown for a corner placement (display %q)", got)
	}

	// A saved position: the first method is what is chosen, and the
	// block under it names the position.
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = 3
	cfg.StampX, cfg.StampY = 393, 12
	win, messages := sharedStampWindow(t, c, cfg, stampRoleSettings)

	if got := evalString(t, win, `document.querySelector('input[name="stamp-method"]:checked').value`); got != stampMethodPlaced {
		t.Errorf("a saved position preselected method %q, want %q", got, stampMethodPlaced)
	}
	if got := evalString(t, win, `getComputedStyle(document.getElementById("stamp-placed")).display`); got == "none" {
		t.Fatal("a saved position is not shown at all")
	}
	for _, id := range []string{"stamp-place-btn", "stamp-placed-reset-btn"} {
		script := fmt.Sprintf(`(function(){var e=document.getElementById(%q);
			var r=e.getBoundingClientRect();
			return (r.width>0 && r.height>0 && e.textContent.length>0) ? "shown" : "no";})()`, id)
		if got := evalString(t, win, script); got != "shown" {
			t.Errorf("%s is %s", id, got)
		}
	}

	// Reset reports itself as a reset, not as a save — so Go can put the
	// corner back without treating it as the person finishing.
	drain(messages)
	if _, err := win.Eval(`document.getElementById("stamp-placed-reset-btn").click(); "ok"`); err != nil {
		t.Fatal(err)
	}
	<-messages
	form, err := readStampSettings(win)
	if err != nil {
		t.Fatal(err)
	}
	if form.Action != "reset" {
		t.Errorf("the reset button reported action %q, want reset", form.Action)
	}
	if form.Saved {
		t.Error("the reset button reported the form as saved")
	}

	// And the Place button reports itself as a place.
	drain(messages)
	if _, err := win.Eval(`document.getElementById("stamp-place-btn").click(); "ok"`); err != nil {
		t.Fatal(err)
	}
	<-messages
	form, err = readStampSettings(win)
	if err != nil {
		t.Fatal(err)
	}
	if form.Action != "place" {
		t.Errorf("the Place button reported action %q, want place", form.Action)
	}
}

// TestTheMethodScreenSaysNothingAboutCoordinates: someone choosing to
// place a stamp is about to place it, and being told the coordinates
// from last time is noise at the moment of deciding. The numbers are
// not under the option, in either role — and there is no longer a step
// between the choice and the picker for them to be on either.
//
// Where they are said is inside the picker, which is where they can be
// acted on: the same line is the button that puts the stamp back on the
// remembered position.
func TestTheMethodScreenSaysNothingAboutCoordinates(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = 3
	cfg.StampX, cfg.StampY = 393, 12

	for _, role := range []stampWindowRole{stampRoleStep, stampRoleSettings} {
		win, _ := sharedStampWindow(t, c, cfg, role)
		// The element that carried the line is gone from the method
		// screen, not merely emptied: an emptied one is one somebody
		// fills in again.
		if !evalBool(t, win, `document.getElementById("stamp-placed-summary") === null`) {
			t.Errorf("role %v: the method screen still carries the placed-position line", role)
		}
		// And the coordinate itself is nowhere on that screen. 393 is
		// the x; 12 would also match the margin note, which is a
		// sentence about every stamp rather than about this one.
		body := evalString(t, win, `document.getElementById("state-method").textContent`)
		if strings.Contains(body, "393") {
			t.Errorf("role %v: the method screen names the saved x coordinate: %q", role, body)
		}
		// The screen that carried them is gone entirely.
		if !evalBool(t, win, `document.getElementById("state-position") === null`) {
			t.Errorf("role %v: the position screen is still in the page", role)
		}
	}

	// The picker says them, because there they are both a reference and
	// somewhere to go back to.
	summary := placedSummaryIfSet(c, cfg)
	for _, part := range []string{"3", "393", "12"} {
		if !strings.Contains(summary, part) {
			t.Errorf("the picker's saved-position line %q does not name %q", summary, part)
		}
	}
	if got := placedSummaryIfSet(c, config.Default()); got != "" {
		t.Errorf("with nothing placed the picker still has a line to show: %q", got)
	}
}

// TestThePickerTitleIsThePositionOfTheSignature is Task 2 of this pass.
// The window is named for what it is about, in the words the step it
// replaced used, so a person recognises where they are.
func TestThePickerTitleIsThePositionOfTheSignature(t *testing.T) {
	for locale, want := range map[string]string{
		"sr-Latn": "Pozicija potpisa",
		"sr-Cyrl": "Позиција потписа",
		"en":      "Signature position",
	} {
		c := i18n.Load(locale)
		if got := c.T("place.title"); got != want {
			t.Errorf("%s: the picker is titled %q, want %q", locale, got, want)
		}
		// The step's own key went with the step, rather than being left
		// in the catalogue for someone to wire back.
		if got := c.T("stampwindow.position_step_title"); got != "stampwindow.position_step_title" {
			t.Errorf("%s: the deleted position-step title is still in the catalogue as %q", locale, got)
		}
	}
}

// TestResettingAPlacedPositionPutsTheCornerBack: the configuration
// after a reset carries a corner and nothing left over from the
// position that was there.
func TestResettingAPlacedPositionPutsTheCornerBack(t *testing.T) {
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = 4
	cfg.StampX, cfg.StampY = 100, 200

	// applyStampSettings keeps a placed position when the form still
	// names the first method and something is placed...
	kept := applyStampSettings(cfg, stampSettings{
		Method: stampMethodPlaced, Page: "first",
	})
	if kept.StampPosition != config.StampPositionCustom || kept.StampPlacedPage != 4 {
		t.Errorf("a placed position was lost by an ordinary save: %+v", kept)
	}

	// ...and a form that names a corner replaces it.
	corner := applyStampSettings(cfg, stampSettings{
		Method: stampMethodCorners, Position: "top-left", Page: "first",
	})
	if corner.StampPosition != "top-left" {
		t.Errorf("choosing a corner did not take: %q", corner.StampPosition)
	}

	// A configuration that says "custom" with nothing placed is not a
	// position; the first method leaves the corner underneath it alone
	// rather than writing a half-written answer to disk.
	half := config.Default()
	half.StampPosition = config.StampPositionCustom
	half.StampPlacedPage = 0
	if got := applyStampSettings(half, stampSettings{
		Method: stampMethodPlaced, Page: "first",
	}); got.StampPosition == config.StampPositionCustom {
		t.Error(`"custom" was accepted with no position stored`)
	}
	// And that is also what the window preselects: an option that
	// cannot be acted on is not the one to open on.
	if got := stampMethodOf(half); got == stampMethodPlaced {
		t.Error("a configuration with nothing placed still reports the placed method")
	}
}

// TestADocumentThatCannotBePreviewedFallsBackToTheCorners is F6b §5:
// a document the renderer cannot draw is not a document that cannot be
// signed. The placement window does not open, the four corners are what
// is left, and the window says why rather than appearing to have
// ignored the click.
func TestADocumentThatCannotBePreviewedFallsBackToTheCorners(t *testing.T) {
	// A file that is not a PDF at all: the strongest form of "cannot be
	// previewed", and the one that does not depend on which parts of the
	// format this renderer reads.
	path := filepath.Join(t.TempDir(), "not-a.pdf")
	if err := os.WriteFile(path, []byte("this is not a PDF"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	before := cfg

	got, placed, note := placeStamp(cfg, c, "sr-Latn", stampWindowDoc{path: path}, 0)
	if note == "" {
		t.Error("the window said nothing about a document it could not draw")
	}
	if placed {
		t.Error("a document that could not be drawn reported a position anyway")
	}
	if want := c.T("place.unavailable"); note != want {
		t.Errorf("the note is %q, want the catalogue's own %q", note, want)
	}
	if got.StampPosition != before.StampPosition || got.StampPlacedPage != 0 {
		t.Errorf("a failed preview changed the placement: %+v", got)
	}
}

// TestTheThreeEndingsOfThePicker is the piece of control flow a screen
// cannot show: what closing the placement picker comes to. All three
// endings, with the picker itself standing in — the real one needs a
// document, a window and a click, and the ending that matters most (a
// document that cannot be drawn) needs a document the renderer refuses.
//
// One of the three signs; the other two do not, and neither of them
// changes what is on disk. There is no step to come back to any more:
// the method screen is what the picker opened over, and it is still
// there behind it.
func TestTheThreeEndingsOfThePicker(t *testing.T) {
	base := config.Default()
	base.VisibleStamp = true
	base.StampPosition = consent.StampPositionTopLeft

	t.Run("a position chosen signs", func(t *testing.T) {
		out := placementResult(base, func(in config.Config) (config.Config, bool, string) {
			in.StampPosition = config.StampPositionCustom
			in.StampPlacedPage, in.StampX, in.StampY = 2, 100, 200
			return in, true, ""
		})
		if !out.sign {
			t.Fatal("a position was chosen and nothing was signed")
		}
		if out.cfg.StampPosition != config.StampPositionCustom || out.cfg.StampPlacedPage != 2 {
			t.Fatalf("the chosen position did not reach the configuration: %+v", out.cfg)
		}
		if out.method != stampMethodPlaced {
			t.Errorf("the method afterwards is %q, want the one that was chosen", out.method)
		}
		if out.note != "" {
			t.Errorf("choosing a position said something: %q", out.note)
		}
	})

	t.Run("cancelling signs nothing and changes nothing", func(t *testing.T) {
		out := placementResult(base, func(in config.Config) (config.Config, bool, string) {
			return in, false, ""
		})
		if out.sign {
			t.Fatal("closing the picker signed the batch anyway")
		}
		if out.method != stampMethodPlaced {
			t.Errorf("the method screen came back showing %q, want the method that was chosen", out.method)
		}
		if out.note != "" {
			t.Errorf("cancelling said something: %q", out.note)
		}
		if out.cfg != base {
			t.Errorf("cancelling changed the configuration: %+v", out.cfg)
		}
	})

	t.Run("a document that cannot be drawn offers the corners", func(t *testing.T) {
		out := placementResult(base, func(in config.Config) (config.Config, bool, string) {
			return in, false, "cannot be previewed"
		})
		if out.sign {
			t.Fatal("a document that could not be drawn was signed anyway")
		}
		if out.method != stampMethodCorners {
			t.Errorf("the screen came back showing %q, want the corners", out.method)
		}
		if out.note == "" {
			t.Error("the screen said nothing about why the corners are what is left")
		}
		// F6b §5: not a document that cannot be signed, and nothing
		// remembered is thrown away for it.
		if out.cfg != base {
			t.Errorf("a failed preview changed the configuration: %+v", out.cfg)
		}
	})
}
