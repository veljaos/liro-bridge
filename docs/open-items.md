# Everything open in Liro Bridge

**As of D-402, 2026-09-28.** One list, to be read in one sitting and acted
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

1. *SPEC §6.5.2: rules, not measurements; the table removed (D-369). Numbering kept.*
2. **B-LT on Serbian cards: a bundled trust store, or an LDAP client.** SPEC §12.6 makes B-LT the default and no Serbian card reaches it today. — D-281; README F11. *Close:* choose; LDAP needs a SPEC §6.8 amendment. *Needs:* owner's decision, then code.
3. *Clause 2 and the keystroke path: decided — conceded in clause 3, not a third exception to clause 2; on Wayland the compositor sees every key by construction, and "anything able to subvert the compositor can also draw a window headed Liro Bridge and collect the PIN directly" (D-398). Numbering kept.*
4. *Core-dump hardening: the agent forbids its own, measured on the installed package — no core, no report, apport never called (D-376, D-377). Numbering kept.*
5. **Per-request deadline for the PKCS#11 worker, and a bound on `Close`/`C_Finalize`.** `pkcs11ShutdownGrace` is 5 s "chosen rather than measured"; a login lasts as long as a person takes. — D-297, D-301, D-309, D-313. *Needs:* owner's decision.
6. **Should SPEC state the source preference (CNG, then PKCS#11, then soft token)?** — D-311. *Needs:* owner's decision.
7. **The icon test that fails on newer Go.** Three answers offered, none chosen. — D-308; still failing per D-318, D-320. *Needs:* owner's decision, then code.
8. **A guard that every error code can be produced**, once it is decided which are legitimately unreachable. — D-312. *Needs:* owner's decision, then code.
9. **A configured module's file name may carry a personal name** into the audit log. — D-319. *Close:* accept the limit, or stop recording configured paths. *Needs:* owner's decision.
10. **A pairing made while the keyring was locked stays in the file store.** — D-347. *Close:* migrate, or document re-pairing. *Needs:* owner's decision.
11. **Race-checking the GTK packages is a probe, not a gate** (gotk4 fails `checkptr`). — ci.yml `linux-gui`; D-335. *Needs:* owner's decision, CI.
12. **The method screen saves a protocol run's corner as the person's default.** Measured on Linux, dev.12: a protocol run with bottom-left chosen changed `config.json`'s `stampPosition` from top-right to bottom-left (D-397). — handover-next-session §2; `signflow.go:684` `saveConfig()`. *Needs:* owner's decision, then code.
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
20. *SPEC §11.11 on Linux: built and measured with the owner's MUP card (D-365). What was not watched is C18. Numbering kept.*
21. **`certs` on Windows still says "Certificates: 0" with no reason.** Linux now prints the window's sentence under the count (D-365); Windows was left unchanged by F12's rule, not by decision. *Needs:* owner's decision.
22. *A native message box on Linux: not built, by the owner's decision (D-373). Numbering kept.*
23. *Ubuntu's updates: taken between measurements on 2026-09-27, the package list recorded before and after — exactly the 22 (D-390). Numbering kept.*
24. *SPEC §6.5.1 clause 3 per platform, with the same-user routes measured and the open ones named (D-381). Numbering kept.*
25. *The Linux PIN dialog's own field: built and measured — no TextChanged, the copy locked while typed, nothing after the wipe (D-385). Numbering kept.*
26. **Report to GTK that a GtkPasswordEntry sends its text in AT-SPI TextChanged**, with the measurement, not a conclusion: the payload equality, the plain AddMatch listener, GTK 4.14.5, and `scripts/a11yprobe` as the reproducer (D-384, D-385). *Needs:* the owner files it (an outward post). And measure Fedora 44's GTK with the same script when the Fedora VM exists (F1).
27. *Paste into the PIN field: measured — one copy in the locked page, none after the wipe, nothing on the accessibility bus (D-388). Numbering kept.*
28. *Both hand-made userns profiles removed by the owner in the sitting's step 0; the baselines before and after each unchanged, `liro-bridge` the only profile left (D-389, D-390). Numbering kept.*
29. **`internal/ui`'s window tests need a profiled path again, and it should not be one the user can write.** Until then they cannot run on this VM (they skip under `go test`, C5). When needed: a profile naming a root-owned binary (e.g. installed with `sudo install -o root -m 0755` under `/usr/local/libexec/`), so the grant covers one known binary rather than whatever is placed in a home directory. *Needs:* owner's decision (a system change), when the window tests are next needed — left open by the owner, not to be decided in the abstract (D-391).

## B. Claims not measured

1. *The shipped field sends IBus nothing: never connected to `ibus-daemon` in eight consecutive runs; the `im-module` half is moot — the field uses no input method. The rest of the input path is B23 (D-392, D-394). Numbering kept.*
2. *What is typed does not survive the new field's wipe as the old field's did: eight valid typed runs, 0 copies after the wipe in each; a rare copy (one run in ten or fewer) is not ruled out (D-392). Numbering kept.*
3. **Why a notification with no action now raises the window when D-337 said it did not.** Not the desktop entry (control). — D-355 §6. SPEC §6.5.2 no longer rests on the answer (D-369). *Close:* vary WebKit view against bare GTK, and posting from Go against another process. *Needs:* owner's hands, measurement.
4. **The Windows keystroke path** (`WM_CHAR` crosses a queue this program does not own). — D-352. *Needs:* Windows, owner's hands.
5. **Focus and raise on other compositors.** — D-337; F12 §11. *Needs:* Fedora machine, a KDE image.
6. **DMABUF and NVIDIA variables on real GPUs.** — D-329, D-324; F12 §0.1. *Needs:* hardware (GPU).
7. *The sandbox on stock 24.04: starts under the package's profile on 6.8.0-142 and on HWE 7.0.0-34, updated; refused without it on both (D-390). Numbering kept.*
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
18. *The audit `flock` across two processes: excludes at the real directory, and a cross-process test now runs on Linux (D-367). Numbering kept.*
19. **Stale discovery files under linger.** The fallback path is measured: with `XDG_RUNTIME_DIR` unset the tray starts, writes `~/.local/state/liro/bridge.json`, leaves it after SIGKILL — where nothing at logout or reboot removes it — replaces it at the next start as stale, and removes it on SIGTERM (D-396). The two-user part is **deliberately not measured**, by the owner's decision: it would measure systemd's per-user runtime directory, already read (D-325), and a pairing gate that is the same code everywhere (D-396). Left: a lingering user's logout — whether the tray survives it (Ubuntu keeps user processes by default), and whether the next login then runs two agents or finds a stale file. **Unmeasured on this VM, by the owner's decision** (D-400): it needs a logout followed by a login within one boot, and the retained record here holds none — 18 sessions since 2026-09-20, every one ended by a reboot, a shutdown or a crash; the one logout seen to begin ended with the machine, unexplained. Its control, C0 (a logout with nothing of this program running), is unmeasured for the same reason. — D-325, D-388, D-399, D-400. *Close:* on any Ubuntu 24.04 desktop where a logout can be shown to be one (logind in `journalctl -b 0`): session 10 §C0, then session 9 §C3a and §C3b. *Needs:* measurement, owner's hands (logouts), `loginctl enable-linger` (reversible, approved; linger survives a reboot, so `disable-linger` is the next login's first act whatever happens).
20. **Is gotk4 v0.3.1 missing an API the remaining UI needs?** Not for D2: FileDialog, DropTarget with FileList, AlertDialog are all generated, and D-338's "FileList has no methods" was wrong (D-371). Open only for UI not yet written. — D-327, D-330, D-371. *Needs:* code.
21. **CI fuzz flake rate.** — D-328. *Needs:* CI.
22. *Does the Windows PIN dialog tell UI Automation, MSAA or `WM_GETTEXT` what is typed? **It does not** — measured, having been assumed neither way until it was: no `ValuePattern` value, no `TextPattern` at all, MSAA's `get_accValue` refused with `E_ACCESSDENIED` and the control flagged `STATE_SYSTEM_PROTECTED`, cross-process `WM_GETTEXT` returning zero so not even the length crosses, against a control that returned the needle through six rows in the same run (D-395). D-384 stays a Linux finding. What crosses is one `ValueProperty` notification carrying no string — a listener learns that something was typed, not what. Method 2 of the four is not closed by this and is B24. Numbering kept.*
23. **What the compositor hands to which clients, and `:1.3`'s lone key release.** `:1.3` is GNOME Shell as IBus's panel (D-398, by name ownership and elimination). A3 conceded the compositor's sight of every key in clause 3 (D-398); that is not an answer to whether it hands keys to other clients, nor to the key release `:1.3` sent in three of eight runs, which is unexplained (D-392, D-394). Not pursued, by the owner's decision not to instrument Mutter, until there is a reason to. — D-392, D-394, D-398. *Needs:* measurement at the compositor's side, when there is a reason.
24. **Whether an MSAA client driven by WinEvents is ever told what is typed into the Windows PIN dialog.** B22's second method, and the only one of its four that D-395 does not close. The hook was installed on the right process over the whole event range and received **no `EVENT_OBJECT_VALUECHANGE` for any phase — including the unprotected control that six other rows read the needle from**, so its silence for the password phases is an instrument that saw nothing anywhere rather than a refusal. Text set with `WM_SETTEXT` raises the event; a person typing, in that run, did not. What bounds it, as reasoning and not as a reading: a WinEvent carries no text, and the callback a client would make is `get_accValue`, measured refused with `E_ACCESSDENIED`. — D-395; D-304's second question. *Close:* a third control that makes the hook fire on typed input for a plain edit, then the same stimulus against the dialog; or an owner's decision that the reasoned bound is enough. *Needs:* Windows, owner's hands, or owner's decision.
25. **Whether a program running as the same user can press Approve through the accessibility bus**, on either display server. D-384 showed the bus reaches into this program's windows for text; whether the consent window's buttons are actionable through it is not measured. SPEC §6.5.2 says so since D-402. Measuring it would itself be synthetic input, so it is D-094's question. — D-402. *Needs:* owner's decision first.

