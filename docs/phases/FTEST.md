# FTEST — Unattended testing

**Prerequisite reading:** `docs/SPEC.md` in full, then `docs/decisions.md` in full.

**Prerequisite phase:** F6b complete. The product signs PDFs with Serbian qualified certificates, in one window, with a visual placement picker, verified against real hardware from two different issuers.

**This phase is not development. It is testing.** You are not adding features. You are finding out what breaks.

**You will be running unattended.** The owner will not be at the machine. Decide for yourself what to run, in what order, and how long to spend. Nobody will answer a question, so where this document leaves something open, choose, act, and record what you chose.

---

## 0. The boundaries, which do not move

You are running for hours on someone else's working computer with their qualified signing certificate in the reader. Everything below is a hard limit.

### 0.1 There is no PIN and there will not be one

The card is in the reader. **You cannot sign with it and must not try.** The owner deliberately did not give you the PIN, and that decision is correct: SPEC §6.5 establishes that the human pressing Approve is the only thing that reliably prevents an unauthorised signature, and a PIN in your hands would make that sentence false on this machine.

Everything that needs a private key runs through the **soft token**, which exercises the same `keysource.Session` interface and the entire PDF pipeline behind it.

The real card is still useful — for enumeration, classification, presence detection, provider behaviour and reader state. Use it for those and stop at the point a PIN would be required.

If a PIN dialog appears, something has gone wrong. Stop that line of work, close the dialog, and record it as a finding.

### 0.2 No synthetic input, ever

D-094. No `SetCursorPos`, no `mouse_event`, no `SendInput`, no `keybd_event`. A synthetic click on this machine once landed on the owner's desktop; had the pointer been over a PIN dialog it would have been a qualified signature nobody authorised.

Window messages posted to your own windows are permitted and are what previous sessions used. Driving a page through `Window.Eval` inside its own DOM is permitted. Screenshots with `PrintWindow` are permitted and do not steal the foreground.

### 0.3 Never kill processes by image name

A broad `taskkill /IM` once killed Windows Search and WhatsApp on this machine. Match on the command line, kill by exact PID, and only processes you started.

### 0.4 Leave the machine as you found it

This matters more than usual because nobody is watching.

Before you start, snapshot and record:

- `%LOCALAPPDATA%\Liro\config.json`
- `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` — the Liro value and what it points at
- `HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\` — the Explorer verb and its command
- The audit log directory

Restore all of it before you finish, and verify the restoration rather than assuming it. `internal/platform`'s own tests are known to write to the real registry and restore only whether an entry exists, not what it pointed at — that is a finding in itself, and one you should fix.

Write your work into a temporary directory and clean it up. Do not leave gigabytes of generated PDFs on the owner's disk.

### 0.5 Git

Commit freely; the history is how the owner sees what you did. Push when the suite is green.

Never force-push, never rewrite history, never delete a branch, and never commit anything under a `local/` directory — those hold real certificates, real client documents and a private key, and the repository is public.

Run `git ls-files | Select-String "local/"` before every push. Only `README.md` files may appear.

### 0.6 What you must never do to make something pass

If a test fails, the test is right until proven otherwise. Do not weaken an assertion, do not add `//nolint`, do not delete a case, and do not adjust an expected value to match what the code produced.

This project has three recorded instances where a green suite accompanied a broken product — an xref parser silently losing 92% of a real document's objects, a font subsetter writing zero for every glyph's left side bearing, and settings that saved correctly and were then read from a stale copy. Every one was found by looking at real output, not by running tests.

---

## 1. What you are looking for

Not "does it pass". Anything that would make a bookkeeper's afternoon worse:

- Something that crashes, hangs, or silently does nothing
- Something that produces a wrong result and reports success
- Something that leaves a half-written file, an orphaned process, a registry entry or a locked handle
- Something that degrades over hours or over volume
- A message that does not say what to do
- A window that clips, overflows or scrolls when it should not
- Anything that behaves differently in one of the three languages

---

## 2. The real card, without a PIN

Everything here stops short of a private key.

- Enumerate with the card in, and again with it out. Both certificates classified correctly, presence per certificate.
- Remove the card while `certs` is running. Insert it during. Insert a different card.
- Unplug the reader entirely; plug it back.
- Stop the Windows Smart Card service, run `certs`, start it again. The distinct error code exists for this; confirm the message tells the user to start a service rather than to insert a card.
- Run `certs` a thousand times in a loop. Watch handle count and memory. A leak in the presence probe would show here and nowhere else.
- Confirm the certificate list in the window matches `certs`, and that `--all` differs in exactly the way it should.

## 3. Everything the soft token can reach

The full pipeline: parse, incremental update, placeholder, CMS, timestamp, DSS, stamp, verify.

- Sign every document you can find or generate. Verify every result with `internal/pades/verify`, and cross-check a sample with OpenSSL and, if you can install it, pyHanko.
- Sign the same document a thousand times. Compare outputs: with a fixed key and a fixed timestamp they should be byte-identical, and any drift is a finding.
- Sign an already-signed document, then sign that, and again — five deep. Every signature must still verify at every depth.
- Sign at each level: B-B, B-T against Pošta's test TSA and against `freetsa.org`, B-LT where revocation is reachable.
- Every stamp position: four corners, explicit coordinates, all four page rotations, first page, last page, a middle page.

## 4. Documents

