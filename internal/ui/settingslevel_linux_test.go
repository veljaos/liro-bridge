//go:build linux

package ui

import (
	"encoding/json"
	"strconv"
	"testing"
)

// The Settings page with each saved level, in WebKitGTK: the radio it
// checks, and the level the form hands back to be saved. The best
// available, the default since D-476, is the empty value: it must check
// its own radio and come back empty, not as B-LT, which is what the
// page did with a value it did not know.
func TestSettingsShowsAndCollectsTheBestAvailable(t *testing.T) {
	for _, tc := range []struct{ saved, checked string }{
		{"", "level-best"},
		{"b-lt", "level-blt"},
		{"b-t", "level-bt"},
		{"b-b", "level-bb"},
	} {
		w := openRealPage(t, "/pages/settings.html", 520, 880)
		if err := w.PostJSON(map[string]any{
			"type":    "init",
			"strings": map[string]string{},
			"model": map[string]any{
				"locale":         "sr-Latn",
				"signatureLevel": tc.saved,
				"pairings":       []any{},
				"cloudOrigins":   []any{},
			},
		}); err != nil {
			t.Fatalf("PostJSON: %v", err)
		}
		got, err := w.Eval("(document.querySelector('input[name=level]:checked') || {}).id || ''")
		if err != nil {
			t.Fatalf("saved %q: %v", tc.saved, err)
		}
		if got != strconv.Quote(tc.checked) {
			t.Errorf("saved %q: the checked radio is %s, want %q", tc.saved, got, tc.checked)
		}
		raw, err := w.Eval("window.__liroCollectState()")
		if err != nil {
			t.Fatalf("saved %q: collecting the form: %v", tc.saved, err)
		}
		var asString string
		if err := json.Unmarshal([]byte(raw), &asString); err != nil {
			t.Fatalf("saved %q: the form's state %s: %v", tc.saved, raw, err)
		}
		var state struct {
			SignatureLevel string `json:"signatureLevel"`
		}
		if err := json.Unmarshal([]byte(asString), &state); err != nil {
			t.Fatalf("saved %q: the form's state %s: %v", tc.saved, asString, err)
		}
		if state.SignatureLevel != tc.saved {
			t.Errorf("saved %q: the form hands back %q", tc.saved, state.SignatureLevel)
		}
		_ = w.Close()
	}
}
