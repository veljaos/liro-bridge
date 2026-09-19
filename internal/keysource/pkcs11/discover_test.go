package pkcs11

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheConfiguredPathComesFirstAndSurvivesNotExisting(t *testing.T) {
	// A path a person typed and got slightly wrong has to reach Modules so it
	// can be reported by name. Filtering it out here would leave them with a
	// configuration that silently does nothing.
	got := Candidates(`Z:\nowhere\liro.dll`)
	if len(got) == 0 {
		t.Fatal("a configured path produced no candidate at all")
	}
	if got[0].Path != `Z:\nowhere\liro.dll` {
		t.Errorf("the first candidate is %q, want the configured path", got[0].Path)
	}
	if got[0].Origin != OriginConfigured {
		t.Errorf("the configured path is marked %v, want configured — a known path that "+
			"is absent is not a failure, and a configured one that is absent is",
			got[0].Origin)
	}
}

func TestAnEmptyConfigurationAddsNothing(t *testing.T) {
	for _, configured := range []string{"", "   ", "\t"} {
		for _, c := range Candidates(configured) {
			if c.Origin == OriginConfigured {
				t.Errorf("Candidates(%q) produced a configured candidate %q", configured, c.Path)
			}
		}
	}
}

func TestOnePathIsNeverTriedTwice(t *testing.T) {
	// Configuring the module discovery would have found anyway must not
	// produce two Sources over one file, which would show every certificate on
	// that card twice before deduplication ever ran.
	known := knownModulePaths()
	if len(known) == 0 {
		t.Skip("no known paths on this platform")
	}
	var existing string
	for _, k := range known {
		if fileExists(k.Path) {
			existing = k.Path
			break
		}
	}
	if existing == "" {
		t.Skip("none of the known paths exists on this machine")
	}

	// The same path, spelled differently: a different case and a redundant
	// element. Both must collapse to one candidate.
	for _, spelling := range []string{
		existing,
		strings.ToUpper(existing),
		filepath.Join(filepath.Dir(existing), ".", filepath.Base(existing)),
	} {
		count := 0
		for _, c := range Candidates(spelling) {
			if strings.EqualFold(filepath.Clean(c.Path), filepath.Clean(existing)) {
				count++
			}
		}
		if count != 1 {
			t.Errorf("configuring %q produced %d candidates for one file, want 1", spelling, count)
		}
	}
}

func TestADirectoryIsNotAModule(t *testing.T) {
	dir := t.TempDir()
	if fileExists(dir) {
		t.Error("a directory was reported as an existing file")
	}
	f := filepath.Join(dir, "x.dll")
	if err := os.WriteFile(f, []byte("not a dll"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !fileExists(f) {
		t.Error("a real file was not reported as existing")
	}
}

// TestAFileThatIsNotAModuleIsAFailureAndNotACrash is F11 §3's rule: say which
// path, say it did not load, carry on. It uses a file that is definitely not a
// module — this test's own source would do, but a temporary file is clearer.
func TestAFileThatIsNotAModuleIsAFailureAndNotACrash(t *testing.T) {
	dir := t.TempDir()
	notAModule := filepath.Join(dir, "definitely-not-a-module.dll")
	if err := os.WriteFile(notAModule, []byte("MZ but not really"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	usable, failures := Modules(notAModule, nil)
	for _, u := range usable {
		if u.Path == notAModule {
			t.Fatal("a text file was accepted as a PKCS#11 module")
		}
	}
	named := false
	for _, f := range failures {
		if f.Candidate.Path == notAModule {
			named = true
			if f.Err == nil {
				t.Error("the failure carries no reason")
			}
			if !strings.Contains(f.Error(), notAModule) {
				t.Errorf("the failure reads %q and does not name the path", f.Error())
			}
		}
	}
	if !named {
		t.Error("a file that is not a module produced no failure naming it; F11 §3 " +
			"requires saying which path did not load rather than passing over it")
	}
}

// TestSourcesAreOnePerUsableModule guards the shape the caller depends on:
// every usable module becomes exactly one Source, and a module that failed
// becomes none.
func TestSourcesAreOnePerUsableModule(t *testing.T) {
	sources, failures := Sources("", nil)
	usable, sameFailures := Modules("", nil)
	if len(sources) != len(usable) {
		t.Errorf("got %d sources for %d usable modules", len(sources), len(usable))
	}
	if len(failures) != len(sameFailures) {
		t.Errorf("got %d failures from Sources and %d from Modules", len(failures), len(sameFailures))
	}
	for i, s := range sources {
		if s.ModulePath() != usable[i].Path {
			t.Errorf("source %d speaks to %q, want %q", i, s.ModulePath(), usable[i].Path)
		}
		if s.Name() != "pkcs11" {
			t.Errorf("source %d is named %q", i, s.Name())
		}
	}
	for _, c := range usable {
		t.Logf("usable: %-26s %s", c.Vendor, c.Path)
	}
	for _, f := range failures {
		t.Logf("not a module: %s", f)
	}
}
