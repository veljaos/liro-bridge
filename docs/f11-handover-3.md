# F11 handover 3 — item 3, and the one thing to decide before writing any of it

**Written:** 2026-09-15, at the end of the third session of F11.
**Master:** pushed, working tree clean.

> **CLOSED, 2026-09-15. This document is history — read `docs/f11-report.md`
> instead.**
>
> It was written mid-phase to carry item 3 across a session boundary, and item
> 3 is done: the native dialog was chosen ([[D-277]]), built in three locales
> and looked at, the login step and `SignDigest` were written to the guard
> ([[D-279]]), and **the exit condition was met on one PIN, first attempt**
> ([[D-280]]). §9's three remaining things are all answered there.
>
> What is left of F11 is §4 and it is gated; the report's §7 says what that
> means and `cmd/liro-bridge/pkcs11reach_test.go` enforces it. Nothing below
> is a live instruction. It is kept because §2 is the argument that produced
> [[D-277]] and SPEC §10's amendment, and that reasoning is worth more than
> the summary of it.

**This is not a summary of the phase.** `docs/f11-handover-2.md` is that, and
D-272 through D-276 carry everything this session established. This document is
about **item 3 — the login step, the PIN screen and `SignDigest`** — and it
exists because the hard part of it is not where it looks, and is much cheaper
to face before the work than inside it.

Read D-273 (why all eight clauses are live), D-269 (the clauses themselves and
why the guard is older than the code), and SPEC §6.5.1 as amended. Then this.

---

## 1. The wall is down, and what is behind it is not what was queued

[[D-273]] closed the question the phase was waiting on: SafeSign's Pošta token
does **not** advertise a protected authentication path, so §6.5.1's fallback is
the live arrangement on the exit condition's own path and all eight clauses are
requirements. `Source.Open` can stop returning `ErrLoginNotBuilt`.

The queued reading of that was: build the fallback, add a PIN screen in three
locales, then `SignDigest`. The PIN screen was expected to be the *large* piece
of work and the backend the delicate one.

**It is the other way round, and the reason is clause 2.**

---

## 2. The hard part: a PIN cannot travel through a WebView2 page and satisfy clause 2

> **The PIN exists only for the duration of `C_Login`,** and is overwritten
> immediately afterwards. It is not left for the garbage collector, not held in
> a struct field, not captured by a closure that outlives the call, and not
> merely dropped.

Every window in this product is HTML in WebView2 (SPEC §10.2). Structured data
comes back from a page exactly one way, established four times over — the page
reports what a click meant and Go reads it through `ExecuteScript`'s own return
value ([[D-083]] settings, [[D-095]] the timestamp choice, [[D-103]] the stamp
choice, [[D-142]] the placement request). The page→Go message surface is three
types and is not to be widened.

So the natural PIN screen is an `<input type="password">`, an `approve`
message, and `Eval("window.__liroPIN()")`. **Trace where the PIN then is:**

| Where | Ours to overwrite? |
|---|---|
| the DOM node's `value`, in the renderer | **no** |
| the JS string, in V8's heap, plus whatever copies a GC has made | **no** |
| the IPC message from renderer to browser process | **no** |
| WebView2's own `LPCWSTR` result buffer | no — `[in]`, the callee's memory (`webview2_windows.go:200-213`) |
| the Go `string` `Eval` returns (`window_windows.go:825`) | **no — a Go string is immutable and cannot be zeroed** |

The last row alone settles it. `Eval` is `func (w *window) Eval(script string)
(string, error)`, and `executeScriptCompletedInvoke` copies the result out with
`windows.UTF16PtrToString`. A Go `string` cannot be overwritten, so clause 2's
"not merely dropped" is unachievable by construction the moment the PIN is one.

