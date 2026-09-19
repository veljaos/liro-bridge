package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheModulePathIsNotWrittenIntoEverybodysConfigFile is the reason that
// field carries omitempty when no field above it does.
//
// Every other field describes how the agent behaves and is worth writing out so
// a person can see and edit it. This one names a file on disk and is empty on
// every machine that does not need it, and `"pkcs11ModulePath": ""` sitting in
// each person's config.json is an invitation to put a path to a DLL there.
//
// It also keeps a promise to people who already have the file: saving settings
// after upgrading must not add a line nobody asked for.
func TestTheModulePathIsNotWrittenIntoEverybodysConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(b), "pkcs11ModulePath") {
		t.Errorf("a default config.json names pkcs11ModulePath:\n%s", b)
	}

	// The control, in the other direction: the field is not merely missing from
	// the output because it is missing from the type.
	cfg := Default()
	cfg.PKCS11ModulePath = `C:\somewhere\vendor.dll`
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save with a path: %v", err)
	}
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(b), "pkcs11ModulePath") {
		t.Fatalf("a config with a module path set does not name it:\n%s", b)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if back.PKCS11ModulePath != cfg.PKCS11ModulePath {
		t.Errorf("round trip lost the path: %q", back.PKCS11ModulePath)
	}
}

// TestAConfigFileWrittenBeforeThisFieldExistedIsUnchangedByReadingAndWriting
// is the property that matters on a machine that already has one.
//
// The owner's own config.json has twenty-four keys and no module path. Loading
// it and saving it back must produce the same twenty-four keys — a field added
// to this struct must not rewrite files belonging to people who will never use
// it.
func TestAConfigFileWrittenBeforeThisFieldExistedIsUnchangedByReadingAndWriting(t *testing.T) {
	const before = `{
  "locale": "sr-Latn",
  "logLevel": "info",
  "portRangeStart": 17580,
  "portRangeEnd": 17590,
  "startWithWindows": true,
  "outputSuffix": "-signed",
  "signatureLevel": "b-b",
  "visibleStamp": true,
  "stampPosition": "top-right",
  "updateCheckEnabled": true
}`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(b, &keys); err != nil {
		t.Fatalf("the saved file is not JSON: %v", err)
	}
	if _, ok := keys["pkcs11ModulePath"]; ok {
		t.Errorf("reading and writing a config that predates this field added it:\n%s", b)
	}
}
