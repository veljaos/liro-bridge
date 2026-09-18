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
go test -run TestARealModule -v .\internal\keysource\pkcs11\worker\
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

## 3. Afterwards

Machine back as you found it: `Run` key value by value against `run.reg`,
`verb.reg` byte for byte, the config copies back in place, tray agent restarted.

Check for leftover processes named `p11worker`, `p11probe` or `worker.test`.
See the office findings below — a NetSeT-loaded process can exit and still not
leave the process table, and the only thing that clears those is a reboot.
