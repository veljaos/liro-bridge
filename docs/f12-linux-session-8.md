# F12 Linux — session 8

**What this is:** the eighth session on the Linux VM, and the handover to the
next one. **It ends at a reboot** — the kernel-and-update sitting (§C) — so
the next session starts by reading what the sitting wrote (§D). Then
`docs/open-items.md`.

**Written:** 2026-09-27, Ubuntu 24.04.5 VM, HWE kernel 7.0.0-31-generic.
**Entries:** D-368 to D-389. Master at the end of the session: `8177efd`
plus this document.

---

## A. The state, in one page

- **CI** was red on one test and is green: the test copied a sentence
  (D-368). Every later push green; F2's first run on the clean images read
  **from the lines themselves** on Ubuntu 24.04, Debian 13 and Fedora 44
  (D-386, corrected in D-387).
- **SPEC amended three times**: §6.5.2 rules, not measurements (D-369);
  §11.11 met by pointing somewhere, not naming issuers (D-370); §6.5.1
  clause 3 per platform, with the same-user routes measured and the open ones
  named (D-381).
- **D2 done** — file and folder choosers, drag and drop (D-371, D-372); the
  message box recorded, not built (D-373).
- **A4 done** — the agent forbids its own core dumps; measured on the
  installed package: no core, no crash report, apport never called (D-376,
  D-377).
- **The PIN dialog's biggest finding of the phase** (D-384): the shipped
  GtkPasswordEntry told the accessibility bus what was typed — AT-SPI
  `TextChanged`, payload the whole text, readable by any same-user process
  with a plain subscription. **Fixed** by the dialog's own field: a locked
  page of this program's own, dots, "N characters entered", paste read
  straight into the page (D-385, D-387, D-388). Measured with pinmem: one
  locked copy while typed or pasted, none after the wipe, nothing on the bus.
- **Not fixed and not closed:** D-382's copy after the wipe (B2 — one run of
  three; zeros since do not close it); the input path in front of the dialog
  (B1 — the dialog's own process never talks to IBus, D-383, but keys typed
  elsewhere reach IBus through the desktop); Windows' accessibility surface of
  its PIN dialog (B22), assumed neither way.
- **Installed:** `liro-bridge 0.9.9~dev.10` (`f598509`). The field, paste and
  focus fix are in master, not in an installed package: dev.11 would carry
  them.

---

## B. What the owner decided this session, for the record

C over A and B for the PIN field (D-385); paste accepted (D-387); B19
deferred out of the sitting (D-388); the message box not built (D-373); the
guide's Linux section written once, at the end of F12 (E8); A only if
something ships before C — C exists now.

---

## C. The kernel-and-update sitting

**Why two boots** (D-388): the update includes AppArmor, which decides
whether WebKit's sandbox may create a user namespace. One boot with both the
update and a new kernel could not be attributed. So each boot changes one
thing, and `scripts/sandboxcheck/run.sh` is run after each.

**Taken before the sitting, by session 8:**
`~/sitting-packages-before.txt` (2,086 packages),
`~/sitting-upgradable-before.txt` (22), and the baseline
`~/sandboxcheck-7.0.0-31-generic-*.txt` — the installed agent's sandbox
starts; the unprofiled control fails with `bwrap: setting up uid map:
Permission denied`.

**The owner's steps:**

0. *(A28 — the owner decided: remove it, D-389.)* Remove the leftover
   profile that gives `python3.12` the userns permission, so the sitting
   measures the stock state:
   `sudo apparmor_parser -R /etc/apparmor.d/liro-f12-probe && sudo rm /etc/apparmor.d/liro-f12-probe`,
   then re-run `~/liro-bridge/scripts/sandboxcheck/run.sh` (a minute; a new
   baseline file). Predicted unchanged: the check no longer launches from
   Python.
1. `sudo aa-status | grep -E 'liro|unprivileged|bwrap'` — note the output.
2. `sudo apt update && sudo apt upgrade` — the 22.
   Then `dpkg-query -W -f='${Package} ${Version}\n' | sort > ~/sitting-packages-after-update.txt`.
3. `sudo apt install linux-image-generic` — Ubuntu's own 24.04 kernel,
   6.8.0-142. GRUB's default stays the newest kernel, 7.0.0-34.
4. **Reboot** (normal). Check `uname -r` → `7.0.0-34-generic`. Run
   `~/liro-bridge/scripts/sandboxcheck/run.sh`; then step 1's `aa-status`
   again.
5. Check that a one-time boot will be honoured:
   `sudo grep -c next_entry /boot/grub/grub.cfg` must be more than 0. **If it
   is 0**, `grub-reboot` would do nothing and the VM would boot 7.0 again —
   use the menu instead: at the reboot, **hold Shift** as GRUB starts (this VM
   boots by legacy BIOS), choose *Advanced options for Ubuntu*, then the
   6.8.0-142 entry; nothing on disk changes, and missing the moment only boots
   the default — try again (D-389). Otherwise
   `sudo grep -E "menuentry |submenu " /boot/grub/grub.cfg | cut -c1-110`
   for the exact titles, and
   `sudo grub-reboot "Advanced options for Ubuntu>Ubuntu, with Linux 6.8.0-142-generic"`
   (the titles as printed), and `sudo grub-editenv list` shows `next_entry`.
