# Session 35 (Windows, the owner's machine) — predictions, written before each measurement

Each block is committed before the run it predicts, so git dates it. The
least certain prediction of each block is named. Results are not written
here; they are in the decision entry that reads the run.

**Read before any block (2026-10-24, not predictions):**
- **Installed:** `67695f6`'s per-user MSI (D-485). `liro-bridge.exe --version` gives `dev (commit 67695f6, built 2026-10-12T20:06:40Z, go1.26.5, windows/amd64)`. It contains D-478 (`3b3f3bc`), D-479 (`971f22e`) and D-480 (`446dcd2`). `mainwindow.go`, `protocolflow.go`, `interactive.go`, `internal/pades/sign.go`, `internal/pades/dss` and `internal/config/config.go` have not changed since. The tray is running as PID 14168.
- **The MUP card's certificates, from `Cert:\CurrentUser\My`.** Issuer, dates and endpoints only; the subject carries the owner's JMBG and is not read.
  - **The new pair:** issued 2026-09-22, valid to 2031-09-22, issuer `MUP Gradjani CA 4`. One is Non-Repudiation (`c0`), the other Key Encipherment (`a0`).
  - **The expired pair, 2021-09-24 to 2026-09-24, is still in the store,** from the same issuer.
  - **Both pairs have the same endpoints:** OCSP `http://ocsp.mup.gov.rs/MUPGradjaniCAocsp`, CRL `http://ca.mup.gov.rs/MUPGradjaniCA4.crl`, CA certificate `http://ca.mup.gov.rs/MUPGradjaniCA4.crt`. These are the ones D-076 and D-466 measured: OCSP not accepting a connection, the CRL about 30 MB against a 5 MB cap.
- **`config.json`:** `signatureLevel` `b-t`, `signatureLevelChosen` true, left from row 1.4. TSA `https://test-tsa.ca.posta.rs/timestamp1`, user and password set. So the machine is **not** on the default now.
- **Baselines:** `bridge.log` 2,352 lines. The audit chain is 320 entries (sequences 0–319); the last is row 1.4's, 2026-10-22T18:58:56Z, `approved`, `B-T`, `windows-cng`.
- **The documents:** `Desktop\liro-s35\row-1.2-best.pdf` and `row-1.2-blt.pdf`, both copies of `testdata/pdfs/blank.pdf` (`b9749fdb…`).
- **What the code does, read for this block:**
  - **The level:** an empty `signatureLevel` is the best available. The window asks the engine for B-LT and keeps B-T if the evidence falls short (`keepBTIfBLTFails: !m.levelRequested`). The report gets no requested level, so `achievedLevelNote` has nothing to say.
  - **A chosen `b-lt` is a request.** The engine signs on the card and fetches the timestamp *first*. Only then does it gather B-LT's evidence, and it refuses with `LEVEL_NOT_REACHED` (`sign.go:361`). **So the refusal comes after the PIN and after a real signature value has been made.** That signature is discarded, never written.
  - **What a shortfall writes when B-T is kept:** a partial `/DSS` (`applyDSS`), and a note, "too large" or "unavailable".
  - **The window never shows the note.** `logSignNotes` writes it to `bridge.log` as WARN "the signer's notes on this signature".
  - **The fetches:** OCSP is two attempts of 10 s. The CRL is read **in full** (up to 64 MB) within a 30 s timeout, and only then compared with the 5 MB cap.

## P1 — row 1.2: a MUP card on the default, then with B-LT chosen (written before the owner's first step)

**The row:** a MUP card on the default stops at B-T with **no** warning. With B-LT **chosen** in Settings it is refused with `LEVEL_NOT_REACHED`, and nothing is saved. **False** if the default warns, or if a chosen B-LT is saved at B-T.

