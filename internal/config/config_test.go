package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("Load(missing) = %+v, want %+v", cfg, Default())
	}
}

func TestLoadPartialJSONFillsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"locale":"en"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Default()
	want.Locale = "en"
	if cfg != want {
		t.Fatalf("Load(partial) = %+v, want %+v", cfg, want)
	}
}

func TestLoadMalformedJSONReturnsDefaultsAndError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{not valid json`)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err == nil {
		t.Fatalf("Load(malformed) returned nil error, want non-nil")
	}
	if cfg != Default() {
		t.Fatalf("Load(malformed) = %+v, want defaults", cfg)
	}

	onDisk, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(onDisk) != string(original) {
		t.Fatalf("Load(malformed) modified the file on disk")
	}
}

func TestLoadBareSrIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"locale":"sr"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Locale != "sr-Latn" {
		t.Fatalf("Locale = %q, want sr-Latn (bare \"sr\" must never resolve to Cyrillic)", cfg.Locale)
	}
}

func TestLoadOutOfRangePortIsReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"portRangeStart":80,"portRangeEnd":99999}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PortRangeStart != defaultPortRangeStart {
		t.Fatalf("PortRangeStart = %d, want default %d", cfg.PortRangeStart, defaultPortRangeStart)
	}
	if cfg.PortRangeEnd != defaultPortRangeEnd {
		t.Fatalf("PortRangeEnd = %d, want default %d", cfg.PortRangeEnd, defaultPortRangeEnd)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Config{
		Locale:         "sr-Cyrl",
		LogLevel:       "debug",
		PortRangeStart: 18000,
		PortRangeEnd:   18010,
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("round-trip = %+v, want %+v", got, want)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Fatalf("leftover temp file after Save: %s", e.Name())
		}
	}
}

func TestInvalidLogLevelIsReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"logLevel":"verbose"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Fatalf("LogLevel = %q, want default %q", cfg.LogLevel, defaultLogLevel)
	}
}
