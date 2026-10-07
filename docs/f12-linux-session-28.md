# F12 Linux — session 28: the order before v1, the package key rotated, and a week of red CI found

**What this is:** the handover after one sitting with no VM measurement in it
(D-433 to D-439): records, the package key, and CI. **Written:** 2026-10-07.
**Working rules:** session 13 §C, session 15 §E, session 22 §E, session 23 §E,
session 24 §E, session 25 §E, and §F below. D-304's five questions before
believing any check.

**The list is `docs/open-items.md`**, which now opens with the owner's order
(D-433). This file says where things are left and what comes next.

---

## A. Where things are left

| | |
|---|---|
| Ubuntu VM | Not measured on this sitting and its state not read: no package installed or removed, no agent started or stopped by me. Session 27 §A is the last reading. |
| Fedora VM | Not touched. |
| The owner's Windows machine | Git for Windows' **GnuPG 2.4.9**, home `%USERPROFILE%\.gnupg`, **keyboxd** (D-436): 39DE… and 37D3… both in it. Not Gpg4win — a second installation is how 39DE… went missing (D-435). |
| The USB stick (`D:`) | Three files: `39DE….rev`, `37D3….rev`, `liro-bridge-packages-secret-37D3….asc` (D-437, D-439). |
| 37D3…'s passphrase | In a notebook, off every machine (D-439). |
| GitHub `package-signing` | 37D3…'s secret and passphrase; `v*` tag rule (A16, D-439). |
| GitHub `release` | The Windows release signing secrets, untouched; the old package secrets deleted (D-439). |
| The committed package key | `37D3C56D5F26F2C1F887429F0FC7D69CDDDD8D65`, "Liro Bridge Linux packages (Konfirs d.o.o. Beograd)", expires 2029-10-06 (D-437). |
| CI | **Run 37669944025** (the re-run of `59b65b3`) **concluded `cancelled` at 20:49:52 UTC, by the owner — nothing in it failed**: `ci` and `linux-gui` green — the first green `ci` since 2026-09-30; `windows` failed once (D37); `linux-packages` cancelled inside the throwaway-key step after a slow Go module download was taken for a hang, `linux-install` with it. **No complete run of `59b65b3` exists.** |

## B. What this sitting did

- **D-433** — the owner's order before v1, in eleven blocks, written as the
  head of open-items; session 27's sort accepted and its deferred bucket
  dissolved; other issuers' cards outside v1; A2/E2 decided as three cases
  with no hard-coded service; A14 kept open because D-340 answers it for
  Linux only.
- **D-434** — `v*-rc` tags publish as prereleases; nFPM's pre-release version
  measured (`0.9.3~rc1` in both packages); the rotation of 39DE… decided; the
  cloud channel defined (A32); B8 closed.
- **D-435** — 39DE…'s revocation certificate found in Git Bash's own gpg
  home after three wrong places had been checked; the record held back for
  one more check rather than written as "abandoned".
- **D-436** — no Authenticode certificate, by the owner's decision; step 0
  read against predictions.
- **D-437** — the new key, 37D3…, under a name that says who signs; gpg
  refused a second key under the old name, a failed prediction of mine;
  checked in an empty keyring and committed.
- **D-438** — **CI red on master for a week, unread**: three causes, each
  reproduced at HEAD and fixed; "local green" meant less than CI.
- **D-439** — A16 done; the re-run green in `ci` and `linux-gui`; a Windows
  test that failed once, recorded and not re-run away; no timeouts anywhere;
  the rc tag decided.

## C. The owner's rulings, for reading in one place

1. The order before v1 (D-433), and only other issuers' cards outside it.
2. A2/E2: B-LT with a qualified timestamp is the goal, three cases, no
   hard-coded service; the Settings presets removed in block 6; **whether a
   Serbian signature can reach B-LT is a separate question, not answered**
   (D-434).
3. The package key rotated; finding 39DE… did not change that — the
   passphrase had been exposed (D-436).
4. No Authenticode certificate: Windows says "Unknown publisher", as v0.9.2
   does (D-436).
5. `d32probe` kept, the GTK guard's comment a rule, not a list (D-438).
6. `check.sh` built at the cost shown, its line literal — "these steps passed
   here", never "CI is green" (D-438, A33).
7. The rc tag: `v0.9.9-rc1` on `59b65b3`, not resting on a re-run (D-439).
8. The revoked 39DE… is committed beside the new key and **does not travel
   into the public repository** (A30).

## D. Next, in this order

1. **A fresh run of `59b65b3`, read in one piece** — 37669944025 was
   cancelled before `linux-packages` finished. **The run block 1 waits on was
   cancelled by the owner, not by a fault**: do not start by looking for a
   problem that is not there. Predictions written before it
   starts: `ci`, `linux-gui`, `linux-packages`, `linux-install` and
   `packaging` green; `windows` green or D37 again, and either is recorded,
   not re-run away. If every job but a D37 failure is green, the owner tags:

   ```
   git tag -a v0.9.9-rc1 59b65b3 -m "v0.9.9-rc1: the first run of the real package key"
   git push origin v0.9.9-rc1
   ```

   **Predictions for the release run written before the push**, at least:
   the release page marked Pre-release; `releases/latest` still v0.9.2;
   `sign-linux` passes with 37D3… (`verify.sh` inside it); the release
   manifest signed and verified; the page carrying `SHA256SUMS`,
   `SHA256SUMS.asc`, `liro-bridge-packages.asc`. A hung apt mirror means
   cancel the run — `release.yml` has no timeouts yet.
2. **C16/F9 — the README's own commands, by the owner, on both VMs**:
   `gpg --verify` and `sha256sum --check` on Ubuntu, `rpm -K` on Fedora
   ("digests signatures OK"), then the install — `0.9.9~rc1` sorts above the
   installed `0.9.9~dev.17` in `dpkg` (measured); rpm's order not measured.
3. **Revoke 39DE…** (A30): in Git Bash, the `.rev` imported (its armour's
   first line carries a `:` guard to remove first), the revoked public key
   exported and pasted; checked here; committed beside the new key with an
   entry — and left out of the public repository.
4. **A15** — where the fingerprint is published besides this repository.
5. **The timeouts and `check.sh`** (A33): `timeout-minutes: 15` on every apt
   install step in `ci.yml` and `release.yml`; `check.sh` with its controls
   (red on the tree before D-438, a refusal on an unknown step type and on a
   lint version mismatch).
6. Block 2: E8, the Guide's Linux section.

## E. What is not done that a reader might assume is

- **The rc tag has not been pushed.** No release exists beyond v0.9.2.
- **39DE… is not revoked**; only rotated out of the repository and GitHub.
- **No package has been signed with 37D3… outside CI's checks**; the
  throwaway-key run refusing a foreign signature is not a signature by
  37D3….
- **`check.sh` does not exist**; until it does, an entry says which of CI's
  steps it ran and how.

## F. Rules added in this sitting

1. **A caveat decides the bucket** — an inference is never "solved" (D-432,
   applied again to A14 in D-433).
2. **Read CI's state before predicting a run** (D-438).
3. **After every push, the run is read to its conclusion before the next
   entry claims anything** (D-438).
4. **A claim of green names what ran** — `check.sh` when it exists; "both
   views passed" is not a claim (D-438).
5. **A key's record names the `gpg` that made it and its home folder**,
   written at generation (D-435).
6. **A key's revocation certificate is made and stored off the machine at
   the moment the key is made** (D-434).
7. **A test that fails once and passes on a re-run is recorded, not re-run
   away; a tag does not rest on a re-run** (D-439).
