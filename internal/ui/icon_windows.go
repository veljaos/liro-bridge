//go:build windows

package ui

// The tray icon (Task 5, F5 review): loads the real Liro brand mark —
// scripts/genicon's output, embedded at assets/icon.ico — instead of
// the generic IDI_APPLICATION placeholder F5 shipped with. Follows the
// same embed-then-extract-to-a-stable-path pattern loader_windows.go
// already uses for WebView2Loader.dll: LoadImageW's LR_LOADFROMFILE
// needs a real path on disk, not an in-memory buffer, and re-extracting
// on every launch is avoided by naming the file after its own content
// length, exactly like ensureLoaderExtracted does.
import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/veljaos/liro-bridge/internal/platform"
)

//go:embed assets/icon.ico
var trayIconICO []byte

const (
	imageIcon      = 1
	lrLoadFromFile = 0x00000010
	lrDefaultColor = 0x00000000

	// smCXICON/smCYICON and smCXSMICON/smCYSMICON are GetSystemMetrics
	// indices for the large- and small-icon sizes Windows expects at the
	// current DPI (SM_CXICON=11, SM_CYICON=12, SM_CXSMICON=49,
	// SM_CYSMICON=50) — the tray's Shell_NotifyIconW wants the small
	// size; loadTrayIcon always requests it so the icon shown in the
	// notification area is never a downscaled large-icon frame.
	smCXSMICON = 49
	smCYSMICON = 50
)

var (
	procLoadImageW       = user32DLL.NewProc("LoadImageW")
	procGetSystemMetrics = user32DLL.NewProc("GetSystemMetrics")

	// procGetSystemMetricsForDpi resolves the icon-size metrics for an
	// explicit DPI (Task 7, F5 first-real-run review) rather than
	// whatever GetSystemMetrics implicitly associates with the calling
	// thread — the tray's message-only window has no monitor of its own
	// to derive that from, so plain GetSystemMetrics is not reliably
	// correct for it on a per-monitor-DPI-aware process (win32_windows.go's
	// ensureDPIAware sets DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2).
	// Available since Windows 10 1607, the same release
	// SetProcessDpiAwarenessContext (already required) shipped in.
	procGetSystemMetricsForDpi = user32DLL.NewProc("GetSystemMetricsForDpi")
)

// ensureTrayIconExtracted writes the embedded .ico to a stable path
// under the agent's per-user config directory, once — see
// loader_windows.go's ensureLoaderExtracted, whose reasoning (name the
// file after its own byte length, skip rewriting when a same-length
// file already exists) applies identically here.
func ensureTrayIconExtracted() (string, error) {
	dir := platform.ConfigDir(runtime.GOOS, platform.OSEnv)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ui: creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("icon-%d.ico", len(trayIconICO)))
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(trayIconICO)) {
		return path, nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, trayIconICO, 0o644); err != nil {
		return "", fmt.Errorf("ui: writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("ui: renaming %s: %w", tmp, err)
	}
	return path, nil
}

// loadTrayIcon loads the real Liro mark at hwnd's actual DPI's
// small-icon size, picking the sharpest matching frame from the
// multi-size .ico scripts/genicon produced (LoadImageW selects the
// best frame itself when given desired dimensions). Falls back to 0
// (the caller's own IDI_APPLICATION fallback) on any failure — a
// missing or unreadable icon file must never stop the tray from
// starting.
//
// hwnd's own DPI (via GetDpiForWindow, win32_windows.go) — not the
// implicit "current thread" DPI plain GetSystemMetrics would use — is
// what GetSystemMetricsForDpi is asked for: a message-only window has
// no monitor of its own for GetSystemMetrics to correctly associate on
// a per-monitor-DPI-aware process (Task 7). Getting this wrong is
// exactly the kind of defect that silently produces a blurry or
// wrong-sized icon at anything other than 100% scaling — never a
// crash, so it goes unnoticed until someone actually looks at 150%.
func loadTrayIcon(hwnd uintptr) uintptr {
	cx, cy := systemIconSize(hwnd, smCXSMICON, smCYSMICON)
	return loadIconAt(cx, cy)
}

// Window-title-bar icons (Task 4, F5 second-real-run review). The tray
// already showed the real Liro mark; every WebView2 window still showed
// the Windows placeholder in its title bar and in Alt+Tab, because the
// window class registered in win32_windows.go carries no hIcon/hIconSm
// and nothing ever sent WM_SETICON. Both sizes are set explicitly —
// Windows does not derive one from the other, so setting only
// ICON_SMALL leaves Alt+Tab (which reads ICON_BIG) on the placeholder,
// and setting only ICON_BIG leaves the title bar on it.
const (
	wmSetIcon = 0x0080

	iconSmall = 0
	iconBig   = 1

	// SM_CXICON / SM_CYICON — the large-icon size Alt+Tab and the task
	// switcher read, as opposed to smCXSMICON/smCYSMICON above.
	smCXICON = 11
	smCYICON = 12
)

var procSendMessageW = user32DLL.NewProc("SendMessageW")

// systemIconSize returns the icon dimensions Windows expects at hwnd's
// own DPI for the given SM_CX*/SM_CY* metric pair, preferring the
// per-DPI API for the reason loadTrayIcon's doc comment gives.
func systemIconSize(hwnd uintptr, cxMetric, cyMetric uintptr) (cx, cy uintptr) {
	dpi := getDpiForWindow(hwnd)
	if dpi == 0 {
		dpi = 96
	}
	cx, _, _ = procGetSystemMetricsForDpi.Call(cxMetric, uintptr(dpi))
	cy, _, _ = procGetSystemMetricsForDpi.Call(cyMetric, uintptr(dpi))
	if cx == 0 || cy == 0 {
		cx, _, _ = procGetSystemMetrics.Call(cxMetric)
		cy, _, _ = procGetSystemMetrics.Call(cyMetric)
	}
	return cx, cy
}

// loadIconAt loads the embedded Liro mark from its extracted file at
// exactly cx x cy pixels, letting LoadImageW pick the sharpest frame in
// the multi-size .ico. Returns 0 on any failure.
func loadIconAt(cx, cy uintptr) uintptr {
	path, err := ensureTrayIconExtracted()
	if err != nil {
		return 0
	}
	pathBuf, bufErr := utf16Buf(path)
	if bufErr != nil {
		return 0
	}
	var pin runtime.Pinner
	defer pin.Unpin()
	h, _, _ := procLoadImageW.Call(0, pinUTF16(&pin, pathBuf), imageIcon, cx, cy, lrLoadFromFile|lrDefaultColor)
	return h
}

// setWindowIcons puts the real Liro mark in hwnd's title bar
// (ICON_SMALL) and in Alt+Tab / the task switcher (ICON_BIG). A failure
// to load either size is silently left as the Windows placeholder — a
// missing icon must never stop a window from opening.
func setWindowIcons(hwnd uintptr) {
	smallCX, smallCY := systemIconSize(hwnd, smCXSMICON, smCYSMICON)
	if h := loadIconAt(smallCX, smallCY); h != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, h)
	}
	bigCX, bigCY := systemIconSize(hwnd, smCXICON, smCYICON)
	if h := loadIconAt(bigCX, bigCY); h != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconBig, h)
	}
}
