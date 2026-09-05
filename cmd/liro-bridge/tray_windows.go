//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// webView2ExitGrace is a brief pause before the tray subcommand returns
// (and the process reaches ExitProcess), given only after at least one
// WebView2 window has actually been opened and closed during this
// session (Task 8, F5 first-real-run review). ui.Window.Close already
// waits for this process's own COM teardown to finish before returning
// (D-0xx), but the WebView2 runtime's own browser process — a separate
// executable — tears itself down asynchronously once that happens, on
// its own schedule; this is what gives it a little more of that
// schedule to run before this process disappears out from under it,
// which is what actually produces its "Failed to unregister class
// Chrome_WidgetWin_0" console line (emitted by that browser process,
// not by any code in this repository) when the two race. Not applied
// when no such window was ever opened this session — a user who only
// ever used the tray menu's Quit should never wait for anything.
const webView2ExitGrace = 400 * time.Millisecond

// runTray implements F5 §3's product shape: the agent starts minimised
// to tray with no window, and stays running until Quit.
//
// cfg is what the configuration said when the process started, and is
// used as nothing but a fallback from here on. The tray outlives every
// window it opens and every save those windows make, so each of them
// asks the file what the configuration is *now* (currentConfig) rather
// than being handed a copy taken at startup. Handing that copy on is
// what made a saved language come back as the old one: the value
// reached disk correctly and was then never read again.
func runTray(cfg config.Config, version string) int {
	c := i18n.Load(currentConfig(cfg).Locale)
	quit := make(chan struct{})
	openedAWindow := false

	// F6 §2: the entry is on by default, so it is registered when the
	// agent starts rather than only when Settings is opened and saved.
	// Registering is idempotent and also refreshes the label after a
	// language change.
	if err := applyExplorerMenu(cfg, c); err != nil {
		slog.Warn("tray: could not apply the Explorer context menu setting", "error", err)
	}

	t, err := ui.NewTray(ui.TrayOptions{
		Version: version,
		Labels: func() ui.TrayLabels {
			c := i18n.Load(currentConfig(cfg).Locale)
			return ui.TrayLabels{
				Open:         c.T("tray.open_window"),
				Settings:     c.T("tray.settings"),
				Certificates: c.T("tray.certificates"),
				AuditLog:     c.T("tray.audit_log"),
				Quit:         c.T("tray.quit"),
			}
		},
		OnOpen: func() {
			// F5 §3's "left click opens the main window", finally
			// pointing at one (F6 §1). Opened empty: the drop zone and
			// Browse are how documents get in from here.
			openedAWindow = true
			now := currentConfig(cfg)
			if code := runMainWindow(context.Background(), now, now.Locale, nil); code != 0 {
				slog.Warn("tray: the main window returned an error", "code", code)
			}
		},
		OnSettings: func() {
			openedAWindow = true
			if err := runSettingsWindow(cfg, 0); err != nil {
				slog.Warn("tray: settings window failed", "error", err)
			}
		},
		OnCertificates: func() {
			openedAWindow = true
			if err := runCertificatesWindow(currentConfig(cfg).Locale); err != nil {
				slog.Warn("tray: certificates window failed", "error", err)
			}
		},
		OnAuditLog: func() {
			openedAWindow = true
			if err := runAuditLogWindow(currentConfig(cfg).Locale); err != nil {
				slog.Warn("tray: audit log window failed", "error", err)
			}
		},
		OnQuit: func() { close(quit) },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "liro-bridge: tray:", err)
		return 1
	}
	defer func() { _ = t.Close() }()

	<-quit
	if openedAWindow {
		time.Sleep(webView2ExitGrace)
	}
	return 0
}

