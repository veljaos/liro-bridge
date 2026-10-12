//go:build darwin && cgo

package pkcs11

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// Module discovery on macOS (F13, [[D-463]]).
//
// **Written without a Mac and never run against a real module.** The
// loading, the probe child and the session code are Linux's own
// (module_unix.go and its neighbours), unchanged: macOS has dlopen with
// the same RTLD_NOW|RTLD_LOCAL meaning, and the unix code uses the
// header's own C types through cgo, so the compiler lays the structures
// out — there is no Go copy of them to disagree with it (Windows has
// one, packed to one byte, which F11 had to measure).
//
// What is macOS's own is where modules live, and every path below is
// from the vendors' documentation as recalled, **not read off an
// installed package**: SafeSign for macOS installs libaetpkss.dylib under
// /usr/local/lib; OpenSC installs under /Library/OpenSC/lib, and
// Homebrew's under its prefix. A module elsewhere is reached by the
// person's own pkcs11ModulePath (F11 §3), which Candidates puts first.
func libraryDirs() []string {
	dirs := []string{
		"/usr/local/lib",
		"/Library/OpenSC/lib",
		"/opt/homebrew/lib", // Homebrew on Apple silicon
	}
	if home := os.Getenv("HOME"); home != "" {
		dirs = append(dirs, filepath.Join(home, "lib"))
	}
	return dirs
}

func knownModulePaths() []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	add := func(vendor, name string) {
		for _, dir := range libraryDirs() {
			p := filepath.Join(dir, name)
			if seen[p] {
				return
			}
			if _, err := os.Stat(p); err == nil {
				out = append(out, Candidate{Path: p, Vendor: vendor, Origin: OriginKnown})
				seen[p] = true
				return
			}
		}
	}
	// SPEC §11.11: Pošta's SafeSign is the one Serbian middleware with a
	// macOS build.
	add("A.E.T. Europe (SafeSign)", "libaetpkss.dylib")
	add("OpenSC", "opensc-pkcs11.so")
	add("OpenSC", "opensc-pkcs11.dylib")
	return out
}

// Modules probes every candidate out of process — Linux's function, for
// Linux's reason (F12 §2, D-275): a module that takes the process down
// takes only the child.
func Modules(configured string, stderr func(modulePath string) io.Writer) ([]Candidate, []Failure) {
	var ok []Candidate
	var bad []Failure
	for _, c := range Candidates(configured) {
		res, err := probeOutOfProcess(context.Background(), c.Path, sinkFor(stderr, c.Path))
		if err != nil {
			bad = append(bad, Failure{Candidate: c, Err: err})
			continue
		}
		c.Manufacturer = res.Manufacturer
		c.LibraryDescription = res.LibraryDescription
		ok = append(ok, c)
	}
	return ok, bad
}

// sinkFor is discover_windows.go's, duplicated for the reason it gives
// there (D-295).
func sinkFor(stderr func(modulePath string) io.Writer, path string) io.Writer {
	if stderr == nil {
		return nil
	}
	return stderr(path)
}
