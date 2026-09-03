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
	"unsafe"

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

// loadTrayIcon loads the real Liro mark at the current DPI's small-icon
// size, picking the sharpest matching frame from the multi-size .ico
// scripts/genicon produced (LoadImageW selects the best frame itself
// when given desired dimensions). Falls back to 0 (the caller's own
// IDI_APPLICATION fallback) on any failure — a missing or unreadable
// icon file must never stop the tray from starting.
func loadTrayIcon() uintptr {
	path, err := ensureTrayIconExtracted()
	if err != nil {
		return 0
	}
	cx, _, _ := procGetSystemMetrics.Call(uintptr(smCXSMICON))
	cy, _, _ := procGetSystemMetrics.Call(uintptr(smCYSMICON))
	pathPtr, pathBuf, bufErr := utf16Ptr(path)
	if bufErr != nil {
		return 0
	}
	h, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(pathPtr)), imageIcon, cx, cy, lrLoadFromFile|lrDefaultColor)
	runtime.KeepAlive(pathBuf)
	return h
}
