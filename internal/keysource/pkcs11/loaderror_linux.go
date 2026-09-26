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

// explainLoadFailure turns dlerror's text into what a person can act on,
// and keeps dlerror's own words beside it (F12 §10: "a Failure with a
// readable reason, not a crash").
//
// The reason matters more here than on Windows because of the failure
// mode Windows does not have: an older vendor module can want an OpenSSL
// the distribution no longer ships, and the loader says so in its own
// vocabulary — a soname or a version node — which reads as noise to the
// person whose card it is. The loader's words are still carried, as data
// (SPEC §9.3): they are what a vendor's support desk will ask for.
func explainLoadFailure(module, dlerr string) string {
	sentence := ""
	switch {
	case reVersion.MatchString(dlerr):
		m := reVersion.FindStringSubmatch(dlerr)
		lib, node := filepath.Base(m[1]), m[2]
		sentence = "it was built against " + node + " of " + lib +
			", and the copy on this system does not provide it"
		if isOpenSSL(lib) || strings.HasPrefix(node, "OPENSSL_") {
			sentence += openSSLHint
		}
	case reUndefined.MatchString(dlerr):
		sym := reUndefined.FindStringSubmatch(dlerr)[1]
		sentence = "it needs the function " + sym +
			", which no library on this system provides; it was probably built for a different system"
	case reCannotOpen.MatchString(dlerr):
		file := reCannotOpen.FindStringSubmatch(dlerr)[1]
		if filepath.Clean(file) == filepath.Clean(module) {
			sentence = "there is no file at this path"
		} else {
			sentence = "it needs " + filepath.Base(file) + ", which is not installed on this system"
			if isOpenSSL(file) {
				sentence += openSSLHint
			}
		}
	case strings.Contains(dlerr, "wrong ELF class: ELFCLASS32"):
		sentence = "it is a 32-bit library, and this program is 64-bit"
	case strings.Contains(dlerr, "invalid ELF header"), strings.Contains(dlerr, "file too short"):
		sentence = "it is not a shared library"
	}
	if sentence == "" {
		return dlerr
	}
	return sentence + " (the system loader said: " + dlerr + ")"
}

// openSSLHint is the one piece of advice the loader's words cannot give:
// what an old OpenSSL soname means for the person holding the card.
const openSSLHint = ". That is an older OpenSSL than this distribution ships: the module was built for an older system, and its vendor's build for this one is needed"

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

// exitHint is appended to a probe child's death. Only the loader's status
// has a hint: anything else is the module's own crash, and guessing at it
// would be worse than the number.
func exitHint(module string, code int) string {
	if code != loaderExit {
		return ""
	}
	return "; exit 127 is the system loader stopping the process while loading this module or a library it needs — `ldd " + module + "` shows which"
}
