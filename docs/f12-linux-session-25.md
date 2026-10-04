# F12 Linux — session 25: the Ubuntu sitting for dev.16, D32's Ubuntu half, D28 on Xorg, and dev.17

**What this is:** the handover after one sitting on the Ubuntu VM, written
there at its end (D-425). **Written:** 2026-10-04. **Working rules:**
session 13 §C, session 15 §E, session 22 §E, session 23 §E, session 24 §E,
and §E below. D-304's five questions before believing any check.

---

## A. Where the Ubuntu VM is left

| | |
|---|---|
| package | `liro-bridge 0.9.9~dev.17` (deb `8fb0ceb0…31e9`), `/usr/bin/liro-bridge` `dc778885…633e`; built at `e87974a`, not modified |
| GTK / WebKitGTK | 4.14.5+ds-0ubuntu0.10 / 2.52.6-0ubuntu0.24.04.1, unchanged since 2026-09-20 |
| session | "Ubuntu on Xorg" (session 12), boot `588c0906…`; dev.17 installed in this boot, no reboot since (D-425 says why that was enough) |
| agent | 18837 (`open`, dev.17), started 22:25:47 from Activities, its main window open, empty list |
| card | in the reader |
| files | `~/s25-predictions.md` (every prediction and reading); `~/Documents/liro-audit-20261004-211259.jsonl` (dev.16's, **0664**) and `-report.json`, `liro-audit-20261004-222752.jsonl` and `-report.json` (dev.17's, both 0600); `dist/linux/` dev.17's deb and rpm (`6952c6a6…e7cc`) |

## B. What this sitting showed (D-425)

**dev.16 on both of Ubuntu's sessions.** D28 fixed on Xorg against dev.14
with the platform unchanged, and closed. D27 watched on GNOME 46. D31's R7
performed, and footer A confirmed by the owner. D32's workaround does not
act here: GTK 4.14 has 98304 from GSettings and never asks the portal
unsandboxed (X1). WebKitGTK 2.52.6 has the defect (X2), so Ubuntu is
unexposed, not safe.

**What dev.17 carries** (`dd87a71`, built at `e87974a`):
- the audit export's entries file written 0600, as the report and the store (D35);
- Izađi and Zatvori send what the page saw of the click, and Go classifies it
  as `pointer`, `key`, `neither-pointer-nor-key` or `script`;
  `closed-from-outside` is Go's alone (D34);
- the quit line no longer says a person: "Izađi in the agent's own window
  sent quit, so the agent stops" `trigger=…`;
- Settings logs Zatvori ("Zatvori sent cancel" `trigger=…`) and a close from
  outside ("the window was closed from outside the page, not by Zatvori")
  differently.

**dev.17 walked on the installed build, Xorg, by the owner's hands:**

| | what the owner did | the log |
|---|---|---|
| Wd | Izađi with the mouse | `trigger: pointer` |
| We | no mouse; Tab ×6, Enter on Izađi | `trigger: key` — the least certain, held |
| Wf | Podešavanja, Zatvori with the mouse | "Zatvori sent cancel" `trigger: pointer` |
| Wg | Podešavanja, the title bar's × | "closed from outside the page, not by Zatvori" |
| Wh | an export to ~/Documents | both files 0600; the 21:12 export's `.jsonl` 0664 beside them |

**What tonight settled: on the real build, the log can tell a mouse from a
key, and Zatvori from a close.** An accessibility press is expected as
`neither-pointer-nor-key` and was not produced. **The 21:36:47 quit stays
unexplained, and that is why the logging exists.** It is the second
unexplained action of the sitting and the third of its shape with D18, and
in all three the log said who without knowing (D34). The next one will be
placed. This one will not.

## C. Unexplained: the 21:36:47 quit (D34)

The log said "the person quit the agent from its own window", and no person
did. Nothing in the code, the previous session's transcript or the journal's
timeline accounts for it. Settings' 21:18:02 cancel is the same shape, and
its log line had two routes. Read D-425's first section before anything
else here.

## D. Next

1. **Fedora, one job: D32's close under the real agent.** dev.17 installed
   there (the rpm above) **as that boot's one change**, checked against
   dev.15's own file first (D-391). Then a pairing and three requests under
   one agent, each drawn, watched by the owner, and the agent's DPI line
   read. Expected there: "GTK had no gtk-xft-dpi, so it is taken from the
   desktop's text-scaling-factor …" (`xftdpi_linux.go:124`). Footer A and
   dev.17's trigger lines may be seen in passing, but they are not that
   sitting's job. Fedora has no Go.
2. **The upstream WebKit report** (`docs/reports/webkitgtk-xft-dpi-unset-zoom.md`):
   its minimal reproducer run once on Fedora as written, then the owner
   files it. Only if it fits beside item 1 without becoming a second change.
3. **Footer, when it is next looked at (the owner, not now):** the Tab order
   is Promeni, Izaberi, Podešavanja, Sertifikati, Dnevnik revizije, Izađi —
   six presses to Izađi, with the doors first. Whether that is right is
   open under D31.

## E. Rules added in session 25

- **A prediction carried forward is a quotation.** Before a prediction's
  wording is used a second time, read the code it names again, whether it
  comes from an earlier section, an earlier session or a handover. The
  handover line failed twice for this (W1, W7d), W3b's chooser once.
- **A log line's wording is a claim; read every route into it before
  reading it as what a person did.** Settings' `OnClosed` logged "the page
  sent cancel", and the quit line said "the person" for a mouse, a key and
  an accessibility press alike.
- **Times in the predictions file come from `date -u`**, never estimated.
  "21:4x" ran past the clock this sitting.
- **Probes run under `env -i`** with only the display variables, crash
  paths included. X2's unplanned crash ran from the shell (D-425).
- **A control's edit is grepped before its run is read**, and a
  before/after comparison is made in a clean `git worktree`, not through
  `git stash`. C1's `sed` did nothing, and the stash left untracked tests
  behind (D-425).
- **No backticks in an unquoted heredoc.** One ran as a command and dropped
  a word from the predictions file.
