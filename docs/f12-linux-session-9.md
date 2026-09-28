# F12 Linux — session 9

**What this is:** the ninth session on the Linux VM, and the handover to the
tenth. **It ends at a logout**, because D20 and B19's linger half need a live
tray watched across logouts, and a logout ends the session doing the watching.
So the next session starts with §C, whose predictions were written here,
before the logout — measure against them, do not rewrite them afterwards.

**Written:** 2026-09-28, Ubuntu 24.04.5 VM, HWE kernel 7.0.0-34-generic.
**Entries:** D-396 to D-398; and the D21 fix (`f5c7269`). Master at the end of
the session: the commit carrying this document.

---

## A. The state, in one page

- **Installed:** `liro-bridge 0.9.9~dev.12` (`3a1037a`), which carries D-394's
  SIGTERM handler. The sandbox check on it reads exactly as D-390's (D-397).
- **B19's fallback path measured** (D-396): with `XDG_RUNTIME_DIR` unset the
  tray starts, writes `~/.local/state/liro/bridge.json`, leaves it after a
  kill, replaces it as stale, removes it on SIGTERM. The two-user part is
  **deliberately not measured**, by the owner's decision. **Linger is left.**
- **C12 closed** (D-397): both demos to the end on Linux, on the owner's Pošta
  card through the installed agent, verified by both tools.
- **C2 narrowed** (D-397): N = 9 windows, no web process survived. **D23
  opened**: a bubblewrap/`xdg-dbus-proxy` chain survives about one window in
  two, 1 → 2 → 4, about 0.86 MB PSS each.
- **A12 measured**: a protocol run's corner became the owner's default.
- **D21 fixed** — and the fix found that the claim was an invention: nobody
  ever measured the Windows edit control's memory (D-397).
- **Done before the logout** (D-398): `:1.3` is GNOME Shell as IBus's panel;
  **A3 decided** — the keystroke path conceded in clause 3, not a third
  exception; **D22 written** — SPEC §6.5.1 clause 3 amended. **B23 stays
  open.** **D24 opened**: clause 2's first exception names a control Linux
  does not have — its own amendment, drafted and shown first, not yet.
- **Opened and not yet worked:** E9 (SPEC §6.5.2's X11 refusal has no code
  behind it) — §C4.

---

## B. What the owner decided this session

- Linger: **yes**, after D20, reversible with `disable-linger`.
- Two users: **not measured**, recorded with the reasoning (D-396).
- A29: left. C2: the narrower form, with N stated and "not the day" said.
- B23: establish `:1.3`, then bring A3 as a decision with the argument that on
  Wayland the compositor sees every key by construction. **Do not instrument
  Mutter.**
- D22: open it today; draft the amendment with A3; **show both before
  writing either.**
- D21: fix it, own commit — and say it was an invention.
- **E9: yes, measured, on its own login.** *"If the agent runs normally under
  Xorg, say so plainly rather than softening it, and bring me the options —
  refuse to start, warn, or amend the clause — with what each costs."*

---

## C. What the next session does first, with the predictions

**Rules for all of it.** One change per login, checked against the
predecessor's own file (D-391). Exact pid only; find the tray with
`ps -C liro-bridge -o pid,ppid,lstart,args` — by name, never `pgrep -f` or
`pkill -f`, which match the asking shell (D-393). **Not `ss -p`**: it cannot
see the agent's sockets (D-396); use `scripts/proctree/proctree.py PID`. The
tray's log is `~/.local/state/liro/logs/bridge.log` (JSON lines). The journal:
`journalctl --user -o cat` (the hostname matches "liro"; memory
`journal-grep-hostname-matches-liro`).

**The state at the logout:** no tray running. `/run/user/1000/liro/bridge.json`
**stale**, mtime 17:39, naming 17580 — left by the tray that died with its
terminal at 18:13:00 (D-397). `loginctl show-user vboxuser -p Linger` → `no`.
`KillUserProcesses` is the default (`#KillUserProcesses=no` in
`/etc/systemd/logind.conf`). The card reader is passed through (a logout does
not drop it; a reboot does).

### C1. The login that ended session 9 (linger off)

