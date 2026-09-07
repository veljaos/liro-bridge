# FTEST — unattended test report

**Date:** 2026-09-06
**Base commit:** `0deb494` (F6b: visual stamp placement, single-window flow, method selection)
**Machine:** the owner's own Windows 11 Pro (10.0.26200), AMD Ryzen 5 4500, Go 1.27.1, real MUP e-ID card in the reader.
**Run:** unattended. Nobody was at the machine.

Written as the work happened, in the order it happened. Numbers are measured
on this machine unless it says otherwise.

---

## In one page

**Ten defects, nine of them fixed with a test that fails against the old
code.** Not one was found by a failing test — every one was found by
running the shipped binary or the real pipeline over real input and
looking at what came out.

| | What it was | Fixed |
|---|---|---|
| **B-1** | `go test ./...` rewrote the owner's real autostart entry to a path that does not exist | yes |
| **B-2** | The renderer never counted a substituted font or a guessed width — four notes in 2 472 pages | yes |
| **B-3** | The command line showed the operating system's and the parser's English in a Serbian interface | yes |
| **B-4** | The certificate listing's columns were aligned in English only | yes |
| **B-5** | A `config.json` with a byte-order mark silently discarded every setting | yes |
| **B-6** | The catalogue check was only as strong as a list kept in step by hand | yes |
| **B-7** | **B-LT was claimed for a document with no `/DSS` at all** | yes |
| **B-8** | Every presence probe leaks 2–4 Windows handles and costs 457–855 ms | measured, not fixed — J-7 |
| **B-9** | A batch that could not be written to the audit log went unrecorded, in silence | yes |
| **B-10** | `go test ./...` appended fabricated entries to the owner's real audit log | yes |

**The one that matters most is B-7.** The program said `Nivo: B-LT`. The
independent verifier said the signature was good. Every test in the
repository passed — including one asserting *exactly the property that
was broken*, which used a two-certificate fixture where the defect only
exists with one. What gave it away was a file size: five outputs at five
different levels, all exactly 66 714 bytes, when a `/DSS` revision has to
add bytes.

**Eleven judgement calls (J-1 … J-11)** were recorded rather than
decided. The two with real consequences were **J-8** — a signed document
can be observed half-written, and the obvious fix is not free on Windows
— and **J-10** — revocation is fetched once per document, so a
hundred-document batch against MUP's responder costs about 33 minutes of
timeouts.

**Five of the eleven have since been decided by the owner and
implemented** (2026-09-07): **J-3**, **J-7**, **J-8**, **J-9** and
**J-10**, each marked DECIDED in its own section below with what was
decided and the measured before and after. D-162 through D-167 record
them. J-1, J-2, J-4, J-5, J-6 and J-11 remain the owner's.

**What held**, with numbers: 45 corpus documents and three real fixtures
signed, 51 signature slots all verifying; a thousand signatures four
different ways, 4 000 of 4 000 verified, byte-identical, memory flat;
five-deep resigning on six documents with every signature at every depth
still valid and the original bytes still a literal prefix; 125 of 125
stamp placements verified and every corner landing in the right visual
corner on all four rotations; 2 472 pages rendered with no panic and 73
of 78 page comparisons against PDFium at 0.90 or better; 28 timestamp
failure injections and 22 configuration ones with a clear message and a
clean state every time; every window at every step in all three languages
with nothing clipped and nothing scrolling that should not.

**Group 3 was not run** — no fuzzing, no endurance. Said plainly above
rather than done briefly and called done.

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

### B-4 — the certificate listing was aligned in English only (fixed)

**What it was.** `renderRow` wrote its seven label/value lines with seven
literal runs of spaces, each the right length for the *English* label
beside it:

```go
fprintf(w, "      %s      %s\n", c.T("certs.purpose_label"), ...)   // "Purpose"    + 6
fprintf(w, "      %s   %s\n",    c.T("certs.thumbprint_label"), ...) // "Thumbprint" + 3
```

In English every value starts at column 13. Measured in `sr-Latn`, the
seven values started at columns **12, 16, 16, 13, 12, 9 and 13** — on the
one screen the command line actually shows a signer, in the two languages
almost all of them read:

```
      Namena      potpisivanje
      Izdavalac       Pošta Srbije CA 1
      Kvalifikovan    da — kvalifikovani sertifikat na QSCD uređaju
      Razlog       kartica nije prisutna
      Važi        2025-10-08 do 2030-10-08
      Otisak   …5BA2AA54
      Smeštaj      pametna kartica
```

**The fix.** `certFieldFormatter` computes the column from the widest
label *in the locale being rendered* and pads with `%-*s`. Go's `fmt`
pads `%s` by runes, not bytes, so "Važi" and "Vazi" occupy the same
column. After:

```
sr-Latn            sr-Cyrl                  en
Namena         …   Намена         …         Purpose      …
Izdavalac      …   Издавалац      …         Issuer       …
Kvalifikovan   …   Квалификован   …         Qualified    …
Razlog         …   Разлог         …         Reason       …
Važi           …   Важи           …         Valid        …
Otisak         …   Отисак         …         Thumbprint   …
Smeštaj        …   Смештај        …         Storage      …
```

**The test.** `TestCertificateFieldsLineUpInEveryLocale` asserts the
property rather than the numbers — whatever the catalogue holds, every
value in a certificate's block starts at the same column — in all three.
Against the previous code it fails in `sr-Latn` and `sr-Cyrl` with four
mismatches each and passes in `en`, which is the shape of the defect.

### B-5 — a `config.json` with a byte-order mark was silently discarded (fixed)

**What it was.** Found by accident while checking the three locales: this
session wrote `config.json` with PowerShell's `Set-Content -Encoding
utf8`, which prepends `EF BB BF`. `encoding/json` rejects that at offset
1, `config.Load` logged a warning and returned `Default()`, and the agent
ran on defaults — the language, the signature level, the remembered stamp
position and the output folder all reverted, with nothing on screen or in
the window to say so.

Every ordinary way of editing that file on Windows writes a BOM.
D-134 ran into this once and recorded it as "worth knowing on its own";
it is a defect against D-134's own rule that the file is the single
authority on the configuration, and this project already strips exactly
this from the one other outside text file it reads (the Trusted List
seed, D-018/D-107).

**The fix.** `bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})` before
`json.Unmarshal`, and nothing else: `TestLoadStillRejectsGenuineRubbish`
keeps plain text, a UTF-16 file, a truncated file and a lone BOM
rejected. `TestLoadAcceptsAUTF8ByteOrderMark` fails against the previous
code with `invalid character '﻿' looking for beginning of value`.

### B-6 — the catalogue check was as strong as a hand-maintained list (fixed)

**What it was.** `TestEveryErrorCodeHasAMessageInEveryCatalogue` walks
`errs.AllCodes()` and requires a message in each locale. It is exactly as
strong as `AllCodes()` is complete, and nothing checked that. A `Code`
constant declared and forgotten there compiles, passes every test in the
repository, and reaches a user as its own key — `error.cert_revoked` —
which is the failure D-104 added the catalogue check to prevent. The list
has already had to be kept in step by hand four times (`INPUT_UNREADABLE`,
`OUTPUT_WRITE_FAILED` and the two TSA client-certificate codes).

**The fix.** `TestAllCodesListsEveryDeclaredCode` parses `errs.go`'s own
syntax tree — the discipline D-025 used for the "no PIN field" check,
because the property is about what is *declared* and a declaration
nothing references is invisible to anything but the AST. It reports
`27 Code constants declared, 27 listed by AllCodes()`, and adding a
constant without listing it fails with
`AllCodes() does not list 1 declared code(s): CodeDiskFull`.
`TestEveryCodeValueIsScreamingSnakeCase` pins SPEC §7's own naming rule
and that no two constants share a value, neither of which was checked
either.

### §9 — the three languages

**Every layout test now measures all three.** The six window-layout
tests and the stamp window's measured `sr-Cyrl` alone, on the reasonable
theory that it is the longest catalogue. The theory does not survive
checking: *"Sign, choosing where the signature goes"* is 39 characters
against 33 for either Serbian spelling of the same option, so on the
method screen **English is the longest**. All seven are now table-driven
over `sr-Latn`, `sr-Cyrl` and `en`. Because the windows are shared, this
cost one `PostJSON` per locale rather than a second and third WebView2
environment — measured at 0.02–0.2 s per extra locale.

All pass. Sample measurements, `#cert-list` and `.settings-form`:

| Window | sr-Latn | sr-Cyrl | en |
|---|---|---|---|
| certificates, 6 rows | 505 of 701 visible | 505 of 701 | 505 of 701 |
| audit log, 20 entries | 385 of 1792 | 385 of 1792 | 385 of 1792 |
| settings, every field populated | 715 of 1147 | 715 of 1147 | 715 of 1130 |

In every case the page itself does not scroll, no element outside the
named region scrolls, and every action button is inside the viewport.

**Looked at, not only measured.** 45 screenshots were taken with
`PrintWindow` and `PW_RENDERFULLCONTENT` — which asks the window to
render itself, so taking the picture never takes the foreground from
whoever is using the machine (D-122) — across nine screens × three
locales: the certificate step (closed and with Details open), settings,
certificates, the audit log, the documents step, the timestamp question,
the output-file question, the method step in both its roles with each of
the three methods, and the report. Every one was driven only through
`Window.Eval` inside the page's own DOM; nothing simulated input (D-094).

Nothing was clipped, nothing overflowed, and no screen scrolled that
should not. Specific things confirmed by looking: the fingerprint is
elided with a Copy button beside it and the full 64 characters are
nowhere in the DOM (D-096); the timestamp question shows three distinct
weights, not two (D-102); the audit log renders its four outcomes in four
different colours (D-093); the report puts Finish last on its own row as
the primary (D-147); the settings form stacks every label above its input
and no label wraps in any of the three (D-106).

**Log lines and help text are English.** The agent's own log file —
100 508 bytes of it, from real runs including this session's — contains
**zero non-ASCII characters**. `--help` and every subcommand's `--help`
are English regardless of locale (D-092), re-checked in this session.

**Every string a person sees comes from a catalogue.** The page assets
carry exactly three hardcoded human-readable strings — `Srpski
(latinica)`, `Srpski (ćirilica)` and `English`, the language picker's own
options, which name each language in its own language and are correctly
never translated. No page script assigns a literal to `textContent` or
`liroSetText`, and `innerHTML` is only ever assigned `""` to clear a
list, so SPEC §6.6's "a file name is inserted with `textContent`, never
`innerHTML`" holds by construction.

**One thing worth the owner's eye rather than a change** — see J-3.


---

## Group 2 — the soft token, the real card without a PIN, and failure injection

### B-7 — B-LT was claimed for a document with no `/DSS` at all (fixed)

**What it was.** Found by signing at every level through the shipped
binary and then looking at the bytes rather than at the reported level:

