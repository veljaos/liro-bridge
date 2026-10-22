# Session 34 (Windows, the owner's machine) — predictions, written before each measurement

Each block is committed before the run it predicts, so git dates it. The
least certain prediction of each block is named. Results are not written
here; they are in the decision entry that reads the run.

**Read before any block (2026-10-22, not predictions):**
- **Installed:** v0.9.9-rc1, the per-user MSI of 2026-10-11, its binary built from `4e6fd47` (the tag, `vcs.modified=false`, go1.26.5); its tray is running, PID 13936, from 18:31:10 today; no `liro-bridge://` key.
- **The handover's build is gone.** Run 38291112853 (`446dcd2`), and its PR run 38291116544, have 0 artifacts. So do all twenty runs D-482 listed for deletion. The only Windows package left is `67695f6`'s, run 38297486252, artifact 11770596983, expiring 2026-11-11.

## P1 — is `67695f6`'s package the same product as `446dcd2`? (written before any download or build)

**Between the two commits,** the one non-test Go file that changed is `internal/logcheck/logcheck.go`, and no non-test file imports it. The rest is tests and documents.

**The instrument:** rebuild each commit as `build/msi/build.ps1` does, in a fresh worktree:
- go1.26.5, `CGO_ENABLED=0`, windows/amd64;
- `gensyso -version dev`;
- `go build -trimpath -ldflags "-s -w -X main.version=dev -X main.commit=… -X main.buildDate=…"`.

Then compare SHA-256.

| | measurement | predicted |
|---|---|---|
| P1a | The downloaded zip's SHA-256 against the API's `digest` | equal |
| P1b | The artifact `.exe`'s build info | go1.26.5, `vcs.revision=67695f68…`, `vcs.modified=false`, `-trimpath` |
| P1c | `67695f6` rebuilt here with the artifact's own commit and build-date stamps, against the artifact `.exe` | **byte-identical**. **Least certain of P1:** Go is reproducible, but `gensyso`'s resource has not been shown deterministic, and a worktree's VCS stamp has not been compared with CI's checkout. If P1c fails, P1d still stands on its own, and the artifact's link to its source rests on P1b alone. |
| P1d | `446dcd2` and `67695f6`, built with identical stamps and `-buildvcs=false` | byte-identical |
| P1e | Control: `67695f6` built twice, the second time with an empty `GOCACHE` | byte-identical. If not, nothing here is reproducible and P1c/P1d mean nothing |
| P1f | Control: `2ec3055^` and `2ec3055` (product code changed: `interactive.go`, `protocolflow.go`, `dss/gather.go`), the same flags | **differ**. If equal, the instrument cannot see a product change |
| P1g | `go list -deps ./cmd/liro-bridge` at `67695f6` for windows, linux, darwin | no `internal/logcheck` |
| P1h | The per-user MSI's `liro-bridge.exe`, extracted with `msiexec /a` (an administrative image, not an install), against the artifact `.exe` | byte-identical |
| P1i | The MSI's `ProductVersion` | `0.0.0`. So installing it over the rc's 0.9.9 hits `DowngradeErrorMessage`: the rc must be uninstalled first, as D-455 found for 0.9.3 |

## P1, read — in D-483

