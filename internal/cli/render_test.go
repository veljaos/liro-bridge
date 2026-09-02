package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

func sampleReport() Report {
	return Report{
		Readers: []platform.ReaderState{{Name: "Generic Smart Card Reader Interface 0", CardPresent: true}},
		Certificates: []CertRow{
			{
				OnHardware: true,
				Info: classify.Info{
					Thumbprint:    "AABBCCDDEEFF00112233445566778899B3D1ECCE",
					Subject:       classify.Subject{DisplayName: "ВЕЉКО СТАНОЈЕВИЋ"},
					IssuerCN:      "MUP Gradjani CA 4",
					NotBefore:     time.Date(2021, 9, 23, 0, 0, 0, 0, time.UTC),
					NotAfter:      time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC),
					Qualification: classify.QualificationQualified,
					Purpose:       classify.PurposeSigning,
					OnQSCD:        true,
					Usable:        true,
				},
			},
			{
				OnHardware: true,
				Info: classify.Info{
					Thumbprint:      "1122334455667788990011223344554F21A0C7",
					Subject:         classify.Subject{DisplayName: "ВЕЉКО СТАНОЈЕВИЋ"},
					IssuerCN:        "MUP Gradjani CA 4",
					NotBefore:       time.Date(2021, 9, 23, 0, 0, 0, 0, time.UTC),
					NotAfter:        time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC),
					Qualification:   classify.QualificationNotQualified,
					Purpose:         classify.PurposeAuthentication,
					Usable:          false,
					NotUsableReason: errs.CodeCertNotUsable,
				},
			},
			{
				OnHardware: false,
				Info: classify.Info{
					Thumbprint:    "0000000000000000000000000000000000AAAA",
					Subject:       classify.Subject{DisplayName: "{GUID}"},
					Qualification: classify.QualificationNotQualified,
					Purpose:       classify.PurposeUnknown,
					Usable:        false,
				},
			},
		},
		TSL: tsl.Provenance{Source: tsl.SourceCache, Sequence: 36, IssuedAt: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)},
	}
}

func TestRenderTextThumbprintShowsLastEightCharacters(t *testing.T) {
	var buf bytes.Buffer
	RenderText(&buf, sampleReport(), i18n.Load("en"), referenceTime, false)
	if !strings.Contains(buf.String(), "…B3D1ECCE") {
		t.Fatalf("output missing the 8-character thumbprint suffix: %s", buf.String())
	}
}

