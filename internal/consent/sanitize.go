// Package consent computes what the consent screen shows — the
// security boundary of the whole product (SPEC §6.5) — as plain Go
// data, so it is fully testable without a window (F5 §10). Nothing
// here renders anything; internal/ui's consent window serialises a
// ViewModel to JSON and lets the page render it.
package consent

import "unicode"

// MaxDisplayLength is F5 §5.3's cap on a displayed file name: 120
// characters, with the middle elided.
const MaxDisplayLength = 120

// MaxVisibleFiles is F5 §5.3/§6.6's cap on the visible file list before
// switching to "...and N more".
const MaxVisibleFiles = 10

// truncationMark is three ASCII periods, not U+2026 — the same
// convention D-057 established for the visual stamp's own truncation
// mark, kept consistent project-wide.
const truncationMark = "..."

// isDirectionOverride reports whether r is one of the eight Unicode
// bidirectional-control characters SPEC §6.6 names explicitly:
// U+202A-U+202E (the "old" embedding/override controls) and
// U+2066-U+2069 (the "isolate" controls). These are exactly the
// characters an attacker uses to make a file name display differently
// from its actual bytes — e.g. "invoice‮fdp.exe" rendering as
// "invoice exe.pdf".
func isDirectionOverride(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

// SanitizeDisplayText implements F5 §5.3's "strip control characters
// and Unicode direction overrides" rule. It is applied to every piece
// of untrusted text the consent and pairing windows display — file
// names and, per F5 §6, the pairing application's name and origin.
//
// unicode.IsControl covers the C0/C1 control ranges (including NUL,
// the ASCII escape character, and friends); isDirectionOverride covers
// the bidi-control characters SPEC §6.6 names by codepoint, which are
// not classified as IsControl by Go's unicode package (they are format
// characters, category Cf, not Cc).
func SanitizeDisplayText(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsControl(r) || isDirectionOverride(r) {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// TruncateMiddle implements F5 §5.3's truncation rule: if s is longer
// than max runes, elide the middle, keeping the start and end (so a
// file extension — the part most likely to matter for recognising what
// a file is — always survives). Operates on runes, not bytes, so a
// multi-byte character is never split.
func TruncateMiddle(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= len(truncationMark) {
		// Degenerate case: no room for any real content alongside the
		// mark. Not reachable with F5's own max of 120, but a pure
		// function should not panic or slice out of range regardless of
		// the caller's max.
		return string(r[:max])
	}
	keep := max - len(truncationMark)
	head := keep - keep/2
	tail := keep / 2
	return string(r[:head]) + truncationMark + string(r[len(r)-tail:])
}

// SanitizeFileName applies F5 §5.3's full pipeline to one untrusted
// file name: strip control characters and direction overrides, then
// truncate to MaxDisplayLength with the middle elided. The result is
// display-only — F5 §5.3: "Never treat a name as a path."
func SanitizeFileName(name string) string {
	return TruncateMiddle(SanitizeDisplayText(name), MaxDisplayLength)
}

// CapFileNames implements F5 §5.3/§6.6: sanitise every name in names,
// then show at most MaxVisibleFiles, reporting how many were left out.
func CapFileNames(names []string) (shown []string, overflow int) {
	shown = make([]string, 0, len(names))
	for _, n := range names {
		shown = append(shown, SanitizeFileName(n))
	}
	if len(shown) > MaxVisibleFiles {
		overflow = len(shown) - MaxVisibleFiles
		shown = shown[:MaxVisibleFiles]
	}
	return shown, overflow
}