## C. Built, and never watched or never run end to end

1. **The two-window fix on Windows.** It changes the Explorer verb whenever a tray agent is running — the same race existed there since D-344 — and nothing has run it on Windows. — D-355 §3. *Needs:* Windows, owner's hands.
2. **The web-process leak fix across a day of requests.** Measured in its narrower form (D-397): on the installed dev.12, N = 9 windows, **no web process survived its window** — D-355 §4's leak did not recur. Not the day-long question, which stays open: nine windows in forty minutes is not a day. What does grow is D23. — D-355 §4, D-397. *Needs:* owner's hands, a day.
3. *GNOME and a left-behind autostart entry: nothing reaches the person; gnome-session logs one warning (D-364). Numbering kept.*
4. *What `remove` and `purge` leave: measured, both (D-363). Numbering kept.*
5. **Window tests run only when built to the profiled path**; CI never runs them. — D-355 §4. *Close:* a CI job with a display and a profile, or a recorded decision. *Needs:* CI.
6. **A batch through PKCS#11, and a one-PIN-per-signature card.** — D-318. *Needs:* hardware.
7. **The certificate chooser for PKCS#11 in the agent on Windows.** Status uncertain: D-318 signed through PKCS#11. — D-316. *Needs:* hardware.
8. **`CodeCertExpired` on real hardware, from 2026-09-24.** — D-310 §8. *Needs:* hardware.
9. **The real module that kills its worker, demonstrated** (F12 §2's box). Linux has only a synthetic SIGABRT. — D-294, D-296, D-349. *Needs:* hardware.
10. **The reap backstop's firing path has no test.** — D-306. *Needs:* code.
11. **The ported sign flow has only `*_windows_test.go` tests.** — D-338. *Needs:* code.
12. *Demo A to a written PDF, demo B to the end: both, on Linux, on a real Pošta card through the installed agent, verified by both tools; the README table updated (D-397). Windows not re-run. Numbering kept.*
13. **`Sign.java` and `sign.php` never executed.** — sdk/examples README. *Needs:* a JDK and PHP.
14. *The stamp with the holder's name: read by the owner — "SAVKA ODŽIĆ", Ž and Ć rendered (D-363). Numbering kept.*
15. **Halcom: no signature ever verified.** — README F11. *Needs:* hardware.
16. **Package signing with the real key has never run.** The CI steps have (D-360): throwaway signing, the README's check on three images, Fedora's `rpm -K`. — D-356. *Close:* the first `v*` tag, after A16. *Needs:* CI, owner's hands.
17. *The notification at approval: watched by the owner with a real card (D-361). Numbering kept.*
18. **§11.11's Linux sentences on a stock desktop with no card program.** Everything else is measured: P1–P3 and the replaced sentence watched (D-370, D-372); `certs` with the card out and with no reader, and what OpenSC and SafeSign present with no reader (D-379). Left: `NO_READER` on a machine with no card program, a unit test only. — D-365, D-370, D-379. *Needs:* a clean machine.

## D. Known defects, not fixed

**Flagged by the owner, above the rest though neither is F12's: D3 and D4.** Both are about a signature, not about Linux, and both mean the Windows version people have installed today may report something it did not establish. Neither should be discovered by a user.

1. *The notification outlived the approval: fixed in D-357; watching it is C17. Numbering kept.*
2. *File and folder choosers and the drop on Linux: built (D-371) and watched by the owner on dev.8 (D-372). The message box is A22. Numbering kept.*
3. **`lowerLevel` has no caller**: a mixed batch may report a level it did not reach. — session 4 §8; `batchlevel_windows.go`. *Needs:* code, measurement.
4. **`CERT_REVOKED` has no producer**; the parsed CRL is thrown away. — D-310, D-312. *Needs:* code (and A8).
5. *The Explorer-menu row: shown on Windows only; on Linux Save keeps the saved value (D-366). Watching it is C18. Numbering kept.*
6. **Five of seven example clients use only the Windows discovery path**, and two documents teach it. No guard can see it. — D-345, D-346. *Needs:* code.
7. *`tray` "exits 144 on SIGTERM": the tool's own shell killed by `pkill -f`; the tray dies of SIGTERM itself (−15, a shell's 143) — what that leaves is D20 (D-393). Numbering kept.*
8. *`pkcs11-worker` processes after a job: one per module for the agent's life, by D-311's design — two modules here, two workers; none outlive a one-shot `certs` (D-393). Numbering kept.*
9. **`build.sh` does not mark a dirty tree in `--version`.** — D-355. *Needs:* code.
10. **`cmd/liro-bridge` tests write HKCU Run, and an audit append nobody explained.** — D-266, D-313 §5. *Needs:* Windows.
11. **The PIN guard walker is copied across seven packages.** — D-270, D-290. *Needs:* code.
12. **A lock test flakes under load.** — session 5 §D. *Needs:* code or CI.
13. **`package.test.mjs` cannot find npm-cli in Debian's layout.** — D-346. *Needs:* code (low).
14. **Session 5's pre-push commands are wrong in two places.** — D-354. *Close:* use D-354's corrections; the document is a record. *Needs:* nothing but reading.
15. **The thinned icon has never reached the owner's installed tray.** — D-321. *Needs:* Windows, owner's hands.
16. **The autostart test cannot see the `$` and backtick escapes.** — D-354. *Needs:* code (low).
17. **A Windows window test pins the singular sentence's wording** (`alreadysigned_windows_test.go:124`, "1 of these documents is") and goes red when it improves. The four such tests that run on Linux read from the catalogue now. — D-368. *Needs:* Windows.
18. **Settings' Save on dev.7 and dev.8 never reached its handler, and nothing explains it.** On dev.9, with log lines at each step (D-373), Save worked three times out of three, including after an export in the same window (D-374). The log also recorded windows the owner does not remember opening — a second Save a minute after the first, and a Settings window cancelled 0.39 s after it opened — left unexplained. — D-370, D-372, D-374. *Close:* a recurrence, which the log lines will place; or accepted as unexplained. *Needs:* nothing to do until it recurs.
19. **gotk4 v0.3.1 double-frees GTK's CSS parse error when a Go handler is connected to `GtkCssProvider::parsing-error`** ("free(): double free detected" in `gtk_css_provider_load_from_string`). The agent never connects that handler, so nothing ships with it; anybody adding one would crash. — D-385, D-387. *Close:* a fixed binding, or never connecting it (the test reads GTK's warning from a child process instead). *Needs:* nothing unless someone reaches for that signal.
20. *The tray's `pkcs11-worker` children under SIGTERM: measured on the installed dev.12 with both workers running and a Certificates window open — the tray, both workers and every WebKit and sandbox process gone within 0.2 s, the discovery file removed, D-394's two lines logged. Which path ended the workers is inferred from the code, not observed (D-399). Numbering kept.*
21. *Four entries cited D-290 for a measurement of the Windows edit control: fixed 2026-09-28. The replacement the record proposed, D-277, was wrong too — D-277 chose the control, D-279 §6 named its copy, and no entry measured it — so the four sentences now say that rather than citing anything for a measurement. Numbering kept.*
22. *SPEC §6.5.1 clause 3 amended: it says what the field does and that the path in front of it is unmeasured; D-380's count and B1 removed. How it survived: the finding was fixed and the quotation of it was not (D-398). Numbering kept.*
23. **A bubblewrap/`xdg-dbus-proxy` chain survives about one window in two.** On the installed dev.12, the tray's `bwrap → bwrap → xdg-dbus-proxy` chains went 1 → 2 → 4 across nine windows while every web process ended; about 0.86 MB PSS and three processes each, never reaped while the tray runs. One more on 2026-09-28: a single Settings window, closed with Zatvori, left one chain while its web process ended (D-401); and the same on Xorg, one window, one chain, the network process also left (D-402). Which windows leave one, and why, is not established; D-355's fix ends the web process and evidently not what was launched for it. — D-397, D-401, D-402. *Close:* find what launches the chain and end it with the window, then count again. *Needs:* code, measurement.
24. **SPEC §6.5.1 clause 2's first exception names "the native dialog's own edit control" with no platform.** Since D-385 the Linux field is this program's own locked page with no toolkit control, so the exception exists only on Windows — the same shape as D22, and it would survive the same way. — D-398. *Close:* its own amendment, drafted and shown to the owner before it is written. *Needs:* a draft, owner's decision.

