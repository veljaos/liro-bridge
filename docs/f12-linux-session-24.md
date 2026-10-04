# F12 Linux — session 24: the Fedora sitting for D32's runs and dev.15's walk-through

**What this is:** the handover after one sitting on the Fedora VM, written
there at its end (D-423). **Written:** 2026-10-04. **Working rules:**
session 13 §C, session 15 §E, session 22 §E, session 23 §E, and §E below.
D-304's five questions before believing any check.

---

## A. Where the Fedora VM is left

| | |
|---|---|
| package | `liro-bridge-0.9.9~dev.15-1`, `/usr/bin/liro-bridge` `44f448f7…5ec7` |
| WebKitGTK | `webkitgtk6.0-2.54.0-2.fc44` — **not updated**; the update is a system change, the owner's |
| agent | none running at the end (agent D ended by SIGTERM, 19:24:20); the next login autostarts one |
| pairings | `2afcb9c2…`, `fffb83a5…`, `b654c2e6…` — test pairings, no secret held anywhere; **the owner's to revoke, left for now** |
| files | `~/s24-predictions.md` (every prediction and reading); `~/s24-d32/` (probe reports, `ctxprobe.py`); `~/s22-card/ugovor-signed.pdf` (D33's) |

## B. What dev.15 showed on Fedora (D-423)

D29, D27, D31, D28, D33 held, each on some agent's first window; every
second window white, recorded as D32's. D28's reading here cannot fail for
dev.15's reason (it cleared under dev.14 too). D33 under four
terminal-started agents, a deviation the owner accepted.

## C. D32: next, in this order (the owner)

**Five candidates out, none of them the cause.** What is left is per process
and below the WebKit context.

1. **GSK_RENDERER — two runs of `scripts/ctxprobe/ctxprobe.py`, no system
   change.** The same file is in `~/s24-d32/` already (`65ed76f7…cacc9`).
   Predictions first, least certain named.
   ```
   cd ~/s24-d32 && GSK_RENDERER=cairo python3 ctxprobe.py --context default --seq "w w" --label G0
   cd ~/s24-d32 && GSK_RENDERER=ngl   python3 ctxprobe.py --context default --seq "w w" --label G1
   ```
   `cairo` first, as the control. Proposed, not yet agreed: `GSK_DEBUG=renderer`
   on both, so the renderer that drew is read — which one GSK uses by
   default here has not been read.
2. **The WebKit update** (2.54.1), on the owner's approval, as a boot's one change.
3. A fresh `WebKitNetworkSession` per view — not written.

## D. For the Ubuntu VM

D28 on Xorg (decides it) and on Ubuntu's Wayland; D31's export and R7;
D31, D33 and D27 on GNOME 46; dev.15 installed there. And D31's footer: a
proposal for dev.16, shown to the owner as a picture, not a description.

## E. Rules added in session 24

- **Say before a command puts a secret on screen**, not only where it goes.
  The pairing one-liner printed the secret to the owner's terminal; "nothing
  to copy" was true and "nothing shown" was not said (D-423).
- **No busy-wait, even bounded.** Waiting for a process to exit is a re-read,
  not `while kill -0`.
- **A VM's clock can jump mid-sitting.** Before reading `ps` start times,
  compare `/proc/uptime` with the boot; use the log's wall-clock stamps when
  they disagree (two jumps in this sitting).
- **An agent run from a terminal is ended by SIGTERM to its exact PID**, never
  by closing the terminal or Ctrl+C: it handles only SIGTERM, and anything
  else ends it with no line and a stale discovery file (agent B).
- **A pairing's secret goes into the shell that will use it, in one step**, and
  that terminal is not cleared or closed until the request is done.