func TestRenderTextHidesUnknownUnqualifiedByDefault(t *testing.T) {
	var buf bytes.Buffer
	RenderText(&buf, sampleReport(), i18n.Load("en"), referenceTime, false)
	if strings.Contains(buf.String(), "{GUID}") {
		t.Fatalf("default view must hide the unknown-purpose, unqualified certificate: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "Certificates: 2") {
		t.Fatalf("expected 2 visible certificates, got: %s", buf.String())
	}
}

func TestRenderTextAllShowsHiddenCertificate(t *testing.T) {
	var buf bytes.Buffer
	RenderText(&buf, sampleReport(), i18n.Load("en"), referenceTime, true)
	if !strings.Contains(buf.String(), "{GUID}") {
		t.Fatalf("--all must show the hidden certificate: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "Certificates: 3") {
		t.Fatalf("expected 3 certificates with --all, got: %s", buf.String())
	}
}

func TestRenderTextNoReaderIsCalmNotAnError(t *testing.T) {
	report := sampleReport()
	report.Readers = nil
	var buf bytes.Buffer
	RenderText(&buf, report, i18n.Load("en"), referenceTime, false)
	out := buf.String()
	if !strings.Contains(out, "Readers: none attached") {
		t.Fatalf("expected the calm no-reader message, got: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "error") {
		t.Fatalf("no-reader output must not read as an error: %s", out)
	}
}

// TestRenderTextStaleWarningPastThirtyDays and its sibling below are
// Task 9: the warning measures how long the agent has gone without a
// *successful fetch*, via Provenance.FetchedAt — never the published
// list's own IssuedAt, which sampleReport() deliberately leaves fixed
// and old (2026-05-20) in every one of these cases to prove IssuedAt
// alone drives nothing.
func TestRenderTextStaleWarningPastThirtyDays(t *testing.T) {
	report := sampleReport()
	report.TSL.FetchedAt = referenceTime.AddDate(0, 0, -31)
	var buf bytes.Buffer
	RenderText(&buf, report, i18n.Load("en"), referenceTime, false)
	if !strings.Contains(buf.String(), "warning") {
		t.Fatalf("expected the staleness warning when the last successful fetch is more than 30 days old, got: %s", buf.String())
	}
}

func TestRenderTextNoStaleWarningWithinThirtyDays(t *testing.T) {
	report := sampleReport()
	report.TSL.FetchedAt = referenceTime.AddDate(0, 0, -5)
	var buf bytes.Buffer
	RenderText(&buf, report, i18n.Load("en"), referenceTime, false)
	if strings.Contains(buf.String(), "warning") {
		t.Fatalf("must not warn when the last successful fetch was recent, got: %s", buf.String())
	}
}

// TestRenderTextNoStaleWarningWhenRecentlyFetchedDespiteOldIssueDate is
// the exact contradiction Task 9 reports: "issued 104 days ago, just
// refreshed" followed by a stale-list warning. The Ministry only
// republishes when something changes, so an old issue date on a list
// that was fetched moments ago is not a problem and must not warn.
func TestRenderTextNoStaleWarningWhenRecentlyFetchedDespiteOldIssueDate(t *testing.T) {
	report := sampleReport()
	report.TSL.Source = tsl.SourceNetwork
	report.TSL.IssuedAt = referenceTime.AddDate(0, 0, -104) // matches the task's own reported example
	report.TSL.FetchedAt = referenceTime
	var buf bytes.Buffer
	RenderText(&buf, report, i18n.Load("en"), referenceTime, false)
	if strings.Contains(buf.String(), "warning") {
		t.Fatalf("must not warn about an old issue date when the refresh just succeeded, got: %s", buf.String())
	}
}

// TestRenderTextStaleWarningOnEmbeddedSeedRegardlessOfIssueDate is Task
// 9's other required case: running on the embedded seed means no fetch
// has ever succeeded, which always warrants a warning — independent of
// how recent the embedded list's own issue date happens to be.
func TestRenderTextStaleWarningOnEmbeddedSeedRegardlessOfIssueDate(t *testing.T) {
	report := sampleReport()
	report.TSL.Source = tsl.SourceEmbedded
	report.TSL.FetchedAt = time.Time{}
	var buf bytes.Buffer
	RenderText(&buf, report, i18n.Load("en"), referenceTime, false)
	if !strings.Contains(buf.String(), "warning") {
		t.Fatalf("expected a warning when running on the embedded seed with no successful fetch, got: %s", buf.String())
	}
}

func TestRenderTextSubjectDisplayedInOwnScript(t *testing.T) {
	// SPEC §9.3: certificate subject data is data, not UI text — a
	// Cyrillic name must render in Cyrillic even when the interface
	// locale is English.
	var buf bytes.Buffer
	RenderText(&buf, sampleReport(), i18n.Load("en"), referenceTime, false)
	if !strings.Contains(buf.String(), "ВЕЉКО СТАНОЈЕВИЋ") {
		t.Fatalf("Cyrillic subject name did not survive into English-locale output: %s", buf.String())
	}
}

func TestRenderTextAllThreeLocalesProduceOutput(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		var buf bytes.Buffer
		RenderText(&buf, sampleReport(), i18n.Load(locale), referenceTime, false)
		if buf.Len() == 0 {
			t.Errorf("locale %s produced no output", locale)
		}
	}
}

func TestRenderJSONStructure(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderJSON(&buf, sampleReport(), referenceTime); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}

	var decoded struct {
		Readers []struct {
			Name        string `json:"name"`
			CardPresent bool   `json:"cardPresent"`
		} `json:"readers"`
		Certificates []struct {
			Qualification string `json:"qualification"`
			Purpose       string `json:"purpose"`
			Hidden        bool   `json:"hidden"`
			Thumbprint    string `json:"thumbprint"`
		} `json:"certificates"`
		TrustedList struct {
			Sequence int  `json:"sequence"`
			Stale    bool `json:"stale"`
		} `json:"trustedList"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}

	if len(decoded.Certificates) != 3 {
		t.Fatalf("--json must always report every certificate regardless of the default hiding rule: got %d", len(decoded.Certificates))
	}
	if !decoded.Certificates[2].Hidden {
		t.Fatal("the unknown-purpose, unqualified certificate must have hidden=true")
	}
	if decoded.Certificates[0].Qualification != "qualified" {
		t.Fatalf("Qualification = %q, want %q", decoded.Certificates[0].Qualification, "qualified")
	}
	if decoded.TrustedList.Sequence != 36 {
		t.Fatalf("TrustedList.Sequence = %d, want 36", decoded.TrustedList.Sequence)
	}
}
