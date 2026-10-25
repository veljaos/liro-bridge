//go:build darwin && cgo

package pkcs11

import (
	"errors"
	"testing"
)

// dyld's wordings, as recalled, read into the kinds; and a module that
// does not exist, dlopened for real on the runner, is LoadNoFile — the
// one case measured rather than recalled.
func TestDyldsWordsAreReadIntoTheKinds(t *testing.T) {
	for dlerr, want := range map[string]LoadFailureKind{
		"dlopen(/x.dylib, 0x0006): Library not loaded: @rpath/libcrypto.3.dylib":                                                       LoadMissingLibrary,
		"dlopen(/x.dylib, 0x0006): Symbol not found: _C_GetFunctionList":                                                               LoadMissingFunction,
		"dlopen(/x.dylib, 0x0006): tried: '/x.dylib' (mach-o file, but is an incompatible architecture (have 'x86_64', need 'arm64'))": LoadWrongClass,
		"dlopen(/x.dylib, 0x0006): tried: '/x.dylib' (not a mach-o file)":                                                              LoadNotALibrary,
		"dlopen(/x.dylib, 0x0006): tried: '/x.dylib' (no such file)":                                                                   LoadNoFile,
	} {
		e := classifyLoadFailure("/x.dylib", dlerr)
		if e == nil || e.Kind != want {
			t.Errorf("%q gave %+v, want kind %v", dlerr, e, want)
		}
	}
	_, err := openModule("/nonexistent/liro-test-module.dylib")
	if err == nil {
		t.Fatal("a module that does not exist opened")
	}
	t.Logf("dyld said: %v", err)
	// Asserted, not only logged: until session 33's review the comment
	// above claimed this case as measured while the test checked only
	// that some error came back.
	var le *LoadError
	if !errors.As(err, &le) || le.Kind != LoadNoFile {
		t.Fatalf("a module that does not exist was not read as LoadNoFile: %v", err)
	}
}