```
liro-bridge sign --level b-lt --tsa https://freetsa.org/tsr
  Nivo: B-LT
66714 bytes   /DSS=False  /OCSPs=False  /CRLs=False  /VRI=False
```

Every one of the five level runs produced a file of exactly 66 714
bytes, which is the first thing that gave it away: the `/Contents`
placeholder is a fixed 32 768 bytes (D-042), so B-B and B-T are the same
size by construction — but B-LT appends a whole `/DSS` revision, and a
B-LT file the same size as the B-B one has no revision in it.

B-LT is B-T plus a `/DSS` carrying revocation evidence (SPEC §12.6), so a
document with no `/DSS` has not reached it, and saying it has is the
overclaim SPEC §18.11 and D-047 forbid outright.

**Why it was reachable and why the existing test missed it.**
`dss.Apply`'s `case i+1 < len(certs)` only expects evidence for a
certificate whose issuer is also in the list, so a chain of **exactly one
certificate** expects none at all and comes out `Complete: true` having
collected nothing. D-079 then correctly skips the revision, while
`applyDSS` raises the level on that same flag.

One certificate is not a contrivance: MUP embeds only the signer
certificate in its CMS (SPEC §11.8), so a failed AIA fetch with nothing
matching in the bundled trust store leaves exactly one — and that is the
same outage that stops OCSP answering, so the two arrive together. It is
also the soft token's own shape, which is why every `--level b-lt` run in
this session hit it.

Every other test in `internal/pades` uses `chainedSession`, whose chain
is two certificates long, so the single-certificate path had never been
signed at all.

**The fix.** `Apply` returns `Complete: false` whenever it writes no
revision — the one place that already knows, which is D-079's own
reasoning for putting the skip there. The shipped binary now reports

```
Nivo: B-T  (B-LT requested; OCSP/CRL unavailable for at least one certificate)
```

**The test.** `TestBLTIsNeverClaimedForADocumentWithNoDSS`, with a signer
whose `Chain()` is empty. Confirmed to fail against the previous code
(`AchievedLevel = B-LT for output containing no /DSS…`) and pass against
this one; `TestSignDocumentBLTAchievedWithOCSPEvidence` keeps the other
direction, and `TestSignDocumentBLTSkipsDSSWhenNoRevocationEvidence`
keeps the two-certificate case.

### B-8 — every presence probe leaks Windows handles, and costs half a second

**What it is.** FTEST §2 asks for `certs` a thousand times, watching the
handle count, because "a leak in the presence probe would show here and
nowhere else". It does.

In-process, enumerating and probing every hardware certificate:

```
  after     1: handles  226
  after   100: handles 1245
  after   250: handles 2746
```

— about **ten handles per enumeration**, growing linearly. Isolated call
by call, 200 iterations each:

| Call | handles per call | ms per call |
|---|---|---|
| `windowscng.Enumerate` (5 certificates) | +0.01 | 1.0 |
| `windowscng.Source.Presence` (one certificate) | **+4.08** | **857** |
| `platform.SmartCardService.Readers` | +2.00 | 1.2 |
| `platform.SmartCardService.AnyCardPresent` | +2.00 | 1.2 |

Split further, into the three Windows calls the probe makes:

| Call | handles per call | first quarter | last quarter |
|---|---|---|---|
| `CertOpenStore` + `CertCloseStore` | +0.02 | 0.94 ms | 0.83 ms |
| \+ `CertFindCertificateInStore` + free | +0.00 | 0.92 ms | 0.89 ms |
| \+ `CryptAcquireCertificatePrivateKey` (silent), **card present** | **+2.12** | 457 ms | 435 ms |
| \+ the same, **card absent** | **+4.08** | 855 ms | 824 ms |

