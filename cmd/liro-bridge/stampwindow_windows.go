//go:build windows

package main

// Step 3 of the three-step flow: how to sign.
//
// The owner's model, and the reason this window exists at all: choose
// documents, choose the certificate, choose how to sign — three steps,
// each asking one thing, and then it signs. Before this pass the stamp
// was asked for twice, once on the main window's footer and again on
// the consent screen, which is the screen a person reads in two
// seconds to decide whether to sign at all.
//
// So the one question here is visible or invisible, and when visible,
// which corner. Everything else the stamp can carry — which page, a
// reference line, the identity document number — is a standing
// preference, not a decision about this batch, and waits behind a
// disclosure that opens closed.
//
// The same window is also Settings' way in, where the answer is a
// preference rather than the last step before signing. The only
// difference is the two action labels, which Go supplies.

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// The window's two sizes, one per role, both measured in a real window
// rather than guessed.
//
// Step 3 asks one thing and is sized for it: the mode selector, the
// corner selector and the two actions. Settings holds the standing
// preferences as well — which page, the reference line, the
// identity-document toggle and the margin note — and needs the height
// they take. It was 680 when one window showed all of that at once,
// step or not.
//
// One window at one size was tried and looked wrong both ways round:
// at the step's size the preferences scrolled inside a hundred points,
// and at the preferences' size step 3 was a mostly-empty window asking
// one question.
const (
	stampWindowWidth = 440

	// 280 rather than 310 since step 3 lost its subtitle: measured in a
	// real window in sr-Cyrl, the longest of the three catalogues, the
	// form needs 143 points and starts to scroll below 270. The ten
	// points above that are there so a one-word label change does not
	// immediately put it back, and the layout test is what says so.
	stampStepHeight     = 280
	stampSettingsHeight = 700
)

// stampWindowHeight is the height for one role.
func stampWindowHeight(role stampWindowRole) int {
	if role == stampRoleSettings {
		return stampSettingsHeight
	}
	return stampStepHeight
}

// stampWindowRole says which of the window's two callers opened it,
// which decides nothing except the two action labels.
type stampWindowRole int

const (
	// stampRoleStep is step 3 of signing: the primary action signs.
	stampRoleStep stampWindowRole = iota
	// stampRoleSettings is Settings' way in: the primary action saves.
	stampRoleSettings
)

// stampSettings is the window's whole form, read back in one call.
type stampSettings struct {
	Saved          bool   `json:"saved"`
	Visible        bool   `json:"visible"`
	Position       string `json:"position"`
	Page           string `json:"page"`
	Reference      string `json:"reference"`
	ShowDocumentID bool   `json:"showDocumentID"`
}

type jsOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// runStampWindow opens the window and blocks until it is answered. It
// returns the configuration as it stands afterwards and whether the
// person went ahead: false means they cancelled or closed the window,
// which for step 3 means nothing is signed.
//
// The answer is saved to disk on the way out, so the next batch starts
// from it — a returning user accepts what is already there and moves
// on, which is the whole point of persisting it (D-103's reasoning,
// unchanged: this is a preference about how one's own documents look,
// not a per-batch security decision, and SPEC §18.15's rule against a
// remembered certificate does not reach it).
//
// owner is the window this one is opened from — the consent window for
// step 3, the settings window for the preferences role. It is never
// zero in production: a window opened from another window that is not
// owned by it can be created underneath it, which is exactly what made
// the whole program stop responding.
func runStampWindow(cfg config.Config, locale string, role stampWindowRole, owner uintptr) (config.Config, bool) {
	c := i18n.Load(locale)
	messages := make(chan ui.Message, 8)

	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("stampwindow.title"),
		Width:       stampWindowWidth,
		Height:      stampWindowHeight(role),
		Owner:       owner,
		AlwaysOnTop: role == stampRoleStep,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/stamp.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		slog.Warn("stamp window: could not open", "error", err)
		return cfg, false
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(buildStampInit(c, cfg, role)); err != nil {
		slog.Warn("stamp window: could not post the form", "error", err)
		return cfg, false
	}

	msg := <-messages
	if msg.Type != ui.MessageTypeApprove {
		return cfg, false
	}

	form, err := readStampSettings(win)
	if err != nil {
		slog.Warn("stamp window: reading the form failed", "error", err)
		return cfg, false
	}
	if !form.Saved {
		return cfg, false
	}
	cfg = applyStampSettings(cfg, form)
	if err := config.Save(config.DefaultPath(), cfg); err != nil {
		// Worth saying, not worth refusing to sign over: the answer is
		// in hand and correct for this batch either way.
		slog.Warn("stamp window: saving the choice failed", "error", err)
	}
	return cfg, true
}

