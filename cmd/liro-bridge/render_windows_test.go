//go:build windows

package main

// TestSettingsWindowRendersLocalisedText is Task 1's regression test
// (F5 review, post-shipping-run findings): a real WebView2 window is
// created, the actual init JSON payload is posted through Window.PostJSON
// exactly as runSettingsWindow does, and the DOM is read back through
// Window.Eval — not the Go-level payload builder alone.
//
// This is deliberately not the same kind of test as
// TestConsentInitRendersCyrillic (ui_payloads_test.go), which only
// proves buildConsentInit/buildSettingsInit build the right map: that
// class of test passed while the shipped binary showed a window with
// every label blank, because the payload never reached the page at
// all — ExecuteScript ran before the page's own <script> tags had
// defined window.__liroReceive, so bridge.js's "window.__liroReceive
// && ..." guard silently dropped it (see docs/decisions.md, and the
// navigationCompletedHandler fix in internal/ui/webview2_windows.go
// this test would have caught before it shipped). Rendering a real
// window and reading textContent back through Eval is the only way to
// prove the string on screen is not blank.
import (
	"encoding/json"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// settingsDataI18nKeys are every data-i18n key settings.html resolves
// via textContent (internal/ui/assets/pages/settings.html) — excludes
// data-i18n-placeholder (settings.tsa_custom_url_placeholder), which is
// an attribute, not rendered text, and so is not what a blank-label bug
// would show as blank on screen.
var settingsDataI18nKeys = []string{
	"settings.window_title",
	"settings.language_label",
	"settings.start_with_windows",
	"settings.tsa_label",
	"settings.output_suffix_label",
	"settings.signature_level_label",
	"settings.level_bt",
	"settings.level_blt",
	"settings.export_audit_log",
	"settings.check_updates_daily",
	"settings.check_updates_now",
	"settings.version_label",
	"settings.copy",
	"settings.close",
	"settings.save",
}

func TestSettingsWindowRendersLocalisedText(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			cfg := config.Config{
				Locale:         locale,
				OutputSuffix:   "-signed",
				SignatureLevel: "b-lt",
			}

			messages := make(chan ui.Message, 8)
			win, err := ui.NewWindow(ui.Options{
				Title:       c.T("settings.window_title"),
				Width:       520,
				Height:      480,
				Assets:      assetsFS,
				VirtualHost: liroVirtualHost,
				StartPage:   "/pages/settings.html",
				OnMessage:   func(m ui.Message) { messages <- m },
				OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
			})
			if err != nil {
				t.Fatalf("NewWindow: %v", err)
			}
			defer func() { _ = win.Close() }()

			if err := win.PostJSON(buildSettingsInit(c, cfg)); err != nil {
				t.Fatalf("PostJSON: %v", err)
			}

			for _, key := range settingsDataI18nKeys {
				want := c.T(key)
				if want == "" {
					t.Fatalf("catalogue has no text for %q in locale %q — test fixture is stale", key, locale)
				}
				got := evalDataI18nText(t, win, key)
				if got == "" {
					t.Errorf("locale %s: label %q rendered blank in the DOM (expected %q)", locale, key, want)
					continue
				}
				if got != want {
					t.Errorf("locale %s: label %q rendered %q, catalogue says %q", locale, key, got, want)
				}
			}
		})
	}
}

// evalDataI18nText reads back the textContent of the single element
// carrying data-i18n="key" via Window.Eval — the same DOM the user
// actually sees, not the Go-side string that was supposed to reach it.
func evalDataI18nText(t *testing.T, win ui.Window, key string) string {
	t.Helper()
	script := "(function(){var el=document.querySelector('[data-i18n=" + jsStringLiteral(key) + "]');return el?el.textContent:null;})()"
	raw, err := win.Eval(script)
	if err != nil {
		t.Fatalf("Eval(%s): %v", key, err)
	}
	var s *string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("Eval(%s): decoding result %q: %v", key, raw, err)
	}
	if s == nil {
		t.Fatalf("Eval(%s): no element with data-i18n=%q found in the DOM", key, key)
	}
	return *s
}

// jsStringLiteral encodes key (always one of this file's own constant
// key strings, never untrusted input) as a JSON string literal so it
// can be embedded directly in the script text passed to Eval.
func jsStringLiteral(key string) string {
	b, err := json.Marshal(key)
	if err != nil {
		panic(err)
	}
	return string(b)
}