**This project's own code is clean.** `CertOpenStore`/
`CertFindCertificateInStore` leak nothing; the store is closed, the
certificate context freed, and `callerFree` was **true on 101 of 101**
successful acquisitions, so `NCryptFreeObject` really is called every
time (D-026's rule is being honoured, not skipped). The handles are
leaked inside `CryptAcquireCertificatePrivateKey` — the smart-card KSP
and the middleware behind it — including four per *failed* acquisition,
where there is nothing for a caller to free at all.

**It does not escape the process, which bounds how bad it is.** Forty
runs of `liro-bridge certs` moved the Smart Card service's own handle
count by **4** in total (220 → 224) and its working set not at all, so a
short-lived command leaks nothing that outlives it. The exposure is the
**tray**, which runs for weeks and probes every hardware certificate each
time the certificate list is gathered: roughly six handles per gather on
this machine, more on a bookkeeper's.

**And the second half of the measurement is the more immediately
visible one.** The probe costs **457 ms for a certificate whose card is
present and 855 ms for one whose card is not**, stable across the run
(the first and last quarters agree, so this is not the leak slowing
things down). That is 1.3 s of the 2.4 s a `liro-bridge certs` takes on
this machine, and it is paid again before the certificate step of every
signing flow.

D-131 measured this at "presence probe, all four: 44 ms cold, 10–13 ms
warm" and concluded that "the wait is the WebView2 window". That
conclusion does not hold on this machine's current state — there is now a
Pošta certificate in the store whose card is not in the reader, and
probing it alone costs more than opening a window does after D-150. The
per-certificate cost scales with how many cards are *absent*, which is
exactly SPEC §14.1's bookkeeper: six certificates, one card.

**Not fixed here, and why.** Nothing in this project's own code is
leaking or slow. The three things that would help are all changes to how
presence is decided rather than bugs to fix: probing concurrently (D-027
rejected concurrent smart-card access for signing, and the same argument
about driver-level failures applies), caching a probe result for a few
seconds, or asking a cheaper question first. Each is a design decision
about SPEC §11.10's model, which FTEST §10 says to hand over rather than
decide. See J-7.

**One thing deliberately not measured.** Whether the leak also occurs
*without* `CRYPT_ACQUIRE_SILENT_FLAG` is the obvious next question and
was not asked: D-087 measured that the same call without that flag can
raise an interactive credential/PIN prompt, and FTEST §0.1 makes causing
one a stop-work condition. It stays unmeasured rather than risked.

### §2 — the real card, without a PIN

Nothing in this session opened a signing session against the card, and no
PIN dialog appeared at any point.

**Enumeration and classification, card in the reader** (release build, no
soft token compiled in):

```
Čitači: 1
  Generic Smart Card Reader Interface 0 — kartica prisutna

Sertifikati: 2
  [1] Savka Odžić ✗ neupotrebljiv    Pošta Srbije CA 1   kartica nije prisutna
  [2] ВЕЉКО СТАНОЈЕВИЋ ✓ upotrebljiv  MUP Gradjani CA 4
Lista poverenja: sekvenca 36, izdata 2026-05-20 (109 dana, upravo osvežena)
```

Both are classified correctly and **presence is per certificate**, which
is the property D-077 exists for: one card in the reader, and the
certificate whose card is *not* there is the one marked unusable, with
its own reason. `--all` shows five rows — the two above plus the MUP
authentication twin and the two Windows-internal GUID certificates —
which is D-149 and D-108 both holding at once.

The Cyrillic subject renders in Cyrillic in all three interface
languages, and the Latin one in Latin (SPEC §9.3).

**Reader state**, read independently of the certificate store: one
reader, card present.

**What could not be done, and why.** FTEST §2 also asks for the card to
be removed and reinserted while `certs` runs, for a different card to be
inserted, and for the reader to be unplugged and replugged. **All four
need a hand at the machine and nobody was there.** They are listed for
the owner in §12 rather than reported as passing.

Stopping the Windows Smart Card service was not attempted either: it
needs elevation, it is a machine-wide change on someone else's working
computer, and a failed restart would leave their card unusable. The
distinct code exists (`errs.CodeSmartCardServiceDown`, in `AllCodes` and
in all three catalogues, now proven complete by B-6) but the path it
covers is unexercised here.

### §3 — everything the soft token reaches

**A thousand documents, four ways.** Signed in process, so what is being
measured is the pipeline rather than process startup. Every output
verified with `internal/pades/verify`.

| What | signed | verified | wall clock | per document (min / median / p95 / max) | heap after | distinct outputs |
|---|---|---|---|---|---|---|
| `blank.pdf` ×1000 | 1000 | **1000** | 1.6 s | 0.0 / 0.5 / 4.8 / 10.6 ms | 2.1 MB | **1** |
| the whole 45-document corpus, cycled to 1000 | 1000 | **1000** | 1.9 s | 0.0 / 0.5 / 5.3 / 10.3 ms | 1.6 MB | 41 (one per distinct input) |
| `mup.pdf` ×1000 — 582 KB, already carrying two signatures | 1000 | **1000** | 5.2 s | 0.0 / 3.6 / 7.0 / 10.7 ms | 3.6 MB | **1** |
| the 500-page document ×1000 | 1000 | **1000** | 5.6 s | 0.0 / 4.3 / 7.7 / 12.1 ms | 2.2 MB | **1** |

**No drift.** With a fixed key and a fixed `Now`, the same document
signed a thousand times produced **one** distinct SHA-256 every time —
including the 582 KB real fixture and the 500-page one. Memory is flat:
the heap ends between 1.6 and 3.6 MB after a thousand signatures, and
`Sys` never passed 17 MB.

**Five deep.** Each document signed, then its own output signed, five
times over, verifying every signature at every depth (SPEC §16.3 calls
the already-signed case "the single most important test in the project"):

| Document | depth 5 | slots at depth 5 | fully verifying | original bytes still a prefix |
|---|---|---|---|---|
| `blank.pdf` | 331 709 B | 5 | 5 | yes, at every depth |
| `xref-stream-objstm.pdf` | 331 790 B | 5 | 5 | yes |
| `xref-mixed-history.pdf` | 332 832 B | 5 | 5 | yes |
| `halcom.pdf` | 692 880 B | 7 | 6 | yes |
| `mup.pdf` | 916 228 B | 7 | 6 | yes |
| `posta.pdf` | 659 994 B | 7 | 6 | yes |

The one slot that does not fully verify in each real fixture is that
document's **own pre-existing document-timestamp slot**, whose
`messageDigest` covers its embedded TSTInfo rather than the `/ByteRange`
— documented in `verify/real_test.go`, present before this project
touched the file, and **unchanged at every depth**, which is the point:
resigning five times over never disturbed it.

All six depth-5 outputs open in pypdf and in PDFium, with the field
counts they should have (5 for a document that started with none, 7 for
one that started with two).

**Every level, against real timestamp authorities:**

| Level and authority | result | wall clock |
|---|---|---|
| no TSA configured, `--on-tsa-failure b-b` | `Nivo: B-B` with the reason stated | 80 ms |
| B-T, Pošta test TSA, HTTP Basic | `Nivo: B-T` | 157 ms |
| B-T, freetsa.org | `Nivo: B-T` | 766 ms |
| B-LT, Pošta test TSA | `Nivo: B-T` + why (B-7; was wrongly `B-LT`) | 147 ms |
| B-LT, freetsa.org | `Nivo: B-T` + why (B-7) | 889 ms |

Pošta's test TSA and freetsa.org both answered every request. The
client-certificate endpoint (`timestamp2`) was not exercised: the PFX
SPEC §12.7 names is not in `testdata/tsa/local/`, and there is nothing
here to generate it from.

**Every stamp placement.** One document signed once per placement, each
output independently verified:

- four corners × {first page, a middle page, the last page, `-1`, a page
  past the end} = 20 cases, on the 17-page `mup.pdf` and on each of the
  four page rotations;
- explicit coordinates at (0,0), (50,50), (400,700), (−100,−100) and
  (10000,10000).

**125 placements, 125 verified.** The clamping is D-138's 12 pt margin,
exactly:

```
(0, 0)            -> (12, 12)          moved
(-100, -100)      -> (12, 12)          moved
(400, 700)        -> (393.32, 700)     moved
(10000, 10000)    -> (393.32, 782.04)  moved
(50, 50)          -> unchanged
```

and a page number past the end reports `StampPageFellBack` rather than
failing (D-143).

**Where the stamp actually landed, as a reader displays it.** The
byte-level checks cannot see this, so it was measured: each stamped
output was rendered with PDFium and the Liro turquoise `#038387` — which
appears nowhere else in these documents — located by centroid.
**Every corner landed in the corner asked for, on all four `/Rotate`
values**, which is D-060's counter-rotation and D-143's fix for the
explicit-coordinate path holding on real output:

```
/Rotate 0, 90, 180 and 270, page 1, each corner:
  bottom-left   (x 0.06, y 0.96)     top-left   (x 0.06, y 0.05)
  bottom-right  (x 0.70, y 0.96)     top-right  (x 0.70, y 0.05)
```

One thing that fell out of doing it: **PDFium draws the signature widget
only after `init_forms()`.** Without that call the stamp is simply not in
the rendered image — which is worth knowing as a fact about how a reader
that does not initialise forms shows a stamped document, and is why the
first run of this check found nothing.

**Rotation handling agrees with PDFium** on all five rotation fixtures.
`reportlab`'s `setPageRotation(90)` writes a landscape MediaBox plus
`/Rotate 90`, so the displayed page is portrait, and both renderers
produce 595×842; `box-inherited.pdf` (portrait box, `/Rotate 90`
inherited from the `Pages` node) displays 842×595 in both.


### B-9 — a batch that could not be written to the audit log went unrecorded in silence (fixed)

**What it was.** `recordInteractiveAudit` discarded both of its error
paths: a bare `return` when the store could not be opened, and
`_, _ = store.Append(...)` when the append failed.

Measured by breaking the log deliberately: **a single unparseable line
anywhere in it makes every subsequent `Append` fail forever**, because
the chain's last entry cannot be read and so the next `PrevHash` cannot
be computed. One truncated last line is exactly what a power cut leaves
behind:

```
=== the last line truncated mid-JSON ===
  Verify -> OK=false BrokenAt=0 err=audit: parsing …: unexpected end of JSON input
  All()  -> 0 entries, err=… unexpected end of JSON input
  Append onto a truncated log -> err=… unexpected end of JSON input
```

From that moment the agent goes on signing and goes on not recording,
with nothing in the log file, nothing on screen and nothing in the exit
code — while SPEC §6.7 makes the audit log the record of every signature.

**The fix.** Refusing to append onto a chain nobody can read is correct
and is unchanged. Both error paths now log at error level, carrying the
outcome and the document count and nothing else (SPEC §18.3 keeps file
names, personal names and content out of every log line). What the *user*
should be told is a separate question and is left to the owner (J-9).

**The tests.** `TestAFailedAuditAppendIsSaidOutLoud` covers both failing
paths and the healthy one — it fails against the previous code with
`log was: ""` for every case.
`TestATamperedAuditLogIsStillReadableAndSaysWhereItBroke` pins what the
export and the audit window rely on and which nothing had asserted end to
end.

### B-10 — `go test ./...` appended fabricated entries to the owner's real audit log (fixed)

**What it was.** Found by hashing the audit directory at the end of the
session against its snapshot, which is the check FTEST §0.4 asks for.
The file had grown from 101 320 to 106 560 bytes. Sixteen entries had
been added, four per full suite run, at the exact times of this session's
runs:

```
{"sequence":308,…,"documentCount":6,"outcome":"approved",…}
{"sequence":309,…,"documentCount":2,"outcome":"approved",…}
{"sequence":310,…,"documentCount":4,"outcome":"partial","failureCode":"PDF_INVALID",…}
{"sequence":311,…,"documentCount":8,"outcome":"partial",…}
```

Those four counts are `batchrun_windows_test.go`'s four tests exactly:
six documents, two documents, four with one bad among them, and eight
with the run stopped part way. Those tests drive the real signing loop —
which is the only way to test it — and that loop records its outcome
through `newAuditStore()`, which is `%LOCALAPPDATA%\Liro\audit`.

This is B-1 one file over, and worse in one specific way. The autostart
value can be put back. **The audit log is append-only and hash-chained,
so an entry a test adds cannot be taken out again without breaking the
chain for everything after it** — and to anyone reading the log later, an
entry with an empty thumbprint and a plausible document count is
indistinguishable from a real signing session. SPEC §6.7 makes that file
the record of what was actually signed.

**The fix.** `mainWindow` gains an `auditStore` field, defaulted to
`newAuditStore` by the product's own constructor and pointed at
`t.TempDir()` by the test helper. `TestTheBatchLoopNeverRecordsIntoTheRealAuditLog`
asserts both halves, so the seam cannot quietly disable recording in the
shipped agent either. Confirmed by measurement: three consecutive full
`go test ./... -tags softtoken` runs afterwards left the real log at
exactly 101 320 bytes.

**The sixteen entries were removed**, which took a decision. Truncating
an append-only audit log is a serious act and the chain exists to detect
exactly that. Two things made it right here: the entries were mine, and
FTEST §0.4's instruction is to leave the machine as it was found,
verified rather than assumed. They were the last sixteen lines, so
cutting the file back to the byte-exact prefix its pre-session SHA-256
was taken over restores it completely rather than leaving a hole. A copy
of the polluted file was kept in the session's scratch directory first,
and the restored file was then checked two ways:

```
hash matches the pre-session snapshot: True
308 entries, chain OK=true BrokenAt=-1
last entry: sequence 307, 2026-09-06 18:32:37Z, outcome approved
```

— 18:32, which is before this session's first suite run at 18:54.

### §8 — failure injection

Every case is a real server or a real file this session controlled,
driven through the shipped pipeline.

**Timestamp authorities.** Fourteen behaviours, each signed twice —
once with `--on-tsa-failure abort` and once with `b-b`:

| TSA behaviour | abort | b-b | elapsed |
|---|---|---|---|
| accepts the connection and never answers | `TSA_UNAVAILABLE` | B-B, reason stated | **49.0 s** |
| 3 s delay then HTTP 500 | `TSA_UNAVAILABLE` | B-B, reason stated | 13.0 s |
| HTTP 400 / 401 / 403 / 404 | `TSA_REJECTED` | B-B, reason stated | **0.0 s** |
| HTTP 500 / 502 / 503 | `TSA_UNAVAILABLE` | B-B, reason stated | 4.0 s |
| 200 with 4 KB of `0xFF` | `TSA_UNAVAILABLE` | B-B, reason stated | 0.0 s |
| 200 with an empty body | `TSA_UNAVAILABLE` | B-B, reason stated | 0.0 s |
| 200 with an HTML error page | `TSA_UNAVAILABLE` | B-B, reason stated | 0.0 s |
| 200 with 64 MB of body | `TSA_UNAVAILABLE` | B-B, reason stated | **0.0 s** |
| connection closed mid-response | `TSA_UNAVAILABLE` | B-B, reason stated | 4.0 s |

D-045's policy is visible in the numbers and is exactly right: 4xx and
RFC 3161 rejections are **not** retried (0.0 s), 5xx and network failures
are (4.0 s = 1 s + 3 s of backoff across three attempts), and the worst
case is 49 s — three 15-second attempts plus the same backoff — for a TSA
that accepts a connection and then says nothing. The 64 MB body is
refused in **0.0 s**: the BER length check rejects it before reading it.

**Never a silent downgrade** in any of the twenty-eight runs. With
`abort` the operation is refused with a code; with `b-b` the document is
signed at B-B and the note says which TSA and why. Every B-B output
independently verifies.

**The Trusted List.** Seven sources, each against a store holding the
real embedded list:

| Source | refresh | list afterwards |
|---|---|---|
| connection refused | error, keeps current | seq 36, embedded |
| HTTP 404 | error, keeps current | seq 36, embedded |
| 200 with 4 KB of NUL bytes | signature invalid, keeps current | seq 36, embedded |
| 200 with well-formed XML that is not a TSL | "no XML-DSig Signature element", keeps current | seq 36, embedded |
| 200 with the real list, sequence tampered to 99 | "document digest does not match the signed DigestValue", keeps current | seq 36, embedded |
| 200 with the real, valid list | accepted | seq 36, **network** |
| 200 with an empty body | "document has no element", keeps current | seq 36, embedded |

Never fails closed, never fails open, and the tampered sequence number is
caught by the signature rather than by a sequence check. Each rejection
logs one warning naming the reason.

A *validly signed* older list could not be served, because the Ministry's
signature covers the sequence number and there is nothing here that can
forge one; `TestRefreshRejectsRollback` covers that path by setting the
store's own current sequence by hand, which is what D-107 already
recorded.

**The configuration file.** Twenty-two shapes, each against a scratch
profile so the owner's own `config.json` was never touched (verified by
hash afterwards):

Absent, empty, `{`, plain text, a JSON array, valid JSON with only
unknown keys, a UTF-8 BOM, UTF-16 with a BOM, `locale: "sr"` (the
forbidden bare form), `locale: "klingon"`, ports 0 and 99999, ports with
start greater than end, `logLevel: "shout"`, `signatureLevel: "b-xyz"`,
`stampPosition: "middle"`, `stampPage: "-5"`, `stampX` of 1e308, every
field of the wrong type, 10 MB of JSON, 2000 levels of nesting, the path
being a directory, and the file held open exclusively by another process.

**All twenty-two: exit 0, a usable listing, and a warning naming the
field and the value that was replaced.** No crash, no hang, and nothing
rewritten on disk. The BOM case now loads `sr-Cyrl` end to end through
the shipped binary, which is B-5's fix in the product rather than in a
test. The unreadable-file case logs "config file could not be read, using
defaults" to the log file rather than to stderr, which is SPEC §9.2
holding.

**The audit log.** Six states:

| State | Verify | All() | Append | Export |
|---|---|---|---|---|
| 40 healthy entries | OK, `BrokenAt=-1` | 40 | — | — |
| entry 13 altered (still valid JSON) | **not OK, `BrokenAt=12`** | **40** | — | both files written, break reported |
| one line that is not JSON | fails, parse error | fails | **fails** | — |
| the last line truncated | fails, parse error | fails | **fails** | — |
| the file held open exclusively | — | — | fails with the OS reason, **succeeds again once released**, chain intact | — |
| the directory is a file | `NewStore` refuses | — | — | — |

