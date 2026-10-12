//go:build darwin && cgo

package pkcs11

import (
	"path/filepath"
	"regexp"
	"strings"
)

// classifyLoadFailure reads dyld's dlerror text (F13, [[D-463]]) into the
// same LoadError kinds Linux's glibc text is read into
// (loaderror_linux.go). The wording below is dyld's as documented and as
// macOS prints it — "tried: '<path>' (no such file)", "Library not
// loaded: <name>", "Symbol not found: _<name>", "incompatible
// architecture", "not a mach-o file" — **recalled, not measured**: no
// module has been made to fail on a Mac. Text it does not recognise is
// nil, and the caller says the loader's own words, which is what Linux
// does for text it does not recognise either.
var (
	reDyldLibrary = regexp.MustCompile(`Library not loaded: (\S+)`)
	reDyldSymbol  = regexp.MustCompile(`Symbol not found: _?(\S+)`)
)

func classifyLoadFailure(module, dlerr string) *LoadError {
	e := &LoadError{Module: module, Loader: dlerr}
	switch {
	case reDyldLibrary.MatchString(dlerr):
		lib := filepath.Base(reDyldLibrary.FindStringSubmatch(dlerr)[1])
		e.Kind, e.Name = LoadMissingLibrary, lib
		e.OldOpenSSL = strings.HasPrefix(lib, "libssl") || strings.HasPrefix(lib, "libcrypto")
	case reDyldSymbol.MatchString(dlerr):
		e.Kind, e.Name = LoadMissingFunction, reDyldSymbol.FindStringSubmatch(dlerr)[1]
	case strings.Contains(dlerr, "incompatible architecture"):
		e.Kind = LoadWrongClass
	case strings.Contains(dlerr, "not a mach-o file"):
		e.Kind = LoadNotALibrary
	case strings.Contains(dlerr, "(no such file)"):
		e.Kind = LoadNoFile
	default:
		return nil
	}
	return e
}
