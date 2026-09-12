# Handover: the drag that does nothing

**Status on 2026-09-12:** not reproducible, not explained, fully
instrumented. One measurement is queued and needs the owner's hands.

Read D-255 through D-258 first; they carry the evidence and the
reasoning. This file is what they do **not** say: what is queued, how to
run it, what to do with each outcome, and what else F10 still owes.

---

## 1. Do not clean `dist/` until the run is done

The queued run lives in `dist\dragtest\`, which `.gitignore` excludes.
`git clean -xfd` deletes it. Nothing there is irreplaceable — this
section says how to rebuild all of it — but it is five minutes and one
`reg export` that no longer exists once it is gone.

| File | Where | Survives a clean? |
|---|---|---|
| `scripts\dragreport.cmd` / `.ps1` | repository | yes, committed |
| `dist\dragtest\A-window.cmd` | gitignored | no — recreate from §2 |
| `dist\dragtest\B-tray.cmd` | gitignored | no — recreate from §2 |
| `dist\dragtest\liro-bridge-master.exe` | gitignored | no — `go build -o dist/dragtest/liro-bridge-master.exe ./cmd/liro-bridge` |
| `dist\dragtest\3-restore.cmd` + two `.reg` files | gitignored | no — see below |

The two `.reg` files are `reg export` of
`HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign`
and `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, taken before
anything ran on 2026-09-12. **Both keys were restored from them and
verified byte-identical**, so the live registry currently *is* their
content and they can be re-exported at any time — until something
re-points them again, which every run of any agent binary does.

They are deliberately not committed: the `Run` export carries the
owner's other startup entries, which are machine configuration and have
no business in a public repository.

---

## 2. The run that is queued

**Purpose.** Demonstrate that a drop aimed at the signing window goes
nowhere when another window is in front of it, and that the agent now
says so. This is a demonstration of a mechanism, not a reconstruction
of 2026-09-12 — see §4.

**The two launchers**, recreate verbatim if they are gone:

`dist\dragtest\A-window.cmd`
```
@echo off
start "" "%~dp0liro-bridge-master.exe" open
```

`dist\dragtest\B-tray.cmd`
```
@echo off
start "" "%~dp0liro-bridge-master.exe" tray
```

`start` gives each its own console, which D-254's fix frees at once
because it is alone on it — so no terminal is left on the desktop. Both
run against the **real** `%LOCALAPPDATA%\Liro`, on purpose, so that
`dragreport.cmd` reads the right log without being told.

**The sequence** (the owner has this; it is here so it is not lost):

1. Close the browser and anything that raises windows or pops toasts.
   `Win`+`N` → Do not disturb.
2. One Explorer window with a PDF, parked at a screen edge, not
   overlapping the middle third.
3. `A-window.cmd` → the signing window, centred, 560×690 client, showing
   the dashed drop area. A console may flash and vanish; one that stays
   is its own finding.
4. `B-tray.cmd` → a second agent, tray icon only, no window.
5. Tray icon → **Podešavanja**. Settings opens centred, 520×880 — taller
   and narrower, so it hides the drop area completely and leaves about
   28 px of the signing window showing down each side.
6. Wait 5 seconds; the cover check runs once a second.
7. **9a**: drag the PDF onto the dead centre of Settings, hold 3 s,
   release. **9b**: drag it onto the visible strip of the signing window
   at the side, hold 3 s, release.
8. **Before closing anything**: `scripts\dragreport.cmd`.
9. Close both windows, tray → Izađi, then `3-restore.cmd`.

---

## 3. What the two drags distinguish, and one thing that does not work

9a and 9b are the same window, seconds apart, in one state. 9a is aimed
where the drop area is and lands on something else; 9b is aimed at the
same window where it is not covered. If 9a does nothing and 9b works,
coverage is the whole difference.

**Two overlapping *signing* windows would not have shown this**, and it
is worth writing down because it is the obvious design and it is wrong:
every signing window accepts drops, so the front one would take the
document and show it. That is "the drop went to the wrong window", not
"nothing happened".