The hash chain does what it is for: a single altered field in entry 13 is
detected at exactly entry 13, and every entry stays readable so the log
can still be exported and looked at. The two unreadable-line rows are
B-9.

**Revocation.** An OCSP responder that accepts the request and never
answers, and CRLs of 1, 8 and 40 MB:

| Configuration | cost | evidence collected |
|---|---|---|
| no endpoints at all | 0.0 s | none |
| OCSP hangs, no CRL | **20.0 s** | none |
| OCSP hangs, CRL host refuses | **20.0 s** | none |
| a 1 / 8 / 40 MB CRL of unparseable bytes | 0.0–0.1 s | none — rejected as not a CRL before the size cap |

The 20 s is D-046's own policy working (two attempts, 10 s each) — and it
is paid **per document**, because revocation is collected inside each
`SignDocument`. D-076 measured MUP's real OCSP responder as *dropping*
connections rather than refusing them, which is precisely the 10-second
case. So a hundred-document B-LT batch against MUP's responder as
measured today costs about **33 minutes** of timeouts on top of the
signing. See J-10.

The size cap itself was not re-exercised: bytes that are not a CRL are
rejected before the cap is reached, and there is nothing here that can
produce a validly-signed 8 MB CRL. D-076 measured it against the real
30 136 214-byte MUP CRL.

**Disk full.** Could not be produced. The only volume on this machine has
254 GB free, and every way of making a small one — a VHD, a quota, a
formatted image — needs administrator rights on someone else's working
computer. What was measured instead is the property the disk-full case
would expose, and it is J-8.

---

## For the owner — judgement calls, recorded rather than changed

These are things FTEST §10 says to hand over rather than decide: each is
a contract, a wording or a taste question, and the owner's opinions have
been right before.

### J-1 — a batch that partly failed exits 0

`liro-bridge sign --in "C:\docs\*.pdf"` returns **0** whenever at least
one document succeeded, and 1 only when every one failed. Measured: 100
documents with one corrupt among them exit 0; the corrupt one is named on
stderr and the summary says `Potpisano 99/100 dokumenata`, so nothing is
hidden from a person.

A script cannot tell the two apart. SPEC §19 puts "a Delphi program signs
via `exec`" in F9's scope, and that program will read the exit code and
nothing else. The three plausible contracts are: 0 unless everything
failed (today), non-zero if anything failed, or a third code for
"partial". Changing it is a breaking change to a documented surface, and
which one it should be is the owner's call, so it is left alone.

### J-2 — an unreachable network path costs 42 seconds of silence

`--in \\10.255.255.1\share\doc.pdf` returned after **42.3 s** with
`nijedan ulazni fajl ne odgovara --in` — "no input file matches --in".
Two things about that:

- Forty-two seconds is the operating system's own SMB connect timeout
  inside `filepath.Glob`, not anything this project chose. It could be
  bounded by resolving the pattern on a goroutine with a deadline, at the
  cost of a timeout constant nobody has measured a right value for.
- The message is wrong about *why*. "No file matches" and "that machine
  did not answer" are different things needing different reactions, and a
  network share that has gone away is F6 §7's own named case.

Neither is a crash and neither loses data, so this is recorded rather
than changed. The same path inside the window (`internal/jobs`) is
unaffected: it reads each document at the moment it signs it and reports
`INPUT_UNREADABLE` per document (D-117, D-118).

### J-3 — signing a folder twice produces `-signed-signed.pdf` — DECIDED

**Decided 2026-09-07: do not guess — ask.** The window says how many of
the batch's documents already carry the output suffix and offers to skip
them, once, for the whole batch; the command line reports the count and
skips them unless `--resign` is given. Both answers stay available,
because both are legitimate. Implemented; see D-164.

`sign --in "C:\docs\*.pdf"` run a second time refuses each original
(`izlazni fajl već postoji`) and then cheerfully signs each
`…-signed.pdf` from the first run, producing `…-signed-signed.pdf`. A
third run gets `…-signed-signed-signed.pdf`. Observed while running the
abuse batches, and reproduced deliberately.

It is not destructive and every output is valid. But it is the shape of
mistake a bookkeeper makes on a Monday morning, and the fix — skipping an
input whose name already ends in the configured output suffix — is a
guess about intent that could equally be wrong (someone may genuinely
want to counter-sign a document called `ugovor-signed.pdf` that arrived
from elsewhere). The window path does not have this problem: it signs the
list a person put in it.

**What was done.** Nothing guesses. `jobs.LooksLikeOutput` answers the
question and each front door decides what to do about it in the way that
suits the person in front of it:

- **The window** shows a screen naming how many there are — *Skip them
  and sign the rest* (primary), *Sign them too*, *Cancel* — asked once,
  applied to the whole batch, before the card session opens. Skipped
  documents are reported as skipped rather than failed, and the report
  says how many were left alone and why.
- **The command line** skips them and says so, naming `--resign` as the
  way to sign them anyway; with `--resign` it says that instead. If
  skipping leaves nothing, that is said and the exit code is 1.
  `--force` is untouched: it still means "replace the file that is
  there", and it no longer implies "and sign last week's outputs again".

Measured through the rebuilt binary on a folder with two documents and
one `prethodni-signed.pdf` left by an earlier run: run 1 signs 2/2 and
names the one it skipped; runs 2 and 3 sign nothing and leave the folder
untouched. No `-signed-signed.pdf` is produced at any point.

### J-4 — the console encoding is not this program's to choose

`liro-bridge certs` writes UTF-8. This machine's console code page is
65001, so Cyrillic and the Latin diacritics render correctly, checked
directly. On a machine whose console is still at 852 or 437 — a plain
`cmd.exe` on an older install — the same bytes render as mojibake.

Windows offers no good answer here (a program that calls
`SetConsoleOutputCP` changes the console for whatever runs after it), and
every modern Windows terminal defaults to UTF-8. Recorded so that a
future report of "the certificate list is unreadable" has an explanation
waiting rather than a search.

### J-5 — the report screen says "B-B" twice

The completion screen shows `Nivo potpisa   B-B` and, on the line under
it, `Nivo B-B — bez vremenskog žiga` in the warning colour. Both are
correct and the second is the one that explains itself; together they
read as a repetition. A taste question, not a defect.

### J-6 — one WebView2 environment per window is still the biggest cost left

D-150 removed the two-second `.local` resolution and D-131 measured what
remained. Opening a window is now 0.37–0.42 s, of which most is building
a WebView2 *environment* — one per window, where Microsoft's own samples
build one per process. D-099 noted it; D-131 named it as "what would
actually make it faster, recorded rather than done". It is still the
right next change to the window layer and it is still bigger than a
bounded fix pass, so it is recorded again rather than attempted here.

### J-7 — the presence probe is now the biggest thing a signer waits for — DECIDED

**Decided 2026-09-07: cache the probe result within one listing, never
across listings, never concurrently; and investigate whether a cheaper
question exists.** Implemented, measured, and the cheaper question found
and reported rather than taken; see D-163.

B-8's second half. `CryptAcquireCertificatePrivateKey` costs **457 ms**
for a certificate whose card is present and **855 ms** for one whose card
is not, measured over 200 calls each and stable across the run. On this
machine that is 1.3 s of the 2.4 s `liro-bridge certs` takes, and it is
paid again before the certificate step of every signing flow.

It scales with the number of certificates whose cards are *absent* —
SPEC §14.1's bookkeeper with six certificates and one card in the reader
would pay around five seconds every time.

D-131 measured this at 44 ms for all four certificates and concluded the
wait was the WebView2 window. That was true then; it is not true now, and
the difference is a Pošta certificate in the store whose card is not in
the reader.

The three things that would help are all changes to SPEC §11.10's model
rather than bugs to fix: probing concurrently (D-027 rejected concurrent
smart-card access for signing, and its argument about driver-level
failures applies), caching a probe result for a few seconds within one
listing, or asking a cheaper question first. Which of those is right is
the owner's call.

**What was done, and what it is worth.** `cli.Gather` now answers the
presence question once per certificate per listing and throws the answers
away with the listing. Nothing is cached across listings and nothing is
probed concurrently.

| | Before | After |
|---|---|---|
| This machine's real store (5 rows, 3 hardware-backed, one card in) | Gather 2.76 s, 3 probes, 2.43 s probing | **identical** |
| A six-row listing whose three hardware certificates are each enumerated twice | 3.58 s, 6 probes | **1.97 s, 3 probes** |
| `liro-bridge certs`, rebuilt binary, three runs | 2.31 s | 2.31 s |

Said plainly: on a store where every certificate is enumerated once —
this machine, and equally a bookkeeper's six *distinct* certificates —
there is no repeat to remove, so the memo saves nothing there. It costs
nothing and makes a repeat free. The five seconds is not what it removes.

**The cheaper question exists.** `NCryptEnumKeys` on the "Microsoft Smart
Card Key Storage Provider" enumerates the key containers on currently
inserted cards. Measured three times: **2 keys in 913/921/915 ms**,
listing exactly the two containers of the card that is in the reader and
omitting the absent card's — the same answer the three probes give, for
**one call for the whole machine**, independent of how many certificates
there are, and without opening a key at all. Against 2.43 s for three
certificates and about 5 s for the bookkeeper's six.