P1a, P1b and P1d–P1h held. P1c failed as written; the cause was the instrument both times (a worktree, then the clone's tags), and with both removed it held.

## P2 — the test timestamp unit's certificate, read before any card (written before the request is sent)

D-481 says the issuer decides row 1.1: (a) a CA the bundled Trusted List carries, or (b) a test CA it does not. The PDF in row 1.4 carries the unit's certificate. So does a plain RFC 3161 reply with `certReq`, from the same endpoint, and that can be read now, without the card.

**The instrument:** OpenSSL, not our code.
1. `openssl ts -query -sha256 -cert` over a fresh file.
2. POSTed with curl to `https://test-tsa.ca.posta.rs/timestamp1`, with D-045's Basic credentials (`Test.Korisnik`, from `integration_test.go`).
3. The reply read with `openssl ts -reply -text` and the token's certificates printed.
4. The issuer looked up among `TSL-RS.xml`'s CA services by subject and by key identifier.

| | measured | predicted |
|---|---|---|
| P2a | The reply | HTTP 200, `application/timestamp-reply`, status granted, the token carrying the unit's certificate |
| P2b | `openssl ts -verify` against the request, with the token's own chain | verifies only if the chain to a root is present. Predicted: the token carries the unit's certificate, not its root, so it fails for want of a root. Then the same with the issuer and root fetched from the certificate's AIA, if it has one |
| P2c | **The unit's issuer** | **(b): a test CA, not one the Trusted List carries. Least certain of P2,** and the one that matters: a test service is commonly issued by a test hierarchy, but nothing read so far says which this is. If (a), row 1.1 can be done with the test server, with no contract |
| P2d | The unit's certificate: `extendedKeyUsage` and CRL/OCSP pointers | `timeStamping`, critical. At least one CRL distribution point over `http://`, not only LDAP |
| P2e | Control for the lookup: "Pošta Srbije CA 1" (the card's issuer, D-361) searched the same way in `TSL-RS.xml` | found as a CA service. If not, the lookup cannot see a listed CA and P2c says nothing |

## P2, read — in D-484

All five held. The unit's issuer is `TEST 2018 CA 1 - Novo`, under `TEST 2018 CA Root - Novo`; neither is in the Trusted List. That is case (b).

## P3 — the rc out, `67695f6`'s per-user MSI in (the owner: "I do it, silently"; written before any step)

**The steps:**
1. End the rc's tray by its exact PID, 13936.
2. `msiexec /x {F18B05B5-5CD6-4FC7-A1DF-DB5A6D477413} /qn /l*v`.
3. `msiexec /i liro-bridge-dev-x64.msi /qn /l*v`.
4. Start the installed agent with `tray`, as its Start-menu shortcut does.

Read from the rc's `uninstall_windows.go` and from both `.wxs` files.

**Guard:** `%LOCALAPPDATA%\Liro\audit` is hashed file by file before step 1. It must read identical after step 3, because nothing but a signature may append to it (D-266).

| | measured | predicted |
|---|---|---|
| P3a | Step 1 | PID 13936 gone. `bridge.json` stays: TerminateProcess runs no cleanup |
| P3b | Step 2's exit and its effects | Exit 0, no reboot asked (3010 would mean a file in use).<br>The product is gone from Windows Installer and from the Uninstall key; the install folder is gone.<br>`uninstall-notice --quiet` runs, because UILevel is 2. It removes Run and the verb and logs "removed 7 of 7 derived items": `bridge.json`, `webview2`, `webview2-profile`, `ui-assets` and the three `icon-*.ico`.<br>Kept: `audit`, `logs`, `build`, `config.json`, `pairings.json`, `secrets.*`, `tsl-cache.xml`, `update-state.json`.<br>`bridge.log` gains a `liro-bridge starting` line, 0.9.9-rc1, from the custom action: D38, the start line written before the subcommand is read |
| P3c | Step 3's exit and its effects | Exit 0. Product 0.0.0, AssignmentType 0 (per-user).<br>The binary at `%LOCALAPPDATA%\Programs\Liro Bridge\liro-bridge.exe` is `ab3e628c…`.<br>**No agent starts**: `LaunchAgent` needs UILevel ≥ 3.<br>No Run value, no verb, no `liro-bridge` scheme key.<br>**Least certain of P3:** the Uninstall entry under **HKLM**, as B31 found for the rc and 0.9.2, which is unexplained |
| P3d | Step 4 | `bridge.log`: `liro-bridge starting`, `dev`, `67695f6`. `config.json`'s `b-b` read as chosen, with no D-476 line.<br>Run, the verb and `HKCU\Software\Classes\liro-bridge` written from the installed path.<br>`bridge.json` written with `agentVersion` `dev`; protocol listening on 17580.<br>No name or JMBG in any new log line (D-480): there is no card yet, so there is nothing to name |
| P3e | The guard | Audit directory identical, file by file |

## P3, read — in D-485

All held, B31's HKLM entry included. The premise was wrong in one place: the Start-menu shortcut runs `open`; `tray` is the installer's and the Run entry's.

## P4 — row 1.4: one B-T signature with the Pošta card against Pošta's test timestamp server (written before the card is plugged in)

**The owner's rulings for this row (2026-10-22):**
- **"Rejects" is read as D-280 read Adobe: red only.** An unknown root is a verdict about the anchor: Adobe's amber, or DSS's INDETERMINATE / NO_CERTIFICATE_CHAIN_FOUND.
- **DSS runs locally:** the Commission's 6.5.1 demo bundle, on this machine.

**The setup:**
- The build is `67695f6`'s per-user MSI (D-485).
- **Settings, by the owner's hands:**
  - URL `https://test-tsa.ca.posta.rs/timestamp1`, user `Test.Korisnik`, password `123456` (D-045's);
  - level **B-T (potpis + vremenski žig)**, so it is chosen, a request (D-476);
  - Save.
- The document is `Desktop\liro-s34\row-1.4.pdf`, a copy of `testdata/pdfs/blank.pdf` (`b9749fdb…`). It is signed from the main window with the Pošta card through SafeSign, the PIN typed by the owner.
- **Baselines:** `bridge.log` 2,195 lines; the audit chain 319 entries.

| | measured | predicted |
|---|---|---|
| P4a | `config.json` after Save | `signatureLevel` `b-t`, `signatureLevelChosen` true, the URL and user set |
| P4b | The report on screen (the owner's eyes), the new audit entry, the log | The report says B-T in the positive colour, with no warning. Audit entry 320: `outcome` signed, `achievedLevel` `b-t`. The log: no `role=signer` or `role=timestamp` lines (nothing is gathered for B-T), and no name or JMBG (D-480) |
| P4c | `verifypdf` (`67695f6`'s) on the output | `ByteRangeDigestOK`, `SignatureOK`, `SigningCertificateOK` true. `SignerChainTrusted` **true** (CA 1 is in the list). `HasTimestamp` true, `TimestampOK` true, **`TimestampChainTrusted` false**. The timestamp certificate is `TEST Timestamp server 290000560`, issued by `TEST 2018 CA 1 - Novo`: **P2c on the real PDF**. Its genTime is within a minute of the audit entry's time. No `/DSS` |
| P4d | OpenSSL, independent of our code | `cms -verify` over the `/ByteRange` content succeeds. The token from the signature's unsigned attributes verifies against SHA-256 of the signature value, with the test root as anchor. Controls fail: one byte of content changed; the token checked against another digest |
| P4e | Adobe Acrobat DC (the owner's eyes) | **Not red.** Amber, "Signature validity is unknown" or the like, as D-280 found, with Pošta not anchored in Adobe. The timestamp is named as embedded, from the TEST unit, and not trusted or not verifiable |
| P4f | DSS 6.5.1, local, default configuration (the EU's list of lists; Serbia is not in it) | The format is **PAdES-BASELINE-T**. The signature is INDETERMINATE, NO_CERTIFICATE_CHAIN_FOUND; the timestamp also INDETERMINATE, NO_CERTIFICATE_CHAIN_FOUND. No FAILED anywhere. **Least certain of P4:** DSS's exact indications and sub-indications. It may report other sub-indications for the signature, for want of trusted time, and the format reading may depend on the timestamp being judged. Any FAILED would make the row false by the ruling |

## P4, read — in D-486

- **Held:** P4b to P4e. P4b's outcome is `approved`, not `signed`.
- **P4a:** read from the log, not the file.
- **P4f failed as written:** DSS reports FAILED in its two EU qualification blocks, for want of an anchor on an EU list. The AdES verdicts and every cryptographic check came out as predicted.
- **My setup instruction was wrong:** the footer never showed a PID.

## P5 — the agent's WebView2 profile, read for row 1.4's third-party data (written before the scan; the agent quit by the owner)

**The profile:** `%LOCALAPPDATA%\Liro\webview2-profile`, 305 files, 72.9 MB, all readable now. D-487 found 43 of them unreadable.

**The needles are narrower than D-487's.** That needles file and the certificate were deleted. The rebuilt set has:
- each word of the name with and without diacritics, and the number in the CN, taken from A36's lines in the repository;
- the certificate serial and thumbprint, as text and as raw bytes, from the same lines.

**Missing:** the email address and the raw certificate. Neither is held anywhere I can read.

**Encodings:** UTF-8 and UTF-16LE, any case.

| | measured | predicted |
|---|---|---|
| P5a | Hits in the 305 files | **None.** The certificate list reaches the page as a web message (`PostJSON`), not over HTTP, so the HTTP cache holds only the static pages. The pages use no browser storage. **Least certain of P5:** Chromium's own files that can hold page memory or text. A crash report under `Crashpad`, a session's restore data, or a leveldb log written for a reason I have not read would each carry the name if anything does |
| P5b | What the files are | Chromium's ordinary profile: the HTTP and code caches (our `ui-assets` pages), GPU and shader caches, leveldb stores with nothing of ours in them, preferences, locks |

## P5, read — in D-488

- **Both held:** no hits in the 305 files.
- **Positive controls:** the profile keeps our pages' addresses (`liro.invalid`) and no page text in any encoding checked, not even the window title.
- **The scan's gap:** the email address and the raw certificate could not be rebuilt.
