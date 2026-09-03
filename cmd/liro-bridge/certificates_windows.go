//go:build windows

package main

// runCertificatesWindow implements the tray's "Certificates" item
// (Task 4, F5 review): previously this logged a placeholder line and
// did nothing visible at all — a menu item that looks enabled and does
// nothing on click reads as a broken program. It reuses the same
// certificate-gathering wiring and CertificateOption view model the
// consent window (interactive_windows.go) and `liro-bridge certs`
// (main.go's runCerts) already use, so the list shown here always
// matches what signing would offer.
import (
	"context"
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

func runCertificatesWindow(locale string) error {
	c := i18n.Load(locale)

	report, err := gatherInteractiveCertificates(context.Background())
	if err != nil {
		return err
	}
	certInfos := make([]classify.Info, 0, len(report.Certificates))
	for _, row := range report.Certificates {
		certInfos = append(certInfos, row.Info)
	}

	messages := make(chan ui.Message, 8)
	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("certswindow.title"),
		Width:       460,
		Height:      440,
		AlwaysOnTop: true,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/certificates.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		return err
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(buildCertificatesInit(c, certInfos)); err != nil {
		return err
	}

	for {
		msg := <-messages
		if msg.Type == ui.MessageTypeCancel {
			return nil
		}
		slog.Warn("certificates window: unexpected message", "type", msg.Type)
	}
}

func buildCertificatesInit(c *i18n.Catalogue, certs []classify.Info) map[string]any {
	options := consent.BuildCertificateOptions(certs)
	return map[string]any{
		"type": "init",
		"strings": map[string]string{
			"certswindow.title":           c.T("certswindow.title"),
			"certswindow.empty":           c.T("certswindow.empty"),
			"certswindow.qualified_badge": c.T("certswindow.qualified_badge"),
			"settings.close":              c.T("settings.close"),
		},
		"model": map[string]any{
			"certificates": buildCertOptions(c, options),
		},
	}
}
