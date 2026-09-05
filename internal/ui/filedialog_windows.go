//go:build windows

package ui

// The file chooser behind the main window's Browse button (F6 §1: "not
// everyone drags").
//
// GetOpenFileNameW rather than IFileOpenDialog, for the same reason
// folder_windows.go gives for SHBrowseForFolderW: this package's COM
// interop is hand-written vtable work (D-080), and the modern dialog
// would mean several more interfaces implemented by hand for a chooser
// with no requirement this one does not meet. Multi-select, a filter,
// and a parent window are all it needs.
import (
	"fmt"
	"path/filepath"
	"runtime"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comdlg32DLL            = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW   = comdlg32DLL.NewProc("GetOpenFileNameW")
	procCommDlgExtendedErr = comdlg32DLL.NewProc("CommDlgExtendedError")
)

const (
	ofnFileMustExist    = 0x00001000
	ofnPathMustExist    = 0x00000800
	ofnAllowMultiSelect = 0x00000200
	// ofnExplorer is what makes a multi-select result NUL-separated
	// (directory, then each file) rather than the ancient
	// space-separated form, which cannot represent a name containing a
	// space — and Serbian document names contain spaces constantly.
	ofnExplorer = 0x00080000
	// ofnNoChangeDir stops the dialog from changing this process's
	// working directory out from under everything else.
	ofnNoChangeDir = 0x00000008

	// multiSelectBufferChars bounds one Browse. The buffer holds the
	// directory plus every selected file name, each NUL-terminated, so
	// it is not a path-length limit but a total-selection one: at a
	// generous 80 characters per name it still holds well over 700
	// files, and F6 §7's largest stated case is two hundred.
	multiSelectBufferChars = 64 * 1024
)

// openFileNameW mirrors OPENFILENAMEW. The field order and the struct
// size are the contract — GetOpenFileNameW rejects a StructSize it does
// not recognise — so nothing here may be reordered.
type openFileNameW struct {
	StructSize    uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	Hook          uintptr
	TemplateName  *uint16
	PvReserved    uintptr
	DwReserved    uint32
	FlagsEx       uint32
}

// pickFiles shows the OS file chooser parented to owner, allowing more
// than one file, and returns the chosen paths. ok is false when the
// user cancelled — a normal outcome, never an error.
//
// filterLabel is the already-localised name of the file type shown in
// the dialog's type dropdown ("PDF documents"); this package has no
// i18n dependency of its own (SPEC §4.2 rule 4), so every string
// arrives translated. The filter offers PDFs first and "all files"
// second, because F6 §1 is explicit that a file a person chooses
// deliberately is theirs to choose — the signing step is what reports
// a non-PDF by name, not the dialog by refusing to show it.
func pickFiles(owner uintptr, title, filterLabel, allFilesLabel string) (paths []string, ok bool, err error) {
	type result struct {
		paths []string
		ok    bool
		err   error
	}
	done := make(chan result, 1)
	go func() {
		// Own the thread for the dialog's lifetime, for the same reason
		// pickFolder does: GetOpenFileNameW runs a modal message loop,
		// and the calling goroutine may otherwise be scheduled onto a
		// thread another window has already put into its own COM
		// apartment.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hr, _, _ := procOleInitialize.Call(0)
		if int32(hr) >= 0 {
			defer func() { _, _, _ = procOleUninitialize.Call() }()
		}

		// A filter is pairs of NUL-terminated strings — label, pattern,
		// label, pattern — closed by a second NUL. utf16Buf cannot
		// build that, since it stops at the first NUL, so it is
		// assembled here as runes.
		filter := utf16Filter(filterLabel, "*.pdf", allFilesLabel, "*.*")
		titleBuf, convErr := utf16Buf(title)
		if convErr != nil {
			done <- result{err: convErr}
			return
		}

		var pin runtime.Pinner
		defer pin.Unpin()
		buf := make([]uint16, multiSelectBufferChars)
		pin.Pin(&buf[0])
		pin.Pin(&filter[0])
		pin.Pin(&titleBuf[0])

		ofn := openFileNameW{
			Owner:       owner,
			Filter:      &filter[0],
			FilterIndex: 1,
			File:        &buf[0],
			MaxFile:     uint32(len(buf)),
			Title:       &titleBuf[0],
			Flags: ofnFileMustExist | ofnPathMustExist | ofnAllowMultiSelect |
				ofnExplorer | ofnNoChangeDir,
		}
		ofn.StructSize = uint32(unsafe.Sizeof(ofn))

		r, _, _ := procGetOpenFileNameW.Call(pinPtr(&pin, &ofn))
		if r == 0 {
			// Zero means cancelled *or* failed; CommDlgExtendedError
			// tells the two apart, and a real failure deserves to be
			// reported rather than silently read as "the user changed
			// their mind".
			code, _, _ := procCommDlgExtendedErr.Call()
			if code != 0 {
				done <- result{err: fmt.Errorf("ui: GetOpenFileNameW failed: CDERR 0x%04X", uint32(code))}
				return
			}
			done <- result{ok: false}
			return
		}
		done <- result{paths: parseMultiSelect(buf), ok: true}
	}()
	res := <-done
	return res.paths, res.ok, res.err
}

// parseMultiSelect decodes OFN_EXPLORER's multi-select result.
//
// With one file chosen the buffer holds one NUL-terminated full path.
// With several, it holds the directory, then each bare file name, each
// NUL-terminated, ended by an empty string. Distinguishing the two is
// what the second element's presence does, and joining is the caller's
// job because the directory has no trailing separator.
func parseMultiSelect(buf []uint16) []string {
	parts := splitUTF16Strings(buf)
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return []string{parts[0]}
	}
	dir := parts[0]
	out := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		if name == "" {
			continue
		}
		// A defensive join: some shells hand back an already-absolute
		// path even in the multi-select form.
		if filepath.IsAbs(name) {
			out = append(out, name)
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out
}

// splitUTF16Strings reads consecutive NUL-terminated strings out of buf
// until an empty one or the end of the buffer.
func splitUTF16Strings(buf []uint16) []string {
	var out []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] != 0 {
			continue
		}
		if i == start {
			break // the empty string that ends the list
		}
		out = append(out, windows.UTF16ToString(buf[start:i]))
		start = i + 1
	}
	return out
}

// utf16Filter builds GetOpenFileNameW's doubly-NUL-terminated filter
// from alternating labels and patterns.
//
// utf16.Encode rather than a rune-by-rune cast: a label is localised
// text, and a cast would silently truncate anything outside the basic
// multilingual plane instead of encoding the surrogate pair. The
// embedded NULs this format is built from are why utf16Buf (which stops
// at the first one) cannot be used here.
func utf16Filter(pairs ...string) []uint16 {
	var b []uint16
	for _, s := range pairs {
		b = append(b, utf16.Encode([]rune(s))...)
		b = append(b, 0)
	}
	return append(b, 0)
}
