//go:build windows

package main

// The signing method: one choice with three outcomes.
//
// It is a step of the signing window (signflow_windows.go) and, in the
// same page and the same words, Settings' way of setting a standing
// preference. This file is what the two share: the vocabulary of the
// three methods, the payload the page renders itself from, and the
// window Settings opens.
//
// The method is one choice with three outcomes rather than three
// nested questions. It used to ask whether to add a stamp, then which
// corner, then — once the placement window existed — whether to place
// it by looking at the page instead; three decisions to make one, with
// a checkbox at the top that changed which controls existed below it.
// The three methods are the whole answer:
//
//	placed   sign, choosing where the signature goes — the placement
//	         window opens on the batch's first document
//	corners  sign using a set position — the four corners appear
//	         inline under the option that needs them
//	none     sign with nothing drawn on the page at all
//
// The method is not a new field: it is what the configuration already
// says. An invisible signature is "none"; a placed position is
// "placed"; anything else is a corner. So the method a person used
// last time is the one preselected this time, and a returning user
// confirms rather than decides — with no fourth thing on disk that can
// disagree with the three that were already there.
//
// The same window is also Settings' way in, where the answer is a
// standing preference rather than the last step before signing: it
// shows the same three methods with the same words, and adds what a
// preference owns and a batch does not — which page, a reference line,
// the identity document number, and the two buttons that change or
// give back a remembered position.

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// The size of Settings' way in. The same page as a step of the flow,
// at a different size, because it carries more: the standing
// preferences — which page, the reference line, the identity-document
// toggle and the margin note — as well as the three methods.
//
// Measured in a real window in sr-Cyrl, the longest of the three
// catalogues, with the window in its *fullest* state: the second method
// chosen, since the four corners are taller than anything the other two
// reveal. A window sized for its emptiest state scrolls the moment
// somebody uses it, which is the defect D-106 went looking for.
//
// Settings' form needs 597 points; the rest is the chrome above and
// below it plus about ten points of headroom, so a one-word label
// change does not immediately put the scrollbar back. The step's own
// size lives with the other steps (signflow_windows.go).
const (
	stampWindowWidth    = 440
	stampSettingsHeight = 775
)

// stampWindowRole says which of the page's two callers built its
// payload, which decides what it shows beyond the three methods and
// what its primary action says.
type stampWindowRole int

const (
	// stampRoleStep is a step of the signing flow: the primary action
	// signs.
	stampRoleStep stampWindowRole = iota
	// stampRoleSettings is Settings' way in: the primary action saves.
	stampRoleSettings
)

// The three signing methods: one choice, three outcomes. They are the
// vocabulary of the screen and of the form it reports back, not a
// fourth thing stored on disk — stampMethodOf derives them from the
// configuration that already exists.
const (
	// stampMethodPlaced signs by choosing where the signature goes: the
	// picker opens on the batch's first document, and using a position
	// from it signs.
	stampMethodPlaced = "placed"
	// stampMethodCorners signs at one of SPEC §13.1's four corners.
	stampMethodCorners = "corners"
	// stampMethodNone signs with nothing drawn on the page at all.
	stampMethodNone = "none"
)

// stampMethodOf is which of the three a configuration already means.
//
// This is why the method step needs no new setting to preselect the
// method a person used last: the answer is already on disk, in the two fields
// that were always there. A configuration claiming a placed position
// with nothing actually placed is not one — config.validate replaces
// it on load, and this agrees with it rather than showing an option
// that cannot be acted on.
func stampMethodOf(cfg config.Config) string {
	switch {
	case !cfg.VisibleStamp:
		return stampMethodNone
	case cfg.StampPosition == config.StampPositionCustom && cfg.StampPlacedPage >= 1:
		return stampMethodPlaced
	default:
		return stampMethodCorners
	}
}

