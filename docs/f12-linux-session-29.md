# F12 Linux — session 29: the rc signed, published and installed on Ubuntu, and the README's first instruction found not to work

**What this is:** the handover after the sitting that took the rc from a
refused signing job to an install on the Ubuntu VM (D-441, D-442).
**Written:** 2026-10-08. **Working rules:** §0 first, then session 13 §C,
session 15 §E, session 22 §E–25 §E, session 28 §F, and §F below. D-304's
five questions before believing any check.

**The list is `docs/open-items.md`**, which opens with the owner's order
(D-433). This file says where things are left and what comes next.

---

## 0. How this project is run (the owner, 2026-10-08)

**Run everything you can without asking.** Make the reasonable call yourself
and say what you decided and why. Do not bring the owner a choice the code,
the record or a measurement can settle.

**Stop and ask only when one of these is true:**

- **the owner's eyes are the instrument** — whether a window painted, what a
  dialog said, what a person would see;
- **the owner's hands are needed** — sudo, a GitHub setting, a card in a
  reader, a logout;
- **something cannot be undone and could reasonably go either way;**
- **the decision is the owner's** — product, money, scope, what we promise
  people.

Everything else: do it, and **report at the end of a block**, not at every
step. "Say ready and wait for my go" was for the window measurements, where a
wrong moment cost the reading; it does not apply to reads, builds, tests, or
anything that can be checked without the owner.

What does not change, because it is how we measure rather than a permission
gate: predictions written before measuring, the least certain named, a
failed one said; D-304's five questions; one process at a time on the VM;
exact PID only; no synthetic input (D-094); no `//nolint`; no trailers; the
owner pushes by hand. Anything longer than a few minutes is announced with
what it is and roughly how long.

## A. Where things are left

| | |
|---|---|
| Ubuntu VM | **`liro-bridge 0.9.9~rc1` installed** from the release page, over `0.9.9~dev.17`, at 18:49:08 UTC (D-442). **The running agent is not the rc**: tray PID 2458, started 17:39:25, was not restarted by the upgrade; by every route that can be read it is dev.17 from a deleted file, and which binary it maps cannot be read (A34). The next measurement here starts from an agent the owner has restarted (quit from the tray, or a logout — check it was one). |
| `~/Downloads/liro-bridge-v0.9.9-rc1` | The four fetched files, untouched: the `.deb`, `SHA256SUMS`, `SHA256SUMS.asc`, `liro-bridge-packages.asc`. |
| `~/.gnupg` | Still empty: the import was made in a throwaway home, its `gpg-agent` stopped by PID (D-442). |
| Fedora VM | Not touched. Holds `0.9.9~dev.17` (session 27 §A). |
| The release | `v0.9.9-rc1`, **Pre-release**, ten assets, published 2026-10-08 17:50:07 UTC; `releases/latest` v0.9.2 (D-441). |
| GitHub `package-signing` | `v*` **tag** rule, read back (D-441); 37D3…'s secret proved by the run that used it. |
| CI | Run 37822575930 on `314303f`: complete, all nine jobs `success` (D-442). **Carries GitHub's notice: `ubuntu-latest` becomes Ubuntu 26 from 2026-10-19.** |
| README | The Debian/Ubuntu lines' comments rewritten to say what each command prints; the fingerprint compared at `--verify` (D-442). Not yet through CI — the push after this handover runs it. |

## B. What this sitting did

- **D-441** — `package-signing`'s rule set to Tag at the second attempt, the
  first caught only by the API read-back; a security fix as much as a repair
  (a branch rule let any `v…` branch reach the key for about 22 hours; the
  deployment record shows nothing used it); the failed jobs re-run with that
  cause fixed; 37D3…'s first signatures; the rc published as a prerelease.
- **D-442** — CI read to its end first (and my first query of it could not
  have found it); the Ubuntu half of C16: fetched, imported, verified,
  checksummed, two controls refused, installed as an upgrade; **the README's
  import line did not work as written**, fixed; D-376's costs listed (A34);
  every invocation writes the agent's start line (D38).

## C. The owner's rulings, for reading in one place

1. §0, how this project is run.
2. `runs-on: ubuntu-24.04` on `ci`, `linux-gui`, `sdk-typescript`, in A33's
   pass, **due before 2026-10-19** (D-442).
3. The README's fix: the comparison belongs at `--verify`, or the import
   line says what it prints (D-442) — both done.
4. D-376's costs kept as one list, for whoever picks D-376 up (A34).
5. The `-race` probe's 14 min 21 s: a second reading when one comes, not
   sought.

## D. Next, in this order

1. **Read the CI run the push of D-442 starts, to its end** — it is the
   README change's first run, and `linux-packages`' README step greps the
   fingerprint (`gh run list --commit $(git rev-parse HEAD)`; the full SHA,
   D-442).
2. **C16/F9, the Fedora half**, as the README says, from the release page
   into a new directory: `sudo rpm --import liro-bridge-packages.asc`,
   `rpm -K liro-bridge-0.9.9-rc1.x86_64.rpm` ("digests signatures OK"), a
   control (`rpm -K` on a copy with one byte changed), then `sudo dnf
   install ./…rpm` — **rpm's order of `0.9.9~rc1` against the installed
   `0.9.9~dev.17` has never been measured**; dnf's own words answer it. The
   owner's hands for sudo. Read the README's Fedora lines against what is
   printed, as the Ubuntu ones were: the Ubuntu half found one wrong.
3. **Revoke 39DE…** (A30), as session 28 §D.3.
4. **A15** — where the fingerprint is published besides this repository.
   The owner's decision.
5. **A33**: the timeouts, **the `ubuntu-24.04` pin before 2026-10-19**, the
   Node.js 20 actions, and `check.sh` once its cost is agreed.
6. Block 2: E8, the Guide's Linux section.

## E. What is not done that a reader might assume is

- **The rc is installed on Ubuntu but not running there**: agent 2458 is the
  dev.17 process from before the upgrade.
- **The Fedora half of C16 is not done**; no `.rpm` signed by 37D3… has been
  checked by `rpm -K` outside CI.
- **What gpg prints in Serbian is not read**; the README now says the words
  differ by language and the digits do not, which is reasoning, not a
  reading.
- **The README fix has not been through CI** until the push is read.
- **Whether an earlier entry counted a "starting" line that was a command is
  not checked** (D38).

## F. Rules added in this sitting

1. **A setting is what the API reads back, not what the page showed when it
   was saved** (D-441).
2. **An empty query is not an absence until a control comes back
   non-empty** — `gh run list --commit` wants the full SHA (D-442).
3. **Instructions for people are tested by following them as written** —
   CI checked the README's fingerprint was present, not that its
   instructions could be carried out (D-442).
4. §0.
