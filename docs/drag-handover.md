# Handover: the drag that does nothing — closed

**Status on 2026-09-13: closed as not reproducible and not explained.**
See [D-260](decisions.md). Nothing here is queued and nothing here needs
anybody's hands.

This file used to carry a run waiting to be performed. It is kept, much
shorter, for the two parts of it that are still about this machine
rather than about that run.

---

## What happened to the investigation

- **D-255** established that dragging had worked (38 delivered drops on
  5 and 6 September, in the agent's own log) and then stopped, with no
  `drag entered` line at all in the failing sessions.
- **D-256** took six drags from the owner's hands. Every one worked, and
  between them they eliminated the Windows updates, D-207's apartment
  change, the binary, the build flags, the home, the launch path,
  navigation and UIPI. Its finding is that `entered=0` is a statement
  about the desktop rather than about this program.
- **D-257** closed a registration asymmetry it found on the way, and
  says plainly that it is not the fix.
- **D-258** made the agent notice, once a second, when something is in
  front of its own window, and added `scripts/dragreport.cmd`.
- **D-260** closes it. The queued two-drag run cannot be performed as it
  was designed — Settings covers the signing window with 20 points
  showing down each side — and would in any case have demonstrated a
  mechanism nobody disputes. The instrument is shipped; the next
  occurrence is the deliverable.

**One thing that was believed here and is not true**, recorded in
[D-259]: this file used to say that Settings, Certificates, the audit
log, pairing and placement "have no drop target at all, so a file
dragged onto any of them is refused silently with a no-entry cursor."
They had no drop target *of ours*. Chromium's own was still registered
on every one of them, because `put_AllowExternalDrop(FALSE)` was called
only for the window that takes documents — and what a browser does with
a dropped file is navigate to it. That is why a PDF dropped on Settings
was rendered inside it.

---

## 1. `dist\dragtest\` is not this session's to clean

`git clean -xfd` deletes it. Nothing there is irreplaceable, but the two
`.reg` files are `reg export` of
`HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign`
and `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` taken on
2026-09-12, and they are deliberately not committed: the `Run` export
carries the owner's other startup entries, which are machine
configuration and have no business in a public repository.

| File | Survives a clean? |
|---|---|
| `scripts\dragreport.cmd` / `.ps1` | yes, committed |
| `dist\dragtest\*.cmd`, `*.exe`, `*.reg` | no |

Recreate a binary with
`go build -o dist/dragtest/liro-bridge-master.exe ./cmd/liro-bridge`.

---

## 2. Notes for whoever works on this machine

Things that cost real time and are not in `decisions.md`:

- **Running any agent binary re-points the Explorer verb, and `tray` and
  `open` also re-point `HKCU\...\Run`.** Export both keys before
  starting anything and restore from the export afterwards — a hash
  proves a change and cannot undo one (D-153, D-243). This is not
  isolated by `LOCALAPPDATA`.
- **Point `LOCALAPPDATA` at a scratch directory** to run an agent
  without touching the real home. The registry is *not* isolated by it.
- **`Start-Process -WindowStyle Hidden` makes the agent's first window
  invisible.** Windows applies the `wShowWindow` from the launching
  process's `STARTUPINFO` to a process's *first* `ShowWindow` call,
  whatever that call asks for. The window is created and fully set up
  and simply never appears — which reads exactly like a hang. Drop the
  flag.
- **`FindWindowEx` with a NULL parent and a non-NULL `hwndChildAfter`
  does not enumerate top-level windows.** Use `EnumWindows` and compare
  the class. Half an hour, once.
- **`WindowFromPoint` plus a process-id comparison is a trap.**
  `Chrome_RenderWidgetHostHWND` belongs to the msedgewebview2 process,
  so "is this window ours" must be membership of the frame's own tree.
- **A drop cannot be driven from another process.** The window carrying
  a drop target is the browser's, so the `IDropTarget*` in its window
  property is a pointer in that process's address space; and OLE's own
  path, `DoDragDrop`, is a modal loop tracking the real mouse, which
  D-094 forbids simulating. What *is* readable from outside is whether
  anything under a window accepts a drop at all.
- **Do not create a child of an agent window from a test goroutine.**
  Destroying the parent then has to destroy a child owned by a thread
  blocked inside `Close()`, and it deadlocks the one UI thread for the
  rest of the run. 25 minutes, once.
- **Kill by exact PID, never by pattern.**
- `go test ./...` twice at once will deadlock on the WebView2 user data
  folder (D-098). Run the suites one after another.
