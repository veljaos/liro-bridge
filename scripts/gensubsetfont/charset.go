package main

import "sort"

// asciiPrintable is the full printable ASCII range (space through tilde,
// U+0020..U+007E) — digits, basic Latin letters and every punctuation
// mark the stamp's generated text (labels, dates, serial numbers) uses
// (F4 §3.2).
const asciiPrintable = " !\"#$%&'()*+,-./0123456789:;<=>?@" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`" +
	"abcdefghijklmnopqrstuvwxyz{|}~"

// serbianLatinExtra is the five Serbian Latin letters not present in
// ASCII, both cases (F4 §3.2: "the Serbian Latin letters (č ć đ š ž and
// their capitals)").
const serbianLatinExtra = "čćđšžČĆĐŠŽ"

// serbianCyrillic is the complete 30-letter Serbian Cyrillic alphabet,
// both cases (F4 §3.2). This covers every real signer name this project
// has evidence for, including SPEC §11.7's MUP example
// ("ВЕЉКО СТАНОЈЕВИЋ"), whose givenName and surname both use letters
// (Љ, Ј, Ћ) that are absent from the main contiguous Cyrillic Unicode
// block and would be missed by a naive code-point-range approach.
const serbianCyrillic = "абвгдђежзијклљмнњопрстћуфхцчџш" +
	"АБВГДЂЕЖЗИЈКЛЉМНЊОПРСТЋУФХЦЧЏШ"

// serbianPunctuationExtra is punctuation Serbian text routinely needs
// that plain ASCII does not cover (Task 3, closing the F4 gap that let
// appearance.MissingGlyphError surface for the stamp's own em dash):
// em dash, en dash, the Serbian/German-style low/high quotation marks
// and their single equivalents, guillemets, the horizontal ellipsis, a
// non-breaking space, and the degree sign.
const serbianPunctuationExtra = "—–„“‚‘«»… °"

// charset returns the complete, deduplicated, sorted set of runes the
// embedded font subset must contain. This is the single source of truth
// for both the generated subset (this program's output) and
// charset.txt, the committed, human-reviewable record of it (F4 §3.2).
func charset() []rune {
	seen := map[rune]bool{}
	var out []rune
	add := func(s string) {
		for _, r := range s {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	add(asciiPrintable)
	add(serbianLatinExtra)
	add(serbianCyrillic)
	add(serbianPunctuationExtra)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
