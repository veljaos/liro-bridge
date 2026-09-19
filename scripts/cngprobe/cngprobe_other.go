//go:build !windows

package main

import (
	"fmt"
	"os"
)

// cngprobe is Windows-only and permanently so, unlike p11probe. It reads the
// Windows current-user certificate store through CNG, which has no counterpart
// on macOS or Linux — there is nothing here for it to port to. The question it
// answers on Windows ("what does the OS show this program?") is answered on
// those platforms by p11probe and a module, which is the whole reason F11 §4's
// step 3 exists.
func main() {
	fmt.Fprintln(os.Stderr,
		"cngprobe: Windows only, and not portable. It reads the Windows\n"+
			"certificate store through CNG; on this platform the equivalent\n"+
			"question is p11probe's, asked of a PKCS#11 module.")
	os.Exit(1)
}
