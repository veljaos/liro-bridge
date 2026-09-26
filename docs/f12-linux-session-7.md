# F12 Linux — session 7

**What this is:** the seventh session on the Linux VM, and the handover to
the next one. **Read §A first.** Then `docs/open-items.md`, which this
session re-sorted with the owner into three groups (§D here).

**Written:** 2026-09-26, Ubuntu 24.04.5 VM, kernel 7.0.0-31-generic.
**Entries:** [[D-356]] package signing (and the passphrase-window defect),
[[D-357]] the notification, [[D-358]] §9, [[D-359]] §10, [[D-360]] SafeSign
loaded twice and CI's answers, [[D-361]] **the first signature on Linux with
a real card in a real reader**, [[D-362]] load failures translated,
[[D-363]] remove and purge.

---

## A. The state, in one page

- **Package signing is built** and exercised in CI with a throwaway key on
  every push; Fedora's `rpm -K` accepts the signature. The real key has never
  signed: that waits for the `package-signing` environment (open-items A16,
  five steps written there) and the first `v*` tag.
- **The Pošta card signed on Linux** through SafeSign 4.6, reader passed
  through to this VM, installed agent, PIN in this program's own dialog, no
  vendor window. Verified by `scripts/verifypdf` and OpenSSL with failing
  controls. The owner read the stamp: "SAVKA ODŽIĆ". A copy of the signed
  file is `~/Desktop/ugovor-signed-2026-09-26.pdf`.
- **§9** tells a person a stopped `pcscd` with the command that starts it;
  **§10** turns a module that will not load into a sentence in their
  language. Both measured through the real program.
- **§11's uninstall is measured** — every prediction held (D-363). **The
  logout check is not done** (§B).
- **CI green** on everything pushed up to `51b5261`. `2372978` (D-363) and
  this document are **not pushed** unless the owner pushed them before
  logging out.

---

## B. What the next session does first

1. **Read the owner's logout result** — or ask for it. The package was
   purged and `~/.config/autostart/liro-bridge.desktop` left in place
   (`Exec="/usr/bin/liro-bridge" tray`, `TryExec=/usr/bin/liro-bridge`). After
   logging back in, what closes open-items C3:
   - `pgrep -a liro-bridge` → nothing;
   - no dialog or notification about Liro Bridge at login;
   - `ls ~/.config/autostart/liro-bridge.desktop` → still there (nothing
     removes it; that is expected);
   - `journalctl --user -b | grep -i -E 'liro|autostart'` → whatever GNOME
     said, if anything. Silence is the predicted answer (D-355 §8: GIO's
     loader refuses an entry whose `TryExec` is missing).
