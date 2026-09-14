# F11 handover — PKCS#11

**Written:** 2026-09-14, at the end of the first session of F11.
**State:** §1 answered and closed. **Nothing is built.** The phase is stopped at
§5 pending the owner's ruling.
**Master:** `9f31125`, working tree clean.

Read this before `docs/phases/F11.md`, because F11.md has already been
corrected once against what this session measured and this file says what the
corrections were for.

---

## 1. §1 is settled: `CGO_ENABLED=0` survives

**SPEC §1 stands unamended and CI does not change.** No `CGO_ENABLED=1`
anywhere, no C toolchain, and none of the cross-compilation cost for three
platforms that §1 said had to be priced before it was paid. That is measured,
against four real modules and a real card, not assumed — which is the thing a
future reader will most want to know.

The mechanism is `syscall.LoadLibrary` + `syscall.GetProcAddress` +
`syscall.SyscallN`, and the probe that established it imported **only the
standard library** — not even `golang.org/x/sys`. That matters: a successful
run is evidence about the toolchain rather than about a dependency. No
third-party PKCS#11 binding was adopted, or looked at, before the question was
answered.

The proof went all the way to a certificate rather than stopping at a function
pointer. Two certificates were read off the owner's MUP card through a
hand-marshalled packed `CK_ATTRIBUTE` template and parsed by `crypto/x509`:

```
CKA_LABEL "ВЕЉКО СТАНОЈЕВИЋ 011445479 Sign"
  serial    20F048A768F56F099E
  keyUsage  digitalSignature+contentCommitment
  SHA-1     7758D4D4B8973EA619B3225185EDE740B3D1ECCE
```

Four independent cross-checks against facts already in `decisions.md`: that
serial is D-209's stamp example, that thumbprint is D-209's and D-243's
`…B3D1ECCE`, the auth twin came back `…DA534AC6` which is D-149's, and the key
usages match SPEC §11.4's table for MUP exactly. So the read is right rather
than accidentally plausible — and **PKCS#11 and CNG produce the same
thumbprints**, which is what makes §4's deduplication work at all.

### What the cost actually is, since it is not cgo

- **`CK_ULONG` is 4 bytes** on Windows x64 (LLP64) and will be **8** on F13's
  Linux (LP64). This layer carries that difference deliberately rather than
  discovering it. Confirmed indirectly but soundly: `CK_INFO`'s
  `libraryDescription` parses cleanly at offset `34 + 4`; at `34 + 8` it would
  begin with four NUL bytes.
- **Structs are packed to one byte.** `CK_FUNCTION_LIST`'s function pointers
  start at offset **+2**, unaligned, immediately after the two-byte
  `CK_VERSION` — measured identically on all four modules. Go reads these
  happily on amd64; a platform that traps unaligned loads would need byte-wise
  reads.
- **`CK_ATTRIBUTE` is 16 bytes**: `type` at +0 (4), `pValue` at +4 (8),
  `ulValueLen` at +12 (4). Go's natural layout for the same three fields is 24,
  and Go cannot express `#pragma pack(1)`, so templates must be marshalled
  byte-wise.
- **The three wrong layouts return `CKR_OK` with a zero length.** This is §2.1's
  silent failure arriving one layer earlier than the signing mechanism: only
  16-byte packed gives a plausible `CKA_VALUE` (1910 bytes), and only it gets
  `CKR_OK` from `C_FindObjectsInit`. The other three are accepted and answer
  wrongly.
