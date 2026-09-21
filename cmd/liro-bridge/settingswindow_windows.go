//go:build windows

package main

// The Settings window, which lived in tray_windows.go because the tray
// is what opens it — and which is not a tray. The split is F12 §3's
// preparation rather than a tidy-up: everything here is ordinary Go
// over internal/ui and internal/config with two exceptions, and the
// exceptions are the whole reason to separate it. handleSettingsAction
// calls platform.NewAutostart, whose Linux shape is an XDG .desktop
// file rather than a registry value (F12 §8), and applyExplorerMenu,
// which F12 §8 refuses to reproduce on Linux at all — "there is no
// universal context menu on Linux". Both stay in tray_windows.go with
// the Explorer menu they belong to.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// settingsFormState is what settings.js's __liroCollectState() returns,
// read back via Window.Eval (never a page->Go message — see
// internal/ui's Window.Eval doc comment).
type settingsFormState struct {
	Action                string `json:"action"`
	RevokeAppID           string `json:"revokeAppId"`
	Locale                string `json:"locale"`
	StartWithWindows      bool   `json:"startWithWindows"`
	TSAURL                string `json:"tsaURL"`
	TSAUser               string `json:"tsaUser"`
	TSAPassword           string `json:"tsaPassword"`
	TSAClientCertPath     string `json:"tsaClientCertPath"`
	TSAClientCertPassword string `json:"tsaClientCertPassword"`
	OutputSuffix          string `json:"outputSuffix"`
	OutputFolder          string `json:"outputFolder"`
	ExplorerMenu          bool   `json:"explorerMenu"`
	DocumentSigning       bool   `json:"documentSigning"`
	CertificateListing    bool   `json:"certificateListing"`
	SignatureLevel        string `json:"signatureLevel"`
	CheckUpdatesDaily     bool   `json:"checkUpdatesDaily"`
}

// settingsOnOpening is everything a settings window decides at the
// moment it opens: it reads the configuration file, takes its interface
// language from what it finds, and builds the payload the page renders
// itself from.
//
// One function because those three are one decision, and because a test
// can then ask a reopened window what it would show without having to
// reproduce the reading half — which is how the version of this that
// only ever checked the view model came to pass while the window on
// screen showed the old values.
func settingsOnOpening(fallback config.Config, pairings *api.Pairings) (*i18n.Catalogue, config.Config, map[string]any) {
	cfg := currentConfig(fallback)
	c := i18n.Load(cfg.Locale)
	return c, cfg, buildSettingsInit(c, cfg, listPairings(pairings))
}

