//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// Save in Settings writes the configuration on Linux. The only tests of the
// save action were Windows' (settingspersist_windows_test.go), and on dev.7
// and dev.8 a person's Save left config.json untouched (open-items D18,
// D-372). This runs the handler itself, against a scratch home.
func TestSettingsSaveWritesTheConfigurationOnLinux(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))

	path := platform.DefaultConfigFile()
	saved := config.Default()
	saved.ExplorerMenuEnabled = true
	if err := config.Save(path, saved); err != nil {
		t.Fatal(err)
	}

	state := settingsFormState{
		Action:         "save",
		Locale:         "en",
		OutputSuffix:   "-potpisano",
		SignatureLevel: saved.SignatureLevel,
		ExplorerMenu:   false,
	}
	if !handleSettingsAction(nil, i18n.Load("en"), saved, nil, state) {
		t.Fatal("save did not report success")
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if got.OutputSuffix != "-potpisano" || got.Locale != "en" {
		t.Errorf("save did not write the form: suffix %q, locale %q", got.OutputSuffix, got.Locale)
	}
	if !got.ExplorerMenuEnabled {
		t.Error("save changed the Explorer-menu setting on Linux, where the row is not offered (D-366)")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "autostart")); err != nil && !os.IsNotExist(err) {
		t.Errorf("autostart directory: %v", err)
	}
}
