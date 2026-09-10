// Package ui implements the agent's on-screen windows (F5): a WebView2
// host driven by hand-written COM interop, a tray icon, and the three
// windows (consent, pairing, settings) built on top of it.
//
// WebView2 has no supported Go binding (F5 §2.1). Everything that talks
// to it directly — vtable layout, callback trampolines, the Win32
// message loop — lives in the *_windows.go files and cannot be
// meaningfully unit-tested, mirroring the split
// internal/keysource/windowscng and internal/platform already use
// between real*.go glue and testable core logic. Everything else (view
// models, message validation, sanitisation) is plain Go, tested without
// a window (F5 §10).
package ui

import (
	"errors"
	"io/fs"
)

// ErrUnsupportedPlatform is returned by every function in this package
// on a platform other than Windows. WebView2 is a Windows-only API
// (SPEC §11.11 establishes the same Windows-only scope for CNG); macOS
// and Linux get their own web view hosts in phases 12 and 13.
var ErrUnsupportedPlatform = errors.New("ui: WebView2 is only supported on Windows")

// ErrWindowClosed is returned by PostJSON and Eval when the window has
// already closed. It is not a failure: a person who closes a window
// while it is still drawing has cancelled it, which is exactly what
// closing it means everywhere else in this program (Options.OnClosed).
// Callers that would otherwise report "the settings window failed"
// should treat it as the cancellation it is — errors.Is finds it
// through any wrapping.
var ErrWindowClosed = errors.New("ui: window is closed")

// Options configures one native window hosting a WebView2 page.
type Options struct {
	// Title is the native window title (shown in the taskbar and Alt+Tab,
	// not inside the page itself).
	Title string

	// Width and Height are the window's client area, in DPI-independent
	// points (F5 §2.3). The window is fixed-size and not resizable.
	Width, Height int

	// AlwaysOnTop keeps the window topmost and takes focus when shown
	// (F5 §2.3) — required for the consent window: a consent request the
	// user does not see is a consent request that times out.
	AlwaysOnTop bool

	// Owner is the HWND of the window this one is opened from, when a
	// caller opens a second window and blocks waiting for its answer.
	// Zero means the window stands on its own.
	//
	// An owned window is kept above its owner by Windows itself,
	// whatever either one's topmost flag says; it is centred on its
	// owner rather than on the monitor under the cursor; and its owner
	// is disabled for as long as it is up.
	//
	// All three matter, and the one window this project opened from
	// another had none of them. Settings is always-on-top and the
	// window it opened was not, so the new window was created
	// *underneath* it, exactly covered, while the caller sat waiting
	// for a click on something nobody could see. Naming the owner is
	// what makes "opened from" mean something to the window manager
	// and not only to the code.
	Owner uintptr

	// Assets is served to the page over a virtual host mapping (F5
	// §2.4) — never file:// and never a local HTTP server, which would
	// be a second listening socket and contradicts the security model
	// (SPEC §6.1).
	Assets fs.FS

	// VirtualHost is the hostname Assets is mapped to, e.g.
	// "liro.invalid". The page's own links and asset references use this
	// host; it resolves to nothing outside the WebView2 instance.
	VirtualHost string

	// ScratchHost and ScratchDir map a second virtual host onto a real
	// directory this caller owns, for content that is generated while
	// the window is open rather than embedded in the binary — the
	// rendered page images the stamp placement window shows (F6b §2.1).
	//
	// They are separate from Assets for two reasons. Assets is
	// content-addressed and shared between every window in the process,
	// which is exactly wrong for files that change while one window is
	// looking at them; and an image that is a page of the document
	// being signed should live for that window's lifetime and no
	// longer, which is the caller's business to arrange and not this
	// package's.
	//
	// Leaving either empty maps nothing, which is what every window but
	// one does.
	ScratchHost string
	ScratchDir  string

	// StartPage is the path within Assets to navigate to first, e.g.
	// "/consent.html".
	StartPage string

	// OnMessage is called for every validated message the page sends
	// (F5 §2.4). Messages that do not parse or whose Type is not one of
	// the three recognised values are dropped and logged before this is
	// ever called (F5 §2.4/§10) — see ParseMessage.
	//
	// It runs on a goroutine this package owns, never on the window's
	// own message-loop thread, and callbacks are delivered strictly in
	// the order the window produced them. A handler that blocks
	// therefore delays later callbacks and nothing else: the window
	// keeps painting, keeps answering the title bar, and can still be
	// closed. It did not always work that way — see the doc comment on
	// window_windows.go's event dispatch for what a blocking handler
	// used to do to the whole program.
	OnMessage func(Message)

	// OnFilesDropped is called with the absolute paths of files dropped
	// onto the window from Explorer (F6 §1). A window that sets it
	// becomes a drop target; one that leaves it nil is not, and behaves
	// exactly as it did before this option existed.
	//
	// Setting it also turns WebView2's own external-drop handling off,
	// because the two are mutually exclusive: with the control handling
	// drops, the page receives them and the host never sees the paths
	// (the web platform deliberately does not expose a File's path).
	// With it off, the drop falls through to the native frame, which is
	// where a desktop application can read real paths from the shell.
	//
	// Paths arrive exactly as the shell supplies them, including
	// directories — deciding what is a PDF, what is a folder to look
	// inside, and what to refuse is the caller's business, not this
	// package's.
	//
	// Delivered on the same goroutine, in the same order, as OnMessage.
	OnFilesDropped func(paths []string)

	// OnClosed is called once when the window is closed by the user (the
	// title bar close button, Alt+F4, or Escape — F5 §5.6) rather than
	// by a call to Window.Close from Go. F5 §2.3: closing the window is
	// equivalent to Cancel: the caller is responsible for treating this
	// the same as an explicit MessageTypeCancel when a decision is still
	// pending.
	//
	// Delivered on the same goroutine, in the same order, as OnMessage —
	// so a message the page sent before the window was closed always
	// reaches the caller first.
	OnClosed func()
}

