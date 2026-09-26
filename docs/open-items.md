# Everything open in Liro Bridge

**As of D-364, 2026-09-26.** One list, to be read in one sitting and acted
from. Every item has a pointer and what would close it. When an item is
closed, delete it here in the same commit as the entry that closes it; when
something new is left open, add it here in the same commit as the entry that
leaves it.

**How it was compiled, and its limit.** Swept from the five F12 Linux
handovers, `docs/handover-next-session.md`, the phase documents' exit
checklists, the READMEs, and `docs/decisions.md` — D-280 onward read for
their open sections, and the whole file searched for "deferred", "not yet",
"still open", "never watched", "never measured". An item stated only in the
prose of an entry before D-280, outside those words, may be missing.

**Needs** says who or what can close it: *code*, *measurement* (this VM),
*owner's hands*, *owner's decision*, *hardware* (card, reader or GPU),
*Fedora machine*, *CI*, *Windows*.

---

## A. Decisions not yet made

1. **SPEC §6.5.2 states measured facts that are now measured false.** Its table's third row and "Measured: clicking it raised nothing". No security clause rests on them. — D-355 §6. *Close:* amend the facts. *Needs:* owner's decision.
2. **B-LT on Serbian cards: a bundled trust store, or an LDAP client.** SPEC §12.6 makes B-LT the default and no Serbian card reaches it today. — D-281; README F11. *Close:* choose; LDAP needs a SPEC §6.8 amendment. *Needs:* owner's decision, then code.
3. **Clause 2 and the keystroke path.** Whether §6.5 already concedes what an input method sees, or clause 2 needs rewriting rather than a third exception. — D-351, D-352. *Close:* after B1 is measured. *Needs:* owner's decision.
4. **Core-dump hardening (clause 3).** Whether the agent sets `prctl(PR_SET_DUMPABLE, 0)` or a zero `RLIMIT_CORE` itself; apport keeps full images when a core is produced, and autostart sends crash traces to the journal. — D-355 §2. *Needs:* owner's decision, then code.
5. **Per-request deadline for the PKCS#11 worker, and a bound on `Close`/`C_Finalize`.** `pkcs11ShutdownGrace` is 5 s "chosen rather than measured"; a login lasts as long as a person takes. — D-297, D-301, D-309, D-313. *Needs:* owner's decision.
6. **Should SPEC state the source preference (CNG, then PKCS#11, then soft token)?** — D-311. *Needs:* owner's decision.
7. **The icon test that fails on newer Go.** Three answers offered, none chosen. — D-308; still failing per D-318, D-320. *Needs:* owner's decision, then code.
8. **A guard that every error code can be produced**, once it is decided which are legitimately unreachable. — D-312. *Needs:* owner's decision, then code.
9. **A configured module's file name may carry a personal name** into the audit log. — D-319. *Close:* accept the limit, or stop recording configured paths. *Needs:* owner's decision.
10. **A pairing made while the keyring was locked stays in the file store.** — D-347. *Close:* migrate, or document re-pairing. *Needs:* owner's decision.
11. **Race-checking the GTK packages is a probe, not a gate** (gotk4 fails `checkptr`). — ci.yml `linux-gui`; D-335. *Needs:* owner's decision, CI.
12. **The method screen saves a protocol run's corner as the person's default.** — handover-next-session §2; `signflow.go` `saveConfig()`. *Needs:* owner's decision, then code.
13. **A deadline on `uiThread.do`'s wait (Windows).** — D-207, D-099. *Needs:* owner's decision, then code.
14. **Should the audit log travel with ordinary backups?** Decided by implication. — D-340. *Needs:* owner's decision.
15. **Publishing the package-signing fingerprint somewhere other than the repository's own host.** — session 6 §E; D-356. *Needs:* owner's decision.
16. **The `package-signing` environment exists only in `release.yml`.** Ruled and wired (D-360); until it exists a tag's `sign-linux` refuses with "LIRO_PACKAGE_SIGNING_KEY is not set", which is the right failure. The owner does it in one sitting on Windows. *Needs:* owner's hands. The steps:
    1. GitHub → Settings → Environments → **New environment** `package-signing`. Deployment branches and tags → **Selected** → add the tag rule `v*` (the same rule as `release`).
    2. On Windows, from the offline backup: `gpg --armor --export-secret-keys 39DE792A503C4F4E26DF1E4586FA14F600AA59B3 > secret.asc`, then base64 it on one line (`certutil -encodehex -f secret.asc secret.b64 0x40000001`, or PowerShell `[Convert]::ToBase64String([IO.File]::ReadAllBytes("secret.asc")) > secret.b64`).
    3. In `package-signing`, add `LIRO_PACKAGE_SIGNING_KEY` (the contents of `secret.b64`) and `LIRO_PACKAGE_SIGNING_PASSPHRASE`. GitHub cannot move a secret or show one, so both are entered again.
    4. Delete `secret.asc` and `secret.b64`.
    5. In `release`, delete `LIRO_PACKAGE_SIGNING_KEY` and `LIRO_PACKAGE_SIGNING_PASSPHRASE`.
17. *A module's load-failure sentence: translated (D-362). Numbering kept.*
18. **Nothing records what a person was shown when nothing could be signed.** SPEC §6.7 records what was signed and refused; the reason on the empty screen reaches no log and no audit entry, so the only record is whoever was looking. — D-360. *Needs:* owner's decision (what SPEC §6.7 should cover), then code.
19. **Credentials outlive an uninstall, on both platforms.** A pairing's secret is in the login keyring on Linux (in DPAPI-protected storage on Windows), and nothing in an uninstall path can see it: the package cannot reach a home directory, and the MSI runs as whoever installs. Nobody has looked at Windows. — D-363. *Close:* decide whether uninstall tells the person, or the agent offers to forget pairings, or it is accepted. *Needs:* owner's decision.
20. **SPEC §11.11 on Linux is not implemented.** "The agent must tell the user which issuers are actually supported rather than reporting 'no certificates found'." Today a MUP or Halcom holder on Linux is told no reader was found, with the reader plugged in — a false statement about their own hardware. Wording drafted with the owner; the MUP claim waits on a measurement with the owner's MUP card, and Halcom is unverified and must be said so. — D-363. *Needs:* owner's MUP card, then code. **Before any other item on this list: it is the only one that tells a person something untrue about their own machine** (the owner).

## B. Claims not measured

1. **Does a real keystroke into the PIN dialog leave copies before the widget, and is an input method in the path?** — D-351, D-352; session 5 §B.4. *Close:* `pinmem`'s `type` mode against its `baseline`, and ask which `GtkIMContext` the entry has. *Needs:* owner's hands, measurement.
2. **D-350's findings (mlocked, one copy, emptied) are true of GTK, not yet of this program's dialog**, whose calls could not run until D-355. *Close:* repeat D-350 against the real `CollectPIN`. *Needs:* owner's hands, measurement.
3. **Why a notification with no action now raises the window when D-337 said it did not.** Not the desktop entry (control). — D-355 §6. *Close:* vary WebKit view against bare GTK, and posting from Go against another process. *Needs:* owner's hands, measurement.
4. **The Windows keystroke path** (`WM_CHAR` crosses a queue this program does not own). — D-352. *Needs:* Windows, owner's hands.
5. **Focus and raise on other compositors.** — D-337; F12 §11. *Needs:* Fedora machine, a KDE image.
6. **DMABUF and NVIDIA variables on real GPUs.** — D-329, D-324; F12 §0.1. *Needs:* hardware (GPU).
7. **The sandbox on a stock 24.04 kernel** (this VM runs 7.0, not 6.8). — D-324. *Needs:* another machine.
8. **WebKitGTK's sandbox under SELinux, with no profile.** — D-354, session 6 §G. *Needs:* Fedora machine.
9. **A module dying inside `C_Login`, or a worker that hangs.** — D-289. *Needs:* hardware.
10. **The reaper's extra ~320 ms after a deliberate crash.** — D-296, D-297, D-301. *Needs:* Windows; possibly unreachable.
11. **Why the D-272 crash no longer reproduces** (three hypotheses). — D-294. *Needs:* hardware.
12. **One broader PKCS#11 search instead of two; why NetSeT 1.1.3.3 is slow.** — D-305, D-309. *Needs:* hardware, then code.
13. **A faulting CNG provider takes the agent down in-process.** — D-311. *Needs:* hardware.
14. **Icon ink at 20 and 24 px.** — D-320, D-321. *Needs:* Windows, owner's hands.
15. **`dnf install ./file.rpm` on a Fedora desktop needs nothing else typed** (SPEC §1.1). The Ubuntu half is closed by the owner's own install. — D-287. *Needs:* Fedora machine.
16. **WebKit's SIGUSR1 against the Go runtime under load.** — D-326, D-327; session 4 §7. *Needs:* another machine.
17. **CI's cache behaviour is reasoned, not measured.** — D-335; session 4 §10.5. *Close:* read step timings. *Needs:* CI.
18. **The audit `flock` across two processes at `$XDG_DATA_HOME`.** — F12 §7; D-340. *Needs:* measurement.
19. **Stale discovery files under linger, with `XDG_RUNTIME_DIR` unset, and across two users.** — D-325. *Needs:* measurement, owner's hands.
20. **Is gotk4 v0.3.1 missing an API the remaining UI needs?** — D-327, D-330. *Needs:* code.
21. **CI fuzz flake rate.** — D-328. *Needs:* CI.

## C. Built, and never watched or never run end to end

1. **The two-window fix on Windows.** It changes the Explorer verb whenever a tray agent is running — the same race existed there since D-344 — and nothing has run it on Windows. — D-355 §3. *Needs:* Windows, owner's hands.
2. **The web-process leak fix across a day of requests.** — D-355 §4. *Needs:* owner's hands.
3. *GNOME and a left-behind autostart entry: nothing reaches the person; gnome-session logs one warning (D-364). Numbering kept.*
4. *What `remove` and `purge` leave: measured, both (D-363). Numbering kept.*
5. **Window tests run only when built to the profiled path**; CI never runs them. — D-355 §4. *Close:* a CI job with a display and a profile, or a recorded decision. *Needs:* CI.
6. **A batch through PKCS#11, and a one-PIN-per-signature card.** — D-318. *Needs:* hardware.
7. **The certificate chooser for PKCS#11 in the agent on Windows.** Status uncertain: D-318 signed through PKCS#11. — D-316. *Needs:* hardware.
8. **`CodeCertExpired` on real hardware, from 2026-09-24.** — D-310 §8. *Needs:* hardware.
9. **The real module that kills its worker, demonstrated** (F12 §2's box). Linux has only a synthetic SIGABRT. — D-294, D-296, D-349. *Needs:* hardware.
10. **The reap backstop's firing path has no test.** — D-306. *Needs:* code.
11. **The ported sign flow has only `*_windows_test.go` tests.** — D-338. *Needs:* code.
12. **Demo A to a written PDF; demo B to the end**, and the README table. — handover-next-session §2; D-265. *Needs:* owner's hands.
13. **`Sign.java` and `sign.php` never executed.** — sdk/examples README. *Needs:* a JDK and PHP.
14. *The stamp with the holder's name: read by the owner — "SAVKA ODŽIĆ", Ž and Ć rendered (D-363). Numbering kept.*
15. **Halcom: no signature ever verified.** — README F11. *Needs:* hardware.
16. **Package signing with the real key has never run.** The CI steps have (D-360): throwaway signing, the README's check on three images, Fedora's `rpm -K`. — D-356. *Close:* the first `v*` tag, after A16. *Needs:* CI, owner's hands.
17. *The notification at approval: watched by the owner with a real card (D-361). Numbering kept.*

## D. Known defects, not fixed

**Flagged by the owner, above the rest though neither is F12's: D3 and D4.** Both are about a signature, not about Linux, and both mean the Windows version people have installed today may report something it did not establish. Neither should be discovered by a user.

1. *The notification outlived the approval: fixed in D-357; watching it is C17. Numbering kept.*
2. **No file or folder chooser, no message box, no drag and drop on Linux.** `ChooseFiles`/`ChooseFolder` return `ErrUnsupportedPlatform` and are called from the main window, audit export and the stamp window. — D-330, D-331, D-338. *Needs:* code.
3. **`lowerLevel` has no caller**: a mixed batch may report a level it did not reach. — session 4 §8; `batchlevel_windows.go`. *Needs:* code, measurement.
4. **`CERT_REVOKED` has no producer**; the parsed CRL is thrown away. — D-310, D-312. *Needs:* code (and A8).
5. **The Explorer-menu row in Settings on Linux** controls nothing. — D-338; D-354. *Needs:* code.
6. **Five of seven example clients use only the Windows discovery path**, and two documents teach it. No guard can see it. — D-345, D-346. *Needs:* code.
7. **`liro-bridge tray` exits 144 on SIGTERM.** — D-355. *Needs:* measurement.
8. **`pkcs11-worker` processes linger after a `CERT_NOT_FOUND` job.** Probably D-299's held module. — D-355. *Needs:* measurement.
9. **`build.sh` does not mark a dirty tree in `--version`.** — D-355. *Needs:* code.
10. **`cmd/liro-bridge` tests write HKCU Run, and an audit append nobody explained.** — D-266, D-313 §5. *Needs:* Windows.
11. **The PIN guard walker is copied across seven packages.** — D-270, D-290. *Needs:* code.
12. **A lock test flakes under load.** — session 5 §D. *Needs:* code or CI.
13. **`package.test.mjs` cannot find npm-cli in Debian's layout.** — D-346. *Needs:* code (low).
14. **Session 5's pre-push commands are wrong in two places.** — D-354. *Close:* use D-354's corrections; the document is a record. *Needs:* nothing but reading.
15. **The thinned icon has never reached the owner's installed tray.** — D-321. *Needs:* Windows, owner's hands.
16. **The autostart test cannot see the `$` and backtick escapes.** — D-354. *Needs:* code (low).

## E. Promises in documents that nothing does yet

1. *Package signing: built in D-356; its first real run is C16. Numbering kept, because F1 cites E3 and E4.*
2. **SPEC §12.6's B-LT default** — see A2.
3. **MUP on Linux needs a direct PC/SC route.** — F11 "Deferred"; SPEC §6.5.1. *Needs:* code, hardware.
4. *The Pošta card through SafeSign on Linux: signed and verified (D-361). Numbering kept.*
5. *`pcscd.socket` left disabled: the agent says so with the command (D-358), and CI found it enabled after install on all three clean images (D-360). Numbering kept.*
6. *Load failures as readable `Failure`s: built and measured with compiled fixtures (D-359). Numbering kept.*
7. **The audit record says which backend signed but perhaps not why.** Status uncertain — check D-313's `signerOrigin`. — D-311. *Needs:* code.

## F. F12's exit checklist, what is not done

The boxes in `docs/phases/F12.md` are all unticked, including the done ones;
the checklist wants updating against D-354 and D-355.

1. **A real card on Fedora 44.** The Ubuntu half is done on this VM with the real reader passed through (D-361) — the OS a VM, the reader and card hardware. *Needs:* Fedora machine (and E3 for a MUP card).
2. **The soft token signs on both distributions in CI.** `linux-install` checks loading, `--version`, entries and remove/purge — not a signature. *Needs:* CI.
3. **Which findings came from a VM, stated — the F12 report.** No `docs/f12-report.md` yet. *Needs:* a document.
4. **A module that kills its worker becomes a `Failure`, with the real module** — C9.
5. **DMABUF and NVIDIA** — B6.
6. **The sandbox on stock 24.04** — B7.
7. *The "marked trusted" box: struck and replaced by the owner (D-363). Numbering kept.*
8. **What a stock GNOME user sees** — only an empty bus so far (D-342). *Needs:* Fedora machine.
9. **Package signing** — built (D-356); the real-key run is C16.
