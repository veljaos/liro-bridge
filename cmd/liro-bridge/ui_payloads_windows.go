// Windows-only, like every caller of it (see ui_assets_windows.go's own
// note on why the filename matters here).

package main

// Builds the JSON payloads pushed to the signing window's pages via
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
			"consent.copy_fingerprint":          c.T("consent.copy_fingerprint"),
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
		},
	}
}

// jsAsk is one of the three questions asked between the approval and
// the first signature: the timestamp, an output file that already
// exists, and a failure that stopped the batch before it started.
type jsAsk struct {
	State string `json:"state"`

	TSAReasonText string `json:"tsaReasonText"`

	// Task 4: the existing file's path, and the label of the button
	// that saves under a different name — which names the name it would
	// use, so the choice is made with the answer visible rather than
	// after it.
	OutputExistsPath string `json:"outputExistsPath"`
	OutputRenameText string `json:"outputRenameText"`
	OutputRenamePath string `json:"outputRenamePath"`

	// J-3: how many of the batch's documents already carry the output
	// suffix, said in words, and the two proceeding actions' own labels
	// — which are singular or plural depending on that count, because a
	// button that says "Skip them" for one document reads as though the
	// program has miscounted.
	AlreadySignedText string `json:"alreadySignedText"`
	AlreadySkipText   string `json:"alreadySkipText"`
	AlreadySignText   string `json:"alreadySignText"`

	FailedMessageText string `json:"failedMessageText"`
	FailedDetails     string `json:"failedDetails"`
}

// achievedLevelIntent is the intent family that colours the level a
// batch actually reached (Task 1, F5 second-real-run review). B-B — a
// signature with no timestamp, and so no proof of when it was made — is
// the warning family; B-T and B-LT are positive.
//
// The level is always stated, never inferred from its absence: SPEC
// §18.11 forbids claiming a level that was not reached, and stating
// nothing at all is how a previous build managed to be silent about a
// downgrade it had already performed.
func achievedLevelIntent(level string) string {
	switch pades.Level(level) {
	case pades.LevelBB:
		return string(ui.IntentWarning)
	case pades.LevelBT, pades.LevelBLT:
		return string(ui.IntentPositive)
	default:
		return ""
	}
}

// achievedLevelNote is the sentence a B-B batch gets under its level,
// and nothing at all for the other two. "B-B" is a level to someone who
// knows the profile and a two-letter code to everyone else; what it
// means — a valid signature with no proof of when it was made — is what
// belongs on the screen.
func achievedLevelNote(c *i18n.Catalogue, level string) string {
	if pades.Level(level) != pades.LevelBB {
		return ""
	}
	return c.T("consent.level_bb")
}

// askTSAChoicePayload puts the window into SPEC §12.8's choice (Task
// 1): sign without a timestamp, configure a timestamp authority, or
// cancel. reason decides only which sentence explains why the choice is
// being offered; the three actions are the same in both cases.
//
// The three "ask" payloads all render on the page that carries the
// progress and the report, because that is where the flow is by the
// time they are asked — after the approval, before the card.
func askTSAChoicePayload(reason consent.TSAReason, c *i18n.Catalogue) map[string]any {
	key := "consent.tsa_reason_unreachable"
	if reason == consent.TSAReasonNotConfigured {
		key = "consent.tsa_reason_not_configured"
	}
	return map[string]any{"type": "ask", "ask": jsAsk{
		State:         string(consent.StateTSAChoice),
		TSAReasonText: c.T(key),
	}}
}

// askOutputExistsPayload puts the window into Task 4's choice:
// overwrite the existing file, save the signed document under
// renamePath instead, or cancel. Both paths are shown in full —
// nothing here decides for the user, and SPEC §12.11's rule that a file
// is never silently replaced is upheld by asking, not by refusing.
func askOutputExistsPayload(existingPath, renamePath string, c *i18n.Catalogue) map[string]any {
	return map[string]any{"type": "ask", "ask": jsAsk{
		State:            string(consent.StateOutputExists),
		OutputExistsPath: existingPath,
		OutputRenameText: fmt.Sprintf(c.T("consent.output_exists_rename"), filepath.Base(renamePath)),
		OutputRenamePath: renamePath,
	}}
}

// askAlreadySignedPayload puts the window into J-3's choice: skip the
// documents whose names already end in the output suffix, sign them too,
// or cancel.
//
// Skipping is the primary action because it is the one that matches what
// a second run of the same folder almost always means. It is offered,
// not applied: someone signing a "ugovor-signed.pdf" that arrived from
// elsewhere is counter-signing, which is ordinary, and this program does
// not know which of the two it is looking at. So it says how many and
// what would happen, and lets the person decide — for the whole batch,
// once, exactly as the output-file choice does.
func askAlreadySignedPayload(n int, suffix string, c *i18n.Catalogue) map[string]any {
	explain := fmt.Sprintf(c.T("consent.already_signed_explain_many"), n, suffix)
	skip := c.T("consent.already_signed_skip_many")
	sign := c.T("consent.already_signed_sign_many")
	if n == 1 {
		explain = fmt.Sprintf(c.T("consent.already_signed_explain_one"), suffix)
		skip = c.T("consent.already_signed_skip_one")
		sign = c.T("consent.already_signed_sign_one")
	}
	return map[string]any{"type": "ask", "ask": jsAsk{
		State:             string(consent.StateAlreadySigned),
		AlreadySignedText: explain,
		AlreadySkipText:   skip,
		AlreadySignText:   sign,
	}}
}

// askFailedPayload is the screen for something that stopped the batch
// before a single document could be signed.
func askFailedPayload(message string, err error, c *i18n.Catalogue) map[string]any {
	_ = c
	details := ""
	if err != nil {
		details = err.Error()
	}
	return map[string]any{"type": "ask", "ask": jsAsk{
		State:             string(consent.StateFailed),
		FailedMessageText: message,
		FailedDetails:     details,
	}}
}