Read, do not change anything:

```
loginctl show-user vboxuser -p State -p Linger
ps -C liro-bridge -o pid,ppid,lstart,args
ls -la --time-style=full-iso /run/user/1000/liro/ && cat /run/user/1000/liro/bridge.json
grep -n 'starting\|stale\|listening' ~/.local/state/liro/logs/bridge.log | tail -5
```

| | predicted |
|---|---|
| a tray | exactly one, `"/usr/bin/liro-bridge" tray`, started at the login by autostart, not from a terminal |
| `bridge.json` | new mtime, `0.9.9-dev.12`, 17580 |
| **the stale line** | **absent** from this start's log: `/run/user/1000` is a tmpfs unmounted when the user's last session ends (D-325), taking the 17:39 file with it. **Least certain** — whether the last session really ended: with `KillUserProcesses=no` a process surviving the logout keeps the user manager, and the directory, alive. If the stale line is there, that is the finding, and `loginctl` / the journal around the logout say what held it |

### C2. D20: SIGTERM to a tray whose workers are running

1. The owner opens **Certificates** from the tray (card in). That starts one
   `pkcs11-worker` per module — two on this machine, OpenSC and SafeSign
   (D-393, D-397). The Certificates window may be closed or left open; say
   which.
2. Before: `python3 scripts/proctree/proctree.py PID before` — expect the two
   workers (`cat /proc/WPID/cmdline` names each module), and WebKit's network
   process and a proxy chain if a window has opened.
3. `kill -TERM PID` — the exact pid.
4. After, within a few seconds: `proctree.py PID after`, each earlier
   descendant's pid by `/proc/PID`, the discovery file, the log's last lines.

| | predicted |
|---|---|
| the log | ends "tray: asked to terminate, so stopping the way Quit does", "protocol: stopped" (D-394) |
| `bridge.json` | gone |
| both workers | gone — through `closePKCS11Modules` now, not their pipe's end-of-file. Whether that logs a line is **not known**; judge by the pids |
| WebKit network process, proxy chains | gone — they were gone after the 18:13 death too (D-397), though by a different path |
| **least certain** | that the workers end *promptly*. A worker blocked inside a module call returns only when the module does (D-393); an idle one should not be. Give it 10 s and record the time |

Then **D20 closes** if all hold, on the installed dev.12. What it cannot see:
a worker mid-`C_Login` at the moment of the signal — say so rather than imply
it.

### C3. Linger — two logouts, one change each

A tray must be running at each logout, started by a login, so: **log out and
in once more** to get one (nothing measured at that login beyond C1's reads).
Then:

**C3a. The control: logout with linger off.** Before: the tray's pid, its
descendants (`proctree.py`), `bridge.json`'s content and mtime,
`loginctl show-user vboxuser -p State -p Linger`. Log out, log in. After: the
same reads, and the old pid by `/proc/PID` and in `ps -C liro-bridge`.

| | predicted |
|---|---|
| the old tray | ended at the logout by **SIGTERM**: its log's last lines are D-394's two |
| trays after the login | exactly one, new |
| stale line in the new start | absent |

**C3b. With linger on.** The owner runs `loginctl enable-linger vboxuser`
(`loginctl show-user vboxuser -p Linger` → `yes`). The system change, approved
and reversible. Then the same before, logout, login, after.

| | predicted |
|---|---|
| `/run/user/1000` across the logout | **stays mounted** — that is what linger is |
| the old tray | **least certain of the whole sitting.** Either it is ended by SIGTERM at the logout as in C3a, and then linger changes nothing for this program because the handler removes the file — or it **survives**: it holds no Wayland connection until a window opens, and under linger the session bus it talks to survives too. If it survives, the new login's autostart finds a live agent and hands over and exits (`liveAgent`), and the agent serving the new session is **the one from the old session**. Read: which pid serves 17580 (the log's "listening" line and its start time), how many trays, and — if the old one survived — what happens when the owner opens a window from its icon (does it appear, on which display?) |
| stale file | none either way if the handler ran; a live file naming the old tray if it survived |

