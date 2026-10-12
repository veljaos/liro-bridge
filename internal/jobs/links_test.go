package jobs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinksAreHandedOverOnceEachAndNeverReachTheDocumentInbox(t *testing.T) {
	dir := t.TempDir()
	box := NewInbox(dir)
	for _, l := range []string{"liro-bridge://sign?job=https%3A%2F%2Fa.example%2F1", "liro-bridge://sign?job=https%3A%2F%2Fa.example%2F2"} {
		if err := box.AppendLink(l); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := box.Count(); n != 0 {
		t.Fatalf("the document inbox holds %d entries after two links", n)
	}
	got, err := box.TakeLinks()
	if err != nil || len(got) != 2 {
		t.Fatalf("TakeLinks: %v %v", got, err)
	}
	again, _ := box.TakeLinks()
	if len(again) != 0 {
		t.Fatalf("a link was handed over twice: %v", again)
	}
	entries, _ := os.ReadDir(filepath.Dir(box.Path()))
	if len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}
