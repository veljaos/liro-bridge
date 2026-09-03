package main

// Builds the JSON payloads pushed to the consent window's page via
// Window.PostJSON. Every display string is resolved through the
// locale catalogue here, in Go — the page only assembles DOM from
// already-localised strings plus untrusted data inserted via
// textContent (bridge.js's liroSetText), never the reverse (F5 §10:
// "the consent screen's data is computed in Go").
import (
	"fmt"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
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
	Files             []string       `json:"files"`
	FilesOverflowText string         `json:"filesOverflowText"`
	Certificates      []jsCertOption `json:"certificates"`
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
			"consent.cancel":                    c.T("consent.cancel"),
			"consent.approve":                   c.T("consent.approve"),
			"consent.application_label":         c.T("consent.application_label"),
			"consent.details_toggle":            c.T("consent.details_toggle"),
			"consent.fingerprint_label":         c.T("consent.fingerprint_label"),
			"consent.files_label":               c.T("consent.files_label"),
			"consent.select_certificate_prompt": c.T("consent.select_certificate_prompt"),
			"consent.no_usable_certificate":     c.T("consent.no_usable_certificate"),
			"consent.state_preparing_card":      c.T("consent.state_preparing_card"),
			"consent.state_done":                c.T("consent.state_done"),
			"consent.state_failed":              c.T("consent.state_failed"),
			"consent.per_signature_pin_warning": c.T("consent.per_signature_pin_warning"),
			"consent.close":                     c.T("consent.close"),
			"consent.copy_technical_details":    c.T("consent.copy_technical_details"),
		},
		"model": jsConsentModel{
			DocumentCount:     vm.DocumentCount,
			DocumentCountText: fmt.Sprintf(c.T("consent.document_count"), vm.DocumentCount),
			ApplicationName:   applicationDisplayName(c, vm.ApplicationName),
			Fingerprint:       vm.Fingerprint,
			Files:             vm.Files,
			FilesOverflowText: filesOverflow,
			Certificates:      certs,
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
	DoneOutputText    string `json:"doneOutputText"`
	FailedMessageText string `json:"failedMessageText"`
	FailedDetails     string `json:"failedDetails"`
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
	jp := jsProgress{
		State:           string(consent.StateDone),
		DoneSummaryText: fmt.Sprintf(c.T("consent.done_summary"), p.Succeeded, p.Succeeded+p.Failed),
		DoneOutputText:  c.T("consent.done_output_label") + ": " + p.OutputPath,
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
