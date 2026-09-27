# B22 — predictions, written before any measurement

Written 2026-09-27, after the two programs compiled and before either was run
against anything. Nothing below has been measured yet, including by the dry
run. Rank language matches the probe's own: *exact needle*, *prefix*, *mask
characters only*, *empty string*, *nothing returned*, *could not run*.

## Phase A — plain EDIT, no ES_PASSWORD (the instrument's control)

| method | prediction | confidence |
|---|---|---|
| 3 WM_GETTEXT (cross-process) | **exact needle** | high |
| 2b MSAA direct `get_accValue` | **exact needle** | high |
| 2 WinEvent hook → `AccessibleObjectFromEvent` → `get_accValue` | **exact needle**, and a non-zero event count | medium — see below |
| 1 UIA `ValuePattern.Value` | **exact needle** | high |
| 1 UIA `TextPattern.DocumentRange.GetText` | **exact needle** | high |
| 1 UIA `IsPassword` | **FALSE** | high |
| 1c UIA `Name` | *not* the needle — the label, or empty | medium |

If any of the first five reports *nothing returned* or *empty string* here, the
instrument is broken and every absence it reports for B and C is worthless.
That is the whole reason this phase exists and is first.

## Phase B — bare EDIT with ES_PASSWORD, the holder's own window

| method | prediction | confidence |
|---|---|---|
| 3 WM_GETTEXT (cross-process) | **empty string**, `WM_GETTEXT` returns 0 — user32 refuses a cross-process text read on a password edit | medium-high |
| 2b MSAA direct `get_accValue` | **could not run** — `E_ACCESSDENIED` (0x80070005), and `accState` carries `STATE_SYSTEM_PROTECTED` | medium-high |
| 2 WinEvent hook | events **do** arrive; `get_accValue` on them **denied**, so no text | medium |
| 1 UIA `ValuePattern.Value` | **empty string** | medium |
| 1 UIA `TextPattern.DocumentRange.GetText` | **empty string** *or* **mask characters only** | **low — the least certain prediction here** |
| 1 UIA `IsPassword` | **TRUE** | high |

## Phase C — the real dialog, `ui.CollectPIN`

**Prediction: every row identical to phase B.** The reason is structural rather
than hopeful: `pindialog_windows.go:320` creates a stock `EDIT` with
`esPassword|esAutoHScroll|wsBorder|wsTabStop|wsGroup`, and the file implements
no UIA provider, no `WM_GETOBJECT` handler and no accessibility code of any
kind. So the dialog's entire accessibility surface is the one Windows gives a
password edit for free, and B is that surface measured on its own. If C differs
from B in any row, something in this program is adding to that surface, and
finding out what would be the result.

## The least certain prediction, named

**Phase B/C, method 1, `TextPattern.DocumentRange.GetText`.** The others rest on
a documented refusal — a password control's value is withheld from clients. A
text range is a different object: it describes what is *rendered*, and what is
rendered in a password edit is a run of mask characters. I do not know whether
the provider refuses the range, returns an empty one, or returns the mask.

**And the reason it is worth measuring rather than dismissing: mask characters
would still be a finding.** A client that learns nothing about the characters
but learns that there are exactly eight of them has learned the PIN's length
from a process that was never told it. That is less than the Linux case in the
briefing and it is not nothing. SPEC §6.5.1 clause 2 says the PIN crosses
exactly one process boundary; it says nothing about its length, because nobody
had thought of length as something that crosses.

## What the four methods cannot settle, said in advance

- **Only what a client is *told*.** If a provider hands over a masked string
  while the plaintext sits in the control's own buffer, that is D-277's
  exception and this probe is not pointed at it.
- **Only this desktop, as this user, with no screen reader running.** A running
  Narrator, or a third-party tool with UIA elevated/priority hooks, may change
  what providers are activated. Not measured.
- **Only the shipped dialog as the holder calls it.** Same code path, same
  styles, same process-level DPI, but `owner` is 0 rather than the agent's main
  window. If a future reading differs, look there first.
- **Nothing about the keystroke path** — B4's question, `WM_CHAR` through a
  queue this program does not own, is a different measurement and this is not
  it.

## D-304's five questions, asked of this probe before it is believed

1. **Could this check have failed?** Yes, and it is arranged to: phase A is the
   same instrument pointed at a control that does have text, and the report
   prints A first. A probe that reports silence everywhere fails visibly at A.
