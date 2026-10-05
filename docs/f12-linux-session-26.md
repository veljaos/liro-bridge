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
| agent | 2330 `tray` from autostart (parent `systemd --user`), dev.17 `e87974a`; its `pkcs11-worker`s 4531 (opensc) and 4543 (libaetpkss) |
| card | the Pošta card in the passed-through reader, attached during the sitting |
| pairings | `12c4e8b03bdb68a0f14814b1cdf19571`, this sitting's; its secret only in the environment of the owner's open terminal, printed nowhere. With session 24's three (`2afcb9c2…`, `fffb83a5…`, `b654c2e6…`) all test pairings, **the owner's to revoke** |
| files | `~/s26-predictions.md` (every prediction and reading); `~/s26-pair.py` (the pairing helper: the secret to `eval` only); `~/s22-card/` unchanged — every request was refused |
| still as the phase set it | GNOME Software's `download-updates` `false` (open-items F10) |

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

1. **The F12 report** (`docs/f12-report.md`, open-items F3) brought up to
   date from D-417 to D-426 — the owner's last item for F12.
2. **The upstream WebKit report** (D36): its reproducer run once on
   Fedora as written, then filed by the owner.
3. **When Fedora's work is done** (F10): `download-updates` back to `true`,
   liro-bridge uninstalled, the test pairings revoked — the owner's hands.

## D. Rules added in session 26

None. The slip was a precondition (the reader) not read before a go; the
owner named it as theirs.
