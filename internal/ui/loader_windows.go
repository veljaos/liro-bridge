//go:build windows

package ui

// Loads WebView2Loader.dll (F5 §2.1/§2.2) and calls its two exported C
// functions: GetAvailableCoreWebView2BrowserVersionString, used to
// detect the Evergreen Runtime before attempting anything else, and
// CreateCoreWebView2EnvironmentWithOptions, which starts the
// environment-creation COM flow webview2_windows.go/window_windows.go
// finish. See D-080 for why the loader DLL is embedded and redistributed
// rather than assumed present on the machine.
import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/platform"
)

//go:embed assets/webview2/WebView2Loader.dll
var webview2LoaderDLL []byte

var (
	loaderOnce sync.Once
	loaderDLL  *windows.LazyDLL
	loaderErr  error

	procCreateEnvWithOptions   *windows.LazyProc
	procGetAvailableVersionStr *windows.LazyProc
)

// ensureLoaderExtracted writes the embedded loader DLL to a stable path
// under the agent's per-user config directory, if it is not already
// there, and returns that path. Re-extracting on every launch is
// avoided by naming the file after byte length alone — good enough to
// avoid rewriting on the overwhelmingly common case (same binary, same
// embedded DLL) without adding a hashing dependency; a corrupted or
// truncated file left by a crash mid-write is vanishingly unlikely and
// would surface as a clear LoadLibrary failure, not silent misbehaviour.
func ensureLoaderExtracted() (string, error) {
	dir := filepath.Join(platform.ConfigDir(runtime.GOOS, platform.OSEnv), "webview2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ui: creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("WebView2Loader-%d.dll", len(webview2LoaderDLL)))
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(webview2LoaderDLL)) {
		return path, nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, webview2LoaderDLL, 0o644); err != nil {
		return "", fmt.Errorf("ui: writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("ui: renaming %s: %w", tmp, err)
	}
	return path, nil
}

func ensureLoader() error {
	loaderOnce.Do(func() {
		path, err := ensureLoaderExtracted()
		if err != nil {
			loaderErr = err
			return
		}
		loaderDLL = windows.NewLazyDLL(path)
		if err := loaderDLL.Load(); err != nil {
			loaderErr = fmt.Errorf("ui: loading %s: %w", path, err)
			return
		}
		procCreateEnvWithOptions = loaderDLL.NewProc("CreateCoreWebView2EnvironmentWithOptions")
		procGetAvailableVersionStr = loaderDLL.NewProc("GetAvailableCoreWebView2BrowserVersionString")

		// Task 8 (F5 first-real-run review): quitting printed Chromium's
		// own "[...:ERROR:ui\gfx\win\window_impl.cc:172] Failed to
		// unregister class Chrome_WidgetWin_0" to the console during the
		// WebView2 browser process's teardown — its logging, not this
		// project's, and CreateCoreWebView2EnvironmentWithOptions has no
		// options object here to configure it through (see
		// createEnvironment's nil third argument). The WebView2 loader
		// also reads WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS straight from
		// the environment and forwards it to every browser process it
		// starts, and --disable-logging is the documented way to turn
		// off Chromium's own logging destinations — set here on general
		// principle (it silences other Chromium console noise too), but
		// verified NOT to reliably suppress this specific line on its
		// own: it reproduced again in testing with this set. The window
		// class in the message appears to belong to a component that
		// runs in-process (loaded alongside WebView2Loader.dll itself)
		// rather than to the sandboxed browser process this flag
		// reaches, and its teardown fires at this process's own exit —
		// window_windows.go's Close (which now waits for this process's
		// own COM teardown before returning) and tray_windows.go's
		// webView2ExitGrace (a short pause before the tray process
		// actually exits, after any WebView2 window has been used) are
		// what actually reduce how often this fires, by giving that
		// teardown more time to finish before ExitProcess — not this.
		setAdditionalBrowserArguments("--disable-logging")
	})
	return loaderErr
}

// setAdditionalBrowserArguments appends args to
// WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS rather than overwriting it, so
// a value the user or an admin policy has already set in the
// environment is preserved rather than silently dropped.
func setAdditionalBrowserArguments(args string) {
	if existing := os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"); existing != "" {
		args = existing + " " + args
	}
	_ = os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", args)
}

