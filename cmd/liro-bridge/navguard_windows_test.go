//go:build windows

package main

// Every window this program opens, at its own size, on its own real
// page: what can make it become a different document.
//
// internal/ui holds the rule and the guards; this is the check that the
// windows a person actually meets are the ones the rule covers. It is
// here rather than there because only this package knows what the eight
// windows are, and because a rule that covered seven of them would look
// exactly like a rule that covered all eight from inside internal/ui.

import (
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

var (
	procEnumChildWindowsT = user32Test.NewProc("EnumChildWindows")
	procGetPropWT         = user32Test.NewProc("GetPropW")
	procGetClassNameWT    = user32Test.NewProc("GetClassNameW")
)

// acceptsDrop asks the window itself, through the property OLE sets on
// a window with a registered IDropTarget — the same thing this
// project's own out-of-process probe reads, and the same thing
// internal/ui's drop-watch tests read. The pointer belongs to whichever
// process registered it and is never dereferenced.
func acceptsDrop(hwnd uintptr) bool {
	name, err := windows.UTF16PtrFromString("OleDropTargetInterface")
	if err != nil {
		return false
	}
	p, _, _ := procGetPropWT.Call(hwnd, uintptr(unsafe.Pointer(name)))
	return p != 0
}

func classOfWindow(hwnd uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetClassNameWT.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// dropAcceptorsUnder lists every window in the hosted tree that accepts
// a drop.
//
// It deliberately does not say whose target each one is, because from
// outside the process that cannot be told: D-123 registers this
// program's own IDropTarget on windows the msedgewebview2 process owns
// — Chrome_RenderWidgetHostHWND is where a real drop actually lands —
// so the owning process identifies the window and says nothing about
// the target on it. What is observable is whether anything accepts a
// drop at all, and that is the whole question for a window that takes
// no documents.
func dropAcceptorsUnder(top uintptr) []string {
	all := []uintptr{top}
	cb := windows.NewCallback(func(child uintptr, _ uintptr) uintptr {
		all = append(all, child)
		return 1
	})
	_, _, _ = procEnumChildWindowsT.Call(top, cb, 0)

	var accepting []string
	for _, h := range all {
		if acceptsDrop(h) {
			accepting = append(accepting, classOfWindow(h))
		}
	}
	return accepting
}

// The defect, as a property of every window this program has: a file
// dropped on one of them must not be able to make it display that file.
//
// Chromium registers a drop target of its own on every WebView2 that
// does not switch external drops off, and a document dropped there
// navigates the window to it — which is what a browser does with a
// dropped file. Measured on the built binary before this was fixed:
// Settings, Certificates and the audit log each had exactly one such
// target, Chrome_WidgetWin_1, owned by the msedgewebview2 process.
//
// The distinguishing assertion is not "no drop targets". The signing
// window genuinely takes dropped documents (F6 §1) and must keep its
// own. What must never be there is a target nothing here registered.
//
// Each window is opened and closed by this test rather than borrowed
// from the package's shared ones. Borrowing them was tried and is
// wrong in a way worth recording: it leaves seven windows on screen
// earlier in the run than they would otherwise be, and
// TestAWindowOpenedFromSettingsIsReachable then finds one of them over
// the window it is asking about. That test is right and this one was
// the newcomer; a test that changes what the tests after it see is
// measuring the suite rather than the product.
func TestNoWindowLetsTheBrowserHandleADroppedFile(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()

	cases := []struct {
		name      string
		opts      ui.Options
		init      func() any
		takesDrop bool
	}{
		{name: "consent (the certificate step, SPEC §6.5's gate)",
			opts: ui.Options{
				Title: c.T("consent.window_title"), Width: stepCertificateWidth, Height: stepCertificateHeight,
				StartPage: "/pages/consent.html",
			}},
		{name: "settings",
			opts: ui.Options{
				Title: c.T("settings.window_title"), Width: 520, Height: 880,
				StartPage: "/pages/settings.html",
			},
			init: func() any {
				return buildSettingsInit(c, cfg, nil, platform.SecretStoreDescription{Mechanism: platform.MechanismDPAPI})
			}},
		{name: "certificates",
			opts: ui.Options{
				Title: c.T("certswindow.title"), Width: 460, Height: 640,
				StartPage: "/pages/certificates.html",
			}},
		{name: "audit log",
			opts: ui.Options{
				Title: c.T("auditwindow.title"), Width: 460, Height: 520,
				StartPage: "/pages/auditlog.html",
			}},
		{name: "pairing",
			opts: ui.Options{
				Title: c.T("pairing.window_title"), Width: pairingWindowWidth, Height: pairingWindowHeight,
				StartPage: "/pages/pairing.html",
			}},
		{name: "the signing method step",
			opts: ui.Options{
				Title: c.T("stampwindow.title"), Width: stampWindowWidth, Height: stampSettingsHeight,
				StartPage: "/pages/stamp.html",
			}},
		{name: "the placement picker",
			opts: ui.Options{
				Title: c.T("place.title"), Width: 900, Height: 700,
				StartPage: "/pages/place.html",
			}},
		{name: "the update prompt",
			opts: ui.Options{
				Title: c.T("update.window_title"), Width: 420, Height: 300,
				StartPage: "/pages/update.html",
			}},
		{name: "the signing window, which does take documents",
			opts: ui.Options{
				Title: c.T("main.title"), Width: stepDocumentsWidth, Height: stepDocumentsHeight,
				StartPage:      "/pages/main.html",
				OnFilesDropped: func([]string) {},
			},
			takesDrop: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			opts.Assets = assetsFS
			opts.VirtualHost = liroVirtualHost
			opts.OnMessage = func(ui.Message) {}
			opts.OnClosed = func() {}

			win, err := ui.NewWindow(opts)
			if err != nil {
				t.Fatalf("NewWindow: %v", err)
			}
			defer func() { _ = win.Close() }()
			if tc.init != nil {
				if err := win.PostJSON(tc.init()); err != nil {
					t.Fatalf("PostJSON(init): %v", err)
				}
			}

			hwnd := waitForWindowTitled(t, opts.Title, 60*time.Second)
			if hwnd == 0 {
				t.Fatalf("no window titled %q appeared", opts.Title)
			}
			accepting := dropAcceptorsUnder(hwnd)

			switch {
			case tc.takesDrop && len(accepting) == 0:
				t.Errorf("this window takes dropped documents and nothing under it accepts a drop")
			case !tc.takesDrop && len(accepting) != 0:
				t.Errorf("this window takes no dropped documents, yet %v accept a drop —\n"+
					"nothing here registered one, so it is the browser's, and a document\n"+
					"dropped on it makes the window display that document", accepting)
			}
		})
	}
}
