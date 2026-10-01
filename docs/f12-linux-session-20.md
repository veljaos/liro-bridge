# F12 Linux — session 20: back to Wayland, then Fedora with dev.14

**What this is:** the handover at the end of session 19's Xorg readings of
dev.14 on the Ubuntu VM (D-415), written before the owner logs out of
"Ubuntu on Xorg", because the logout ends the session that wrote it.
**Written:** 2026-10-01. **Working rules:** session 13 §C, session 15 §E,
session 16 §E, session 17 §F, session 18 §D, session 19 §E, and §E below.
D-304's five questions before believing any check.

---

## A. Where things stand

**dev.14 on the Ubuntu VM is done on both display servers.** Wayland
(D-414): R1–R3, R5, R7 held; R6 not performable by a person. Xorg (D-415):
R1, R2, R5 held; **R6 performed by a person, twice** — once by accident,
then on purpose — and the helper's stop path did what it was built for.
Hashes unchanged from session 18 §A; the rpm
`f5b1ecdbc22f63673af18ee2b3d13f0864705bceee2ad45aecf8274f6e88c022`.

Running on Xorg at the time of writing: tray **15968** (session 18, from
21:36:56), with WebKit's NetworkProcess 17919 and two
`bwrap → xdg-dbus-proxy` chains, 17920 and 19127 (D23).

**For dev.15, the owner's**: D27 (first start), D28 (the drop outline
never clears), D29 (remove the document size everywhere a person sees
it), and D30 (a logout's end) once it is understood.

On the Fedora VM: unchanged — dev.12 installed. **dev.13 never goes
there.**

## B. The logout back to Wayland, and what it can tell us

The owner logs out and picks **"Ubuntu"** at the gear explicitly — GDM
remembers Xorg (D-403). Expect a VT switch only if the screen is black;
on Xorg it has been black twice (D-402, D-415).

**Prove it was a logout first**, as session 19 §B: `uptime -s` still
**2026-10-01 20:13:41**; `last -x -F`; logind's "Session 18 logged out";
`loginctl show-session N -p Type` says **wayland**; a greeter `cN` after
the login is read before it is called anything.

**Then 15968's end — open-items D30's Xorg half, readable only now.**
Predictions before reading; least certain: whether Xorg going ends the tray
as the Wayland compositor did (GDK's X11 backend reporting the display lost)
or whether a SIGTERM arrives first. Read: 15968's last `bridge.log` line
(`command grep -a`); its scope's lines in the journal — a "Stopping" before
"Consumed" means systemd stopped it; Shell's "Shutting down" and Xorg's
exit, with `-o short-precise`; and whether 17919, 17920 and 19127 went with
it.

## C. Fedora

Session 19 §D, unchanged in substance, with **dev.14's rpm**: its hash
checked on the Fedora VM, `rpm -K` read first, then the install; the reboot
question is the owner's (session 16 §D). R1–R7 as session 16 §C1.

Added by D-415:

- **R6 on Fedora: read geometry, do not assume.** With the chooser up, the
  window's and the dialog's geometry, the dialog's `WM_TRANSIENT_FOR` /
  modal state where readable, and `attach-modal-dialogs`. Whether the
  corner X is reachable is the question; attachment alone did not decide it
  on Ubuntu (B28). The planned instance is a deliberate click on the
  window's corner X with the dialog up.
- **The drop**: the refused `RetrieveFiles` appeared on both of Ubuntu's
  backends and did not stop the drop. On GTK 4.22 it is still the first
  place to look if a drop fails (D-408).
- **The drop outline (D28)**: look at it after a drop and say whether it
  clears; nobody has looked on Wayland, and Fedora runs Wayland.
- **Connections**: the window's bus connection is the process's GTK
  connection and outlives the window (D-415) — name it "from the window's
  process".

**D26 closes when dev.14 has run on both machines** (the owner, D-413).

## D. Not taken here

- B28's Wayland half: the geometry on Ubuntu's Wayland with the chooser up.
  Cheap after the logout, if the owner wants it before Fedora.
- Whether the discovery file is left behind by a logout (D30): it is
  overwritten by the next tray within a second of login, so it needs either
  a reading in between or the tray's own log line.

## E. Rules added in session 19

- **An observation on screen is a hypothesis about geometry until both
  states are read.** "It settled into place" and "not attached" were both
  wrong; a constant 140 px offset and the work area's left edge decided it.
- **A passive watch that ends a loop does not end its producer.** `xprop
  -spy | while … break` kept `xprop` until its timeout. Bound it, and say
  how long it can run.
- **An accident that produces the reading is recorded as an accident.**
  How it came about goes in the entry; the planned instance follows it, and
  the entry says which came first.
- **A logout is not a shutdown for the program either.** D-393's SIGTERM
  path was measured on shutdowns; on the first logout the display went
  first.
