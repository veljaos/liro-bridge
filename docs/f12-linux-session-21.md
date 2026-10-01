# F12 Linux — session 21: Fedora with dev.14

**What this is:** the handover at the end of session 20 on the Ubuntu VM
(D-416), for the sitting on the Fedora VM. Everything here is from the
record and cited. Nothing in it is to be taken as anyone's recollection of
session 20. **Written:** 2026-10-01. **Working rules:** session 13 §C,
session 15 §E, session 16 §E, session 17 §F, session 18 §D, session 19 §E,
session 20 §E, and §F below. D-304's five questions before believing any
check.

---

## A. Where things stand

**The Ubuntu VM is done for dev.14**, on both display servers: Wayland
(D-414) and Xorg (D-415), the logout back read in D-416. Nothing more is
built or measured there.

**The Fedora VM: dev.12 installed. dev.13 never goes there** — R5 killed
the agent on Ubuntu (D-412). dev.14 carries two crash fixes and nothing else
(D-413): the drop target given its formats after construction (D25), and
`load-failed` connected in C (D26). **D26 closes when dev.14 has run on both
machines** (the owner, D-413).

What D-416 adds for anyone reading a log on Fedora: **a logout ends the tray
inside GDK by `_exit(1)`, on both backends**; GTK 4.14's X11 line is logged
at DEBUG and never reaches `bridge.log`; the Wayland line is a MESSAGE and
does. Fedora's GTK is 4.22.1 (D-405) — **its levels and calls are not read;
do not carry 4.14's over.**

## B. What crosses, and how

