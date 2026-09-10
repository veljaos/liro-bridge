//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
)

// What `sign --force` forces, pinned (F9b §3a).
//
// It answers one question in advance — "a file already exists where
// this document's signature would go: replace it, or write beside it?"
// — and it answers nothing else. In particular it can never reach the
// document being signed, which is what SPEC §18.10 ("no silent
// overwrite of a user's original file") and SPEC §12.11 ("the original
// is never silently overwritten") are about.
//
// That is a property of two things together rather than a check
// anywhere in the signing path: the output name is base + suffix + ext
// (jobs.OutputPathFor), and the suffix can never be empty, because
// config.Load replaces an empty one with "-signed"
// (TestEmptyOutputSuffixIsReplaced). So the output's base name is
// strictly longer than the input's and the two can never be the same
// file — including for a document whose own name already ends in the
// suffix, which is the case a person actually meets, on the second run
// over a folder.
func TestForceCanNeverOverwriteTheDocumentBeingSigned(t *testing.T) {
	inputs := []string{
		`C:\docs\ugovor.pdf`,
		`C:\docs\ugovor-signed.pdf`,
		`C:\docs\ugovor-signed-signed.pdf`,
		`C:\docs\-signed.pdf`,
		`C:\docs\UGOVOR.PDF`,
		`C:\docs\no-extension`,
	}

	// The output folder unset (beside the input) and set to the input's
	// own folder are the two arrangements that could collide.
	for _, dir := range []string{"", `C:\docs`} {
		cfg := config.Default()
		cfg.OutputFolder = dir
		for _, in := range inputs {
			out := outputPathIn(in, cfg.OutputFolder, cfg.OutputSuffix)
			if strings.EqualFold(filepath.Clean(out), filepath.Clean(in)) {
				t.Errorf("with output folder %q, signing %q writes to %q — --force would overwrite the input", dir, in, out)
			}
		}
	}
}

// TestAnEmptyOutputSuffixCannotReachTheSigningFlow is the other half of
// the property above, and it is checked here rather than trusted: the
// claim is not "nobody would configure an empty suffix" but "an empty
// suffix in config.json is not what the flow is handed".
func TestAnEmptyOutputSuffixCannotReachTheSigningFlow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"outputSuffix": ""}`), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OutputSuffix == "" {
		t.Fatal("an empty outputSuffix survived config.Load, so an output path could equal its input")
	}
	in := `C:\docs\ugovor.pdf`
	if out := outputPathIn(in, "", cfg.OutputSuffix); strings.EqualFold(out, in) {
		t.Fatalf("output %q equals input %q", out, in)
	}
}

// `open` takes no arguments (F9b §3b). It is the window; `sign --in` is
// a batch. Two commands, no overlap, and one way to say "these
// documents" rather than two that disagree about what a pattern means.
func TestOpenTakesNoArgumentsAndPointsAtSignInstead(t *testing.T) {
	withIsolatedHome(t)
	var out bytes.Buffer

	code := run([]string{"open", `C:\docs\ugovor.pdf`}, &out)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(out.String(), "sign --in") {
		t.Errorf("output does not point at the command that does take documents: %q", out.String())
	}
}
