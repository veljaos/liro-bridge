//go:build windows

package main

// The stamp window (F6 §6). The owner's request after using F5: the
// stamp controls do not belong squeezed onto the consent screen, which
// is the screen a person reads in two seconds to decide whether to
// sign. Everything about how the stamp looks lives here instead, and
// the consent and main windows show the current answer in one line with
// a link to change it.

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// Measured, not guessed. 560 points was the first attempt and was too
// short: with every control shown — the stamp on, a specific page
// chosen, so the page-number box present too — the form's own region
// scrolled and the margin note sat below the fold. It is the one line
// on this window a person needs to read once and never again, so having
// it be the line that gets cut is the wrong way round. 680 shows the
// whole form with the actions pinned below it (D-106), and still fits
// this machine's 1032-point work area with room to spare.
const (
	stampWindowWidth  = 460
	stampWindowHeight = 680
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

// runStampWindow opens the stamp window and blocks until it is closed,
// saving the configuration if Save was pressed.
func runStampWindow(cfg config.Config, locale string) error {
	c := i18n.Load(locale)
	messages := make(chan ui.Message, 8)

	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("stampwindow.title"),
		Width:       stampWindowWidth,
		Height:      stampWindowHeight,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/stamp.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		return err
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(buildStampInit(c, cfg)); err != nil {
		return err
	}

	msg := <-messages
	if msg.Type != ui.MessageTypeApprove {
		return nil
	}

	form, err := readStampSettings(win)
	if err != nil {
		return err
	}
	if !form.Saved {
		return nil
	}
	return config.Save(config.DefaultPath(), applyStampSettings(cfg, form))
}

// buildStampInit is the payload the page renders itself from. Both
// dropdowns' labels are resolved here, in Go, so the page never holds a
// catalogue (F5 §10).
func buildStampInit(c *i18n.Catalogue, cfg config.Config) map[string]any {
	keys := []string{
		"stampwindow.title", "stampwindow.visible", "stampwindow.position_label",
		"stampwindow.page_label", "stampwindow.page_number_label",
		"stampwindow.reference_label", "stampwindow.reference_hint",
		"stampwindow.show_document_id", "stampwindow.show_document_id_hint",
		"stampwindow.margin_note", "stampwindow.save", "stampwindow.cancel",
	}
	strings := make(map[string]string, len(keys))
	for _, k := range keys {
		strings[k] = c.T(k)
	}

	return map[string]any{
		"type":    "init",
		"strings": strings,
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

// currentStampConfig re-reads the configuration from disk, for a caller
// that has just let the user change it in this window.
func currentStampConfig(fallback config.Config) config.Config {
	cfg, err := config.Load(platform.DefaultConfigFile())
	if err != nil {
		slog.Warn("stamp window: re-reading the configuration failed", "error", err)
		return fallback
	}
	return cfg
}

// stampPageLabel renders a page selection for a one-line summary.
//
// Each answer carries its own noun ("First page", "Page 3") rather than
// a bare value the caller has to introduce. The first version of the
// summary read "Vidljivi pečat: uključen, Dole desno, strana Prva
// strana" — "page First page" — because both the format and the label
// supplied the word.
func stampPageLabel(c *i18n.Catalogue, page string) string {
	switch page {
	case config.StampPageFirst:
		return c.T("stampwindow.page_first")
	case config.StampPageLast:
		return c.T("stampwindow.page_last")
	default:
		if n, err := strconv.Atoi(page); err == nil {
			return fmt.Sprintf(c.T("main.stamp_page_number"), strconv.Itoa(n))
		}
		return page
	}
}
