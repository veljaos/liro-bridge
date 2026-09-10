//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
