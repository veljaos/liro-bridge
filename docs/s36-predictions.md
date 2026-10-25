# Session 36 (Windows, the owner's machine) — predictions, written before each measurement

Each block is committed before the run it predicts, so git dates it. The
least certain prediction of each block is named. Results are not written
here; they are in the decision entry that reads the run.

**Read before any block (2026-10-25 — measurements, not predictions.
Nothing here is carried over from session 35; every line was read today.)**

- **Display scale 100 %.** DPI-aware before any GDI call (D-501's rule):
  `GetDpiForMonitor` (MDT_EFFECTIVE_DPI) 96, `GetDpiForSystem` 96,
  aware screen metrics 1920×1080 against WMI's physical 1920×1080 — **a
  ratio of 1.0, which is what makes the 96 trustworthy.**
  - **`HKCU\Control Panel\Desktop\LogPixels` still reads 192,** stale from
    session 35's 200 % run, with `Win8DpiScaling` 0 and
    `PerMonitorSettings\…\DpiValue` 0. D-502 left this trap written down;
    it is still armed, and it is ignored here.
- **Text size 100 %:** `TextScaleFactor = 100` under
  `HKCU\Software\Microsoft\Accessibility` — present and explicit, where
  D-501 found it absent and D-502 found it 150. The owner put it back.
- **The agent: the tray.** PID **12808**, command line
  `"…\AppData\Local\Programs\Liro Bridge\liro-bridge.exe" tray`, parent
  `explorer.exe`, started **17:42:55**. It owns the listener on
  127.0.0.1:17580, `bridge.json` names 17580 and was written by it at
  17:42:56, and `/v2/health` answers 200. **Not the Start menu's `open`**,
  which D-502 explains is a window with no agent behind it.
  - **It has signed nothing in its life yet.** That is E11's clock: the
    first card operation in this process costs a PIN, later ones do not.
- **Installed:** `67695f6`'s per-user MSI. `--version` gives
  `dev (commit 67695f6, built 2026-10-12T20:06:40Z, go1.26.5, windows/amd64)`.
  Unchanged since session 35.
- **`config.json`** (written 2026-10-24 23:36:35) — **not** what session
  35's baseline recorded. `signatureLevel` is **`""`** and
  `signatureLevelChosen` is **absent**: the machine is on the default,
  "the best available", not on a chosen `b-t`. TSA
  `https://test-tsa.ca.posta.rs/timestamp1`, user and password set.
  `preferPKCS11` absent, i.e. false.
- **The card:** the MUP card, in the reader, with the new certificate —
  issuer `MUP Gradjani CA 4`, valid 2026-09-22 to 2031-09-22, thumbprint
  `EE8EC0F36917D9E5EC4A05A52FE93645FA7671BD`, Non-Repudiation. The owner
  says so and the reader shows one card; the certificate store is not
  evidence of which card is in, because it keeps stale card certificates
  (the expired 2021 pair is still in it). The Pošta card is out.
- **Where the files are:** `%LOCALAPPDATA%\Liro` — `platform.ConfigDir`,
  not `…\Liro Bridge`, which is the installed program's folder and holds
  none of this. My first read of this machine looked in the wrong one.
- **Baselines:** `bridge.log` **2826** lines. The audit chain's last entry
  is sequence **330**, 2026-10-24T21:37:14Z, `approved`, `B-T`,
  `windows-cng`. `cloud-origins.json` **absent**. No window of class
  `LiroBridgeWindow` on screen.
- **The documents:** `Desktop\liro-s36\row-1.3.pdf`, `row-1.5-a.pdf`,
  `row-1.5-b.pdf`, `row-1.6.pdf`, all copies of `testdata/pdfs/blank.pdf`
  (`b9749fdb…`).
- **The clock:** the machine left summer time today. Session 35's log lines
  are `+02:00`; today's are `+01:00`.

## P10 — P9e, Esc on the origin window: three keystrokes at 100 % scale, 100 % text

**What is being finished.** Row 2.1's window half has been measured at
100 % (D-499), at 200 % scale (D-501) and at 150 % text (D-502, D-503).
**Esc was never pressed** — the owner closed with the X both times — so
P9e is unmeasured, and D-503 left it as the next sitting's first job.
D54's *Not measured* list names it: "whether **Esc** refuses when the
window was never the foreground (the window does not take the foreground
when it opens, measured twice)".

**Why 100 % text, and the same 255-character origin.** The keystroke is
the only new variable. Text size changes what the window *shows* and
nothing about where a keystroke goes, and changing it costs a logon
session. The origin is session 35's, `https://` + 244 × `a` + `.rs`, so
these three runs are directly comparable to D-502's and D-503's.
**At 100 % text the name fills its region in six lines (D-499), so there
is no white space in it to click** — run (c) clicks the name text.

**The three runs. My hands send the link and watch; the owner's hands do
one thing each.**

| | the owner does | what I am asking |
|---|---|---|
| **R6a** | **presses Esc, having never touched the window** — no click, no Alt-Tab | Does Esc refuse a window that was never the foreground? |
| **R6b** | **clicks the title bar**, then presses Esc | Does activating the frame, without touching the page, give the page the key? |
| **R6c** | **clicks the server's name** (inside the page, no button there), then presses Esc | Does Esc refuse once the page has the key? |

**What the code says, read for this block:**

- `origin.js:20` — `document.addEventListener("keydown", …)`, and
  `ev.key === "Escape"` → `window.liroSend("cancel")`. **The listener is
  inside WebView2's document.** So Esc works only if the page has the
  keyboard, and for nobody else.
- `askOrigin` (`cloudlink.go`) — `MessageTypeCancel` and `OnClosed` both
  reach `decide(false)`. **Esc and the X are the same answer and the same
  log line**, `cloud: the person did not allow this server`
  (`internal/cloud/channel.go:90`). The 120-second timeout is the one that
  is told apart: it logs `cloud: nobody answered whether a server may send
  requests` (`cloudlink.go:323`) **first**, and then the refusal.
- `showAndFocusWindow` (`win32_windows.go:258`) — every window of ours
  calls `ShowWindow`, then topmost, then **`SetForegroundWindow(hwnd)`**
  and **`SetFocus(hwnd)`**. So the program does ask for the foreground.
  D-502 and D-503 measured that it does not get it: the tray is not the
  foreground process, and Windows refuses foreground to a process that
  does not own it. `SetFocus` is on the **host** window, not on the
  WebView2 child.
- **Nothing in `internal/ui` ever calls `ICoreWebView2Controller::MoveFocus`,
  and `window_windows.go` has no `WM_ACTIVATE` or `WM_SETFOCUS` case and
  no `WM_KEYDOWN` case.** Nothing in our code hands the keyboard to the
  page, and nothing in our code answers a key that reaches the host.
  Whatever R6b does, WebView2 does of its own accord.

| | measured | predicted |
|---|---|---|
| P10a | The window, before any keystroke | Client **420×406** at `GetDpiForWindow` 96; title "Dozvoliti ovaj server?"; `WS_THICKFRAME` absent; topmost; **no scroll bar anywhere**, and the whole 255-character origin in **six lines** — D-499 and D-501's page at this machine's scale. The frame size is not predicted firmly: D-499 recorded 422×438, D-502's 150 % frame implies about 436×445 for this client, and the two do not agree |
| P10b | Whether the window is the foreground when it opens | **No.** Measured twice (D-502 frame 00, D-503 frame 00). The foreground stays with whatever the owner was in |
| P10c | The focus chain while the window sits untouched | `GetForegroundWindow` is **not** the origin window; the focused window of the foreground thread belongs to **another process**. Our window's own thread has focus on the **host** `LiroBridgeWindow`, not on a WebView2 child |
| P10d | **R6a — Esc, never touched** | **Esc does nothing.** The keystroke goes to whatever has the foreground, the page never sees a `keydown`, and the window stays up. It then **waits out the full 120 seconds** and ends as D-500's accidental R5 did: `cloud: nobody answered whether a server may send requests`, then `cloud: the person did not allow this server`. **Nothing is told to the owner, and the server is never contacted.** This is the prediction I am most confident of, and it is the one that matters: the key a person reaches for to say no does not say no |
| P10e | **R6b — title bar, then Esc** | **Least certain of P10.** I predict **Esc still does nothing**, and the window again waits out the 120 seconds. The title bar is non-client area: clicking it activates the frame, and our code moves focus to the host window and never into the page. For the page to get the key, WebView2's controller would have to take focus on activation by itself — which nothing in our code asks for and nothing has measured. **If the window closes on this Esc, I am wrong about WebView2, not about our code,** and the answer is worth more than the prediction |
| P10f | **R6c — the name, then Esc** | **Esc refuses, within a few hundred milliseconds.** A click inside the page gives the WebView2 child the keyboard; `origin.js` fires; `liroSend("cancel")` → `MessageTypeCancel` → `decide(false)`. The log gets **only** `cloud: the person did not allow this server` — **no "nobody answered" line**, which is how the record will tell this from a timeout |
| P10g | What the owner is told, in all three | **Nothing, in all three.** No box, no notification, no line on any window. A refusal by Esc, by the X and by the clock are silent alike (P9f, held three times) |
| P10h | `cloud-origins.json` | **Still absent** after all three. A refusal writes nothing (P9g, held twice) |
| P10i | `open-link`, each run | `open-link: an agent is running, so the link was left for it`, exit 0; the window opens in **12808**, the tray. The agent is resolved from the window at the moment it appears and checked against the listener, never carried over (D-500's correction) |

**What would make this unreadable rather than false:**

- **The owner touching the window before pressing Esc in R6a** — an
  Alt-Tab, a taskbar click, anything that gives it the foreground. Then
  R6a is R6b.
- **The agent going between runs,** as it did in D-500. The owner's
  correction stands: the window's owner is read every time.
- **A keystroke landing in one of my own windows.** I open none while a
  run is live, and the watcher only reads.

**What I will not do:** close any of the three windows myself while the run
is live. R6a and R6b are predicted to end on the clock, and letting them
is the measurement. The watcher stops reading at 135 seconds; if a window
is somehow still up then, I will say so before touching it.

## P11 — row 1.3: a program that names `b-lt`, on a MUP card

**The row:** a program that names `b-lt` gets `LEVEL_NOT_REACHED` on a MUP card, and nothing is saved (C26). **False** if a document comes back at B-T.

**Why this is not row 1.2 again.** D-491 measured a level **chosen in Settings**. This is a level **named by a caller** — a different field, a different path into the window (`protocolserver.go:166`: `req.Level` overwrites `m.cfg.SignatureLevel` for that batch only), and a different way out: the caller is answered over HTTP rather than shown a report. D-476's rule is that the two are held to the same standard; this is the half of it nothing has run.

**Why it must come before row 1.5.** The engine signs on the card and fetches the timestamp **first**, and only then gathers B-LT's evidence and refuses (`sign.go:361`). With a dead timestamp authority the document would fail `TSA_UNAVAILABLE` and the row would measure nothing. So the real Pošta test TSA must still be configured, and it is.

**The steps.**

1. **Mine:** `POST /v2/pair/request`, name "Row 1.3 (b-lt)", origin `https://test.local`.
2. **The owner's hands:** read the six-digit code off the window and tell me. (Valid 5 minutes.)
3. **Mine:** `POST /v2/pair/confirm`; then `POST /v2/sign/pdf` with `certificateThumbprint` `EE8EC0F3…`, one document `row-1.3.pdf`, **`"level": "b-lt"`**, and `stamp` supplied in full — so there is no certificate chooser and no stamp step, and the owner meets one window.
4. **The owner's hands:** approve the consent window (**120 seconds**, countdown in the last 30) and type the PIN.
5. **Mine:** poll the result; then read the log, the audit chain, the folder and `config.json`.

**What the code says, read for this block:**

- `levelIsARequest(cfg)` is `cfg.SignatureLevel != ""`, and the caller's level was just written into it. So `keepBTIfBLTFails` is **false**: there is no saving at B-T.
- A protocol batch is **exempt from the timestamp question** (`mainwindow.go`, the `m.remote != nil` branch): "there is nobody watching the window by the time a batch it approved is running". A TSA failure here is reported, not asked about.
- `succeeded == 0` makes the job **failed, not completed with an empty result** (`jobhandlers.go:241`), with `firstFailureCode(result)`, and **no body is stored**. `handleJobResult` then finds `TakeResult` empty and answers `fail(w, state.Code)`.

| | measured | predicted |
|---|---|---|
| P11a | `pair/request` and its window | 200, a `requestId`, `expiresInSeconds` **300**. A window shows a **six-digit code** and names "Row 1.3 (b-lt)" and `https://test.local`. **It does not take the foreground** — D-504 measured that for the origin window and nothing about the pairing window differs. **Esc will not close it either**: `pairing.js:42` registers the same listener D-504 proved unreachable |
| P11b | `pair/confirm` | 200, `appId` 32 hex characters, `deviceSecret` 32 random bytes base64 (44 characters). Written to the scratchpad and nowhere else; never to the repository, never printed in full |
| P11c | `POST /v2/sign/pdf` | **202**, a `jobId`, and `batchFingerprint` equal to SHA-256 over the document exactly as sent — `b9749fdb57fcadf802492c78b5da3cc10b7a3772fa91ecc776f9340ccd536fda`, which I compute independently and compare |
| P11d | The consent window | Names **"Row 1.3 (b-lt)"** above the request, **one** document, `row-1.3.pdf`, and the batch fingerprint the 202 carried. **No certificate chooser and no stamp step.** A countdown appears in the last 30 seconds |
| P11e | The PIN | **Asked.** Agent 12808 has signed nothing in its life, so this is E11's first card operation in this process (D-495) |
| P11f | From the PIN to the end | **About 25–55 seconds**, as P1d predicted and D-491 measured: OCSP's two 10-second attempts against MUP's dead responder, then the 29.9 MB CRL (D52) |
| P11g | The window at the end | The refusal: "Traženi nivo potpisa nije mogao biti dostignut, pa ništa nije sačuvano." No *Sign without a timestamp*, no offer to save at B-T |
| P11h | **What the caller gets** | **Least certain of P11. HTTP 422 with `{"code":"LEVEL_NOT_REACHED"}` — not a 200 carrying a `failures` array.** Every document failed, so `job.Fail` is taken and no body is ever marshalled, and the result endpoint answers with the job's code. **PROTOCOL.md §5.2's worked example shows `documents`, `failures` and `counts`, which is what a reader would expect here and is not what the code does when *every* document fails.** If it is 422, §5.2 needs a sentence; if it is 200 with `counts {1, 0, 1}`, I have misread `jobhandlers.go:241` and the row still holds |
| P11i | Nothing saved | **No file anywhere.** The protocol path's output is a response, not a file: `Desktop\liro-s36` gains nothing, `row-1.3.pdf` is unchanged at `b9749fdb…`, and no temporary or partial file appears |
| P11j | Audit entry **331** | `outcome` **failed**, `failureCode` **LEVEL_NOT_REACHED**, `documentCount` 1, `application` **"Row 1.3 (b-lt)"** (not "local"), `channel` **"api-documents"**, `backend` `windows-cng`, `isTestKey` false, `achievedLevel` **empty**. The chain verifies from 330 |
| P11k | The log | One WARN with `role=signer` for `EE8EC0F3…` (by design, D-480), then the B-LT shortfall note — "larger than the cap" with `bytes` about 30,000,000 against `cap=5242880`, or "no OCSP response or CRL could be had" — then `LEVEL_NOT_REACHED`. WARNs with `role=timestamp` for Pošta's test unit and its test CA (D-484) may come too; I do not predict their content. **No name and no JMBG in any new line** |
| P11l | What the card did | **The card signed, and the signature was thrown away.** `sign.go:361` refuses only after the card has produced a real signature value and the timestamp has been fetched. So the PIN bought a signature nobody will ever see. That is the row holding, and it is also worth saying plainly |
| P11m | `config.json` | **Unchanged.** A caller's level is this batch's only (`protocolserver.go:166` writes `m.cfg`, a copy). `signatureLevel` still `""`, `signatureLevelChosen` still absent |

**What would make the row unreadable rather than false, in which case I stop and say so before anything else is tried:**

- **MUP's OCSP answering now, or its CRL under the 5 MB cap.** The document would reach B-LT, which is the level that was asked for — not a failure of the row, but no refusal to see with this card.
- **Pošta's test TSA not answering.** The document then fails `TSA_UNAVAILABLE` before B-LT's evidence is ever gathered, and because a protocol batch is exempt from the timestamp question there is no question either — just a different code. That is row 1.5's ground arriving in row 1.3's run.
- **The consent window timing out at 120 seconds** while the owner is reading. Then the job ends `CONSENT_TIMEOUT` and nothing about the level was tested.

## P12 — row 1.5: the timestamp authority stops answering, and what the question offers

**The row:** when the TSA stops answering mid-batch, the question offers only *Configure* and *Cancel* (C26). **False** if *Sign without a timestamp* is on screen. Point Settings at a dead URL to provoke it.

**Why through the window and not a program.** A protocol batch is **exempt from the question by design** (`mainwindow.go`, the `m.remote != nil` branch): "there is nobody watching the window by the time a batch it approved is running". So this row cannot be measured through the path row 1.3 used. It needs the local window.

**Why a dead URL rather than an authority that dies between documents.** The row's own instruction says to point Settings at a dead URL, and the client is built **once per batch** from the configuration — changing the setting mid-run would not reach it. So the authority is dead from the first document. **It still fails *after* the card is open and the PIN spent,** which is the sense of "mid-batch" that matters: the question arrives when a person has already committed. **A truly live-then-dead authority is not measured by this and I will say so.**

**The URL:** `https://127.0.0.1:9/timestamp`. Port 9 is closed, so the connection is refused immediately rather than hanging — three attempts of `attemptTimeout` never used, plus `backoffAttempt1` 1 s and `backoffAttempt2` 3 s (`internal/pades/tsa/client.go:15–20`).

**The steps.**

1. **The owner's hands:** open the signing window from the tray; add **both** `row-1.5-a.pdf` and `row-1.5-b.pdf`.
2. **The owner's hands:** open **Settings from the window's own gear**, set the timestamp service to `https://127.0.0.1:9/timestamp`, Save, close. The window re-reads the configuration when its Settings closes (`mainwindow.go:705`), so **no restart**.
3. **The owner's hands:** approve. Then **read the question out to me before pressing anything.**
4. **The owner's hands:** press **Podesi izdavaoca vremenskog žiga**, put `https://test-tsa.ca.posta.rs/timestamp1` back, Save, close — which measures the loop *and* restores the setting by the owner's own hand.
5. **Mine:** the log, the audit chain, the folder, `config.json`, and `verifypdf` on both outputs.

| | measured | predicted |
|---|---|---|
| P12a | `config.json` after step 2 | `tsaURL` `https://127.0.0.1:9/timestamp`; `tsaUser` and `tsaPassword` **unchanged**; `signatureLevel` still `""` |
| P12b | The PIN | **Not asked.** D-495: what verifies the card is held in the agent's process, and this window is in **12808**, which row 1.3 already unlocked at 18:57 and which has held the card since. The card has not left the reader and the agent has not quit. **If it is asked, E11's account is wrong and that matters more than this row** |
| P12c | From approve to the question | **About 4 seconds** after the card step: three attempts refused at once, with 1 s and 3 s of backoff between them. Not 45 s — nothing waits on a timeout, because a closed port answers |
| P12d | **The buttons on the question** | **Only two: "Podesi izdavaoca vremenskog žiga" and "Otkaži".** `offersWithoutTimestamp(Unreachable, …)` is **false whatever the level**, and `main.js:269` hides `tsa-without-btn`. **The row holds if and only if "Potpiši bez vremenskog žiga" is absent** |
| P12e | **What the rest of the screen says** | **Least certain of P12, and the thing I expect to be wrong about the row's spirit.** The heading and the explanation are **static `data-i18n` elements of `state-tsachoice`** — only the button is hidden. So I predict the screen is **titled "Bez vremenskog žiga"** and carries the paragraph "Potpis bez vremenskog žiga je nivoa B-B: važeći, ali bez dokaza o tome kada je napravljen…" — **a screen headed by the one option it does not offer, explaining a level it will not produce**, above two buttons that do something else. The row as written would still be *true*; what a person reads would not be |
| P12f | The reason line | "Podešeni izdavalac vremenskog žiga nije odgovorio." (`consent.tsa_reason_unreachable`), not the not-configured one |
| P12g | After *Podesi…*, with the real URL restored | The batch **continues**: `buildTSAClient` returns a client, `allowBB` stays false, and `signInteractiveOne` is retried for document 1. **The question is not asked again** for document 2 |
| P12h | The end | **Both documents at B-T**, saved beside their inputs as `row-1.5-a-signed.pdf` and `row-1.5-b-signed.pdf`. The level is "the best available", so `keepBTIfBLTFails` is **true** and the CRL shortfall keeps B-T rather than refusing — the opposite of row 1.3, from the same card, in the same hour |
| P12i | How long the two documents take | **About 40 seconds each, and the second is not faster** (D52, D-493): the 29.9 MB CRL is fetched again for every document, because nothing remembers it was too large. `revocationMemory` is per batch and remembers endpoints that did not answer, not evidence that was too big |
| P12j | The log | Two `role=signer` WARNs, one per document, each `bytes` about 29,858,766 against `cap=5242880`. Before them, the TSA failures for document 1. **I do not predict whether a line names the question being asked** — row 1.3 showed this program does not always log what it decided |
| P12k | Audit entry **332** | One entry for the batch: `outcome` `approved`, `achievedLevel` **`B-T`**, `documentCount` **2**, `application` `local`, `channel` absent (ChannelLocal is the empty string), `backend` `windows-cng`. `prevHash` is entry 331's hash |
| P12l | `verifypdf` on both | `ByteRangeDigestOK`, `SignatureOK`, `SigningCertificateOK`, `HasTimestamp`, `TimestampOK` all true; a **partial** `/DSS` — certificates, no CRL or OCSP for the signer |

**A code reading this run will probably not measure, recorded now so it is not invented later.** `resolveTSAChoice` opens **its own pairing store** before showing Settings (`interactive.go:247`), on the comment "this is a `sign` process, not the tray, so nothing else in it holds one and there is no second copy to disagree with". **That premise is false here:** this window is in the tray, which holds the live store the protocol authenticates against (`tray.go:129` passes it). So pressing *Podesi…* in a tray-hosted window opens Settings against a **second** copy. Nothing in this run depends on it — the owner is changing a URL, not revoking a pairing — and I will not revoke one to find out. **Flagged for the owner, not claimed.**

**What would make the row unreadable rather than false, in which case I stop and say so:**

- **The PIN being asked again,** which would mean the card session did not survive from row 1.3 and E11's account needs re-reading before anything else.
- **Something answering on 127.0.0.1:9.** Then there is no failure to provoke; I check the port before the run.
- **The batch failing before the timestamp** — a card gone, a document unreadable — which is a different question than the one being asked.

## P13 — row 1.6: the PIN dialog when the signing window is closed under it, during a real card login

**The row:** the PIN dialog ends as a cancel when the signing window closes under it (C21, measured only with a test owner). **False** if the login hangs or the dialog stays.

### The limit this measurement has, written before it runs

**Our PIN dialog is not on the path this machine normally takes.** The MUP card signs through CNG (`backend: windows-cng` in every audit entry today), and CNG raises **Windows'** own prompt — seen directly in D-505's log as `Credential Dialog Xaml Host`, pid 14604. C21's dialog is `internal/ui`'s, and on Windows it is reached only through PKCS#11, which needs **`preferPKCS11: true`** — a key **Settings does not expose** (it is in no page of `settings.html` and in no handler). The owner's ruling, kept here and in the row: **changing a setting Settings does not expose is a measurement of a path no person reaches by hand.** What this run can show is that the dialog and its watchdog behave as C21 built them. What it cannot show is anything about the dialog a person actually meets on this machine, which is Microsoft's.

**`config.json` is copied before it is touched and put back after.** The copy, its digest, and the restored digest all go in the entry.

### The steps

1. **Mine:** copy `config.json` to the scratchpad; record its SHA-256.
2. **The owner's hands:** quit the tray (Izađi). The agent must be gone or a new `open` hands its link off to it (D-500's failure) and reads the old configuration.
3. **Mine:** verify no `liro-bridge.exe` is left; write `preferPKCS11: true`; change nothing else.
4. **Mine:** start `liro-bridge open` — with no agent this is `openWithoutAgent` → `runMainWindow`, a window with no protocol behind it (D-502).
5. **The owner's hands:** add `row-1.6.pdf`, pick the 2031 MUP certificate, approve. **Then type nothing at all.**
6. **Mine:** the moment a window of class `LiroBridgePINDialog` appears, read it, photograph it, and **post `WM_CLOSE` to the signing window** — because the owner *cannot*: see P13e.
7. **Mine:** restore `config.json` and verify the digest. **The owner's hands:** start the tray again.

### What the code says, read for this block

- `CollectPIN` runs the dialog **on its own locked OS thread** and blocks its caller. Its caller here is `openInteractiveSession`, reached from `startSigning` on an ordinary **goroutine**.
- **The window's messages are pumped by a dedicated OS thread** — `uiThread`, class `LiroBridgeUIThread`, "the one OS thread every WebView2 window in this process lives on". **So the PIN blocks a goroutine, not the pump.** This is the whole hinge of the row: if the pump were the blocked thread, a posted `WM_CLOSE` would never be dispatched, the owner window would never be destroyed, the watchdog would never fire, and the login would hang — which is the row's own falsifier.
- `runPINDialog` calls `EnableWindow(owner, 0)` before showing, and sets a **250 ms timer** (`pinOwnerInterval`) that calls `cancel()` when `IsWindow(owner)` goes false — built for exactly this, because D-457 measured a dialog outliving its owner by the whole 15 s a test waited.
- `wmClose` calls `DestroyWindow` **synchronously inside the window procedure**.
- A cancelled PIN **stops the chain**: `openThroughPKCS11` returns it rather than walking on, and `openInOrder` returns it rather than falling through to CNG — "nothing may turn 'no' into 'ask somewhere else'".

| | measured | predicted |
|---|---|---|
| P13a | The machine before the window | No `liro-bridge.exe` of any kind. `config.json` identical to the backup except `preferPKCS11: true`. **`bridge.json` is left stale** — an `open` process starts no protocol, so it still names 17580 with nobody listening |
| P13b | The signing window | Class `LiroBridgeWindow`, owned by **the process I started**, resolved at the moment it appears — D-500's correction, as every run today |
| P13c | **Which dialog appears** | **Ours: class `LiroBridgePINDialog`**, owned by the signing window, titled in Serbian, naming the card and its 4-to-8 limits (D-268). **`Credential Dialog Xaml Host` does not appear** — that is the whole discriminator between CNG and PKCS#11, and D-505 showed what CNG looks like four hours ago |
| P13d | **Which module asks, and how many times** | **Least certain of P13.** Three modules are on this machine (D-505): `aetpkss1.dll`, TrustEdgeID's `netsetpkcs11_x64.dll`, MUP's. They are walked **in turn**, and a module that must log in before it can say whether it has the certificate would ask first and find out after. I predict **exactly one dialog**, from one of the two NetSeT modules, SafeSign having answered "not mine" without a login. **If two appear, that is one card asked for two PINs, which SPEC §6.5.1 clause 5 forbids, and it is a bigger finding than the row.** NetSeT is also the slow module (D45, B12), so the dialog may take ten seconds or more to arrive |
| P13e | The signing window while the dialog is up | **Disabled** — `WS_DISABLED` set by `EnableWindow(owner, 0)`. **This is why the close has to be posted:** a person looking at this screen cannot click the window's X, so the case the row describes is not one a person can produce by hand. Worth saying in the row |
| P13f | Whether `WM_CLOSE` is dispatched | **Yes.** The pump is `LiroBridgeUIThread` and it is not the blocked thread. The window is destroyed inside the window procedure, synchronously |
| P13g | **The dialog, after the window goes** | **It disappears, within about 250 ms** — one tick of its own watchdog. Not immediately, and not never |
| P13h | Whether a second PIN is asked | **No.** `ErrPINCancelled` stops the PKCS#11 walk and stops `openInOrder` before CNG. A person who gave no PIN is not asked again somewhere else |
| P13i | The process | **It exits.** It does not hang and does not have to be killed. The exit code is not predicted firmly |
| P13j | The card | **No PIN attempt spent.** Nothing is typed, so `C_Login` is never called with a wrong value. This run costs the card nothing |
| P13k | What is written | **Nothing.** No signed file, no audit entry — `recordAudit` is inside `runBatch`, which is never reached, so the chain stays at **332** |
| P13l | `config.json` afterwards | **Byte-for-byte the backup**, digest compared and recorded |

### What would make the row unreadable rather than false, in which case I stop and say so

- **No dialog of ours at all** — every module answering "not mine", so the walk falls through to CNG and Windows' prompt appears instead. Then `preferPKCS11` did not reach this path and nothing about C21 was tested.
- **Two dialogs.** I stop at the first one, measure the row against it, and record the second as its own finding rather than folding it in.
- **The dialog never appearing because NetSeT hangs.** That is D45's ground, not this row's.

**If it does hang, that is the row false, and I will say so and then kill the process I started — announced first.**
