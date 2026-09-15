# F11 handover 2 — PKCS#11, at the wall

**Written:** 2026-09-15, at the end of the second session of F11.
**State:** §1 settled. **§5 answered and ruled on.** §2 and §3 built up to
`C_Login` and stopped there. **Nothing below `C_Login` exists.**
**Master:** `fb65452`, working tree clean.

Read `docs/f11-handover.md` first — it carries §1 and everything expensive
about the first session. This one carries what happened after it. Read
`docs/f11-home-list.md` too: it is the one thing the phase is waiting on, and
this document deliberately does not repeat it.

---

## 1. Where §2 stopped, and why the wall is where it is

`internal/keysource/pkcs11` implements everything above `C_Login` and refuses
below it. `Source.Open` returns `ErrLoginNotBuilt`, a sentinel, and always
will until the home readings land.

**`SignDigest` is behind the login step, not beside it.** `C_Sign` on a
token's private key needs a logged-in session, so there is no arrangement in
which signing gets built while the login question stays open. That is further
than "hold the login step" sounds and it is the single most important sentence
in this document: **the home readings are not a narrowing question, they are
the thing the rest of the phase waits on.**

The reason the login step was not simply written to §6.5.1's fallback anyway:
SPEC §6.5.1's first clause uses the protected authentication path wherever a
module offers one and only falls back to asking where it does not, and
SafeSign — the module F11's own exit condition turns on — has never been asked
which it is, because it answers `CKR_TOKEN_NOT_RECOGNIZED` for the MUP card.
The reading costs nothing and spends no PIN attempt. Building the fallback
first is building for a question nobody has closed.

---

## 2. §5 is closed. The measurement cost an attempt

[[D-268]] took the one authorised `C_Login(session, CKU_USER, NULL, 0)` on the
MUP token, with the owner at the machine:

```
C_Login(NULL PIN)  ->  CKR_PIN_INCORRECT (0xA0)
PIN counter        0x10040D -> 0x11040D     USER_PIN_COUNT_LOW set
```

**An attempt of three was consumed.** No dialog appeared — recorded by a
watcher rather than merely unnoticed. The module takes a NULL PIN as an empty
one and hands it to the card.

The counter was cleared afterwards by an ordinary signature **through Adobe on
the CNG path**, which is a better result than the one that was being chased: a
CNG signature reset a counter a PKCS#11 login had raised, so the card's
authentication state really is the card's, shared across every interface. That
is SPEC §6.5's own premise, measured from a direction §6.5 never used, and it
is what the amendment rests on.

[[D-269]] is the owner's ruling and **SPEC §6.5.1** is the amendment — the
first ever made to the paragraph the specification calls its most important.
Eight clauses, each a requirement. Read it before writing the login step; it
is the specification for that step and not background.

**Do not take another `C_Login` measurement.** The question it answered is
answered. Nothing on the home list logs in.

---

## 3. What is built and verified, and what is built and unexercisable

**Verified against the real card**, through both NetSeT modules:

| | |
|---|---|
| module loading, `C_GetFunctionList`, `C_Initialize` | all four modules on this machine |
| slots, `C_GetTokenInfo`, flags, `minPin`/`maxPin` | measured |
| mechanism list | 15 from TrustEdgeID, 21 from Celik; both offer `CKM_RSA_PKCS` |
| the packed `CK_ATTRIBUTE` marshalling | both certificates come off and parse |
| `List` and deduplication | two certificates, one row each |
| discovery | four modules found, all four answer |

The certificate read cross-checks against four facts already in the log —
[[D-209]]'s serial and thumbprint, [[D-243]]'s thumbprint, [[D-149]]'s
authentication twin, and SPEC §11.4's key usages exactly. So the bytes are
right rather than accidentally plausible. **That standard matters here more
than usual: three wrong `CK_ATTRIBUTE` layouts return `CKR_OK` with a zero
length, so a call succeeding is not evidence.**

**Built and unexercisable until login exists:**

- `Chain` — `issuersFor` is written and tested against a synthetic PKI, but
  `Session.Chain` cannot be called because no session can be opened.
- `SignDigest` — does not exist at all.
- **Which mechanism is actually asked for.** F11 §2.1 is explicit that this is
  verified with the independent verifier of SPEC §16.4 and never by observing
  that bytes came back, because a wrong mechanism verifies against nothing
  while every layer reports success. Nothing about that can be checked yet.

---

## 4. `Chain` is empty on a MUP card, and that is a missing signature level

**Measured, both NetSeT modules: the token carries two certificates — the
signing one and its authentication twin — and no CA certificate at all. Both
are issued by "MUP Gradjani CA 4", which is not on the card.**

SPEC §11.8 already records this from the other side: MUP embeds **1**
certificate in the CMS of a signed document where Halcom and Pošta embed 3.

