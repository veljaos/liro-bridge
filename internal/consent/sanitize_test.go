package consent

import "testing"

// TestSanitizeDisplayTextNeutralisesDirectionOverride is F5 §5.3's
// required test: "a file name containing a right-to-left override must
// render neutralised." Verifies both that the override character is
// gone and that its concrete spoofing effect (an .exe reading as .pdf)
// cannot occur any more — checking only "the character is absent" would
// pass for a fix that left some other exploitable sequence behind.
func TestSanitizeDisplayTextNeutralisesDirectionOverride(t *testing.T) {
	// "racun" + U+202E (RIGHT-TO-LEFT OVERRIDE) + "fdp.exe" is F5 §5.3's
	// own example: the override makes everything after it render
	// reversed, so "fdp.exe" displays as "exe.pdf" — the file looks
	// like a PDF. Built with string(rune(...)) rather than a literal
	// direction-override character in the source file, so the source
	// itself stays plain ASCII.
	malicious := "racun" + string(rune(0x202E)) + "fdp.exe"
	got := SanitizeDisplayText(malicious)

	if got == malicious {
		t.Fatal("SanitizeDisplayText did not change the malicious input at all")
	}
	for _, r := range got {
		if isDirectionOverride(r) {
			t.Fatalf("SanitizeDisplayText(%q) = %q still contains a direction-override character U+%04X", malicious, got, r)
		}
	}
	want := "racunfdp.exe"
	if got != want {
		t.Fatalf("SanitizeDisplayText(%q) = %q, want %q", malicious, got, want)
	}
}

// TestSanitizeDisplayTextStripsEveryDirectionOverride covers all eight
// codepoints SPEC §6.6 names explicitly, not just U+202E.
func TestSanitizeDisplayTextStripsEveryDirectionOverride(t *testing.T) {
	overrides := []rune{0x202A, 0x202B, 0x202C, 0x202D, 0x202E, 0x2066, 0x2067, 0x2068, 0x2069}
	for _, r := range overrides {
		s := "a" + string(r) + "b"
		got := SanitizeDisplayText(s)
		if got != "ab" {
			t.Errorf("SanitizeDisplayText(%q) = %q, want %q (U+%04X not stripped)", s, got, "ab", r)
		}
	}
}

func TestSanitizeDisplayTextStripsControlCharacters(t *testing.T) {
	s := "a\x00b\x1bc\td"
	got := SanitizeDisplayText(s)
	// \t (tab) is itself a control character (category Cc) and must be
	// stripped too — this test intentionally includes one to prove the
	// function does not special-case "printable-looking" controls.
	if want := "abcd"; got != want {
		t.Fatalf("SanitizeDisplayText(%q) = %q, want %q", s, got, want)
	}
}

func TestSanitizeDisplayTextLeavesOrdinaryTextUntouched(t *testing.T) {
	// Cyrillic and Latin-with-diacritics must survive unchanged — this
	// is display sanitisation, not script normalisation (SPEC §9.3).
	for _, s := range []string{"уговор.pdf", "račun-č-ć-đ-š-ž.pdf", "invoice (final).pdf"} {
		if got := SanitizeDisplayText(s); got != s {
			t.Errorf("SanitizeDisplayText(%q) = %q, want unchanged", s, got)
		}
	}
}

func TestTruncateMiddle(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"shorter than max is unchanged", "short.pdf", 120, "short.pdf"},
		{"exactly max is unchanged", "1234567890", 10, "1234567890"},
		{"longer than max is elided in the middle", "1234567890", 6, "12...0"},
		{"odd remaining space favours the head", "1234567890", 8, "123...90"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncateMiddle(tc.s, tc.max); got != tc.want {
				t.Fatalf("TruncateMiddle(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
			}
		})
	}
}

// TestSanitizeFileNameCombinesBothRules proves the two rules compose:
// a name that is both malicious and overlong is neutralised AND kept
// under MaxDisplayLength.
func TestSanitizeFileNameCombinesBothRules(t *testing.T) {
	long := "a" + string(rune(0x202E)) + "bbbbbbbbbb" // direction override, then a long run
	for i := 0; i < 130; i++ {
		long += "x"
	}
	long += ".pdf"

	got := SanitizeFileName(long)
	if len([]rune(got)) > MaxDisplayLength {
		t.Fatalf("SanitizeFileName result is %d runes, want <= %d", len([]rune(got)), MaxDisplayLength)
	}
	for _, r := range got {
		if isDirectionOverride(r) {
			t.Fatalf("SanitizeFileName(%q) still contains a direction override", long)
		}
	}
}

func TestCapFileNames(t *testing.T) {
	names := make([]string, 12)
	for i := range names {
		names[i] = "file.pdf"
	}
	shown, overflow := CapFileNames(names)
	if len(shown) != MaxVisibleFiles {
		t.Fatalf("got %d shown names, want %d", len(shown), MaxVisibleFiles)
	}
	if overflow != 2 {
		t.Fatalf("got overflow %d, want 2", overflow)
	}
}

func TestCapFileNamesUnderTheCapHasNoOverflow(t *testing.T) {
	shown, overflow := CapFileNames([]string{"a.pdf", "b.pdf"})
	if len(shown) != 2 || overflow != 0 {
		t.Fatalf("got shown=%v overflow=%d, want 2 names and 0 overflow", shown, overflow)
	}
}

func TestCapFileNamesSanitisesEachEntry(t *testing.T) {
	shown, _ := CapFileNames([]string{"racun" + string(rune(0x202E)) + "fdp.exe"})
	if shown[0] != "racunfdp.exe" {
		t.Fatalf("CapFileNames did not sanitise its entries: got %q", shown[0])
	}
}
