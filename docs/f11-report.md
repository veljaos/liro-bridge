# F11 — the phase report

**Date:** 2026-09-15
**Exit condition: met.** A Pošta card signed a PDF through SafeSign's PKCS#11
module rather than through CNG, on one PIN, first attempt, and the result is
accepted by the independent verifier of SPEC §16.4 and by Adobe Acrobat
([[D-280]]).

F11's own **Report:** line asks for six things. They are §§1–6 below; §7 is what
remains, and §8 is what this phase does not claim.

---

## 1. What `C_GetFunctionList` returned, and from where

Four modules, all found from the environment rather than from written-out
paths, and all four loaded and answered ([[D-271]]). On the home machine three
of them are present and two are at **different versions** from the machine
every earlier session used ([[D-272]]):

| Vendor | Path | Office machine | Home machine |
|---|---|---|---|
| A.E.T. Europe (SafeSign) | `%SystemRoot%\System32\aetpkss1.dll` | 3.9.24.1 | **3.9.32.1** |
| NetSeT (TrustEdgeID) | `%ProgramFiles%\TrustEdgeID\netsetpkcs11_x64.dll` | 1.1.3.3 | **1.1.3.2** |
| NetSeT (MUP RS) | `%ProgramFiles%\MUP RS\Celik\netsetpkcs11_x64.dll` | 1.1.0.0 | 1.1.0.0 |
| Nexus Personal | `%ProgramFiles(x86)%\Personal\bin64\personal64.dll` | 5.17.0 | **absent** |

**So discovery can assume neither a path nor a version**, which is the open
question [[D-271]] left and [[D-272]] answered as both. A measurement taken
through a module is a measurement of *that build*.

**A file is a module because it exports `C_GetFunctionList` and answers
`C_GetInfo`, never because its name looks promising.** The office machine
carries `SecurityTray\lib\pkcs11wrapper_64.dll` — "pkcs11" in the name, in a
directory a reasonable search would look in, and IAIK's Java JNI wrapper, which
*consumes* modules rather than being one. The home machine carries
`TrustEdgeID\netsetpkcs11_x86.dll`, which `LoadLibrary` refuses outright. Both
are `Failure`s in a list and neither is fatal (F11 §3).

