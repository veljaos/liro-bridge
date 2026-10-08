# F12 Linux — session 30: the rc checked and installed on Fedora, C16 closed, nothing at install time checks who signed the package; 39DE… revoked; A15 and A35 decided

**What this is:** the handover after the sitting that did the Fedora half of
C16/F9 (D-444), 39DE…'s revocation and the owner's rulings on A35 and A15
(D-445). **Written:** 2026-10-08. **Working rules:** session 29 §0
first, then session 13 §C, session 15 §E, sessions 22–25 §E, session 28 §F,
session 29 §F, and §F below. D-304's five questions before believing any
check.

**The list is `docs/open-items.md`**, which opens with the owner's order
(D-433).

---

## A. Where things are left

| | |
|---|---|
| Fedora VM | **`liro-bridge 0.9.9~rc1-1` installed** from the release page's `.rpm`, over `0.9.9~dev.17`, at 19:57:20 UTC, dnf history 10 (D-444). **The running agent is not the rc**: tray PID 2342, started 19:44:48 UTC, was not restarted by the upgrade. rpm's database holds 37D3… as `gpg-pubkey-37d3c56d…8d65-6ac68b7c`. `~/.gnupg` exists, with no keys. |
| Fedora updates | GNOME Software's pending offline update (140 packages, WebKitGTK 2.54.1, cairo and glibc among them) **invalidated by the install, on the owner's word**. To be applied as its own step, in its own boot, after the rc is read running (D-391). |
| `~/rc1` (Fedora) | The four files, untouched (re-hashed after the controls). |
| Ubuntu VM | As session 29 §A: the rc installed, agent 2458 still dev.17. |
| The release | `v0.9.9-rc1`, Pre-release; `releases/latest` v0.9.2. |
| README | The Fedora block says nothing at install checks the signer, quotes dnf's "skipped OpenPGP checks" as expected, names gpg's first-run lines, and says any word in capitals from `rpm -K` means stop (D-444); **pushed, CI `success` all nine jobs** (run 37836947039, 20:20:16 UTC). "What this does and does not protect against" now opens with signed ≠ checked at install, and the release notes template says the same (D-445): **not pushed**. |
| 39DE… | **Revoked** and committed as `build/linux/liro-bridge-packages-39DE-revoked.asc` (D-445). The `.rev` is in `~/rev` on the Fedora VM, unchanged; the owner's to put back on the stick and remove. |
| CI | `dbf439a` (session 29's handover) **failed** on `windows`, unread until tonight: D39. `5588678` green. |

## B. What this sitting did

- **D-445** — A35 left as written (a repository would close it); "signed" ≠ "installing checks it" written into the README and the release notes; A15 decided: a statement signed with the owner's qualified certificate, verified twice with controls, and konfirs.com; 39DE… revoked, "no reason given"; D39 found and counted (2 of unknown cause in 65 Windows runs since 09-23).
- **D-444** — C16's Fedora half, as the README says: gpg present on stock
  Fedora 44 (Fedora's image build installed it); `--show-keys` printed the
  forty digits, imported nothing; `rpm -K` NOKEY before the import, `digests
  signatures OK` after; two one-byte copies refused; `dnf install` upgraded
  dev.17 to the rc — **rpm sorts `0.9.9~rc1` above `0.9.9~dev.17`**. Two
  predictions failed: F7a (rpm's short line capitalises both words whatever
  failed) and F8b (dnf's "skipped OpenPGP checks"). **C16 closed.**

## C. The owner's rulings

1. **A35: left as written**; a package repository is what would close it, with the cloud work. **Signed and checked-at-install are different claims; nothing we publish may read as the second** (D-445).
2. **A15: the statement signed with the owner's own qualified certificate, naming the owner, verified as a measurement (two verifiers, each with a control), and a konfirs.com page.**
3. **39DE…'s revocation: "no reason given" stands**; the key was seen before the commit.
4. **check.sh: written on Fedora, run on Ubuntu** — no Go toolchain on this VM, convenience notwithstanding.
5. The pending offline transaction invalidated (answered y after it was
   read and explained).
6. "Skipped OpenPGP checks" was not passed over: the owner asked which it
   is; it is the same for signed and unsigned, and rpm checks digests only.

## D. Next, in this order

1. **Push and read CI to its end** — every push, the handover's own
   included (D39 is what happens otherwise): `gh run list --commit $(git
   rev-parse HEAD)`, the full SHA.
2. **`check.sh` on the Ubuntu VM — its controls first** (§G). They have
   never been run.
3. **A15**: the statement — its text, then signed by the program with the
   owner's card, then verified in Adobe Reader and a Serbian validator, each
   with a control; the konfirs.com page; the README pointing to both.
4. **A33**: the timeouts, **the `ubuntu-24.04` pin before 2026-10-19**, the
   Node.js 20 actions.
5. On either VM, before any measurement of the rc running: the agent
   restarted by the owner, and the restart proved. On Fedora, then the
   updates in their own boot.
6. Block 2: E8, the Guide's Linux section.

## E. What is not done that a reader might assume is

- **The rc is installed on both VMs and running on neither.**
- **That dnf printed the same warning for dev.17** is the owner's account;
  dnf's history keeps no warnings.
- **That rpm's install would accept an unsigned or foreign-signed package**
  is read from `%_pkgverify_level`, not by installing one.
- **That gpg is on every Fedora 44 Workstation** is inferred from this VM's
  history, not read on a fresh install.
- **What gpg prints in Serbian** is still not read, on either VM.
- **D-445's README and release-notes change has not been through CI.**
- **The rc's published release page still has the old notes**; the new row
  reaches the next release.
- **`check.sh` and its controls have never run** (§G).

## F. Rules added in this sitting

1. **A warning that is the same for the good case and the bad case is not a
   check** — read what produces it before reading it as one (D-444).
2. **A summary line is not the verdict on its parts**: `rpm -K`'s capitals
   do not say which part failed; `-Kv` does (D-444).
3. **A handover's own push is a push**: read its run to the end like any
   other (D39, D-445). The control query for the previous commit is what
   found it.
4. **Backticks run inside double quotes too**: a Markdown heading passed to
   `python3 -c "…"` lost a word to command substitution (D-445's heading,
   fixed). Text with backticks goes through a file, never a shell string.
