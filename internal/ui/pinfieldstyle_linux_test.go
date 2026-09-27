//go:build linux

package ui

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const pinCSSChildEnv = "LIRO_TEST_PIN_CSS_CHILD"

// The field's focus outline is CSS, and a CSS mistake is only reported at
// load time: a typo would leave the field with no sign of focus and nothing
// would fail (D-385). So the stylesheet is loaded in a child process and the
// child's stderr must carry no "Theme parser error" — GTK's own warning.
//
// Not through the provider's parsing-error signal: connecting a Go handler to
// it through gotk4 v0.3.1 double-frees the GError when an error occurs
// ("free(): double free detected" inside gtk_css_provider_load_from_string —
// the marshaller takes ownership of an error GTK then frees again). The
// agent never connects that signal; only a test would have.
func TestThePINFieldStylesheetParses(t *testing.T) {
	if os.Getenv(pinCSSChildEnv) == "1" {
		if err := theUIThread.start(); err != nil {
			os.Exit(3)
		}
		_ = theUIThread.do(func() { gtk.NewCSSProvider().LoadFromString(pinFieldCSS) })
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestThePINFieldStylesheetParses$")
	cmd.Env = append(os.Environ(), pinCSSChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 3 {
		t.Skip("no display for GTK")
	}
	if err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "Theme parser error") {
			t.Errorf("pinFieldCSS does not parse: %s", line)
		}
	}
}
