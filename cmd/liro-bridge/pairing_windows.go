package main

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// The pairing window's size, measured in a real window in all three
// locales — the numbers below are identical in each, because the only
// text on this screen that differs between them is three short labels.
//
// It holds three things and no list, so nothing about it grows: two
// fields, six digits at --liro-font-size-code with a two-line note
// under them, and one button.
//
// 369 rather than the 330 it was, and the height is set by one rule:
// the identity block must not scroll for an ordinary two-line company
// name, with one --liro-space-4 of slack (D-202). What made the old
// number too small is that everything below the block grew — the
// origin is the page's own face at --liro-font-size-heading rather
// than twelve points of monospace in a card, the gap above the code is
// --liro-space-5 rather than --liro-space-4, and the note under the
// code is two lines rather than one (D-208). Measured at 330 with the
// new fields, that name needed 121 points in a block that could be
// given 118; at 352, with the note still one line, 123. Three points
// short, and then two, is a scrollbar beside an ordinary name — D-202's
// own defect one screen over. At 369 the block can be given 140.
const (
	pairingWindowWidth  = 420
	pairingWindowHeight = 369

	// The screen shown once the pairing has succeeded carries a mark,
	// one word and the name — no code, no origin — so it needs about
	// two thirds of the height. The window shrinks to it rather than
	// leaving half of itself empty, measured by looking at it.
	//
	// 228 rather than the 226 it was: the sentence that used to sit
	// under the name is gone (D-208) and a 40-point mark stands above
	// it, and what the height is set by is unchanged — the identity
	// block must not scroll for an ordinary two-line company name.
	// Measured: that name needs 45 points, and at 228 the block can be
	// given 61 — one --liro-space-4 of slack, which is the margin
	// D-202 chose so that a one-word label change in any of the three
	// catalogues does not put the scrollbar straight back.
	pairingConnectedHeight = 228
)

// pairingUI opens the agent's own pairing window (F7 §2.1). It is what
// cmd/liro-bridge hands internal/api, which never imports internal/ui:
// SPEC §4.2 rule 4 keeps the three front doors independent, and this
// file is the one place that knows a pairing prompt is drawn by
// WebView2.
type pairingUI struct {
	// fallback is the configuration to fall back on when the file
	// cannot be read; the locale is taken from disk at the moment the
	// window opens, like every other window this agent shows (D-134).
	fallback config.Config
}

func newPairingUI(fallback config.Config) pairingUI { return pairingUI{fallback: fallback} }

// ShowPairing implements api.PairingUI.
func (u pairingUI) ShowPairing(prompt api.PairingPrompt) (api.PairingWindow, error) {
	cfg := currentConfig(u.fallback)
	c := i18n.Load(cfg.Locale)

	w := &pairingWindow{denied: make(chan struct{})}

	win, err := ui.NewWindow(ui.Options{
		Title:  c.T("pairing.window_title"),
		Width:  pairingWindowWidth,
		Height: pairingWindowHeight,
		// A pairing request nobody sees is a pairing request that
		// expires with a person sitting two feet away — the same
		// reasoning F7 §7.4 gives for the consent window.
		AlwaysOnTop: true,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/pairing.html",
		OnMessage:   func(m ui.Message) { w.onMessage(m) },
		OnClosed:    func() { w.refuse() },
	})
	if err != nil {
		return nil, err
	}
	w.win = win

	if err := win.PostJSON(buildPairingInit(c, prompt)); err != nil {
		_ = win.Close()
		// A window closed before its first payload landed is a window
		// the person closed, which is a refusal, not a failure (D-145).
		if errors.Is(err, ui.ErrWindowClosed) {
			w.refuse()
			return w, nil
		}
		return nil, err
	}
	return w, nil
}

// pairingWindow adapts one open window to api.PairingWindow.
type pairingWindow struct {
	win    ui.Window
	denied chan struct{}
	once   sync.Once
}

// onMessage handles the page's only two buttons. Deny refuses;
// Close, on the screen shown after a successful pairing, is a person
// dismissing a window whose answer has already been given — refusing
// then reaches nobody, because the flow stopped watching the moment it
// issued the secret.
func (w *pairingWindow) onMessage(m ui.Message) {
	if m.Type != ui.MessageTypeCancel {
		return
	}
	w.refuse()
	_ = w.win.Close()
}

func (w *pairingWindow) refuse() {
	w.once.Do(func() { close(w.denied) })
}

// Denied implements api.PairingWindow.
func (w *pairingWindow) Denied() <-chan struct{} { return w.denied }

// Confirmed implements api.PairingWindow: the window says so rather
// than vanishing at the moment the person was reading a code out of it.
func (w *pairingWindow) Confirmed() {
	// Keeping its own centre rather than re-centring on a monitor, so
	// the window does not walk across the screen at the moment the
	// person is reading it (D-148's rule for the signing flow's steps).
	if err := w.win.Resize(pairingWindowWidth, pairingConnectedHeight); err != nil {
		slog.Debug("pairing: could not resize to the connected screen", "error", err)
	}
	if err := w.win.PostJSON(map[string]any{"type": "connected"}); err != nil {
		if !errors.Is(err, ui.ErrWindowClosed) {
			slog.Warn("pairing: could not show the connected state", "error", err)
		}
	}
}

// Close implements api.PairingWindow.
func (w *pairingWindow) Close() {
	if w.win != nil {
		_ = w.win.Close()
	}
}

// buildPairingInit is the payload the pairing page renders.
//
// The application's name and its origin arrive already checked by
// internal/api — the name sanitised the way every other piece of
// untrusted display text is, the origin refused outright if it could
// not be shown verbatim (SPEC §6.2, §6.6) — and go into the DOM through
// textContent, never innerHTML.
func buildPairingInit(c *i18n.Catalogue, prompt api.PairingPrompt) map[string]any {
	keys := []string{
		"pairing.request_label",
		"pairing.origin_label",
		"pairing.code_label",
		"pairing.code_explain",
		"pairing.code_validity",
		"pairing.deny",
		"pairing.connected_title",
		"pairing.close",
	}
	strs := make(map[string]string, len(keys))
	for _, k := range keys {
		strs[k] = c.T(k)
	}
	return map[string]any{
		"type":    "init",
		"strings": strs,
		"model": map[string]any{
			"applicationName": prompt.Name,
			"origin":          prompt.Origin,
			"code":            prompt.Code,
		},
	}
}