**The steps, by the owner's hands:**
1. **Settings:** level **"Najbolji dostupan (B-LT gde izdavalac sertifikata to omogućava, inače B-T)"**, then Save. Nothing else changed.
2. **Main window:** `row-1.2-best.pdf`, the MUP certificate valid to 2031-09-22, approve, the PIN. Read the report, then close it.
3. **Settings:** level **"B-LT (potpis + vremenski žig + dokazi o opozivu)"**, then Save.
4. **Main window:** `row-1.2-blt.pdf`, the same certificate, approve, the PIN. Read what the window says.
5. Settings left as the owner wants it afterwards. The default is step 1's.

Then I read `config.json` after each Save, the log after line 2,352, audit entries 320 and 321, the folder, and the step-2 output with `verifypdf`.

| | measured | predicted |
|---|---|---|
| P1a | `config.json` after step 1 | `signatureLevel` empty, `signatureLevelChosen` false. The TSA settings unchanged |
| P1b | The certificate chooser | The new signing certificate is offered, as **one row**. If the expired 2021 certificate is listed, it is marked "Ovaj sertifikat je istekao" and cannot be chosen, as D-486 saw |
| P1c | The PIN dialog | Says Liro Bridge is asking, names the card, and gives MUP's limits, 4 to 8 (D-268) |
| P1d | Time from the PIN to the report, step 2 | **About 25–55 s longer than row 1.4's B-T.** That is OCSP's two 10 s attempts, then the CRL's download of up to 30 s. Less if OCSP now refuses at once rather than hanging |
| P1e | The report on screen, step 2 (the owner's eyes) | **Signed, B-T, in the positive colour, with no warning or note beside it.** No timestamp question |
| P1f | The log for step 2 | **One WARN with `role=signer`** for the new certificate's thumbprint (by design, D-480). Then WARN "the signer's notes on this signature", `requested=b-lt`, `achieved=B-T`. No name and no JMBG anywhere in the new lines |
| P1g | Which `role=signer` WARN, P1f's | **Least certain of P1.** If the whole CRL (about 30 MB) arrives within 30 s: "larger than the cap", `bytes` about 30,000,000, `cap=5242880`, and the note "revocation data too large to embed (… bytes); saved at B-T". If not: "no OCSP response or CRL could be had", and the note "B-LT requested; OCSP/CRL unavailable for at least one certificate". **Either way B-T, and either way the row holds.** WARNs with `role=timestamp`, for Pošta's test unit and its test CA (D-484), may come too. I do not predict their content. The row does not turn on them |
| P1h | Audit entry 320 | `outcome` `approved`, `achievedLevel` `B-T`, `documentCount` 1, `backend` `windows-cng`, no failure code. The chain verifies |
| P1i | The folder after step 2 | `row-1.2-best-signed.pdf`, and nothing else new |
| P1j | `verifypdf` on it | `ByteRangeDigestOK`, `SignatureOK`, `SigningCertificateOK` true; `HasTimestamp` and `TimestampOK` true. **A `/DSS` is present and partial:** certificates, and no CRL or OCSP for the signer's certificate. Whether `SignerChainTrusted` is true depends on MUP's root being in the bundled list; not predicted |
| P1k | `config.json` after step 3 | `signatureLevel` `b-lt`, `signatureLevelChosen` true |
| P1l | Step 4, on screen | **The PIN is asked and the card signs.** After about the same wait as P1d, the batch ends **refused** with "Traženi nivo potpisa nije mogao biti dostignut, pa ništa nije sačuvano." No *Sign without a timestamp*, and no offer to save at B-T |
| P1m | The folder after step 4 | **No `row-1.2-blt-signed.pdf`,** and no partial or temporary file. `row-1.2-blt.pdf` unchanged, `b9749fdb…` |
| P1n | The log for step 4 | The same `role=signer` WARN as P1g, with the same branch, unless the line speed changed. Then the refusal: `LEVEL_NOT_REACHED`, with "B-LT was requested and not reached, so nothing was saved; the evidence fell short as follows: …" carrying P1g's note |
| P1o | Audit entry 321 | `outcome` `failed`, failure code `LEVEL_NOT_REACHED`, `documentCount` 1. `achievedLevel` empty or absent, not predicted firmly. The chain verifies |

**What would make the row unreadable rather than false:**
- **MUP's OCSP answering now, or its CRL under 5 MB.** Step 2 would then save B-LT, which is still no warning. But step 4 would succeed, and the refusal could not be seen with this card.
- **Pošta's test server not answering.** Then there is no B-T at all: the timestamp question instead (row 1.5's ground).

