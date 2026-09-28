# F12 Linux — session 10

**What this is:** the tenth session on the Linux VM, and the handover to the
eleventh. **It ends at a logout**, like session 9: C3 needs a tray started by
a login, and the last one was ended by D20's SIGTERM. The next session starts
at §C, whose predictions were written here, before the logout — measure
against them, do not rewrite them afterwards. Session 9 §C's own predictions
for C3 and C4 stand unchanged; this document adds to them and does not
restate them.

**Written:** 2026-09-28, Ubuntu 24.04.5 VM, HWE kernel 7.0.0-34-generic.
**Entries:** D-399. Master at the end of the session: the commit carrying
this document.

---

## A. The state

- **Installed:** `liro-bridge 0.9.9~dev.12` (`3a1037a`), unchanged.
- **CI 36465789774** on `70bd443`: all nine jobs green (D-399).
- **Session 9's "logout" was a restart** (D-399). C1's stale-file check is
  void — a reboot empties `/run/user/1000` whatever a logout does. **The
  question it existed for is unanswered**: whether a logout with
  `KillUserProcesses=no` ends the user manager and unmounts the directory.
- **D20 closed** (D-399): SIGTERM to a tray with both workers and a
  Certificates window open — all ten processes gone within 0.2 s, the file
  removed, D-394's two lines logged. Which path ended the workers is inferred,
  not observed.
- **At the logout ending this session:** **no tray running** (ended by D20's
  SIGTERM at 18:46:54). **No discovery file**: `/run/user/1000/liro/` is
  empty. `Linger=no`. The reader was attached after the restart.

---

## B. What changed in the method

**Every login is checked to be one first.** Session 9's handover could not
tell a logout from a reboot, and nothing at the login did either; it was found
by chance. So the first read at every login, before anything else:

```
uptime -s; last -x -F | head -6; loginctl list-sessions --no-legend
```

A logout leaves `uptime -s` at **18:39:43** (this boot) and a `last` line for
the old tty2 session ending in a time, not `down`. Anything else: stop, say
so, and nothing measured at that login counts for a logout.

**The instrument for the runtime directory is its birth time**, not the stale
line: `stat -c '%n birth=%w' /run/user/1000`. At this boot it reads
**18:39:58.19**, the second of the login in `last`, so it can see a mount. A
new birth time after a logout is the tmpfs mounted again; an unchanged one is
the directory kept. It needs no stale file, and after D20 there is none.

---

## C. What the next session does first, with the predictions

Session 9 §C's rules apply: exact pid only, `ps -C liro-bridge`, never
`pgrep -f`/`pkill -f`, not `ss -p`, `proctree.py` for descendants,
`journalctl --user -o cat`.

### C0. The login after this session's logout — C3a's control

Nothing of this program is running at this logout, so this login answers C1's
question on its own and is C3a's control: C3a differs from it by one thing, a
tray running at the logout. Read, change nothing:

```
uptime -s; last -x -F | head -6; loginctl list-sessions --no-legend
stat -c '%n birth=%w' /run/user/1000
loginctl show-user vboxuser -p State -p Linger
ps -C liro-bridge -o pid,ppid,lstart,args; cat /proc/PID/cgroup
ls -la --time-style=full-iso /run/user/1000/liro/ && cat /run/user/1000/liro/bridge.json
grep -n 'starting\|stale\|listening' ~/.local/state/liro/logs/bridge.log | tail -3
```

| | predicted |
|---|---|
| a logout, not a reboot | `uptime -s` 18:39:43; the old tty2 line ends in a time |
| **`/run/user/1000`'s birth time** | **new**: the second of the new login, not 18:39:58. **Least certain**, and it is C1's question: with `KillUserProcesses=no`, a process of the old session surviving the logout keeps the session "closing" and the user — and the directory — alive. The likeliest such process is this session's own, Claude Code and the terminal it runs in. If the birth time is unchanged, `loginctl list-sessions` shows the old session and `loginctl session-status ID` lists what held it: **that is the finding** |
| old sessions | none left; one session, the new one |
| a tray | exactly one, `tray`, by autostart (`app-gnome-liro\x2dbridge-PID.scope`), as at session 9's restart |
| `bridge.json` | new, dev.12, 17580 |
| the stale line | absent — **and it cannot count either way**: there was no file to be stale. Recorded only so nobody reads its absence as a result |

### C3a and C3b, as session 9 §C3, with two reads added

Before and after each logout, add the first-read block from §B and the birth
time. Predictions added here, before either logout:

| | C3a (linger off) | C3b (linger on) |
|---|---|---|
| `/run/user/1000`'s birth time across the logout | **new** — as C0, unless C0 found otherwise, in which case C3a predicts what C0 measured | **unchanged** — linger keeps the user manager and its directory |

If C0's birth time was **unchanged**, stop before C3a: the premise of C3a's
control is gone, and the owner decides whether to go on.

`loginctl disable-linger vboxuser` after C3b, **always**, and
`loginctl show-user vboxuser -p Linger` → `no`.

### C4. E9 — unchanged from session 9 §C4.

---

## D. Instruments that failed this session

| the instrument | what it reported | what was wrong |
|---|---|---|
| session 9 §C1's stale line | absent, as predicted | the login followed a restart; a reboot empties the directory, so the check could not have failed (D-399) |
| session 9 §C's "a reboot drops" the reader | — | after the restart the reader was attached (D-399) |

---

## E. Owed to the owner, needing no logout

Unchanged from session 9 §E: **D24's amendment**, drafted and shown before it
is written; **E9's options**, after C4.

---

## F. This machine

Rules unchanged. **No apt packages installed this session.** Nothing left in
`/tmp` by this session. `liro-demo/` untouched.