// stampSettings is the window's whole form, read back in one call.
type stampSettings struct {
	Saved bool `json:"saved"`

	// Action says what the click meant: "save" the form, "place" the
	// stamp by looking at the page, or "reset" a placed position back to
	// a corner. Three buttons, one message type, and what the click
	// meant reported explicitly rather than inferred (D-095).
	Action string `json:"action"`

	// Method is one of the three constants above: the whole of what
	// the method step asks. Position is the corner it means when Method is
	// stampMethodCorners, and is ignored otherwise — a corner chosen
	// before switching to another method is remembered rather than
	// thrown away, so switching back does not lose it.
	Method         string `json:"method"`
	Position       string `json:"position"`
	Page           string `json:"page"`
	Reference      string `json:"reference"`
	ShowDocumentID bool   `json:"showDocumentID"`
}

// stampWindowDoc is what the placement window needs to show a real
// page: a document to draw and the certificate whose name goes on the
// stamp. The signing flow has both — the batch's first document, and
// the certificate chosen on the step before the method. Settings has
// neither until it asks for one.
type stampWindowDoc struct {
	path string
	cert *x509.Certificate
}

type jsOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// runStampWindow opens Settings' way in and blocks until it is
// answered. It returns the configuration as it stands afterwards and
// whether the person saved.
//
// The answer is saved to disk on the way out, so the next batch starts
// from it — a returning user accepts what is already there and moves
// on, which is the whole point of persisting it (D-103's reasoning,
// unchanged: this is a preference about how one's own documents look,
// not a per-batch security decision, and SPEC §18.15's rule against a
// remembered certificate does not reach it).
//
// owner is the settings window this one is opened from. It is never
// zero in production: a window opened from another window that is not
// owned by it can be created underneath it, which is exactly what made
// the whole program stop responding (D-129).
func runStampWindow(cfg config.Config, locale string, owner uintptr) (config.Config, bool) {
	c := i18n.Load(locale)
	messages := make(chan ui.Message, 8)

	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("stampwindow.title"),
		Width:       stampWindowWidth,
		Height:      stampSettingsHeight,
		Owner:       owner,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   pageStamp,
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		slog.Warn("stamp window: could not open", "error", err)
		return cfg, false
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(buildStampInit(c, cfg, stampRoleSettings, stampMethodOf(cfg))); err != nil {
		slog.Warn("stamp window: could not post the form", "error", err)
		return cfg, false
	}

	// repost redraws the form with an explicit chosen method, which is
	// not always the one the configuration would imply: a document that
	// could not be previewed leaves the remembered position untouched
	// on disk while the window has to come back showing the corners.
	repost := func(method string) bool {
		if err := win.PostJSON(buildStampInit(c, cfg, stampRoleSettings, method)); err != nil {
			slog.Warn("stamp window: could not repost the form", "error", err)
			return false
		}
		return true
	}

	for {
		msg := <-messages
		if msg.Type != ui.MessageTypeApprove {
			return cfg, false
		}

		form, err := readStampSettings(win)
		if err != nil {
			slog.Warn("stamp window: reading the form failed", "error", err)
			return cfg, false
		}

		switch form.Action {
		case "place":
			// Settings' own Place button. The form's other answers are
			// folded in first, so a reference typed just before pressing
			// it is not lost while the placement window is open.
			cfg = applyStampSettings(cfg, form)
			next, placed, note := placeStamp(cfg, c, locale, stampWindowDoc{}, win.Handle())
			cfg = next
			method := stampMethodPlaced
			if note != "" {
				// F6b §5: a document that cannot be previewed is not a
				// document that cannot be signed. The corners are still
				// there, and this says why they are what is left.
				method = stampMethodCorners
			} else if !placed {
				method = stampMethodOf(cfg)
			}
			if !repost(method) {
				return cfg, false
			}
			if note != "" {
				postWindowStatus(win, note, ui.IntentWarning)
			}
			continue
		case "reset":
			cfg = applyStampSettings(cfg, form)
			cfg.StampPosition = consent.StampPositionBottomRight
			cfg.StampPlacedPage, cfg.StampX, cfg.StampY = 0, 0, 0
			if !repost(stampMethodCorners) {
				return cfg, false
			}
			continue
		}

		if !form.Saved {
			return cfg, false
		}
		cfg = applyStampSettings(cfg, form)

		if err := config.Save(config.DefaultPath(), cfg); err != nil {
			slog.Warn("stamp window: saving the choice failed", "error", err)
		}
		return cfg, true
	}
}

