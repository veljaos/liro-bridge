// Windows-only, like every caller of it (see ui_assets_windows.go's own
// note on why the filename matters here).

package main

// Builds the JSON payloads pushed to the consent window's page via
// Window.PostJSON. Every display string is resolved through the
// locale catalogue here, in Go — the page only assembles DOM from
// already-localised strings plus untrusted data inserted via
// textContent (bridge.js's liroSetText), never the reverse (F5 §10:
// "the consent screen's data is computed in Go").
import (
	"fmt"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/ui"
)

type jsCertOption struct {
	Thumbprint         string `json:"thumbprint"`
	DisplayName        string `json:"displayName"`
	RoleText           string `json:"roleText"`
	IssuerText         string `json:"issuerText"`
	ThumbprintTail     string `json:"thumbprintTail"`
	Qualified          bool   `json:"qualified"`
	Usable             bool   `json:"usable"`
	DisabledReasonText string `json:"disabledReasonText"`
	IsTestKey          bool   `json:"isTestKey"`
	TestKeyLabel       string `json:"testKeyLabel"`
}

type jsConsentModel struct {
	DocumentCount     int            `json:"documentCount"`
	DocumentCountText string         `json:"documentCountText"`
	ApplicationName   string         `json:"applicationName"`
	Fingerprint       string         `json:"fingerprint"`
	FingerprintShort  string         `json:"fingerprintShort"`
	Files             []string       `json:"files"`
	FilesOverflowText string         `json:"filesOverflowText"`
	Certificates      []jsCertOption `json:"certificates"`

	// Task 1 (F5 fourth-real-run review): the visible-stamp choice, as
	// the configuration currently holds it. StampPositions carries the
	// four corners and their localised labels, so the page never holds
	// a list of positions of its own that could drift from Go's.
	StampVisible   bool              `json:"stampVisible"`
	StampPosition  string            `json:"stampPosition"`
	StampPositions []jsStampPosition `json:"stampPositions"`
}

// jsStampPosition is one corner of the stamp position selector: the
// value stored in configuration and passed to appearance.Corner, plus
// the text the user reads.
type jsStampPosition struct {
	Value string `json:"value"`
	Text  string `json:"text"`
}

// stampPositionText is the localised name of one corner. The four keys
// mirror consent.StampPositions' four values exactly; an unrecognised
// value cannot reach here, because consent.StampChoice.Normalised has
// already replaced it with the default.
func stampPositionText(c *i18n.Catalogue, position string) string {
	switch position {
	case consent.StampPositionBottomRight:
		return c.T("consent.stamp_position_bottom_right")
	case consent.StampPositionBottomLeft:
		return c.T("consent.stamp_position_bottom_left")
	case consent.StampPositionTopRight:
		return c.T("consent.stamp_position_top_right")
	case consent.StampPositionTopLeft:
		return c.T("consent.stamp_position_top_left")
	default:
		return position
	}
}

func stampPositionOptions(c *i18n.Catalogue) []jsStampPosition {
	positions := consent.StampPositions()
	out := make([]jsStampPosition, 0, len(positions))
	for _, p := range positions {
		out = append(out, jsStampPosition{Value: p, Text: stampPositionText(c, p)})
	}
	return out
}

func roleText(c *i18n.Catalogue, r consent.Role) string {
	switch r {
	case consent.RoleSigning:
		return c.T("consent.role_signing")
	case consent.RoleAuthentication:
		return c.T("consent.role_authentication")
	default:
		return c.T("consent.role_unknown")
	}
}

func applicationDisplayName(c *i18n.Catalogue, name string) string {
	if name == consent.ApplicationLocal {
		return c.T("consent.local_application")
	}
	return name
}

// buildCertOptions converts every consent.CertificateOption in certs
// into its page-ready jsCertOption shape — shared by the consent
// window (buildConsentInit) and the Certificates tray window
// (buildCertificatesInit, certificates_windows.go), so a certificate
// row looks and localises identically in both places.
func buildCertOptions(c *i18n.Catalogue, certs []consent.CertificateOption) []jsCertOption {
	out := make([]jsCertOption, 0, len(certs))
	for _, cert := range certs {
		reasonText := ""
		if !cert.Usable {
			reasonText = c.T(i18n.CodeKey(cert.DisabledReason))
		}
		out = append(out, jsCertOption{
			Thumbprint:         cert.Thumbprint,
			DisplayName:        cert.DisplayName,
			RoleText:           roleText(c, cert.Role),
			IssuerText:         cert.Issuer,
			ThumbprintTail:     cert.ThumbprintTail,
			Qualified:          cert.Qualified,
			Usable:             cert.Usable,
			DisabledReasonText: reasonText,
			IsTestKey:          cert.IsTestKey,
			TestKeyLabel:       c.T("certs.test_key_marker"),
		})
	}
	return out
}

