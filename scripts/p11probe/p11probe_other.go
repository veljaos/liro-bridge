//go:build !windows

package main

import (
	"fmt"
	"os"
)

// p11probe is Windows-only for now. It reaches a PKCS#11 module through
// syscall.LoadLibrary, which has no counterpart on macOS or Linux, and the
// layouts it reads are measured for Windows x64: CK_ULONG is 4 bytes there
// (LLP64) and 8 on Linux (LP64), so the offsets in the Windows file are wrong
// for this platform rather than merely unreachable.
//
// F12 and F13 are where that changes, and when it does this file is where the
// dlopen/dlsym half belongs — with its own measured offsets, not with the
// Windows ones widened by hand.
func main() {
	fmt.Fprintln(os.Stderr,
		"p11probe: Windows only for now. It loads a module with LoadLibrary and\n"+
			"reads structures whose offsets are measured for Windows x64; CK_ULONG is\n"+
			"4 bytes there and 8 here, so the layouts would be wrong rather than just\n"+
			"unavailable. See F12/F13.")
	os.Exit(1)
}