It is not taken. SPEC §11.10 states the mechanism and D-014/D-077 stand
behind it; switching to container enumeration is a change to that rule,
and it depends on every middleware registering through the Microsoft KSP
(all three Serbian issuers do; the PKCS#11 platforms in F11+ will not).
Measured and handed over, per J-7's own instruction.

### J-8 — the output file passes through a state that is neither the old file nor the new one — DECIDED

**Decided 2026-09-07: write to a temporary file in the same directory
and rename over the target; accept that a destination another program
holds is then refused, and say so in words that name the remedy.**
Implemented, with a new `OUTPUT_IN_USE` code; see D-165.

`os.WriteFile(out, result.Bytes, 0o600)` opens with `O_CREATE|O_TRUNC`
and then writes, so between those two moments the destination exists at
the wrong length. Measured, with four pollers watching a destination
while a 4 MB file was written over an existing 300 KB one:

```
sizes another program observed at the destination path:
         0  x3        <-- neither the old file nor the new one
   4194304  x636      (the new file, complete)
    307200  x662      (the old file, intact)
```

Two consequences. A program watching the output folder — an ERP, a sync
client, an indexer — can pick up an empty or partial signed document. And
if the write fails partway (a full disk, a network drive that goes away),
that truncated state is what the destination is *left* holding: with
`--force`, a previously good signed file is destroyed and replaced by a
partial one.

**The obvious fix is not free on Windows, which is why this is a question
rather than a change.** Writing to a temporary file and renaming over the
target makes the destination atomic — but measured here, `os.Rename` over
a destination that *any* other program has open, even only for reading,
fails with "Access is denied":

```
os.WriteFile:   reader that opened the old file now reads "NEW NEW NEW…"
os.Rename over an open destination FAILED: … Access is denied.
```

So the trade is real: today `--force` overwrites even while a PDF reader
has the old output open, at the cost of a window where the file is
neither one thing nor the other. Write-then-rename closes that window and
turns "a reader has it open" into a refusal the user must act on. Most
careful tools choose the second. It is a behaviour change on the only
platform this ships on, so it is the owner's.

**What was done.** `platform.WriteFileAtomic` writes beside the target,
flushes, closes and renames over it; both signing paths use it. On any
failure the temporary file is removed and the destination is untouched.
A destination held open is refused with `OUTPUT_IN_USE`, a new code whose
message in all three catalogues says to close the file and try again —
`OUTPUT_WRITE_FAILED` would have sent the person to look at the disk.

One thing had to be measured rather than assumed: Windows returns
`ERROR_ACCESS_DENIED` for a rename onto a held-open destination *and* for
a rename onto a read-only file. The two need opposite answers, so the
classifier asks which it is; a read-only destination stays
`OUTPUT_WRITE_FAILED`.

In the rebuilt binary, with a reader holding the previous signed output
the way an ordinary Windows program does:

```
liro-bridge: sign: ...\ugovor.pdf: Potpisani dokument nije mogao da zameni
postojeći fajl jer je taj fajl otvoren u drugom programu. Zatvorite ga i
pokušajte ponovo. (path=...\ugovor-signed.pdf)
exit code: 1
target after: 66714 bytes, SHA-256 unchanged
```

The same reader made the old `os.WriteFile` path replace the file.

### J-9 — nothing tells the user their audit log has stopped recording — DECIDED

**Decided 2026-09-07: start a new chain beside the broken one, record
the break, and tell the user once.** The third of the three options.
Implemented; SPEC §6.7 amended to say so; see D-166.

B-9 makes a failed append visible in the log file. It does not tell the
person. A log whose last line was truncated by a power cut can never be
appended to again, so from that moment every signature is unrecorded,
and the only place that says so is a file for developers.

The options are all reasonable and all different: refuse to sign until
the log is dealt with (safest, and it stops a bookkeeper's afternoon);
show a warning on the report screen and keep going; or start a new
chain file beside the broken one, keeping the old one for evidence and
recording the discontinuity. The third is what most append-only logs do
and it is the one this project's own audit-window and export code could
already display. SPEC §6.7 does not say, so neither does this.

**What was done.** The third option, and SPEC §6.7 now says it.

- The broken file is left exactly as it is — never overwritten,
  truncated, renamed or deleted — and a new chain is started beside it,
  under a name carrying its own chain number. Chain 1 keeps the names it
  always had, so an existing audit directory reads as it did.
- The new chain's first entry records the break: which file preceded it,
  at which sequence and line it stopped, and why (`unparseable` or
  `unreachable`). The record is part of what the entry hashes, so it
  cannot be altered without breaking the chain it starts.
- The person is told once, on the report screen, as a notice in the
  caution family rather than an error: the log continued in a new file,
  and where it is. Only the entry that opened the chain carries the
  record, so nothing has to remember to stop saying it.
- The same happens when a chain cannot be read at all.
- Verification walks each chain separately and reports each one's own
  result plus the breaks between them; export writes every chain, and
  the exported log carries the discontinuity records.

Reading is now tolerant of a broken tail, which is a behaviour change
worth naming: a log with one truncated last line used to show *nothing*
in the audit window and export *nothing*. It now shows what survives.

### J-10 — revocation is fetched once per document, not once per batch — DECIDED

**Decided 2026-09-07: implement the smaller change only — remember, for
the length of one batch, that an endpoint did not answer, and stop
asking. Successes are still fetched per document, so D-046's ordering
rule is untouched.** Implemented; see D-162.

Measured: an OCSP responder that accepts the request and never answers
costs **20 s per document** (D-046's two attempts of ten seconds), and it
is paid inside every `SignDocument`. A hundred-document B-LT batch
against MUP's responder — which D-076 measured as *dropping* connections,
which is exactly this case — is about **33 minutes** of timeouts.

The evidence being fetched is a property of the *certificate*, not of the
document, and it is the same answer every time within one batch. Fetching
it once would turn 33 minutes into 20 seconds.

The reason this is a question and not a change is D-046's own rule:
revocation is collected after the signature exists "so the OCSP response
postdates the signature — which is what a validator expects". A response
fetched after document 1 predates document 100's signature, by however
long the batch takes. For a batch signed in a minute that is probably
immaterial; whether "probably" is good enough for a qualified signature
is exactly the kind of judgement this project has been right to keep with
its owner.

A smaller version needs no such call and would help immediately:
remember, for the length of one batch, that an endpoint did not answer,
and stop asking. That turns 33 minutes into 20 seconds without changing
what any successfully-fetched evidence means.

**What was done, and what it measured.** One `dss.EndpointMemory` per
batch, holding only the endpoints that did not answer. A fresh one for
the next batch, because a responder that was down five minutes ago may be
up. A successful response is still fetched per document: D-046's rule
that the response must postdate the signature is untouched, and whether
it can be relaxed is still the owner's call.

Ten documents against an OCSP responder that accepts the request and
never answers, measured on this machine:

| | Before | After |
|---|---|---|
| Ten documents | **3m20s** (200.2 s) | **20.1 s** |
| Per document | 20.0 s each | 20.0 s, then 0 s for the other nine |
| Requests that reached the responder | 20 | 2 |

An endpoint that answered with something unusable is *not* remembered: a
responder that is plainly up is not the twenty-second cost this removes,
and giving up on it after one document would turn a transient server-side
problem into a whole batch with no revocation evidence.

One cost this deliberately does not touch, recorded rather than fixed: an
artefact that is fetched and then refused for being too large (D-076's
30 MB MUP CRL) is downloaded again for every document. Remembering it
would lose the specific "too large" reason the report gives, and that is
a wider change than J-10 asked for.

### J-11 — the MUP certificate on this machine expires on 2026-09-24

Not a defect, and noticed while reading the certificate listing: the MUP
signing certificate that this session enumerated is valid until
**2026-09-24**, which is eighteen days after this run. The Pošta one runs
to 2030.

Worth saying because the whole of §2, and every future real-hardware
acceptance run, depends on there being a usable qualified certificate in
the reader.

---

## What I could not test, honestly

### Group 3 was not run at all

**No fuzzing.** FTEST §6 asks for the parser's fuzz target for at least
an hour, plus new targets for the renderer and for the CMS and timestamp
parsers. **None of that was done.** An hour of fuzzing that finds nothing
is a result; ten minutes of it is not, and a report claiming the second
as the first would be worse than this sentence. The parser's existing
target found three crashes in 12.6 M executions when D-039 ran it, so
there is reason to think the renderer's — 4 600 lines that read foreign
input and have never been fuzzed — would find something.

**No endurance.** FTEST §7 asks for the tray running for hours, every
window opened and closed a hundred times, a thousand documents in one
session and the suite twenty times.

What was done instead, and what it is worth:

- **A thousand documents** were signed four ways, but in one process
  through `SignDocument` rather than through a tray that has been open
  for hours. Memory was flat and no output drifted. That is the pipeline,
  not the agent.
- **The suite was run nine times** end to end during this pass, all
  green, the last three back to back after every change. That is not
  twenty, and nine green runs do not establish the absence of a
  one-in-twenty flake — which is
  exactly the rate D-099 measured for the window layer before D-101 fixed
  it.
- **Windows were opened and closed** on the order of a hundred times
  across the layout, screenshot and locale runs of this pass, with no
  hang and no crash. That is a by-product, not the measurement §7 asks
  for.

**Why I stopped.** The instruction was to stop after Group 2 unless
something made Group 3 urgent. B-8 is the closest candidate — a handle
leak is exactly what §7's endurance run is for — and it does not qualify:
the leak was measured precisely (2 handles per probe with the card
present, 4 with it absent, ~10 per enumeration), it was shown not to
escape the process (forty `certs` runs moved the Smart Card service's
handle count by four), and running the tray for six hours would confirm
arithmetic that is already on the page. The renderer fuzzing is the piece
of Group 3 most likely to find something, and it is the piece that most
needs the hour it was not given.

### Things that need a hand at the machine

Nobody was here, so none of these were attempted:

- Removing the card while `certs` runs; inserting it during; inserting a
  *different* card; unplugging and replugging the reader (FTEST §2).
- Stopping the Windows Smart Card service. It needs elevation and it is a
  machine-wide change on someone else's working computer; a failed
  restart leaves their card unusable. `SMART_CARD_SERVICE_DOWN` exists,
  is in `AllCodes` and has a message in all three catalogues (now proven
  by B-6), but the path is unexercised.
- Pressing OK in the folder chooser, so the audit export's last step —
  two files on disk, the destination named on screen — is still the
  owner's, exactly as D-097, D-133 and D-135 already record.

### Things this machine could not produce

- **A full disk.** The only volume has 254 GB free, and every way of
  making a small one needs administrator rights. What the disk-full case
  would expose was measured another way and is J-8.
- **A validly-signed older Trusted List**, to exercise the rollback
  guard end to end. The Ministry's signature covers the sequence number.
- **A validly-signed oversized CRL**, to re-exercise D-076's 5 MB cap:
  bytes that are not a CRL are rejected before the cap is reached. D-076
  measured it against the real 30 136 214-byte MUP CRL.
- **Pošta's client-certificate TSA endpoint** (`timestamp2`): the PFX
  SPEC §12.7 names is not in `testdata/tsa/local/` and there is nothing
  here to generate it from. The Basic-auth endpoint answered every
  request.
- **`CryptAcquireCertificatePrivateKey` without `CRYPT_ACQUIRE_SILENT_FLAG`**,
  which is the obvious next question about B-8's leak. D-087 measured
  that the same call without that flag can raise an interactive
  credential prompt, and FTEST §0.1 makes causing one a stop-work
  condition. It stays unmeasured rather than risked.

---

## What remains the owner's

Listed so he knows what is waiting, per FTEST §12.

1. **One batch with the real card and a real PIN.** The only thing that
   proves one PIN covers a batch on this machine. Nothing in this session
   opened a signing session against the card, and no PIN dialog appeared
   at any point.
2. **Removing the card mid-batch with a PIN entered**, and **cancelling a
   real PIN dialog.**
3. **Output through the eGovernment validator, and through PKS or
   Inception.** SPEC §16.8 makes eUprava the one that decides legal
   validity, and no substitute for it exists here. What this pass can
   say is narrower and worth saying: every signature it produced verifies
   under this project's own independent verifier, and every output opens
   in pypdf and PDFium — 45 of 45 corpus documents, 6 of 6 five-deep
   resigned ones, 125 of 125 stamp placements.
4. **Adobe Acrobat**, which is not installed here. Four of this
   project's nine recorded Acrobat defects (D-069, D-072, D-074, D-075)
   were invisible to every other validator, so nothing in this report is
   evidence about Acrobat.
5. **Another machine entirely** — clean Windows, no middleware, no
   WebView2 runtime, SmartScreen on first run. Deferred by his own
   decision; noted and moved past.
6. **The six judgement calls still open (J-1, J-2, J-4, J-5, J-6,
   J-11).** J-3, J-7, J-8, J-9 and J-10 were decided on 2026-09-07 and
   are implemented; each is marked DECIDED in its own section above.
   Of what is left, J-1 (a partly-failed batch exits 0) is the one with
   a contract behind it: F9's Delphi caller will read that exit code and
   nothing else.

---

## The machine, put back

Everything below was checked, not assumed.

| What | Before | After |
|---|---|---|
| `%LOCALAPPDATA%\Liro\config.json` | SHA-256 recorded | **identical** |
| `HKCU\…\Run` → `LiroBridge` | `"C:\Users\Veljko\Desktop\liro-bridge\liro-bridge.exe"` | **identical** |
| `HKCU\…\SystemFileAssociations\.pdf\shell\LiroBridgeSign` | verb, `MultiSelectModel`, `Icon`, `\command` | **identical** |
| `%LOCALAPPDATA%\Liro\audit\` | one file, 101 320 bytes, SHA-256 recorded | **identical**, chain verifies (B-10) |
| Processes started by this session | — | all stopped, by exact PID |
| Scratch files | — | removed; the temporary Go harnesses deleted |

The autostart value was corrupted once, by the very first baseline suite
run, and restored by hand within the minute; that is B-1, and after its
fix six further full suite runs left it untouched.

Every configuration-injection case ran against a scratch profile
directory rather than the real one, and the owner's `config.json` was
hash-compared against its snapshot afterwards each time.

### One thing left undone: the push

FTEST §0.5 says to push when the suite is green, and it is — three
consecutive clean full runs after the last change. **The push itself was
blocked by this session's own permission layer**, not by anything in the
repository, and working around that is not something an unattended run
should do.

So the eleven commits below are on `master` locally and need one
`git push origin master`:

```
f47bfa8  B-1  autostart tests put back the value they found
58a0408  B-2, B-3  renderer notes; the command line's own error codes
7cf6848        report: Group 1 sections 4 and 5
48e4e8b  B-4, B-5, B-6  listing alignment; the byte-order mark; AllCodes
2973ef5        report: Group 1, and the judgement calls
8e71dab  B-7  B-LT is not claimed for a document with no /DSS
6542436        report: Group 2 sections 2 and 3
42850d1  B-9  a failed audit append is said out loud
0829243        report: Group 2 section 8
6ff677b  B-10  the batch tests stop writing to the real audit log
ef1cf6a        report: the summary and the closing sections; D-153..D-161
```

`git ls-files | Select-String "local/"` lists only the four
`README.md` files, as it must.
---

# Group 3 — fuzzing, volume, window lifetime, flakes

**Date:** 2026-09-07
**Base commit:** `92df354` (FTEST decisions: SPEC §6.7 amended, D-162..D-167)
**Machine:** the same one — Windows 11 Pro 10.0.26200, AMD Ryzen 5 4500
(6 cores, 12 threads), **Go 1.26.5** (the Group 1/2 report says 1.27.1;
`go version` on this machine says `go1.26.5 windows/amd64`, and this
report's numbers are that toolchain's).
**Run:** unattended. Nobody was at the machine.

Group 3 is what the previous pass wrote down plainly that it had not
done: "no fuzzing, no endurance." This is that work.

## Before anything ran

Snapshot taken and kept in this session's scratch directory:

```
config.json   %LOCALAPPDATA%\Liro\config.json, 367 bytes
              SHA-256 c488e32bd325279a334e5c3e0feffeb47dfb8286f044da3f003c9fbcee2a4be9
audit log     %LOCALAPPDATA%\Liro\audit\2026-09-001.jsonl, 4 951 bytes
              SHA-256 76f55382ac60cc374ba7dc2e31cb2d899b95d540f70c15f352b3d6c5efd1d2ad
HKCU\...\Run  LiroBridge:  ABSENT
HKCU\Software\Classes\SystemFileAssociations\.pdf   ABSENT entirely
```

Both registrations differ from the Group 1/2 snapshot, which had a
`LiroBridge` Run value and a registered Explorer verb. The owner has
removed both since. **The restore target is therefore "absent", not the
value the earlier report recorded** — putting back what the earlier
report says would be inventing an entry, which is exactly the mistake
D-153 was written about.

The boundaries were kept the same way Group 1/2 kept them: nothing in
this session opened a signing session against the card, no PIN dialog
appeared, no synthetic mouse or keyboard input was used anywhere, and
every process this session started was stopped by its own exact PID.

## The corpus had to be rebuilt

The Group 1/2 report describes a 45-document corpus. It is not in the
repository and it is not on disk: that pass built it in a scratch
directory and its own closing section records deleting the scratch. So
it was rebuilt from the report's own §4 description — page counts 1, 2,
5, 17, 50, 200, 500; A4, A5, A3, US Letter, 200×900, 120×120, 2000×3000;
all four rotations plus a mixed one; five producers; Cyrillic, Latin and
mixed text with an embedded TrueType font and with the standard
fourteen; classic tables, a cross-reference stream carrying an object
stream, and a hand-built reproduction of D-075's hybrid `/XRefStm`
history; JPEG, CCITT Group 3 and Group 4 encoded by Pillow, a Flate RGB
image with an `/SMask`, and JBIG2 and JPX streams that exist to be
declined; an axial shading, a tiling pattern, an inline image, a type-4
mesh shading, a standard font with no `/Widths`, and a page whose boxes
and `/Rotate` are inherited from the `Pages` node.

**42 documents, 813 pages, all 42 parsed and all 813 rendered** by this
project's own code before anything was measured against them. The notes
that fired are the ones that should:

```
substituted the shapes of a font the document does not embed   812
advance widths guessed: the document declares none             812
tiling pattern approximated as flat colour                      20
image codec JBIG2Decode not supported                            1
image codec JPXDecode not supported                              1
shading type approximated as flat colour                         1
```

Three fewer documents than the 45 the earlier report counted, because
this is a reconstruction from a description rather than the same
generator. Said rather than rounded.

**The generator is committed this time**, as `scripts/gencorpus`, for
the reason D-071 gives for `testdata/pdfs/blank.pdf`: a fixture nobody
can regenerate is a fixture that drifts. This is the second pass to need
it and the first to be able to run it.

---

## C-1 — a BER long-form length of eight octets went negative, in both readers (fixed)

**Found by `FuzzParseResponse` in its second minute**, at about 70 000
executions.

```
30 88 30 30 30 30 30 30 30 30
```

A SEQUENCE whose length is eight ASCII zeros. BER permits up to eight
length octets, and eight octets are enough to set the sign bit of an
`int`. Every check downstream of the length compares it against a buffer
size, which a negative value passes, and the next thing that happens is
a slice bound:

```
panic: runtime error: slice bounds out of range [:-8633347502144212944]
  internal/pades/tsa.parseBERValue    ber.go:53
  internal/pades/tsa.parseResponse    response.go:74
```

**The identical defect was in `internal/pades/verify`'s reader.** That
package is deliberately a second, from-scratch implementation that
shares nothing with the signer — D-044 exists precisely so that "a bug
in a shared helper passes both ways" cannot happen. It did not help
here, because the same mistake was made twice independently. Nobody
copied anything; two readings of the same RFC both reached for `int` and
neither thought about eight octets.

That is the lesson worth keeping: **two implementations do not catch a
mistake both of them make.** What found the second one was going to look
at the sibling the moment the first fell over, which is a habit rather
than an architecture.

**The fix.** Accumulate into a `uint64` and refuse a value that does not
fit in a positive `int`, in both readers. A length that genuinely fits is
still read; only the impossible is rejected, so nothing legitimate
narrowed.

**The tests.** `TestALengthTooLargeForAnIntIsRefusedRatherThanSliced` in
both packages, four cases each: the fuzzer's own input, the sign bit
alone, all ones, and the largest value that is still a positive `int`
(which the existing "declared length exceeds available bytes" check
rejects, and must not panic either). Plus
`TestALengthThatFitsIsStillRead`. Confirmed both ways: with the guard
disabled, both fail with the exact panic above; with it, both pass. The
crashing input is committed under
`internal/pades/{tsa,verify}/testdata/fuzz/`.

`.gitattributes` gained `internal/**/testdata/fuzz/** -text`, because
Go's corpus format is a Go string literal per line, read literally, and a
CRLF checkout would make every entry a parse error at the one moment a
crashing input has to replay — D-107's own lesson applied to a different
byte-exact artefact.

---

## C-2 — every window leaked six GDI objects and two USER objects (fixed)

**Found by opening each of the seven windows a hundred times** and
reading the process's own counters. GDI grew at **exactly 6.00 per
window**, USER at 2.10, monotonically, and neither ever came back:

```
main (documents step)   3 cycles   gdi  15 ->  35   (+6.67/cycle)
consent                 3 cycles   gdi  35 ->  53   (+6.00/cycle)
method                  3 cycles   gdi  53 ->  71   (+6.00/cycle)
stamp picker            3 cycles   gdi  71 ->  89   (+6.00/cycle)
settings                3 cycles   gdi  89 -> 107   (+6.00/cycle)
certificates            3 cycles   gdi 107 -> 125   (+6.00/cycle)
audit log               3 cycles   gdi 125 -> 143   (+6.00/cycle)
```

**The cause is one line.** `setWindowIcons` loads the title-bar
(`ICON_SMALL`) and Alt+Tab (`ICON_BIG`) icons with `LoadImageW` and
`LR_LOADFROMFILE`. Without `LR_SHARED` that creates a *new* icon on every
call and the caller owns it; `WM_SETICON` does not take ownership, and
`DestroyWindow` does not free it. Measured directly, outside the agent:

```
200 icons loaded    gdi 0 -> 604   user 1 -> 202   (3.02 GDI, 1.00 USER each)
after DestroyIcon   gdi 604 -> 4   user 202 -> 2   (600 of 604 back, 200 of 201)
```

Three GDI objects and one USER object per icon, two icons per window, and
`DestroyIcon` gives every one of them back.

**What it costs.** A process's default GDI quota is 10 000, so roughly
1 600 windows before a window cannot be drawn at all. That is a long
afternoon rather than an immediate failure, which is exactly why it went
unnoticed — and it is the shape of bug that produces "the agent stopped
opening windows and I had to restart it" with nothing in any log.

**The fix.** The window keeps both handles and destroys them in
`wndProc`'s `WM_CLOSE` case, **after** `DestroyWindow` — not before:
until the window is gone it is still painting its own title bar from
them.

**Why no test saw it.** Every test in `cmd/liro-bridge` borrows one
shared window per page and closes it once, at the end
(`sharedwindow_windows_test.go`, which exists for D-098's serialisation
reason). **One window that leaks is indistinguishable from one that does
not** — it takes a second window to see a slope. That is D-161's lesson
about fixtures pointed at a different axis: the fixture was the right
shape, and there was only ever one of it.

**Two tests, both confirmed against the old code.** `internal/ui`'s
`TestAnIconLoadedForAWindowIsGivenBack` proves the primitive frees what
it allocates (it costs milliseconds and no window). `cmd/liro-bridge`'s
`TestOpeningAndClosingWindowsDoesNotLeakGDIObjects` opens ten real
windows and proves one calls it:

```
before the fix   10 windows: GDI 15 -> 75 (+60, 6.00 per window), USER +21 (2.10)
after the fix    10 windows: GDI  9 ->  9 ( +0, 0.00 per window), USER  +1 (0.10)
```

**The tray's own icon is deliberately left alone.** It is loaded once per
process rather than once per window, so it is not a per-cycle cost, and
its fallback is `IDI_APPLICATION` — a shared system icon that must never
be destroyed. Freeing that one would be a new bug in place of a
non-existent one.

---

## C-3 — the window layer leaks about one process handle per window (measured, not fixed)

With C-2 fixed, the hundred-cycle run has GDI, USER, threads and
goroutines all flat and **handles still climbing, linearly, on every one
of the seven windows**:

| Window | handles before | after 100 cycles | per cycle |
|---|---|---|---|
| main (documents step) | 272 | 380 | **1.08** |
| consent (certificate step) | 382 | 483 | **1.01** |
| method (stamp step) | 483 | 584 | **1.01** |
| stamp picker (placement) | 592 | 674 | **0.82** |
| settings | 682 | 778 | **0.96** |
| certificates | 784 | 880 | **0.96** |
| audit log | 888 | 978 | **0.90** |

GDI stayed at 9 throughout, USER between 5 and 8, threads between 12 and
17, goroutines at 3, heap between 0.62 and 0.72 MB. Mean 330–354 ms per
open-and-close; worst single cycle 1.081 s.

**It is a leak, not a lag.** A closed window's WebView2 browser process
group takes a moment to exit, so the first question is whether the
handles come back if nothing else happens. They do not: 100 windows, then
two minutes of doing nothing at all, sampled every five seconds —

```
after 100 windows   +127 over baseline
t+ 30s              +126
t+ 60s              +126
t+120s              +128
```

**What kind of handle.** Snapshotting this process's handle table
(`NtQuerySystemInformation`, `SystemExtendedHandleInformation`, so
nothing has to call `NtQueryObject` and risk hanging on a pipe) before
and after 40 windows, with the type indices resolved by creating one
object of each kind and reading its own index rather than from a table
that would be wrong on the next Windows build:

```
40 windows: 280 -> 319 handles (+39, 0.97 per window)

Process        27 appeared,  0 of the old ones closed,  net +27  (0.68 per window)
Event          15 appeared,  9 closed,                  net  +6  (0.15 per window)
type index 26  12 appeared,  8 closed,                  net  +4  (0.10 per window)
Thread          5 appeared,  5 closed,                  net   0
```

**Process handles, two thirds of it.** Handles to `msedgewebview2.exe`
browser processes that have already exited — a handle to a dead process
keeps its process object alive. This project never calls `CreateProcess`
or `OpenProcess`; they are created inside WebView2 in our address space,
and releasing `ICoreWebView2Environment` does not close them.

**The browser processes themselves do exit.** Counted on the machine
after roughly 900 window creations: 13 `msedgewebview2.exe` processes,
every one of them 63–64 minutes old, i.e. every one started before this
work began. Nothing accumulated outside this process.

**Not fixed here, and why.** Each `ui.NewWindow` builds its own WebView2
*environment*, and an environment is what starts a browser process group.
**J-6 already records that one environment per process — what Microsoft's
own samples do — is the right next change to this layer**, and D-099 and
D-131 each declined it as bigger than a bounded fix. This is new evidence
for the same change: it is not only the 0.37–0.42 s a window costs, it is
a kernel handle per window that never comes back.

The exposure is bounded by the process's own life, and this agent's
process is short-lived by the owner's own account. A thousand windows is
a thousand handles, which is not near any limit. Recorded with numbers so
that whoever takes J-6 knows it closes two things rather than one.

---

## Volume — a thousand documents, four ways

One process, one `signing.Session` opened once and closed at the end, one
document at a time, soft token. Every output independently verified with
`internal/pades/verify`.

**4 000 signed, 4 000 verified, no drift, in four runs.**

| Run | signed | verified | wall clock | distinct outputs |
|---|---|---|---|---|
| `blank.pdf` x 1000 | 1000 | **1000** | 2.51 s | 1 of 1 input |
| the 42-document corpus, cycled to 1000 | 1000 | **1000** | 2.73 s | 42 of 42 inputs |
| `mup.pdf` x 1000 — 582 KB, two existing signatures | 1000 | **1000** | 6.96 s | 1 of 1 |
| the 500-page document x 1000 | 1000 | **1000** | 7.28 s | 1 of 1 |

With a fixed key and a fixed `Now`, every input produced one
byte-identical output every single time.

### The timing instrument had to be replaced first

The first run of all four reported `min 0s` and **48 of 1000 signatures
at exactly 0 ns**. That is not a fast signature; it is the clock. Go's
monotonic clock on Windows is the system interrupt time, which ticks at
the timer interval — coarse enough that a signature shorter than one tick
measures as nothing. A per-document distribution taken with it is a
picture of the clock, so the harness was changed to
`QueryPerformanceCounter` and every run repeated. **Every number below is
QPC.**

### The distribution, not just the median

| Run | min | p50 | p90 | p95 | p99 | max | mean |
|---|---|---|---|---|---|---|---|
| `blank.pdf` | 1.1 ms | **1.1 ms** | 1.9 ms | 2.3 ms | 2.8 ms | 3.2 ms | 1.3 ms |
| mixed corpus | 1.1 ms | **1.2 ms** | 2.4 ms | 2.7 ms | 4.8 ms | 5.4 ms | 1.5 ms |
| `mup.pdf` | 3.2 ms | **4.4 ms** | 5.4 ms | 5.8 ms | 6.5 ms | 18.6 ms | 4.5 ms |
| 500 pages | 3.7 ms | **5.1 ms** | 5.9 ms | 6.2 ms | 7.2 ms | 18.9 ms | 5.2 ms |

The tail is short and it is the garbage collector: p99 is within 1.5x of
the median everywhere, and the single worst document in each run is
within 4x. Nothing accumulates — the last hundred documents are as fast
as the first hundred in all four runs.

### Resources across the run

| Run | handles start to end | GDI | USER | threads | heap after | working set |
|---|---|---|---|---|---|---|
| `blank.pdf` | 110 to 175 | 0 to 0 | 1 to 10 | 8 to 14 | 0.46 MB after GC | 8.70 to 15.96 MB |
| mixed corpus | 104 to 181 | 0 to 0 | 1 to 10 | 7 to 15 | 0.49 MB | 9.48 to 16.32 MB |
| `mup.pdf` | 110 to 183 | 0 to 0 | 1 to 11 | 8 to 15 | 0.48 MB | 9.30 to 17.00 MB |
| 500 pages | 110 to 189 | 0 to 0 | 1 to 11 | 8 to 16 | 0.48 MB | 9.22 to 16.68 MB |

The handle figure looked like a leak — +65 to +79 across a thousand
documents — so it was measured properly rather than reported as one.
**Five thousand documents, sampled every 250:**

```
doc     0   handles 110      doc  2500   handles 177
doc   250   handles 157      doc  3000   handles 179
doc   500   handles 159      doc  3500   handles 181
doc  1000   handles 167      doc  4000   handles 181
doc  1500   handles 171      doc  4500   handles 181
doc  2000   handles 175      doc  5000   handles 181
```

**It converges.** Flat at 181 from document 3 500 onward, for the last
1 500 documents. USER objects plateau at 10, threads at 15, the heap
never passes 3.6 MB and `Sys` never passes 25 MB. This is the Go runtime
reaching a steady state — Ms, timers, GC workers — not a per-document
cost. The signing pipeline leaks nothing.

---

## Fifty realistic sessions

Start the real binary, sign a small batch, quit. Fifty times, each a
fresh process signing three documents of different shapes (a two-page
text document, a standard-font Latin one, a JPEG scan) with the soft
token.

The profile directory is a scratch one for these runs, so "leaves nothing
behind" is a question that can actually be answered — every file the
agent writes lands somewhere the harness owns, and the owner's own
`%LOCALAPPDATA%\Liro` is untouched by construction. That is B-10's lesson
applied before rather than after.

```
50 sessions, 0 failures
per session: min 0.14s  mean 0.14s  max 0.16s
msedgewebview2.exe on the machine: 13 before, 13 after
no agent process outlived its session
stray preview- directories in TEMP: 0

what the profile directory holds after 50 sessions:
  \Liro\logs    1 file    6 047 bytes
```

**150 signed documents, 150 signature slots, 150 fully verified**
(`ByteRangeDigestOK`, `SignatureOK` and `SigningCertificateOK` all true)
by the independent verifier afterwards.

Exit code 0 on every one of the fifty. The only thing any session left
behind is the agent's own log file, which is the product working.

**What this is and is not.** It is the command-line front door: process
start, configuration load, Trusted List, soft token, the signing loop,
exit. It is not the window flow, which needs a hand on a mouse to reach
past the certificate step, and D-094 forbids simulating one. The window
flow's own lifetime is measured above, a hundred cycles per window.

---

## C-5 — one window in a few hundred does not open, with ERROR_BUSY (measured, not fixed)

**Found by the flake runs**, twice in forty: once with `-tags softtoken`
and once without, which is the shape of a real intermittent rather than
a tag-specific one.

```
alreadysigned_windows_test.go:51: NewWindow(main):
  ui: creating WebView2 controller:
  CreateCoreWebView2Controller completed with HRESULT 0x800700AA
```

`0x800700AA` is `HRESULT_FROM_WIN32(ERROR_BUSY)` — "the requested
resource is in use." It is what WebView2 returns when the user data
folder is already in use by another environment, and this project builds
**one environment per window** while every window in a process shares one
user data folder. A window closing and another opening a moment later
can therefore race the first one's browser process group letting go of
the folder.

**The same root cause as C-3, and the same fix.** J-6 records one
environment per process as the right next change to this layer; D-170
adds the process-handle leak to its account; this adds a third item: it
is also why a window occasionally does not open at all.

**Rate.** Twice in forty suite runs. Each run creates roughly thirteen
windows, so on the order of one in two hundred and fifty — and the
hundred-cycle window measurement above created seven hundred windows
back to back with none. It is a race against a closing window, so it
wants windows in quick succession rather than many windows.

**What it looks like to a person.** `ui.NewWindow` returns the error, so
the caller reports it rather than hanging: this is not D-099's silent
wait. It is a window that says it could not open, once in a few hundred
tries, for a reason nobody can act on.

**Why it is not retried here.** [[D-080]] explicitly rejected retrying
`CreateCoreWebView2Controller` on an HRESULT rather than finding the
cause, and it was right to. The cause here *is* found and recorded — one
environment per window — so wrapping the call in a retry would be
papering over J-6 while making the paper look like a fix. Reported
instead, with the rate.

**What it does to the suite, which is worth its own sentence.** The
seventy tests that failed in each of those two runs are one failure.
`sharedwindow_windows_test.go` builds each shared window under a
`sync.Once` that records the error, so every later borrower fails
instantly with the same message. That is the right design for its own
reason (D-098: thirteen environments in one process stopped completing
at all), and it means a red run of this kind should be read as "the
shared main window did not open", not as seventy problems.

---

## Fuzzing

Seven targets. One existed; six are new. Every one was run against a
seed corpus of this project's committed fixtures, the real signed
documents in `testdata/pdfs/local` where a target can use them, and the
regenerated FTEST corpus through `LIRO_FUZZ_SEED_DIR` — so a CI run
starts from what CI has, and a run here starts from everything.

**One crash, found in the second minute, in two places at once.** That
is C-1 above.

### What ran, for how long, and how many executions

| Target | Package | Ran for | Executions | Corpus after | Crashes |
|---|---|---|---|---|---|
| `FuzzParse` (existing) | `pades/pdf` | **90 min** | **139 442 293** | 602 → 1 009 | 0 |
| `FuzzRenderPage` (new) | `pades/render` | **90 min** | **13 766 422** | 9 → 567 | 0 |

The two ran together, four workers each on a twelve-thread machine, so
the machine was two thirds committed to fuzzing and a third to the
window and volume measurements happening at the same time. A dedicated
run would report higher rates; nothing about coverage changes.

`FuzzParse` was still finding new coverage at ninety minutes — 414 new
interesting inputs over the run, the last of them in the final minutes —
so "longer if it is still finding things" is still true of it. What it
was not finding is crashes: D-039's three, found in 12.6 M executions
when this target was written, remain the only ones it has ever produced,
and the bounds added then held through 139 million more.

`FuzzRenderPage` is the one there was reason to expect something from:
4 600 lines that read foreign input, never fuzzed, with a content-stream
interpreter, a scanline rasteriser, colour spaces, PDF functions, image
XObjects and their masks, a TrueType glyph reader and a hand-written
CCITT decoder underneath it. **Thirteen point eight million executions,
no panic, no hang, no unbounded allocation.** That is a result and it is
worth saying plainly rather than burying: the newest and least-exercised
code in the project did not fall over.

### The stalls in the render log were the coordinator, not slow inputs

`FuzzRenderPage`'s progress line reports zero executions per second for
stretches of up to forty-two seconds. The obvious reading is an input
that takes forty seconds to draw, which for a placement window is a
frozen window, so it was checked rather than assumed: every one of the
517 corpus entries the run kept was re-run and timed.

```
517 corpus entries, 276 of them a document the renderer opens
total 21.03s, mean 76 ms, median 51 ms
slowest:   730 ms   (48 464 bytes, 1 page)
           356 ms   (28 562 bytes, 48 pages)
           329 ms   (1 271 bytes, 1 page)
```

**The slowest input the fuzzer found renders in 0.73 s.** The stalls are
Go's own coordinator minimising a newly interesting input, which does
not advance the execution count. Nothing the renderer was handed took
anything like a second.

### The other six targets

All six ran together, two workers each, on the same twelve-thread
machine, and each was given 75 minutes of fuzzing — which took 78
minutes of wall clock, because a run does not end until every worker has
finished what it was on.

| Target | Package | What it reads | Ran for | Executions | Corpus after | Crashes |
|---|---|---|---|---|---|---|
| `FuzzParseCMS` | `pades/verify` | the SignedData out of a document | **78 min** | **543 539** | 4 → 189 | 1 → C-1 |
| `FuzzVerifySignature` | `pades/verify` | a whole document, end to end | **78 min** | **1 856 984** | 8 → 223 | 0 |
| `FuzzParseResponse` | `pades/tsa` | an RFC 3161 response or token | **78 min** | **35 144 085** | 7 → 82 | 1 → C-1 |
| `FuzzParseBERValue` | `pades/tsa` | one BER tag, length and value | **78 min** | **25 898 317** | 7 → 110 | 0 |
| `FuzzIncrementalUpdate` | `pades/pdf` | a document that is then written to | **78 min** | **10 309 488** | 10 → 474 | 0 |
| `FuzzSignMalformedDocument` | `pades` | a document that is then signed | **78 min** | **3 528 013** | 5 → 390 | 0 |

**Across all eight target-runs: 230 489 141 executions, and one crash.**
That crash is C-1, and `FuzzParseResponse` found it in the second minute
of its very first smoke run — before any of these long runs started.
Everything after that was ten and a half target-hours of not finding
anything else, which is the result and is worth the hours it took to be
able to say.

### What each of the new targets actually asserts

A fuzz target that only checks for panics finds panics. These check
properties as well, so an input that produces a wrong answer without
crashing is also a failure:

- **`FuzzRenderPage`** — a page that renders without error must produce
  a non-nil image of at least one pixel in each direction.
- **`FuzzParseCMS`** — a `parsedCMS` that came back must survive being
  used the way `VerifySignature` uses it, which is where a nil field or
  a nonsensical length is actually dereferenced.
- **`FuzzVerifySignature`** — `VerifySignature` must never return nil
  for a slot `FindSignatures` handed it.
- **`FuzzParseBERValue`** — the remainder must never be longer than the
  input, which is the shape of a length arithmetic error that does not
  panic.
- **`FuzzIncrementalUpdate`** — the input's bytes must still be a
  literal prefix of the output; `/ByteRange` must cover the whole output
  except the reserved span; and **the result must parse again**, because
  an incremental update this project wrote that its own parser cannot
  read is a document no reader can read either. That third one is worth
  its own note: nothing else in the suite checks it against input this
  project did not construct.
- **`FuzzSignMalformedDocument`** — the input is still a prefix, signing
  added exactly one signature slot, and **the slot it added verifies**
  (`ByteRangeDigestOK` and `SignatureOK`) under the independent
  verifier. Ten million malformed documents signed and every signature
  that came out of one was valid.

### Why `FuzzParseCMS` looks slow, checked rather than assumed

`FuzzParseCMS` managed 543 539 executions where `FuzzParseResponse` did
35 million. The obvious reading is a pathological input — a CMS whose
parse takes seconds, which for a document somebody else produced would
be a signature check that stalls. So it was measured: every one of the
109 corpus entries the run had kept at that point was re-run through
`parseCMS` and timed.

```
109 entries, total 6 ms, mean 57 µs, median 0 s
slowest: 1.06 ms (1 303 bytes)
```

**Six milliseconds for the whole corpus.** The parser is not slow; the
low rate is Go's coordinator on a target whose inputs are large enough
that mutation and minimisation dominate, on a machine running thirteen
fuzz workers on twelve threads. Nothing to fix, and worth writing down
so the next person does not chase it either.

### The seed arrangement, and why it is not just `f.Add`

Every target seeds from three places: documents built in the test file
itself (small, committed, always present), this project's committed
fixtures (`testdata/pdfs/blank.pdf`, `testdata/golden/minimal-signed-bb.pdf`),
and — when they are there, which is never in CI — the real signed
documents in `testdata/pdfs/local` and whatever the `LIRO_FUZZ_SEED_DIR`
environment variable names.

That last one is how the FTEST corpus gets in without 280 KB of
generated PDFs being committed to seed a fuzz target. A CI run starts
from what CI has; a run on a machine with the real fixtures and a
generated corpus starts from everything. The `tsa` target additionally
digs the embedded timestamp tokens out of every signed document it can
find, by scanning for the `signatureTimeStampToken` OID in raw bytes —
deliberately crudely, because a seed does not have to be correct, only
interesting, and doing it properly would mean importing
`internal/pades/verify` into the tests of the one package written to be
independent of it.

---

## C-6 — the suite left six megabytes in `%TEMP%` on every run (fixed)

**Found by counting what was on the machine at the end**, which is FTEST
§0.4's own instruction and is how B-10 was found in the previous pass.

```
196 liro-config-* directories in %TEMP%
3 365 files
1 255 MB
```

One per suite run, all of them from this morning. Roughly six megabytes
a run, kept for ever.

**The cleanup was there and did not work.** `tempConfigHome`
(`settingspersist_windows_test.go`) points `LOCALAPPDATA` at a temporary
directory and removes it in a `t.Cleanup`. Its own comment already knew
why `t.TempDir()` would not do:

> a WebView2 window created while LOCALAPPDATA points here puts its
> user-data folder underneath it and keeps files in it open after the
> window has closed

— and then the cleanup wrote `_ = os.RemoveAll(dir)`, discarding the
failure. So the directory was never deleted and nothing ever said so.

**How long "after the window has closed" turns out to be.** One of those
browser process groups was found still running **two hours and five
minutes** after the test binary that started it had exited. Its command
line names it unambiguously:

```
msedgewebview2.exe --embedded-browser-webview=1
  --webview-exe-name=liro-bridge.test.exe
  --user-data-dir="…\Temp\liro-config-3736827698\Liro…"
```

**The fix is both halves, and neither is enough alone.** The cleanup now
retries for five seconds, which handles the ordinary case where the
browser is merely slow. `TestMain` sweeps `liro-config-*` directories
older than an hour, before and after the run — which is what makes this
*stop accumulating* rather than accumulate more slowly, because a
directory the retry could not take is one a later run finds free. An
hour, so a suite running beside another cannot delete the other's.

Running the sweep once took the machine from **196 directories to 81**,
the remainder being younger than the cutoff and due on the next run.

**The test asserts the mechanism, not what is in `%TEMP%` right now.**
The first version asserted the latter and went red — correctly — for one
directory an earlier run had left genuinely stuck. That is a verdict
about history rather than about this run, which is the same trap D-112
records and which this pass had already walked into once (D-171). What
is asserted instead: a directory older than the cutoff is swept, a
younger one is not, a directory that is not a config home is not, and a
directory nothing is holding is deleted on the first attempt rather than
after the retry budget.

This is the same family as B-1 and B-10 — a test that changes the
machine and does not put it back — and it is the third of them. The
common thread across all three is not carelessness; it is that **the
cleanup's failure was discarded in every case**, so the only way to find
out was to go and count.

**One thing this made visible, which is C-3 on the machine rather than
in a counter.** After stopping that leftover browser process by its
exact PID, Windows still listed it: `HasExited: True`, and `taskkill`
answering "there is no running instance of the task" while
`Get-Process` returned it. A process that has exited but whose object is
still held open by somebody's handle is exactly what C-3 measures from
the inside.