func buildConsentInit(c *i18n.Catalogue, vm consent.ViewModel) map[string]any {
	certs := buildCertOptions(c, vm.Certificates)

	filesOverflow := ""
	if vm.FilesOverflow > 0 {
		filesOverflow = fmt.Sprintf(c.T("consent.files_overflow"), vm.FilesOverflow)
	}

	return map[string]any{
		"type": "init",
		"strings": map[string]string{
			// Static labels the page resolves itself via data-i18n.
			"consent.cancel":                     c.T("consent.cancel"),
			"consent.approve":                    c.T("consent.approve"),
			"consent.application_label":          c.T("consent.application_label"),
			"consent.details_toggle":             c.T("consent.details_toggle"),
			"consent.fingerprint_label":          c.T("consent.fingerprint_label"),
			"consent.files_label":                c.T("consent.files_label"),
			"consent.select_certificate_prompt":  c.T("consent.select_certificate_prompt"),
			"consent.no_usable_certificate":      c.T("consent.no_usable_certificate"),
			"consent.state_preparing_card":       c.T("consent.state_preparing_card"),
			"consent.state_done":                 c.T("consent.state_done"),
			"consent.state_failed":               c.T("consent.state_failed"),
			"consent.per_signature_pin_warning":  c.T("consent.per_signature_pin_warning"),
			"consent.close":                      c.T("consent.close"),
			"consent.tsa_choice_title":           c.T("consent.tsa_choice_title"),
			"consent.tsa_choice_explain":         c.T("consent.tsa_choice_explain"),
			"consent.tsa_save_without_timestamp": c.T("consent.tsa_save_without_timestamp"),
			"consent.tsa_configure":              c.T("consent.tsa_configure"),
			"consent.copy_technical_details":     c.T("consent.copy_technical_details"),
			"consent.copy_fingerprint":           c.T("consent.copy_fingerprint"),
			"consent.stamp_visible":              c.T("consent.stamp_visible"),
			"consent.stamp_position_label":       c.T("consent.stamp_position_label"),
			"consent.output_exists_title":        c.T("consent.output_exists_title"),
			"consent.output_exists_explain":      c.T("consent.output_exists_explain"),
			"consent.output_exists_path_label":   c.T("consent.output_exists_path_label"),
			"consent.output_exists_overwrite":    c.T("consent.output_exists_overwrite"),
		},
		"model": jsConsentModel{
			DocumentCount:     vm.DocumentCount,
			DocumentCountText: fmt.Sprintf(c.T("consent.document_count"), vm.DocumentCount),
			ApplicationName:   applicationDisplayName(c, vm.ApplicationName),
			Fingerprint:       vm.Fingerprint,
			FingerprintShort:  vm.FingerprintShort,
			Files:             vm.Files,
			FilesOverflowText: filesOverflow,
			Certificates:      certs,
			StampVisible:      vm.Stamp.Visible,
			StampPosition:     vm.Stamp.Normalised().Position,
			StampPositions:    stampPositionOptions(c),
		},
	}
}

type jsProgress struct {
	State             string `json:"state"`
	SigningLabelText  string `json:"signingLabelText"`
	Percent           int    `json:"percent"`
	ETAText           string `json:"etaText"`
	PerSignaturePIN   bool   `json:"perSignaturePIN"`
	DoneSummaryText   string `json:"doneSummaryText"`
	DoneLevelText     string `json:"doneLevelText"`
	DoneLevelIntent   string `json:"doneLevelIntent"`
	DoneOutputText    string `json:"doneOutputText"`
	FailedMessageText string `json:"failedMessageText"`
	FailedDetails     string `json:"failedDetails"`
	TSAReasonText     string `json:"tsaReasonText"`

	// Task 4: the existing file's path, and the label of the button
	// that saves under a different name — which names the name it would
	// use, so the choice is made with the answer visible rather than
	// after it.
	OutputExistsPath string `json:"outputExistsPath"`
	OutputRenameText string `json:"outputRenameText"`
	OutputRenamePath string `json:"outputRenamePath"`
}