// buildStampInit is the payload the page renders itself from. Both
// dropdowns' labels are resolved here, in Go, so the page never holds a
// catalogue (F5 §10).
func buildStampInit(c *i18n.Catalogue, cfg config.Config, role stampWindowRole) map[string]any {
	keys := []string{
		"stampwindow.title", "stampwindow.mode_label",
		"stampwindow.position_label",
		"stampwindow.page_label", "stampwindow.page_number_label",
		"stampwindow.reference_label", "stampwindow.reference_hint",
		"stampwindow.show_document_id", "stampwindow.show_document_id_hint",
		"stampwindow.margin_note",
	}
	strings := make(map[string]string, len(keys))
	for _, k := range keys {
		strings[k] = c.T(k)
	}

	primary, secondary := c.T("stampwindow.save"), c.T("stampwindow.cancel")
	// Step 3 carries no subtitle. It had one — "the last step,
	// everything else is already decided" — which said what the person
	// could already see, on a window whose whole value is being small.
	// Settings keeps its own, because there the window is a preference
	// rather than a step and the line says which preference.
	subtitle := c.T("stampwindow.subtitle_settings")
	if role == stampRoleStep {
		primary, secondary = c.T("stampwindow.sign"), c.T("stampwindow.back")
		subtitle = ""
	}

	return map[string]any{
		"type":           "init",
		"strings":        strings,
		"primaryLabel":   primary,
		"secondaryLabel": secondary,
		"subtitle":       subtitle,
		// The standing preferences are Settings' business; step 3 does
		// not show them at all.
		"showMore": role == stampRoleSettings,
		"modes": []jsOption{
			// Visible first: it is what most people signing a contract
			// or an invoice want, and D-103 established the default on
			// the evidence that an invisible signature reads, to the
			// person who just signed, as no signature at all.
			{Value: "visible", Label: c.T("stampwindow.mode_visible")},
			{Value: "invisible", Label: c.T("stampwindow.mode_invisible")},
		},
		"positions": []jsOption{
			// bottom-right first: SPEC §13.1's own default.
			{Value: consent.StampPositionBottomRight, Label: c.T("stampwindow.position_bottom_right")},
			{Value: consent.StampPositionBottomLeft, Label: c.T("stampwindow.position_bottom_left")},
			{Value: consent.StampPositionTopRight, Label: c.T("stampwindow.position_top_right")},
			{Value: consent.StampPositionTopLeft, Label: c.T("stampwindow.position_top_left")},
		},
		"pages": []jsOption{
			{Value: config.StampPageFirst, Label: c.T("stampwindow.page_first")},
			{Value: config.StampPageLast, Label: c.T("stampwindow.page_last")},
			{Value: "number", Label: c.T("stampwindow.page_number")},
		},
		"model": map[string]any{
			"visible":        cfg.VisibleStamp,
			"position":       cfg.StampPosition,
			"page":           cfg.StampPage,
			"reference":      cfg.StampReference,
			"showDocumentID": cfg.StampShowDocumentID,
		},
	}
}

// applyStampSettings folds the form onto cfg, validating every value
// the page could have produced. The page is this project's own, but it
// is still the outside of a boundary: a position or a page it does not
// recognise falls back to the default rather than being written to
// disk for config.Load to complain about on the next run.
func applyStampSettings(cfg config.Config, form stampSettings) config.Config {
	cfg.VisibleStamp = form.Visible
	cfg.StampPosition = consent.StampChoice{
		Visible: form.Visible, Position: form.Position,
	}.Normalised().Position
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
