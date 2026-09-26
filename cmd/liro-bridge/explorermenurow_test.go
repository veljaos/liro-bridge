package main

import (
	"runtime"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// TestTheExplorerMenuRowIsOfferedOnlyWhereThereIsAnExplorer is open item D5:
// on Linux the row switched nothing, because F12 §8 refuses a context-menu
// verb there. The page hides it on the model's word, and Save keeps the saved
// value rather than reading a checkbox nobody could see.
func TestTheExplorerMenuRowIsOfferedOnlyWhereThereIsAnExplorer(t *testing.T) {
	for _, tc := range []struct {
		goos    string
		offered bool
	}{{"windows", true}, {"linux", false}} {
		if got := explorerMenuOffered(tc.goos); got != tc.offered {
			t.Errorf("%s: offered = %v, want %v", tc.goos, got, tc.offered)
		}
		for _, saved := range []bool{true, false} {
			got := explorerMenuSetting(tc.goos, saved, !saved)
			want := !saved // the form's answer
			if !tc.offered {
				want = saved
			}
			if got != want {
				t.Errorf("%s: saved %v, form %v → wrote %v, want %v", tc.goos, saved, !saved, got, want)
			}
		}
	}

	init := buildSettingsInit(i18n.Load("en"), config.Config{ExplorerMenuEnabled: true}, nil, platform.SecretStoreDescription{})
	model, _ := init["model"].(map[string]any)
	if got, ok := model["explorerMenuOffered"].(bool); !ok || got != (runtime.GOOS == "windows") {
		t.Errorf("the page is told explorerMenuOffered = %v (present: %v) on %s", model["explorerMenuOffered"], ok, runtime.GOOS)
	}
}