**The index map is confirmed by the module rather than by counting.** SafeSign
thunks every export, so a pointer comparison says nothing — but the thunk table
is the declaration order at a 16-byte stride, so
`(export − C_Initialize's export) / 16` reproduces every index this project
uses: **18 of 18, including `C_SignInit`=42 and `C_Sign`=43, which nothing had
ever exercised on any module** ([[D-280]]'s pre-flight).

## 2. Whether `CGO_ENABLED=0` survived, and what settled it

**It survived. SPEC §1 is unamended and CI is unchanged.**

`syscall.LoadLibrary` + `GetProcAddress` + `SyscallN`, standard library only
([[D-271]]). What settled it is not an argument but the whole phase running
that way: four modules loaded, two cards read, and a qualified signature
produced, with no C compiler anywhere in the build. F11 §1's instruction to
answer this before building on it was followed and the answer held to the end.

`go vet`'s `unsafeptr` fires throughout, as [[D-080]] already disabled it
project-wide for.

## 3. Which mechanism each module wanted, and how the signature was verified

**`CKM_RSA_PKCS`, and it signs a DigestInfo this layer builds.**

| Module | Mechanisms | `CKM_RSA_PKCS` | Also offers |
|---|---|---|---|
| SafeSign (Pošta card) | 63 | yes | `CKM_SHA256_RSA_PKCS`, `CKM_SHA1_RSA_PKCS` |
| TrustEdgeID (MUP card) | 15 | yes | `CKM_SHA256_RSA_PKCS`, `CKM_SHA1_RSA_PKCS`, `CKM_MD5_RSA_PKCS` |
| MUP RS\Celik (MUP card) | 21 | yes | as above |

Every one of them offers SHA-1, which SPEC §18.8 forbids this program
producing. `digestInfoPrefix` carries exactly one entry and a test asserts it,
so this layer cannot produce a SHA-1 signature even if asked ([[D-279]]).

**The mechanism difference from CNG is the trap of the phase and is worth
stating once more.** `CKM_RSA_PKCS` pads and signs the bytes it is given and
adds no DigestInfo; Windows CNG builds one inside Windows from
`BCRYPT_PKCS1_PADDING_INFO`, so the other backend has no equivalent code. Hand
`CKM_RSA_PKCS` a bare digest, or use `CKM_SHA256_RSA_PKCS` and sign a hash of a
hash, and every layer reports success while the signature verifies against
nothing.

**How it was verified, in two independent steps:**

1. Off the card — the DigestInfo this package builds is byte-identical to the
   standard library's, proved by signing one RSA key two ways
   (`crypto.SHA256` over the digest, `crypto.Hash(0)` over our structure) and
   requiring the signatures to match, with a control showing a bare digest
   produces a different signature ([[D-279]] §4).
2. On the card — `internal/pades/verify`, the second from-scratch
   implementation [[D-044]] built so a shared bug cannot pass both ways:
   `ByteRangeDigestOK`, `SignatureOK`, `SigningCertificateOK` all true, no
   errors ([[D-280]]).

**Never by observing that bytes came back.** F11 §2.1's own rule.

## 4. What `Chain` gave, per module, and what it does to B-LT

**Empty, on both Serbian cards, for two different reasons — and the second is a
trap rather than a gap.**

| Card | On the token | `Chain()` |
|---|---|---|
| MUP, through both NetSeT builds | signer + its authentication twin, **no CA** | empty — nothing to match ([[D-271]]) |
| Pošta, through SafeSign | signer + the **root**; the issuer is the intermediate | empty — **a CA is present and does not match** ([[D-274]]) |

`issuersFor` matches only on an exact `RawSubject`/`RawIssuer` byte comparison.
On the Pošta card a looser rule — by name, by prefix, by "the CA the token
happens to carry" — would build a chain that is **wrong rather than short**:
nothing signed the signer with that root, a validator would reject the path as
a broken signature rather than a missing certificate, and [[D-046]] would
collect revocation evidence for a certificate that issued nothing and embed it
in a `/DSS` as long-term validation evidence.

**What it does to B-LT: it puts it out of reach on every card this project
has**, and this phase measured the last route closed:

- A chain of one certificate expects no revocation evidence ([[D-159]]), so
  `/DSS` is empty, so [[D-079]] writes no revision, so the document is B-T at
  best. **A missing chain is a missing signature level, not a missing field.**
- MUP: the chain is absent from the card, its OCSP responder refuses a TCP
  connection outright, and its CRL is 30 MB — over the embedding cap
  ([[D-076]]).
- Pošta: the chain is absent from the card, and **its AIA `caIssuers` is an
  LDAP URL**, which this program's HTTP fetcher cannot dial ([[D-281]]). So the
  obvious completion route is closed too.

SPEC §11.8's table — Pošta embeds 3 certificates in a signed document where MUP
embeds 1 — remains true, and this phase measured that **the card is not where
those three come from**.

The exit condition's document is **B-B**, and that is a choice of the run
rather than a property of the card: no TSA was configured, so none was
contacted. A timestamp is orthogonal to what the run established and can be
added later without a card ([[D-243]] made the same call for the same reason).

## 5. Whether the protected authentication path was available

**No, on every token measured, and the amendment therefore bites on the exit
condition's own path.**

| | Protected path | Measured |
|---|---|---|
| MUP token, both NetSeT builds | **no** | [[D-268]] |
| Pošta token, SafeSign | **no** | [[D-273]] |

[[D-268]] cost one of three attempts to establish the first: `C_Login` with a
NULL PIN returned `CKR_PIN_INCORRECT`, the module having handed it to the card
as an empty PIN, with no dialog of its own and no length check anywhere between
this program and the card. [[D-269]] is the owner's ruling and SPEC §6.5.1 the
amendment — eight clauses, each a requirement.

All eight are live on the path F11's exit condition travels. SPEC §6.5.1's
closing sentence, which said SafeSign's answer was not yet known, is amended
([[D-276]]).

**The PIN screen is a native Win32 dialog — the only window in this product
that is not HTML** ([[D-277]], SPEC §10 amended by [[D-278]]). The reason in one
line: clause 2 is a statement about this program's memory, and a WebView2 page
is not this program's memory. `Eval` returns a Go string, which cannot be
overwritten, and upstream of it the PIN would live in a DOM node, a script
engine's heap and an inter-process message in a process this program cannot
reach. It is also the familiar shape rather than the novel one, which is why it
has to say who is asking.

**`ulMinPinLen`/`ulMaxPinLen` are enforced in this layer, and the two cards
disagree** — MUP declares 4 and 8, Pošta 5 and 15, and both live in one
person's drawer. The numbers come from `C_GetTokenInfo` at the moment the
screen is built; SPEC §6.5.1's seventh clause now says so.

**Nothing retries.** There is no loop in `login` and no caller that calls it
twice, and a test counts the asks.

## 6. What is written but unverified

**Halcom: the module is verified and the signature is not, and the line is
here.**

- **Verified**, on the office machine: `personal64.dll` (Nexus Personal 5.17.0)
  loads, `C_GetFunctionList` answers, `C_Initialize` succeeds, and the slot,
  token and mechanism lists can be read ([[D-271]]).
- **Not verified**: that a Halcom card produces a signature this project can
  verify. **There is no Halcom card**, there never was one in this phase, and
  the home machine has no Nexus installed at all ([[D-272]]) — so the module
  that could be interrogated on one machine cannot even be loaded on the other.

An untested issuer claimed as supported is the defect; the honest form names
the line the verification stops at, which is the one above. F11 §6 requires
this in the report, in `README.md` and in the entry.

**Also written and unverified:**

- **The protected-authentication-path branch of `login`.** No module measured
  offers one, so `C_Login(NULL)` has never been executed. The branch is checked
  per token every time, because the flag is a property of a token through a
  module rather than of an issuer, and a reader with a pinpad answers
  differently.
- **`softhsm2` in CI.** F11 §6 asks whether it can cover this layer in CI.
  Not established — it was not installed and not tried. The soft-token path is
  no harder to reach than before and nothing in this phase requires a card in
  CI.
- **macOS and Linux.** `module_other.go` says what it says: the struct layouts
  are measured for Windows x64, where `CK_ULONG` is 4 bytes, and are **wrong**
  elsewhere rather than merely unavailable. F12 and F13.

## 7. What remains, and what is gated

**The crash, and it reaches nobody today.** `MUP RS\Celik\netsetpkcs11_x64.dll`
dies inside its own first `C_Initialize` about **once in a hundred calls** —
two terminations (`0xE06D7363`, a C++ throw, and `0xC0000417`, a CRT
fail-fast), and **no Go process can survive either**: `recover` catches neither
and a vectored handler does not help ([[D-272]]).

[[D-272]]'s table records four occurrences in roughly 450 runs. **A fifth
arrived during this phase's own closing check run**, with ten clean runs
immediately after, which is the rate holding rather than a new fact. It is
recorded here rather than in that entry because the entry is pushed and SPEC
§17 does not edit one somebody may have read. It is also the shape the fault
will keep taking: a suite that goes red once, passes on the re-run, and has
nothing to do with the change being tested.

What it kills today is `go test ./internal/keysource/pkcs11/`, **not the
program** — `go list -deps` of the agent does not contain this backend and
nothing outside the package imports it. The remedy is named and deferred:
`Modules` probes each candidate in a child process, so one that dies becomes a
`Failure` in a list, which is what F11 §3 asks for ([[D-275]]). It is deferred
because it introduces a hidden mode in a release binary, the surface [[D-222]]
and [[D-228]] spent two phases removing.

**The deferral is a check rather than a sentence.**
`cmd/liro-bridge/pkcs11reach_test.go` fails the moment anything in the agent's
dependency graph imports this backend, names the entry, and says that deleting
the test is part of building the remedy. It earned itself twice in one session:
once on the `PINRequest`→`PINPrompt` adapter, and once on the exit condition's
own harness.

**So F11 §4 is what remains, and it is the gated part.** Which source signs
(CNG stays the Windows default), one certificate one row across two backends
deduplicated on thumbprint, and what the audit log records about which backend
signed. None of it is built, all of it needs the import the gate refuses, and
the order is therefore: the out-of-process remedy first, then §4.

Two smaller things also open: [[D-281]]'s choice between a bundled trust store
and an LDAP client for Pošta chain completion, and the eight `pindialog.*`
catalogue keys, which have no production reader until §4 wires one ([[D-279]]).

## 8. What this phase does not claim

- **The agent has never signed through PKCS#11.** The exit condition was met by
  a harness, because the gate forbids the wiring. Two different claims and only
  one is true ([[D-280]]).
- **Adobe's amber verdict is about the trust anchor, not the signature.** It
  does not carry Pošta Srbije CA, cannot build a path to a root it recognises,
  and would show red if the bytes were wrong. It is what any program signing
  with this card produces in Adobe — SPEC §16.7 already documents the same
  shape for MUP.
- **One card, one reader, one machine.** Everything about the Pošta card is one
  card's; everything about SafeSign is version 3.9.32.1's.
- **The crash's trigger is unknown.** 350 runs across nine shapes moved nothing,
  and whether the card is even the variable is not established — the original
  card-out control was one run against a one-per-cent fault, which is the
  reasoning error [[D-272]] records at length.

---

## The exit checklist, item by item

**The cgo question**

- [x] Answered by measurement, on Windows, against a real module — and the
      whole phase ran that way
- [x] `CGO_ENABLED=0` survived: CI unchanged, SPEC §1 unamended
- [x] n/a — it did not fail
- [x] No third-party PKCS#11 binding adopted

**The client**

- [x] A new `keysource.Source` beside `windowscng` and `softtoken`
- [x] Nothing above `keysource` knows it exists — enforced by a test
- [x] The signing mechanism signs the digest rather than re-hashing it,
      verified two independent ways
- [x] No SHA-1, no MD5, whatever the module offers
- [x] `Chain` returns what the module has and never a guess

**Finding the module**

- [x] Discovery paths established by reading two machines, not by recall
- [x] A configured path is honoured, and is tried first even if absent
- [x] A protocol-supplied path is refused — nothing here reads a request
- [ ] **A module that fails to load degrades rather than crashes** — true for
      every failure mode except [[D-272]]'s, which cannot be survived in
      process. Remedy named and gated ([[D-275]]).

**Which source signs**

- [ ] CNG remains the Windows default — not reached; F11 §4, gated
- [ ] One certificate, one row — `List` deduplicates within a module and the
      thumbprint matches CNG's byte for byte, but collapsing across backends is
      §4's and is not built
- [ ] What the audit log records — not decided; §4

**The PIN**

- [x] Protected path used wherever supported — live, and taken by nothing
      measured
- [x] Raised rather than built: [[D-268]] → [[D-269]] → SPEC §6.5.1
- [x] Never logged, never in an error, never retained — and guarded by tests
      in two packages
- [x] Nothing retries a PIN, ever

**Testing**

- [ ] **Whether `softhsm2` can cover this in CI: not established.** §6
- [x] Halcom named as written-but-unverified, with the line — here, in
      [[D-271]], and in `README.md`
- [x] The soft-token path no harder to reach than before

**Deferred, and recorded as such**

- [x] MUP on macOS/Linux needs a direct PC/SC route (F11 §0.1)
- [x] `srb-id-pkcs11` recorded as reading, not a dependency
- [x] `-race` on `cmd/liro-bridge` still impossible here — no C compiler

**Exit condition**

- [x] **The Pošta card signed a PDF through SafeSign's PKCS#11 module rather
      than through CNG**, verified by the independent verifier and by Adobe
      ([[D-280]])
