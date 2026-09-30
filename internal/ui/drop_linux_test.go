//go:build linux

package ui

import (
	"reflect"
	"testing"

	"github.com/veljaos/liro-bridge/internal/chooser"
)

// A drop is read as text/uri-list (D-410), so what reaches the window is
// what this parser and chooser.LocalPaths make of the text a file manager
// writes: one URI per line, CRLF, comments and blank lines ignored,
// percent-encoding undone, and anything that is not a file on this disk
// left out — a location the desktop can browse is not a path a caller can
// open, and an empty string must never reach one as a path. No display
// needed.
func TestADroppedURIListBecomesLocalPathsAndNothingElse(t *testing.T) {
	// The shape D-409 read from Files, extended with what RFC 2483 allows.
	text := "# dragged from Files\r\n" +
		"file:///home/someone/ugovor.pdf\r\n" +
		"\r\n" +
		"https://example.invalid/faktura.pdf\r\n" +
		"file:///tmp/%C4%8Cita%C4%8D%20kartica/ra%C4%8Dun.pdf\r\n" +
		"file://localhost/home/someone/folder\n" +
		"file://other-host/share/x.pdf\r\n" +
		"file://\r\n"
	uris := parseURIList(text)
	if len(uris) != 6 {
		t.Fatalf("parseURIList kept %d lines, want 6 (no comment, no blank): %q", len(uris), uris)
	}
	got, skipped := chooser.LocalPaths(uris)
	want := []string{"/home/someone/ugovor.pdf", "/tmp/Čitač kartica/račun.pdf", "/home/someone/folder"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
	if skipped != 3 {
		t.Errorf("skipped = %d, want 3 (https, another host, an empty file URI)", skipped)
	}
}

func TestAnEmptyDropIsNoPaths(t *testing.T) {
	for _, text := range []string{"", "\r\n", "# only a comment\r\n"} {
		if got, _ := chooser.LocalPaths(parseURIList(text)); len(got) != 0 {
			t.Errorf("%q gave %q, want nothing", text, got)
		}
	}
}