And the rows above it are worse, because **the page does not run in this
process.** [[D-259]] measured that directly while chasing a different defect:
the window under a WebView2 host that carries the drop target belongs to
`msedgewebview2`, at a pid that is not the agent's. A PIN typed into a page is
in another process's heap, for an unbounded time, in memory this program cannot
reach, let alone wipe.

**So the sentence to hold onto: clause 2 is a statement about this program's
memory, and a WebView2 page is not this program's memory.**

This is not an argument that the arrangement is *insecure* — §6.5 already
establishes that the PIN is not an access-control boundary and the consent
screen is the gate, which is the whole reason [[D-269]] permitted a PIN in
memory at all. It is an argument that **the clause as written cannot be
honoured on that path**, and this project does not ship a clause it does not
meet.

---

## 3. So the first decision is not how to plumb it — it is where the PIN screen lives

> **Settled, 2026-09-15: (a), the native Win32 dialog — see [[D-277]].** The
> options below stand as the reasoning that produced the ruling rather than as
> a live question. D-277 also carries the owner's own reason, which is not
> below: Windows already collects the PIN in its own window on the CNG path, so
> a native dialog is the *familiar* thing and an HTML PIN box would be the
> novel one — and novelty is the wrong quality for that screen. It records the
> price as a price, the clause 6 tension the choice creates, and that the
> dialog is not to be tidied into a page later.

**Do not start writing the login step until this is answered.** Its shape
depends entirely on the answer, and building the backend first is building for
a question nobody has closed — which is the mistake [[D-271]] avoided at the
wall and this document exists to avoid again one step further on.

Three options, with what each costs:

**(a) A native Win32 dialog.** A real window, an edit control with
`ES_PASSWORD`, `WM_GETTEXT` into a buffer this program allocated, `C_Login`,
zero the buffer, `WM_SETTEXT` the control to empty, destroy the window. The PIN
never leaves this process and every byte holding it is ours.

- Satisfies clause 2 completely, and it is the only option that does.
- It is the **only window in the product that is not HTML**, against SPEC
  §10.2's "the agent's windows are HTML rendered in an embedded browser view"
  and §10.1's token rule, which a native control cannot use.
- Clause 6 — *the screen says whose PIN it is* — is a design requirement on a
  surface that would have none of this project's design system. Three locales
  is the easy half; looking like the rest of the product is not.
- Testable without synthetic input: driven by window messages, which is what
  [[D-134]] and [[D-166]] already do for the tray ([[D-094]] forbids the
  cursor, not `WM_SETTEXT`).

**(b) The WebView2 page, and amend clause 2.** Narrow it to what is actually
achievable — say, the PIN is overwritten wherever this program can reach it and
the page's own copy is cleared and navigated away from. Honest, and it is a
SPEC amendment to the paragraph the specification calls its most important,
which is the owner's and nobody else's.

**(c) The page collects nothing and only opens (a).** (a) with a step in front
of it, and [[D-148]]'s whole finding was that a step which asks nothing is not
a step.

**The recommendation is (a), and the reason is that (b) is a promise being
resized to fit an implementation.** But it is the owner's call, because it
touches SPEC §10 either way and possibly §6.5.1, and because [[D-225]] and
[[D-232]] both record that a phase which edits the specification to suit itself
is a phase that can soften a constraint by rewording it.

---

## 4. What the login step looks like once that is settled

The guard already written ([[D-269]], [[D-270]], `internal/keysource/pkcs11/pin_test.go`,
`internal/pinname`) is not a check to pass afterwards. It is the shape:

- No struct field, no function parameter, no named result, no package-level var
  in this package may be named after a PIN and be of a type that could hold
  one. **A local variable inside one function is the only place permitted.**
- Which means **the function that obtains the PIN is the function that calls
  `C_Login`.** There is no `login(session, pin []byte)` to write, no helper to
  pass it to, and no second function that has ever seen it. That is the
  inconvenience [[D-269]] predicted and called "the rule working".

Five more things that are easy to get wrong and are not obvious:

1. **Size the buffer from `ulMaxPinLen`, refuse longer input here.** Clause 7,
   and SPEC §6.5.1 now carries why: this token declares **5 and 15** where the
   MUP token declares **4 and 8**, and both cards live in one person's drawer.
   Nothing may hard-code either pair.
2. **Pin the buffer** (`runtime.Pinner`) before its address crosses into the
   module — [[D-101]]'s finding, unchanged: `KeepAlive` stops collection and
   says nothing about a *copy*, and a stack that grows moves the frame.
3. **Zeroing is a thing to verify, not to assume.** A wipe loop over a buffer
   nothing reads afterwards is a wipe the compiler is permitted to notice. Read
   the bytes back through the pinned address and assert they are zero, in a
   test, rather than trusting the loop.
4. **Nothing retries.** Clause 5. No loop, no caller that calls it twice, and a
   test that says so — this card has three attempts and [[D-268]] already spent
   one of the MUP card's.
5. **`C_Login` is called once, per session, by a human who typed it.** The
   first real one is the owner's and it is not a measurement.

---

## 5. `SignDigest`, and the only way it may be verified

`CKM_RSA_PKCS`, which signs a pre-built DigestInfo — not `CKM_SHA256_RSA_PKCS`,
which hashes the data itself and would sign a hash of a hash. Both are offered
by this token; so is `CKM_SHA1_RSA_PKCS`, which SPEC §18.8 forbids producing.

**F11 §2.1 is explicit and it is the trap of the whole phase: a wrong mechanism
verifies against nothing while every layer reports success.** Verified with the
independent verifier of SPEC §16.4, never by observing that bytes came back.
[[D-271]] records the sharper version of the same rule for the struct layouts —
three wrong `CK_ATTRIBUTE` shapes return `CKR_OK` with a zero length.

`Chain` will be empty on this card ([[D-274]]) and that is correct rather than
a bug to fix here: the card carries the root, the signer's issuer is the
intermediate, and the intermediate is on neither Serbian card. So the exit
condition's document will reach **B-T, not B-LT** ([[D-159]], [[D-079]]), and
the report must say so rather than let it read as a failure.

---

## 6. What the owner is needed for, and when

He has said he will be at the machine, and that the PIN is one call with no
retry.

| When | What |
|---|---|
| **before any code** | the §3 decision — where the PIN screen lives |
| after the screen exists | sight of it, in three locales, before it is final ([[D-208]]: a green layout suite is not evidence about what a window shows) |
| the first `C_Login` | **one PIN, typed by him.** One call. The card has three attempts and all three flags are clear |
| the exit condition | at the machine — the consent window, the certificate row, Approve |

---

## 7. Three things carried forward that will otherwise cost an hour each

