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
	"strings"
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
	"settings.tsa_url_label",
	"settings.tsa_preset_freetsa",
	"settings.tsa_preset_freetsa_warning",
	"settings.tsa_preset_rsgov",
	"settings.tsa_preset_rsgov_note",
	"settings.tsa_user_label",
	"settings.tsa_password_label",
	"settings.tsa_client_cert_label",
	"settings.tsa_client_cert_password_label",
	"settings.output_suffix_label",
	"settings.signature_level_label",
	"settings.level_bb",
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

			win, _ := sharedSettingsWindow(t, c, cfg)

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

// TestSettingsTSAPresetsFillTheURLAndStartUnselected is Task 1c (F5
// second-real-run review). Two named presets, neither preselected out
// of the box, each filling the URL field it is a shortcut for — and a
// hand-typed URL selecting neither, so a preset radio never claims an
// authority the user did not choose.
func TestSettingsTSAPresetsFillTheURLAndStartUnselected(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := sharedSettingsWindow(t, c, config.Default())

	if got := evalString(t, win, "String(document.querySelectorAll('input[name=tsa-preset]:checked').length)"); got != "0" {
		t.Fatalf("%s preset radios are checked with the default (empty) configuration; want none", got)
	}

	for _, tc := range []struct{ id, wantURL string }{
		{"tsa-preset-freetsa", tsaPresetFreeTSA},
		{"tsa-preset-rsgov", tsaPresetRSGOV},
	} {
		script := "(function(){var el=document.getElementById('" + tc.id + "');el.checked=true;" +
			"el.dispatchEvent(new Event('change'));return document.getElementById('tsa-url').value;})()"
		if got := evalString(t, win, script); got != tc.wantURL {
			t.Errorf("choosing %s put %q in the URL field, want %q", tc.id, got, tc.wantURL)
		}
	}

	// A URL that is neither preset selects neither.
	script := "(function(){var u=document.getElementById('tsa-url');u.value='https://example.invalid/tsa';" +
		"u.dispatchEvent(new Event('input'));" +
		"return String(document.querySelectorAll('input[name=tsa-preset]:checked').length);})()"
	if got := evalString(t, win, script); got != "0" {
		t.Errorf("a hand-typed URL left %s preset radios checked, want none", got)
	}
}

// TestSettingsPresetSelectionReflectsASavedURL: reopening Settings with
// a preset's own URL already saved shows that preset as the chosen one,
// rather than silently presenting it as a custom URL.
func TestSettingsPresetSelectionReflectsASavedURL(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.TSAURL = tsaPresetFreeTSA
	win, _ := sharedSettingsWindow(t, c, cfg)

	if got := evalString(t, win, "String(document.getElementById('tsa-preset-freetsa').checked)"); got != "true" {
		t.Errorf("freetsa.org preset checked = %s with its own URL saved, want true", got)
	}
	if got := evalString(t, win, "String(document.getElementById('tsa-preset-rsgov').checked)"); got != "false" {
		t.Errorf("the other preset is checked too, want only one")
	}
}

// TestSettingsStatusLineRendersWhatAnActionDid is Task 3: "Export audit
// log" and "Check for updates" must say something on screen. This
// proves the status channel the two of them report through actually
// reaches the DOM — the previous build's failure was not that the
// handlers were wrong, but that nothing they did was visible.
func TestSettingsStatusLineRendersWhatAnActionDid(t *testing.T) {
	c := i18n.Load("sr-Latn")
	// A fresh init is what a newly opened Settings window gets, and it
	// clears the status line — so this really is "before any action".
	win, _ := sharedSettingsWindow(t, c, config.Default())

	if got := evalString(t, win, "String(document.getElementById('action-status').hidden)"); got != "true" {
		t.Errorf("the status line is visible before any action ran")
	}

	// The status line is a heading plus the list of files an action
	// wrote, so its own textContent carries the whitespace between them:
	// the sentence is read off the heading. Its class keeps
	// liro-fixed-region — posting a status used to replace the class
	// list outright, which took away the one thing stopping a growing
	// form from squeezing the status line off the window.
	postWindowStatus(win, c.T("settings.updates_not_available"), ui.IntentWarning)
	if got := evalString(t, win, "document.getElementById('action-status-text').textContent"); got != c.T("settings.updates_not_available") {
		t.Errorf("status line reads %q, want %q", got, c.T("settings.updates_not_available"))
	}
	if got := evalString(t, win, "String(document.getElementById('action-status').hidden)"); got != "false" {
		t.Errorf("the status line is still hidden after a status was posted")
	}
	if got := evalString(t, win, "document.getElementById('action-status').className"); !strings.Contains(got, "liro-outcome-warning") {
		t.Errorf("status line class = %q, want it to carry liro-outcome-warning", got)
	}
	if got := evalString(t, win, "document.getElementById('action-status').className"); !strings.Contains(got, "liro-fixed-region") {
		t.Errorf("status line class = %q, want it to keep liro-fixed-region", got)
	}
	if got := evalNumber(t, win, "document.getElementById('action-status').getBoundingClientRect().height"); got <= 0 {
		t.Errorf("the status line has zero height — it is not actually on screen")
	}

	postWindowStatus(win, "C:\\Users\\Test\\Desktop", ui.IntentPositive)
	if got := evalString(t, win, "document.getElementById('action-status').className"); !strings.Contains(got, "liro-outcome-positive") {
		t.Errorf("status line class = %q, want it to carry liro-outcome-positive", got)
	}
}

// TestSettingsWindowFitsWithoutHorizontalOverflow guards Task 2's
// "any long value wraps or truncates rather than overflowing" for the
// window this round added a whole preset group to.
func TestSettingsWindowFitsWithoutHorizontalOverflow(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	cfg := config.Default()
	cfg.TSAURL = "https://" + strings.Repeat("a", 200) + ".example/tsa"
	cfg.TSAClientCertPath = "C:\\" + strings.Repeat("b", 200) + ".p12"
	win, _ := sharedSettingsWindow(t, c, cfg)

	scroll := evalNumber(t, win, "document.body.scrollWidth")
	client := evalNumber(t, win, "document.body.clientWidth")
	if scroll > client {
		t.Errorf("body scrollWidth %v exceeds clientWidth %v", scroll, client)
	}
}
