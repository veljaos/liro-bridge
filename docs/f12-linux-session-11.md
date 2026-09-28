# F12 Linux — session 11

**What this is:** the eleventh session on the Linux VM, and the handover to the
twelfth. **It measured nothing across a logout.** The login it began at
followed a boot, not a logout, for the second session running (D-400). The
owner then decided to stop for the day, and to leave C0, C3a and C3b
unmeasured on this VM. It ends at no particular boundary. How the machine
goes down after it is not part of any measurement.

**Written:** 2026-09-28, Ubuntu 24.04.5 VM, HWE kernel 7.0.0-34-generic.
**Entries:** D-400. Master at the end of the session: the commit carrying
this document.

---

## A. The state

- **Installed:** `liro-bridge 0.9.9~dev.12` (`3a1037a`), unchanged.
- **Two boots ended with no shutdown record** after session 10's commit:
  18:39:43 → journal stops 18:53:56, the next boot at 18:54:32, and a logout
  **had begun** at 18:53:55. Then 18:54:32 → journal stops 18:58:43, the next
  boot at 18:59:44, with no logout and DING relaunched 75 times. **The person
  who was there cannot account for either**, and the record attributes
  neither (D-400).
- **C0 not measured**: this login followed a boot. `/run/user/1000`'s birth
  time, 19:00:03.33, is void for C0.
- **C0, C3a, C3b: unmeasured on this VM, by the owner's decision.** The
  retained record (since 2026-09-20) holds **no logout followed by a login in
  the same boot**: 18 sessions, 11 `down`, 6 `crash`, 1 current. Whether this
  machine survives a logout is **not established either way**. One logout was
  seen to begin and none to complete (D-400).
- **E9 needs a login, not a logout**, and "Ubuntu on Xorg" is offered at this
  VM's greeter. Kept for the next session (§C).
- **At the end of this session:** a tray running, pid 2400, started 19:00:06
  by autostart (`app-gnome-liro\x2dbridge-2400.scope`); `bridge.json` live,
  dev.12, 17580. `Linger=no`. **The card reader is not attached**: `0bda:0165`
  is absent from `lsusb` and `pcscd` is inactive.

---

## B. What the owner decided

- *"Do not try again today. Three logins, none of them a logout, and I am
  tired enough that I will get the fourth wrong too. That is its own
  reason."*
