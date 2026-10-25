package main

import (
	"os"
	"regexp"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// TestSettingsSendsEveryLabelItsPageNames is the check that would have
// caught D-478's slip on any machine: the page shows only the strings
// Go sends it, so a label added to settings.html and not to
// buildSettingsInit shows its key. TestSettingsWindowRendersLocalisedText
// sees the same thing, but only in WebView2, on Windows.
func TestSettingsSendsEveryLabelItsPageNames(t *testing.T) {
	html, err := os.ReadFile("../../internal/ui/assets/pages/settings.html")
	if err != nil {
		t.Fatal(err)
	}
	keys := regexp.MustCompile(`data-i18n="([^"]+)"`).FindAllSubmatch(html, -1)
	if len(keys) == 0 {
		t.Fatal("no data-i18n labels found in settings.html; this test reads nothing")
	}
	init := buildSettingsInit(i18n.Load("en"), config.Default(), nil, platform.SecretStoreDescription{})
	sent, _ := init["strings"].(map[string]string)
	for _, k := range keys {
		if _, ok := sent[string(k[1])]; !ok {
			t.Errorf("settings.html labels %q, and buildSettingsInit does not send it: the page would show the key", k[1])
		}
	}
}