// Window is a live, on-screen WebView2 host.
type Window interface {
	// PostJSON marshals v to JSON and runs it as a script argument in
	// the page via ExecuteScript (F5 §2.4: "Go -> page:
	// ExecuteScript with a JSON payload. Never build JavaScript by
	// string concatenation with user data in it."). The page-side script
	// embedded in every window's HTML defines a single
	// `__liroReceive(json)` function that PostJSON's generated call
	// invokes; JSON.parse, not string concatenation, is what turns the
	// payload back into an object on the page side.
	PostJSON(v any) error

	// Eval runs script in the page and returns its JSON-encoded result
	// (the same value ExecuteScript itself returns — "undefined", a
	// thrown exception, or a value JSON cannot encode all become the
	// JSON string "null", per the WebView2 API's own documented
	// behaviour). Used by the settings window to read back its form
	// state without widening the page->Go message surface past the
	// three types F5 §2.4 specifies (see D-08x) — script is trusted,
	// written by this project, never built from untrusted input.
	Eval(script string) (string, error)

	// Navigate replaces the page this window is showing with another
	// one from Assets, e.g. "/pages/consent.html". It blocks until the
	// new page has loaded and its scripts have run, exactly as
	// NewWindow blocks for Options.StartPage — so the caller's first
	// PostJSON after it lands in a page that is ready to receive it.
	//
	// This is what makes a several-step flow one window rather than
	// several: the native frame, its position and its WebView2 instance
	// are kept, and only the content changes. A second WebView2 window
	// costs a little over two seconds to create on the machine this was
	// measured on; a navigation between two embedded pages costs a
	// fraction of that and does not take the foreground from anything.
	//
	// Only VirtualHost content can be navigated to: the argument is a
	// path within Assets, never a URL, so nothing this method is given
	// can send the window somewhere off the machine.
	Navigate(page string) error

	// Resize changes the window's client area to width x height
	// DPI-independent points, keeping the window's own centre where it
	// is rather than re-centring on a monitor — a window that jumps
	// across the screen at every step of a flow is a window that has to
	// be found again at every step.
	Resize(width, height int) error

	// Close closes the window from Go. Idempotent. Does not invoke
	// OnClosed — that callback fires only for a user-initiated close (see
	// Options.OnClosed).
	Close() error

	// Handle returns the native HWND backing this window, so callers
	// that must parent an OS dialog to it — the smart card KSP's PIN
	// prompt via NCRYPT_WINDOW_HANDLE_PROPERTY (Task 2, F2 §2.3) — have
	// something other than 0 to hand it. Never 0 once NewWindow has
	// returned successfully.
	Handle() uintptr
}

