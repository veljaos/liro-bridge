//go:build windows

package main

// runAuditLogWindow implements the tray's "View audit log" item
// (Task 4, F5 review): previously this logged the audit directory path
// and did nothing visible — the same silently-inert defect
// certificates_windows.go fixes for "Certificates". It is a plain,
// read-only list (F5 review's own suggestion), newest entry first,
// reusing the same *audit.Store the Settings window's "Export audit
// log" button already opens (tray_windows.go's exportAuditLogNow).
import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

func runAuditLogWindow(locale string) error {
	c := i18n.Load(locale)

	dir := filepath.Join(platform.ConfigDir("windows", platform.OSEnv), "audit")
	store, err := audit.NewStore(dir)
	if err != nil {
		return err
	}
	entries, err := store.All()
	if err != nil {
		return err
	}

	messages := make(chan ui.Message, 8)
	win, err := ui.NewWindow(ui.Options{
		Title: c.T("auditwindow.title"),
		Width: 460,
		// Task 1 (F5 second-real-run review): every entry gained a
		// signature-level line, so 440 points no longer fits four of
		// them plus the Close button without the list scrolling.
		Height:      520,
		AlwaysOnTop: true,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/auditlog.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		return err
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(buildAuditLogInit(c, entries)); err != nil {
		return err
	}

	for {
		msg := <-messages
		if msg.Type == ui.MessageTypeCancel {
			return nil
		}
		slog.Warn("audit log window: unexpected message", "type", msg.Type)
	}
}

type jsAuditEntry struct {
	TimestampText     string `json:"timestampText"`
	Outcome           string `json:"outcome"`
	OutcomeText       string `json:"outcomeText"`
	OutcomeIntent     string `json:"outcomeIntent"`
	ApplicationText   string `json:"applicationText"`
	DocumentCountText string `json:"documentCountText"`
	ThumbprintTail    string `json:"thumbprintTail"`
	IsTestKey         bool   `json:"isTestKey"`
	TestKeyLabel      string `json:"testKeyLabel"`
	LevelText         string `json:"levelText"`
	LevelIntent       string `json:"levelIntent"`
}

// outcomeIntent maps an audit.Outcome to its Task 6 colour family:
// approved -> positive, denied -> warning, failed -> negative, partial
// -> caution. The page renders this as the CSS class
// "liro-outcome-<intent>" (intents.css) — the colour itself always
// comes from IntentFamilyColor (scripts/synctokens), never chosen
// here.
func outcomeIntent(o audit.Outcome) ui.Intent {
	switch o {
	case audit.OutcomeApproved:
		return ui.IntentPositive
	case audit.OutcomeDenied:
		return ui.IntentWarning
	case audit.OutcomeFailed:
		return ui.IntentNegative
	case audit.OutcomePartial:
		return ui.IntentCaution
	default:
		return ui.IntentNegative
	}
}

// outcomeText maps an audit.Outcome to its localised display text —
// this window's own small vocabulary, distinct from errs.Code (F5
// review's audit entries are not error reports, and SPEC §6.7 already
// keeps the audit log free of anything but these four outcomes).
func outcomeText(c *i18n.Catalogue, o audit.Outcome) string {
	switch o {
	case audit.OutcomeApproved:
		return c.T("auditwindow.outcome_approved")
	case audit.OutcomeDenied:
		return c.T("auditwindow.outcome_denied")
	case audit.OutcomeFailed:
		return c.T("auditwindow.outcome_failed")
	case audit.OutcomePartial:
		return c.T("auditwindow.outcome_partial")
	default:
		return string(o)
	}
}

// auditThumbprintTail mirrors internal/consent's own thumbprintTail
// (unexported there): the last 8 characters of a SHA-1 thumbprint, the
// part that actually distinguishes two certificates with an identical
// subject (SPEC §11.5).
func auditThumbprintTail(thumbprint string) string {
	const n = 8
	if len(thumbprint) <= n {
		return thumbprint
	}
	return thumbprint[len(thumbprint)-n:]
}

// auditLevelText renders the PAdES level a batch reached (Task 1, F5
// second-real-run review). B-B says what B-B means, in words, because
// "B-B" alone tells a non-specialist nothing; B-T and B-LT stand on
// their own. An entry with no level (a denied batch, or one written
// before Entry.AchievedLevel existed) renders nothing at all rather
// than a guess.
func auditLevelText(c *i18n.Catalogue, level string) string {
	switch pades.Level(level) {
	case pades.LevelBB:
		return c.T("auditwindow.level_bb")
	case pades.LevelBT, pades.LevelBLT:
		return level
	default:
		return ""
	}
}

// auditLevelIntent colours B-B in the warning family — a signature
// with no proof of when it was made — and everything else neutrally.
func auditLevelIntent(level string) string {
	if pades.Level(level) == pades.LevelBB {
		return string(ui.IntentWarning)
	}
	return ""
}

func buildAuditLogInit(c *i18n.Catalogue, entries []audit.Entry) map[string]any {
	// Newest first: a plain append-only log is written oldest-first, but
	// the window's whole point is "what happened most recently" (F5
	// review's own framing, "a plain list").
	js := make([]jsAuditEntry, len(entries))
	for i := range entries {
		src := entries[len(entries)-1-i]
		js[i] = jsAuditEntry{
			TimestampText:     src.Timestamp.Local().Format("2006-01-02 15:04:05"),
			Outcome:           string(src.Outcome),
			OutcomeText:       outcomeText(c, src.Outcome),
			OutcomeIntent:     string(outcomeIntent(src.Outcome)),
			ApplicationText:   applicationDisplayName(c, src.Application),
			DocumentCountText: fmt.Sprintf(c.T("auditwindow.document_count"), src.DocumentCount),
			ThumbprintTail:    auditThumbprintTail(src.Thumbprint),
			IsTestKey:         src.IsTestKey,
			TestKeyLabel:      c.T("certs.test_key_marker"),
			LevelText:         auditLevelText(c, src.AchievedLevel),
			LevelIntent:       auditLevelIntent(src.AchievedLevel),
		}
	}

	return map[string]any{
		"type": "init",
		"strings": map[string]string{
			"auditwindow.title":    c.T("auditwindow.title"),
			"auditwindow.empty":    c.T("auditwindow.empty"),
			"auditwindow.level_bb": c.T("auditwindow.level_bb"),
			"settings.close":       c.T("settings.close"),
		},
		"model": map[string]any{
			"entries": js,
		},
	}
}
