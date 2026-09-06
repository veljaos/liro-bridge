# FTEST — unattended test report

**Date:** 2026-09-06
**Base commit:** `0deb494` (F6b: visual stamp placement, single-window flow, method selection)
**Machine:** the owner's own Windows 11 Pro (10.0.26200), AMD Ryzen 5 4500, Go 1.27.1, real MUP e-ID card in the reader.
**Run:** unattended. Nobody was at the machine.

Written as the work happened, in the order it happened. Numbers are measured
on this machine unless it says otherwise.

---

## 0. The boundaries, and what was actually done about them

| Boundary | What was done |
|---|---|
| No PIN, no signing with the real card | Nothing in this session opened a signing session against the card. Every signature came from the soft token (`testdata/softtoken/local/test.p12`). No PIN dialog appeared at any point. |
| No synthetic input | No `SetCursorPos`, `mouse_event`, `SendInput` or `keybd_event` anywhere. Windows were driven through `Window.Eval` inside their own DOM and through window messages posted to this session's own windows, and photographed with `PrintWindow`. |
| Never kill by image name | Every process this session started was stopped by its own exact PID. |
| Leave the machine as found | Snapshot taken before anything ran (below). Restoration verified at the end, not assumed. |

### 0.1 The snapshot taken before anything ran

```
config.json          locale sr-Latn, level b-b, visibleStamp true, stampPosition custom,
                     stampX 185.37141393442624, stampY 478.0389344262295, stampPlacedPage 1,
                     outputFolder "", explorerMenuEnabled true, startWithWindows true
HKCU\...\Run
  LiroBridge         "C:\Users\Veljko\Desktop\liro-bridge\liro-bridge.exe"
HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign
  (default)          Потпиши користећи Liro Bridge
  MultiSelectModel   Player
  Icon               C:\Users\Veljko\AppData\Local\Liro\icon-13717.ico
  \command           "C:\Users\Veljko\Desktop\liro-bridge\liro-bridge.exe" --shell-verb "%1"
audit log            %LOCALAPPDATA%\Liro\audit\2026-09-001.jsonl, 101 320 bytes,
                     SHA-256 f3d7a5fb917cf336acabed2e011e93e95ce1f9e3662a7425822f1d694d930fe1
```

Copies of all of it are in this session's scratch directory, and the
restoration check is at the end of this report.

---

## What broke

### B-1 — `go test ./...` rewrites the owner's real autostart entry (fixed)

**What it was.** `internal/platform.TestWindowsAutostartRoundTrip` recorded
only *whether* autostart was enabled (`a.IsEnabled()`, a bool) and restored it
with `a.SetEnabled(before, testExePath)`. `testExePath` is
`C:\test\liro-bridge.exe`, a path deliberately chosen not to exist. So on any
machine where autostart was on — this one — one `go test ./...` replaced the
real value with a path to nothing, and the agent would silently stop starting
with Windows.

Measured, not inferred. Before the baseline suite run:

```
LiroBridge = "C:\Users\Veljko\Desktop\liro-bridge\liro-bridge.exe"
```

After it:

```
LiroBridge = "C:\test\liro-bridge.exe"
```

D-134 already recorded this as a known defect in a package that pass did not
touch. FTEST §0.4 asks for it to be fixed, and it is.

**The fix.** `keepAutostartValue(t, keyPath)` snapshots the value *verbatim*
plus whether it was present at all, and puts exactly that back — deleting the
value again if there was none before. `TestWindowsAutostartRoundTrip` uses it.

**The tests that now cover it.** `TestAutostartTestsPutBackWhatTheyFound` seeds
a scratch key with a value that looks like a real installation, runs the
round-trip inside a nested `t.Run` (the only way to observe what a `t.Cleanup`
actually restores), and asserts the seeded string comes back byte for byte.
`TestAutostartTestsDoNotInventAnEntry` covers the other direction: a machine
with no entry must not have one afterwards.

Confirmed to fail against the old shape — the bool-and-`SetEnabled` restore
reintroduced by hand produced

```
autostart value = "\"C:\\test\\liro-bridge.exe\"" after a test run,
want it untouched at "\"C:\\Users\\Somebody\\Desktop\\liro-bridge\\liro-bridge.exe\""
```

— and to pass after. The owner's real value was restored by hand as soon as
the corruption was measured.

**Not in scope of this fix, and checked:** `internal/platform`'s shell-menu
tests already use a scratch key of their own and never touch the real Explorer
verb; `cmd/liro-bridge`'s `keepThisMachinesAutostartAndMenu` already restores
both registrations verbatim. Only this one test was writing to the real one.

