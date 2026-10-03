package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
)

// TestTheDocumentListShowsNoSize is D29 (the owner): "Broj dokumenata: 1 ·
// 427 B" and "427 B" beside the name told a person signing nothing they
// could act on and took room from the name. Neither the count line nor a
// row carries a size any more, in any of the three catalogues.
func TestTheDocumentListShowsNoSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ugovor.pdf")
	if err := os.WriteFile(path, make([]byte, 427), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		m := newMainWindow(config.Default(), locale)
		m.queue.Add([]string{path})

		raw, err := json.Marshal(m.filesPayload("files"))
		if err != nil {
			t.Fatal(err)
		}
		payload := string(raw)
		for _, gone := range []string{"427 B", "sizeText", "·"} {
			if strings.Contains(payload, gone) {
				t.Errorf("%s: the document step's payload still carries %q: %s", locale, gone, payload)
			}
		}
		if got := m.countText(); !strings.HasSuffix(got, ": 1") {
			t.Errorf("%s: the count line is %q, want the count alone", locale, got)
		}
	}

	js, err := os.ReadFile("../../internal/ui/assets/pages/main.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(js), "sizeText") || strings.Contains(string(js), "file-size") {
		t.Error("main.js still renders a size beside each document")
	}
}
