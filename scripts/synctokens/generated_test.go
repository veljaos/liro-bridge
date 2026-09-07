package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The committed CSS must be exactly what this generator produces.
//
// It was not, and nothing said so. The step header the one-window
// signing flow added — .liro-steps and its four companions — was
// written straight into internal/ui/assets/intents.css and never into
// this generator, so the first person to run `go run ./scripts/synctokens`
// would have deleted fifty-two lines of live CSS and taken the step
// indicator off every screen of the flow with it. Nothing ran the
// generator between the two, which is the only reason it did not
// happen.
//
// The generated files' own header says "Do not edit by hand", which is
// the rule; this is the check that the rule was followed. A generated
// artefact nobody regenerates is a generated artefact in name only.
func TestGeneratedFilesMatchWhatIsCommitted(t *testing.T) {
	root := repoRoot(t)
	for _, tc := range []struct {
		name string
		want string
	}{
		{"tokens.css", tokensCSS},
		{"intents.css", intentsCSS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "internal", "ui", "assets", tc.name)
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			if string(got) != tc.want {
				t.Fatalf("%s on disk differs from what this generator produces.\n"+
					"Either the file was edited by hand — its own header says not to — "+
					"or a change to this generator was never written out.\n"+
					"Run: go run ./scripts/synctokens\n"+
					"committed: %d bytes, generated: %d bytes", tc.name, len(got), len(tc.want))
			}
		})
	}
}

// repoRoot walks up from the test's working directory until it finds
// go.mod, so this test does not depend on where `go test` is run from.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the working directory")
		}
		dir = parent
	}
}
