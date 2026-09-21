# F12 — the home list

**One sitting, one machine, the Pošta card.** Everything here needs the Pošta
card, which is at home. Nothing needs more than one visit.

**Written:** 2026-09-18, at the office, with only the MUP card.
**Why it exists:** F12 §2's PIN seam is built, guarded and mutation-tested, and
has never touched a card. Item 2 is the one that costs something.

---

## Before anything runs

```
reg export "HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign" verb.reg
reg export "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" run.reg
```

and **copy** — not hash ([[D-243]]) — `config.json`, the `audit` directory,
`pairings.json`, `secrets.*` and `update-state.json` out of `%LOCALAPPDATA%\Liro`.

Three things that will otherwise waste your time:

- `reg export` writes values in an unstable order, so compare the `Run` key
  **value by value**, never by hashing the export.
- `go test ./cmd/liro-bridge/` writes `HKCU\…\Run\LiroBridge` reproducibly
  ([[D-266]]); `internal/ui`'s window tests extract the icon into the real
  config directory ([[D-285]]). Neither is on this list. Do not run them.
- `%LOCALAPPDATA%\Liro`, not `%APPDATA%\LiroBridge`. The second exists, is a
  stale older layout, and comparing against it reports every file missing.

**Stop the tray agent first.** D-268 did, so that nothing else could reach the
card while a measurement was running, and concurrent smart-card access is
[[D-027]]'s known hazard. Start it again at the end.

---

## 1. Does the Pošta token offer a protected authentication path? — **do this first**

**Everything below depends on the answer and it costs nothing.**

Pošta card in the reader:

```
set LIRO_PKCS11_MODULE=C:\Windows\System32\aetpkss1.dll
set LIRO_PKCS11_WORKER_CARD=in
go test -count=1 -run TestARealModule -v .\internal\keysource\pkcs11\worker\
```

Read-only throughout — `C_Initialize`, `C_GetSlotList`, `C_GetTokenInfo`, a
read-only public session, `C_FindObjects`, `C_GetAttributeValue`,
`C_Finalize`. No `C_Login`. It spends no PIN attempt and can be re-run at will.

**What to look for**, one field per certificate:

```
[0] 1234 bytes  label="..."  token="..." serial="..." slot=N protectedPIN=false
```

**What each answer means:**

- **`protectedPIN=false`** — expected, and what item 2 is written for. SafeSign
  does not collect the PIN itself, so the worker asks the parent for one and
  the whole two-phase exchange runs. **Go on to item 2.**
- **`protectedPIN=true`** — nobody has ever seen this. SafeSign collects the PIN
  itself, `C_Login` is called with NULL, and the asking half of the seam is
  never reached on this card. **Stop and tell me**: item 2 as written would
  measure nothing, and SPEC §6.5.1 clause 1 would bite on more cards than we
  thought.

**Also worth noting down:** the certificate count, the labels, and the timings
block. Two certificates is expected on a Serbian card (SPEC §11.5). The timings
go with the office set below.

**Roughly how long:** under a minute.

---

## 2. One login through the worker — **this costs a PIN attempt if it goes wrong**

**Not yet written.** It needs a PIN entry that does not exist: the real PIN
screen is `internal/ui`, which the worker's own contract test forbids the
worker package from reaching, and `go test` is not a reliable place to type a
PIN. It belongs in a `scripts/p11worker` alongside `p11probe`, with `p11probe`'s
two guards copied exactly:

- it refuses to call `C_Login` at all unless the token's three user-PIN flags
  (`CKF_USER_PIN_COUNT_LOW`, `CKF_USER_PIN_FINAL_TRY`, `CKF_USER_PIN_LOCKED`)
  are **all clear** beforehand;
- it reads them again immediately afterwards, so whether an attempt was
  consumed is measured rather than inferred ([[D-268]]).

One attempt. No retry. No code path that could take a second.

**I will write it and send the step-by-step as one message before you type
anything**, including what a dialog would mean and whose it would be, what to do
if something unexpected happens *before* you have typed, and the two counter
readings. Item 1's answer is the input to it.

---

## 2b. Does a mixed batch report a level it did not reach? — **no card, no PIN, ten minutes**

**Added 2026-09-21 from the Linux machine, which is where it was found and is
not where it can be answered.** It needs no card and spends nothing, so it can
be done before or after the card work — but it has to be done on Windows,
because the reporting path it is about only runs there today.

**What was found.** `lowerLevel` and `levelRank` compute the weakest PAdES
level a batch actually reached. **Nothing in the program calls them.** The
only reference in the whole tree is `tsa_choice_windows_test.go`, a test. They
are parked in `cmd/liro-bridge/batchlevel_windows.go` with this written on
them ([[D-338]]).

**Why it matters, and it is not a tidiness question.** SPEC §12.8 forbids a
silent downgrade and §18.11 is the prohibition; `pades.Result` reports "the
achieved level, never the requested one". A batch of ten where the TSA answers
for eight and fails for two is **B-T for eight documents and B-B for two**, and
what the report screen and the audit entry say about that batch is the thing
to check. If they say B-T, a person has been told every document carries a
timestamp when two do not.

**What to do, in order:**

1. **Read, do not run, first.** In `cmd/liro-bridge`, find where the batch's
   level reaches the report and the audit entry — `recordInteractiveAudit`'s
   `achievedLevel`, and whatever `buildReportInit` sends the report screen.
   Establish **which document's level that is**: the last one signed, the
   first, or a fold across all of them. If it is a fold, `lowerLevel` is dead
   and the answer is to delete it and its test. If it is one document's, the
   defect is real.
2. **Then produce a mixed batch, with no hardware.** The soft token signs
   without a card (`-tags softtoken`), and the TSA is what has to fail for
   *some* documents: point `TSAURL` at an address that is reachable but
   refuses, or unplug the network after the first document. Two documents is
   enough. SPEC §12.8's three attempts with backoff mean this takes a minute
   per failing document, so keep the batch small.
3. **Read what it reports**, on the report screen and in
   `%LOCALAPPDATA%\Liro\audit`: one entry per batch, and the field is
   `achievedLevel`.
4. **Record it either way.** "The fold is already there and these two
   functions are dead" is as much an answer as a defect, and the entry should
   say which — with the batch it was measured on.

**What is already known and does not need re-measuring:** the two functions
compile, are tested, and are correct in themselves. The question is only
whether anything asks them.

---

## 3. Afterwards

Machine back as you found it: `Run` key value by value against `run.reg`,
`verb.reg` byte for byte, the config copies back in place, tray agent restarted.

Check for leftover processes named `p11worker`, `p11probe` or `worker.test`.
See the office findings below — a NetSeT-loaded process can exit and still not
leave the process table, and the only thing that clears those is a reboot.