## E. Promises in documents that nothing does yet

1. *Package signing: built in D-356; its first real run is C16. Numbering kept, because F1 cites E3 and E4.*
2. **SPEC §12.6's B-LT default** — see A2.
3. **MUP on Linux needs a direct PC/SC route** — or a module that already is one. MUP ships nothing for Linux (its page, D-365), but an independent open-source module exists, `ubavic/srb-id-pkcs11`, which discovery would find if registered with p11-kit. Untried: trying it loads third-party code against the owner's national identity card. — F11 "Deferred"; SPEC §6.5.1; D-365. *Needs:* owner's decision, then hardware.
4. *The Pošta card through SafeSign on Linux: signed and verified (D-361). Numbering kept.*
5. *`pcscd.socket` left disabled: the agent says so with the command (D-358), and CI found it enabled after install on all three clean images (D-360). Numbering kept.*
6. *Load failures as readable `Failure`s: built and measured with compiled fixtures (D-359). Numbering kept.*
7. **The audit record says which backend signed but perhaps not why.** Status uncertain — check D-313's `signerOrigin`. — D-311. *Needs:* code.
8. **The guide says nothing about Linux — deliberately, for now.** Since D-370 the Linux sentences point to "the documentation or the issuer", and `docs/guide/Uputstvo.html` and `Guide.html` have no Linux section. **Not written yet by the owner's decision**: a guide is written once, when the Linux work is finished and what it has to say is known, not rewritten after each measurement. *Close:* a Linux section — which cards work (D-361, D-365), where SafeSign comes from, that Halcom is untried — at the end of F12. *Needs:* a document, then.
9. **SPEC §6.5.2 on X11: decided, not built.** Measured on an "Ubuntu on Xorg" session (D-402): the agent's Settings window is an X11 client, `_NET_WM_PID` the tray's, `WM_CLASS` empty, and nothing refused it or logged the backend. The owner decided and SPEC §6.5.2 now says: on Wayland the agent never chooses X11 (`GDK_BACKEND=wayland` before GTK starts when `XDG_SESSION_TYPE` is `wayland`, in `PrepareWebKitEnvironment()`); in an Xorg session it does **not** refuse, but the consent window says in its own text that other programs can press its buttons, the agent logs the display server, and every audit entry on Linux records it (§6.7); and the consent screen stops a caller within the protocol, not code running as the same user. **None of it is built**, and the amendment says so. The warning and the audit field are one piece of work; **the field records the backend actually in use — the display GTK opened — not what was configured or requested.** The fallback (a Wayland session whose socket cannot be reached, with `DISPLAY` set) stays reasoned, not measured. That the consent window behaves as Settings did is inferred, not measured. — D-396, D-400, D-401, D-402. *Close:* build the three, then read the audit entry of a real signature on each display server. *Needs:* code, then measurement with a card.

## F. F12's exit checklist, what is not done

The boxes in `docs/phases/F12.md` are all unticked, including the done ones;
the checklist wants updating against D-354 and D-355.

1. **A real card on Fedora 44.** The Ubuntu half is done on this VM with the real reader passed through (D-361) — the OS a VM, the reader and card hardware. *Needs:* Fedora machine (and E3 for a MUP card).
2. *The soft token signs on Ubuntu 24.04, Debian 13 and Fedora 44 in CI, both verifiers agreeing (D-386). Numbering kept.*
3. **Which findings came from a VM, stated — the F12 report.** No `docs/f12-report.md` yet. *Needs:* a document.
4. **A module that kills its worker becomes a `Failure`, with the real module** — C9.
5. **DMABUF and NVIDIA** — B6.
6. *The sandbox on stock 24.04 — B7, closed (D-390). Numbering kept.*
7. *The "marked trusted" box: struck and replaced by the owner (D-363). Numbering kept.*
8. **What a stock GNOME user sees** — only an empty bus so far (D-342). *Needs:* Fedora machine.
9. **Package signing** — built (D-356); the real-key run is C16.
