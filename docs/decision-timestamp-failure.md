# A timestamp that is not obtained — what the program does, and the owner's options (C26)

**Status: decided by the owner and built (D-469)**: refuse wherever a level was asked for; the library refuses by default; the shortfall is said. Still the owner's: whether Settings counts as asked (it needs the default level changed first). Was: Written in session 33 at the owner's request (D-465). Nothing here is built.

## 1. I told the owner something the code does not do

At the end of session 33 I wrote that "a failed timestamp is saved as B-B rather than reported". That came from the real-TSA test, which builds `pades.Options` without setting `OnTSAFailureAbort`. **It is not what a release build does.** Reading every caller (below) shows no path on which a person or a calling program gets B-B without either asking for it or being asked.

What *is* true leaves three things worth deciding. They are §3, and only one of them is about B-B.

## 2. What happens today (read in the code, 2026-10-12; none of it run with a card in this session)

| path | no TSA configured | TSA configured, fails (after F3 §6.3's three attempts) | what is said |
|---|---|---|---|
| **The window** (local batch) | Before the card, the person is asked (SPEC §12.8): *Sign without a timestamp* / *Configure a timestamp authority* / cancel (`mainwindow.go` `startSigning`, `consent.TSAReasonNotConfigured`) | Signing stops on that document, and the same three-way question is asked. The answer holds for the rest of the batch (`mainwindow.go` ~1320–1347) | The report states the level reached. B-B shows in the warning colour with "Level B-B — no timestamp" (`ui_payloads.go` `achievedLevelIntent`/`achievedLevelNote`) |
| **The protocol** (an ERP; the cloud channel goes through the same path) | The same question, asked of the person before the card, because `startSigning` serves both | The document **fails** to the caller with `TSA_UNAVAILABLE` or `TSA_REJECTED`. Nobody is asked: "there is nobody watching the window by the time a batch it approved is running" (`mainwindow.go` ~1300). This applies unless the person had already chosen B-B | `achievedLevel` per document in the result (`api/jobhandlers.go:75`) |
| A caller or Settings **asking for B-B** | No timestamp is attempted at all. That is a request, not a failure (`sign.go` ~266–275) | — | B-B, said as above |
| The CLI's `sign` | Not in a release build (`internal/cli/sign_noconsent.go` is `//go:build softtoken`). It aborts by default; `--on-tsa-failure=b-b` opts in | | |

## 3. What is worth deciding

### 3.1 The library's safe behaviour is not its zero value

`pades.Options.OnTSAFailureAbort`'s comment says "true (the default) aborts". But a Go field's default is its zero value, which is **false**: save at B-B with a line in `Result.Notes`. Every production caller sets the field explicitly (`interactive.go:559`, `protocolflow.go:212`, `sign_noconsent.go:347`). A future caller that forgets it gets exactly the quiet downgrade the owner does not want, and the tests that leave it unset are the ones that misled me.

- **Option A (recommended): invert the field** to `SaveWithoutTimestamp bool`, so the zero value refuses. Mechanical: three production callers and the tests. No behaviour changes for anyone.
- Option B: keep it, and add a test that every non-test caller sets it. Weaker: a rule kept by a test rather than by the type.

### 3.2 Whether "sign without a timestamp" should be offered *after a failure* at all

Today, when the authority fails mid-batch, the person may answer *Sign without a timestamp*, and the report says B-B in the warning colour. It is not quiet, but it is offered at the moment when the person most wants the work done.

- **Option A: keep it** (SPEC §12.8 as written).
- **Option B: refuse after a failure.** After a failure, the question offers only *Configure* and *Cancel*. B-B remains reachable only as a deliberate standing choice (Settings level B-B, or a caller's explicit `level: "b-b"`), never as a way past a dead authority. This is closest to the owner's "I would rather it refuse".
- Option C: B, and also before a batch when no authority is configured. Effectively: no TSA, no signing, unless B-B was chosen in Settings. Strictest, and it ends signing for anyone who has not configured an authority.

### 3.3 The downgrade people actually meet: B-LT asked, B-T saved

This is the one the owner's sentence describes, and it is not about B-B. The default level is B-LT (`config.go`, SPEC §12.6), and **no Serbian card has reached B-LT** (D-466). So, as the code stands, a person on a default install who signs successfully gets **B-T**:
- The report shows "B-T" in the **positive** colour, with no sentence saying B-LT was asked for and not reached (`achievedLevelNote` speaks only for B-B).
- The calling program gets `achievedLevel: "B-T"` and no field saying what was requested.
- Why it fell short is in `Result.Notes`, which is English and is not shown.

The level is stated, so it is not hidden; but nothing says it fell short.

- **Option A: say it.** Under a level below the one requested, the report adds a sentence in the warning colour: "B-LT was requested; this reached B-T because …", with the reason. The protocol result gains `requestedLevel` beside `achievedLevel`. Nothing is refused.
- **Option B: refuse below the requested level.** The document fails with a new code (for example `LEVEL_NOT_REACHED`) and nothing is saved. Today this would make **every default-configured Serbian signature fail**, so it only makes sense together with C.
- **Option C: make B-T the default** until B-LT has been reached on a real Serbian card. A person who asks for B-LT then asks knowingly, and B (refuse) can be the rule for an explicit request.
- A variant of B worth knowing: B-LT evidence for the **signer's** chain can be gathered **before the PIN**, since the certificate is known by then. The batch could then refuse before the card is used, not after. The timestamp authority's evidence can only come after.

**My recommendation:** 3.1 A now. For 3.2 and 3.3 I would take **3.2 B, 3.3 C, then 3.3 B for an explicit B-LT request, with 3.3 A's sentence in every case where a level falls short**. That refuses rather than downgrades wherever someone asked for something specific, without making a default install unable to sign. 3.3 C changes SPEC §12.6's default and is the owner's alone.

## 4. Not known, and what would tell

- Whether any Serbian card reaches B-LT with the current code: one signing at B-LT with the Pošta card (D-466).
- How a dead authority looks to the person in the window: never seen in this session. The three-way question was measured in earlier phases (F5).