Generate a corpus. Vary what actually varies in the wild:

- Producers: Word, LibreOffice, scanners, ERP output, the eGovernment applet, whatever you can synthesise convincingly
- Page counts from 1 to 500
- Classic xref tables, cross-reference streams, mixed histories, object streams
- Page sizes: A4, A5, A3, US Letter, and something non-standard
- Rotations 0, 90, 180, 270, and different rotations on different pages of one document
- Already carrying one, two and three signatures
- Cyrillic, Latin and mixed content; embedded fonts and substituted ones
- Scanned images: CCITT G3 and G4, JPEG, and — to confirm they degrade rather than fail — JBIG2 and JPEG 2000

Then the abuse, all of which F6 §7 listed and which you should now confirm against the shipped binary rather than in tests:

Locked by another process. Read-only. Zero bytes. Not a PDF despite the extension. Encrypted. Truncated mid-object. A PDF with a corrupt `startxref`. The same file listed twice. Two files of the same name from different folders. A path longer than 260 characters. Names with Cyrillic, spaces, emoji, and a right-to-left override. A folder dropped where a file was expected. Two hundred files at once. One corrupt file among a hundred good ones. Output opened in another program while being written. A network path that disappears. A disk that fills.

For each: **a clear message and a clean state.** Never a crash, never a silent skip, never a half-written file.

## 5. The renderer

`internal/pades/render` is 4,600 lines written in this project and is the newest and least exercised code in it.

- Render every page of every document in your corpus. Nothing may panic, hang or allocate unboundedly.
- Compare a sample against an independent renderer if you can obtain one. If you cannot, say so — do not claim visual correctness you have not established.
- Confirm the counted notes for substituted fonts, skipped JBIG2 and flattened patterns actually fire, and that a document relying on them still renders something usable.
- Time it: a page at Fit, at 400%, a dense page, a scanned page, a 500-page document.
- Confirm laziness: a 500-page document renders three pages, not 500.

## 6. Fuzzing

The PDF parser accepts files from outside. So now does the renderer.

- Run the existing parser fuzz target for **at least an hour**, and longer if it is still finding things.
- Add a fuzz target for the renderer, seeded from your corpus.
- Add one for the CMS and timestamp parsers — they read attacker-supplied bytes from an existing document.
- Any crash found is a finding, is fixed, and gets a regression test with the crashing input committed.

## 7. Endurance

- Leave the tray running for several hours. Watch memory, handles, GDI objects and thread count.
- Open and close every window a hundred times each. The window layer has a recorded history of lifetime defects — D-099, D-101, D-114, D-129 — and this is how the next one surfaces.
- Sign a thousand documents in one session with the soft token. Watch the same counters and the timing.
- Run the whole test suite twenty times. Any test that fails once in twenty is a flake, and a flake is a finding with a cause, not something to re-run.

## 8. Failure injection

- A TSA that never answers, one that answers slowly, one that returns 500, one that returns 400, one that returns malformed bytes, one that returns a valid token for the wrong digest.
- An OCSP responder that hangs, and a CRL that is enormous — MUP's is 29 MB and the cap exists for it.
- The Trusted List server unreachable, returning garbage, returning an older sequence, returning a list with a broken signature.
- Configuration file corrupt, empty, unreadable, holding values out of range.
- The audit log unwritable, and its chain deliberately broken at a known entry.
- Disk full during a batch.

Each must produce a clear message and leave a clean state.

## 9. The three languages

- Every window in `sr-Latn`, `sr-Cyrl` and `en`, at every step, with realistic content.
- Nothing clipped, nothing overflowing, no unexpected scrolling. Serbian labels run longer than English ones, and Cyrillic longer still.
- Every error code reaches a message in all three; a test already enforces this, so confirm it is not vacuous.
- Every string the person can see is localised, and every log line and every help text is English.

## 10. What you decide

Where this document is silent, choose. Where it asks for something that turns out to be impossible or worthless, say so and do something better with the time.

If you find something that is clearly a defect, **fix it**, with a test that fails against the old code.

If you find something that is a judgement call — a message that could be clearer, a window that could be smaller — record it for the owner rather than changing it. He has opinions and they have been right.

If you find something that would take longer than the rest of this phase put together, record it and move on.

---

## 11. Reporting

Write to **`docs/ftest-report.md`** as you go, not at the end. If the session dies, the findings survive.

Structure it as:

- **What broke** — every defect found, what it was, whether you fixed it, and the test that now covers it
- **What held** — what you exercised that behaved correctly, with the numbers
- **What you could not test** — honestly, and why
- **For the owner** — judgement calls, and what needs his hands

Numbers throughout. "Signed a thousand documents, 1000/1000 verified, median 214 ms, memory flat at 41 MB" is worth more than "signing works".

Commit the report as you go.

---

## 12. What remains the owner's

Do not attempt these; list them in the report so he knows what is waiting.

- One batch with the real card and a real PIN — the only thing that proves one PIN covers a batch on this machine, and it has already been measured at 20 documents on two issuers
- Removing the card mid-batch with a real PIN entered
- Cancelling a real PIN dialog
- Output through the eGovernment validator, and through PKS or Inception
- Another machine entirely: a clean Windows with no middleware, no WebView2 runtime, and SmartScreen on first run

The last one is deferred until later phases by his own decision — installing Go on someone else's computer is not something he can ask for. Note it and move on.
