package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// staleWarningAge is how long the agent can go without a *successful
// fetch* before a visible warning appears (Task 9), independent of
// whether it still works — it always does, offline or not (SPEC §11.1).
// This is deliberately not measured against the list's own issue date:
// the Ministry only republishes when something changes, so a list issued
// months ago may well still be the current one — its age is not a
// problem. What is a problem is this agent going a long time unable to
// confirm that.
const staleWarningAge = 30 * 24 * time.Hour

// thumbprintSuffixLen is how much of the SHA-1 thumbprint is shown per
// row — the last 8 characters, which is what distinguishes two
// certificates with an identical Subject (SPEC §11.5, F1 §6.2).
const thumbprintSuffixLen = 8

func thumbprintSuffix(tp string) string {
	if len(tp) <= thumbprintSuffixLen {
		return tp
	}
	return "…" + tp[len(tp)-thumbprintSuffixLen:]
}

// fprintf and fprintln discard the write error from every call site
// below. The destination is always the CLI's own stdout-shaped output
// writer; there is no recovery action for a failed terminal write, and
// letting each of the many calls below ignore it individually (rather
// than through one documented pair of helpers) is what errcheck would
// otherwise flag repeatedly for no actionable benefit.
func fprintf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
func fprintln(w io.Writer, a ...any)               { _, _ = fmt.Fprintln(w, a...) }

// RenderText writes the human-readable report described in F1 §6.1. all
// includes rows that are hidden by default (CertRow.Hidden).
func RenderText(w io.Writer, report Report, c *i18n.Catalogue, now time.Time, all bool) {
	renderReaders(w, report.Readers, c)
	fprintln(w)

	visible := visibleRows(report.Certificates, all)
	fprintf(w, c.T("certs.certificates_heading")+"\n", len(visible))
	fprintln(w)
	for i, row := range visible {
		renderRow(w, i+1, row, c)
		fprintln(w)
	}

	renderTSL(w, report.TSL, c, now)
}

func visibleRows(rows []CertRow, all bool) []CertRow {
	if all {
		return rows
	}
	out := make([]CertRow, 0, len(rows))
	for _, r := range rows {
		if !r.Hidden() {
			out = append(out, r)
		}
	}
	return out
}

func renderReaders(w io.Writer, readers []platform.ReaderState, c *i18n.Catalogue) {
	if len(readers) == 0 {
		fprintln(w, c.T("certs.readers_none"))
		return
	}
	fprintf(w, c.T("certs.readers_heading")+"\n", len(readers))
	for _, r := range readers {
		state := c.T("certs.card_absent")
		if r.CardPresent {
			state = c.T("certs.card_present")
		}
		// The reader name is data from the OS, displayed verbatim in
		// whatever script it arrives in (SPEC §9.3) — never localised.
		fprintf(w, "  %s — %s\n", r.Name, state)
	}
}

func renderRow(w io.Writer, index int, row CertRow, c *i18n.Catalogue) {
	status := c.T("certs.not_usable")
	mark := "✗"
	if row.Info.Usable {
		status = c.T("certs.usable")
		mark = "✓"
	}
	// Subject data is displayed in its own script regardless of
	// interface locale (SPEC §9.3) — never passed through T().
	name := row.Info.Subject.DisplayName
	if row.Info.IsTestKey {
		// SPEC §16.6: every soft-token signature is visibly marked as a
		// test signature — this is that mark's first appearance, in the
		// only UI surface F2 has.
		name = name + " " + c.T("certs.test_key_marker")
	}
	fprintf(w, "  [%d] %s %s %s\n", index, name, mark, status)

	fprintf(w, "      %s      %s\n", c.T("certs.purpose_label"), purposeLabel(row.Info.Purpose, c))
	fprintf(w, "      %s       %s\n", c.T("certs.issuer_label"), row.Info.IssuerCN)
	fprintf(w, "      %s    %s\n", c.T("certs.qualified_label"), qualifiedLabel(row.Info, c))
	if !row.Info.Usable && row.Info.NotUsableReason != "" {
		fprintf(w, "      %s       %s\n", c.T("certs.reason_label"), reasonLabel(row.Info.NotUsableReason, c))
	}
	fprintf(w, "      %s        %s\n", c.T("certs.valid_label"),
		fmt.Sprintf(c.T("certs.valid_range"), row.Info.NotBefore.Format("2006-01-02"), row.Info.NotAfter.Format("2006-01-02")))
	fprintf(w, "      %s   %s\n", c.T("certs.thumbprint_label"), thumbprintSuffix(row.Info.Thumbprint))
	fprintf(w, "      %s      %s\n", c.T("certs.storage_label"), storageLabel(row.OnHardware, c))
}

func purposeLabel(p classify.Purpose, c *i18n.Catalogue) string {
	switch p {
	case classify.PurposeSigning:
		return c.T("certs.purpose_signing")
	case classify.PurposeAuthentication:
		return c.T("certs.purpose_authentication")
	default:
		return c.T("certs.purpose_unknown")
	}
}

func qualifiedLabel(info classify.Info, c *i18n.Catalogue) string {
	switch info.Qualification {
	case classify.QualificationQualified:
		if info.OnQSCD {
			return c.T("certs.qualified_yes_qscd")
		}
		return c.T("certs.qualified_yes")
	case classify.QualificationNotQualified:
		return c.T("certs.qualified_no")
	default:
		return c.T("certs.qualified_unknown")
	}
}

func reasonLabel(r errs.Code, c *i18n.Catalogue) string {
	switch r {
	case errs.CodeCertExpired:
		return c.T("certs.reason_expired")
	case errs.CodeCardNotPresent:
		return c.T("certs.reason_card_not_present")
	default:
		return c.T("certs.reason_not_usable")
	}
}

func storageLabel(onHardware bool, c *i18n.Catalogue) string {
	if onHardware {
		return c.T("certs.storage_smart_card")
	}
	return c.T("certs.storage_software")
}

func renderTSL(w io.Writer, prov tsl.Provenance, c *i18n.Catalogue, now time.Time) {
	age := now.Sub(prov.IssuedAt)
	ageDays := int(age.Hours() / 24)

	var source string
	switch prov.Source {
	case tsl.SourceEmbedded:
		source = c.T("certs.tsl_source_embedded")
	case tsl.SourceCache:
		source = c.T("certs.tsl_source_cache")
	case tsl.SourceNetwork:
		source = c.T("certs.tsl_source_network")
	}

	fprintf(w, c.T("certs.tsl_heading")+"\n", prov.Sequence, prov.IssuedAt.Format("2006-01-02"), ageDays, source)
	if tslNeedsStaleWarning(prov, now) {
		fprintln(w, strings.TrimSpace(c.T("certs.tsl_stale_warning")))
	}
}

// tslNeedsStaleWarning reports whether the agent has gone too long
// without successfully fetching the Trusted List (Task 9) — never
// whether the *published* list itself is old. A list issued long ago can
// still be the current one; what actually needs a warning is this agent
// being unable to confirm that, either because it is still running on
// the embedded seed (no fetch has ever succeeded) or because its last
// successful fetch is more than staleWarningAge old.
func tslNeedsStaleWarning(prov tsl.Provenance, now time.Time) bool {
	if prov.Source == tsl.SourceEmbedded {
		return true
	}
	return now.Sub(prov.FetchedAt) > staleWarningAge
}
