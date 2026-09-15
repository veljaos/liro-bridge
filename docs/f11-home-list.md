# F11 — the home list

**One sitting, one machine, one card.** Everything on this list needs the
home machine, the Pošta card, or both. Nothing on it needs more than one
visit, and nothing on it is dangerous.

**Written:** 2026-09-15.
**Why it exists:** F11 §2's backend is built up to `C_Login` and stops there.
Item 1 is what the rest of the phase waits on.

---

## Before anything runs

```
reg export "HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign" verb.reg
reg export "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" run.reg
```

and **copy** — not hash ([[D-243]], [[D-153]]) — `config.json`, the `audit`
directory, `pairings.json`, `secrets.*` and `update-state.json` out of
`%LOCALAPPDATA%\Liro`.

Two things that will otherwise waste your time:

- `reg export` writes values in an unstable order, so compare the `Run` key
  **value by value**, never by hashing the export. A hash there
  over-reports as well as under-reports (F11 handover §7).
- `go test ./cmd/liro-bridge/` writes `HKCU\…\Run\LiroBridge` pointing at a
  temporary binary, reproducibly ([[D-266]]). Do not run it on that machine
  unless you mean to.

---

## 1. SafeSign's protected authentication path — **the one that unblocks the phase**

**This is first and everything else on the list is smaller.**

SPEC §6.5.1 uses the protected authentication path wherever a module offers
one and only falls back to asking for the PIN where it does not. SafeSign is
the module F11's own exit condition turns on, and it has never been asked
which it is, because it answers `CKR_TOKEN_NOT_RECOGNIZED` for the MUP card
([[D-271]]).

**Pošta card in the reader**, then:

```
go run ./scripts/p11probe --module "C:\Windows\System32\aetpkss1.dll"
```

**No `--login`.** This is `C_GetTokenInfo` and nothing else. It spends no PIN
attempt and cannot: the probe has no code path that logs in unless `--login`
is passed, and that flag refuses to run at all unless the token's three
user-PIN flags are clear ([[D-268]]).

**What to look for**, one line of the output:

```
PROTECTED_AUTHENTICATION_PATH: true | false
```

**What each answer means:**

- **`true`** — SafeSign collects the PIN itself. §6.5.1's fallback is never
  reached on the exit condition's own module, the login step is `C_Login`
  with a NULL PIN and no PIN box at all, and the amendment bites on fewer
  cards than it currently appears to.
- **`false`** — the fallback is real for Pošta as well as MUP, and the login
  step is the whole of §6.5.1's eight clauses: a PIN screen naming Liro
  Bridge, in three locales, with `minPin`/`maxPin` enforced here.

Either way the answer decides the shape of the next commit, which is why
nothing after `C_Login` has been written.

**Also worth capturing from the same run**, since it is free: the token
label, serial, `minPin`, `maxPin` and the whole flags word. Paste the output.

---

## 2. Does SafeSign see the Pošta card at all?

Same command, same run. If `slots with a token present` is 0, or
`C_GetTokenInfo` answers `CKR_TOKEN_NOT_RECOGNIZED`, then SafeSign does not
recognise that card either and the exit condition needs rethinking rather
than implementing.

---

## 3. The mechanism list, for §2.1

Same run again — the probe prints nothing about mechanisms today, so run the
package's own test instead:

```
set LIRO_PKCS11_MODULE=C:\Windows\System32\aetpkss1.dll
go test ./internal/keysource/pkcs11/ -count=1 -v -run TestTheMechanismList
```

**What to look for:** `CKM_RSA_PKCS=true`. That is the mechanism that signs a
pre-computed DigestInfo, which is what `SignDigest` has. If it is false for
SafeSign, §2.1 has a second answer and the signing path is not the same for
both issuers.

---

## 4. Certificates, and what `Chain` will be

```
go test ./internal/keysource/pkcs11/ -count=1 -v -run "TestCertificates|TestWhetherTheToken"
```

**What to look for:** how many certificates the Pošta card carries, and
whether any of them is a CA.

This matters more than it sounds. On the MUP card the answer is two
certificates and **no CA**, so `Chain` is empty — and SPEC §11.8 says Pošta
embeds **3** certificates in a signed document where MUP embeds 1. If the
Pošta card carries its own issuers, a document signed through this path
reaches SPEC §12.6's default level (B-LT) without the chain being completed
from AIA; if it does not, it needs the same completion MUP needs ([[D-271]]).

---

## 5. Which modules exist on that machine, and at which versions

```
go test ./internal/keysource/pkcs11/ -count=1 -v -run TestSourcesAreOnePerUsableModule
```

It prints every module it found, with the vendor label, and every candidate
that was not a module.

**Specifically worth knowing:**

- Does `C:\Windows\System32\aetpkss1.dll` exist there, and at what version?
- **Do `MUP RS\Celik` and `TrustEdgeID` both exist there, and at which
  versions?** This machine has NetSeT at two paths in two builds five years
  apart. Whether that is true of a second machine is the difference between
  "discovery cannot assume a version" and "discovery cannot assume a path"
  ([[D-271]]).
- Anything present that this list does not name.

---

## 6. Pošta card out — the same commands again

Run items 2 and 4 with the card removed, so that presence is a **measured
difference** rather than an assumption. Expect `slots with a token present: 0`
or `CKR_TOKEN_NOT_PRESENT`; anything else is a finding.

---

## 7. Only if you separately decide to: one signature

Not needed for anything above, and it is the exit condition rather than a
measurement. If you do it, it needs a PIN and it is yours to type — and note
that `SignDigest` through PKCS#11 does not exist yet, so this would be the
existing CNG path, not the new one.

---

## What is NOT on this list, and why

- **Another `C_Login` measurement.** [[D-268]] cost one attempt of three and
  answered the question for the MUP token. Nothing here needs a second.
- **Anything with `--login`.** Every command on this list is read-only.
- **A Halcom card.** There isn't one. The module loads and answers here, and
  F11 §6 already requires the report to say which half of Halcom is verified
  and which is not.

---

## When you get back

Paste the output and I will:

1. Close [[D-269]]'s open question with the measurement, in an entry.
2. Build the login step to whichever of §6.5.1's two arrangements the answer
   names — and if it is the fallback, the eight clauses are the specification
   for it and the PIN screen is new UI surface in three locales.
3. Then `SignDigest`, the mechanism verified with the independent verifier of
   SPEC §16.4 rather than by observing that bytes came back — because F11
   §2.1's whole warning is that a wrong mechanism verifies against nothing
   while every layer reports success.