// placeStamp opens the placement window and folds its answer into the
// configuration. Cancelling it changes nothing at all — not the
// position, and not whether a placed position is in use.
//
// placed reports whether a position was actually chosen, which is a
// different question from whether note is empty: a cancelled window
// says nothing and changes nothing, while a document that cannot be
// drawn at all says so and leaves the corners as what is left.
//
// Settings has no document of its own to draw, so it asks for one. That
// is not a detour: the point of setting a position from Settings is to
// line the stamp up against a document of the shape you always sign,
// and there is nothing to line it up against otherwise.
func placeStamp(cfg config.Config, c *i18n.Catalogue, locale string, doc stampWindowDoc, owner uintptr) (_ config.Config, placed bool, note string) {
	path := doc.path
	if path == "" {
		chosen, ok, err := ui.ChooseFiles(owner, c.T("place.title"),
			c.T("main.file_filter_pdf"), c.T("main.file_filter_all"))
		if err != nil {
			slog.Warn("stamp window: the file chooser failed", "error", err)
			return cfg, false, ""
		}
		if !ok || len(chosen) == 0 {
			return cfg, false, ""
		}
		path = chosen[0]
	}

	saved, ok, previewable := runPlacementWindow(cfg, locale, path, doc.cert, owner)
	if !previewable {
		return cfg, false, c.T("place.unavailable")
	}
	if !ok {
		return cfg, false, ""
	}
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = saved.Page
	cfg.StampX, cfg.StampY = saved.X, saved.Y
	return cfg, true, ""
}

// buildStampInit is the payload the page renders itself from. Every
// label is resolved here, in Go, so the page never holds a catalogue
// (F5 §10).
//
// method is passed rather than derived so that the window can be
// redrawn showing a method the configuration does not (yet) hold — the
// case that matters is a document the renderer cannot draw, where the
// remembered position stays on disk and the corners are what the person
// is being offered instead.
func buildStampInit(c *i18n.Catalogue, cfg config.Config, role stampWindowRole, method string) map[string]any {
	keys := []string{
		"stampwindow.title",
		"stampwindow.method_placed", "stampwindow.method_corners",
		"stampwindow.method_none",
		"stampwindow.position_label",
		"stampwindow.place_button", "stampwindow.placed_reset",
		"stampwindow.page_label", "stampwindow.page_number_label",
		"stampwindow.reference_label", "stampwindow.reference_hint",
		"stampwindow.show_document_id", "stampwindow.show_document_id_hint",
		"stampwindow.margin_note", "stampwindow.sign",
	}
	strings := make(map[string]string, len(keys))
	for _, k := range keys {
		strings[k] = c.T(k)
	}

	primary, secondary := c.T("stampwindow.save"), c.T("stampwindow.cancel")
	// The method step carries no subtitle. It had one — "the last step,
	// everything else is already decided" — which said what the person
	// could already see, on a screen whose whole value is being small.
	// Settings keeps its own, because there this is a preference rather
	// than a step and the line says which preference.
	subtitle := c.T("stampwindow.subtitle_settings")
	if role == stampRoleStep {
		// The primary's words are overwritten by the step that posts
		// this (signflow_windows.go), which knows whether anything
		// follows this screen. The way back is the step header, so the
		// secondary is what it has always been on a screen that can be
		// abandoned — Cancel.
		primary, secondary = c.T("stampwindow.sign"), c.T("stampwindow.cancel")
		subtitle = ""
	}

	return map[string]any{
		"type":           "init",
		"strings":        strings,
		"primaryLabel":   primary,
		"secondaryLabel": secondary,
		"subtitle":       subtitle,
		// The standing preferences are Settings' business; a step of
		// signing does not show them at all.
		"showMore": role == stampRoleSettings,
		"positions": []jsOption{
			// The four corners in the order they sit on a page, so the
			// two-by-two grid the page draws reads as a page rather than
			// as a list: top row first, left before right.
			{Value: consent.StampPositionTopLeft, Label: c.T("stampwindow.position_top_left")},
			{Value: consent.StampPositionTopRight, Label: c.T("stampwindow.position_top_right")},
			{Value: consent.StampPositionBottomLeft, Label: c.T("stampwindow.position_bottom_left")},
			{Value: consent.StampPositionBottomRight, Label: c.T("stampwindow.position_bottom_right")},
		},
		"pages": []jsOption{
			{Value: config.StampPageFirst, Label: c.T("stampwindow.page_first")},
			{Value: config.StampPageLast, Label: c.T("stampwindow.page_last")},
			{Value: "number", Label: c.T("stampwindow.page_number")},
		},
		"model": map[string]any{
			"method":         method,
			"position":       stampCornerOf(cfg),
			"page":           cfg.StampPage,
			"reference":      cfg.StampReference,
			"showDocumentID": cfg.StampShowDocumentID,
		},
	}
}

