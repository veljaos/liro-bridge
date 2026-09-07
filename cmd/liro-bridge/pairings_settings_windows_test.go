//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// newTestPairings opens a real pairing store — the real DPAPI-backed
// secret store included — in a directory of this test's own, never the
// agent's. It deliberately does not redirect LOCALAPPDATA: a WebView2
// process may only open one user data folder, and every window test in
// this package shares the one the first of them created.
func newTestPairings(t *testing.T) *api.Pairings {
	t.Helper()
	dir := t.TempDir()
	secrets, err := platform.NewSecretStore(dir)
	if err != nil {
		t.Fatalf("NewSecretStore: %v", err)
	}
	p, err := api.OpenPairings(filepath.Join(dir, "pairings.json"), secrets, nil)
	if err != nil {
		t.Fatalf("OpenPairings: %v", err)
	}
	return p
}

// settingsWithPairings posts a settings init carrying list, and returns
// the window.
func settingsWithPairings(t *testing.T, c *i18n.Catalogue, list []api.Pairing) (ui.Window, chan ui.Message) {
	t.Helper()
	win, messages := sharedSettingsWindow(t, c, config.Default())
	if err := win.PostJSON(buildSettingsInit(c, config.Default(), list)); err != nil {
		t.Fatalf("PostJSON(settings init): %v", err)
	}
	return win, messages
}

// F7 §2.4: name, origin, when paired, when last used, and a way to
// revoke — every one of them on screen, in every language.
func TestSettingsListsEveryPairedApplication(t *testing.T) {
	paired := time.Date(2026, 9, 3, 9, 15, 0, 0, time.Local)
	list := []api.Pairing{
		{
			AppID: "aaaa", Name: "Knjigovodstvo d.o.o. — ERP",
			Origin:   "https://erp.knjigovodstvo.rs",
			PairedAt: paired, LastUsedAt: paired.Add(30 * time.Hour),
		},
		{
			AppID: "bbbb", Name: "Skladiste",
			// http:// rather than https:// is exactly what a person is
			// meant to be able to see here.
			Origin:   "http://skladiste.local:9000",
			PairedAt: paired.Add(-48 * time.Hour),
		},
	}

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win, _ := settingsWithPairings(t, c, list)

			if n := evalNumber(t, win, "document.querySelectorAll('#pairings .pairing-row').length"); int(n) != len(list) {
				t.Fatalf("%d rows rendered, want %d", int(n), len(list))
			}
			if hidden := evalString(t, win, "String(document.getElementById('pairings-empty').hidden)"); hidden != "true" {
				t.Fatal("the empty-state message is shown alongside two applications")
			}

			text := evalString(t, win, "document.getElementById('pairings').textContent")
			for _, want := range []string{
				list[0].Name, list[0].Origin,
				list[1].Name, list[1].Origin,
				"03.09.2026. 09:15",             // when the first was paired
				"04.09.2026. 15:15",             // when it last asked
				c.T("settings.pairings_revoke"), // the way to take it away
			} {
				if !strings.Contains(text, want) {
					t.Fatalf("the list does not carry %q:\n%s", want, text)
				}
			}

			// An application that has never asked for anything says so
			// rather than showing a zero time, which would read as the
			// first of January in year one.
			if strings.Contains(text, "01.01.0001") {
				t.Fatalf("a never-used application rendered a zero time:\n%s", text)
			}

			// Every button carries the identifier it would revoke, and
			// no two carry the same one.
			ids := evalString(t, win,
				"Array.from(document.querySelectorAll('#pairings .pairing-revoke'))"+
					".map(function(b){return b.getAttribute('data-app-id')}).join(',')")
			if ids != "aaaa,bbbb" {
				t.Fatalf("the revoke buttons name %q, want aaaa,bbbb", ids)
			}

			assertPageDoesNotScroll(t, win, "settings with pairings ("+locale+")", ".settings-form")
			assertButtonsVisible(t, win, "settings with pairings ("+locale+")", ".settings-form")
		})
	}
}

func TestSettingsSaysSoWhenNothingIsPaired(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := settingsWithPairings(t, c, nil)

	if hidden := evalString(t, win, "String(document.getElementById('pairings-empty').hidden)"); hidden != "false" {
		t.Fatal("nothing is paired and the window says nothing about it")
	}
	if n := evalNumber(t, win, "document.querySelectorAll('#pairings .pairing-row').length"); n != 0 {
		t.Fatalf("%v rows rendered with nothing paired", n)
	}
}

