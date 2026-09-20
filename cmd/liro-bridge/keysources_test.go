//go:build windows || softtoken

package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"runtime"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// TestAConfiguredModulesPathIsNotWrittenIntoTheAuditLog is where SPEC §6.7 is
// paid, and the test is here because the rule is easy to undo by accident:
// audit.Entry.Module is a string, rec.origin.module is a string, and assigning
// one to the other compiles and looks right.
//
// A module found at one of this project's known installation paths is recorded
// whole — those are under Program Files or System32 and carry no personal name
// by construction. A module found where a person put it themselves is recorded
// by file name only, because that path can be anywhere, including under their
// user profile, where it carries their name.
//
// # Every case below holds on every platform, and that is the point
//
// This file carries no OS suffix, so CI runs it on Linux under the softtoken
// tag and a developer runs it on Windows. It used to assert a Windows fact in
// both places: filepath.Base returns the whole of a Windows path on Linux,
// including the user profile directory, so the Linux run reported the exact
// leak §6.7 forbids. Adding a suffix would have stopped CI seeing it and left
// the defect for the platform F12 is heading for, so the *subject* lost its
// dependence on the host platform instead (fileNameOfAnyPath). Both runs now
// assert the same thing, and both mean it.
func TestAConfiguredModulesPathIsNotWrittenIntoTheAuditLog(t *testing.T) {
	t.Run("a known path is kept whole", func(t *testing.T) {
		o := signerOrigin{
			backend:    "pkcs11",
			module:     `C:\Program Files\MUP RS\Celik\netsetpkcs11_x64.dll`,
			configured: false,
		}
		if got := o.auditModule(); got != o.module {
			t.Errorf("auditModule() = %q, want the whole path %q", got, o.module)
		}
	})

	t.Run("a configured path is reduced to its file name", func(t *testing.T) {
		// Both platforms' shapes, each with a personal name in a directory,
		// because a configured path is written by a person on whichever machine
		// they are using and this rule has to hold for all of them. The Linux
		// rows are F12's own case: /usr/lib is a known path, and a person whose
		// issuer put a module somewhere else types the rest.
		for _, c := range []struct {
			what   string
			module string
			want   string
		}{
			{"windows, under a user profile", `C:\Users\Veljko\Desktop\vendor\netsetpkcs11_x64.dll`, "netsetpkcs11_x64.dll"},
			{"windows, typed with forward slashes", `C:/Users/Veljko/vendor/lib.dll`, "lib.dll"},
			{"windows, drive-relative with no separator", `C:netsetpkcs11_x64.dll`, "netsetpkcs11_x64.dll"},
			{"windows, a UNC share named after a person", `\\fileserver\Veljko\vendor\lib.dll`, "lib.dll"},
			{"linux, under a home directory", "/home/veljko/vendor/libaetpkss.so", "libaetpkss.so"},
			{"linux, a deeper path", "/opt/veljko/lib/pkcs11/libsofthsm2.so", "libsofthsm2.so"},
			{"already a bare file name", "netsetpkcs11_x64.dll", "netsetpkcs11_x64.dll"},
		} {
			t.Run(c.what, func(t *testing.T) {
				o := signerOrigin{backend: "pkcs11", module: c.module, configured: true}
				got := o.auditModule()
				if got != c.want {
					t.Errorf("auditModule() = %q, want %q (GOOS=%s)", got, c.want, runtime.GOOS)
				}
				// The property, asserted separately from the expected value, so
				// that a wrong want cannot make this pass. §6.7 is about what
				// survives, not about one string.
				if i := strings.IndexAny(got, `/\:`); i >= 0 {
					t.Errorf("auditModule() = %q, which still carries a directory separator at %d: "+
						"SPEC §6.7 says this log never contains personal names, and a configured "+
						"path can be under a user profile (GOOS=%s)", got, i, runtime.GOOS)
				}
				if strings.Contains(strings.ToLower(got), "veljko") {
					t.Errorf("auditModule() = %q, which carries a personal name (GOOS=%s)", got, runtime.GOOS)
				}
			})
		}
	})

	t.Run("a path that is only separators carries none back", func(t *testing.T) {
		// Unreachable in production — auditModule is called only after a module
		// at this path has been loaded and has opened a session, and none of
		// these is a file. Asserted anyway because the answer it gives is the
		// one worth having if it ever is reached: empty, which a reader of the
		// log sees as "no module", rather than a string with a separator in it.
		for _, module := range []string{`C:\`, "/", `\\`, "::"} {
			o := signerOrigin{backend: "pkcs11", module: module, configured: true}
			if got := o.auditModule(); strings.ContainsAny(got, `/\:`) {
				t.Errorf("auditModule() = %q for module %q, which carries a separator", got, module)
			}
		}
	})

	t.Run("no module means no module", func(t *testing.T) {
		if got := (signerOrigin{backend: "windows-cng"}).auditModule(); got != "" {
			t.Errorf("auditModule() = %q for a CNG session, want empty", got)
		}
		// Including the case where nothing was opened at all, which is what a
		// refusal records.
		if got := (signerOrigin{}).auditModule(); got != "" {
			t.Errorf("auditModule() = %q for the zero origin, want empty", got)
		}
	})
}

// auditPathRuleFunctions are the two declarations that must not reach for the
// host platform's idea of a path separator.
var auditPathRuleFunctions = []string{"auditModule", "fileNameOfAnyPath"}

// TestTheAuditPathRuleDoesNotDependOnTheHostPlatform is the guard against the
// one edit that would put the defect back, which is the obvious one:
// `filepath.Base(o.module)` compiles, reads correctly, is shorter, and is what
// was there before. Nothing about it looks wrong on the machine it was written
// on, because on Windows it gives the right answer for a path from either
// platform. It is only wrong where nobody was looking.
//
// A comment asking for it not to be done is not a guard — this project has
// recorded a note failing to prevent the thing it describes four times over
// (D-285, D-293) — so the rule is a check, on the property rather than on the
// spelling: neither function may call into path/filepath or path at all.
//
// It reads the file from disk rather than through the build, so it asks the
// same question in the GOOS=windows and GOOS=linux views and cannot answer
// differently in the one nobody runs it in (D-299).
func TestTheAuditPathRuleDoesNotDependOnTheHostPlatform(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "keysources.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing keysources.go: %v", err)
	}

	found, bad := platformPathCalls(fset, file, auditPathRuleFunctions)

	// The positive control comes first, because a walker that finds no
	// declarations reports a clean tree for the emptiest possible reason
	// (D-296's first question; D-307 found this exact guard shape with no
	// control after six months).
	if len(found) != len(auditPathRuleFunctions) {
		t.Fatalf("this check found %v in keysources.go, want all of %v — "+
			"a renamed or deleted function makes it pass by looking at nothing",
			found, auditPathRuleFunctions)
	}

	for _, b := range bad {
		t.Errorf("%s: SPEC §6.7's path rule must not depend on the host platform, "+
			"and this calls %s. Measured: filepath.Base returns a Windows path "+
			"unchanged on Linux, user profile directory included, which is the "+
			"personal name §6.7 forbids this log to carry. Use fileNameOfAnyPath.", b.where, b.call)
	}
}

// TestTheAuditPathRuleGuardWouldActuallyFire runs the same checker over a
// fixture that does the forbidden thing, in each of the spellings it could
// arrive in, so that the clean verdict above is evidence rather than a claim.
func TestTheAuditPathRuleGuardWouldActuallyFire(t *testing.T) {
	const fixture = `package main

import (
	"path"
	"path/filepath"
)

func auditModule(p string) string {
	return filepath.Base(p)
}

func fileNameOfAnyPath(p string) string {
	_, f := filepath.Split(p)
	return path.Base(f)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", fixture, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}

	found, bad := platformPathCalls(fset, file, auditPathRuleFunctions)
	if len(found) != len(auditPathRuleFunctions) {
		t.Fatalf("the checker found %v in the fixture, want all of %v", found, auditPathRuleFunctions)
	}
	// Three calls: filepath.Base, filepath.Split, path.Base.
	if len(bad) != 3 {
		t.Fatalf("the checker reported %d forbidden calls in the fixture, want 3: %v", len(bad), bad)
	}
	for _, want := range []string{"filepath.Base", "filepath.Split", "path.Base"} {
		if !containsCall(bad, want) {
			t.Errorf("the checker did not report %s in the fixture: %v", want, bad)
		}
	}
}

// pathCall is one forbidden call, with somewhere to read it.
type pathCall struct {
	where string
	call  string
}

// platformPathCalls returns which of names it found, and every call into path
// or path/filepath inside them.
func platformPathCalls(fset *token.FileSet, file *ast.File, names []string) (found []string, bad []pathCall) {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || !want[fn.Name.Name] {
			continue
		}
		found = append(found, fn.Name.Name)
		ast.Inspect(fn, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || (pkg.Name != "filepath" && pkg.Name != "path") {
				return true
			}
			bad = append(bad, pathCall{
				where: fn.Name.Name + " at " + fset.Position(sel.Pos()).String(),
				call:  pkg.Name + "." + sel.Sel.Name,
			})
			return true
		})
	}
	return found, bad
}

func containsCall(calls []pathCall, want string) bool {
	for _, c := range calls {
		if c.call == want {
			return true
		}
	}
	return false
}

// TestDropOriginPassesBothAnswersThrough. The headless commands write no audit
// entry, so they take the chooser with the origin dropped rather than a second
// copy of a three-backend fallback that would have to be kept in step.
func TestDropOriginPassesBothAnswersThrough(t *testing.T) {
	want := errors.New("the card is not in the reader")
	called := 0
	open := dropOrigin(func(context.Context, keysource.Thumbprint) (keysource.Session, signerOrigin, error) {
		called++
		return nil, signerOrigin{backend: "pkcs11"}, want
	})
	sess, err := open(context.Background(), "ABCD")
	if called != 1 {
		t.Errorf("the wrapped chooser was called %d times, want 1", called)
	}
	if sess != nil {
		t.Error("a session came back from a call that failed")
	}
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