- C0, C3a and C3b written as **unmeasured on this VM**, with what each would
  take and what stands in the way (D-400's table).
- The two endings recorded as **unexplained**, with the owner's own account:
  *"I cannot reconstruct which of the two boots that was or what I saw before
  it."*
- E9: if a boot will do instead of a logout, **keep it for the next session**.
  It will.
- **Not reopened:** B19's two-user part, A29.

---

## C. What the next session does first, with the predictions

Session 9 §C's rules apply: exact pid only, `ps -C liro-bridge`, never
`pgrep -f`/`pkill -f`, not `ss -p`, `proctree.py` for descendants,
`journalctl --user -o cat`. One change per boot, checked against this file
(D-391).

**The first read at every login**, amended by D-400. `last`'s check has never
had a positive on this VM, so logind is read as well:

```
uptime -s; last -x -F | head -6; loginctl list-sessions --no-legend
journalctl -b 0 -o short-precise _COMM=systemd-logind | grep -E 'New session|Removed session'
echo $XDG_SESSION_TYPE $WAYLAND_DISPLAY $DISPLAY
loginctl show-user vboxuser -p State -p Linger
ps -C liro-bridge -o pid,ppid,lstart,args
```

**None of the next logins is meant to follow a logout.** A new `uptime -s` is
expected at each, and nothing at them is measured across the boundary. If a
future session does try a logout, it counts only if `-b 0` shows
"Removed session N" and then "New session M of user vboxuser".

### C1. The next login: the Wayland control for E9

This session ends in a Wayland session, and GDM keeps the last session
chosen, so the next login should be Wayland without choosing anything.

| | predicted |
|---|---|
| `uptime -s` | new: a boot, which is expected |
| `$XDG_SESSION_TYPE` | `wayland`; `WAYLAND_DISPLAY` `wayland-0`; `DISPLAY` `:0` (Xwayland), as at this session |
| `Linger` | `no` |
| a tray | exactly one, `tray`, by autostart |

Then session 9 §C4's control: the owner opens **Settings** from the tray, and
`xwininfo -root -tree | grep -i -c liro` → predicted **0**. Close Settings.

Before the shutdown, write what the Xorg login needs, as each handover has
done. Then **shut down** (not log out, not restart) and boot.

### C2. The login after that: E9, on "Ubuntu on Xorg"

At GDM's greeter: choose the user, then the **gear** (bottom right), then
**"Ubuntu on Xorg"**, then enter the password. The first read, then session
9 §C4's reads and predictions, **unchanged**: the session is `x11`; the tray
starts and runs normally; the Settings window is in the X tree, found with
`xwininfo`, with `xprop` giving `WM_CLASS` and `_NET_WM_PID`; and the least
certain is whether GTK or WebKitGTK complains on X11 in a way the agent turns
into a failure.

Added here, before either login:

| | predicted |
|---|---|
| `$DISPLAY` | `:0` or `:1`. **Not predicted beyond that**: which one GDM gives an Xorg user session next to a Wayland greeter has not been read on this VM |
| `loginctl show-session ID -p Type`, ID from `list-sessions` (`$XDG_SESSION_ID` is empty in this terminal's environment) | `x11`, agreeing with the environment |
| the agent's log at that start | the same three lines as a Wayland start — "starting", "secrets are kept…", "listening" — and **no line naming the backend**, because nothing in the code reads it (D-396) |

The optional consent-window step (demo A, then **refuse**; nothing clicked by
any program, `xdotool` not run, D-094) needs the reader only if it gets as
far as the card. **It is not attached at this session's end**, so the owner
would attach it in VirtualBox first.

### C3. Back to Wayland

Shut down; at the greeter choose **"Ubuntu"** explicitly, because GDM will
now offer Xorg by default. The first read's `$XDG_SESSION_TYPE` →
`wayland`. Then bring the owner E9's options, **refuse to start**, **warn**
or **amend the clause**, each with its cost, having said plainly what
happened on Xorg.

### C0, C3a, C3b — not on this VM

Unmeasured by decision (D-400). Their predictions (session 10 §C0 and §C3,
session 9 §C3) stand for any Ubuntu 24.04 desktop where a logout can be
shown to be one. If they are ever run and linger is turned on, `disable-linger`
is the first act of the next login, **whatever happened**: linger is a file
and survives a crash.

---

## D. Instruments that failed this session

| the instrument | what it reported | what was wrong |
|---|---|---|
| session 10 §B's check, "the old tty2 line ends in a time" | — | it has never had a positive on this VM: no `wtmp` line since 2026-09-20 ends in a time, so whether GDM writes one here is not known. logind in `-b 0` added (D-400) |
| the journal's last line, read as when a boot ended | 18:53:56, 18:58:43 | an unclean end loses about the last half-minute (dirty write-back 30 s), so it is where the record stops (D-400) |
| session 10 §C0's birth-time read | 19:00:03.33, new | not wrong, but void at a boot; it separates the two cases only after a proven logout |

---

## E. Owed to the owner, needing no logout

1. **D24's amendment**: drafted and shown before it is written (session 9
   §E).
2. **E9's options**, after §C2.

---

## F. This machine

Rules unchanged. **No apt packages installed this session.** Nothing written
to `/tmp` or to the scratch directory. `liro-demo/` untouched. Nothing on the
system changed: no linger, no configuration, no session choice. The one
change outside the repository is a note to Claude's own memory: the login
check's third case (`crash`) and the journal's lost half-minute.