6. **Reboot.** `uname -r` → `6.8.0-142-generic`. Run
   `~/liro-bridge/scripts/sandboxcheck/run.sh`; `aa-status` again.
   **If the sandbox fails on 6.8** (or on boot 2): do not fix anything —
   capture, each into `~`, then continue (D-389 says why each):
   `sudo journalctl -k -b -o cat | grep -i apparmor > ~/sitting-6.8-kernel-apparmor.txt`,
   `sudo aa-status > ~/sitting-6.8-aa-status.txt`,
   `sudo apparmor_parser -r -v /etc/apparmor.d/liro-bridge > ~/sitting-6.8-parser.txt 2>&1`,
   `sudo ls -R /sys/kernel/security/apparmor/features/namespaces > ~/sitting-6.8-features.txt 2>&1`
   (on boot 2, name the files `sitting-7.0-34-…`).
7. **Reboot** once more: back on 7.0.0-34, GRUB's default.

The passed-through reader is dropped at every reboot; nothing in the sitting
needs it. The agent starts at each login (autostart, dev.10).

**Predictions, written before:**

| | boot 2: 7.0.0-34, updated | boot 3: 6.8.0-142, updated |
|---|---|---|
| `apparmor_restrict_unprivileged_userns` | 1 | 1 |
| installed agent: sandbox starts | yes | yes — **least certain**: whether 6.8's AppArmor mediates userns for the package's profile the way 7.0's does; F12 §3.2's worry was bwrap failing outright on 23.10+ |
| control, unprofiled: sandbox | fails, `setting up uid map: Permission denied` | the same |
| AppArmor package | 4.0.1…0.24.04.8 | the same |
| a11y probe (a bare GtkPasswordEntry) | still sends its text — the update does not touch GTK (4.14.5) | the same |

---

## D. What the next session does first

1. Read the owner's account of the sitting, and the files it left:
   `~/sandboxcheck-*.txt` (the baseline, and one per boot), both
   `~/sitting-packages-*.txt` (diff them: exactly the 22, plus the 6.8
   kernel's packages), and the owner's `aa-status` notes.
2. Compare each boot with its predecessor, **one change at a time**: boot 2
   against the baseline is the update; boot 3 against boot 2 is the kernel.
   A difference is explained or it is a finding.
3. Write the entry (D-390; D-389 is the contingencies), close or re-open open-items B7/F6, A23 and A28. If any `~/sitting-*-kernel-apparmor.txt` exists, a boot failed: read those first.
4. Then what is left on Ubuntu: B1/B2 (more pinmem runs on the new field —
   D-382's copy after the wipe is still open), B19 (its own sitting), a dev.11
   package carrying the field, paste and focus fix for the owner's window.
   The owner files the GTK report (A26) under their own name.

---

## E. Instruments that failed this session

| the instrument | what it reported | what was wrong |
|---|---|---|
| untagged `go test ./...` (D-365) | green | never compiled a `softtoken` test; CI's job did (D-368) |
| `-run` pattern that matched nothing | `ok` | ran zero tests — twice |
| negative pins in two tests | green | a copied fragment that no longer matched would pass for ever (D-368) |
| `go tool nm` on the stripped package | "no symbol section" | a guard through it could never fire (D-375) |
| `pdfsig` | exit 0 | exit 0 on "Digest Mismatch" too (D-375) |
| `journalctl -t apport` | nothing | apport logs to `/var/log/apport.log` (D-376) |
| a window check from a 110-character runtime dir | failed for both builds | Unix socket path limit; the unchanged build was the control (D-378) |
| IBus's `GetConnectionUnixProcessID` | nothing | IBus's bus refuses it; the socket table answers (D-382) |
| pinmem's selftest wipe | the planted copy survived | the compiler dropped a dead store (D-383) |
| the terminal as a TextChanged control | none sent | the prediction was wrong; the monitor saw pinmem's own (D-384) |
| gotk4 `parsing-error` handler | crashed GTK | double free in the binding (D-385, D-387; open-items D19) |
| `gh run view --log` (gh 2.45) | nothing, exit 0 | the REST API had the lines (D-387) |
| the sandbox check launched from Python | "unprofiled" control worked | a leftover profile gives python3.12 userns (D-388, A28) |

---

## F. This machine

**Rules unchanged:** one process at a time, no load generators, `go test -p 1`,
both test views before claiming green (memory `local-green-means-both-test-views`),
tooling that touches a secret fails rather than prompts.

**Installed by session 8:** nothing (`sudo -n` could not install `shellcheck`;
`poppler-utils` was already there).

**Left deliberately:** `~/pinmem` (the probe, built from master), its reports
`~/pinmem-report-*.txt`; `/home/vboxuser/liro-f12probe` (the `internal/ui` test
binary, at the profiled path); the sitting's files in `~`; the owner's
`config.json` with `outputFolder` put back by the owner. **Removed:** the test
cores in `/var/lib/apport/coredump` (the owner, with sudo); the test crash
report in `/var/crash` (session 8, before the desktop could offer it);
`~/pinmem-simple`.