2. **Could this instrument have seen the thing whose absence it reports?** Two
   independent controls. At the instrument: phase A. At the subject: the holder
   reads its own control at the moment OK is pressed and states whether the
   needle was in it, so an empty box can never be read as a clean absence.
3. **Is the resolution finer than the thing measured?** The poll is 10 Hz and
   the owner is told to leave the needle in the box for two seconds — twenty
   polls of steady state. A miss is therefore not a sampling miss. The event
   subscriptions are not sampled at all.
4. **Did this run actually run?** Three needles, from `crypto/rand`, fresh per
   run, and a run id printed by both programs. A stale capture cannot match,
   and a cross-phase confusion is visible as needle A matching in phase C.
5. **Was anything read, and was a discrepancy explained or absorbed?** The
   report prints one row per method per phase including the rows that could not
   run, with the reason. `1b UIA legacy` already reports *not attempted* with
   its reason rather than being deleted.

---

# Addendum, after D-384 was pulled and read

Written after `git pull` brought D-384 through D-394, and after three synthetic
dry runs. **Two things have to be said plainly before the typed run, because
they change what the word "prediction" is worth above.**

## Which predictions are still blind, and which are not

Everything above was written before anything ran. The dry runs then exercised
most of those rows on a **synthetic path** — the text installed with
`WM_SETTEXT`, not typed. So:

- **No longer blind:** every row the dry run reached. Their values are in
  `dryrun-probe-synthetic.txt`. I am not going to re-predict them and call it a
  prediction.
- **Still blind, and the real content of tonight's run:** what **typing** does
  that a message did not. One row is already suspicious for exactly this
  reason — see below.

The one prediction above that the dry run settled: my least-certain, the
`TextPattern` document range on a password edit. It is neither empty nor mask
characters — **the element exposes no `TextPattern` at all**
(`TryGetCurrentPattern` returns false). The mask-character worry, and with it
the length-leak worry, did not arise on that path. Whether it arises when a
person types is what the run answers.

## B22 asks for one thing the first build did not do

B22 (1) says *"subscribed to text-changed and property-changed (Value) events
**on all windows** while the needle is typed"*. The first build subscribed to
the dialog's own subtree. That is narrower than what was asked, so a second,
desktop-wide subscription was added (`1d`), installed **before any of the three
windows exists**, so that it is live while the needle is typed rather than
attached afterwards.

It is desktop-wide, so it receives events belonging to other processes. Those
are counted and dropped **without reading any property** — the report prints
both counts. "Exact PID only" is kept where it decides something, which is what
gets read, rather than where keeping it would mean not doing what B22 asks.

## Predictions for the two added methods, and for typing

| row | prediction | confidence |
|---|---|---|
| `1d` all-windows, phase A | exact needle, and a non-zero event count | high |
| `1d` all-windows, phase B/C | events **arrive** and carry no text | medium-high — the dry run saw this on the synthetic path and typing should not change who is notified |
| `2` WinEvent hook, phase B/C | **this is the one to watch** | low |

**The WinEvent hook is the least certain thing in tonight's run, and it has
replaced the text range as the answer to "name the least certain".** On the
synthetic path it recorded *0 deliveries* for both password phases while
recording deliveries for the plain one. A password edit that raises no
`EVENT_OBJECT_VALUECHANGE` when its text is *set by a message* may still raise
one per keystroke when a person *types*, because that is a different code path
inside the control. If it does, the probe reads `accValue` on each and the
answer turns on whether that read is refused the way the direct read is. I do
not know, and it is the row where typing most plausibly differs from the dry
run.

## What the dry run already establishes, stated so it is not over-read

On the synthetic path, for both the bare password edit and the real dialog: the
UIA value pattern refuses, `TextPattern` is absent, MSAA's `get_accValue` is
refused with `E_ACCESSDENIED` while `accState` carries
`STATE_SYSTEM_PROTECTED`, cross-process `WM_GETTEXT` returns 0 bytes, and
`IsPassword` is `TRUE`.

**One reading in there is worth naming rather than filing as an absence.** The
desktop-wide subscription delivered **eight `ValueProperty` change events for
the password field, none carrying a string**. The provider announces that the
value changed and withholds what it changed to. That is the shape of D-384 with
the payload removed: the notification crosses, the text does not. It says
nothing about the characters and it does say *that somebody typed, and roughly
how many times* — which is less than D-384's exposure by a long way, and is not
the same as nothing.

