//go:build windows

package ui

import (
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// buildBuffer lays strings out the way GetOpenFileNameW does: each
// NUL-terminated, the list closed by an empty one.
func buildBuffer(parts ...string) []uint16 {
	var b []uint16
	for _, p := range parts {
		b = append(b, utf16.Encode([]rune(p))...)
		b = append(b, 0)
	}
	return append(b, 0)
}

// TestParseMultiSelect covers both shapes OFN_EXPLORER produces. The
// single-file shape is a whole path; the multi-file shape is a
// directory followed by bare names, which is the one that breaks if
// treated as absolute paths.
func TestParseMultiSelect(t *testing.T) {
	cases := []struct {
		name string
		buf  []uint16
		want []string
	}{
		{
			name: "one file is a whole path",
			buf:  buildBuffer(`C:\docs\ugovor.pdf`),
			want: []string{`C:\docs\ugovor.pdf`},
		},
		{
			name: "several files are a directory plus names",
			buf:  buildBuffer(`C:\docs`, "a.pdf", "b.pdf"),
			want: []string{filepath.Join(`C:\docs`, "a.pdf"), filepath.Join(`C:\docs`, "b.pdf")},
		},
		{
			name: "names with spaces survive",
			buf:  buildBuffer(`C:\docs`, "Ugovor o radu.pdf", "Račun 2026.pdf"),
			want: []string{
				filepath.Join(`C:\docs`, "Ugovor o radu.pdf"),
				filepath.Join(`C:\docs`, "Račun 2026.pdf"),
			},
		},
		{
			name: "an absolute name in the multi-select form is not joined twice",
			buf:  buildBuffer(`C:\docs`, `D:\elsewhere\c.pdf`),
			want: []string{`D:\elsewhere\c.pdf`},
		},
		{
			name: "an empty buffer yields nothing",
			buf:  buildBuffer(),
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMultiSelect(tc.buf)
			if len(got) != len(tc.want) {
				t.Fatalf("parseMultiSelect = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parseMultiSelect = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestUTF16FilterIsDoublyTerminated pins the format GetOpenFileNameW
// requires: alternating NUL-terminated strings, the whole list closed
// by one more NUL. Getting this wrong shows the dialog with no file
// types at all.
func TestUTF16FilterIsDoublyTerminated(t *testing.T) {
	f := utf16Filter("PDF dokumenti", "*.pdf", "Sve datoteke", "*.*")
	if f[len(f)-1] != 0 || f[len(f)-2] != 0 {
		t.Fatal("filter is not doubly NUL-terminated")
	}
	got := splitUTF16Strings(f)
	want := []string{"PDF dokumenti", "*.pdf", "Sve datoteke", "*.*"}
	if len(got) != len(want) {
		t.Fatalf("split = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("split = %v, want %v", got, want)
		}
	}
}
