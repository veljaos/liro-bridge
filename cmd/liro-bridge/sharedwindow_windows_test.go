//go:build windows

package main

// One consent window and one settings window, shared by every test in
// this package that needs them.
//
// F5 second-real-run review: each ui.NewWindow builds its own WebView2
// environment, and each environment starts its own group of
// msedgewebview2.exe browser processes that outlive the window — a
// closed window's processes are still running seconds later (which is
// what webView2ExitGrace exists to give them time for). Opening a
// window per test worked while this package opened four of them; at
// thirteen, CreateCoreWebView2Controller stopped completing at all —
// no error, no timeout of its own, just a message pump waiting forever
// until the test binary's own deadline fired.
//
// Sharing is safe because every window in this project is driven
// entirely by what Go posts into it: an "init" payload re-renders the
// whole page from scratch (consent.js's renderWaiting, settings.js's
// __liroOnMessage), so a test that starts by posting one is looking at
// a page in a known state regardless of what the test before it did. A
// test that needs a genuinely pristine DOM — one asserting what is on
// screen *before* anything happens — opens its own window instead.
import (
	"sync"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

type sharedWindow struct {
	once     sync.Once
	win      ui.Window
	messages chan ui.Message
	err      error
}

var (
	consentShared      sharedWindow
	settingsShared     sharedWindow
	certificatesShared sharedWindow
	auditLogShared     sharedWindow
	mainShared         sharedWindow
	stampShared        sharedWindow
)

// drain empties any messages left over from an earlier test, so a
// borrowed window's channel starts empty.
func drain(ch chan ui.Message) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// sharedConsentWindow returns the package's one consent window, at the
// size runSignInteractive itself uses. Callers must not Close it —
// closeSharedWindows does that once, from TestMain.
func sharedConsentWindow(t *testing.T) (ui.Window, chan ui.Message) {
	t.Helper()
	consentShared.once.Do(func() {
		consentShared.messages = make(chan ui.Message, 16)
		consentShared.win, consentShared.err = ui.NewWindow(ui.Options{
			Title:       i18n.Load("sr-Latn").T("consent.window_title"),
			Width:       520,
			Height:      860,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/consent.html",
			OnMessage:   func(m ui.Message) { consentShared.messages <- m },
			OnClosed:    func() { consentShared.messages <- ui.Message{Type: ui.MessageTypeCancel} },
		})
	})
	if consentShared.err != nil {
		t.Fatalf("NewWindow(consent): %v", consentShared.err)
	}
	drain(consentShared.messages)
	return consentShared.win, consentShared.messages
}

// sharedSettingsWindow returns the package's one settings window, with
// cfg's init payload freshly posted for locale c.
func sharedSettingsWindow(t *testing.T, c *i18n.Catalogue, cfg config.Config) (ui.Window, chan ui.Message) {
	t.Helper()
	settingsShared.once.Do(func() {
		settingsShared.messages = make(chan ui.Message, 16)
		settingsShared.win, settingsShared.err = ui.NewWindow(ui.Options{
			Title:       i18n.Load("sr-Latn").T("settings.window_title"),
			Width:       520,
			Height:      880,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/settings.html",
			OnMessage:   func(m ui.Message) { settingsShared.messages <- m },
			OnClosed:    func() { settingsShared.messages <- ui.Message{Type: ui.MessageTypeCancel} },
		})
	})
	if settingsShared.err != nil {
		t.Fatalf("NewWindow(settings): %v", settingsShared.err)
	}
	drain(settingsShared.messages)
	if err := settingsShared.win.PostJSON(buildSettingsInit(c, cfg)); err != nil {
		t.Fatalf("PostJSON(settings init): %v", err)
	}
	return settingsShared.win, settingsShared.messages
}

// sharedCertificatesWindow returns the package's one certificates
// window, at the size runCertificatesWindow itself uses.
func sharedCertificatesWindow(t *testing.T) ui.Window {
	t.Helper()
	certificatesShared.once.Do(func() {
		certificatesShared.messages = make(chan ui.Message, 16)
		certificatesShared.win, certificatesShared.err = ui.NewWindow(ui.Options{
			Title:       i18n.Load("sr-Latn").T("certswindow.title"),
			Width:       460,
			Height:      640,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/certificates.html",
			OnMessage:   func(m ui.Message) { certificatesShared.messages <- m },
			OnClosed:    func() { certificatesShared.messages <- ui.Message{Type: ui.MessageTypeCancel} },
		})
	})
	if certificatesShared.err != nil {
		t.Fatalf("NewWindow(certificates): %v", certificatesShared.err)
	}
	drain(certificatesShared.messages)
	return certificatesShared.win
}

// sharedAuditLogWindow returns the package's one audit log window, at
// the size runAuditLogWindow itself uses.
func sharedAuditLogWindow(t *testing.T) ui.Window {
	t.Helper()
	auditLogShared.once.Do(func() {
		auditLogShared.messages = make(chan ui.Message, 16)
		auditLogShared.win, auditLogShared.err = ui.NewWindow(ui.Options{
			Title:       i18n.Load("sr-Latn").T("auditwindow.title"),
			Width:       460,
			Height:      520,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/auditlog.html",
			OnMessage:   func(m ui.Message) { auditLogShared.messages <- m },
			OnClosed:    func() { auditLogShared.messages <- ui.Message{Type: ui.MessageTypeCancel} },
		})
	})
	if auditLogShared.err != nil {
		t.Fatalf("NewWindow(auditlog): %v", auditLogShared.err)
	}
	drain(auditLogShared.messages)
	return auditLogShared.win
}

// sharedMainWindow returns the package's one main window, at the size
// runMainWindow itself uses (F6 §1).
func sharedMainWindow(t *testing.T) (ui.Window, chan ui.Message) {
	t.Helper()
	mainShared.once.Do(func() {
		mainShared.messages = make(chan ui.Message, 16)
		mainShared.win, mainShared.err = ui.NewWindow(ui.Options{
			Title:       i18n.Load("sr-Latn").T("main.title"),
			Width:       mainWindowWidth,
			Height:      mainWindowHeight,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/main.html",
			OnMessage:   func(m ui.Message) { mainShared.messages <- m },
			OnClosed:    func() { mainShared.messages <- ui.Message{Type: ui.MessageTypeCancel} },
			// Deliberately no OnFilesDropped: a window that registers as
			// a drop target also turns WebView2's own drop handling off,
			// and a test has nothing to drop on it. What that option
			// does is covered by internal/ui's own tests and by the
			// running binary.
		})
	})
	if mainShared.err != nil {
		t.Fatalf("NewWindow(main): %v", mainShared.err)
	}
	drain(mainShared.messages)
	return mainShared.win, mainShared.messages
}

// sharedStampWindow returns the package's one stamp window (F6 §6),
// with cfg's init payload freshly posted.
func sharedStampWindow(t *testing.T, c *i18n.Catalogue, cfg config.Config) (ui.Window, chan ui.Message) {
	t.Helper()
	stampShared.once.Do(func() {
		stampShared.messages = make(chan ui.Message, 16)
		stampShared.win, stampShared.err = ui.NewWindow(ui.Options{
			Title:       i18n.Load("sr-Latn").T("stampwindow.title"),
			Width:       stampWindowWidth,
			Height:      stampWindowHeight,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/stamp.html",
			OnMessage:   func(m ui.Message) { stampShared.messages <- m },
			OnClosed:    func() { stampShared.messages <- ui.Message{Type: ui.MessageTypeCancel} },
		})
	})
	if stampShared.err != nil {
		t.Fatalf("NewWindow(stamp): %v", stampShared.err)
	}
	drain(stampShared.messages)
	if err := stampShared.win.PostJSON(buildStampInit(c, cfg)); err != nil {
		t.Fatalf("PostJSON(stamp init): %v", err)
	}
	return stampShared.win, stampShared.messages
}

// closeSharedWindows is called once, from TestMain, after every test
// has finished with them.
func closeSharedWindows() {
	for _, w := range []*sharedWindow{&consentShared, &settingsShared, &certificatesShared, &auditLogShared, &mainShared, &stampShared} {
		if w.win != nil {
			_ = w.win.Close()
		}
	}
}
