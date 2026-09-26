package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// TestEveryLoadFailureReachesAPersonInTheirLanguage is D-362: a module that
// would not load is told in Serbian in a Serbian report, through the real
// mapping, for every kind there is — so a kind added later without a
// sentence fails here rather than switching language at the worst moment.
// The loader's own words stay beside it, verbatim.
func TestEveryLoadFailureReachesAPersonInTheirLanguage(t *testing.T) {
	const loader = "libcrypto.so.1.1: cannot open shared object file: No such file or directory"
	for _, kind := range pkcs11.LoadFailureKinds() {
		le := &pkcs11.LoadError{Kind: kind, Module: "/opt/v/p.so", Name: "X_NAME", Library: "libx.so.1", OldOpenSSL: true}
		if kind != pkcs11.LoadLoaderStopped {
			le.Loader = loader
		}
		f := asModuleFailure(pkcs11.Failure{
			Candidate: pkcs11.Candidate{Path: "/opt/v/p.so"},
			Err:       fmt.Errorf("pkcs11: /opt/v/p.so could not be loaded: %w", le),
		})
		if f.Load == nil {
			t.Fatalf("%s: the LoadError did not reach the report", kind)
		}
		for _, locale := range []string{"sr-Latn", "sr-Cyrl"} {
			var b strings.Builder
			cli.RenderText(&b, cli.Report{ModuleFailures: []cli.ModuleFailure{f}}, i18n.Load(locale), time.Now(), false)
			out := b.String()
			for _, english := range []string{"which is not installed", "no library on this system", "was built against",
				"not a shared library", "no file at this path", "32-bit library", "system loader stopping", "older OpenSSL"} {
				if strings.Contains(out, english) {
					t.Errorf("%s in %s still reads %q:\n%s", kind, locale, english, out)
				}
			}
			if strings.Contains(out, "certs.module_load") {
				t.Errorf("%s in %s shows a message key, not a sentence:\n%s", kind, locale, out)
			}
			if le.Loader != "" && !strings.Contains(out, loader) {
				t.Errorf("%s in %s lost the loader's own words:\n%s", kind, locale, out)
			}
		}
	}
}
