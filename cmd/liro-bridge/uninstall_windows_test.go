//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// What an uninstall takes and what it leaves, checked against a
// directory holding both.
//
// F10 §3.3 is unconditional about the audit log: it is the record of
// what a person signed, it outlives the program that wrote it, and an
// uninstall does not remove it. This is that, plus the rest of what
// the notice promises stays.
func TestAnUninstallTakesOnlyWhatItCanMakeAgain(t *testing.T) {
	dir := t.TempDir()

	// Everything both lists name, as a file or a directory with a file
	// in it, so a name that should be removed recursively is exercised
	// as one rather than as an empty stub.
	for _, name := range append(append([]string{}, derivedState...), keptState...) {
		if filepath.Ext(name) == "" {
			if err := os.MkdirAll(filepath.Join(dir, name, "inside"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name, "inside", "a.txt"), []byte(name), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removed, failed := removeDerivedState(dir)
	if failed != 0 {
		t.Errorf("removeDerivedState reported %d failures", failed)
	}
	if removed != len(derivedState) {
		t.Errorf("removeDerivedState removed %d of %d", removed, len(derivedState))
	}

	for _, name := range derivedState {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s survived an uninstall; it is derived state and should have gone", name)
		}
	}
	for _, name := range keptState {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s did not survive an uninstall: %v", name, err)
		}
	}
	// And the directory itself: what is left in it is the point.
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("the agent's own directory was removed: %v", err)
	}
}

// A preview directory holds rendered pages of the documents somebody
// was about to sign (D-141). Whatever else an uninstall leaves behind,
// it is not those.
//
// Named by prefix rather than in full, because the name carries a
// random suffix — which is the reason it was in neither list when
// D-243 went looking, and the reason removeDerivedState has to read
// the directory rather than only consult a constant.
func TestAnUninstallTakesThePageImagesACrashLeftBehind(t *testing.T) {
	dir := t.TempDir()

	previews := []string{"preview-1672968169", "preview-42"}
	for _, name := range previews {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "page-1.png"), []byte("a page of somebody's contract"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Kept state alongside, so this proves the prefix match is a
	// prefix match and not "remove everything that is left".
	if err := os.MkdirAll(filepath.Join(dir, "audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if removed, failed := removeDerivedState(dir); removed != len(previews) || failed != 0 {
		t.Errorf("removeDerivedState removed %d and failed %d, want %d removed and none failed", removed, failed, len(previews))
	}
	for _, name := range previews {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s survived an uninstall, and it holds page images of somebody's documents", name)
		}
	}
	for _, name := range []string{"audit", "config.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s did not survive an uninstall: %v", name, err)
		}
	}
}

// The extracted icon is named for the size of the asset it came from,
// the way the extracted WebView2 loader is, so an uninstall has to
// match it the same way it matches a preview directory.
//
// This is a real defect measured before it was fixed: derivedState
// named "icon.ico", ensureTrayIconExtracted writes "icon-<len>.ico",
// and every uninstall left the file behind. The name is built here the
// way the extractor builds it rather than typed as a literal, so a
// change to that convention fails this test instead of silently
// reopening the hole.
func TestAnUninstallTakesTheExtractedIconWhateverItsSizeIsCalled(t *testing.T) {
	dir := t.TempDir()

	for _, size := range []int{13717, 1, 999999} {
		name := fmt.Sprintf("icon-%d.ico", size)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ico"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Something kept whose name also begins with a letter of that
	// prefix, so the match is a prefix match and not a substring one.
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if removed, failed := removeDerivedState(dir); removed != 3 || failed != 0 {
		t.Errorf("removeDerivedState removed %d and failed %d, want 3 removed and none failed", removed, failed)
	}
	for _, size := range []int{13717, 1, 999999} {
		name := fmt.Sprintf("icon-%d.ico", size)
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s survived an uninstall; the extracted icon is derived state", name)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("config.json did not survive an uninstall: %v", err)
	}
}

// The startup sweep takes preview directories and nothing else.
//
// Worth its own test because the sweep and the uninstall read the same
// kind of list and must not read the same list: the extracted icon is
// derived state an uninstall should take and a *starting* agent must
// not, since it is about to load it. Before the two were separated this
// passed only because the icon happens to be a file and the sweep
// happens to skip files.
func TestTheStartupSweepNeverTakesTheIconTheAgentIsAboutToLoad(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-365 * 24 * time.Hour)

	icon := filepath.Join(dir, "icon-13717.ico")
	if err := os.WriteFile(icon, []byte("ico"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(icon, old, old); err != nil {
		t.Fatal(err)
	}
	// A directory with the icon prefix too, so the sweep cannot pass
	// this merely by skipping files.
	iconDir := filepath.Join(dir, "icon-cache")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(iconDir, old, old); err != nil {
		t.Fatal(err)
	}

	if got := sweepStalePreviews(dir, time.Now()); got != 0 {
		t.Errorf("the startup sweep removed %d entries, and none of them was a preview", got)
	}
	for _, p := range []string{icon, iconDir} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("the startup sweep took %s, which an agent that is starting still needs: %v", filepath.Base(p), err)
		}
	}
}