// settingsFormState is what settings.js's __liroCollectState() returns,
// read back via Window.Eval (never a page->Go message — see
// internal/ui's Window.Eval doc comment).
type settingsFormState struct {
	Action                string `json:"action"`
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
func settingsOnOpening(fallback config.Config) (*i18n.Catalogue, config.Config, map[string]any) {
	cfg := currentConfig(fallback)
	c := i18n.Load(cfg.Locale)
	return c, cfg, buildSettingsInit(c, cfg)
}

// owner is the window Settings was opened from, or zero when it was
// opened from the tray and stands on its own.
//
// fallback is only that: the window reads the configuration file itself,
// here, at the moment it opens, and takes both the form's values and its
// own interface language from what it finds. Its callers are long-lived
// — the tray for the life of the process, the consent window for the
// life of a batch — and a Config handed down from one of them says what
// was true when *that* started, which is how a language saved a moment
// ago came back as the old one on reopening.
func runSettingsWindow(fallback config.Config, owner uintptr) error {
	c, cfg, init := settingsOnOpening(fallback)
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
			if handleSettingsAction(win, c, cfg, state) {
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
func handleSettingsAction(win ui.Window, c *i18n.Catalogue, cfg config.Config, state settingsFormState) bool {
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
	case "stampSettings":
		// F6 §6: the stamp window, reachable from Settings as well as
		// from the main window. Settings stays open behind it, owned by
		// it and inert until it is answered — without the ownership it
		// was drawn over the new window entirely, which is what made
		// the whole program look dead (D-129).
		if _, ok := runStampWindow(currentConfig(cfg), localeOf(cfg), stampRoleSettings, win.Handle()); !ok {
			slog.Debug("settings: the stamp window was closed without saving")
		}
		return false
	case "checkUpdatesNow":
		// SPEC §15.2's update channel — the embedded public key, the
		// GitHub Releases check, the signature verification and the
		// "ask before installing" prompt — is built in F10 with the
		// packaging it belongs to. Until then the button says exactly
		// that, on screen, in the user's own language: a button that
		// looks enabled and produces no response reads as a broken
		// program, which is the whole finding this replaces.
		slog.Info("settings: check-for-updates requested (the update channel arrives with packaging, F10)")
		postWindowStatus(win, c.T("settings.updates_not_available"), ui.IntentWarning)
		return false
	default:
		return false
	}
}

// postWindowStatus shows one line of feedback under a window's action
// buttons. intent picks its colour family, never a colour (D-093).
//
// Named for windows rather than for Settings because the audit log
// window renders the same payload in the same place: both have an
// Export button, and both have to say where the files went.
func postWindowStatus(win ui.Window, text string, intent ui.Intent) {
	postWindowStatusFiles(win, text, nil, intent)
}

// exportedFile is one file an action wrote: its name, and in one line,
// in the person's own language, what that file is.
type exportedFile struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// postWindowStatusFiles is postWindowStatus for an action that wrote
// files. The heading says where they went; each file then names itself
// and says what it is, because two files appearing in a folder with
// nothing on screen about either of them is how the verification report
// came to be opened and asked about.
func postWindowStatusFiles(win ui.Window, text string, files []exportedFile, intent ui.Intent) {
	_ = win.PostJSON(map[string]any{
		"type": "status",
		"status": map[string]any{
			"text":   text,
			"files":  files,
			"intent": string(intent),
		},
	})
}

// exportAuditLogNow writes the audit log and its verification report to
// a folder the user chooses (Task 3), and says on screen where they
// went. A cancelled folder chooser is silent: the user withdrew the
// request, and there is nothing to report about it.
//
// One implementation, two buttons. Settings has had this since F5; the
// audit log window now offers the same action, because that is where a
// person is when they decide they want the log — walking to Settings
// to export what is already on screen is a trip with no purpose. The
// format is the one already in use, and the one this log should keep:
// one JSON object per line plus a verification report, append-only,
// readable years later without this program, and checkable against the
// hash chain by anyone.
func exportAuditLogNow(win ui.Window, c *i18n.Catalogue) {
	dir := filepath.Join(platform.ConfigDir("windows", platform.OSEnv), "audit")
	store, err := audit.NewStore(dir)
	if err != nil {
		slog.Warn("settings: opening audit store for export failed", "error", err)
		postWindowStatus(win, c.T("settings.export_failed"), ui.IntentNegative)
		return
	}

	target, chosen, err := ui.ChooseFolder(win.Handle(), c.T("settings.export_choose_folder"), "")
	if err != nil {
		slog.Warn("settings: choosing an export folder failed", "error", err)
		postWindowStatus(win, c.T("settings.export_failed"), ui.IntentNegative)
		return
	}
	if !chosen {
		return
	}

	stamp := time.Now().Format("20060102-150405")
	entriesName := "liro-audit-" + stamp + ".jsonl"
	reportName := "liro-audit-" + stamp + "-report.json"
	entriesPath := filepath.Join(target, entriesName)
	reportPath := filepath.Join(target, reportName)
	report, err := store.Export(entriesPath, reportPath)
	if err != nil {
		// Export writes both files even when the chain does not verify,
		// and then reports that as an error — which is a finding about
		// the log, not a failure to export it. Say which happened.
		if report.EntryCount > 0 && !report.Result.OK {
			slog.Warn("settings: exported audit log failed chain verification", "brokenAt", report.Result.BrokenAt)
			postWindowStatusFiles(win,
				fmt.Sprintf(c.T("settings.export_done"), target),
				exportedFileLines(c, entriesName, reportName, report),
				ui.IntentNegative)
			return
		}
		slog.Warn("settings: exporting audit log failed", "error", err)
		postWindowStatus(win, c.T("settings.export_failed"), ui.IntentNegative)
		return
	}
	slog.Info("settings: audit log exported", "entries", entriesPath, "report", reportPath)
	postWindowStatusFiles(win,
		fmt.Sprintf(c.T("settings.export_done"), target),
		exportedFileLines(c, entriesName, reportName, report),
		ui.IntentPositive)
}

// exportedFileLines names the two files an export writes and says, in
// one line each, what they are.
//
// The report was opened by the owner and asked about, because on screen
// nothing distinguished it from the log beside it. It is the hash-chain
// check (SPEC §6.7) and its whole value is the case where it fails, so
// that case says plainly that the log was altered and where — never
// "OK: false, BrokenAt: 42", which is what the file itself says and
// what nobody should have to read.
//
// BrokenAt is an index into the exported entries, which the log file
// holds one per line; it is reported as the line number a person would
// count to, so the sentence points at something they can actually find.
func exportedFileLines(c *i18n.Catalogue, entriesName, reportName string, report audit.ExportReport) []exportedFile {
	entries := fmt.Sprintf(c.T("settings.export_entries"), report.EntryCount)
	if report.EntryCount == 1 {
		entries = c.T("settings.export_entries_one")
	}
	check := c.T("settings.export_check_ok")
	if !report.Result.OK {
		check = fmt.Sprintf(c.T("settings.export_check_broken"), report.Result.BrokenAt+1)
	}
	return []exportedFile{
		{Name: entriesName, Detail: entries},
		{Name: reportName, Detail: check},
	}
}

func buildSettingsInit(c *i18n.Catalogue, cfg config.Config) map[string]any {
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
			"signatureLevel":        cfg.SignatureLevel,
			"checkUpdatesDaily":     cfg.UpdateCheckEnabled,
			"version":               version,
		},
	}
}

func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
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

// applyExplorerMenu makes the Explorer context-menu entry match the
// configuration (F6 §2). Registering is idempotent, so this is also
// what keeps the menu label in the user's current language after they
// change it.
func applyExplorerMenu(cfg config.Config, c *i18n.Catalogue) error {
	menu := platform.NewShellMenu()
	if !cfg.ExplorerMenuEnabled {
		return menu.Unregister()
	}
	exe := exePath()
	// The real Liro mark, not the executable: the binary carries no
	// icon resource of its own until F10 builds one, so pointing the
	// menu at it would show the generic Windows application icon.
	// A failure to extract is not a reason to leave the entry
	// unregistered — an entry with the default icon still works.
	icon, err := ui.IconFilePath()
	if err != nil {
		slog.Warn("settings: could not extract the menu icon, falling back to the executable", "error", err)
		icon = exe
	}
	return menu.Register(c.T("settings.explorer_menu_verb"), exe, icon)
}
