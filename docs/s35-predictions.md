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
