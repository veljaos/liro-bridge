# F12 Linux — session 11

**What this is:** the eleventh session on the Linux VM, and the handover to the
twelfth. **It measured nothing across a logout.** The login it began at
followed a boot, not a logout, for the second session running (D-400). The
owner then decided to stop for the day, and to leave C0, C3a and C3b
unmeasured on this VM. Later the same evening, by the owner's choice, it took
E9's Wayland control at its own login (§G, D-401). **It ends at a shutdown**,
and the next login is E9's Xorg login (§C2). Nothing is measured across that
boundary.

**Written:** 2026-09-28, Ubuntu 24.04.5 VM, HWE kernel 7.0.0-34-generic.
**Entries:** D-400, D-401. Master at the end of the session: the commit carrying
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
  VM's greeter. **Its Wayland control is done** (D-401): with Settings open,
  no X window appeared at all. The Xorg login is next (§C2).
- **At the end of this session:** a tray running, pid 2400, started 19:00:06
  by autostart (`app-gnome-liro\x2dbridge-2400.scope`); `bridge.json` live,
  dev.12, 17580, with one `bwrap → bwrap → xdg-dbus-proxy` chain (5237–5239)
  left by the Settings window (D23, D-401). `Linger=no`. **The card reader is not attached**: `0bda:0165`
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

### C1. The Wayland control — done at session 11's own login

Taken in §G (D-401). **The next login is §C2.**

### C2. The next login: E9, on "Ubuntu on Xorg"

At GDM's greeter: choose the user, then the **gear** (bottom right), then
**"Ubuntu on Xorg"**, then enter the password. Take the first read. Then
session 9 §C4's predictions, **unchanged**: the session is `x11`; the tray
starts and runs normally; the Settings window is in the X tree; and the least
certain is whether GTK or WebKitGTK complains on X11 in a way the agent turns
into a failure.

**The method changes (D-401): not `grep -i liro`.** The Settings window's
title is "Podešavanja" (`settings.window_title`), and the code sets no
program name or application id (`gtk.InitCheck()`,
`internal/ui/uithread_linux.go:108`). So whether "liro" appears anywhere in
the window's X properties depends on GTK deriving `WM_CLASS` from the
binary's name, which has not been read. Instead, as the control did:

1. Settings **closed**: `xwininfo -root -tree > tree-before.txt`, with its
   exit status checked.
2. The owner opens **Settings** from the tray. Check that the log's
   "settings: window open" line is new, then `xwininfo -root -tree >
   tree-open.txt`.
3. Compare the window IDs:

   ```
   ids(){ grep -o '^ *0x[0-9a-f]*' "$1" | tr -d ' ' | sort; }
   comm -13 <(ids tree-before.txt) <(ids tree-open.txt)
   ```

   For each new id, `xprop -id ID WM_NAME _NET_WM_NAME WM_CLASS _NET_WM_PID`.
4. The owner closes it with **Zatvori** (there is no "Cancel" button; it
   sends `cancel`, as the corner X does, and the log cannot tell them apart).

Added here, before the login:

| | predicted |
|---|---|
| `$DISPLAY` | `:0` or `:1`. **Not predicted beyond that**: which one GDM gives an Xorg user session next to a Wayland greeter has not been read on this VM |
| `loginctl show-session ID -p Type`, ID from `list-sessions` (`$XDG_SESSION_ID` is empty in this terminal's environment) | `x11`, agreeing with the environment |
| new window IDs with Settings open | **at least one** whose `WM_NAME`/`_NET_WM_NAME` is "Podešavanja". Probably **two**, the window and a Mutter frame named after it (`mutter-x11-frames`), as the control's xmessage showed on Xwayland. That the frame client does the same under Xorg is not read |
| `_NET_WM_PID` on the Settings window | **the new tray's pid**, as `ps -C liro-bridge` gives it at that login. **Least certain of the added rows**: xmessage set none (D-401). GTK is expected to set it, but that was not read. If it is absent, the window is attributed by being new between the two trees, with the log's line |
| `WM_CLASS` | **Not predicted**: "liro-bridge"/"Liro-bridge" if GTK takes the binary's name, anything else otherwise. Recorded as read |
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

Rules unchanged. **No apt packages installed this session.** The control's
three `xwininfo` trees went to this session's scratch directory under `/tmp`,
and are gone at the shutdown. `liro-demo/` untouched. Nothing on the
system changed: no linger, no configuration, no session choice. The one
change outside the repository is a note to Claude's own memory: the login
check's third case (`crash`) and the journal's lost half-minute.

---

## G. The Wayland control, taken at this session's own login

The owner chose to take §C1's control **at this login**, not the next. The
control reads a window in the current session and carries nothing across a
boundary, so the login it is taken at does not matter. This login is Wayland
(`loginctl show-session 2 -p Type` → `wayland`, `DISPLAY` `:0`), and the tray
is pid 2400, started by autostart. Checked before writing this: `xwininfo
-root -tree` reaches Xwayland on `:0`, exit 0, 19 windows. The count of
`liro` in it was **not** read.

**Predictions, written before the steps:**

| step | read | predicted |
|---|---|---|
| 0. no Liro window open | `xwininfo -root -tree \| grep -i -c liro` | **0**, with `xwininfo`'s own exit 0 checked separately. `grep -c` prints 0 on an error too (D-304, the second question) |
| 1. positive control: the owner runs `xmessage -name liro-control -title liro-control 'Liro control - close me'` in their own terminal | the same `grep`, without `-c` | **one or more lines naming `liro-control`**. That is an X client on this Xwayland, which is where a GTK window that fell back to X11 would be. This is the step that shows the check can say yes here |
| 2. the owner closes the xmessage, then opens **Settings** from the tray | the log's last lines; the same `grep -c`; `ps -C liro-bridge` | log: "settings: window open" with a new time; **0**; still one tray, 2400 |
| 3. the owner closes Settings with **Cancel** | the log | "settings: the page sent", `type` `cancel`. Nothing saved |

**Least certain: step 1.** Whether `xwininfo -tree` prints `xmessage`'s
`-name` and `-title` as expected. `-tree` prints the window name and the class
pair, and both are set, so one line at least. If it prints nothing, the
control has no positive and step 2's 0 counts for nothing.

What this cannot see: the tray's own `DISPLAY` and `WAYLAND_DISPLAY`.
`/proc/2400/environ` is root's, because the agent is not dumpable (D-376).
That it was given `:0` and `wayland-0`, like this terminal, is assumed from
its having been started by the same gnome-session.

**Measured**, 20:06–20:17, tray 2400 throughout:

| step | predicted | measured |
|---|---|---|
| 0. no Liro window | 0; exit 0 | held: exit 0, 19 windows, **0** at 20:06:31 |
| 1. xmessage `liro-control` open (least certain) | one or more lines | held: exit 0, 27 windows, **two** lines: the client `("liro-control" "Xmessage")` and **Mutter's frame**, `("mutter-x11-frames" …)`, named after its title. The frame was not predicted |
| 2. Settings open | "settings: window open"; 0; one tray | held: the line at 20:10:50.136; exit 0, 20 windows, **0**; tray 2400; 7 descendants, a web process at 73 MB PSS |
| 3. closed with **Zatvori** | `type` `cancel` | held: 20:16:26.705; `config.json` unchanged since 18:12:48 |

**The check was weaker than its prediction said**, and D-304's second question
found it only after step 2. The positive control proved that `xwininfo` sees
an X client on this Xwayland. It did not prove that the Settings window, had
it been on X11, would match "liro": its title is "Podešavanja", and nothing in
the code names the program. **The reading that does not depend on a name**:
the step-2 tree has **no window ID that the step-1 tree lacked**. Against step
0 the only new window is `0xa00009`, 1×1, unnamed, present since step 1, in
Mutter's frame client's range (`0xa00002`–`0xa00005`). So while Settings was
open, no X window appeared at all. The id range is an inference; the window
has no name, class or pid.

**Not predicted, recorded for D23 and not chased:** after the window closed,
the web process was gone and one `bwrap → bwrap → xdg-dbus-proxy` chain
(5237–5239) remained. That is one window, one chain surviving.

**The xprop row, tried on the control:** `WM_CLASS` read `"liro-control",
"Xmessage"`; `_NET_WM_PID` was **not found**. Each client sets that property
itself, and xmessage does not.
