//go:build linux

package pkcs11

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// F12 §10: "an older vendor module can want symbols from an OpenSSL the
// distribution no longer ships, and dlopen fails with an undefined symbol
// rather than anything meaningful. That is a Failure with a readable
// reason, not a crash."
//
// The modules here are compiled by the test, because the failure is a
// property of how a library was linked and nothing short of linking one
// produces the loader's real words. Each exports C_GetFunctionList, so
// what fails is the load and not the entry-point test F11 built for a
// different impostor. Each goes through Modules — the real out-of-process
// probe the agent uses — as a configured path, which is how a person with
// such a module would meet it.

const fixtureModuleC = `
typedef unsigned long CK_RV;
extern int NEEDED_FN(void);
CK_RV C_GetFunctionList(void **list) { (void)list; return (CK_RV)NEEDED_FN(); }
`

func needCC(t *testing.T) string {
	t.Helper()
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler here, so no module can be built to fail loading; the explanation is untested on this machine")
	}
	return cc
}

func build(t *testing.T, cc, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(cc, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cc %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// moduleWith builds a module whose C_GetFunctionList calls fn, linked with
// extra, and returns its path.
func moduleWith(t *testing.T, cc, dir, fn string, extra ...string) string {
	t.Helper()
	src := filepath.Join(dir, "module.c")
	if err := os.WriteFile(src, []byte(strings.ReplaceAll(fixtureModuleC, "NEEDED_FN", fn)), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"-shared", "-fPIC", "-o", "vendor-pkcs11.so", "module.c"}, extra...)
	build(t, cc, dir, args...)
	return filepath.Join(dir, "vendor-pkcs11.so")
}

func TestAModuleThatWillNotLoadIsAFailureThatSaysWhy(t *testing.T) {
	cc := needCC(t)

	for _, tc := range []struct {
		name string
		// make builds the module in dir and returns its path.
		make func(t *testing.T, dir string) string
		// want are the phrases a person reads; loader is what the
		// loader itself said, which must still be there.
		want   []string
		loader string
	}{
		{
			name: "it needs an OpenSSL this system does not ship",
			make: func(t *testing.T, dir string) string {
				// A stand-in for OpenSSL 1.1, named and versioned as the
				// real one is. Linked by path and never on the loader's
				// search path, exactly as on a machine that lacks it.
				if err := os.WriteFile(filepath.Join(dir, "old.c"), []byte("int SSL_library_init(void){return 1;}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				build(t, cc, dir, "-shared", "-fPIC", "-Wl,-soname,libcrypto.so.1.1", "-o", "libcrypto.so.1.1", "old.c")
				return moduleWith(t, cc, dir, "SSL_library_init", "./libcrypto.so.1.1")
			},
			want:   []string{"it needs libcrypto.so.1.1, which is not installed on this system", "older OpenSSL", "vendor's build"},
			loader: "libcrypto.so.1.1: cannot open shared object file",
		},
		{
			name: "it needs a function nothing provides",
			make: func(t *testing.T, dir string) string {
				return moduleWith(t, cc, dir, "liro_fixture_nobody_provides_this")
			},
			want:   []string{"it needs the function liro_fixture_nobody_provides_this"},
			loader: "undefined symbol: liro_fixture_nobody_provides_this",
		},
		{
			name: "it was built against a symbol version its library no longer has",
			make: func(t *testing.T, dir string) string {
				// Link against a library exporting LIRO_OLD_1.0, then
				// replace the library with a build of the same soname
				// that has no version nodes — a distribution's newer copy.
				if err := os.WriteFile(filepath.Join(dir, "v.c"), []byte("int old_fn(void){return 0;}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "v.map"), []byte("LIRO_OLD_1.0 { global: old_fn; local: *; };\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				build(t, cc, dir, "-shared", "-fPIC", "-Wl,-soname,libliroold.so.1", "-Wl,--version-script=v.map", "-o", "libliroold.so.1", "v.c")
				path := moduleWith(t, cc, dir, "old_fn", "./libliroold.so.1", "-Wl,-rpath,$ORIGIN")
				// The newer copy has versions of its own, just not that
				// one: the shape of a distribution's newer OpenSSL.
				if err := os.WriteFile(filepath.Join(dir, "v.map"), []byte("LIRO_NEW_2.0 { global: old_fn; local: *; };\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				build(t, cc, dir, "-shared", "-fPIC", "-Wl,-soname,libliroold.so.1", "-Wl,--version-script=v.map", "-o", "libliroold.so.1", "v.c")
				return path
			},
			want:   []string{"it was built against LIRO_OLD_1.0 of libliroold.so.1"},
			loader: "version `LIRO_OLD_1.0' not found",
		},
		{
			name: "it is not a shared library",
			make: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "vendor-pkcs11.so")
				if err := os.WriteFile(p, []byte("this is a text file with a library's name\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			want:   []string{"it is not a shared library"},
			loader: "",
		},
		{
			name: "there is no file at the configured path",
			make: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "libnot-here.so")
			},
			want:   []string{"there is no file at this path"},
			loader: "cannot open shared object file",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.make(t, t.TempDir())

			ok, failures := Modules(path, nil)
			for _, c := range ok {
				if c.Path == path {
					t.Fatalf("%s loaded; the fixture does not fail the way it is meant to", path)
				}
			}
			var f *Failure
			for i := range failures {
				if failures[i].Candidate.Path == path {
					f = &failures[i]
				}
			}
			if f == nil {
				t.Fatalf("no Failure names %s: the person would see nothing about their module", path)
			}
			if errors.Is(f.Err, errWorkerDied) || errors.Is(f.Err, errWorkerSilent) {
				t.Fatalf("the probe did not survive: %v — a load failure must be a reason, not a crash", f.Err)
			}
			msg := f.Err.Error()
			t.Logf("reason: %s", msg)
			for _, w := range tc.want {
				if !strings.Contains(msg, w) {
					t.Errorf("the reason does not say %q:\n%s", w, msg)
				}
			}
			if tc.loader != "" && !strings.Contains(msg, tc.loader) {
				t.Errorf("the loader's own words %q were lost:\n%s", tc.loader, msg)
			}
		})
	}
}

// TestTheExplanationDoesNotSendAPersonTheWrongWay covers the branches no
// fixture here can produce, with the loader's words as glibc prints them,
// and the one piece of advice that must not be given: a module missing the
// *current* OpenSSL is not an old module.
func TestTheExplanationDoesNotSendAPersonTheWrongWay(t *testing.T) {
	const m = "/opt/vendor/lib/pkcs11.so"
	for _, tc := range []struct {
		dlerr, want, mustNot string
	}{
		{m + ": wrong ELF class: ELFCLASS32", "it is a 32-bit library", ""},
		{"libcrypto.so.3: cannot open shared object file: No such file or directory", "it needs libcrypto.so.3", "older OpenSSL"},
		{"libssl.so.1.0.0: cannot open shared object file: No such file or directory", "older OpenSSL", ""},
		{"something the loader has never said before", "something the loader has never said before", "the system loader said"},
	} {
		got := explainLoadFailure(m, tc.dlerr)
		if !strings.Contains(got, tc.want) {
			t.Errorf("%q explained as %q, want it to say %q", tc.dlerr, got, tc.want)
		}
		if tc.mustNot != "" && strings.Contains(got, tc.mustNot) {
			t.Errorf("%q explained as %q, which must not say %q", tc.dlerr, got, tc.mustNot)
		}
	}
}

// TestTheLoaderKillingTheProbeIsStillAFailureThatSaysWhereToLook is the
// case measured while writing the one above (D-359): a module needing a
// symbol version from a library with no versions at all makes glibc's
// ld.so assert inside dlopen and end the process with status 127 —
//
//	Inconsistency detected by ld.so: dl-lookup.c: 106: check_match:
//	Assertion `version->filename == NULL || ...' failed!
//
// No in-process code survives that; the child is why the agent does. The
// reason is gone with the child, so the Failure says where to look.
func TestTheLoaderKillingTheProbeIsStillAFailureThatSaysWhereToLook(t *testing.T) {
	cc := needCC(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "v.c"), []byte("int old_fn(void){return 0;}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "v.map"), []byte("LIRO_OLD_1.0 { global: old_fn; local: *; };\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	build(t, cc, dir, "-shared", "-fPIC", "-Wl,-soname,libliroold.so.1", "-Wl,--version-script=v.map", "-o", "libliroold.so.1", "v.c")
	path := moduleWith(t, cc, dir, "old_fn", "./libliroold.so.1", "-Wl,-rpath,$ORIGIN")
	build(t, cc, dir, "-shared", "-fPIC", "-Wl,-soname,libliroold.so.1", "-o", "libliroold.so.1", "v.c")

	_, failures := Modules(path, nil)
	for _, f := range failures {
		if f.Candidate.Path != path {
			continue
		}
		msg := f.Err.Error()
		t.Logf("reason: %s", msg)
		if !errors.Is(f.Err, errWorkerDied) {
			// A newer glibc may report this instead of asserting; then the
			// case above covers it, and this one has nothing to show.
			t.Skipf("this glibc did not end the process: %s", msg)
		}
		if !strings.Contains(msg, "ldd "+path) {
			t.Errorf("the Failure gives a bare exit and no place to look:\n%s", msg)
		}
		return
	}
	t.Fatalf("no Failure names %s", path)
}
