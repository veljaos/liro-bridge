//go:build windows

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/consent"
)

// captureLog swaps slog's default logger for one writing into a buffer,
// restoring the real one when the test ends.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// seedAuditLog writes n valid entries and returns the store's directory
// and the file they went into.
func seedAuditLog(t *testing.T, n int) (dir, file string) {
	t.Helper()
	dir = t.TempDir()
	store, err := audit.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for i := 0; i < n; i++ {
		if _, err := store.Append(audit.Entry{
			Timestamp:     time.Now().Add(time.Duration(i) * time.Minute),
			Thumbprint:    strings.Repeat("AB12", 10),
			Application:   consent.ApplicationLocal,
			DocumentCount: 1,
			Outcome:       audit.OutcomeApproved,
		}); err != nil {
			t.Fatalf("seeding entry %d: %v", i, err)
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected exactly one log file, got %v (%v)", files, err)
	}
	return dir, files[0]
}

// TestAFailedAuditAppendIsSaidOutLoud is the regression test for what
// FTEST §8 found by breaking the audit log deliberately.
//
// recordInteractiveAudit discarded both of its error paths outright — a
// bare `return` when the store could not be opened, and `_, _ =
// store.Append(...)` when the append itself failed. That made a real and
// reachable state completely silent: measured, a single unparseable line
// anywhere in the log makes every subsequent Append fail forever,
// because the chain's last entry cannot be read and so the next
// PrevHash cannot be computed. One truncated last line is exactly what a
// power cut leaves behind.
//
// From that moment the agent went on signing and went on not recording,
// with nothing in the log file, nothing on screen, and nothing in the
// exit code — while SPEC §6.7 makes the audit log the record of every
// signature.
//
// Refusing to append onto a chain that cannot be read is correct and is
// not changed here. Losing the fact in silence is what this fixes.
func TestAFailedAuditAppendIsSaidOutLoud(t *testing.T) {
	t.Run("a log whose last line was truncated", func(t *testing.T) {
		dir, file := seedAuditLog(t, 6)
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		// Cut the last line in half, as an interrupted write would.
		if err := os.WriteFile(file, raw[:len(raw)-30], 0o600); err != nil {
			t.Fatal(err)
		}

		store, storeErr := audit.NewStore(dir)
		if storeErr != nil {
			t.Fatalf("NewStore over a truncated log: %v", storeErr)
		}

		buf := captureLog(t)
		recordInteractiveAudit(store, nil, "AB12AB12", 3, audit.OutcomeApproved, nil, false, "B-T")

		got := buf.String()
		if !strings.Contains(got, "could not be appended") {
			t.Errorf("nothing was logged about the failed append.\nlog was: %q", got)
		}
		if !strings.Contains(got, "level=ERROR") {
			t.Errorf("the failure was not logged at error level.\nlog was: %q", got)
		}
		// SPEC §18.3: no file name, no personal name, no document
		// content ever reaches a log line. The outcome and the count are
		// what the entry itself would have carried.
		if !strings.Contains(got, "documents=3") || !strings.Contains(got, "outcome=approved") {
			t.Errorf("the log line does not say what was not recorded.\nlog was: %q", got)
		}
	})

	t.Run("a store that could not be opened at all", func(t *testing.T) {
		buf := captureLog(t)
		recordInteractiveAudit(nil, context.DeadlineExceeded, "AB12AB12", 7, audit.OutcomeFailed, nil, false, "B-B")

		got := buf.String()
		if !strings.Contains(got, "could not be opened") {
			t.Errorf("nothing was logged about the unopenable store.\nlog was: %q", got)
		}
		if !strings.Contains(got, "documents=7") {
			t.Errorf("the log line does not say how many documents went unrecorded.\nlog was: %q", got)
		}
	})

	t.Run("a healthy log logs nothing and records the entry", func(t *testing.T) {
		dir, _ := seedAuditLog(t, 2)
		store, err := audit.NewStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		buf := captureLog(t)
		recordInteractiveAudit(store, nil, "AB12AB12", 4, audit.OutcomeApproved, nil, false, "B-LT")
		if strings.Contains(buf.String(), "level=ERROR") {
			t.Errorf("a successful append logged an error: %q", buf.String())
		}
		entries, err := store.All()
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(entries) != 3 {
			t.Fatalf("log holds %d entries, want 3", len(entries))
		}
		if entries[2].DocumentCount != 4 {
			t.Errorf("the appended entry says %d documents, want 4", entries[2].DocumentCount)
		}
		if r, err := store.Verify(); err != nil || !r.OK {
			t.Errorf("the chain no longer verifies: OK=%t BrokenAt=%d err=%v", r.OK, r.BrokenAt, err)
		}
	})
}

// TestATamperedAuditLogIsStillReadableAndSaysWhereItBroke pins what the
// export and the audit window depend on: a chain broken at a known entry
// is detected at exactly that entry, and every entry is still readable,
// so the log can be exported and looked at rather than being lost.
func TestATamperedAuditLogIsStillReadableAndSaysWhereItBroke(t *testing.T) {
	dir, file := seedAuditLog(t, 20)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 20 {
		t.Fatalf("log holds %d lines, want 20", len(lines))
	}
	// Alter one field of entry 13 (index 12), leaving it valid JSON —
	// which is what a tamperer would do and what the hash chain exists
	// to catch.
	lines[12] = strings.Replace(lines[12], `"documentCount":1`, `"documentCount":999`, 1)
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := audit.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := store.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if r.OK {
		t.Fatal("Verify says the chain is intact after entry 13 was altered")
	}
	if r.BrokenAt != 12 {
		t.Errorf("BrokenAt = %d, want 12 (the altered entry, zero-based)", r.BrokenAt)
	}
	entries, err := store.All()
	if err != nil {
		t.Fatalf("All over a tampered log: %v — a broken chain must still be readable", err)
	}
	if len(entries) != 20 {
		t.Errorf("All returned %d entries, want all 20", len(entries))
	}

	// And the export still writes both files, reporting the break rather
	// than refusing to produce evidence (D-135).
	out := t.TempDir()
	rep, err := store.Export(filepath.Join(out, "entries.jsonl"), filepath.Join(out, "report.json"))
	if err == nil {
		t.Error("Export reported no error for a broken chain")
	}
	if rep.EntryCount != 20 {
		t.Errorf("Export reported %d entries, want 20", rep.EntryCount)
	}
	for _, name := range []string{"entries.jsonl", "report.json"} {
		if st, err := os.Stat(filepath.Join(out, name)); err != nil || st.Size() == 0 {
			t.Errorf("%s was not written (%v)", name, err)
		}
	}
}