---

## What held

### Baseline suite

`go test ./... -count=1 -tags softtoken` on this machine, before any change:
**28 packages, all green.** Slowest: `cmd/liro-bridge` 47.0 s,
`internal/pades` 19.6 s, `internal/ui` 16.5 s, `internal/jobs` 11.9 s,
`internal/pades/tsa` 9.7 s (it contacts Pošta's real test TSA).

---


---

## Group 1 — documents, the renderer, and the three languages

### B-2 — the renderer never said it had substituted a font (fixed)

**What it was.** D-136 states that everything the renderer cannot draw
faithfully "increments a counted note on the result", and D-137 states
that "every metric comes from the document". Neither held for the most
ordinary substitution there is: a standard-14 font named with no
`/FontDescriptor` and no `/Widths`, which PDF 32000-1 §9.6.2.2 entitles
a producer to write.

Measured: **380 documents rendered, 2 472 pages, and the only notes that
fired in the whole sweep were four** —

```
gfx-mesh-shading.pdf             shading type approximated as flat colour=1
gfx-shading-pattern-inline.pdf   tiling pattern approximated as flat colour=1
img-jbig2.pdf                    image codec JBIG2Decode not supported=1
img-jpx.pdf                      image codec JPXDecode not supported=1
```

Every document whose text was drawn entirely in substituted glyphs —
five of the corpus, plus the standard fonts inside the three real
fixtures — reported an empty `Notes` map, including the ones whose
advance widths the renderer had to invent. The existing note fires only
when a font program is *present and unreadable*, which is the rarer half.

**The fix.** Two notes, kept separate because they are different facts:

- `substituted the shapes of a font the document does not embed` —
  harmless for placing a stamp; every word still begins and ends where
  the real one does.
- `advance widths guessed: the document declares none` — the case where
  D-137's load-bearing property does not hold, and a line comes out a
  few per cent long.

Both are noted once per font per page, from `showText` rather than at
load time, so a font declared in `/Resources` and never drawn reports
nothing.

**The tests.** `TestSubstitutedFontAndGuessedWidthsAreCounted` and
`TestWidthsTheDocumentSuppliesAreNotReportedAsGuessed` — the second is
the one that keeps the note worth reading, since a document that *does*
say how wide its characters are must not be accused of having declined
to. Both confirmed to fail against the previous code (`notes were
map[]`) and pass against this one.

### B-3 — the command line showed raw operating-system and parser English (fixed)

**What it was.** Driven against the shipped binary over the abuse corpus,
`liro-bridge sign` printed, in a `sr-Latn` interface:

| Input | What the user was shown |
|---|---|
| a directory named `folder-not-a-file.pdf` | `read C:\...\folder-not-a-file.pdf: Incorrect function.` |
| a read-only output, with `--force` | `open C:\...\readonly-target.pdf: Access is denied.` |
| an exclusively-locked output, with `--force` | `open C:\...: The process cannot access the file...` |
| `--out` into a directory that does not exist | `open C:\...: The system cannot find the path specified.` |
| an exclusively-locked *input* | `open C:\...: The process cannot access the file...` |
| `corrupt-startxref.pdf` | `pdf: document has no startxref to chain /Prev from` |
| `deep-nesting.pdf` | `pdf: /Root does not resolve to a dictionary` |
| `page-parent-loop.pdf` | `pdf: no /Page found under node 2` |

`INPUT_UNREADABLE` (D-118), `OUTPUT_WRITE_FAILED` (D-104) and
`PDF_INVALID` exist for exactly these, with a sentence in all three
catalogues, and the window path has used them since F6. The command line
returned the raw `os` error and the raw parser error, which `errMessage`
then rendered with `err.Error()`.

**The fix.** `os.ReadFile` becomes `INPUT_UNREADABLE`, `os.WriteFile`
becomes `OUTPUT_WRITE_FAILED` in `internal/cli.signOneFile`; the seven
structural failures in `internal/pades/pdf` and `internal/pades` are
wrapped as `PDF_INVALID` at the point the fact is known, which fixes both
front doors rather than only the one that was measured — the window path
maps an unclassified error to `INTERNAL`, "An unexpected error
occurred", for a document that is simply broken, which is the same
defect D-104 fixed one case over.

`sign.output_exists` is deliberately left alone: it is already localised
and it names `--force`, which is what a person needs to be told, exactly
as D-104 recorded.

**The tests.** `TestSignCommandNeverShowsARawOperatingSystemError`
(three situations, asserting both that the catalogue's sentence appears
and that six specific raw wordings do not) and
`TestSignCommandSaysTheSameThingInEveryLanguage` (the same situation in
all three locales). Both confirmed to fail against the previous code, in
every sub-case, and pass against this one.

### §4 — what the corpus is, and what the abuse cases did

**The corpus: 45 generated documents plus the three real fixtures.**
Page counts 1, 2, 5, 17, 50, 200, 500. Sizes A4, A5, A3, US Letter,
200×900, 120×120, 2000×3000. Rotations 0, 90, 180, 270, and one document
with all four on different pages. Producers labelled as Microsoft Print
to PDF, LibreOffice, a Xerox scanner, SAP and the eGovernment applet.
Cyrillic, Latin and mixed text, with an embedded TrueType font and with
the standard fourteen. Cross-reference mechanisms: classic tables, a
genuine cross-reference stream carrying an object stream, and a
hand-built mixed history reproducing exactly the shape D-075 measured in
`mup.pdf` — a classic base revision, a stream revision reached through a
hybrid `/XRefStm` stub, and two further classic revisions. Images: JPEG
(DCTDecode), CCITT Group 3 and Group 4 produced by Pillow, a Flate RGB
image with an `/SMask`, and JBIG2 and JPEG 2000 streams that exist to be
degraded rather than decoded. An axial shading, a tiling pattern, an
inline image, a type-4 mesh shading, a standard font with no `/Widths`
at all, and a page whose `/MediaBox` and `/Rotate` are inherited from the
`Pages` node with a `/CropBox` that differs.

**Signing the whole corpus: 45 of 45, in 0.44 s.** Every output verified
with `internal/pades/verify`: **51 signature slots, every one of them
`ByteRangeDigestOK`, `SignatureOK` and `SigningCertificateOK`.** The
three documents that report a failing check report it on their own
*pre-existing* signatures, and both failures are already documented: the
state tool's SHA-1 timestamp imprint (SPEC §12.4) and the
document-timestamp slot whose `messageDigest` covers its embedded
TSTInfo rather than the `/ByteRange` (`verify/real_test.go`).

Every one of the 45 signed outputs opens in **two independent readers**:
pypdf 6.10.2 and PDFium via pypdfium2 5.8.0 — 45/45 each, with the page
count and the AcroForm field count matching what was signed (one new
field on a fresh document, three on a real fixture that already had two).

**The abuse cases, against the shipped binary.**

| Case | Result |
|---|---|
| zero bytes, one byte, plain text, HTML | refused, `PDF_INVALID`, nothing written |
| truncated at half, truncated mid-object | refused, `PDF_INVALID`, nothing written |
| encrypted RC4-128, AES-256, AES-128 with an empty user password | refused, `PDF_ENCRYPTED`, nothing written |
| a directory named `.pdf` | refused; now `INPUT_UNREADABLE` (was a raw OS error, B-3) |
| a file that does not exist | refused, "no input file matches --in" |
| 5 000 NUL bytes before `%PDF` | signed; output verifies; pypdf opens it, PDFium refuses it — and PDFium refuses the *input* the same way, so this is inherited, not introduced |
| `/Length` claiming 999 999 999 | signed; output verifies; both readers open it |
| `%%EOF` removed; `startxref` misspelt; `startxref` 999999999; `startxref -42` | the first, third and fourth are signed through the rebuild fallback and verify; the second is refused (`PDF_INVALID`) |
| NUL bytes inside three `obj` keywords | signed; output verifies |
| 600 levels of nested array | refused, `PDF_INVALID` — D-039's 500-level cap holds |
| a 100 MB Flate decompression bomb | signed in 60 ms; output verifies; both readers open it |
| a self-referential `/Parent` page tree | refused, `PDF_INVALID` — no loop, no hang |
| read-only input | signed |
| read-only output, with `--force` | refused; the existing file is byte-identical afterwards |
| output held open exclusively by another process, with `--force` | refused; the existing file is byte-identical afterwards |
| input held open exclusively by another process | refused, `INPUT_UNREADABLE` |
| output already exists, no `--force` | refused; the existing file is byte-identical afterwards; the message names `--force` |
| a path 726 characters long | refused — see J-1 |
| names with Cyrillic, an emoji, three consecutive spaces, a right-to-left override, a curly apostrophe, `%`/`&`/`#`, and a 200-character component | all seven signed, all seven outputs verify |
| a glob one of whose members was deleted first | the rest are signed |
| a UNC path to an unreachable host | refused after **42.3 s** — see J-2 |
| 100 documents with #047 corrupt | 99 signed, 1 reported, batch continues (SPEC §12.10) |
| 200 documents at once | 200 signed in 2.2 s |
| two files of one name in two folders | both signed, each beside its own input |

No crash, no hang other than J-2, and **no half-written file anywhere**:
in every refusal the output either did not exist or was byte-identical
to what it had been, checked by hash.

### §5 — the renderer

**The sweep.** 380 files — the corpus, the abuse cases, the real
fixtures, the golden file and every signed output — **2 472 pages, 0
panics, 0 hangs, every page of every parseable document rendered**, 64.7 s
in total. The fifteen files that produced an error produced the right
one: three `PDF_ENCRYPTED`, nine `PDF_INVALID` on files that are not
PDFs at all (including a `.gitkeep` and a `README.md`, handed in
deliberately), and the two structural cases now classified by B-3.

**Against an independent renderer.** Each page rendered at 1.5 px/pt was
compared against PDFium's rendering of the same page, reduced to a 48×48
grid of ink fractions — a measure sensitive to a lost image, a flipped
axis, a missing page of text or a stamp in the wrong place, and
insensitive to anti-aliasing and letterform differences.

**73 of 78 page comparisons correlate at 0.90 or better; 60 of them at
0.99 or better.** The five that do not are each explained, and each was
looked at rather than waved through:

| Page | corr | mean abs | Why |
|---|---|---|---|
| `gfx-shading-pattern-inline.pdf` | 0.172 | 0.0159 | The metric, not the renderer. The page is one smooth red→blue axial gradient, so the ink field is nearly constant and correlating its residuals means nothing. Sampled directly, the two agree to within 1–2 of 255 at all four corners and at the centre. The 1.6% difference is the tiling pattern, which this renderer flattens to grey by design, and the inline image. |
| `font-no-widths.pdf` | 0.896 | 0.0005 | D-137's honest gap: a standard font with no `/Widths`, so the advances are guessed and the line comes out a few per cent long. Exactly the case B-2 now counts. |
| `text-times-std14.pdf` | 0.898 | 0.0070 | Substituted shapes: Noto Serif where PDFium has Times. |
| `text-latin-std14.pdf` | 0.964 | 0.0074 | The same, sans. |
| `gfx-mesh-shading.pdf` | 1.000 | 0.4980 | A type-4 mesh shading, flattened to the middle of its colour ramp (D-136). Both images are uniform at very different levels, so the correlation is vacuous and the mean difference is the real number. Noted on the result. |
| `box-inherited.pdf` | — | — | Not comparable, for the reason D-136 states: this renderer shows the `/MediaBox` and PDFium shows the `/CropBox`. Measured at 1263×893 against 1203×833, for a page whose two boxes differ by 20 pt a side with `/Rotate 90` inherited from the `Pages` node. Documented behaviour, now with a number against it. |

**The counted notes fire.** `image codec JBIG2Decode not supported`,
`image codec JPXDecode not supported`, `tiling pattern approximated as
flat colour` and `shading type approximated as flat colour` each fired
once, on exactly the document built to provoke them, and each of those
documents still rendered a usable page: the JBIG2 and JPX pages carry
their text and their frame with the image area left blank, and both
signed and verified afterwards. The two font notes are B-2.

**Timing** (median of three, after one warm-up):

| Page | 0.7 px/pt (fit) | 1.33 | 1.5 | 2.66 (200%) | 5.33 (400%) |
|---|---|---|---|---|---|
| A4 text page, corpus | 6.3 ms | 9.6 ms | 10.5 ms | 23.4 ms | 65.5 ms |
| `mup.pdf` page 5, dense justified Cyrillic | — | 24.3 ms | 25.5 ms | — | 104.1 ms |
| CCITT G4 scan | — | — | 21.6 ms | — | 259.1 ms |
| JPEG scan | — | — | 31.2 ms | — | 361.0 ms |

At 400% an A4 page is 14.2 million pixels; the 40-million-pixel guard
(`MaxRenderPixels`) refuses anything larger rather than allocating it,
which the 2000×3000 pt fixture exercises.

**Laziness confirmed, and it is the property the placement window
depends on:**

```
500-page document:  Open 2.68 ms, +1.3 MB;  three pages 34.8 ms, +34.1 MB
200-page document:  Open 1.08 ms, +0.5 MB;  three pages 35.3 ms, +34.1 MB
```

The three-page cost is identical for both, so it scales with pages
rendered, not with pages present.