func consentProgressPayload(p consent.Progress, c *i18n.Catalogue) map[string]any {
	jp := jsProgress{State: string(p.State)}
	switch p.State {
	case consent.StateSigning:
		jp.SigningLabelText = fmt.Sprintf(c.T("consent.state_signing"), p.Current, p.Total)
		if p.Total > 0 {
			jp.Percent = p.Current * 100 / p.Total
		}
		if p.ETAKnown {
			jp.ETAText = fmt.Sprintf(c.T("consent.eta_label"), p.ETA.Round(1e9))
		}
		jp.PerSignaturePIN = p.PerSignaturePIN
	}
	return map[string]any{"type": "progress", "progress": jp}
}

func consentDonePayload(p consent.Progress, c *i18n.Catalogue) map[string]any {
	levelText, levelIntent := achievedLevelDisplay(c, p.AchievedLevel)
	jp := jsProgress{
		State:           string(consent.StateDone),
		DoneSummaryText: fmt.Sprintf(c.T("consent.done_summary"), p.Succeeded, p.Succeeded+p.Failed),
		DoneLevelText:   levelText,
		DoneLevelIntent: levelIntent,
		DoneOutputText:  c.T("consent.done_output_label") + ": " + p.OutputPath,
	}
	return map[string]any{"type": "progress", "progress": jp}
}

// achievedLevelDisplay renders the level a batch actually reached, and
// the intent family that colours it (Task 1, F5 second-real-run
// review). B-B — a signature with no timestamp, and so no proof of
// when it was made — is the warning family; B-T and B-LT are positive.
// The level is always stated, never inferred from its absence: SPEC
// §18.11 forbids claiming a level that was not reached, and stating
// nothing at all is how the previous build managed to be silent about a
// downgrade it had already performed.
func achievedLevelDisplay(c *i18n.Catalogue, level string) (text string, intent string) {
	if level == "" {
		return "", ""
	}
	switch pades.Level(level) {
	case pades.LevelBB:
		return c.T("consent.level_bb"), string(ui.IntentWarning)
	case pades.LevelBT:
		return fmt.Sprintf(c.T("consent.level_label"), string(pades.LevelBT)), string(ui.IntentPositive)
	case pades.LevelBLT:
		return fmt.Sprintf(c.T("consent.level_label"), string(pades.LevelBLT)), string(ui.IntentPositive)
	default:
		return fmt.Sprintf(c.T("consent.level_label"), level), ""
	}
}

// consentTSAChoicePayload puts the consent window into SPEC §12.8's
// choice (Task 1): sign without a timestamp, configure a timestamp
// authority, or cancel. reason decides only which sentence explains
// why the choice is being offered; the three actions are the same in
// both cases.
func consentTSAChoicePayload(reason consent.TSAReason, c *i18n.Catalogue) map[string]any {
	key := "consent.tsa_reason_unreachable"
	if reason == consent.TSAReasonNotConfigured {
		key = "consent.tsa_reason_not_configured"
	}
	jp := jsProgress{
		State:         string(consent.StateTSAChoice),
		TSAReasonText: c.T(key),
	}
	return map[string]any{"type": "progress", "progress": jp}
}

// consentOutputExistsPayload puts the consent window into Task 4's
// choice: overwrite the existing file, save the signed document under
// renamePath instead, or cancel. Both paths are shown in full —
// nothing here decides for the user, and SPEC §12.11's rule that a file
// is never silently replaced is upheld by asking, not by refusing.
func consentOutputExistsPayload(existingPath, renamePath string, c *i18n.Catalogue) map[string]any {
	jp := jsProgress{
		State:            string(consent.StateOutputExists),
		OutputExistsPath: existingPath,
		OutputRenameText: fmt.Sprintf(c.T("consent.output_exists_rename"), filepath.Base(renamePath)),
		OutputRenamePath: renamePath,
	}
	return map[string]any{"type": "progress", "progress": jp}
}

func consentFailedPayload(message string, err error, c *i18n.Catalogue) map[string]any {
	details := ""
	if err != nil {
		details = err.Error()
	}
	jp := jsProgress{
		State:             string(consent.StateFailed),
		FailedMessageText: message,
		FailedDetails:     details,
	}
	return map[string]any{"type": "progress", "progress": jp}
}