// owner is the window Settings was opened from, or zero when it was
// opened from the tray and stands on its own.
//
// fallback is only that: the window reads the configuration file itself,
// here, at the moment it opens, and takes both the form's values and its
// own interface language from what it finds. Its callers are long-lived
// — the tray for the life of the process, the signing window for the
// life of a batch — and a Config handed down from one of them says what
// was true when *that* started, which is how a language saved a moment
// ago came back as the old one on reopening.
func runSettingsWindow(fallback config.Config, owner uintptr, pairings *api.Pairings) error {
	c, cfg, init := settingsOnOpening(fallback, pairings)
	messages := make(chan ui.Message, 8)

	win, err := ui.NewWindow(ui.Options{
		Title: c.T("settings.window_title"),
		Owner: owner,
		Width: 520,
		// Task 1c/3 (F5 second-real-run review): the window grew by a
		// timestamp-authority preset group and a status line; 480 points
		// no longer reaches the Save button without scrolling.
		Height:      880,
		AlwaysOnTop: true,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/settings.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		return err
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(init); err != nil {
		// A window closed before its first payload landed is a window
		// the person closed, not one that failed — the same outcome as
		// closing it a moment later, which every other path here
		// already treats as a plain cancellation.
		if errors.Is(err, ui.ErrWindowClosed) {
			return nil
		}
		return err
	}

	for {
		msg := <-messages
		switch msg.Type {
		case ui.MessageTypeCancel:
			return nil
		case ui.MessageTypeApprove:
			raw, err := win.Eval("window.__liroCollectState()")
			if err != nil {
				slog.Warn("settings: reading form state failed", "error", err)
				continue
			}
			var jsonStr string
			if err := json.Unmarshal([]byte(raw), &jsonStr); err != nil {
				slog.Warn("settings: decoding form state envelope failed", "error", err)
				continue
			}
			var state settingsFormState
			if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
				slog.Warn("settings: decoding form state failed", "error", err)
				continue
			}
			if handleSettingsAction(win, c, cfg, pairings, state) {
				return nil
			}
		}
	}
}

// handleSettingsAction returns true when the settings window should
// close after handling state.Action. Every action that does not close
// the window reports what it did back to the page (Task 3, F5
// second-real-run review): "Export audit log" and "Check for updates"
// both used to run to completion with their only output in the log
// file, which on screen is indistinguishable from a dead button.
func handleSettingsAction(win ui.Window, c *i18n.Catalogue, cfg config.Config, pairings *api.Pairings, state settingsFormState) bool {
	switch state.Action {
	case "save":
		// Start from the configuration as it stands on disk *now* and
		// overwrite only what the form actually holds.
		//
		// Building a fresh config.Config here instead reset every field
		// the form does not show — the log level and the port range to
		// hard-coded literals, and the visible-stamp choice and its
		// corner to their zero values, so saving any setting silently
		// switched the stamp off again.
		//
		// Starting from the caller's copy, which is what replaced it,
		// has the same effect one step removed: that copy is as old as
		// whoever is holding it. The stamp window is reachable from
		// this very window and writes its own answer to disk while
		// Settings is still open, so a save that folded the form onto
		// the copy Settings was opened with wrote the pre-stamp values
		// straight back over it.
		newCfg := currentConfig(cfg)
		newCfg.Locale = state.Locale
		newCfg.StartWithWindows = state.StartWithWindows
		newCfg.TSAURL = state.TSAURL
		newCfg.TSAUser = state.TSAUser
		newCfg.TSAPassword = state.TSAPassword
		newCfg.TSAClientCertPath = state.TSAClientCertPath
		newCfg.TSAClientCertPassword = state.TSAClientCertPassword
		newCfg.OutputSuffix = state.OutputSuffix
		newCfg.OutputFolder = strings.TrimSpace(state.OutputFolder)
		newCfg.ExplorerMenuEnabled = state.ExplorerMenu
		newCfg.DocumentSigningEnabled = state.DocumentSigning
		newCfg.CertificateListingEnabled = state.CertificateListing
		newCfg.SignatureLevel = state.SignatureLevel
		newCfg.UpdateCheckEnabled = state.CheckUpdatesDaily
		if err := config.Save(config.DefaultPath(), newCfg); err != nil {
			slog.Warn("settings: saving config failed", "error", err)
			return false
		}
		if err := platform.NewAutostart().SetEnabled(state.StartWithWindows, exePath()); err != nil {
			slog.Warn("settings: updating autostart registration failed", "error", err)
		}
		// F6 §2: the Explorer entry follows the setting immediately,
		// not on the next launch — a person who unticks it and then
		// right-clicks a PDF must not still see it there.
		if err := applyExplorerMenu(newCfg, c); err != nil {
			slog.Warn("settings: updating the Explorer context menu failed", "error", err)
			postWindowStatus(win, c.T("settings.explorer_menu_failed"), ui.IntentNegative)
			return false
		}
		return true
	case "exportAuditLog":
		exportAuditLogNow(win, c)
		return false
	case "revokePairing":
		// F7 §2.4: immediate. The device secret is gone before this
		// returns, so a request already in flight from that application
		// authenticates against nothing.
		text, ok := revokePairing(c, pairings, state.RevokeAppID)
		if !ok {
			return false
		}
		// The list the person clicked is now wrong by exactly one row,
		// so it is re-rendered from the store rather than left to be
		// believed. Only the list: re-posting the whole init payload
		// would put every unsaved edit in the form back to what is on
		// disk, which is not what disconnecting an application asked
		// for.
		_ = win.PostJSON(map[string]any{
			"type":     "pairings",
			"pairings": jsPairings(c, listPairings(pairings)),
		})
		postWindowStatus(win, text, ui.IntentPositive)
		return false
	case "stampSettings":
		// F6 §6: the stamp window, reachable from Settings as well as
		// from the main window. Settings stays open behind it, owned by
		// it and inert until it is answered — without the ownership it
		// was drawn over the new window entirely, which is what made
		// the whole program look dead (D-129).
		// No document and no certificate: Settings asks for one when the
		// person presses Place, because there is nothing to line a stamp
		// up against otherwise (F6b §4).
		if _, ok := runStampWindow(currentConfig(cfg), localeOf(cfg), win.Handle()); !ok {
			slog.Debug("settings: the stamp window was closed without saving")
		}
		return false
	case "checkUpdatesNow":
		// SPEC §15.2's check, on demand. It is the same check the daily
		// one runs and it has the same power: it can open the window
		// that asks, and nothing else.
		//
		// On a goroutine, and the button says so first. A check is one
		// or two HTTP requests with a 20-second ceiling on each, and
		// running it on this loop would freeze the settings window for
		// as long as a slow server took — the exact defect D-129
		// recorded, where a window that stops answering reads as a
		// program that has died.
		postWindowStatus(win, c.T("settings.update_checking"), ui.IntentCaution)
		go checkForUpdatesFromSettings(win, c, cfg)
		return false
	default:
		return false
	}
}
func buildSettingsInit(c *i18n.Catalogue, cfg config.Config, pairings []api.Pairing) map[string]any {
	return map[string]any{
		"type": "init",
		"strings": map[string]string{
			"settings.window_title":                   c.T("settings.window_title"),
			"settings.language_label":                 c.T("settings.language_label"),
			"settings.start_with_windows":             c.T("settings.start_with_windows"),
			"settings.tsa_label":                      c.T("settings.tsa_label"),
			"settings.tsa_custom_url_placeholder":     c.T("settings.tsa_custom_url_placeholder"),
			"settings.tsa_preset_freetsa":             c.T("settings.tsa_preset_freetsa"),
			"settings.tsa_preset_freetsa_warning":     c.T("settings.tsa_preset_freetsa_warning"),
			"settings.tsa_preset_rsgov":               c.T("settings.tsa_preset_rsgov"),
			"settings.tsa_preset_rsgov_note":          c.T("settings.tsa_preset_rsgov_note"),
			"settings.tsa_url_label":                  c.T("settings.tsa_url_label"),
			"settings.export_choose_folder":           c.T("settings.export_choose_folder"),
			"settings.tsa_user_label":                 c.T("settings.tsa_user_label"),
			"settings.tsa_password_label":             c.T("settings.tsa_password_label"),
			"settings.tsa_client_cert_label":          c.T("settings.tsa_client_cert_label"),
			"settings.tsa_client_cert_placeholder":    c.T("settings.tsa_client_cert_placeholder"),
			"settings.tsa_client_cert_password_label": c.T("settings.tsa_client_cert_password_label"),
			"settings.output_suffix_label":            c.T("settings.output_suffix_label"),
			"settings.output_folder_label":            c.T("settings.output_folder_label"),
			"settings.output_folder_default":          c.T("settings.output_folder_default"),
			"settings.explorer_menu":                  c.T("settings.explorer_menu"),
			"settings.document_signing":               c.T("settings.document_signing"),
			"settings.document_signing_hint":          c.T("settings.document_signing_hint"),
			"settings.certificate_listing":            c.T("settings.certificate_listing"),
			"settings.certificate_listing_hint":       c.T("settings.certificate_listing_hint"),
			"settings.stamp_settings":                 c.T("settings.stamp_settings"),
			"settings.signature_level_label":          c.T("settings.signature_level_label"),
			"settings.level_bb":                       c.T("settings.level_bb"),
			"settings.level_bt":                       c.T("settings.level_bt"),
			"settings.level_blt":                      c.T("settings.level_blt"),
			"settings.export_audit_log":               c.T("settings.export_audit_log"),
			"settings.check_updates_now":              c.T("settings.check_updates_now"),
			"settings.check_updates_daily":            c.T("settings.check_updates_daily"),
			"settings.version_label":                  c.T("settings.version_label"),
			"settings.copy":                           c.T("settings.copy"),
			"settings.save":                           c.T("settings.save"),
			"settings.close":                          c.T("settings.close"),
			"settings.pairings_label":                 c.T("settings.pairings_label"),
			"settings.pairings_empty":                 c.T("settings.pairings_empty"),
			"settings.pairings_revoke":                c.T("settings.pairings_revoke"),
		},
		"model": map[string]any{
			"tsaPresets":            tsaPresets(),
			"locale":                cfg.Locale,
			"startWithWindows":      cfg.StartWithWindows,
			"tsaURL":                cfg.TSAURL,
			"tsaUser":               cfg.TSAUser,
			"tsaPassword":           cfg.TSAPassword,
			"tsaClientCertPath":     cfg.TSAClientCertPath,
			"tsaClientCertPassword": cfg.TSAClientCertPassword,
			"outputSuffix":          cfg.OutputSuffix,
			"outputFolder":          cfg.OutputFolder,
			"explorerMenu":          cfg.ExplorerMenuEnabled,
			"documentSigning":       cfg.DocumentSigningEnabled,
			"certificateListing":    cfg.CertificateListingEnabled,
			"signatureLevel":        cfg.SignatureLevel,
			"checkUpdatesDaily":     cfg.UpdateCheckEnabled,
			"version":               version,
			"pairings":              jsPairings(c, pairings),
		},
	}
}

// tsaPresetFreeTSA and tsaPresetRSGOV are Task 1c's two named timestamp
// authorities. Neither is a default: config.Default's TSAURL is empty
// and stays empty, and neither radio is preselected — these are
// shortcuts for filling in the URL field, not a policy about which
// authority to use (Task 1a).
//
// freetsa.org's endpoint was verified directly: one RFC 3161 request
// over SHA-256 returned HTTP 200, Content-Type
// application/timestamp-reply, PKIStatus granted.
//
// tsa.gov.rs is the RS-GOV TSA of the Office for Information
// Technologies and eGovernment — the authority that timestamped the
// real MUP-signed document in testdata/pdfs/local/mup.pdf (signer
// certificate CN "RS-GOV TSA-3 200103828", TSTInfo policy
// 1.3.6.1.4.1.55016.1.1.0, exactly SPEC §12.7's OID). The address is
// the one its operator publishes on the eUprava service page for the
// service; probing it is recorded in docs/decisions.md, including that
// it is contract-gated and was returning a maintenance page when
// probed.
const (
	tsaPresetFreeTSA = "https://freetsa.org/tsr"
	tsaPresetRSGOV   = "https://tsa.gov.rs/"
)

// tsaPresets is what the settings page renders as its two preset rows.
// The URL travels to the page so the page never has to know one, and
// so a preset row and the free-text field can never disagree about
// what a preset means.
func tsaPresets() []map[string]string {
	return []map[string]string{
		{"id": "freetsa", "url": tsaPresetFreeTSA},
		{"id": "rsgov", "url": tsaPresetRSGOV},
	}
}

// localeOf is the configured interface language, for a caller that has
// a Config and needs the locale it implies.
func localeOf(cfg config.Config) string { return cfg.Locale }
