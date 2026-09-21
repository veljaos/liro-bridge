package main

// Exporting the audit log, and saying what came out — including the
// discontinuities SPEC §6.7 requires a person to be told about once.
// It is reached from the Settings window and is not the Settings
// window, and it has nothing to do with the tray, where it lived.

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

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
	case audit.BreakUnsound:
		return c.T("settings.export_break_unsound")
	case audit.BreakUnguarded:
		return c.T("settings.export_break_unguarded")
	default:
		return string(r)
	}
}
