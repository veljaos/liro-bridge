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

## P2, added before it runs: E1b, the control for E2 (written 19:25, after E2)

E2's refusal means "per process" only if the agent still held the PIN when E2 ran. **E1b:** sign `Desktop\liro-s35\e1b.pdf` (`b9749fdb…`) in the same agent, PID 14168, with nothing else changed.

| | measured | predicted |
|---|---|---|
| P2i | E1b, a PIN prompt (the owner's eyes; a `Credential Dialog Xaml Host` line in the log) | **None.** Audit entry 324: `approved`, `B-T`. If the PIN **is** asked, E2 is unreadable: the agent may have lost the PIN before E2 ran, and E2 must be repeated straight after a signature that is seen to need none |

## P2, read whole — in D-493

- **Held:** P2b, P2d, P2f, P2g and P2i.
- **Failed:** P2a as written, unreadable because of the sleep. P2c half: Windows Terminal drew a window at the agent's start.
- **Unreadable:** P2e. In E4 the card came out *and* the process was new.
- **Moot:** P2h.
- **A clean E4,** one tray agent across the card change, is described in D-493.

## P3 — the clean E4: does taking the card out clear the PIN in a living agent? (written 2026-10-24, for the next sitting; not run)

**Why:** E4 changed the card and the process at once (D-493). This is the one answer a person could act on (E11).

**The documents:** `Desktop\liro-s35\p3-1.pdf`, `p3-2.pdf`, `p3-3.pdf`, each a copy of `testdata/pdfs/blank.pdf` (`b9749fdb…`).

**The steps (E11):**
0. **Before:**
   - one tray agent, started by signing in to Windows: the `Run` entry starts `tray`. Not from the Start menu, which starts `open` and ends with its window;
   - I read its PID and start `flashwatch2.py`, which runs to the end;
   - the machine kept awake, the level the default, the card in.
1. **Sign `p3-1.pdf`** from that agent's window.
2. **Sign `p3-2.pdf`,** with nothing changed.
3. **The card out for five seconds, then back in;** wait until the reader settles. The agent is left alone.
4. **Sign `p3-3.pdf`.**

| | measured | predicted |
|---|---|---|
| P3a | Step 1 | **The PIN asked once:** one `Credential Dialog Xaml Host` window. As this is the agent's first preparation: three `pkcs11-probe` and three `pkcs11-worker` children, with **six Terminal windows** (D53 is not fixed). Audit `approved`, `B-T`. About a minute (D52 is not fixed) |
| P3b | Step 2 | **No PIN,** no new children, no flash |
| P3c | Step 3 | The agent's PID unchanged. Whether the workers notice the token going and are replaced is not predicted |
| P3d | Step 4 | **The PIN asked again.** **Least certain of P3:** My basis is Microsoft's smart-card provider PIN cache, which I recall as tied to the card and cleared when the card is removed. It is recalled, not re-read tonight, and not measured here. **If no PIN is asked,** taking the card out does not protect a person who walks away with the agent running, and E11's close has to say so |

If anything asks for the PIN twice, stop: the card has three attempts.

## P3, read — in D-495

- **Held:** P3a, P3b, P3c and P3d. PID 14140 throughout; the PIN asked for p3-1 and p3-3, not for p3-2.
- **P3c, the part not predicted:** the workers did not notice the card going. None exited and none was started.
- **P3d's basis** is still recalled, not read. The result agrees with it and does not confirm it.
- **Not predicted:** Windows' smart-card audit log shows every PIN-less signature as a fresh verification from the agent's process.
- **Not done from the setup:** the machine was not kept awake. The run took four minutes of activity, under the 15-minute sleep.

## P4 — one signature in 14140 after its sleep: did the sleep, or something in 51+ minutes, clear the PIN? (written 2026-10-24 23:19, before it runs)

**Why:** E1 was asked for the PIN after a 75-minute sleep and two and a half hours (D-492), and D-495 guessed that a reader powered down in sleep looks like a removed card. 14140 has now slept too.

**Not a clean measurement (the owner).** The sleep lasted 51 minutes. The longest gap measured with no prompt is 11 min 13 s (E1b). So a prompt cannot separate the sleep from any timer longer than 11 minutes. E1 had the same confound.

**Read before it (not predictions):**
- **The agent:** 14140, `tray`, since 21:00:01. Its three workers (10328, 9348, 14424, since 22:04:45–47) are still running.
- **The PIN:** entered at 22:05:01.9 (CHV, `CredentialUIBroker.exe`). 14140's last verification was at 22:07:28.2. **No verification event since,** and no `SmartCard-DeviceEnum` event.
- **The sleep:** S3, the only standby this machine offers, 22:24:48 to 23:16:17 (Kernel-Power 42 "Application API", Power-Troubleshooter 1). The card was left in, by the owner's word.
- **The document:** `Desktop\liro-s35\p4.pdf`, a copy of `testdata/pdfs/blank.pdf` (`b9749fdb…`).

**The step:** I start `flashwatch2.py`. The owner signs `p4.pdf` once from 14140's window (the tray icon), with the card left alone. Then I read the watcher, the verification events, the audit log and the PID.

| | measured | predicted |
|---|---|---|
| P4a | The PIN | **Asked:** one `Credential Dialog Xaml Host` window, then the pair of events, `CredentialUIBroker.exe` and then 14140. **Least certain of P4.** My basis is E1, plus a guess I have not measured: that S3 takes the reader's power, and the provider sees that as the card going, as in P3. **If not asked:** one event from 14140 alone, as in p3-2 |
| P4b | Children and windows | **No new child and no Terminal window.** The workers survived the card change in P3, and the sleep should not change that. If they are replaced, the watcher shows a new preparation: six windows |
| P4c | The process and the audit | PID 14140. Audit entry 330, `approved`, `B-T`. About a minute (D52) |

**What each answer can settle:**
- **Asked:** the PIN was cleared at some point between 22:07:28 and the signature. That interval holds both the sleep and more than 11 minutes, so this **cannot** say which did it, or how (reader power, the provider, or time). It is E1 again, with a watcher and the verification log this time. It adds the bounds, nothing more.
- **Not asked: this settles more.**
  - This sleep did not clear the PIN.
  - No timer shorter than the interval since 22:07:28 (and since the PIN at 22:05:02) cleared it. That is over 70 minutes if the signature comes after 23:18.
  - E1's prompt then needs another cause. It had 75 minutes of sleep, and about 2.5 hours in all.
  - **For E11, worse:** a PIN left for over an hour, across a sleep, still signs.
- **Neither answer separates sleep from a timer.** That would take one of:
  - a short sleep straight after a PIN: sign, sleep two minutes, sign;
  - a wait longer than this one, awake, without the card touched.
  - The first is cheap. It is not part of P4.

**Stop** if anything asks for the PIN twice.

## P4, read — in D-496

- **Held:** P4a (the PIN asked), P4b (no child, no window), P4c (14140, entry 330, `B-T`).
- **It settles nothing about E1** (the owner). The interval, 1 h 29 min from the last verification, holds a 51-minute sleep and more than 11 minutes, so the sleep and a timer cannot be told apart.

## P5 — sign, sleep two minutes, sign: does a sleep alone clear the PIN? (written 2026-10-24 after P4; not run)

**Why:** E1 and P4 were both asked after a sleep **and** after more than 11 minutes. P5 keeps the whole run well under 11 min 13 s, the longest gap E1b showed with no prompt. A timer of that length then cannot be the cause. Only the sleep changes.

**The documents:** `Desktop\liro-s35\p5-1.pdf` and `p5-2.pdf`, copies of `testdata/pdfs/blank.pdf` (`b9749fdb…`), made when it runs.

**The steps:**
0. **Before (mine):**
   - One tray agent, alive since a sign-in, its PID read. Any living agent will do; it need not be fresh.
   - The card in.
   - The watcher started with **1,800 s**. Its clock counts the sleep (D-496), so this covers the run, the sleep and the reading.
   - The verification events and `Kernel-Power` read as a baseline.
1. **Sign `p5-1.pdf`** from that agent's window (the owner). A prompt or none: either way the agent holds the PIN after it. Its verification event's time is **T1**.
2. **At once: Start → Power → Sleep** (the owner). Not `rundll32 powrprof.dll,SetSuspendState`, which hibernates on a machine with hibernation on, and this one has it.
   - **Two minutes asleep, by a clock off the machine.** The card stays in.
   - Wake it with the keyboard or the power button.
3. **Within one minute of waking, sign `p5-2.pdf`** (the owner) in the same agent.
   - **The limit:** from T1 to this signature's verification event (**T2**) must be **under 8 minutes.** That leaves a margin below 11 min 13 s. If it runs over, the run is void, not read.
4. **After (mine):**
   - the PID unchanged;
   - Kernel-Power 42 and 107, and Power-Troubleshooter 1: the sleep's start, end and kind (S3, not hibernation);
   - credential windows counted; the verification events;
   - T2 − T1;
   - audit entries.

| | measured | predicted |
|---|---|---|
| P5a | Step 1 | Whatever the agent's state: a prompt with the pair of events, or one event from the agent alone. Audit `approved`, `B-T` |
| P5b | The sleep | One Kernel-Power 42 and one 107, about two minutes apart; Power-Troubleshooter 1 naming S3 |
| P5c | Step 3, the PIN | **Asked:** one credential window, then the pair of events. **Least certain of P5.** My basis is E1 and P4, plus an unmeasured guess: that S3 takes the reader's power and the provider sees the card go, as in P3 |
| P5d | Children and windows | None new: the workers survived P3's card change and P4's sleep |

**What each answer settles:**
- **Asked:** a two-minute S3 sleep clears the PIN, and a timer cannot be the cause (T2 − T1 is under 8 minutes, and E1b went 11 min 13 s with no prompt).
  - **Not settled:** how. The reader losing power, or the provider reacting to the resume. Telling those apart needs the reader's power state, which nothing here reads.
  - **For E11:** "put the computer to sleep" joins "take the card out" and "quit from the tray", on one run.
- **Not asked:** a short sleep does not clear it. E1's and P4's prompts then come from something longer:
  - a timer between 11 min 13 s and 1 h 29 min;
  - or something a longer sleep does and a short one does not.
  - **The next run:** a wait awake of about 1 h 30 min, the card untouched, then a signature.
- **Either way,** a single run on one card and one reader.

**Stop** if anything asks for the PIN twice.

## P6 — row 1.7: is D45 a probe timeout? (written 2026-10-24 23:41, before it runs)

**The row:** on the NetSeT machine, `TestSourcesAreOnePerUsableModule` five times, with the log. **False** if a run takes about 10 s with no probe timing out.

**Read before it (not predictions):**
- **This is the machine D45 was seen on** (TrustEdgeID's and MUP's NetSeT `netsetpkcs11_x64.dll`).
- **Since D-458 the test asks one discovery** (`Modules`, then `sourcesFor`). So "the two disagree" can no longer happen. What is left to see is a slow run, and why.
- **A probe that runs out `ProbeTimeout` (10 s)** comes back as the failure "the probe process did not answer in time" (`errWorkerSilent`), which the test logs as `not a module: <path>: …`.
- **The card is in the reader.** 14140 is running, with its three workers holding the three modules loaded.
- **How:** `go test ./internal/keysource/pkcs11 -run '^TestSourcesAreOnePerUsableModule$' -count=1 -v`, five times, one after another, under the watcher (each probe child's start and end).

| | measured | predicted |
|---|---|---|
| P6a | Each run's time | **2.5–5 s,** with **no probe timing out,** in all five. **Least certain of P6.** D45 came once in five, so a repeat in five runs is about as likely as not |
| P6b | Usable modules | The three the agent's workers hold: `aetpkss1.dll`, TrustEdgeID's and MUP's `netsetpkcs11_x64.dll`. Anything else is listed as `not a module` with its reason, the same in every run |
| P6c | The probe children (the watcher) | One per candidate, one at a time, each well under a second |

**What each answer settles:**
- **A run of about 10 s that logs "did not answer in time" for a NetSeT module:** D45 is a probe timeout. The row holds, and the inference becomes a measurement.
- **A run of about 10 s with no such failure:** the row is false. Something else takes the time, and the watcher shows which child.
- **No slow run in five:** the row is not read. D45 did not recur, and it stays inferred, as the owner ruled (D-467). Five runs is the row's own number. I do not add more without the owner.

## P7 — row 5.1: does the window CI photographed show the page? (written 2026-10-24, before the artifact is opened)

**The row:** look at the `liro-macos` artifact. **False** if the picture is blank or wrong.

**Read before it (not predictions):**
- **What takes the photograph:** `TestAMacWindowLoadsAPageAndCarriesMessagesBothWays` (`internal/ui/window_darwin_test.go`), with `LIRO_SCREENSHOT_DIR` set by the `macos` job.
  - It opens a WKWebView window titled "Liro Bridge (CI)", 420×406, on `/pages/origin.html`.
  - It posts English strings and the origin `https://erp.example.com`, checks through `Eval` that the origin landed in the page, waits 500 ms, and runs `screencapture -x -o -l<window>` to `origin-window.png`.
- **The artifact:** the newest, from run 40121684835's sibling at `6bb89b9` (artifact 12571531628, 2026-10-24 18:40 UTC). Downloaded into a new, empty folder in my scratchpad. It also holds the bundle's zip, which I do not open or run.

| | measured | predicted |
|---|---|---|
| P7a | The file | `origin-window.png`, about 840×812 pixels (the runner's display at 2×), with no window shadow (`-o`) |
| P7b | What it shows | **The origin page, whole:** the title "Liro Bridge (CI)", the label "Server", `https://erp.example.com`, "A page on this server asked to send you documents to sign.", "Every request still opens the signing screen.", and two buttons, "Allow" and "Do not allow". **Least certain of P7.** On a runner the window can photograph as blank, or as the desktop behind it, if the page has not painted or if `screencapture` lacks screen-recording permission. The test's `Eval` proves the page holds the text, not that it was drawn |
| P7c | Fit | Nothing cut off at 406 points, and no scroll bar |

**What it settles:** P7b holding reads the row: WKWebView draws our page on macOS 15. It says nothing of 5.2–5.4: no bundle opened by a person, no card, no Gatekeeper.

## P8 — rows 2.1 (the origin window), 2.2 and 2.3 on the owner's screen (written 2026-10-24, before any window is opened)

**The owner:** "yes, open windows on my screen. For 2.2, write down the allowed-servers list before you touch it and again after you put it back … Ask me for the clicks when you reach them."

**Read before it (not predictions):**
- **The build:** the installed `67695f6`. `cloudlink.go`, `settingswindow.go`, `internal/cloud`, `internal/ui` and `internal/i18n` are unchanged from it to HEAD. The locale is `sr-Latn`.
- **The screen:** one display, 1920×1080, 96 DPI (100 %), so a point is a pixel.
- **The order in the channel** (`channel.go:88`): **the origin is asked before anything is fetched.** So the origin window can be raised by a link whose server does not exist. Closing the window is a refusal, and a refusal is told nothing (no key in `cloudOutcomeKeys`). **The list is written only on Allow.**
- **The list of allowed servers** is `%LOCALAPPDATA%\Liro\cloud-origins.json`. **It does not exist** (read 23:45, again before step 3): the list is empty.
- **The instrument:** a screenshot of each window's rectangle (`CopyFromScreen`, DPI-aware), read by me, and the watcher.

**The steps:**
1. **2.3, a link that cannot be used (alone).** `liro-bridge.exe open-link "liro-bridge://sign?job=http://example.invalid/j"`: http, not https. I take a screenshot, then close the box myself.
2. **2.1, the origin window (alone).** A link whose origin is the Linux test's longest, `https://` + 244 × `a` + `.rs` (255 characters), handed to 14140 through `open-link`. I take a screenshot, then close the window myself: a refusal, and the list untouched.
3. **2.2 and 2.3's second half (the owner's clicks):**
   - **Before:** the list's file read and shown (expected: absent).
   - **I write three entries** in the program's own format (sorted, indented, 0600): `https://erp.example.com`, `https://unreachable.invalid` and the 255-character origin. **Written directly, not through Allow.**
   - **2.3, second half (alone):** a link to `https://unreachable.invalid/j`. It is now allowed, so the agent fetches it, fails, and says so in a box. I take a screenshot and close it.
   - **Click 1:** the owner opens Settings from the tray. I take a screenshot.
   - **Click 2:** the owner presses Remove on `https://erp.example.com`. I read the file.
   - **Put back:** Settings closed (by the owner or by me), and the file deleted, since it did not exist before. **After:** the file read and shown (expected: absent).
4. **The consent step with a 255-character origin is not in P8.** Through the cloud it needs a TLS server at that name, which means the hosts file and a trusted root on this machine. Through the loopback protocol it needs a pairing, which writes `pairings.json`. Either way it is the owner's decision first.

| | measured | predicted |
|---|---|---|
| P8a | Step 1 | A native box (`#32770`) owned by the `open-link` process, titled "Liro Bridge", saying "Ovaj link nije mogao da se upotrebi. Zatražite novi od stranice koja ga je prikazala." `open-link` exits 2 once the box is closed. 14140 is untouched |
| P8b | Step 2, the window | One `LiroBridgeWindow` owned by 14140, titled "Dozvoliti ovaj server?", always on top, 576 wide (the pairing window's width) and 406 tall in client area |
| P8c | Step 2, the fit | **The whole origin visible,** wrapped across lines; both buttons on screen; no scroll bar. **Least certain of P8.** WebKitGTK needed 372 points in Serbian Latin with its own fonts. Windows' Segoe UI is different, and nothing has measured it. If the origin or a button is cut, or a scroll bar shows, row 2.1's first half is false |
| P8d | Step 2, after | `cloud-origins.json` still absent; the log says the person did not allow the server; no box |
| P8e | Step 3, the agent's box | A box owned by 14140, "Liro Bridge", saying "Server https://unreachable.invalid nije dostupan, pa njegov zahtev nije prikazan. Pokušajte ponovo sa njegove stranice." |
| P8f | Step 3, Settings | The three entries listed, the 255-character one whole or wrapped, each with Remove, within 880 points and without the page cut. Less certain: whether the page scrolls by design once the list is long. I read the page as drawn |
| P8g | Step 3, Remove | The file then holds two entries, `https://erp.example.com` gone. The page drops the row |
| P8h | Step 3, after | The file absent, as before |
