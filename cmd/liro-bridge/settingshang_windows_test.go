//go:build windows

package main

// The defect the owner reported as "after changing something in
// Settings, the program goes dead", and the two properties that make it
// impossible rather than unlikely.
//
// What it was, measured before anything was changed: pressing the stamp
// button in Settings opened a second window that was not owned by
// Settings and was not always-on-top, while Settings was — so Windows
// drew the new window *underneath* it, exactly covered, with Go blocked
// waiting for a click on something the person could not see. Clicking
// Settings then did nothing, because the goroutine that reads its
// messages was in that wait; and on the ninth click the eight-deep
// message channel filled, the callback blocked inside the window's own
// message loop, and Settings itself stopped repainting and could not be
// closed. Nothing crashed and nothing deadlocked in COM: the process
// was alive and both windows' threads were healthy, one of them parked
// on a channel send.
//
// Two tests, because it took two things to happen. The first is here;
// the second — that a blocked callback can no longer freeze a window at
// all — is internal/ui's TestBlockingCallbackDoesNotFreezeTheWindow,
// where the guarantee belongs.

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

var (
	user32Test           = windows.NewLazySystemDLL("user32.dll")
	procGetWindowRectT   = user32Test.NewProc("GetWindowRect")
	procWindowFromPointT = user32Test.NewProc("WindowFromPoint")
	procGetAncestorT     = user32Test.NewProc("GetAncestor")
	procIsWindowEnabledT = user32Test.NewProc("IsWindowEnabled")
	procFindWindowT      = user32Test.NewProc("FindWindowW")
	procIsWindowVisibleT = user32Test.NewProc("IsWindowVisible")
	procPostMessageT     = user32Test.NewProc("PostMessageW")
)

type screenRect struct{ Left, Top, Right, Bottom int32 }

func windowRectOf(hwnd uintptr) screenRect {
	var r screenRect
	_, _, _ = procGetWindowRectT.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

// topWindowAt is the top-level window a click at a point on screen
// would reach — which is what "covered" and "visible" actually mean, as
// opposed to what a z-order flag says.
func topWindowAt(x, y int32) uintptr {
	packed := uintptr(uint32(x)) | uintptr(uint32(y))<<32
	h, _, _ := procWindowFromPointT.Call(packed)
	root, _, _ := procGetAncestorT.Call(h, 2 /* GA_ROOT */)
	return root
}

// newProductionSettingsWindow builds the settings window exactly as
// runSettingsWindow does — always on top, at its own size — rather than
// borrowing the shared one, because being always-on-top is half of what
// this file is about.
func newProductionSettingsWindow(t *testing.T, c *i18n.Catalogue, cfg config.Config) (ui.Window, chan ui.Message) {
	t.Helper()
	messages := make(chan ui.Message, 8)
	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("settings.window_title"),
		Width:       520,
		Height:      880,
		AlwaysOnTop: true,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/settings.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		t.Fatalf("NewWindow(settings): %v", err)
	}
	if err := win.PostJSON(buildSettingsInit(c, cfg)); err != nil {
		t.Fatalf("PostJSON(settings init): %v", err)
	}
	return win, messages
}

// readSettingsForm is what runSettingsWindow's own loop does with an
// approve: read the whole form back through Eval's return value rather
// than widening the page->Go message surface (D-083).
func readSettingsForm(t *testing.T, win ui.Window) settingsFormState {
	t.Helper()
	raw, err := win.Eval("window.__liroCollectState()")
	if err != nil {
		t.Fatalf("Eval(__liroCollectState): %v", err)
	}
	var envelope string
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("decoding the form envelope %q: %v", raw, err)
	}
	var state settingsFormState
	if err := json.Unmarshal([]byte(envelope), &state); err != nil {
		t.Fatalf("decoding the form state: %v", err)
	}
	return state
}

