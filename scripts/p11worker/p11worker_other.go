//go:build !windows

package main

import (
	"fmt"
	"os"
)

// p11worker is Windows-only for now, and for two reasons rather than one.
//
// It drives internal/keysource/pkcs11 through the worker, and that binding
// reaches a module with syscall.LoadLibrary, whose offsets are measured for
// Windows x64: CK_ULONG is 4 bytes there (LLP64) and 8 here (LP64), so the
// layouts would be wrong rather than merely unavailable (D-287 measured the
// whole table). And it draws the PIN screen, which is a native Win32 dialog —
// the one window in this product that is not HTML (SPEC §10, D-277, D-278).
//
// F12 §5 is where the second half changes. internal/pinscreen already builds
// the sentences on this platform; what is missing here is the window, not the
// words.
func main() {
	fmt.Fprintln(os.Stderr,
		"p11worker: Windows only for now. It loads a module with LoadLibrary, whose\n"+
			"structure offsets are measured for Windows x64, and it draws a native Win32\n"+
			"PIN dialog. See F12 §5 for the second half and SPEC §1.1 for the first.")
	os.Exit(1)
}