If either happens, I stop and say so before anything else is tried.

## P1, read — in D-491

Row 1.2 done: both halves held. **Held:** P1d–P1k, P1m and P1o (as entry 322). P1g held on its first branch. **P1c failed:** Windows asked, not our dialog. **P1l half failed:** the refusal held, but no PIN was asked. **P1n failed:** no refusal line in the log. **P1b** was not read. **Found:** the PIN was asked once for three signatures (open-items E11).

## P2 — where the PIN is held, what clears it, and the console flashes (written before E1; the owner's go, 2026-10-24)

**The owner's go:** "Run the watcher, and E1, E3 and E4." E2: "yes, with one condition. Sign a fixed byte string that is written down in the record beforehand and is plainly not a document … Never a digest of anything I have not seen."

**Read before any step (17:18, not predictions):**
- **The agent:** PID 14168, `tray`, since 14:07:48.
- **Its three PKCS#11 workers, alive since the first preparation,** all children of 14168:
  - 14752, `pkcs11-worker C:\WINDOWS\System32\aetpkss1.dll`, from 16:46:58;
  - 18308, `… TrustEdgeID\netsetpkcs11_x64.dll`, from 16:46:59;
  - 9076, `… MUP RS\Celik\netsetpkcs11_x64.dll`, from 16:47:00.
- **"module available" was logged only for the first run** (16:46:58), not the two after it. So the later runs started no probes, and reused these workers.
- **Baselines:** `bridge.log` 2,496 lines; the audit chain 323 entries; `config.json`'s level `b-lt`, chosen.
- **The last signature with no PIN:** about 16:51:28 (D-491).
- **The documents:** `Desktop\liro-s35\e1.pdf`, `e3.pdf`, `e4.pdf`, each a copy of `testdata/pdfs/blank.pdf` (`b9749fdb…`).
- **The watcher:** `flashwatch.py`, started by me before step 0 and left running to the end. It logs every new process (PID, parent, image) and every new visible top-level window (class, owning PID, size). It sees image names, not command lines; probes and workers are told apart by whether they exit, and the workers' command lines are read with `Get-CimInstance` afterwards.
- **E2's probe:** `e2probe.py`, its `find` mode run (the store only; the card untouched). It found the signing certificate, `EE8EC0F3…`, in `CurrentUser\My`.

**What E2 signs, exactly, written here before it runs:**
- **The message:** the 31 ASCII bytes `liro-bridge E2 probe 2026-10-24`. Hex `6c69726f2d6272696467652045322070726f626520323032362d31302d3234`.
- **What the card is given:** SHA-256 of those bytes, `38424151195d458b3da03a64d62f4501dcd6ef9d16a5a9137c1b211f626a251c`, with PKCS#1 v1.5 padding. That is how an RSA key signs a message: the card signs the digest of exactly the bytes above, and nothing else.
- **How:** `CryptAcquireCertificatePrivateKey` with `CRYPT_ACQUIRE_SILENT_FLAG`, then `NCryptSignHash` with `NCRYPT_SILENT_FLAG`. Silent means Windows may refuse, but may not show anything. The probe sends no PIN, so it cannot spend an attempt.
- **Why:** to learn whether a process that is not the agent can sign with the PIN the agent's first signature unlocked (SPEC §6.5's claim, E10).
- **What is kept:** if a signature comes back, the probe checks it with OpenSSL against the certificate's public key over those 31 bytes. Its SHA-256 goes into the decision entry.

