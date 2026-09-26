//go:build linux

package pkcs11

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Module discovery on Linux (F12 §10), which differs from the Windows
// list in three ways rather than in path separators.
//
//   - **Multiarch.** There is no one library directory:
//     /usr/lib/x86_64-linux-gnu on Debian and Ubuntu, /usr/lib64 on
//     Fedora, /usr/lib on both for some packages. A list that names one
//     finds nothing on the other distribution.
//   - **The p11-kit registry**, which Windows has no equivalent of:
//     /usr/share/p11-kit/modules/*.module and its per-user counterpart
//     are files a *module's own package* installs to say where it is.
//     That is better evidence than a path this project guessed, and it
//     is the only mechanism by which a vendor this project has never
//     heard of can be found at all.
//   - **Fewer vendors.** SafeSign is the only one of the three Serbian
//     middlewares shipping a Linux build (SPEC §11.11), so a person
//     holding a MUP or Halcom card gets nothing from the known list and
//     must be told so rather than shown "no certificates found" —
//     which is SPEC §11.11's own requirement and F11 §0.1's finding.

// p11KitModuleDirs are where p11-kit keeps the .module files that name
// an installed module. The per-user one first, because a person who has
// installed a module for themselves means that one.
func p11KitModuleDirs() []string {
	var out []string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		out = append(out, filepath.Join(xdg, "pkcs11", "modules"))
	} else if home := os.Getenv("HOME"); home != "" {
		out = append(out, filepath.Join(home, ".config", "pkcs11", "modules"))
	}
	return append(out, "/etc/pkcs11/modules", "/usr/share/p11-kit/modules")
}

// libraryDirs are the multiarch directories a module may be installed
// into, most specific first.
func libraryDirs() []string {
	return []string{
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib64",
		"/usr/lib",
		"/usr/local/lib",
	}
}

// registeredModulePaths reads p11-kit's registry.
//
// A .module file is a small key/value document whose `module:` line
// names either an absolute path or a bare filename to be found on the
// library path. Both are handled; a bare name is resolved against
// libraryDirs rather than left for dlopen, so that a candidate this
// program reports is a path a person can look at.
func registeredModulePaths() []Candidate {
	var out []Candidate
	for _, dir := range p11KitModuleDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".module") {
				names = append(names, e.Name())
			}
		}
		// Sorted, so that two machines with the same modules installed
		// produce the same order and a failure list is comparable.
		sort.Strings(names)
		for _, name := range names {
			path, vendor := readModuleFile(filepath.Join(dir, name))
			if path == "" {
				continue
			}
			if !filepath.IsAbs(path) {
				resolved := ""
				for _, ld := range libraryDirs() {
					if _, err := os.Stat(filepath.Join(ld, path)); err == nil {
						resolved = filepath.Join(ld, path)
						break
					}
				}
				if resolved == "" {
					continue
				}
				path = resolved
			}
			if vendor == "" {
				vendor = "registered with p11-kit (" + name + ")"
			}
			out = append(out, Candidate{Path: path, Vendor: vendor, Origin: OriginKnown})
		}
	}
	return out
}

// readModuleFile returns the module path and description a .module file
// names. It reads a file this program did not write and treats every
// line it does not understand as a line it does not understand.
func readModuleFile(path string) (modulePath, vendor string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(io.LimitReader(f, 64*1024))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "module":
			modulePath = strings.TrimSpace(value)
		case "description":
			vendor = strings.TrimSpace(value)
		}
	}
	return modulePath, vendor
}

// knownModulePaths is the registry, then the vendor paths this project
// knows by name.
//
// The registry first: a .module file was written by the package that
// installed the module, and is therefore better evidence than anything
// this project can guess. The named paths are what is left for a vendor
// that does not register itself, which F12 §10 says exists.
func knownModulePaths() []Candidate {
	out := registeredModulePaths()

	seen := map[string]bool{}
	for _, c := range out {
		seen[c.Path] = true
	}
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

	// SPEC §11.11: the only Serbian middleware with a Linux build.
	//
	// Measured off the vendor's own packages as Pošta distributes them
	// (SafeSign_4600_Linux.zip, 4.6.0.0-AET.000; D-359), not recalled:
	//
	//	ub2404 .deb    /usr/lib/libaetpkss.so   -> libaetpkss.so.3.9.33.1
	//	redhat10 .rpm  /usr/lib64/libaetpkss.so -> libaetpkss.so.3.9.33.1
	//
	// Plain /usr/lib on Ubuntu, not the multiarch directory, and **no
	// p11-kit .module file and no maintainer scripts** in either — so the
	// registry above never finds it, and this line is the only thing that
	// does. It needs libcrypto.so.3, libpcsclite.so.1 and
	// libgdbm_compat.so.4, and exports C_GetFunctionList.
	add("A.E.T. Europe (SafeSign)", "libaetpkss.so")
	add("A.E.T. Europe (SafeSign)", "libaetpkss.so.3")
	// OpenSC, which reaches a good many national cards and is what a
	// person is most likely to already have installed.
	add("OpenSC", "opensc-pkcs11.so")
	add("OpenSC", "pkcs11/opensc-pkcs11.so")
	return out
}

// Modules probes every candidate out of process and returns the ones
// that answered.
//
// It is the Windows function's shape and the Windows function's reason:
// the check that holds is that the module answers C_GetInfo, performed
// by a child that had to load it anyway, so that a module which takes
// the process down takes only the child (F12 §2, D-275).
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

// sinkFor is discover_windows.go's, and is duplicated here rather than
// shared because it is three lines and moving it would put a function
// with no platform content into a file with a platform name. If a third
// platform arrives it belongs in discover.go.
func sinkFor(stderr func(modulePath string) io.Writer, path string) io.Writer {
	if stderr == nil {
		return nil
	}
	return stderr(path)
}
