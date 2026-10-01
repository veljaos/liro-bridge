# F12 Linux — session 19: dev.14 on Xorg, then Fedora

**What this is:** the handover at the end of session 18's Wayland
readings of dev.14 on the Ubuntu VM (D-414), written before the owner logs
out to "Ubuntu on Xorg", because the logout ends the session that wrote it
(Claude runs in gnome-terminal under the owner's login). **Written:**
2026-10-01. **Working rules:** session 13 §C, session 15 §E, session 16 §E,
session 17 §F, session 18 §D, and §E below. D-304's five questions before
believing any check.

---

## A. Where things stand

**dev.14 is installed on the Ubuntu VM and running** — tray **2453**,
started 20:14:47 by autostart, session 2 on Wayland. Its hashes are session
18 §A's: the `.deb` `f41006ed…`, the binary `df7bedd5…6f61`; the rpm
`f5b1ecdbc22f63673af18ee2b3d13f0864705bceee2ad45aecf8274f6e88c022`.

**On Wayland (D-414):** R1, R2, R3, R5 and R7 held; **R6 cannot be
performed by a person** — the attached modal dialog leaves no route to
close the window (corner X covered, no × in the overview, the dock offers
only the dialog's close). On every drop the window's process calls
`FileTransfer.RetrieveFiles` and is refused ("Unable to open
/proc/PID/root"); our drop does not need it; which code makes the call is
not attributed.

The corrected drop-target window test is committed with D-414 and green;
it runs only with WebKit's sandbox off (the owner's ruling for that test).

On the Fedora VM: unchanged — dev.12 installed. **dev.13 never goes
there.**

## B. The logout and the first reads on Xorg

The owner logs out, picks **"Ubuntu on Xorg"** at the gear, logs in.
**This is not a boot**: nothing is installed, so D-391's one change is the
session type. GDM remembers the choice, so the login after C2 picks
"Ubuntu" again, explicitly (D-403).

**Prove it was a logout first** (D-399, D-400, D-402): `uptime -s` still
**2026-10-01 20:13:41**; `last -x -F`; logind in `journalctl -b 0`:
"Removed session 2", then a new session; `loginctl show-session N -p Type`
says **x11**; a greeter `cN` after the login is read before it is called
anything (D-402). Then: tray 2453 gone, its last `bridge.log` line
"tray: asked to terminate, so stopping the way Quit does" (a logout's
SIGTERM, D-393); a new tray by `ps -C liro-bridge -o pid,ppid,lstart,args`,
its start line `0.9.9-dev.14 27c59d7` and `StatusNotifierItem-PID-1`.
**Read `bridge.log` with `command grep -a`**: the shell's `grep` is a
`ugrep` wrapper that skips files with binary bytes silently, and line 456
has 765 NULs (D-414).

## C. C2 on Xorg: R1, R2, R6 — and one drag, if the owner agrees

The monitor of session 17 §D, run by the owner, to a new file
(`/tmp/s19-bus.log`), its controls first (D-414's two calls: one
`Documents.GetMountPoint`, one `FileTransfer.StopTransfer('s19-control')`).

| | predicted |
|---|---|
| R1 | the helper's `OpenFile` with **`x11:` and the window's X id in hex** (found through `dlsym`, D-410), "Izaberite PDF dokumente", `modal`, the filters; no `AccessDenied`; the portal's dialog |
| R2 | as on Wayland: one-line environment, cwd `/`, `/proc/WINDOW/root` refused |
| R6 | **least certain: whether the dialog is modal to and attached to the window at all on Xorg.** If attached: as on Wayland, not performable — record it the same way. If not attached: the corner X can be reached; then the helper sends `Request.Close` on its request path within a second, the dialog goes, the helper is gone within 2 s (`StopGrace`), `bridge.log` one INFO "ui: the file chooser" cancelled "the window was closed", no "killing it"; the tray alive. **That would be the first time a person watched C19's path** |
| one drag (owner's choice) | the fix is not display-specific, but GDK's X11 drag code is not Wayland's: in the list, the tray alive; whether the refused `RetrieveFiles` appears on X11 too is unknown |

## D. Fedora, after C2

Session 16 §D with **dev.14's rpm**: its hash checked on the Fedora VM
(`f5b1ecdb…` above), `rpm -K` read first, then the install; the reboot
question is the owner's (session 16 §D). R1–R7 as session 16 §C1. **Watch
the refused `RetrieveFiles` on a drop**: D-408 saw a drop fail silently on
GTK 4.22; dev.14's target reads `text/uri-list` and should deliver anyway,
and if it does not, this call is the first place to look. R6 on Fedora's
GNOME is expected to be as on Ubuntu (same default for attached dialogs) —
read `attach-modal-dialogs` there first rather than assume. The corrected
window test cannot run on Fedora (no Go toolchain).

**D26 closes when dev.14 has run on both machines** (the owner, D-413).

## E. Rules added in session 18

- **The shell's `grep` skips binary files without a word.** It is a
  `ugrep` wrapper with `-I`; on `bridge.log`, which holds NUL bytes, it
  prints nothing, not even a count. Use `command grep -a`, and give any
  absence a positive control on the same file.
- **Numbers taken out of tool output keep the tool's decoration unless
  stripped by its shape**: `tr -dc '0-9'` on gdbus's `(uint32 2152,)` gave
  322152. Strip by pattern, and check the result names a real process.
- **A prediction's risk is checked for being possible before it is called
  held.** R7's "the abrupt boot may have damaged the audit log" could not
  happen: nothing had written to it since 09-28. Reading the file's time
  would not have spoiled the prediction.
- **Read when a control is shown, not only what it does.** Option A rested
  on an overview × that Shell hides for a window with an attached dialog;
  `_deleteAll` was read and `_windowCanClose` was not.
- **A test that counts controllers counts GTK's own as well.** A bare
  widget of the same type, in the same process, is the baseline.
