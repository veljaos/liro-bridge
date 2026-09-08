//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
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

	// One pairing store for the life of the process (see openPairings):
	// the settings window revokes through it, and the protocol
	// authenticates against it, and F7 2.4 makes revoking immediate.
	pairings := openPairingsOrNil()
	quit := make(chan struct{})
	openedAWindow := false

	// F6 §2: the entry is on by default, so it is registered when the
	// agent starts rather than only when Settings is opened and saved.
	// Registering is idempotent and also refreshes the label after a
	// language change.
	if err := applyExplorerMenu(cfg, c); err != nil {
		slog.Warn("tray: could not apply the Explorer context menu setting", "error", err)
	}

	// F7 §4: the loopback listener, the discovery file and the
	// protocol's own routes. It lives here because the tray is the
	// agent — the process that is running when nobody is looking at a
	// window — and because one machine's user session must have exactly
	// one of it (SPEC §14.1).
	protocol, protocolErr := startProtocol(cfg, version, pairings)
	if protocolErr != nil {
		// Not fatal. An agent that cannot serve the protocol can still
		// sign for the person sitting at it, and saying so in the log is
		// more use than refusing to start.
		slog.Error("tray: the protocol could not be started", "error", protocolErr)
	}
	defer protocol.stop()

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
			if err := runSettingsWindow(cfg, 0, pairings); err != nil {
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
	switch {
	case !report.Result.OK && report.Result.BrokenAt >= 0:
		check = fmt.Sprintf(c.T("settings.export_check_broken"), report.Result.BrokenAt+1)
	case !report.Result.OK:
		// Not tampered with — nothing failed its own chain's walk — but
		// a chain could not be read to its end, which is a different
		// finding and needs a different sentence. Saying "passed" here
		// and then "not all intact" in the same line, which is what a
		// two-way OK/broken split produced, is a contradiction on the
		// one screen that must not have one.
		check = c.T("settings.export_check_incomplete")
	}
	if chains := chainsSummary(c, report); chains != "" {
		// A log that has survived a break holds more than one chain, and
		// saying only "OK" or "not OK" about the whole of it would hide
		// both which chain broke and that the others are intact. Named
		// only when there is more than one: an ordinary log has one, and
		// saying so every time is noise.
		check += " — " + chains
	}
	return []exportedFile{
		{Name: entriesName, Detail: entries},
		{Name: reportName, Detail: check},
	}
}

// chainsSummary says how many chains the exported log holds and where
// they broke — "three chains, each intact, breaks: 12.03. (an entry
// could not be read), 04.09. (the file could not be read)".
//
// Empty for a log with one chain, which is every log that has never
// been interrupted.
func chainsSummary(c *i18n.Catalogue, report audit.ExportReport) string {
	if len(report.Chains) < 2 {
		return ""
	}
	intact := c.T("settings.export_chains_all_intact")
	for _, ch := range report.Chains {
		if !ch.Result.OK || ch.TruncatedReason != "" {
			intact = c.T("settings.export_chains_not_all_intact")
			break
		}
	}
	line := fmt.Sprintf(c.T(chainCountKey(len(report.Chains))), len(report.Chains), intact)

	var breaks []string
	for _, ch := range report.Discontinuities() {
		when := ""
		if !ch.FirstAt.IsZero() {
			when = ch.FirstAt.Local().Format("02.01.2006.")
		}
		breaks = append(breaks, fmt.Sprintf(c.T("settings.export_break_at"),
			when, breakReasonText(c, ch.Discontinuity.Reason)))
	}
	if len(breaks) > 0 {
		line += ", " + fmt.Sprintf(c.T("settings.export_breaks"), strings.Join(breaks, ", "))
	}
	return line
}

// chainCountKey picks the plural form for a number of chains.
//
// Serbian has two plural stems where English has one: 2, 3 and 4 take
// "lanca" and everything else "lanaca", with the usual exception that
// 12-14 behave like the larger group and a number ending in 2-4 above
// that takes the smaller one again. Getting it wrong reads as a machine
// wrote it, which on the one screen that reports the integrity of an
// audit log is not the impression to give.
func chainCountKey(n int) string {
	last, lastTwo := n%10, n%100
	if last >= 2 && last <= 4 && (lastTwo < 12 || lastTwo > 14) {
		return "settings.export_chains_few"
	}
	return "settings.export_chains_many"
}

// breakReasonText turns an audit.BreakReason into words. The reason is
// an enumerated value on purpose (SPEC §7 — codes, never prose, and
// audit.Entry's field set is an allow-list); this is the one place it
// becomes a sentence, in the reader's own language.
func breakReasonText(c *i18n.Catalogue, r audit.BreakReason) string {
	switch r {
	case audit.BreakUnparseable:
		return c.T("settings.export_break_unparseable")
	case audit.BreakUnreachable:
		return c.T("settings.export_break_unreachable")
	default:
		return string(r)
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