What makes Settings the right coverer is a fact worth keeping:
**`OnFilesDropped` is set on exactly one window in this program** —
`mainwindow_windows.go`, the signing window. Settings, Certificates, the
audit log, pairing and placement have no drop target at all, so a file
dragged onto any of them is refused silently with a no-entry cursor.
Any of them over the signing window reproduces the reported symptom.

---

## 4. What to do with each outcome

**9a nothing + 9b works.** The mechanism is demonstrated. Record it as a
decision, and be precise about what it proves: *this is how a drop
disappears here*, **not** *this is what happened on 2026-09-12*. Nothing
established today shows anything was in front of the window that
afternoon; D-256 says so and the new entry must not quietly upgrade it.

Then there is a product question, and it is the owner's rather than
anyone's to take unilaterally: a signing window that knows it is covered
could raise itself, or say so on the drop area, or do nothing and only
log. Each has a cost — a window that raises itself is a window that
steals the foreground, which this project has been careful about since
D-122.

**9a nothing + 9b also nothing.** More serious, and not about coverage.
Read the report: were the targets still registered at that moment, and
what was under the centre? Then repeat 9b with Settings closed. If it
still refuses uncovered, that is the original fault reproduced live and
with an instrument attached, which is the best position anyone has been
in all week.

**9a works.** Settings is not covering the drop area on that display —
scaling, or the windows did not land concentrically. Take the geometry
from the report and re-plan. Conclude nothing about the fault.

**No cover line in the log.** A defect in what D-258 shipped.
`classifyCover`'s unit tests pass, so look at the live sampling in
`reportCover`: the `isIconic` guard, the empty-rect guard, or the tree
membership test. Fix it before anything else — an instrument that does
not fire is worse than none, because it reads as evidence of absence.

---

## 5. What F10 still owes after this

The drag is one item. The others, none of which this investigation
touched:

- **The exit condition itself.** A stranger installs it and signs a
  document. Neither the agent nor the owner can be that stranger.
- **D-246**: the installer's WebView2 `LaunchCondition` is written, read
  and **never exercised**, because that needs a machine genuinely
  without the Evergreen runtime. The agent's own run-time behaviour with
  the runtime absent *is* measured; the two are different checks reading
  different things and must not be reported as one.
- **D-249**: the per-user MSI has been installed under a SAFER
  Basic User token, which is stricter than a standard user. It has not
  been installed under a standard account on a machine that never held a
  Go toolchain, which is what F10's rules ask for.
- **D-253**: `docs/guide/slike/09-upozorenje-izdavac.png` shows a file
  named `liro-bridge-1.0.2-x64.exe` — a version that has never existed —
  and a `From:` line naming the owner's profile and an agent working
  directory. It ships in both guides and both MSIs. Recapturing it needs
  the owner, but not a browser: the `Zone.Identifier` stream can be
  written by hand.
- **v0.9.1.** The console fix has been on master since `5babf33`;
  what is published still puts an empty terminal beside the agent on
  every launch. The owner intends to cut it after this run.

---

## 6. Notes for whoever works on this machine

Things that cost this session time and are not in `decisions.md`:

- **Running any agent binary re-points the Explorer verb, and the
  `master` binary also re-points `HKCU\...\Run`.** `775dfa3` moves only
  the verb; `applyAutostart` arrived with D-247. Export both keys before
  starting anything and restore from the export afterwards — a hash
  proves a change and cannot undo one (D-153, D-243).
- **Point `LOCALAPPDATA` at a scratch directory** to run an agent
  without touching the real home. The registry is *not* isolated by it.
- **`WindowFromPoint` plus a process-id comparison is a trap.**
  `Chrome_RenderWidgetHostHWND` belongs to the msedgewebview2 process,
  so "is this window ours" must be membership of the frame's own tree.
  Both this session's probe and, briefly, `classifyCover` got it wrong.
- **Do not create a child of an agent window from a test goroutine.**
  Destroying the parent then has to destroy a child owned by a thread
  blocked inside `Close()`, and it deadlocks the one UI thread for the
  rest of the run. 25 minutes, once.
- **Kill by exact PID, never by pattern.** This session used
  `pkill -f liro-bridge` once. Nothing was harmed and that was luck.
- `go test ./...` twice at once will deadlock on the WebView2 user data
  folder (D-098). Run the suites one after another.