**`go test ./internal/keysource/pkcs11/` goes red about once in a hundred
runs, and it is not your change.** Four occurrences measured, the last two out
of routine check runs with only a doc comment between them ([[D-272]]'s table).
It passes on the re-run, which is exactly the shape that gets it dismissed.
Point at the table, do not re-derive it, and do not chase it: [[D-275]] defers
the remedy and gates the only thing that would make it matter.

**Do not wire discovery into the agent.** `cmd/liro-bridge/pkcs11reach_test.go`
will fail and tell you why. Deleting that test is part of building the remedy,
not a way around it ([[D-275]]).

**`go test ./cmd/liro-bridge/` writes `HKCU\…\Run\LiroBridge` at a temporary
binary** ([[D-266]]). Snapshot it by `reg export` first and compare **value by
value**, never by hashing the export. And snapshot every file **by copy, not by
hash** ([[D-153]], [[D-243]]) — a hash proves something changed and only a copy
can put it back.

---

## 8. Two smaller things this session established that are easy to miss

**This machine's module versions differ from the other machine's** — SafeSign
3.9.32.1 against 3.9.24.1, TrustEdgeID 1.1.3.2 against 1.1.3.3, and no Nexus at
all ([[D-272]]). So discovery can assume neither a path nor a version, and a
measurement taken through a module is a measurement of *that build*. Before
quoting [[D-271]]'s mechanism lists or [[D-268]]'s `minPin` at a third machine,
check which build answered.

**The Pošta card's certificate is already in the Windows `My` store**, created
2026-09-05 by `CertPropSvc` and re-touched whenever the card is inserted. It is
not something this project put there and not something to restore; it is worth
knowing before a snapshot comparison reports it as a change.

---

## 9. What actually remains, written after the build

Item 3 is done to the last line before the card. [[D-279]] is the entry;
this is the short form for whoever picks it up.

### Three things, and all three need the card

1. **`C_Login` with a correct PIN**, and `privateKeyFor` then finding a key
   that was invisible a moment earlier. **One call. No retry.** The card has
   three attempts and all three of its user-PIN flags are clear.
2. **A signature verified with the independent verifier of SPEC §16.4.**
   [[D-279]] §4 proves this package builds the same DigestInfo the standard
   library does, by signing one key two ways and requiring the signatures to be
   identical. That is not the same as the card signing it into something that
   verifies, and F11 §2.1 forbids accepting "bytes came back" as evidence.
3. **The exit condition**: that PDF, verified by the independent verifier and
   by one external tool.

### What the owner does, in order

| | |
|---|---|
| 1 | Look at the PIN dialog. Three captures were taken and two defects came out of looking at the first — it is worth one more pair of eyes before a PIN goes into it. |
| 2 | Type the PIN, once, at the machine. |
| 3 | Approve, at the machine, for the exit condition. |

### Two things to expect rather than to diagnose

**The document will be B-T, not B-LT**, and that is [[D-274]]: this card
carries the root, its signer's issuer is the intermediate, and the intermediate
is on neither Serbian card, so `Chain` is empty and `/DSS` has nothing to
carry. It is the measured consequence of the card, not a failure of the phase,
and the report should say so in those words.

**`go test ./internal/keysource/pkcs11/` goes red about once in a hundred
runs.** Four occurrences measured, the last two out of routine check runs
([[D-272]]'s table). It is not your change. Do not chase it — [[D-275]] defers
the remedy and gates the only thing that would make it matter.

### Three things not to do

- **Do not wire the backend into the agent.**
  `cmd/liro-bridge/pkcs11reach_test.go` will fail and say why. It already
  stopped this session writing the `PINRequest`→`PINPrompt` adapter, which is
  the dozen lines that join the two halves; they belong with §4's wiring,
  after [[D-275]]'s remedy.
- **Do not delete the eight `pindialog.*` catalogue keys** as unread surface.
  They have a named reader arriving with §4 and are read now by the
  three-locale test ([[D-279]] §7).
- **Do not add a `t.Skip`-guarded test that calls `C_Login`.** Considered and
  rejected: a test that spends a PIN attempt when an environment variable is
  set is a test somebody sets that variable for while running the whole suite.
  The first `C_Login` is a deliberate act with a person watching.

**`TestOpenTakesNoArgumentsAndPointsAtSignInstead` fails on this machine, and
it is not yours either.** Measured 4 of 4 on the working tree *and* 4 of 4 in a
worktree at the commit before this session's build, so it predates the work:

```
testing.go:1617: TempDir RemoveAll cleanup:
  unlinkat ...\AppData\Local\Liro\webview2\WebView2Loader-163680.dll: Access is denied.
```

It is a cleanup failure rather than an assertion — `t.TempDir` cannot remove
the extracted WebView2 loader because something still has it loaded — which is
the family [[D-172]] recorded as C-6, "1.26 GB of temporary directories the
suite could not delete". Left alone: it is not this phase's, and a fix is a
change to how the suite gets and releases a config home, which is the same
decision [[D-266]] left open and named as bigger than one line.