- **The `CK_FUNCTION_LIST` index map is normative** (PKCS#11 §3.6) and is *not*
  confirmable by comparing `list[3]` against the exported `C_GetFunctionList`:
  **SafeSign's export is a `jmp rel32` thunk** and does not match, where the
  other three do. Confirm behaviourally instead — `C_GetInfo` returning
  recognisable strings is the check that actually holds.
- `go vet`'s `unsafeptr` check fires throughout, as it already does for the COM
  interop this project hand-writes. D-080 disabled it project-wide for that
  reason and the same reasoning will apply here.

---

## 2. §5 is OPEN and blocking. Do not design around it.

**The MUP card does not advertise a protected authentication path.** Through
*both* NetSeT modules:

```
flags 0x10040d  RNG LOGIN_REQUIRED USER_PIN_INITIALIZED TOKEN_INITIALIZED SO_PIN_COUNT_LOW
CKF_PROTECTED_AUTHENTICATION_PATH (0x100) is NOT set
```

`LOGIN_REQUIRED` set, protected path absent. Read literally, `C_Login` on this
token takes the PIN as an argument — which means the PIN would exist in this
program's memory. That is an amendment to SPEC §6.5, the paragraph the
specification calls the most important in the document, and §5 says plainly it
is to be raised rather than built around. It has been raised. Nothing has been
built.

**The one measurement that would settle it is deliberately not taken.** Calling
`C_Login(session, CKU_USER, NULL, 0)` would show whether the module raises its
own dialog despite not advertising the flag — some modules do. The reason it
waits:

- PKCS#11 leaves the behaviour **undefined** when the flag is absent and `pPin`
  is `NULL_PTR`. Most implementations return `CKR_ARGUMENTS_BAD` without
  touching the card. Some return `CKR_PIN_INCORRECT`, which **consumes a PIN
  attempt**.
- Three consumed attempts block the card. For a national identity card,
  unblocking means a visit to a police station — which is the reason `docs/CLI.md`
  and F8 §4.4 already forbid anything here from retrying a PIN automatically.

The owner has authorised this measurement for the next session, on the
condition that he is at the machine when it happens so that he sees the
attempt counter himself if one is consumed. **Do not take it otherwise.**

**SafeSign's answer to §5 is not in yet.** It returns `CKR_TOKEN_NOT_RECOGNIZED`
for the MUP card, so the exit condition's own module has never been asked this
question. That needs the Pošta card, which is on the home machine.

---

## 3. What is installed here, and what that does to §3 and §4

Four paths, **three** middlewares, plus one file that is not a module at all.
Measured 2026-09-14 on the development machine:

| Path | Vendor | File ver | Built | `C_GetInfo` libraryDescription |
|---|---|---|---|---|
| `C:\Windows\System32\aetpkss1.dll` | A.E.T. Europe (SafeSign) | 3.9.24.1 | 2024-12-13 | Cryptographic Token Interface |
| `C:\Program Files\TrustEdgeID\netsetpkcs11_x64.dll` | NetSeT | 1.1.3.3 | 2024-12-12 | CardEdge PKCS#11 Library |
| `C:\Program Files\MUP RS\Celik\netsetpkcs11_x64.dll` | NetSeT | 1.1.0.0 | 2019-03-27 | CardEdge PKCS#11 Library |
| `C:\Program Files (x86)\Personal\bin64\personal64.dll` | Nexus | 5.17.0 | — | Personal NG PKCS 11 |
| `C:\Program Files\SecurityTray\lib\pkcs11wrapper_64.dll` | IAIK | 1.2.16 | — | **not a module** |

`pkcs11wrapper_64.dll` does not export `C_GetFunctionList`. It is IAIK's Java
JNI wrapper, which *consumes* PKCS#11 modules rather than being one. Discovery
must test for the entry point, not for a promising file name.

**NetSeT ships at two paths in two builds five years apart.** Not one file in
two places — which is the harder case, because whichever one a person's machine
prefers, the other is somebody else's machine. Whether the *home* machine has
either of these, and at which versions, is on the list in §6 for exactly that
reason: the version a person has is an accident of what they installed and
when, and that is a fact about every user rather than about one developer.

Also installed: `SafeSign IC MiniDriver 4.2.1.0`. SafeSign's PKCS#11 library is
therefore present on **both** machines, which was not assumed at the start of
the session and is why every question except the Pošta signature is better
answered here.

---

## 4. The three findings to carry into §2/§3/§4

1. **Both NetSeT builds see one card identically** — same slot id 0, same token
   serial `ID011445479`, same label `NetSeT's CardEdge Token`, same two
   certificate thumbprints. So deduplication on thumbprint will work, and it is
   needed: one card, two modules, plus CNG, is three sightings of one
   certificate. §4's "one certificate, one row" is a real problem on this
   machine today.
2. **A slot with a token present is not a card the module can use.** SafeSign
   answered `C_GetSlotList(tokenPresent=TRUE)` with one slot, and then
   `CKR_TOKEN_NOT_RECOGNIZED` to `C_GetTokenInfo`, `C_GetMechanismList` and
   `C_OpenSession` alike. §4 must read that as "not mine", not as an error, and
   §3's degradation vocabulary needs to accommodate it.
3. **Certificate order differs between the two NetSeT builds** — Sign-then-Auth
   from the 2024 build, Auth-then-Sign from the 2019 one. `List` cannot rely on
   the order objects come back in.

And one robustness fact that shapes how this layer must be written:

> **Nexus's `personal64.dll` crashes the host process** — access violation,
> reproduced twice — when handed a `CK_ATTRIBUTE` template of the wrong shape.
> Not an error code, not a hang: process death, taking the agent's windows and
> any batch in flight with it. This layer must get the layout right by
> construction and must never probe it at runtime.

Two mechanism facts for §2.1, recorded now because they are cheap to lose:

- Both NetSeT builds offer `CKM_RSA_PKCS` (0x0001) and `CKM_SHA256_RSA_PKCS`
  (0x0040). §2.1's distinction is live on this card: `CKM_RSA_PKCS` is the one
  that signs a pre-computed DigestInfo, which is what `SignDigest` has.
- Every module on this machine also offers mechanisms SPEC §18.8 forbids this
  program from ever producing — `MD5_RSA_PKCS`, `SHA1_RSA_PKCS`, `SHA_1`, and
  on Nexus also `MD5` and `SHA1_RSA_PKCS_PSS`. Offering them is not a defect in
  the module; using one would be a defect here.

---

## 5. Two mistakes of mine, recorded because both were nearly conclusions

- **A Go `const` block with an explicit first value and implicit continuations
  repeats rather than increments.** `iOpenSession = 12` followed by bare names
  made *every* index 12, so `C_FindObjectsInit` was really `C_OpenSession`. It
  produced `CKR_ARGUMENTS_BAD` from both NetSeT modules and crashed Nexus, and
  I was one step from recording that as a fact about the modules. Confirmed in
  isolation before believing it. Use `iota`, or write every value out.
- **A wrong lookup table reads like an answer.** The first `CKR` table had
  several values wrong — `0x10` labelled `CKR_DEVICE_ERROR` when it is
  `CKR_ATTRIBUTE_READ_ONLY` — so `0xE1` printed as `?` when it is
  `CKR_TOKEN_NOT_RECOGNIZED`, the single most informative return in the whole
  sweep. A table that is wrong is worse than none, because it does not look
  like a gap.

Both are the project's own recurring lesson (D-087, D-122, D-161, D-172,
D-219, D-247) pointed at the instrument rather than at the product: a harness
is code too, and a green-looking result from one nobody has checked is worth
what a green test against the wrong fixture is worth.

