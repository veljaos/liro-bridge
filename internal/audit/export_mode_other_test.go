//go:build !windows

package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestExportedFilesAreReadableOnlyByTheirOwner is D35: both files of an
// export are 0600, as the store's own, whatever the umask. Ubuntu's
// umask 002 made the entries file 0664 while the report beside it was
// 0600 (D-425).
func TestExportedFilesAreReadableOnlyByTheirOwner(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Append(Entry{Timestamp: time.Now(), Thumbprint: "AA", Application: ApplicationLocalForTest, DocumentCount: 1, Outcome: OutcomeApproved}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	entriesPath := filepath.Join(dir, "export.jsonl")
	reportPath := filepath.Join(dir, "report.json")
	if _, err := s.Export(entriesPath, reportPath); err != nil {
		t.Fatalf("Export: %v", err)
	}
	for _, p := range []string{entriesPath, reportPath} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != 0o600 {
			t.Errorf("%s is %#o, want 0600", filepath.Base(p), got)
		}
	}
}
