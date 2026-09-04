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
		Locale:             "sr-Cyrl",
		LogLevel:           "debug",
		PortRangeStart:     18000,
		PortRangeEnd:       18010,
		StartWithWindows:   false,
		TSAURL:             "https://tsa.example.rs",
		OutputSuffix:       "-signed",
		SignatureLevel:     "b-t",
		UpdateCheckEnabled: false,
		// Task 1 (F5 fourth-real-run review): the stamp choice is
		// persisted like any other setting, so the round trip has to
		// carry it. Both values are deliberately not the defaults.
		VisibleStamp:  false,
		StampPosition: "top-left",
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

func TestInvalidSignatureLevelIsReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"signatureLevel":"b-lta"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SignatureLevel != defaultSignatureLevel {
		t.Fatalf("SignatureLevel = %q, want default %q", cfg.SignatureLevel, defaultSignatureLevel)
	}
}

func TestEmptyOutputSuffixIsReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"outputSuffix":""}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OutputSuffix != defaultOutputSuffix {
		t.Fatalf("OutputSuffix = %q, want default %q", cfg.OutputSuffix, defaultOutputSuffix)
	}
}

// TestDefaultVisibleStampIsOn is Task 1's default, pinned where it is
// decided (D-103): a signature the signer cannot see reads as one that
// was never applied, so the visible stamp is on out of the box and the
// invisible signature is the deliberate choice.
func TestDefaultVisibleStampIsOn(t *testing.T) {
	cfg := Default()
	if !cfg.VisibleStamp {
		t.Error("Default().VisibleStamp = false, want true")
	}
	if cfg.StampPosition != "bottom-right" {
		t.Errorf("Default().StampPosition = %q, want bottom-right (SPEC §13.1)", cfg.StampPosition)
	}
}

// TestVisibleStampFalseSurvivesLoad guards the one thing a bool default
// of true can get wrong: a user who switched the stamp off must not
// have it switched back on by the defaults being applied over their
// file.
func TestVisibleStampFalseSurvivesLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"visibleStamp": false}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.VisibleStamp {
		t.Error("VisibleStamp = true after loading a file that says false")
	}
}

// TestSignatureLevelBBIsValid is Task 3: a user who has deliberately
// decided against a timestamp has a level to choose, and it is not
// replaced by the default on the next load.
func TestSignatureLevelBBIsValid(t *testing.T) {
	for _, level := range []string{"b-b", "b-t", "b-lt"} {
		cfg := Config{SignatureLevel: level}
		validate(&cfg)
		if cfg.SignatureLevel != level {
			t.Errorf("validate replaced signatureLevel %q with %q", level, cfg.SignatureLevel)
		}
	}
}

func TestInvalidStampPositionFallsBackToTheDefaultCorner(t *testing.T) {
	for _, position := range []string{"", "middle", "BOTTOM-RIGHT", "centre"} {
		cfg := Config{StampPosition: position}
		validate(&cfg)
		if cfg.StampPosition != defaultStampPosition {
			t.Errorf("validate(%q) left %q, want %q", position, cfg.StampPosition, defaultStampPosition)
		}
	}
	for _, position := range []string{"bottom-right", "bottom-left", "top-right", "top-left"} {
		cfg := Config{StampPosition: position}
		validate(&cfg)
		if cfg.StampPosition != position {
			t.Errorf("validate replaced valid position %q with %q", position, cfg.StampPosition)
		}
	}
}