// DetectRuntime reports whether the WebView2 Evergreen Runtime is
// installed (F5 §2.2), and its version string if so. It never launches
// a window — this is safe to call at startup to decide whether to show
// the "runtime missing" native message box before attempting anything
// else.
func DetectRuntime() (available bool, version string, err error) {
	return detectRuntime()
}

// ShowWarning shows a native, blocking message box about something
// that is wrong — the WebView2 runtime not being installed (F5 §2.2's
// "must not crash and must not fail silently" requirement), which is
// deliberately not reported in a WebView2 window, since by definition
// none can exist on that path.
//
// title and body are supplied already localised by the caller
// (cmd/liro-bridge, via internal/i18n): this package has no i18n
// dependency of its own, matching SPEC §4.2 rule 4's independence
// between internal/ui, internal/api and internal/cli.
func ShowWarning(title, body string) {
	showNativeMessage(title, body, false)
}

// ShowNotice is the same box for something that is not wrong: what an
// uninstall left behind and where (F10 §3.3). Two functions rather
// than one with a flag, because the two say different things to a
// person and the icon is part of what they say — a warning triangle
// over "your audit log is still here" would report good news as a
// problem.
func ShowNotice(title, body string) {
	showNativeMessage(title, body, true)
}

// ChooseFolder shows the OS folder chooser parented to the window
// whose HWND is owner (Window.Handle), returning the chosen path and
// whether the user chose one at all. A cancelled dialog is ok == false
// with a nil error — cancelling is a normal outcome, not a failure.
// title arrives already localised, like ShowRuntimeMissingMessage's.
//
// initial is the folder to start on and preselect; empty starts where
// the OS would. Passing the folder the caller already holds is what
// keeps an accidental OK from meaning "the Desktop" — see pickFolder.
func ChooseFolder(owner uintptr, title, initial string) (path string, ok bool, err error) {
	return pickFolder(owner, title, initial)
}

// IconFilePath returns a path to the real Liro mark as an .ico file on
// disk, extracting it from the embedded asset on first use.
//
// It exists for callers that must hand Windows a *file* rather than an
// HICON — the Explorer context-menu registration, whose "Icon" value is
// a path (F6 §2). The executable itself cannot serve: it carries no
// icon resource of its own until F10 builds one (SPEC §5 puts
// cmd/liro-bridge/rsrc.syso under packaging), so pointing at it gives
// the generic Windows application icon.
func IconFilePath() (string, error) { return iconFilePath() }

// ChooseFiles shows the OS file chooser parented to owner, allowing
// more than one file to be selected at once (F6 §1's Browse button —
// "not everyone drags"). A cancelled dialog is ok == false with a nil
// error.
//
// title, filterLabel and allFilesLabel arrive already localised, like
// every other string this package is handed. Both a PDF filter and an
// "all files" entry are offered: F6 §1 is explicit that a file a person
// chooses deliberately is not something to filter away, and the signing
// step is what reports a non-PDF by name.
func ChooseFiles(owner uintptr, title, filterLabel, allFilesLabel string) (paths []string, ok bool, err error) {
	return pickFiles(owner, title, filterLabel, allFilesLabel)
}