**What it costs, because it reads like a field that happens to be blank and it
is not.** SPEC §12.6 makes **B-LT the default** level. B-LT is B-T plus a
`/DSS` carrying revocation evidence, and revocation is collected *per
certificate in the chain*. [[D-159]] already found the sharp edge: a chain of
exactly one certificate expects no evidence at all, so `/DSS` comes out empty,
[[D-079]] then correctly writes no revision, and the document is **B-T**.

So on this path the default level silently becomes B-T unless the chain is
completed from the certificate's own AIA `caIssuers` or from a bundled trust
store — which SPEC §11.8 already requires for MUP, and which is the caller's
work rather than this layer's (`keysource.Session.Chain`'s own contract says
the chain may be empty and the caller completes it).

**It is a property of the card, not of anything written here.** Whether the
Pošta card is the same is on the home list, and SPEC §11.8 suggests it is not.

---

## 5. Two things `List` cannot do

**It cannot tell which certificates have a private key behind them.** Measured
on the MUP card: a public session sees **2 certificates, 2 public keys and 0
private keys**, because private objects are hidden from a session that has not
logged in. So "can this certificate actually sign" is not a question this layer
answers — `internal/trust/classify` answers the part that matters from KeyUsage
(SPEC §11.4: `contentCommitment`, never `digitalSignature`), and the card
answers the rest when a signature is attempted.

Do not be tempted to find out by logging in during enumeration. SPEC §11.10
spent a whole rule keeping a PIN prompt out of a liveness check on the CNG
side, and the reasoning is the same here.

**The order objects come back in differs between builds.** TrustEdgeID 1.1.3.3
returns Sign-then-Auth; MUP RS\Celik 1.1.0.0 returns Auth-then-Sign. Same card,
same two certificates, opposite order. Nothing relies on it and nothing should.

---

## 6. Discovery's two findings

**The entry-point test earns itself.** A file is a module because it exports
`C_GetFunctionList` and answers `C_GetInfo`, never because its name looks
promising. This machine carries
`C:\Program Files\SecurityTray\lib\pkcs11wrapper_64.dll` — "pkcs11" in the
name, sitting exactly where a reasonable search would look, and it is IAIK's
Java JNI wrapper, which *consumes* PKCS#11 modules rather than being one and
exports no `C_GetFunctionList`. Leaving it out of a list is not what protects
against it; the entry-point test is.

**Probing is not silent.** Every candidate is loaded, which runs its
`DllMain`. Loading Nexus's `personal64.dll` writes a line of its own to
stderr — `Personal::config::file::read: Personal config file '…' does not
exist` — a foreign library talking to a console this program did not open for
it. Nothing here can stop it. [[D-254]] gives back a console this process is
alone on before anything is written, so an agent started from Explorer has
none by then; a developer at a prompt will see it. **Probe when a listing is
wanted, never on a schedule.**

Also worth carrying: **NetSeT is installed at two paths in two builds five
years apart** and neither is canonical. Whether that is true of a second
machine is on the home list, and it is the difference between "discovery
cannot assume a version" and "discovery cannot assume a path".

---

## 7. What happens when the home output arrives

The whole of it turns on one line of `scripts/p11probe`'s output for the Pošta
card through SafeSign:

```
PROTECTED_AUTHENTICATION_PATH: true | false
```

**Clause 1 of §6.5.1 — use the protected path wherever a module advertises it
— is live in both outcomes and its check is already written**
(`tokenInfo.HasProtectedAuthenticationPath()`). The login step branches on it
per token, always. What changes is which branch the phase's exit condition
actually travels.

| §6.5.1 clause | SafeSign `true` | SafeSign `false` |
|---|---|---|
| 1. protected path used where offered | **live** — and it is the whole login step for Pošta | **live**, and never taken by any module measured so far |
| 2. PIN alive only for the call, then overwritten | falls away for Pošta; still needed for the Windows NetSeT fallback | **live, on the exit condition's own path** |
| 3. never logged, in an error, in a dump, in a report | as above | **live** |
| 4. never retained between signatures | as above | **live** |
| 5. nothing retries a PIN, ever | **live unconditionally** | **live unconditionally** |
| 6. the screen says whose PIN it is | not built for the exit condition; needed for the fallback | **live** — new UI surface, three locales |
| 7. `minPin`/`maxPin` enforced in this layer | still needed wherever a PIN is collected | **live** |
| 8. CNG path untouched | **live unconditionally** | **live unconditionally** |

