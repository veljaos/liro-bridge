# F12 Linux — session 26: D32's close on the Fedora VM under dev.17

**What this is:** the handover after one sitting on the Fedora VM (D-426).
**Written:** 2026-10-05. **Working rules:** session 13 §C, session 15 §E,
session 22 §E, session 23 §E, session 24 §E, session 25 §E. D-304's five
questions before believing any check.

---

## A. Where the Fedora VM is left

| | |
|---|---|
| package | `liro-bridge 0.9.9~dev.17` (rpm `6952c6a6…e7cc`), `/usr/bin/liro-bridge` `dc778885…633e`; dnf transaction 9; `rpm -V` clean |
| platform | Fedora 44, kernel 7.2.7-200.fc44, GNOME Shell 50.5, GTK 4.22.5-2.fc44, WebKitGTK 2.54.0-2.fc44, Wayland; text-scaling-factor 1.0 |
| boot | `8eee1feb…`, started 20:33:24 CEST after dev.17's install, the boot's one change |
| agent | 8627 `open` (became the agent at 21:34:35 from an Activities launch, D27), dev.17; 2330, which drew D-426's and D-428's windows, quit at 21:30:02 — the log says Izađi by pointer, the owner remembers closing the window (D-429, not reconciled) |
| card | the Pošta card in the passed-through reader, attached during the sitting |
| pairings | none: the four test pairings revoked by the owner in Podešavanja, `pairings.json` empty and no item of ours in the keyring (D-429) |
| files | `~/s26-predictions.md` (every prediction and reading); `~/Documents/liro-audit-20261005-212209.jsonl` and `-report.json` (0600, D-428); `~/s26-pair.py` (the pairing helper: the secret to `eval` only); `~/s22-card/` unchanged — every request was refused |
| not returned to stock | by the owner's choice: dev.17 and SafeSign stay installed; GNOME Software's `download-updates` still `false` at D-429 — the owner's to set (F10) |

## B. What this sitting showed (D-426)

**D32 is closed.** Under one dev.17 agent started by autostart, a pairing
and three requests — four windows — each drawn, watched by the owner.
On dev.15 on this machine, every window after the first was white.

| | window | the owner | the log |
|---|---|---|---|
| D1 | pairing | painted, six digits, "Uspešno povezano" | portal WARN (B29); "GTK had no gtk-xft-dpi, so it is taken from the desktop's text-scaling-factor (D32's workaround, D-424)" 1 → 98304; paired |
| D2 | consent | drawn, application and certificate shown; Otkaži | CONSENT_DENIED |
| D3 | consent | drawn; Otkaži; the client HTTP 403 | CONSENT_DENIED |
| D4 | consent | drawn; Otkaži; the same | CONSENT_DENIED |

One DPI line in the process, none on a later window, no "text scaling
changed". **No prediction failed**; the least certain, D2, held. One slip,
the owner's: the reader was not attached at the first request, which ended
CERT_NOT_FOUND before any window opened (`protocolserver.go:69–77`; the
owner saw none), and the request was run again.

**What stays open** (D-426): the defect is WebKitGTK's (open-items D36),
its report drafted and not filed; the workaround cannot act on a desktop
without `text-scaling-factor`; the live-follow path is tested, not
watched; footer A and dev.17's trigger lines not seen on Fedora.

## C. Next

1. **Done after the sitting: the F12 report** brought up to date through
   D-426 (D-427). Its exit-condition box for Fedora is left to the owner.
   Writing it found D-419's condition for Linux being done not shown.
   **Taken the same evening (D-428)**: under agent 2330, the main window,
   Podešavanja with the export (`~/Documents/liro-audit-20261005-212209*`,
   both 0600, nine entries, chain intact), Sertifikati and Dnevnik
   revizije, each drawn, watched by the owner — eight web windows under one
   agent. Open-items F8 closed; the report updated through D-428.
2. **The upstream WebKit report** (D36): its reproducer run once on
   Fedora as written, then filed by the owner.
3. **F10** (D-429): the pairings revoked; the package kept by the owner's
   choice; `download-updates` back to `true` — the owner's hands. The
   owner closes the pairing terminal and deletes `~/s26-pair.py`.
4. **The exit condition's Fedora half stays unticked by the owner's
   decision (D-429)** until F1's two checks are done.

## D. Rules added in session 26

None. The slip was a precondition (the reader) not read before a go; the
owner named it as theirs.
