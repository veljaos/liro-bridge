package main

import "testing"

// TestHexLiteralFailsTheCheck is the "write a test that makes it fire"
// discipline F5 §4.2 asks for explicitly: a hex colour literal in agent
// CSS must be caught, not just theoretically catchable.
func TestHexLiteralFailsTheCheck(t *testing.T) {
	tests := []struct {
		name string
		css  string
		want int
	}{
		{"six-digit hex", "body { color: #038387; }", 1},
		{"three-digit hex", "a { color: #fff; }", 1},
		{"eight-digit hex with alpha", "a { color: #038387cc; }", 1},
		{"uppercase hex", "a { color: #ABCDEF; }", 1},
		{"two hex literals on separate lines", "a { color: #fff; }\nb { color: #000; }", 2},
		{"var() reference only, no literal", "body { color: var(--liro-color-brand); }", 0},
		{"no colour at all", "body { margin: 0; }", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lines := splitLines(tc.css)
			got := checkCSS("consent.css", lines)
			if len(got) != tc.want {
				t.Fatalf("checkCSS found %d violation(s), want %d: %+v", len(got), tc.want, got)
			}
		})
	}
}

// TestTokensCSSIsExempt proves the one deliberate exception: tokens.css
// itself, which defines the hex values as custom properties, must never
// be flagged — otherwise the check could never be satisfied by any
// build at all.
func TestTokensCSSIsExempt(t *testing.T) {
	lines := splitLines(":root { --liro-color-brand: #038387; }")
	if got := checkCSS("tokens.css", lines); len(got) != 0 {
		t.Fatalf("checkCSS flagged tokens.css itself: %+v", got)
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