// A preview directory nothing came back for is collected when the
// agent next starts, and one that is merely recent is not — because a
// second agent in this session may have a placement window open over
// it right now.
func TestAStalePreviewIsSweptAndARecentOneIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	cases := []struct {
		name string
		age  time.Duration
		want bool // swept
	}{
		{"preview-stale", stalePreviewAge + time.Hour, true},
		{"preview-justunder", stalePreviewAge - time.Hour, false},
		{"preview-fresh", 0, false},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "page-1.png"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		// The directory's own mtime, not the file's: that is what the
		// sweep reads, and on Windows writing a file inside a
		// directory updates the directory too.
		when := now.Add(-c.age)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	// Something kept, with an ancient timestamp, to prove the sweep is
	// about the prefix and not about age alone.
	auditDir := filepath.Join(dir, "audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ancient := now.Add(-10 * 365 * 24 * time.Hour)
	if err := os.Chtimes(auditDir, ancient, ancient); err != nil {
		t.Fatal(err)
	}

	if got, want := sweepStalePreviews(dir, now), 1; got != want {
		t.Errorf("sweepStalePreviews removed %d, want %d", got, want)
	}
	for _, c := range cases {
		_, err := os.Lstat(filepath.Join(dir, c.name))
		if c.want && err == nil {
			t.Errorf("%s is %v old and survived the sweep", c.name, c.age)
		}
		if !c.want && err != nil {
			t.Errorf("%s is only %v old and was swept: a window may still be serving images out of it", c.name, c.age)
		}
	}
	if _, err := os.Lstat(auditDir); err != nil {
		t.Errorf("the sweep removed the audit directory: %v", err)
	}
}

// The two lists say opposite things about the same directory, so a
// name in both would be a promise the behaviour cannot keep. Checked
// rather than trusted, because the lists are edited by hand and are
// the only thing the uninstall notice's sentence is true because of.
func TestNothingIsBothKeptAndRemoved(t *testing.T) {
	inDerived := map[string]bool{}
	for _, n := range derivedState {
		if inDerived[n] {
			t.Errorf("%q is listed twice in derivedState", n)
		}
		inDerived[n] = true
	}
	for _, n := range keptState {
		if inDerived[n] {
			t.Errorf("%q is in both derivedState and keptState", n)
		}
		// A prefix is a list entry too, and a prefix that reaches a
		// kept name is the same promise broken in a way the exact-name
		// check above cannot see.
		for _, p := range derivedStatePrefixes {
			if strings.HasPrefix(n, p) {
				t.Errorf("keptState names %q, which the derived prefix %q would remove", n, p)
			}
		}
	}
	// And the sweep's own prefix has to be one of them, or a directory
	// it collects at startup is one an uninstall would leave.
	found := false
	for _, p := range derivedStatePrefixes {
		if p == previewPrefix {
			found = true
		}
	}
	if !found {
		t.Errorf("the startup sweep takes %q and no uninstall prefix matches it", previewPrefix)
	}
}

// The audit log is named in keptState by exactly the name the store
// uses, so that "the audit log is kept" is a statement about the
// directory that actually holds it rather than about a string that
// happens to look like one.
func TestTheAuditLogIsWhatTheKeptListNames(t *testing.T) {
	dir := t.TempDir()
	auditDir := filepath.Join(dir, "audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// newAuditStore resolves the same directory from the same place
	// the uninstall does; if the two ever disagree this is what says
	// so.
	if got := filepath.Base(auditDir); !contains(keptState, got) {
		t.Fatalf("the audit directory is %q and keptState does not name it: %v", got, keptState)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
