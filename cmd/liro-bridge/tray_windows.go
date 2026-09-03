//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// runTray implements F5 §3's product shape: the agent starts minimised
// to tray with no window, and stays running until Quit.
func runTray(cfg config.Config, version, locale string) int {
	c := i18n.Load(locale)
	quit := make(chan struct{})

	t, err := ui.NewTray(ui.TrayOptions{
		Version: version,
		Labels: ui.TrayLabels{
			Open:         c.T("tray.open"),
			Settings:     c.T("tray.settings"),
			Certificates: c.T("tray.certificates"),
			AuditLog:     c.T("tray.audit_log"),
			Quit:         c.T("tray.quit"),
		},
		OnOpen: func() {
			// F5 §3: "Left click opens the main window (in this phase, a
			// placeholder; F6 fills it)." There is nothing to show yet.
			slog.Info("tray: open requested (placeholder — F6 supplies the main window)")
		},
		OnSettings: func() {
			if err := runSettingsWindow(cfg, locale); err != nil {
				slog.Warn("tray: settings window failed", "error", err)
			}
		},
		OnCertificates: func() {
			if err := runCertificatesWindow(locale); err != nil {
				slog.Warn("tray: certificates window failed", "error", err)
			}
		},
		OnAuditLog: func() {
			if err := runAuditLogWindow(locale); err != nil {
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
	SignatureLevel        string `json:"signatureLevel"`
	CheckUpdatesDaily     bool   `json:"checkUpdatesDaily"`
}

func runSettingsWindow(cfg config.Config, locale string) error {
	c := i18n.Load(locale)
	messages := make(chan ui.Message, 8)

	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("settings.window_title"),
		Width:       520,
		Height:      480,
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

	if err := win.PostJSON(buildSettingsInit(c, cfg)); err != nil {
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
			if handleSettingsAction(state) {
				return nil
			}
		}
	}
}

// handleSettingsAction returns true when the settings window should
// close after handling state.Action.
func handleSettingsAction(state settingsFormState) bool {
	switch state.Action {
	case "save":
		newCfg := config.Config{
			Locale:                state.Locale,
			LogLevel:              "info",
			PortRangeStart:        17580,
			PortRangeEnd:          17590,
			StartWithWindows:      state.StartWithWindows,
			TSAURL:                state.TSAURL,
			TSAUser:               state.TSAUser,
			TSAPassword:           state.TSAPassword,
			TSAClientCertPath:     state.TSAClientCertPath,
			TSAClientCertPassword: state.TSAClientCertPassword,
			OutputSuffix:          state.OutputSuffix,
			SignatureLevel:        state.SignatureLevel,
			UpdateCheckEnabled:    state.CheckUpdatesDaily,
		}
		if err := config.Save(config.DefaultPath(), newCfg); err != nil {
			slog.Warn("settings: saving config failed", "error", err)
			return false
		}
		if err := platform.NewAutostart().SetEnabled(state.StartWithWindows, exePath()); err != nil {
			slog.Warn("settings: updating autostart registration failed", "error", err)
		}
		return true
	case "exportAuditLog":
		exportAuditLogNow()
		return false
	case "checkUpdatesNow":
		// SPEC §15.2's update channel is a later phase; nothing to do yet
		// beyond acknowledging the request was received.
		slog.Info("settings: check-for-updates requested (not implemented before F10)")
		return false
	default:
		return false
	}
}

func exportAuditLogNow() {
	dir := filepath.Join(platform.ConfigDir("windows", platform.OSEnv), "audit")
	store, err := audit.NewStore(dir)
	if err != nil {
		slog.Warn("settings: opening audit store for export failed", "error", err)
		return
	}
	exportDir := filepath.Join(dir, "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		slog.Warn("settings: creating export directory failed", "error", err)
		return
	}
	stamp := time.Now().Format("20060102-150405")
	entriesPath := filepath.Join(exportDir, "audit-"+stamp+".jsonl")
	reportPath := filepath.Join(exportDir, "audit-"+stamp+"-report.json")
	if _, err := store.Export(entriesPath, reportPath); err != nil {
		slog.Warn("settings: exporting audit log failed", "error", err)
		return
	}
	slog.Info("settings: audit log exported", "entries", entriesPath, "report", reportPath)
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
			"settings.tsa_user_label":                 c.T("settings.tsa_user_label"),
			"settings.tsa_password_label":             c.T("settings.tsa_password_label"),
			"settings.tsa_client_cert_label":          c.T("settings.tsa_client_cert_label"),
			"settings.tsa_client_cert_placeholder":    c.T("settings.tsa_client_cert_placeholder"),
			"settings.tsa_client_cert_password_label": c.T("settings.tsa_client_cert_password_label"),
			"settings.output_suffix_label":            c.T("settings.output_suffix_label"),
			"settings.signature_level_label":          c.T("settings.signature_level_label"),
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
			"locale":                cfg.Locale,
			"startWithWindows":      cfg.StartWithWindows,
			"tsaURL":                cfg.TSAURL,
			"tsaUser":               cfg.TSAUser,
			"tsaPassword":           cfg.TSAPassword,
			"tsaClientCertPath":     cfg.TSAClientCertPath,
			"tsaClientCertPassword": cfg.TSAClientCertPassword,
			"outputSuffix":          cfg.OutputSuffix,
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
