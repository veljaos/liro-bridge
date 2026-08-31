package config

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactShortStringCollapsesEntirely(t *testing.T) {
	for _, s := range []string{"", "a", "abcd1234"} {
		got := Redact(s)
		if strings.Contains(got, s) && s != "" {
			t.Fatalf("Redact(%q) = %q, leaks the original", s, got)
		}
	}
}

func TestRedactLongStringKeepsAtMostFourEachSide(t *testing.T) {
	got := Redact("supersecretdevicetoken")
	want := "supe…oken"
	if got != want {
		t.Fatalf("Redact = %q, want %q", got, want)
	}
}

func TestRedactNeverReturnsInputVerbatim(t *testing.T) {
	secret := "0123456789abcdef"
	got := Redact(secret)
	if got == secret {
		t.Fatalf("Redact returned the secret unchanged")
	}
}

func TestSetupLoggingWritesJSONToFile(t *testing.T) {
	dir := t.TempDir()

	logger, closer, err := SetupLogging("info", dir, false)
	if err != nil {
		t.Fatalf("SetupLogging: %v", err)
	}
	defer closer.Close()

	logger.Info("startup", "version", "0.1.0-dev")

	b, err := os.ReadFile(filepath.Join(dir, "bridge.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1: %q", len(lines), string(b))
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v", err)
	}
	if decoded["msg"] != "startup" {
		t.Fatalf("msg = %v, want startup", decoded["msg"])
	}
}

func TestSetupLoggingRespectsLevel(t *testing.T) {
	dir := t.TempDir()

	logger, closer, err := SetupLogging("warn", dir, false)
	if err != nil {
		t.Fatalf("SetupLogging: %v", err)
	}
	defer closer.Close()

	logger.Info("should be filtered out")
	logger.Warn("should appear")

	b, err := os.ReadFile(filepath.Join(dir, "bridge.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines at warn level, want 1: %q", len(lines), string(b))
	}
}

func TestRotatingWriterRotatesBySize(t *testing.T) {
	dir := t.TempDir()

	// Small threshold so a handful of writes force multiple rotations.
	w, err := newRotatingWriter(dir, "bridge.log", 20, 3)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer w.Close()

	line := []byte("0123456789\n") // 11 bytes
	for i := 0; i < 10; i++ {
		if _, err := w.Write(line); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) > 3 {
		t.Fatalf("got %d log files, want at most 3 (maxFiles): %v", len(entries), namesOf(entries))
	}
	if len(entries) < 2 {
		t.Fatalf("expected rotation to have produced at least one backup file, got %v", namesOf(entries))
	}
}

func namesOf(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func TestSetupLoggingDebugAlsoWritesStderr(t *testing.T) {
	dir := t.TempDir()

	r, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = wPipe
	defer func() { os.Stderr = origStderr }()

	logger, closer, err := SetupLogging("info", dir, true)
	if err != nil {
		t.Fatalf("SetupLogging: %v", err)
	}
	logger.Info("debug line")
	closer.Close()
	wPipe.Close()

	scanner := bufio.NewScanner(r)
	found := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "debug line") {
			found = true
		}
	}
	os.Stderr = origStderr
	if !found {
		t.Fatalf("expected the debug line on stderr when debug=true")
	}
}
