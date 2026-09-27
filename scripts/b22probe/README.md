# b22probe — what Windows accessibility is told about the PIN dialog's text

Windows-only. It ships in nothing. The reading it took is **D-395**; the item it
would be wanted for again is **B24**.

It answers B22: does the shipped PIN dialog hand what is typed to another
process, the way the GTK dialog was found to on Linux (**D-384**)? Measured
answer: **it does not** — not through UI Automation's value or text patterns,
not through a desktop-wide UIA subscription, not through MSAA's `get_accValue`,
and not through a cross-process `WM_GETTEXT`, which returns zero so not even the
length crosses. One of the four methods is **not** closed by that run; see
"What B24 needs".

It is committed for the reason ci.yml gives beside `scripts/pinmem`: the first
pinmem lived in `/tmp` and went with a reboot, and was rebuilt at the cost of
forty-five minutes. The Go half is compiled on every push by the `windows` job's
`go vet ./...`.

## The two halves

| file | what it is |
|---|---|
| `b22probe_windows.go` | **the subject.** Raises three windows and reads its own controls. Calls the real `ui.CollectPIN`. |
| `probe.cs` | **the instrument.** A separate process. UIA, MSAA and `WM_GETTEXT`, all filtered to the holder's exact pid. |
| `b22probe_other.go` | a stub, so `go vet ./...` and `go list ./...` are valid off Windows |
| `build.ps1` | builds both. Installs nothing; the C# half uses the .NET Framework compiler and UIA/MSAA assemblies that are part of Windows. |
| `dryrun.ps1` | drives the rig with `WM_SETTEXT` to prove the instrument works. **Not a measurement** — see below. |
| `predictions.md` | written before the run, with the least certain named, and an addendum saying which rows stopped being blind once the dry runs had exercised them |
| `overlay.json` | a record, not a tool. See "History". |
| `runs/` | the report of the real run, and the synthetic dry-run transcripts |

## Running it

```powershell
powershell -ExecutionPolicy Bypass -File scripts\b22probe\build.ps1 C:\some\dir
```

Then two terminals, **holder first** — the probe reads the holder's pid from the
file the holder writes, so it cannot watch the wrong process:

```powershell
& "C:\some\dir\holder.exe" "C:\some\dir"    # prints its pid and three needles, then waits
& "C:\some\dir\probe.exe"  "C:\some\dir"    # prints the pid it is watching
```

Check the two pids match, press Enter in the holder, and type each needle by
hand into its window — **wait two seconds, then Enter**, so a 10 Hz poll gets
twenty samples of steady state. The probe writes `b22-report-<run>.txt`.

**Nothing here sends input** (D-094). Window C is the real dialog wired to
nothing: no card, no reader, no module, no `C_Login`, nothing signed, and its
subject line says so.

## Why there are three windows, and two controls

The three windows are one phase each:

- **A** — a plain `EDIT`, no `ES_PASSWORD`. **The instrument's control.** If the
  probe cannot read this, every absence it reports afterwards is worthless. On
  Linux the first control failed silently and the absence read as clean (D-384),
  and in this probe's own real run **one method failed exactly here**.
- **B** — the same control with `ES_PASSWORD` set. Separates *the style stops
  it* from *something about this program's dialog stops it*. Without B, neither
  answer is available.
- **C** — `ui.CollectPIN`, unmodified. The measurement.

The second control is at the **subject** rather than the instrument: the holder
reads its own control when OK is pressed and states whether the needle was in
it. This is not a nicety. Every refusal the probe records is
content-independent — `E_ACCESSDENIED`, a missing `TextPattern` and
`WM_GETTEXTLENGTH` = 0 read identically against an empty field — so without
that line the whole result would rest on nobody having mistyped.

## The report never prints what it captured

Each row gives a length and a verdict: *EXACT NEEDLE*, *PREFIX OF NEEDLE*,
*mask characters only*, *empty string*, *no value returned* with the provider's
own refusal, *N events delivered, none carried a string value*, or *COULD NOT
RUN* with the reason — and, where a method could not read something, whether
events nonetheless arrived. Unexpected text is identified by a character-class
summary and eight hex of a SHA-256 rather than shown.

Needles are `crypto/rand`, fresh per run, three of them, one per phase: a stale
capture cannot match, and a cross-phase mix-up shows up as needle A matching in
phase C.

## The dry run is not a measurement

`dryrun.ps1` installs the text with `WM_SETTEXT` instead of typing it, the way
`cmd/liro-bridge/pindialog_windows_test.go` does. It exists to prove the
instrument reads anything at all before a person spends an evening on it, and it
earns its place: it caught three defects in the probe, all of them the exact
failure being hunted — a missing row where a method never ran, a dropped
`E_ACCESSDENIED` reported as "nothing returned", and a blank count where one
event had arrived carrying no string.

**It does not answer B22**, and `runs/dryrun-*-synthetic.txt` are labelled
accordingly. D-351 and D-352 are the finding that installed text and typed text
are not the same path — and this probe met that difference head-on: on the
synthetic path the WinEvent hook read the needle from an
`EVENT_OBJECT_VALUECHANGE`; when a person typed, no value-change event reached
it at all.

## What B24 needs

Method 2, the out-of-process WinEvent hook, is **not** closed. In the real run
it received no `EVENT_OBJECT_VALUECHANGE` for any phase — including the
unprotected control that six other rows read the needle from — so its silence
for the password phases is an instrument that saw nothing anywhere rather than a
refusal.

Closing it needs **a third control**: a stimulus that makes the hook fire on
*typed* input into a plain edit, and then the same stimulus against the dialog.
What bounds the gap meanwhile is reasoning and not a reading, and D-395 says so:
a WinEvent carries no text, only an hwnd and an object id, and the callback a
client must then make is `get_accValue` — measured refused with
`E_ACCESSDENIED`.

One method could not be built at all and says so in every report rather than
going quiet: `1b`, UIA `LegacyIAccessiblePattern`, which exists only on the COM
client. `2b` reads the same provider through MSAA directly, which is what
`LegacyIAccessible` wraps.

## History

During the measurement this tool was outside the repository, because the session
was told not to change any code. The Go half was built with `go build
-overlay=`, which maps a file into the module for the duration of one build —
enough to import `internal/ui` without a file existing in the tree, so
`git status` stayed clean throughout. `overlay.json` is that file, kept as the
record. **It is not needed now**, and its absolute paths are that session's
scratchpad directory.