// A caller's name and origin are untrusted text and go into the DOM as
// text (SPEC §6.6). Nothing here is markup, whatever it looks like.
func TestAPairingsNameIsRenderedAsTextNeverAsMarkup(t *testing.T) {
	c := i18n.Load("en")
	hostile := "<img src=x onerror=alert(1)>"
	win, _ := settingsWithPairings(t, c, []api.Pairing{{
		AppID: "aaaa", Name: hostile, Origin: "https://x.example.com", PairedAt: time.Now(),
	}})

	if n := evalNumber(t, win, "document.querySelectorAll('#pairings img').length"); n != 0 {
		t.Fatalf("%v image elements were created from an application's display name", n)
	}
	if got := evalString(t, win, "document.querySelector('#pairings .pairing-name').textContent"); got != hostile {
		t.Fatalf("the name renders as %q, want it verbatim as text", got)
	}
}

// F7 §2.4: revoking is immediate. The device secret is gone before the
// handler returns, so the next request that presents it authenticates
// against nothing.
func TestDisconnectingAnApplicationRevokesItImmediately(t *testing.T) {
	pairings := newTestPairings(t)
	keep, _, err := pairings.Add("Stays", "https://stays.example.com")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	goes, secret, err := pairings.Add("Goes away", "https://goes.example.com")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(secret) != api.DeviceSecretLength {
		t.Fatalf("device secret is %d bytes", len(secret))
	}

	c := i18n.Load("sr-Latn")
	win, _ := settingsWithPairings(t, c, pairings.List())

	// Pressing the row's own button, through the page's own DOM
	// (D-094's carve-out), is what carries the identifier back.
	click := "document.querySelector('#pairings .pairing-revoke[data-app-id=" +
		jsStringLiteral(goes.AppID) + "]').click()"
	if _, err := win.Eval(click); err != nil {
		t.Fatalf("Eval(click revoke): %v", err)
	}
	state := collectSettingsState(t, win)
	if state.Action != "revokePairing" {
		t.Fatalf("the click reported action %q, want revokePairing", state.Action)
	}
	if state.RevokeAppID != goes.AppID {
		t.Fatalf("the click reported %q, want %q", state.RevokeAppID, goes.AppID)
	}

	if closed := handleSettingsAction(win, c, config.Default(), pairings, state); closed {
		t.Fatal("revoking a pairing closed the settings window")
	}

	if _, ok := pairings.Get(goes.AppID); ok {
		t.Fatal("the pairing is still listed after being revoked")
	}
	if _, ok := pairings.Secret(goes.AppID); ok {
		t.Fatal("the device secret still resolves after the pairing was revoked")
	}
	if _, ok := pairings.Get(keep.AppID); !ok {
		t.Fatal("revoking one pairing took the other one with it")
	}

	// And what is on screen agrees: one row, the one that stayed, and a
	// line saying what just happened.
	if n := evalNumber(t, win, "document.querySelectorAll('#pairings .pairing-row').length"); int(n) != 1 {
		t.Fatalf("%v rows on screen after revoking one of two", n)
	}
	if got := evalString(t, win, "document.getElementById('pairings').textContent"); !strings.Contains(got, keep.Name) {
		t.Fatalf("the surviving application is not on screen:\n%s", got)
	}
	status := evalString(t, win, "document.getElementById('action-status-text').textContent")
	if status != c.T("settings.pairings_revoked") {
		t.Fatalf("the status line says %q, want the revocation message", status)
	}
}

// Revoking is about the pairing list and nothing else. Re-posting the
// whole init payload would have put every unsaved edit in the form back
// to what is on disk, which is not what disconnecting an application
// asked for.
func TestRevokingDoesNotDiscardUnsavedSettings(t *testing.T) {
	pairings := newTestPairings(t)
	goes, _, err := pairings.Add("Goes away", "https://goes.example.com")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	c := i18n.Load("sr-Latn")
	win, _ := settingsWithPairings(t, c, pairings.List())

	const typed = "https://tsa.example.com/typed-but-not-saved"
	if _, err := win.Eval("document.getElementById('tsa-url').value = " + jsStringLiteral(typed)); err != nil {
		t.Fatalf("Eval(set tsa-url): %v", err)
	}

	click := "document.querySelector('#pairings .pairing-revoke[data-app-id=" +
		jsStringLiteral(goes.AppID) + "]').click()"
	if _, err := win.Eval(click); err != nil {
		t.Fatalf("Eval(click revoke): %v", err)
	}
	handleSettingsAction(win, c, config.Default(), pairings, collectSettingsState(t, win))

	if got := evalString(t, win, "document.getElementById('tsa-url').value"); got != typed {
		t.Fatalf("the unsaved timestamp URL is now %q, want %q", got, typed)
	}
}
