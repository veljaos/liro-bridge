# F12 Linux — session 25: the Ubuntu sitting for dev.16, D32's Ubuntu half and D28 on Xorg

**What this is:** the handover after one sitting on the Ubuntu VM, written
there at its end (D-425). **Written:** 2026-10-04. **Working rules:**
session 13 §C, session 15 §E, session 22 §E, session 23 §E, session 24 §E,
and §E below. D-304's five questions before believing any check.

---

## A. Where the Ubuntu VM is left

| | |
|---|---|
| package | `liro-bridge 0.9.9~dev.16` (deb `8805f486…12ef`), `/usr/bin/liro-bridge` `44bb9720…738c` |
| GTK / WebKitGTK | 4.14.5+ds-0ubuntu0.10 / 2.52.6-0ubuntu0.24.04.1, unchanged since 2026-09-20 |
| session | "Ubuntu on Xorg" (session 12), boot `588c0906…` |
| agent | tray 7242, autostarted 21:37:18, its window open with one queued document (the W7 drop), nothing signed |
| card | in the reader |
| files | `~/s25-predictions.md` (every prediction and reading); `~/Documents/liro-audit-20261004-211259.jsonl` (**0664**, D35) and `-report.json` (0600), the owner's export |

## B. What dev.16 showed on Ubuntu (D-425)

W0–W5 and W7 walked; W6 (D33 with the card) not this sitting. **D28 fixed on
Xorg** against dev.14 with the platform unchanged — closed. D27 watched on
GNOME 46; D31's R7 performed and footer A confirmed (the owner). D32's
workaround does not act here (GTK 4.14 has 98304 from GSettings, never asks
the portal unsandboxed — X1), and WebKitGTK 2.52.6 has the defect (X2):
Ubuntu unexposed, not safe.

## C. Unexplained, and first: the 21:36:47 quit (D34)

The log says "the person quit the agent from its own window"; no person did.
Nothing in the code, the previous session's transcript or the journal's
timeline accounts for it. Settings' 21:18:02 cancel is the same shape, and
its log line has two routes. **dev.17's trigger logging exists because of
this.** Read D-425's first section before anything else in this list.

## D. Next

1. **dev.17's code is committed, not built** (D-425's last section): the
   audit export's entries file 0600 (D35); the quit and Settings' closes say
   how they were triggered, the quit line no longer asserting a person
   (D34); every control failed, both views green. **Next**: build dev.17,
   install it here, and the owner watches Izađi by mouse and by Tab then
   Enter, Zatvori and Settings' ×, and an export's mode on disk.
2. **Fedora: D32's close under the real agent** with dev.16 (or dev.17):
   a pairing and three requests under one agent, each drawn, the agent's
   DPI line read ("GTK had no gtk-xft-dpi, so it is taken from the
   desktop's text-scaling-factor …", `xftdpi_linux.go:124`, expected there),
   watched by the owner. Footer A seen there too. Fedora has no Go.
3. **The upstream WebKit report** (`docs/reports/webkitgtk-xft-dpi-unset-zoom.md`):
   its minimal reproducer run once on Fedora as written, then the owner files it.

## E. Rules added in session 25

- **A prediction carried forward is a quotation.** Before a prediction's
  wording is used a second time — from an earlier section, an earlier
  session or a handover — read the code it names again. The handover line
  failed twice for this (W1, W7d), W3b's chooser once.
- **A log line's wording is a claim; read every route into it before
  reading it as what a person did.** Settings' `OnClosed` logs "the page
  sent cancel"; the quit line says "the person" for a mouse, a key or an
  accessibility press alike.
- **Times in the predictions file come from `date -u`**, never estimated;
  "21:4x" ran past the clock this sitting.
- **Probes run under `env -i`** with only the display variables, crash
  paths included: X2's unplanned crash ran from the shell (D-425).