---

## 6. The home-machine list

The Pošta card is at home. SafeSign is installed on both machines, so only the
exit condition needs the trip. One sitting, one `.exe`, one command.

1. `reg export` the Explorer verb key
   (`HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign`)
   and the `Run` key (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`),
   **before anything runs**. Copies, not hashes (D-243).
2. Does `C:\Windows\System32\aetpkss1.dll` exist there, and at what version?
3. **Do `MUP RS\Celik` and `TrustEdgeID` exist there, and at which versions?**
   Two builds five years apart on one machine means discovery cannot assume a
   version; this establishes whether it can even assume a path.
4. **Pošta card in** — run the sweep.
5. **Pošta card out** — the same command, so that presence is a measured
   difference rather than an assumption.
6. **§5 for SafeSign**: does its token set
   `CKF_PROTECTED_AUTHENTICATION_PATH`? The sweep prints this and needs no PIN.
   This is the answer the exit condition actually turns on.
7. **Only if separately authorised:** one signature with the Pošta card, to see
   whether a PIN dialog appears and whose it is.

---

## 7. Machine state at handover

Snapshotted by copy before anything ran, per D-243 and D-153: `reg export` of
both registry keys, plus copies of `config.json`, the `audit` directory,
`pairings.json`, `secrets.*` and `update-state.json`.

Verified afterwards against those copies: every file **byte-identical**, the
audit chain still 20 entries, the Explorer verb export **byte-identical**, and
no agent process left running. The agent was never started and no test suite
was run, so D-266's `HKCU\…\Run` hazard was never triggered.

One honest wrinkle. The `Run` key's **export hash differed and its contents did
not**: `reg export` emitted two unrelated values in a different order. Six
names before and after, none added, none removed, no data changed, `LiroBridge`
still pointing at the repository-root build output. Checked value by value
rather than by trusting the text diff. Worth knowing for the next person: a
hash taken over a `reg export` can **over**-report, which is the mirror of
D-243's lesson that a hash under-reports — the values are the thing to compare,
not the file.

---

## 8. What the next session should do, in order

1. With the owner present: take the `C_Login(session, CKU_USER, NULL, 0)`
   measurement on the MUP card, and read the attempt counter before and after.
2. Take his ruling on §5 — protected path, or an amendment to SPEC §6.5, or
   neither and the phase narrows.
3. Only then start §2. Not before.

The probe this session used lived in a session scratchpad and is gone; §1 of
this document carries everything expensive about it, and rebuilding it from
those facts is twenty minutes.