**The steps:**
0. **The owner:** Settings, level "Najbolji dostupan", Save.
1. **E1, the owner:** sign `e1.pdf` in the agent, with nothing else changed.
2. **E2, me, straight after E1:** `python -I e2probe.py sign …`. The owner touches nothing.
3. **E3, the owner:** quit the agent from the tray, start Liro Bridge from the Start menu, sign `e3.pdf`.
4. **E4, the owner:** take the card out, wait five seconds, put it back, wait until the reader settles, then sign `e4.pdf` with E3's agent.

| | measured | predicted |
|---|---|---|
| P2a | E1: a PIN prompt (the owner's eyes, and the watcher: a new `Credential Dialog Xaml Host` window) | **None.** E1 signs with no prompt, more than 25 minutes after the last signature. So no timer under about half an hour clears it. Audit entry 323: `approved`, `B-T`. Less certain than it looks: a timed cache policy longer than five minutes would also have expired by now, and then E1 asks |
| P2b | E2's result | **Refused, with no window:** `NTE_SILENT_CONTEXT` (`0x80090022`), at the key or at the signature. So the PIN is held per process, not on the card. **That is the opposite of SPEC §6.5,** which predicts a signature. **Least certain of P2:** if the minidriver or the card keeps a verified state that Windows reuses across processes, E2 signs, and OpenSSL verifies it over the 31 bytes. Either way: no audit entry (the agent is not involved), no new log line, no watcher window |
| P2c | E3, the agent quit | The agent's three workers end with it. The new agent starts with no console window anyone sees (D-254) |
| P2d | E3, signing in the new agent | **The PIN is asked, once:** one `Credential Dialog Xaml Host` window. Audit entry 324: `approved`, `B-T` |
| P2e | E4, after the card is taken out and put back | **The PIN is asked again,** with the agent unchanged: one credential window. Audit entry 325: `approved`, `B-T` |
| P2f | The flashes, during E1's preparation | **No new `liro-bridge.exe` child and no console window,** because E1 reuses the agent's discovery and workers. If the owner still sees a flash in E1, it is not our children, and the watcher names the window's class and owner |
| P2g | The flashes, during E3's first preparation | **Three probe children, one at a time** (`pkcs11-probe <module>`, each exiting), **then three `pkcs11-worker` children that stay,** all with the new agent as parent. **For each child: one visible console window,** class `ConsoleWindowClass` or `CASCADIA_HOSTING_WINDOW_CLASS`, alive for well under a second. That is the flash, and it contradicts D-254 for children. Less certain: D-254 measured no window for a process that frees its console this early. If no window is recorded beside the children, the flashes are something else, and the watcher names it |
| P2h | E4's preparation | The same as E1's (P2f) if the agent keeps its workers across a card change. Not predicted firmly: a worker whose token went away may be replaced |

**What would stop the run:**
- **E2 showing any window, or the card's light or Windows asking for anything.** I stop and say so.
- **Any step asking for the PIN twice.** That would be a wrong-PIN path, and attempts matter on this card.

## P2, partly read — in D-492

- **E1:** came at 19:22, after the watcher had stopped and after a 75-minute sleep (18:05:38–19:20:54). **P2a failed as written:** the PIN was asked. The PIN was cleared between 16:52:06 and 19:21:40 by something E1 cannot name.
- **E2:** refused, `NTE_SILENT_CONTEXT`, at 19:24:01. **P2b held.** No signature was made.
- **P2f held:** no flash in E1 (the owner), and no new discovery in the log.

## P2, added before it runs: E1b, the control for E2 (written 19:2x, after E2)

E2's refusal means "per process" only if the agent still held the PIN when E2 ran. **E1b:** sign `Desktop\liro-s35\e1b.pdf` (`b9749fdb…`) in the same agent, PID 14168, with nothing else changed.

| | measured | predicted |
|---|---|---|
| P2i | E1b, a PIN prompt (the owner's eyes; a `Credential Dialog Xaml Host` line in the log) | **None.** Audit entry 324: `approved`, `B-T`. If the PIN **is** asked, E2 is unreadable: the agent may have lost the PIN before E2 ran, and E2 must be repeated straight after a signature that is seen to need none |
