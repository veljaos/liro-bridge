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

### J-3 — signing a folder twice produces `-signed-signed.pdf`

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
