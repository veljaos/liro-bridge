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

	// Assets is served to the page over a virtual host mapping (F5
	// §2.4) — never file:// and never a local HTTP server, which would
	// be a second listening socket and contradicts the security model
	// (SPEC §6.1).
	Assets fs.FS

	// VirtualHost is the hostname Assets is mapped to, e.g.
	// "liro.local". The page's own links and asset references use this
	// host; it resolves to nothing outside the WebView2 instance.
	VirtualHost string

	// StartPage is the path within Assets to navigate to first, e.g.
	// "/consent.html".
	StartPage string

	// OnMessage is called for every validated message the page sends
	// (F5 §2.4). Messages that do not parse or whose Type is not one of
	// the three recognised values are dropped and logged before this is
	// ever called (F5 §2.4/§10) — see ParseMessage.
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
	OnFilesDropped func(paths []string)

	// OnClosed is called once when the window is closed by the user (the
	// title bar close button, Alt+F4, or Escape — F5 §5.6) rather than
	// by a call to Window.Close from Go. F5 §2.3: closing the window is
	// equivalent to Cancel: the caller is responsible for treating this
	// the same as an explicit MessageTypeCancel when a decision is still
	// pending.
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

// ShowRuntimeMissingMessage shows a native, blocking message box
// explaining that the WebView2 runtime is not installed (F5 §2.2's
// "must not crash and must not fail silently" requirement) — deliberately
// not a WebView2 window, which by definition cannot exist yet on this
// path. title and body are supplied already localised by the caller
// (cmd/liro-bridge, via internal/i18n): this package has no i18n
// dependency of its own, matching SPEC §4.2 rule 4's independence
// between internal/ui, internal/api and internal/cli.
func ShowRuntimeMissingMessage(title, body string) {
	showRuntimeMissingMessage(title, body)
}

// ChooseFolder shows the OS folder chooser parented to the window
// whose HWND is owner (Window.Handle), returning the chosen path and
// whether the user chose one at all. A cancelled dialog is ok == false
// with a nil error — cancelling is a normal outcome, not a failure.
// title arrives already localised, like ShowRuntimeMissingMessage's.
func ChooseFolder(owner uintptr, title string) (path string, ok bool, err error) {
	return pickFolder(owner, title)
}

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
