//go:build windows

package ui

// The folder chooser behind Settings' "Export audit log" (Task 3, F5
// second-real-run review): F5 §8.4 says the export writes "a copy plus
// a verification report", and this task says it goes where the user
// chooses, not to a directory only the log file knows about.
//
// SHBrowseForFolderW is used rather than IFileOpenDialog with
// FOS_PICKFOLDERS: this package's COM interop is hand-written vtable
// work (D-080), and the modern dialog would mean four more interfaces
// implemented by hand for a chooser with no requirement this one does
// not meet. BIF_NEWDIALOGSTYLE already gives the resizable,
// type-a-path, create-new-folder dialog users expect.
import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSHBrowseForFolderW     = shell32DLL.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW   = shell32DLL.NewProc("SHGetPathFromIDListW")
	procOleInitialize          = ole32DLL.NewProc("OleInitialize")
	procOleUninitialize        = ole32DLL.NewProc("OleUninitialize")
	procCoTaskMemFreeForBrowse = ole32DLL.NewProc("CoTaskMemFree")
)

const (
	// BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE: only real filesystem
	// directories are selectable, in the modern resizable dialog.
	bifReturnOnlyFSDirs = 0x00000001
	bifNewDialogStyle   = 0x00000040

	// bffmInitialized is BFFM_INITIALIZED, the one callback message
	// this dialog is used for: it fires once, when the dialog is ready
	// to be told what to select. bffmSetSelectionW is
	// BFFM_SETSELECTIONW (WM_USER+103), whose lParam is a wide path.
	bffmInitialized   = 1
	bffmSetSelectionW = 0x0400 + 103

	// maxPathW is the classic MAX_PATH the SHGetPathFromIDListW contract
	// is defined against — it writes at most this many wide characters.
	maxPathW = 260
)

// browseCallback is BFFM_INITIALIZED's handler: when the dialog is
// ready, tell it to select the path BROWSEINFOW.lParam points at.
//
// It is created once, as a package-level singleton, rather than per
// call: syscall.NewCallback's trampolines are never released, so one
// per folder chooser would be a slow leak in a tray process meant to
// run for weeks — the same reasoning com_windows.go's vtable
// singletons are built on. It needs no per-call state, because the
// path travels in lParam, which Windows hands back as the data
// argument.
func browseCallbackProc(hwnd uintptr, msg uint32, _ uintptr, data uintptr) uintptr {
	if msg == bffmInitialized && data != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, bffmSetSelectionW, 1, data)
	}
	return 0
}

var browseCallback = syscall.NewCallback(browseCallbackProc)

// browseInfoW mirrors BROWSEINFOW.
type browseInfoW struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	LParam      uintptr
	Image       int32
}

// pickFolder shows the OS folder chooser parented to owner and returns
// the chosen path. ok is false when the user cancelled — which is not
// an error and must not be reported as one.
//
// title is supplied already localised by the caller, like
// ShowRuntimeMissingMessage's: this package has no i18n dependency
// (SPEC §4.2 rule 4).
//
// The dialog runs on its own OS thread with its own OLE apartment
// rather than on the calling goroutine: BIF_NEWDIALOGSTYLE requires
// OleInitialize on the thread that calls SHBrowseForFolderW, and the
// caller here is a plain Go goroutine that may be scheduled onto any
// thread — including one that another window has already initialised
// into a COM apartment of its own (window_windows.go's package doc
// comment). Owning a thread for the dialog's lifetime is the only way
// to be sure of what apartment it is in.
func pickFolder(owner uintptr, title, initial string) (path string, ok bool, err error) {
	type result struct {
		path string
		ok   bool
		err  error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hr, _, _ := procOleInitialize.Call(0)
		if int32(hr) >= 0 {
			defer func() { _, _, _ = procOleUninitialize.Call() }()
		}

		titleBuf, convErr := utf16Buf(title)
		if convErr != nil {
			done <- result{err: convErr}
			return
		}
		// SHBrowseForFolderW runs its own modal message loop, so this
		// goroutine's stack is deep inside foreign code — and can grow,
		// and so move — while the dialog holds the address of bi and of
		// the two buffers bi points at. All three are pinned rather than
		// merely kept alive; see com_windows.go's pinning commentary.
		var pin runtime.Pinner
		defer pin.Unpin()
		display := make([]uint16, maxPathW)
		pin.Pin(&display[0])
		pin.Pin(&titleBuf[0])
		bi := browseInfoW{
			Owner:       owner,
			DisplayName: &display[0],
			Title:       &titleBuf[0],
			Flags:       bifReturnOnlyFSDirs | bifNewDialogStyle,
		}
		// With no starting selection this dialog opens on the Desktop
		// with the Desktop itself selected, so pressing OK — the thing
		// a person does when they meant to look around and changed
		// their mind — silently answers "the Desktop". That is how the
		// agent came to be writing every signed document there
		// (docs/decisions.md). Starting on the folder already chosen,
		// when there is one, makes OK mean "keep this".
		if initial != "" {
			initialBuf, convErr := utf16Buf(initial)
			if convErr == nil {
				pin.Pin(&initialBuf[0])
				bi.LParam = uintptr(unsafe.Pointer(&initialBuf[0]))
				bi.Callback = browseCallback
			}
		}
		idList, _, _ := procSHBrowseForFolderW.Call(pinPtr(&pin, &bi))
		if idList == 0 {
			done <- result{ok: false}
			return
		}
		defer func() { _, _, _ = procCoTaskMemFreeForBrowse.Call(idList) }()

		buf := make([]uint16, maxPathW)
		r, _, _ := procSHGetPathFromIDListW.Call(idList, pinUTF16(&pin, buf))
		if r == 0 {
			done <- result{err: fmt.Errorf("ui: SHGetPathFromIDListW: the chosen item is not a filesystem folder")}
			return
		}
		done <- result{path: windows.UTF16ToString(buf), ok: true}
	}()
	r := <-done
	return r.path, r.ok, r.err
}
