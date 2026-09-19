package pkcs11

import (
	"os"
	"testing"
)

// TestAConfiguredModuleIsNamedByWhatItSaysAboutItself is the half of discovery
// that only a real module can answer.
//
// Candidate.Vendor is this project's own name for a *known* path, taken from a
// table of installations measured off real machines. A configured path is the
// one case that table cannot cover — it is a person's own installation that
// nothing here anticipated — so Vendor is blank for exactly the candidate whose
// identity matters most when something goes wrong with it.
//
// Manufacturer and LibraryDescription fill that gap, and they cost nothing: the
// probe child loads the module and reads CK_INFO in order to decide whether it
// is a module at all. Until now the parent assigned that answer to _ and threw
// it away, so the only description of a configured module this program could
// ever have was discarded by the one call that had it.
//
// It opts in through LIRO_PKCS11_MODULE like every other real-module test in
// this package (D-038's pattern), and it is read-only: the child calls
// C_Initialize, C_GetInfo and C_Finalize and nothing else. **No card is needed
// and nothing here can spend a PIN attempt.**
//
//	LIRO_PKCS11_MODULE="C:\Windows\System32\aetpkss1.dll" \
//	go test -count=1 -run TestAConfiguredModuleIsNamedBy -v ./internal/keysource/pkcs11/
func TestAConfiguredModuleIsNamedByWhatItSaysAboutItself(t *testing.T) {
	path := os.Getenv("LIRO_PKCS11_MODULE")
	if path == "" {
		t.Skip("set LIRO_PKCS11_MODULE to a real PKCS#11 module to run this")
	}

	usable, failures := Modules(path)

	var found *Candidate
	for i := range usable {
		if usable[i].Path == path {
			found = &usable[i]
			break
		}
	}
	if found == nil {
		// Reported with the failures rather than as a bare "not found": if the
		// module did not load, the reason is the thing worth reading, and a
		// test that said only "absent" would send the reader back to the
		// machine to find out why.
		t.Fatalf("%s did not become a usable module.\nfailures: %v", path, failures)
	}

	if found.Origin != OriginConfigured {
		t.Errorf("origin is %v, want %v — this path was supplied as the configured one",
			found.Origin, OriginConfigured)
	}
	// The assertion. Either is enough to name the module to a person; both
	// empty is the state describeModule already refuses to call a module, so
	// an empty pair here means the parent dropped what the child sent rather
	// than that the module said nothing.
	if found.Manufacturer == "" && found.LibraryDescription == "" {
		t.Errorf("the module at %s reached the parent with no manufacturer and no "+
			"library description.\nThe probe refuses a module that answers with "+
			"neither, so this is the parent discarding what the child sent.", path)
	}

	// Reported rather than asserted against a fixed string: these are the
	// vendor's own words and this project does not get to say what they should
	// be. What a later reader needs is what this machine actually answered.
	t.Logf("%s\n  manufacturer       %q\n  libraryDescription %q\n  vendor (table)     %q",
		found.Path, found.Manufacturer, found.LibraryDescription, found.Vendor)
}
