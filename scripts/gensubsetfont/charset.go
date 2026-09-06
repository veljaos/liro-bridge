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

// previewCharset is the character set of the *substitute* font the
// placement preview draws with when a document's own font program is
// not embedded, or is embedded in a form the preview's reader does not
// draw (F6b §2.1).
//
// It is deliberately much wider than the stamp's set and deliberately
// still not the whole of Noto Sans. What it has to cover is the text
// that actually appears in the documents this agent is pointed at:
// Serbian in both scripts, the neighbouring Latin alphabets a contract
// with a foreign counterparty carries, the punctuation a word processor
// produces without being asked (curly quotes, en and em dashes,
// ellipses), currency and the handful of mathematical and geometric
// signs that turn up in tables and forms.
//
// Characters the source font does not have are dropped with a note
// rather than failing the build: this is a substitute, and one missing
// dingbat is a missing tick in a preview, not a missing letter in a
// signature.
func previewCharset() []rune {
	seen := map[rune]bool{}
	var out []rune
	add := func(r rune) {
		if r > 0 && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	addRange := func(lo, hi rune) {
		for r := lo; r <= hi; r++ {
			add(r)
		}
	}
	addString := func(s string) {
		for _, r := range s {
			add(r)
		}
	}

	addRange(0x0020, 0x007E) // printable ASCII
	addRange(0x00A0, 0x00FF) // Latin-1 supplement
	addRange(0x0100, 0x017F) // Latin Extended-A: Serbian, Croatian, Czech, Polish, Hungarian
	addString("ƒȘșȚțŴŵŶŷ")   // the Latin Extended-B letters WinAnsi and Romanian need
	addRange(0x02C6, 0x02DD) // the spacing modifiers the Latin encodings name
	addRange(0x0384, 0x03CE) // Greek, which turns up in formulae and unit names
	addRange(0x0400, 0x045F) // Cyrillic, including the Serbian letters
	addRange(0x2010, 0x2015) // hyphens and dashes
	addRange(0x2018, 0x201E) // single and double quotation marks
	addRange(0x2020, 0x2022) // dagger, double dagger, bullet
	addString("‰′″‹›⁄…‾")
	addRange(0x20A0, 0x20BF) // currency signs, the euro among them
	addString("™Ω№℅")
	addRange(0x2190, 0x2193) // arrows
	addString("∂∆∏∑−√∞∫≈≠≤≥∙·")
	addString("■□▪▫●○◊◄►▲▼")
	addString("✓✔✗✘☐☑☒")
	addString("ﬁﬂ")

	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