| what | route | check at the other end |
|---|---|---|
| `liro-bridge-0.9.9-dev.14.x86_64.rpm` (from `dist/linux/` on the Ubuntu VM; **dev.13's rpm sits beside it — copy by the full name**) | **the USB stick** — the only thing that must go on it | 11 525 575 bytes; sha256 **`f5b1ecdbc22f63673af18ee2b3d13f0864705bceee2ad45aecf8274f6e88c022`** (session 18 §A, re-hashed in D-416) |
| this file, `docs/decisions.md` (D-413–D-416), `docs/open-items.md` | git, if `/home/velja/liro-bridge` (D-409) is a clone that can pull: the record shows the path exists, **not how it got there**. If it is not, these three files go on the stick too | `git log -1` there shows the commit that adds this file, or `sha256sum` of the three files against this machine's |
| `blank.pdf` | **already on Fedora**, `/home/velja/liro-bridge/testdata/pdfs/blank.pdf` (D-409's drag) | sha256 `b9749fdb57fcadf802492c78b5da3cc10b7a3772fa91ecc776f9340ccd536fda`, 427 bytes, as on the Ubuntu VM |

Nothing else. No Go toolchain on the Fedora VM (session 16 §A), so the
corrected drop-target window test cannot run there (session 19 §D).

## C. In order, before anything is measured

1. **The Fedora VM's first reads**: `uptime -s` and `last -x -F` (what the
   last boot was, and whether it ended cleanly); `rpm -q liro-bridge` says
   dev.12; `ps -C liro-bridge -o pid,ppid,lstart,args` — the autostart
   entry D-407 wrote starts a `tray` at each login, so a dev.12 tray may be
   running. Exact PIDs only.
2. `sha256sum` of the rpm against §B.
3. **`rpm -K` read first**, and what it says written down. It was unsigned
   for dev.12 and accepted on the hash (D-405, D-406).
4. `sudo dnf install ./liro-bridge-0.9.9-dev.14.x86_64.rpm` — an upgrade
   from dev.12. D-406: dnf fetched nothing but the package and asked for the
   password and `y`. **The password is typed by the owner, in their own
   terminal**; never prompted for from this side.
5. **The reboot question is the owner's** (session 16 §D). One change per
   boot (D-391): the install is the change, and the readings below are taken
   against dev.12's own file, not against what is installed. A running
   dev.12 tray is still dev.12 until it is restarted. Say which binary each
   reading came from.

**Before the install, the owner's choice — D23.** D23 asks for the chain
count on Fedora "with dev.12 **before** any fix". What exists from Fedora is
one window (D-407: one `xdg-dbus-proxy`, descendants 7). dev.14's two
changes are the drop target and `load-failed` (D-413), neither of which is
what D23 names, so a count under dev.14 is still before any fix. Either
count with dev.12 now, before step 4, or count under dev.14 and say so.

## D. R1–R7 on dev.14

Session 16 §C1's table, with what D-414 and D-415 added. Predictions are
written before each reading, the least certain named. The monitor is
session 17 §D's command, run by the owner in their own terminal, to
`/tmp/s21-bus.log`:

```
dbus-monitor --session \
  "type='method_call',interface='org.freedesktop.portal.FileChooser'" \
  "type='method_call',interface='org.freedesktop.portal.Request'" \
  "type='signal',interface='org.freedesktop.portal.Request'" \
  "type='method_call',interface='org.freedesktop.DBus.Properties',destination='org.freedesktop.portal.Desktop'" \
  "type='method_call',interface='org.freedesktop.portal.FileTransfer'" \
  "type='method_call',interface='org.freedesktop.portal.Documents'" \
  "type='error'" \
  "type='method_return',sender='org.freedesktop.portal.Desktop'" \
  > /tmp/s21-bus.log 2>&1
```

Its controls, before any drag (D-414, D-415): one
`org.freedesktop.portal.Documents.GetMountPoint`, and one
`FileTransfer.StopTransfer('s21-control')` — both should be seen, the
second answered `AccessDenied` "Invalid transfer".

| | what | from the record |
|---|---|---|
| R1 | **Izaberi…** | Ubuntu: the portal's dialog for the helper, "Izaberite PDF dokumente", `modal`, `multiple`, the PDF filter and "Sve datoteke"; answered in 4.4 ms (Wayland), 12–20 ms (Xorg). On Fedora 4.22 D-408 saw the window hang on a refused chooser — **dev.14's helper is what is being tested** |
| R2 | while R1's dialog is open | `liro-bridge file-chooser`, parent the window's process; environment one line, `DBUS_SESSION_BUS_ADDRESS=…`; cwd `/`; core limit 0/0; `readlink /proc/WINDOW/root` **refused** |
| R3 | cancel | `Response` 1, no `Request.Close`, helper gone, the window silent |
| R4 | **Promeni…** | not taken on dev.14 anywhere (D-414); a folder dialog on the current output folder |
| R5 | `blank.pdf` dragged from Files | in the list, `documents added` with the counts. **Watch the refused `RetrieveFiles`**: on Ubuntu, both backends, the window's process calls it on each drop, is refused "Unable to open /proc/PID/root", and the drop delivers anyway (D-414, D-415); not attributed. D-408 saw a drop fail silently on Fedora — if this one fails, that call is the first place to look. **The drop outline (D28)**: after the drop, say whether the dashed border clears; nobody has looked on Wayland, and Fedora runs Wayland |
| R6 | the corner X with R1's dialog open | **read geometry, do not assume** (session 20 §C): `gsettings get org.gnome.mutter attach-modal-dialogs` first; the window's and the dialog's geometry with the chooser up; whether the corner X is reachable. On Ubuntu's Wayland it was covered and R6 could not be performed (D-414); on Xorg it was reachable, and the helper sent `Request.Close` 60 ms before the window left (D-415). The planned instance is a deliberate click on the corner X. **Read which process owns the window before predicting what the X ends** — session 16 predicted `open` exits on Fedora, the tray stays on Ubuntu |
| R7 | Settings → **Izvezi dnevnik revizije** | a folder dialog through the helper; "Dnevnik revizije je izvezen u …"; read the audit log's own mtime before predicting anything about it (D-414) |

Name the window's bus connection "from the window's process": it is the
process's GTK connection and outlives the window (D-415).

## E. Also waiting on the Fedora VM, not this sitting's unless the owner says

- open-items B8: WebKitGTK's sandbox under SELinux — the audit log is root's,
  never read (D-407).
- open-items A26: Fedora 44's GTK with `scripts/a11yprobe`.
- open-items F10: **the Fedora VM left as found** when its work is done —
  `gsettings set org.gnome.software download-updates true`, and `liro-bridge`
  removed; `gh` is the owner's.
- D30 on Fedora: if a logout happens there, read the tray's end knowing the
  level matters — find GTK 4.22's display-loss calls and their levels before
  predicting a line (D-416's rule).

## F. Rules added in session 20

- **Before predicting that a line will appear in a log, read the level it is
  written at and what the writer between it and the file keeps.** gotk4's
  writer drops DEBUG from a named domain; D-416's prediction could not have
  held, as D-409's could not have failed.
- **What a logout leaves in `/run/user/UID` is logind's doing, and the
  journal says when**: `user@UID.service` stopping and
  `run-user-UID.mount` deactivated, `UserStopDelaySec` after "logged out",
  unless the user lingers or has another session.
- **A comment stating a mechanism is a claim, as a SPEC sentence is** (D22).
  When a measurement contradicts it, the open item names the comment and
  its correction goes with the fix.
