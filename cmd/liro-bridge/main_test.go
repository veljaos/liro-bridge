package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// withIsolatedHome points LOCALAPPDATA/HOME/XDG_CONFIG_HOME at a temp
// directory so config and log writes in this test never touch the real
// user profile (F0 §10 trap).
func withIsolatedHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
}

func TestVersionFlag(t *testing.T) {
	withIsolatedHome(t)
	var out bytes.Buffer

	code := run([]string{"--version"}, &out)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.HasPrefix(out.String(), "liro-bridge dev (commit none, built unknown, ") {
		t.Fatalf("unexpected --version output: %q", out.String())
	}
}

func TestHelpFlag(t *testing.T) {
	withIsolatedHome(t)
	var out bytes.Buffer

	code := run([]string{"--help"}, &out)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Usage of liro-bridge") {
		t.Fatalf("expected usage text, got %q", out.String())
	}
}

func TestNoArgsPrintsUsageAndExitsZero(t *testing.T) {
	withIsolatedHome(t)
	var out bytes.Buffer

	code := run(nil, &out)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Usage of liro-bridge") {
		t.Fatalf("expected usage text, got %q", out.String())
	}
}

func TestStartupWritesOneLogLine(t *testing.T) {
	withIsolatedHome(t)
	var out bytes.Buffer

	if code := run([]string{"--version"}, &out); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	b, err := os.ReadFile(filepath.Join(platform.DefaultLogDir(), "bridge.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(b), "liro-bridge starting") {
		t.Fatalf("log file missing startup line: %q", string(b))
	}
}