// stampCornerOf is which of the four corners the window should show as
// chosen. A configuration holding a placed position, or one whose
// position never was a corner, still has to offer a sensible one the
// moment somebody picks the second method — SPEC §13.1's own default.
func stampCornerOf(cfg config.Config) string {
	if consent.ValidStampPosition(cfg.StampPosition) {
		return cfg.StampPosition
	}
	return consent.StampPositionBottomRight
}

// applyStampSettings folds the form onto cfg, validating every value
// the page could have produced. The page is this project's own, but it
// is still the outside of a boundary: a method, a position or a page it
// does not recognise falls back rather than being written to disk for
// config.Load to complain about on the next run.
//
// The three methods land on the two fields that already existed:
// "none" turns the stamp off and leaves the corner alone, so switching
// back restores it; "corners" writes the chosen corner; "placed" writes
// the custom marker, but only once something has actually been placed —
// a position nobody has chosen is not a position, and config.validate
// would replace it on the next load anyway.
func applyStampSettings(cfg config.Config, form stampSettings) config.Config {
	switch form.Method {
	case stampMethodNone:
		cfg.VisibleStamp = false
	case stampMethodPlaced:
		cfg.VisibleStamp = true
		if cfg.StampPlacedPage >= 1 {
			cfg.StampPosition = config.StampPositionCustom
		} else {
			// Nothing has been placed yet, so there is no position to
			// point at. The corner underneath it stands rather than
			// half an answer reaching disk for config.Load to replace
			// on the next run.
			cfg.StampPosition = stampCornerOf(cfg)
		}
	case stampMethodCorners:
		cfg.VisibleStamp = true
		cfg.StampPosition = consent.StampChoice{
			Visible: true, Position: form.Position,
		}.Normalised().Position
	default:
		slog.Warn("stamp window: unrecognised signing method, keeping the current one", "value", form.Method)
	}

	if config.ValidStampPage(form.Page) {
		cfg.StampPage = form.Page
	} else {
		slog.Warn("stamp window: unrecognised page selection, keeping the current one", "value", form.Page)
	}
	// F5 §5.3's sanitiser: the reference line is free text a person
	// types, and it is drawn into a PDF that other people read. Control
	// and direction-override characters have no business there for the
	// same reason they have none in a file name.
	cfg.StampReference = consent.SanitizeDisplayText(form.Reference)
	cfg.StampShowDocumentID = form.ShowDocumentID
	return cfg
}

func readStampSettings(win ui.Window) (stampSettings, error) {
	raw, err := win.Eval("window.__liroStampSettings()")
	if err != nil {
		return stampSettings{}, fmt.Errorf("reading the stamp settings: %w", err)
	}
	var jsonStr string
	if err := json.Unmarshal([]byte(raw), &jsonStr); err != nil {
		return stampSettings{}, fmt.Errorf("decoding the stamp settings envelope: %w", err)
	}
	var form stampSettings
	if err := json.Unmarshal([]byte(jsonStr), &form); err != nil {
		return stampSettings{}, fmt.Errorf("decoding the stamp settings: %w", err)
	}
	return form, nil
}

// currentConfig re-reads the configuration from disk, for a caller
// about to open a window that may have been changed since it last
// looked — the tray holds one Config for its whole life, and every
// window that saves one writes it here.
func currentConfig(fallback config.Config) config.Config {
	cfg, err := config.Load(platform.DefaultConfigFile())
	if err != nil {
		slog.Warn("settings: re-reading the configuration failed", "error", err)
		return fallback
	}
	return cfg
}