Afterwards, **always**: `loginctl disable-linger vboxuser`, and
`loginctl show-user vboxuser -p Linger` → `no`. B19 closes on C3a and C3b.

### C4. E9: the consent window on an Xorg session — its own login

After C3, with linger off again. **First, the control, in the Wayland session**
(before logging out): open Settings from the tray, then
`xwininfo -root -tree | grep -i -c liro`. Predicted **0**: a native Wayland
window is not in the X server's tree, which is what makes the check able to say
yes later. Close Settings.

Then log out, choose **"Ubuntu on Xorg"** at the login screen (the gear icon;
`/usr/share/xsessions/ubuntu-xorg.desktop` exists), log in. Read:

```
echo $XDG_SESSION_TYPE $WAYLAND_DISPLAY $DISPLAY
ps -C liro-bridge -o pid,ppid,lstart,args
```

Open Settings from the tray; `xwininfo -root -tree | grep -i liro`; then
`xprop` on that window id for `WM_CLASS` and `_NET_WM_PID`.

| | predicted |
|---|---|
| session | `x11`, `WAYLAND_DISPLAY` empty |
| the tray | starts and runs normally — nothing refuses X11 |
| the window | **in the X tree**: GTK chose X11, with no line in the agent's log saying so |
| **least certain** | whether anything in GTK or WebKitGTK refuses or complains on X11 in a way the agent turns into a failure. Nothing in this program's code does (D-396); the libraries are not read |

Optionally, the consent window itself: run demo A again (its pairing is kept
in `liro-demo/`) to the approval window, check it the same way, and **refuse**.
**Nothing is clicked by any program, and `xdotool` is not run** — D-094; that
another X client *could* click Approve is X11's design (any client may use
XTEST), stated as reasoning, not demonstrated.

Log out, back to the ordinary (Wayland) session. Then bring the owner the
options with their costs — **refuse to start**, **warn**, or **amend the
clause** — having said plainly what happened if the agent ran normally.

---

## D. Instruments that failed this session

| the instrument | what it reported | what was wrong |
|---|---|---|
| `ss -ltnp`, for the tray's port | no port for the tray's pid | the agent is not dumpable (D-376); `/proc/PID/fd` is root's, and `ss` maps sockets through it (D-396) |
| a diff with bus names filtered out | only the version differed | the filter removed the a11y probe's lines, which carry bus names; compared separately (D-397) |
| `verifypdf … \| grep …; echo $?` | exit 0 on a changed file | `grep`'s status, not the verifier's; its own was 1 (D-397) |
| the C2 control sample's label | "approval window" | the log put it before the job existed: the certificate chooser (D-397) |
| "start the tray from the app grid" | — | the app-grid entry runs `open`, which cannot start a tray (D-397) |
| decisions-corrections.md §1's fix | D-277 is the right citation | D-277 measured nothing either; nobody measured the control (`f5c7269`, D-397) |

---

## E. Owed to the owner, needing no logout

1. ~~B23's step, A3, D22~~ — done in D-398, before the logout.
2. **D24's amendment**: draft it (clause 2's first exception is Windows-only
   since D-385) and show it before writing it.
3. **E9's options**, after C4.

---

## F. This machine

**Rules unchanged:** one process at a time, no load generators, `go test -p 1`,
both test views before claiming green, tooling that touches a secret fails
rather than prompts.

**Installed this session:** `liro-bridge 0.9.9~dev.12` (by the owner). **No
apt packages** — `x11-utils` (for E9) was already there.

**Left deliberately:** `liro-demo/` (gitignored) — **it holds the demos'
pairings, with live device secrets in cleartext**, and today's signed PDFs;
the owner's `config.json` back to `top-right` (byte-identical to the morning);
`~/sandboxcheck-7.0.0-34-generic-20260928-174010.txt`. **In `/tmp`, gone at
the next reboot:** this session's scratch directory — B19's scratch home
(its trays never paired and wrote nothing to the keyring), dev.11 and dev.12
extracted, the C2 log and predictions. The counter is committed as
`scripts/proctree`. **Stale:** `/run/user/1000/liro/bridge.json` (C1 reads
what happens to it).