// TestAWindowOpenedFromSettingsIsReachable is the direct regression
// test. It presses the stamp button through the page's own DOM
// (D-094's carve-out — nothing here touches the real cursor), lets
// handleSettingsAction open the window it opens, and then asks the two
// questions a person asks: can I see it, and does the window behind it
// still behave.
//
// Run against the build that was reported, the first assertion fails:
// the window at the centre of the new window is the settings window.
func TestAWindowOpenedFromSettingsIsReachable(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	win, messages := newProductionSettingsWindow(t, c, cfg)
	defer func() { _ = win.Close() }()
	settings := win.Handle()

	if _, err := win.Eval("document.getElementById('stamp-settings-btn').click()"); err != nil {
		t.Fatalf("clicking the stamp button: %v", err)
	}
	select {
	case <-messages:
	case <-time.After(10 * time.Second):
		t.Fatal("the page never reported the click")
	}
	state := readSettingsForm(t, win)
	if state.Action != "stampSettings" {
		t.Fatalf("the page recorded action %q, want stampSettings", state.Action)
	}

	returned := make(chan bool, 1)
	go func() { returned <- handleSettingsAction(win, c, cfg, state) }()

	child := waitForWindowTitled(t, c.T("stampwindow.title"), 15*time.Second)
	select {
	case <-returned:
		t.Fatal("handleSettingsAction returned without the window being answered")
	default:
	}

	r := windowRectOf(child)
	if on := topWindowAt((r.Left+r.Right)/2, (r.Top+r.Bottom)/2); on != child {
		t.Fatalf("the window opened from Settings is covered: a click at its own centre would reach %#x, not %#x", on, child)
	}
	if enabled, _, _ := procIsWindowEnabledT.Call(settings); enabled != 0 {
		t.Fatal("Settings still accepts clicks while the window it opened is waiting to be answered")
	}

	// Twenty clicks into Settings while Go is not reading its channel.
	// Before this pass the ninth one froze the window; now they queue.
	for i := range 20 {
		if _, err := win.Eval("document.getElementById('save-btn').click()"); err != nil {
			t.Fatalf("click %d into Settings failed — the window stopped answering: %v", i+1, err)
		}
	}

	const wmClose = 0x0010
	_, _, _ = procPostMessageT.Call(child, wmClose, 0, 0)
	select {
	case <-returned:
	case <-time.After(20 * time.Second):
		t.Fatal("handleSettingsAction did not return after its window was closed")
	}
	if enabled, _, _ := procIsWindowEnabledT.Call(settings); enabled == 0 {
		t.Fatal("Settings was left disabled after the window it opened closed")
	}
	go func() {
		for range messages {
		}
	}()
}

// waitForWindowTitled finds one of this process's own windows by title,
// and waits until it is actually on screen.
//
// The two are not the same moment: ui.NewWindow creates the frame
// first and only shows and raises it once its WebView2 control and page
// are ready, which is a couple of seconds later. Asserting z-order
// against a window that has been created but not yet shown measures
// nothing about what a person would see.
func waitForWindowTitled(t *testing.T, title string, within time.Duration) uintptr {
	t.Helper()
	class, err := windows.UTF16PtrFromString("LiroBridgeWindow")
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(title)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		h, _, _ := procFindWindowT.Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)))
		if h != 0 {
			if visible, _, _ := procIsWindowVisibleT.Call(h); visible != 0 {
				return h
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no visible window titled %q appeared within %v", title, within)
	return 0
}

// settingsCycles is how many times TestSettingsOpensAndClosesRepeatedly
// runs. Overridable so a longer run can be taken without editing the
// test, the way internal/ui's own lifecycle tests are.
func settingsCycles(t *testing.T) int {
	t.Helper()
	if s := os.Getenv("LIRO_SETTINGS_CYCLES"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			t.Fatalf("LIRO_SETTINGS_CYCLES=%q: want a positive integer", s)
		}
		return n
	}
	return 100
}

// TestSettingsOpensAndClosesRepeatedly is the volume the owner asked
// for: Settings opened, saved and closed a hundred times over.
//
// Each cycle does what runSettingsWindow's own loop does — create the
// window at its production size, post the form, press Save through the
// page, read the whole form back through Eval, then close — and
// alternates between the two closes that exist, Go's own and the title
// bar's, because they take different paths through the teardown.
//
// It deliberately stops short of handleSettingsAction's side effects. A
// save writes the configuration file, the autostart registry value and
// the Explorer context menu entry, and os.Executable() inside a test
// binary is the test binary: running that a hundred times would point
// the developer's own autostart at a temporary file. What is under test
// here is the window lifecycle, which is where the report lived, and
// every part of it up to those side effects is exercised.
func TestSettingsOpensAndClosesRepeatedly(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	n := settingsCycles(t)

	for i := range n {
		win, messages := newProductionSettingsWindow(t, c, cfg)

		if _, err := win.Eval("document.getElementById('save-btn').click()"); err != nil {
			t.Fatalf("cycle %d: clicking Save: %v", i, err)
		}
		select {
		case msg := <-messages:
			if msg.Type != ui.MessageTypeApprove {
				t.Fatalf("cycle %d: Save produced %v, want approve", i, msg.Type)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("cycle %d: Save never reached Go", i)
		}
		if state := readSettingsForm(t, win); state.Action != "save" {
			t.Fatalf("cycle %d: the page recorded action %q, want save", i, state.Action)
		}

		closed := make(chan struct{})
		go func() {
			if i%2 == 0 {
				_ = win.Close()
			} else {
				const wmClose = 0x0010
				_, _, _ = procPostMessageT.Call(win.Handle(), wmClose, 0, 0)
				<-messages // OnClosed's cancel
				_ = win.Close()
			}
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(30 * time.Second):
			t.Fatalf("cycle %d of %d: the settings window would not close", i, n)
		}
	}
}
