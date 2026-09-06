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

		// F6's own additions, likewise deliberately not the defaults —
		// except StampPage, which has no valid "off" value: validate
		// replaces an empty one, so a round trip must carry a real page
		// selection or it is testing validate rather than the round
		// trip.
		StampPage:           "3",
		StampReference:      "Ugovor 2026/114",
		StampShowDocumentID: true,
		OutputFolder:        `D:\potpisano`,
		ExplorerMenuEnabled: false,
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

// TestValidStampPage covers F6 §6's page selection: two words and any
// positive page number, and nothing else. A zero or a negative is not a
// page, and guessing at what someone meant by "0" is how a stamp lands
// somewhere nobody asked for.
func TestValidStampPage(t *testing.T) {
	valid := []string{"first", "last", "1", "2", "17", "9999"}
	for _, s := range valid {
		if !ValidStampPage(s) {
			t.Errorf("ValidStampPage(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "0", "-1", "First", "LAST", "1.5", "one", "1a", " 1"}
	for _, s := range invalid {
		if ValidStampPage(s) {
			t.Errorf("ValidStampPage(%q) = true, want false", s)
		}
	}
}

func TestInvalidStampPageFallsBackToFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"stampPage":"middle"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.StampPage != "first" {
		t.Fatalf("StampPage = %q, want the default", got.StampPage)
	}
}

// TestNewSettingsDefaults pins the two defaults F6 states outright: the
// Explorer entry is on, and output goes beside the input.
func TestNewSettingsDefaults(t *testing.T) {
	d := Default()
	if !d.ExplorerMenuEnabled {
		t.Error("ExplorerMenuEnabled defaults off; F6 §2 says on by default")
	}
	if d.OutputFolder != "" {
		t.Errorf("OutputFolder = %q, want empty — F6 §4 defaults to the input's own folder", d.OutputFolder)
	}
	if d.StampPage != "first" {
		t.Errorf("StampPage = %q, want first", d.StampPage)
	}
	if d.StampShowDocumentID {
		t.Error("StampShowDocumentID defaults on; SPEC §13.5 says never the default")
	}
}

// TestLoadAcceptsAUTF8ByteOrderMark is FTEST's finding from §8's
// "configuration file corrupt" case, reached by accident while checking
// the three locales: writing config.json with PowerShell's
// `Set-Content -Encoding utf8` — the obvious way to script an edit on
// Windows — prepends EF BB BF, encoding/json then rejects the file at
// offset 1, and Load silently returns Default(). Every setting reverts:
// the language, the signature level, the remembered stamp position, the
// output folder. The only trace is a line in a log file.
//
// D-134 ran into this once and recorded it as "worth knowing on its
// own". It is a defect, not a curiosity: the file is documented as the
// single authority on the configuration (D-134), and a hand-edited copy
// of it must not be silently discarded for carrying transfer encoding
// the project already strips from the one other outside text file it
// reads (the Trusted List seed, D-018/D-107).
func TestLoadAcceptsAUTF8ByteOrderMark(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	body := `{"locale":"sr-Cyrl","signatureLevel":"b-t","outputSuffix":"-potpisan"}`
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, body...)
	if err := os.WriteFile(path, withBOM, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with a BOM: %v", err)
	}
	if cfg.Locale != "sr-Cyrl" {
		t.Errorf("Locale = %q, want sr-Cyrl — the file was discarded for its byte-order mark", cfg.Locale)
	}
	if cfg.SignatureLevel != "b-t" {
		t.Errorf("SignatureLevel = %q, want b-t", cfg.SignatureLevel)
	}
	if cfg.OutputSuffix != "-potpisan" {
		t.Errorf("OutputSuffix = %q, want -potpisan", cfg.OutputSuffix)
	}
}

// TestLoadStillRejectsGenuineRubbish keeps the other half true: a
// byte-order mark is stripped, and nothing else is. A file that is not
// JSON is still a file that is not JSON.
func TestLoadStillRejectsGenuineRubbish(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"plain text", []byte("this is not json at all\n")},
		{"UTF-16 little-endian", append([]byte{0xFF, 0xFE}, []byte("{\x00}\x00")...)},
		{"truncated", []byte(`{"locale":"sr-Cyr`)},
		{"a BOM and nothing else", []byte{0xEF, 0xBB, 0xBF}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, "c-"+tc.name+".json")
			if err := os.WriteFile(path, tc.body, 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err == nil {
				t.Fatalf("Load(%s) returned no error", tc.name)
			}
			if cfg.Locale != defaultLocale {
				t.Errorf("Locale = %q, want the default %q", cfg.Locale, defaultLocale)
			}
		})
	}
}
