package pkcs11

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// knownModulePaths is where each issuer's own installer actually puts its
// module, read off a real machine rather than recalled (F11 §3, measured
// 2026-09-14 and confirmed 2026-09-15):
//
//	A.E.T. Europe (SafeSign)  aetpkss1.dll         3.9.24.1  2024-12-13
//	NetSeT (TrustEdgeID)      netsetpkcs11_x64.dll 1.1.3.3   2024-12-12
//	NetSeT (MUP RS\Celik)     netsetpkcs11_x64.dll 1.1.0.0   2019-03-27
//	Nexus Personal            personal64.dll       5.17.0    2025-03-19
//
// # Two things this list is shaped by
//
// NetSeT ships at two paths in two builds five years apart. That is not one
// file in two places, which would be the easy case — they are different builds
// of one vendor's module, and whichever one a given person's machine prefers,
// the other one is somebody else's machine. Both are here, and neither is
// treated as the canonical one.
//
// The directories are built from the environment rather than written out as
// C:\Program Files, because that is not where they are on every Windows: a
// machine can have ProgramFiles somewhere else, and a non-English install
// certainly does. The literal paths above are what they resolve to here.
//
// # What is deliberately not in the list
//
// C:\Program Files\SecurityTray\lib\pkcs11wrapper_64.dll. It has "pkcs11" in
// its name, it is on this machine, and it is IAIK's Java JNI wrapper — which
// consumes PKCS#11 modules rather than being one, and exports no
// C_GetFunctionList. It is the reason Modules tests for the entry point rather
// than for a promising file name, and leaving it out of the list is not what
// protects against it; the entry-point test is.
func knownModulePaths() []Candidate {
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}

	var out []Candidate
	add := func(vendor string, parts ...string) {
		if parts[0] == "" {
			return // the environment does not define that directory
		}
		out = append(out, Candidate{Path: filepath.Join(parts...), Vendor: vendor, Origin: OriginKnown})
	}

	add("A.E.T. Europe (SafeSign)", systemRoot, "System32", "aetpkss1.dll")
	add("NetSeT (TrustEdgeID)", programFiles, "TrustEdgeID", "netsetpkcs11_x64.dll")
	add("NetSeT (MUP RS)", programFiles, "MUP RS", "Celik", "netsetpkcs11_x64.dll")
	add("Nexus Personal", programFilesX86, "Personal", "bin64", "personal64.dll")
	return out
}

// Modules loads each candidate and returns the ones that are really PKCS#11
// modules, alongside every candidate that was not, with its reason.
//
// A file is a module if it exports C_GetFunctionList and answers C_GetInfo
// with something — never because its name looks promising. That is not a
// hypothetical distinction: this project's development machine carries
// C:\Program Files\SecurityTray\lib\pkcs11wrapper_64.dll, which is IAIK's
// Java JNI wrapper. It has "pkcs11" in its name, it sits in a directory a
// reasonable search would look in, and it *consumes* PKCS#11 modules rather
// than being one — it exports no C_GetFunctionList at all.
//
// Nothing here is fatal. A module that will not load is a Failure in the
// second return value and the search carries on, because a person with three
// middlewares installed and one of them broken should still be able to sign
// with the other two (F11 §3).
//
// # Probing is not free and not silent
//
// Every candidate is loaded, which runs its DllMain — in a child, but the cost
// is still paid. Measured on this machine: loading Nexus's personal64.dll
// writes a line of its own to stderr —
//
//	Personal::config::file::read: Personal config file '...Personal.cfg' does not exist
//
// — which is a foreign library talking to a console this program did not open
// for it. The child inherits this process's standard error so that it lands
// where every other diagnostic does rather than being swallowed. Nothing here
// can stop it being written, and a caller probing on a schedule rather than
// once would be paying for it, and for a process spawn, repeatedly. Probe when
// a listing is actually wanted.
//
// # Every candidate is loaded in a child process, and that is the whole point
//
// D-272 measured NetSeT 1.1.0.0 — the build MUP's own middleware installs —
// dying inside its own C_Initialize about once in a hundred calls, in two
// different ways, and established by direct measurement that no Go process
// survives either: recover() catches neither, and a vectored handler does not
// help. There is no in-process remedy, so the load is not in this process.
//
// A child that dies is a Failure with its path attached and the search carries
// on, which is what F11 §3 asks for and what the crash made impossible while
// the load was here (D-275).
func Modules(configured string, stderr func(modulePath string) io.Writer) ([]Candidate, []Failure) {
	var ok []Candidate
	var bad []Failure
	for _, c := range Candidates(configured) {
		// Answering C_GetInfo with recognisable strings is the check that
		// actually holds, and the child is what performs it. Comparing the
		// function list's fourth entry against the exported C_GetFunctionList
		// does not: SafeSign's export is a jmp rel32 thunk and does not match,
		// where three other modules do.
		// The result is kept rather than discarded. The child loaded the
		// module and read CK_INFO to answer at all, so what it says about
		// itself is already paid for; assigning it to _ threw away the only
		// description of a configured module this program will ever have,
		// because the next thing to ask would have to load it again.
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
