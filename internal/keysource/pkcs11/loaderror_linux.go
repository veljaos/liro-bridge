//go:build linux

package pkcs11

import (
	"path/filepath"
	"regexp"
	"strings"
)

// The dynamic loader's messages this file recognises, in glibc's words.
// Each was produced by a real module in loaderror_linux_test.go rather than
// copied from documentation (D-359).
var (
	// "<file>: cannot open shared object file: No such file or directory"
	reCannotOpen = regexp.MustCompile(`^(.+?): cannot open shared object file`)
	// "<module>: undefined symbol: <name>"
	reUndefined = regexp.MustCompile(`undefined symbol: (\S+)`)
	// "<library>: version `<node>' not found (required by <module>)"
	reVersion = regexp.MustCompile("^(.+?): version `([^']+)' not found")
)

// classifyLoadFailure reads dlerror's text into a LoadError, or nil for
// words it does not recognise — which are then passed on as they are (F12
// §10: "a Failure with a readable reason, not a crash").
//
// The reason matters more here than on Windows because of the failure mode
// Windows does not have: an older vendor module can want an OpenSSL the
// distribution no longer ships, and the loader says so in its own vocabulary
// — a soname or a version node — which reads as noise to the person whose
// card it is. The sentence they read is the catalogue's (D-362); the loader's
// words travel with it.
func classifyLoadFailure(module, dlerr string) *LoadError {
	e := &LoadError{Module: module, Loader: dlerr}
	switch {
	case reVersion.MatchString(dlerr):
		m := reVersion.FindStringSubmatch(dlerr)
		e.Kind, e.Name, e.Library = LoadMissingVersion, m[2], filepath.Base(m[1])
		e.OldOpenSSL = isOpenSSL(e.Library) || strings.HasPrefix(e.Name, "OPENSSL_")
	case reUndefined.MatchString(dlerr):
		e.Kind, e.Name = LoadMissingFunction, reUndefined.FindStringSubmatch(dlerr)[1]
	case reCannotOpen.MatchString(dlerr):
		file := reCannotOpen.FindStringSubmatch(dlerr)[1]
		if filepath.Clean(file) == filepath.Clean(module) {
			e.Kind = LoadNoFile
		} else {
			e.Kind, e.Name, e.OldOpenSSL = LoadMissingLibrary, filepath.Base(file), isOpenSSL(file)
		}
	case strings.Contains(dlerr, "wrong ELF class: ELFCLASS32"):
		e.Kind = LoadWrongClass
	case strings.Contains(dlerr, "invalid ELF header"), strings.Contains(dlerr, "file too short"):
		e.Kind = LoadNotALibrary
	default:
		return nil
	}
	return e
}

// explainLoadFailure is the English form of the same, for a caller that
// wants a string.
func explainLoadFailure(module, dlerr string) string {
	if e := classifyLoadFailure(module, dlerr); e != nil {
		return e.Error()
	}
	return dlerr
}

// isOpenSSL reports whether a soname is one of OpenSSL's two libraries,
// at a version this distribution does not ship. libssl.so.3 and
// libcrypto.so.3 are current on every supported distribution (F12 §3.1),
// so a module asking for them and not finding them has a different
// problem, and the OpenSSL advice would send the person the wrong way.
func isOpenSSL(file string) bool {
	base := filepath.Base(file)
	for _, lib := range []string{"libssl.so.", "libcrypto.so."} {
		if strings.HasPrefix(base, lib) {
			return !strings.HasPrefix(base, lib+"3")
		}
	}
	return false
}

// loaderExit is the status glibc's dynamic loader ends a process with when
// it cannot continue — measured here as the probe child's exit when ld.so
// asserted inside dlopen, on a module needing a symbol version from a
// library that has none (D-359). No in-process code survives that, which
// is F12 §2's reason for the child; this only makes the Failure say where
// to look instead of giving a bare number.
const loaderExit = 127

// exitHint is the LoadError for a probe child's death, when the status
// says the loader was the reason. Only the loader's status has one: anything
// else is the module's own crash, and guessing at it would be worse than the
// number.
func exitHint(module string, code int) *LoadError {
	if code != loaderExit {
		return nil
	}
	return &LoadError{Kind: LoadLoaderStopped, Module: module}
}
