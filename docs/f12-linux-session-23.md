# F12 Linux — session 23: the Fedora sitting for D32's runs and dev.15

**What this is:** the sheet for one sitting on the Fedora VM, written on
the Ubuntu VM at the end of session 23 (D-421, D-422). Everything here is
from the record and cited. **Written:** 2026-10-03. **Working rules:**
session 22 §E and those before it, and §E below. D-304's five questions
before believing any check.

---

## A. What to carry

| | sha256 |
|---|---|
| `~/d32probe` (the probe, from `scripts/d32probe` at `56e425f`) | `02b87318…1e56` |
| `dist/linux/liro-bridge-0.9.9-dev.15.x86_64.rpm` | `e7f3420c…813b` |
| its `/usr/bin/liro-bridge` (`9950197`) | `44f448f7…5ec7` |

dev.14's own binary on Fedora is `df7bedd5…6f61` (D-417): what the boot
check compares against (D-391).

## B. The order

1. **The boot check** as in D-417: uptime, `last -x`, the package, the
   binary's hash, the agent's PID and scope. Agent 12473 was left running
   (session 22 §D); after a reboot there is a new one.
2. **D32's five runs, on dev.14 as installed, WebKitGTK 2.54.0**, before
   anything is installed — the version that showed the defect (the owner,
   D-421). Table and predictions in D-421: R0, RA, RB with the probe, then
   RE0 and RE with the agent. **R0 first; if its window 2 paints, stop the
   probe runs** — the probe lacks what matters — and take RE0 and RE anyway.
   One window at a time, the owner's eyes; paste each `d32probe-report-*.txt`.
3. **Then install dev.15** (`dnf install` of the rpm above) as the boot's
   one change, and reboot.
4. **The walk-through** (§C).

## C. The walk-through, and what D32 does to it

**D32 is not fixed in dev.15.** On Fedora every window after an agent's
first has been white (D-420), and the walk-through opens many. **Decided
(the owner, D-422):** if RE showed every window painted with
`WEBKIT_DISABLE_DMABUF_RENDERER=0`, the walk-through is taken under a dev.15
agent started that way from a terminal — `WEBKIT_DISABLE_DMABUF_RENDERER=0
/usr/bin/liro-bridge tray`, after quitting the autostarted one with its
window's **Izađi** — **recorded as a deviation from what ships**. If RE did
not, it is taken under the agent as installed, and every white window is
recorded as D32's, not as a finding about the item being walked. Not a third
crossing either way.

**D27 under that deviation**: its first step quits the agent and launches
from Activities, which starts an agent *without* the variable. So on the
variable-off route, take D27 last, and read its window as D32 predicts.

| item | what a person does | what is read |
|---|---|---|
| D27 | quit the agent (window's **Izađi**); launch **Liro Bridge** from Activities; close the window with the corner X; launch it again. The sentence, if it appears, is the owner's approved wording (D-422) | one `liro-bridge open` that becomes the agent (log: "started by a launch that asked for the window"); the process alive after the X, discovery file present; the second launch hands over. GNOME 50's half of D-421's premise |
| D31 | in that window, **Podešavanja**, then **Izvezi dnevnik revizije**; **Sertifikati**; **Prikaži dnevnik revizije** | each opens; the export writes a file — **R7, and "a person on Fedora can reach Settings and export the audit log" (D-419)** |
| D29 | add a document | no size beside its name; the count line is "Broj dokumenata: 1" |
| D28 | drag a PDF from Files onto the window | the outline clears on the drop (it already did on Fedora's Wayland, D-417 — the reading that matters is Ubuntu's Xorg, §D) |
| D33 | pair afresh (session 22 §D: the old pairing's secret is gone); one request; Odobri, the PIN, Potpiši | **the caller's file arrives while the report still says Završeno**, before anyone presses Završi; the log line "the run ended, so the caller has its result" |

The doors' row costs the document list about one row against D-106's eight;
look at it with eight documents in the list.

## D. Left for the Ubuntu VM after Fedora

- **D28 on Ubuntu's Xorg** — where the outline stayed (D-415) — and on its
  Wayland, never looked at.
- D31, D33 and D27 on GNOME 46; dev.15 installed here too.
- D28's window test with the sandbox off, if the owner rules it (D-413's
  precedent); CI runs it.

## E. Rules added in session 23

- **A guard, approved by the owner, to be installed when they are back
  (D-422)** — not before: "a note that depends on somebody remembering is not
  a control." **Its refusal must say why**, so that a future session can tell
  the guard from a broken build. What was proposed:
  the "no background jobs" rule was broken twice, D-410 and D-421, both
  times by the Bash tool moving an over-long command into the background on
  its own — not by a decision. A memory note does not stop a tool. The
  proposal: a project `PreToolUse` hook on Bash that refuses `go test` or
  `go build` unless the command begins with `timeout` and a bound below the
  tool's limit, and says why when it refuses.

- **Bound the build, not only the test**: `timeout` around the whole
  `go test`, with a build first as its own step; a `-race` build of gotk4
  takes over 16 minutes cold (D-421).
- **Do not write files while a build is reading them**: `go test` takes its
  file list first and the files later, and failed to build a package whose
  new file it had not listed (D-422).
- **A string search in a binary needs a control from the binary before**:
  "size unknown" matched Go's own table (D-422).