**If `true`**, the exit condition — a Pošta card signed through SafeSign's
module — needs no PIN box at all: `C_Login(session, CKU_USER, NULL, 0)` and
the module collects the PIN itself. Note what else follows: SafeSign is the
only one of the three Serbian middlewares shipping macOS and Linux builds
(SPEC §11.11), and MUP ships no `.so` or `.dylib` at all (F11 §0.1) — so on
the platforms PKCS#11 exists to serve, SafeSign may be the only module there
is, and the fallback would be implemented for the Windows case where CNG has
failed rather than for the main path. That is a much smaller piece of work and
a much smaller risk.

**If `false`**, all eight clauses are live on the exit condition's own path,
the PIN screen is required before the exit condition can be met, and it is
real UI work in three locales rather than a backend detail.

**In either case, write the entry closing [[D-269]]'s open question first**,
with the measurement in it, before building anything on the answer.

---

## 8. The guard that will fire on whoever writes the login step

`internal/keysource/pkcs11/pin_test.go` fails if a PIN is held anywhere it
could outlive the call: a struct field, a function parameter, a named result,
or a package-level var. **A local variable inside one function is the one
place SPEC §6.5.1 permits, and the test is why.**

It was written before any of the backend existed, deliberately ([[D-269]]): a
test written after a backend is a test written around whatever that backend
already does. If its shape turns out to be inconvenient when the login step is
written, the inconvenience is the rule working — the PIN does not travel, so
there is no parameter to pass it through and no second function that has ever
seen it.

The matcher is `internal/pinname`, shared by four packages. [[D-270]] records
why it asks both what a declaration is called and what its type can hold: a
name-only rule demanded that `signing.PINPolicy` be renamed, and every
PIN-named declaration in this tree is about a PIN without being one.

What the guard does **not** establish, and what therefore has to be read for:
that the PIN is actually overwritten, that nothing copies it under an innocent
name, and that no closure captures it. A syntax tree cannot see any of those.

---

## 9. The package, file by file

```
pkcs11.go            the package doc: what governs this package, and why the test is older than the code
pin_test.go          the guard above
ckr.go / ckr_test.go CK_RV names, tested — a wrong table is worse than none, and this project has had one
module_windows.go    the binding: layouts, index map, sessions, packed CK_ATTRIBUTE
module_other.go      the stub, and why the layouts are wrong here rather than merely unavailable
source_windows.go    Source: Enumerate, List, Open (the wall), ChainFor
certificate.go       CertificateInfo, dedupe, ErrLoginNotBuilt
chain.go             issuersFor, and the B-LT consequence in full
thumbprint.go        SHA-1 uppercase hex, byte-identical to windowscng's
discover.go          Candidates, Sources, and where a configured path may come from
discover_windows.go  the four known paths, from the environment; Modules
discover_other.go    no known paths on macOS/Linux yet, and why that is a statement
```

Tests that need a real module opt in:

```
set LIRO_PKCS11_MODULE=C:\Program Files\TrustEdgeID\netsetpkcs11_x64.dll
go test ./internal/keysource/pkcs11/ -count=1 -v
```

All of them are read-only. `scripts/p11probe` is the standalone instrument;
read its doc comment before passing `--login`, which is the one thing in this
repository that can spend a PIN attempt.

---

## 10. Machine notes that cost time

**`go test ./cmd/liro-bridge/` writes `HKCU\…\Run\LiroBridge`, pointing at a
temporary binary, reproducibly** ([[D-266]]). It is why that suite was not run
this session. Snapshot the key before running it, or do not run it.

**`reg export` emits values in an unstable order**, so a hash over the export
file reports a change when nothing changed. Compare the `Run` key **value by
value**. A hash there over-reports as well as under-reports.

**PowerShell corrupts files on a `Get-Content -Raw | Set-Content`
round-trip.** It turned every `§` into `Â§` and every `—` into `â€”` in
`module_windows.go` — 14 sequences, caught by a grep, reverted from the commit
and redone. Every file in this project is full of both characters. **Use the
editing tools or `python`/`sed`; do not round-trip a source file through
PowerShell.**

**Snapshot by copy, not by hash** ([[D-243]], [[D-153]]). A hash proves
something changed; only a copy can put it back.

**Stop processes by exact PID**, never by image name ([[D-122]]).

---

## 11. What the next session should do, in order

1. **Nothing until the home output exists.** `docs/f11-home-list.md`, item 1.
2. Write the entry closing [[D-269]]'s open question with the measurement.
3. Build the login step to whichever of §6.5.1's two arrangements the answer
   names, against the table in §7 above.
4. Then `SignDigest` with `CKM_RSA_PKCS`, verified with the independent
   verifier of SPEC §16.4 — never by observing that a call returned bytes.
5. Then F11 §4 (which source signs, deduplication across backends, what the
   audit log records) and the exit condition.

The owner is at the machine for anything needing a card, a PIN or a click
([[D-094]]). Tell him what to do and what to look for.
