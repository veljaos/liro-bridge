# F12 Linux — session 12

**What this is:** the twelfth session on the Linux VM, and the handover to the
thirteenth. It was E9's Xorg login (session 11 §C2). **It measured nothing
across a boundary**: the login followed a clean shutdown and a boot, as
planned. It measured the Settings window on Xorg, brought the owner E9's
options, and the owner decided; SPEC §6.5.2 and §6.7 were amended (D-402).
**It ends at a shutdown**, and the next login chooses **"Ubuntu"** (§C1).

**Written:** 2026-09-28, Ubuntu 24.04.5 VM, HWE kernel 7.0.0-34-generic.
**Entries:** D-402. Master at the end of the session: the commit carrying
this document.

---

## A. The state

- **Installed:** `liro-bridge 0.9.9~dev.12` (`3a1037a`), unchanged.
- **The previous boot ended cleanly**: a `shutdown` record in `wtmp`, 20:21:29
  → 20:22:12, the first since D-400's two `crash` endings. This boot 20:22:07,
  the login 20:23:57, session `2`.
- **This session is Xorg**, read four ways: logind `Type=x11`; `gdm-x-session`
  → `Xorg vt2` (2080), no Xwayland; `xdpyinfo` "The X.Org Foundation";
  `DISPLAY=:0`, `WAYLAND_DISPLAY` empty (D-402).
- **A VT switch at 20:25:52–20:26:00**, with a GDM greeter session `c2`: the
  owner, Ctrl+Alt+F1 then F2, because the Xorg login sat on a black screen
  with an X-shaped cursor. Accounted for. The long blank screen is recorded
  as what somebody choosing Xorg on this hardware would meet (D-402).
- **E9 measured**: the Settings window is an X11 client, `_NET_WM_PID` the
  tray's, `WM_CLASS` `"", ""`; nothing refused X11 or logged it. One new X
  client, `0x3200000`, unattributed (D-402).
- **SPEC §6.5.2 amended** (three bullets and a note), **§6.7** gains the
  display server, **F12 §4.1** corrected. **None of the three mechanisms is
  built** (open-items E9).
- **At the end of this session:** tray 2601, started 20:24:03 by autostart
  (`app-gnome-liro\x2dbridge-2601.scope`); `bridge.json` live, dev.12, 17580;
  the network process (4453) and one `bwrap → bwrap → xdg-dbus-proxy` chain
  (4454–4457) left by the Settings window (D23). `Linger=no`. **The card
  reader is not attached**: `0bda:0165` absent from `lsusb`, `pcscd`
  inactive.

---

## B. What the owner decided

- **E9**, taking the recommendation whole: on Wayland the agent never chooses
  X11 (`GDK_BACKEND=wayland`); on Xorg it does **not** refuse, but says so in
  the consent window's own text, logs it, and records it in the audit entry;
  the consent screen stops a caller within the protocol, not code running as
  the same user. *"Refusing trades a certain cost against an attacker who has
  clause 3's routes on either display server."*
- **F12 §4.1's "under Wayland it cannot"** is the owner's error, corrected,
  and the correction is not to be softened.
- **The warning and the audit field are one piece of work**, in a later code
  session. **The field records the backend actually in use, not what was
  configured or requested.**
- **Not now:** whether the accessibility bus can press Approve (B25, D-094's
  question and the owner's); the unguarded fallback stays reasoned, not
  measured.

---

## C. What the next session does first, with the predictions

Session 9 §C's rules apply: exact pid only, `ps -C liro-bridge`, never
`pgrep -f`/`pkill -f`, not `ss -p`, `scripts/proctree/proctree.py` for
descendants, `journalctl --user -o cat`. One change per boot, checked against
this file (D-391).

**The first read at every login** (session 11 §C, unchanged):

```
uptime -s; last -x -F | head -6; loginctl list-sessions --no-legend
journalctl -b 0 -o short-precise _COMM=systemd-logind | grep -E 'New session|Removed session'
echo $XDG_SESSION_TYPE $WAYLAND_DISPLAY $DISPLAY
loginctl show-user vboxuser -p State -p Linger
ps -C liro-bridge -o pid,ppid,lstart,args
```

**A greeter session (`cN` of `gdm`) after the login** is what a VT switch
looks like here (D-402): read the journal for Xorg's or the compositor's
"drop master" and "resume" before calling it anything else, and ask.

### C1. The next login: back to Wayland

Shut down; at the greeter choose the user, then the **gear**, then
**"Ubuntu"** explicitly — GDM is expected to offer Xorg by default now, from
its documented per-user memory, not read here (D-400). Predictions:

| | predicted |
|---|---|
| `uptime -s` | new; a `shutdown` record for this boot in `last -x` |
| `loginctl show-session ID -p Type` | `wayland` |
| the display server | `gnome-shell` as the compositor, an Xwayland process, **no `Xorg`** process |
| the environment | `XDG_SESSION_TYPE=wayland`, `WAYLAND_DISPLAY=wayland-0`, `DISPLAY=:0` |
| the tray | one, started by autostart |
| **least certain** | whether GDM does default to Xorg — if "Ubuntu" was already selected at the gear, say so; it is recorded either way |

Nothing else at that login is planned. What follows is the owner's choice.

### C2. What is ready to be chosen

1. **E9's code** (open-items E9): `GDK_BACKEND=wayland` in Wayland sessions;
   the backend read from the display GTK opened; the sentence in the consent
   window; the log line; the audit field. **The field records what was used,
   not what was configured.** A new build installed is a change, so one per
   boot. Then a signature's audit entry read on each display server, which
   needs the card.
2. **D24's amendment**: drafted and shown before it is written (session 9
   §E).
3. **B25**: the owner's ruling, if any.

### C0, C3a, C3b — not on this VM

Unchanged from session 11: unmeasured by decision (D-400). If linger is ever
turned on, `disable-linger` is the first act of the next login, whatever
happened.

---

## D. Instruments that failed this session

| the instrument | what it reported | what was wrong |
|---|---|---|
| the owner's account of the VT switch | "Ctrl+Alt+F2 and then back" | F2 from vt2 goes nowhere; the journal showed a switch away with a greeter starting. F1 then F2, corrected with the owner's agreement (D-402) |
| `xdpyinfo`'s version | 21.1.11 | the package is 21.1.12-1ubuntu1.8. Not reconciled; nothing depends on it |

---

## E. Owed to the owner, needing no logout

1. **D24's amendment**: drafted and shown before it is written.
2. **E9's code**, when the owner chooses a code session.

---

## F. This machine

Rules unchanged. **No apt packages installed.** The two `xwininfo` trees went
to this session's scratch directory under `/tmp` and are gone at the
shutdown. `liro-demo/` untouched. Nothing on the system changed: no linger,
no configuration. The session choice at the greeter was the owner's, and the
next login reverses it. One note added to Claude's own memory: a greeter
session after a login on this VM has been a VT switch.