2. **Reinstall**: `sudo apt install ./dist/linux/liro-bridge_0.9.9-dev.6_amd64.deb`
   (built from `2372978`). **After reinstall the autostart entry is live
   again** and the agent starts at the next login (`startWithWindows` is true
   in the owner's config). Ask the owner whether they want that.
3. **Item 5 before anything else** (the owner's order): SPEC §11.11 on Linux
   — §C.2 below.
4. **The SPEC §6.5.2 amendment** waits for the owner's approval of §C.1's
   text. Do not write it without that.

---

## C. Two drafts that exist nowhere else

### C.1 SPEC §6.5.2 — proposed, not written (it becomes D-364)

The owner asked for rules instead of measurements. **Correction to the
owner's brief, agreed in session:** only one of the three rows is measured
false (the clicked notification), plus the paragraph beneath the table ("no
token without a desktop entry"); row 2 (`present()` on an existing window)
is unrefuted but not re-measured since D-337.

> #### 6.5.2 When the window cannot put itself in front
>
> On Wayland a client cannot raise its own window; the compositor decides. **This subsection states rules, not measurements.** The readings behind it are in D-337 and D-355, with their limits, and they stay there: a reading written into a specification stops being re-measured, and one of this subsection's earlier readings was overturned within a week of being written.
>
> What the rules rest on, and nothing more:
>
> - Whether a **new** window takes focus is the compositor's decision. It has been seen to; nothing below depends on it.
> - An **existing** window cannot be relied on to come forward when this program asks.
> - Whether **clicking a notification** brings the window forward is the desktop's behaviour. It has been seen to, with and without an activation token; nothing below depends on it either way.
>
> Every clause below is a requirement.
>
> - **A new window for every request.** Never a hidden window shown again, never one re-used between requests. It is the one arrangement that does not depend on raising an existing window, and it is what a person sees: a window that appears is one that was not there a moment ago.
> - **A desktop notification alongside, where one can be posted.** It tells the person a request is waiting. It is withdrawn when the request is answered — approved, refused or timed out — not when the window closes (D-341, D-357).
> - *(the remaining clauses unchanged: best-effort; the caller half; nothing depends on the window having been seen; the X11 fallback refused; Windows unchanged)*
>
> *Amended 2026-09-26 (D-364).* The earlier text carried a table of three readings from D-337. The third — a clicked notification raised nothing and carried no token — was contradicted by D-355 §6: a notification with no action raised the window, by a mechanism not yet explained. The sentence beneath it, "no token without a desktop entry", was contradicted by the same readings: a token arrives whenever an action is invoked, with or without the entry. The second row has not been re-measured. The table was removed rather than corrected, because a corrected reading would be as liable to be overturned as the first, and none of the rules depends on any of them. What remains true is that the compositor decides — and the rules were always written so that it could decide either way.

### C.2 SPEC §11.11 on Linux — the sentence, and what blocks it

SPEC §11.11: "On those platforms the agent must tell the user which issuers
are actually supported rather than reporting 'no certificates found'." Today
the Linux `NO_READER` sentence ("Nijedan čitač kartica nije pronađen…") is
what a MUP or Halcom holder sees, **with the reader plugged in**. The owner
ranks this first: the only item that tells a person something untrue about
their own machine.

The owner's shape, with my corrections (a comma before "koji"; "Linuxu", not
"Linux-u" — the earlier Linux sentences should change to match; Cyrillic
"Линуксу" or "Linux-у" is the owner's call):

> *(no module at all)* "Nijedan program za čitanje kartica nije pronađen."
> *(modules found, no signing certificate on any card)* "Ni na jednoj kartici nije pronađen sertifikat za potpisivanje."
> *(both continue)* "Na Linuxu Liro Bridge podržava kartice Pošte Srbije, preko programa SafeSign, koji se preuzima sa sajta Pošte. Kartice MUP-a za sada nemaju podršku na Linuxu, jer MUP ne isporučuje program za ovaj sistem. Za kartice Halcoma podrška na Linuxu još nije proverena."

- **Two variants, because the owner's first sentence ("Nijedan modul…") is
  false** on a machine with SafeSign or OpenSC installed — this one. The
  report carries no count of usable modules yet; adding one is small.
- **The MUP sentence waits on a measurement**: the owner's MUP card in the
  passed-through reader, `liro-bridge certs`. If OpenSC reads it, the true
  sentence is different. F11 §0.1's "MUP ships middleware for Windows only"
  came from a briefing, not a check. Optionally, the vendors' download pages
  (reading public pages only; the owner has not yet said yes).
- **Halcom is unverified** — no card, nobody has looked for a Linux Nexus
  Personal — and the sentence says so, by the owner's rule: telling somebody
  their card is unsupported when it might work is its own kind of wrong.

---

## D. The open list, sorted with the owner

**Group 3 (accepted, recorded, not chased):** A6, A8, A11, A14, A15, B10, B11,
B17, B21, B5's KDE half, C13, D11, D13, D14, D16.

**Group 2 (before v1, not this phase):** everything else not below. **D3 and
D4 are flagged above the rest** — about a signature, and the Windows version
installed today may report what it did not establish.

**Group 1 — before F12 is done.** Can be answered on Ubuntu today:

| | item | takes |
|---|---|---|
| 1 | C3 GNOME and the left-behind autostart entry | §B.1 |
| 2 | A20 SPEC §11.11 on Linux | §C.2; owner's MUP card; code |
| 3 | A1 SPEC §6.5.2 | owner approves §C.1 |
| 4 | B1, B2, then A3 — the real PIN dialog's memory with real keystrokes | owner at the keyboard with `pinmem`, ~1 h |
| 5 | A4 core-dump hardening | owner's decision, then a few lines |
| 6 | D2 (+B20) file chooser, message box, drag and drop on Linux | code; the largest item |
| 7 | D5 the Explorer-menu row in Settings on Linux | hide it; small |
| 8 | B18 the audit `flock` across two processes | measurement; small |
| 9 | B19 discovery files under linger, unset `XDG_RUNTIME_DIR`, two users | owner's hands (a second user) |
| 10 | F6 (B7) the sandbox on a stock 24.04 kernel | install Ubuntu's 6.8 kernel on this VM, reboot into it |
| 11 | F2 the soft token signs on both distributions in CI | CI code |

Not on Ubuntu: F1 real card on Fedora 44 (**risk: SafeSign's Red Hat builds
target RHEL 9/10, not Fedora 44**), B15, B8, F8 with B5's GNOME 50 half, F4
(no Linux module known to kill its worker — find one, or the owner rules),
F5 (a real GPU, or the owner rules the box means "set and respected"), F3
the report, last.

Closed this session: C14 (stamp), C17 (notification), E4 (Pošta card), F7
(struck), C4 (remove/purge), A17 (translation), F1's Ubuntu half.

---

## E. Instruments that failed this session

| the instrument | what it reported | what was wrong |
|---|---|---|
| a negative test of `sign.sh` | a log line | **it opened a passphrase window on the owner's desktop and the owner typed the real signing passphrase into it** (D-356). Every gpg now runs `--pinentry-mode error` |
| a pipe into `grep`, twice | exit 0 | the exit status was `grep`'s — the third time this week (session 6 §F) |
| `opensc-tool -l` | "No smart card readers found" in every state | it hides pcsc-lite's error; a ten-line C client printed the real code |
| my loop setting `PCSCLITE_CSOCK_NAME=` | "Service not available" with pcscd up | the empty variable is an empty path to pcsc-lite — which found a real bug in my check (D-358) |
| `pkill -f` with a path in the pattern | exit 144 | it matched my own shell |
| libsecret's item list after a delete | 1 left | a cached list; a fresh search found 0 |
| the owner's `find -newermt "today"` | nothing | GNU find reads "today" as now |
| my first fixture for a version mismatch | the probe child died | not the fixture's fault — glibc's `ld.so` asserted inside `dlopen` (D-359), and that became a test |

---

## F. This machine

**Rules unchanged:** one process at a time, no load generators, `go test -p 1`.
**New rule from this session** (memory `never-prompt-for-secrets-on-owners-desktop`):
tooling that touches a secret must fail, never prompt — `--batch
--pinentry-mode error`, `sudo -n`.

**Installed this session** (by the owner): `safesignidentityclient
4.6.0.0-AET.000` with `libwxbase3.2-1t64` and `libwxgtk3.2-1t64`. **Left
deliberately:** SafeSign; `pcscd.socket` running; `~/.config/liro/config.json`
(rewritten at 13:15 by the card run's method screen — open-items A12); the
audit log, logs, TSL cache; the autostart entry (for the logout check);
the signed PDF on the Desktop.

**Purged:** `liro-bridge` (reinstall is §B.2). **Removed:** today's test
pairing from the keyring and `pairings.json`; the client log that held its
secret, shredded.

**Gone at the next reboot** (scratch, under `/tmp`): the unpacked `rpm` 4.18
prefix used to test `rpmsign` locally (no sudo here), the throwaway signing
key, the SafeSign packages unpacked for listing, the §11 snapshots. The
evidence is in D-356 to D-363.

**The owner's MUP card** is in the owner's hands, not in the reader; the
Pošta card was in the passed-through Realtek reader (`0bda:0165`, generic
CCID).