// detectRuntime implements F5 §2.2: calls
// GetAvailableCoreWebView2BrowserVersionString(nil, &versionInfo).
// A non-nil version string with HRESULT S_OK means the Evergreen
// Runtime is installed; any failure (most commonly
// HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND) when nothing is installed)
// means it is not, and the caller shows the native MessageBox this
// package's detectRuntime does not itself show — DetectRuntime is a
// pure query, deliberately with no UI side effect, so callers can log
// or test the result independently of showing anything.
func detectRuntime() (available bool, version string, err error) {
	if err := ensureLoader(); err != nil {
		return false, "", err
	}
	var pin runtime.Pinner
	defer pin.Unpin()
	var versionPtr uintptr
	r1, _, _ := procGetAvailableVersionStr.Call(0, pinPtr(&pin, &versionPtr))
	if int32(r1) < 0 {
		// A failing HRESULT here — most commonly
		// HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND) — means the Evergreen
		// Runtime is not installed. That is a normal, expected outcome
		// this function reports through the boolean return, not an error
		// (F5 §2.2: the caller must not crash or fail silently, it must
		// show a clear message, which requires distinguishing "absent"
		// from "something else went wrong").
		if versionPtr != 0 {
			coTaskMemFree(versionPtr)
		}
		return false, "", nil
	}
	return true, stringFromLPWSTR(versionPtr), nil
}

// showNativeMessage implements ShowWarning and ShowNotice (window.go)
// on Windows: a native MessageBox, since on both of those paths there
// is no WebView2 window to say anything in.
func showNativeMessage(title, body string, informational bool) {
	icon := uintptr(mbIconWarning)
	if informational {
		icon = mbIconInformation
	}
	messageBox(title, body, icon)
}

// createEnvironment issues CreateCoreWebView2EnvironmentWithOptions and
// pumps the message loop (win32_windows.go) until the completion
// handler fires — this must run on the same OS thread that will host
// the window's message loop (F5 §2.1's STA requirement), which is
// exactly the thread window_windows.go's NewWindow goroutine locks
// itself to before calling this.
func createEnvironment(userDataFolder string) (uintptr, error) {
	if err := ensureLoader(); err != nil {
		return 0, err
	}
	udfBuf, err := utf16Buf(userDataFolder)
	if err != nil {
		return 0, err
	}

	// The user data folder path and the completion handler are both
	// pinned across the whole wait below, not just across the call that
	// starts it: CreateCoreWebView2EnvironmentWithOptions is
	// asynchronous, so WebView2 holds both addresses until it invokes
	// the handler, and pumpUntil is exactly the kind of deep call chain
	// that grows — and therefore moves — this goroutine's stack. See
	// com_windows.go's pinning commentary and D-101.
	var pin runtime.Pinner
	defer pin.Unpin()
	h := newEnvironmentCompletedHandler()
	defer h.release()
	r1, _, callErr := procCreateEnvWithOptions.Call(0, pinUTF16(&pin, udfBuf), 0, h.addr())
	if int32(r1) < 0 {
		return 0, fmt.Errorf("CreateCoreWebView2EnvironmentWithOptions: HRESULT 0x%08X (%v)", uint32(r1), callErr)
	}

	pumpUntil(func() bool { return h.done })
	if int32(h.hr) < 0 {
		return 0, fmt.Errorf("CreateCoreWebView2EnvironmentWithOptions completed with HRESULT 0x%08X", uint32(h.hr))
	}
	return h.env, nil
}
