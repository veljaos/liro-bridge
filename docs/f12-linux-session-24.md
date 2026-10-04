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
| files | `~/s24-predictions.md` (every prediction and reading); `~/s24-d32/` (probe reports and snapshots, G0–K6; `ctxprobe.py` `b58ef17f…63ba`); `~/s22-card/ugovor-signed.pdf` (D33's) |

## B. What dev.15 showed on Fedora (D-423)

D29, D27, D31, D28, D33 held, each on some agent's first window; every
second window white, recorded as D32's. D28's reading here cannot fail for
dev.15's reason (it cleared under dev.14 too). D33 under four
terminal-started agents, a deviation the owner accepted.

## C. D32: found as far as this program reaches (D-424)

**Superseded the same evening**: the order D-423 gave (GSK_RENDERER, then the
WebKit update) was taken and overtaken. G0 ruled out GTK's renderer; K0–K6
found the value: GTK has no `gtk-xft-dpi` (−1) because the Settings portal
refuses our non-dumpable process, and with −1 WebKitGTK 2.54.0 gives the
first view zoom 1.0 and every later one NaN. K5 and K6: the DPI value alone
decides it, both ways; D-376's flag stays.

1. **dev.16, on the Ubuntu VM — (c), a workaround, called one** (D-424):
   first read `gdksettings-wayland.c` for how GDK turns `text-scaling-factor`
   into `gtk-xft-dpi` (not read; 1.0 → 98304 confirms only the shape); then,
   after `gtk.InitCheck()` and only if `gtk-xft-dpi` is −1, set it from
   `org.gnome.desktop.interface text-scaling-factor` read directly, follow
   the key live, leave −1 and log it when the schema is missing, one log line
   when it acts. A unit test of the rule, each with a failing control.
2. **Read `gtk-xft-dpi` on Ubuntu** (the owner): it refuses the portal too and
   does not show D32 — does it get −1 and survive, or a real value?
3. **Draft the upstream WebKit report** for the owner to file, with
   `scripts/ctxprobe/ctxprobe.py` and K3/K6 as the reproducer.
4. **D32's close on Fedora** with dev.16 installed: a pairing and three
   requests under one agent, each drawn, watched by the owner.
5. **Open, the actual defect**: why view 1 survives −1 and view 2 does not.
   The WebKit update is no longer the next step; it may still change the
   answer.

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
