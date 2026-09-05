# Decisions

This file records what was decided, why, and what was tried and rejected.
Entries are numbered sequentially and never edited or deleted. A
superseded decision gets a new entry that references the old one.

---

## D-001 — Go as the implementation language

**Date:** 2026-08-31
**Phase:** F0

**Decision.** Liro Bridge is written in Go.

**Why.** The agent must ship as a single binary with no runtime dependency,
run as a background/tray process, and reach low-level OS facilities
(Windows CNG, PC/SC, DPAPI, Keychain, Secret Service) directly. Go compiles
to a static, dependency-free binary, cross-compiles to all three target
platforms from one toolchain, and has mature standard-library support for
crypto, ASN.1 and networking — all needed for the hand-written PDF/CMS
layer specified in SPEC §12.1.

**Rejected.**
- **.NET.** Requires a runtime (or a much larger self-contained publish),
  and cross-compiling a native macOS/Linux tray app from .NET is a worse
  fit than Go's native cross-compilation.
- **Rust.** Comparable technical fit, but a smaller pool of contributors
  familiar with it reduces the project's bus factor for an open-source
  tool that other people are expected to read, audit and extend.
- **Node.js (Electron or similar).** Packaging size and startup latency
  are poor for a small background agent, and shipping a JS runtime
  conflicts with the "no runtime dependencies" design centre (SPEC §1).

---

## D-002 — Errors are codes, never messages

**Date:** 2026-08-31
**Phase:** F0

**Decision.** Every error that crosses an API boundary is a stable
`SCREAMING_SNAKE_CASE` code plus optional structured `Details`. No
human-readable string — in any language, including English — crosses the
boundary. `internal/errs.Error.Error()` returns an English description,
but that method exists only for local logs; it is explicitly excluded from
JSON serialisation.

**Why.** The agent serves three interface languages (SPEC §9.1) and an
open set of third-party SDKs and callers it does not control. If the
agent's own language leaked into the response, every caller would have to
either display English to a Serbian user or maintain their own
brittle string-matching translation layer. Pushing translation to the
edge (SDK or UI) is the only place that actually knows the caller's
locale.

**Rejected.**
- **Code plus an English message field, for "debugging".** This is the
  trap named directly in F0 §4.1: a `Message` field looks harmless and
  gets added the first time someone wants a quick debug hint, and from
  then on some client, somewhere, displays it to a user. The serialisation
  test in `internal/errs` exists specifically to catch this being
  reintroduced.

---

## D-003 — The dependency rule is enforced by a checker we own

**Date:** 2026-08-31
**Phase:** F0

**Decision.** `scripts/checkdeps` is a small hand-written Go program that
parses `go list -json ./...` output and checks the seven rules in SPEC
§4.2 with plain string-prefix comparisons. It is not a third-party import
linter or a `golangci-lint` module.

**Why.** The rule set is five lines of specification (SPEC §4.2) and will
not grow much. A generic import-linter dependency is a whole tool with its
own config surface, versioning and potential breaking changes; the risk is
that it stops enforcing the rule silently (a config key renamed upstream,
a flag deprecated) and nobody notices until a violation ships. A checker
we own is small enough to read end to end in a minute and has a test that
proves it actually fires (F0 §6.3).

**Rejected.**
- **A `golangci-lint` "depguard" rule.** Would work, but depguard's
  configuration format has changed across major versions before, and a
  silently-ignored malformed depguard config is worse than an explicit
  failure from our own program.
- **`go/analysis`-based custom linter registered with `golangci-lint`.**
  More machinery than the problem needs; a standalone program invoked as
  its own CI step is simpler to read, run and reason about.

---

## D-004 — `log/slog` with hand-written rotation, no logging library

**Date:** 2026-08-31
**Phase:** F0

**Decision.** Logging uses the standard library `log/slog`. File rotation
(5 MB per file, 3 files retained) is implemented directly in
`internal/config` (`rotatingWriter`), about 90 lines including the
multi-destination fan-out handler.

**Why.** `log/slog` is the standard library's structured logger and covers
everything the agent needs: JSON output, levels, structured fields. Size
rotation is a small, well-understood piece of logic (open, track size,
rename-and-reopen on threshold) that does not justify a dependency; SPEC
§8.6 explicitly asks for the standard library to be preferred and for
every dependency's necessity to be recorded here.

**Rejected.**
- **`lumberjack` (or similar) for rotation.** Well-tested and small, but
  still an external dependency for something achievable in well under 100
  lines, for a project whose stated default is "prefer the standard
  library."
- **`zap` / `zerolog` instead of `slog`.** Faster in some benchmarks, but
  the agent's logging volume is trivial (batches of documents, not a
  high-throughput service), so the performance argument does not apply,
  and `slog` avoids a dependency entirely.

---

## D-005 — Licence: Apache License 2.0

**Date:** 2026-08-31
**Phase:** F0

**Decision.** Liro Bridge is licensed under the Apache License, Version
2.0.

**Why.** F0 §1.3 specifies Apache 2.0 as the default choice unless
instructed otherwise; no instruction to the contrary was given. Apache 2.0
is a permissive, OSI-approved licence with an explicit patent grant, which
suits a project that other companies (ERPs, accounting software vendors)
are expected to integrate against via the SDKs in later phases.

**Rejected.**
- **MIT.** Similarly permissive but has no explicit patent grant, which
  matters more for a project that touches cryptography and signing.
- **GPL/AGPL family.** Copyleft terms would discourage exactly the
  third-party integration (ERPs, ISVs) that SPEC §4.3 and §20 are built
  around.

---

## D-006 — Config field validation is independent per field

**Date:** 2026-08-31
**Phase:** F0

**Decision.** `internal/config.Load` validates `Locale`, `LogLevel`,
`PortRangeStart` and `PortRangeEnd` independently. Each invalid field is
replaced with its own default and logged; there is no cross-field check
(for example, no requirement that `PortRangeStart <= PortRangeEnd`).

**Why.** F0 §2.3/§2.4 specify exactly four validity conditions, each
scoped to a single field ("a locale of `sr`", "a port outside
1024–65535"). Adding a cross-field invariant is not asked for anywhere in
F0, and inventing one now would be exactly the kind of unspecified
requirement SPEC §0 says to avoid. The port range is not used for
anything until F7 (protocol/port discovery), so a temporarily
nonsensical-but-in-range pair is harmless in this phase.

**Rejected.**
- **Validating `Start <= End` and falling back to defaults on
  violation.** Plausible for a later phase once the port range is
  actually used to open a listener, but out of scope for F0, which does
  not implement the listener at all.

---

## D-007 — "3 files retained" counts the active file

**Date:** 2026-08-31
**Phase:** F0

**Decision.** `logFilesRetained = 3` means three files total on disk at
any time: `bridge.log` (active) plus `bridge.log.1` and `bridge.log.2`
(backups). Rotating drops `bridge.log.2`, shifts `.1` to `.2`, and renames
the active file to `.1`.

**Why.** F0 §3.2 says "5 MB per file, 3 files retained" without
specifying whether the active file counts toward the three. Reading it as
"three files exist on disk, full stop" is the simpler of the two
interpretations and bounds disk usage at a predictable 15 MB, which is
almost certainly the intent of a size-based retention policy.

**Rejected.**
- **Three backups plus the active file (four total).** Also a reasonable
  reading of the sentence, but it makes the retained size 20 MB for a
  policy whose stated numbers (5 MB, 3 files) suggest a round ~15 MB
  budget.

---

## D-008 — The CLI wires up config and logging before dispatching any flag

**Date:** 2026-08-31
**Phase:** F0

**Decision.** `cmd/liro-bridge` loads configuration, sets up logging, and
writes one `slog.Info` startup line *before* parsing `--version`/`--help`,
for every invocation. The example transcript in F0 §7.2 shows only stdout,
which is unaffected — the startup log line goes to the JSON file (and to
stderr only under `LIRO_DEBUG=1`), so it does not appear in the example
output.

**Why.** F0 §7.2 lists "load config, set up logging, log one startup line"
as part of what `main.go` wires up, without tying it to a specific flag.
Running it unconditionally means `--version`, `--help` and a bare
invocation all exercise the same startup path, which is what later phases
will build on — there is only one code path to keep correct, not a
version-only fast path plus a separate "real" path.

**Rejected.**
- **Only wiring config/logging on bare invocation (no flags).** Would
  match the example transcript just as well, but would mean `--version`
  — the command most likely to be run by scripts and installers checking
  the binary — never proves the config/logging path works, and CI would
  not exercise it via `--version` checks either.

---

## D-009 — `.golangci.yml` targets the v2 config schema, unverified locally

**Date:** 2026-08-31
**Phase:** F0

**Decision.** `.golangci.yml` is written in the `golangci-lint` v2
configuration format (top-level `version: "2"`, `linters.enable`,
separate `formatters.enable`).

**Why.** `golangci-lint` is not installed in this environment, so the
config could not be run and verified locally. v2 is the current major
version at the time of writing and is what CI's `golangci-lint-action`
will fetch by default. This is flagged here rather than silently hoping
it works, per SPEC §0's instruction not to silently work around an
unverifiable constraint.

**Rejected.**
- **Guessing the v1 schema instead.** No more verifiable than v2 without
  the binary, and v1 is the older format.
- **Leaving `.golangci.yml` out of F0.** Explicitly required by F0 §6.1
  and the exit checklist; not an option.

**Follow-up.** The first real CI run is the actual verification of this
file. If it fails, fixing `.golangci.yml`'s schema is a mechanical,
same-phase fix, not a design decision — update this entry's status rather
than opening a new one for a schema typo.

---

## D-010 — `Redact` collapses short strings entirely

**Date:** 2026-08-31
**Phase:** F0

**Decision.** `config.Redact` returns a single "…" for any input of 8
characters or fewer, instead of showing up to 4 leading and 4 trailing
characters.

**Why.** F0 §3.3 specifies "at most 4 leading and 4 trailing." For a
string of 8 characters or fewer, showing 4 and 4 reveals the entire input
— zero characters are actually hidden, which defeats the purpose of a
redaction helper for exactly the short secrets (short PINs, short tokens)
where redaction matters most.

**Rejected.**
- **Always showing min(4, len/2) on each side.** Still leaks meaningful
  fragments of short secrets (e.g. a 6-character value would show 3+3,
  hiding nothing of substance). Collapsing to a fixed placeholder below
  the threshold is simpler and strictly more conservative.

---

## D-011 — `checkdeps` hardcodes the module path

**Date:** 2026-08-31
**Phase:** F0

**Decision.** `scripts/checkdeps` hardcodes
`github.com/veljaos/liro-bridge` as a constant rather than reading it from
`go.mod` at runtime.

**Why.** F0 §1.1 fixes the module path for the life of the project. Reading
it from `go.mod` would add a small amount of parsing code to remove one
constant that is not expected to change; the simpler option was chosen per
SPEC §0.

**Rejected.**
- **Parsing `go.mod` for the module path.** Marginally more "correct" in
  the abstract, but there is no scenario in this project's plan where the
  module path changes, so it is complexity with no corresponding benefit.

---

## D-012 — `CGO_ENABLED=0` is per-step in CI, not a job-wide default

**Date:** 2026-08-31
**Phase:** F0

**Decision.** The CI workflow does not set `CGO_ENABLED` at the job level.
The `test` step (which runs `go test ./... -race -count=1`) sets
`CGO_ENABLED=1`; the three cross-compilation build steps each set
`CGO_ENABLED=0` explicitly.

**Why.** F0 §10 says to keep `CGO_ENABLED=0` "for now" so nothing in the
agent's own code accidentally depends on a C toolchain before phase 11
(PKCS#11). F0 §8 separately requires `go test ./... -race -count=1` in
CI. The race detector's instrumentation itself requires cgo regardless of
whether the code under test uses it — `go test -race` fails outright with
`CGO_ENABLED=0` (confirmed locally: this machine has no C compiler and
`-race` fails immediately with "requires cgo"). These are not actually in
conflict: one is about our source code, the other is about a test tool's
build requirements. Scoping `CGO_ENABLED` per step keeps both rules true
at once instead of silently dropping `-race` to work around the trap.

**Rejected.**
- **Dropping `-race` from CI.** Would resolve the conflict by deleting
  half of it — exactly the "silently soften a constraint" move SPEC §0
  warns against. `-race` is explicitly required by F0 §8.
- **Setting `CGO_ENABLED=1` for the whole job.** Would make the
  cross-compilation build steps silently capable of linking C code,
  undermining the point of the F0 §10 trap, which is to catch a cgo
  dependency creeping in before phase 11.

---

## D-013 — Qualification is decided by the Trusted List; OIDs and qcStatements are display evidence only

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/trust/classify.qualify` decides `Qualification`
by one test only: does the certificate's `RawIssuer` byte-match the
`RawSubject` of a Trusted List service whose `ServiceTypeIdentifier` ends
in `/CA/QC` and whose status, resolved at the reference time via
`Service.StatusAt`, ends in `/granted`. The eIDAS policy OID
(`0.4.0.194112.1.2`), the issuer-specific policy OIDs, and the
`qcStatements` extension are decoded (`internal/trust/classify/oids.go`)
and used only for `OnQSCD` and future display purposes — never to decide
`Qualification`.

**Why.** SPEC §11.1 states this as the primary rule directly. SPEC §11.3
is the concrete reason OID-only heuristics are untrustworthy: Halcom's
*published Certificate Policy document* states the OID
`1.3.6.1.4.1.5939.11.2.6`, but the certificates Halcom actually issues
carry `1.3.6.1.4.1.5939.10.1.6` — a different branch. A classifier keyed
on the documented OID would silently misclassify every real Halcom
certificate. The Trusted List additionally carries revocation-relevant
status *over time* (`ServiceHistoryInstance`), which no certificate
extension can express.

**Rejected.**
- **OID-only heuristics** (trust any certificate carrying the eIDAS
  policy OID). Rejected for the reason above, and because it cannot
  represent a service being withdrawn.
- **Full chain building and signature verification against the TSL
  service certificate.** F1 §5.4 explicitly scopes this phase to
  issuer-name matching only; AIA fetching and chain completion are
  deferred to F3, where the completed chain is needed in the output
  document anyway.

---

## D-014 — Hardware presence is read from `SCardListReaders`/status, never from certificate enumeration

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/platform`'s `SmartCardService.Readers` /
`AnyCardPresent`, backed by `SCardListReadersW` and
`SCardGetStatusChangeW`, is the only source of hardware presence.
`internal/keysource/windowscng.Enumerate` never reports presence; it
only reports whether a certificate's key is *capable* of living on
hardware (`OnHardware`, from the CNG provider name). `classify.Classify`
combines the two: `Usable` is false with `NotUsableReason ==
CardNotPresent` exactly when `OnHardware && !hardwarePresent`.

**Why.** SPEC §11.10, measured directly during this phase by running the
real enumeration and status-change calls together on this machine's own
Windows certificate store: a real Halcom-issued certificate remained
fully enumerable — subject, extensions, provider name, all present and
readable — while the smart card reader attached to the machine reported
`SCARD_STATE_EMPTY` (no card inserted). If presence were inferred from
enumeration, the agent would offer to sign with that certificate, the
user would click Approve, and the failure would only surface after a PIN
prompt.

**Rejected.**
- **Attempting a signature to test presence.** Would trigger a PIN
  prompt as a side effect — unacceptable for a liveness check, and
  explicitly out of scope for a phase that must never call a signing
  API (F1 §7).
- **Inferring presence from the CNG provider alone.** The provider name
  (`OnHardware`) only says the key *would* live on a card if one were
  present; it is static metadata on the certificate, not a live status
  query, and does not change when the card is removed.

---

## D-015 — Purpose is decided by `contentCommitment` alone

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/trust/classify.purposeFromKeyUsage` returns
`PurposeSigning` whenever `x509.KeyUsageContentCommitment` is set,
checked before anything else. `digitalSignature` and `keyEncipherment`
together mean `PurposeAuthentication`; neither bit present means
`PurposeUnknown`. `digitalSignature` is never required for
`PurposeSigning`.

**Why.** SPEC §11.4, and reproduced directly in this phase's synthetic
fixture and independently on a real card: a genuine Halcom signing
certificate enumerated on the development machine during F1 shows
KeyUsage `Non Repudiation` only (confirmed with
`openssl x509 -text` — no `Digital Signature` bit at all), while its
paired authentication certificate on the same physical card shows
`Digital Signature, Key Encipherment` and no `Non Repudiation`. A filter
requiring `digitalSignature` for "signing" would reject every Halcom
signing certificate and misclassify the authentication certificate as
usable for signing (SPEC §18.4 makes this a hard prohibition, not a
style preference).

**Rejected.**
- **`digitalSignature` as a required or alternative signal for
  `PurposeSigning`.** Directly contradicted by measurement, and
  forbidden outright by SPEC §18.4.

---

## D-016 — Exclusive C14N is implemented in-house, in `internal/trust/tsl/c14n`

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/trust/tsl/c14n` is a from-scratch implementation
of Exclusive XML Canonicalization 1.0 without comments
(`http://www.w3.org/2001/10/xml-exc-c14n#`): a small DOM built with
`encoding/xml`'s `RawToken` (which preserves literal prefixes instead of
resolving them, unlike `Token`), plus a serializer that tracks a
namespace-rendering context independent of the source document's actual
declaration sites. No XML-DSig or XML-Security third-party package is
imported anywhere in this project.

**Why.** F1 §4.5 requires it explicitly, and explains why: this code
decides what certificates the agent will ever call qualified, so it must
be small enough to read end to end, and an imported library's bug here
would be invisible. The implementation is validated against the two
worked examples in the W3C Exclusive XML Canonicalization specification
itself (simple and complex re-enveloping, §2.1/§2.2 of
https://www.w3.org/TR/xml-exc-c14n/), not invented vectors, and — more
importantly — against the real, government-published Trusted List: this
implementation's canonical form of that document's `SignedInfo`, run
through RSA-SHA512 verification against the embedded signer certificate,
verifies correctly (`TestVerifyBundledSeedSucceeds`).

**Rejected.**
- **An existing Go XML-DSig or XML-Security package.** None was
  imported, importing one is exactly what F1 §4.5 forbids, and the two
  candidates known at the time of writing (`github.com/russellhaering/goxmldsig`
  and similar) pull in their own C14N implementations as an unauditable
  unit rather than the ~250 lines this project can read in one sitting.
- **Reusing `encoding/xml`'s `Token()` (namespace-resolving) instead of
  `RawToken()`.** `Token()` throws away the literal prefix a document
  used, which exclusive C14N's output format requires reproducing
  verbatim (e.g. the real Trusted List binds the XML-DSig namespace to
  `ns2` at the document root but to `ds` locally on the `Signature`
  element itself — both must round-trip correctly).

---

## D-017 — TSL signer certificates are pinned by SHA-256 fingerprint, not chain-validated

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/trust/tsl.PinnedSigners` is a fixed map of two
SHA-256 fingerprints. `Verify` rejects any signer certificate whose
fingerprint is not in that map, with no attempt to build or validate a
certificate chain for it.

**Why.** F1 §4.6 states both Ministry signer certificates are
self-signed, which this phase confirmed directly: the certificate
embedded in the real Trusted List's `KeyInfo` has SHA-256
`cfd20b5a6696621266171c7cd3969bce23bbb2910ddf73bbf54e235d26b7e4b1`,
matches "Serbian Trusted List Signer 1" exactly as specified in F1 §4.6,
and its Subject and Issuer fields are identical (self-signed). There is
no chain to validate for a self-signed certificate — a chain-validation
code path here would either be dead code or, worse, would have to
special-case self-signed roots into "trusted," which is a strictly
weaker and more complex check than a direct fingerprint comparison.

**Rejected.**
- **Trusting any self-signed certificate whose Subject matches an
  expected distinguished name string.** DN string comparison is exactly
  the kind of parser-dependent, format-fragile check SPEC §11.6 warns
  about elsewhere in this same phase; a byte-for-byte fingerprint has no
  such ambiguity.

---

## D-018 — A real, verified Trusted List is bundled as the embedded seed

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/trust/tsl/seed/TSL-RS.xml` is not a fixture —
it is the actual Republic of Serbia Trusted List, fetched from
`https://www.mit.gov.rs/TrustedList/TSL-RS.xml` (the Ministry of
Information and Telecommunications' own publication endpoint) on
2026-08-31, with the server's leading UTF-8 BOM stripped. Its SHA-256,
`3f5744843bcfaba0698c6b5f7121ee9f133151249808c9de0ebb8e351d9ddb8b`,
matches the digest specified in F1 §4.7 exactly, confirming this is the
same publication the phase document was written against. `DefaultURL` in
`internal/trust/tsl/store.go` is the same address, used as the default
(configurable) refresh source.

**Why.** F1 §4.7's stated purpose — a truthful, byte-verifiable answer
on a machine with no network — is best satisfied by the genuine article
rather than a synthetic stand-in invented for testing. Using the real
list also let this phase's signature-verification work
(`internal/trust/tsl/c14n`, `Verify`) be validated against real
XAdES-BES output from the start, rather than against a hand-built
document that might not exercise the same edge cases (this list's
signature is RSA-SHA512 over SHA-512 digests, and its `ds:Signature`
element declares its own local `ds` prefix distinct from the root
document's `ns2` — details that would be easy to miss if the test
document were invented rather than real).

**Note on F1 §4.3's provider count.** F1 §4.3 states "11 providers."
Direct measurement of this exact, hash-verified file
(`TestParseSeedProviderCount`) found 10
`<TrustServiceProvider>` elements. Every other measurement in F1 §4.3
and §4.4 against this file matches exactly (sequence 36, issued
2026-05-20T01:00:00Z, 31 `ServiceHistoryInstance` elements, all seven
named service certificates present with status `granted`). This is
recorded here rather than silently changed to match: SPEC §0 says
measured facts are ground truth, but this is a case where a second,
independently reproducible measurement of the identical byte-verified
artifact disagrees with the phase document's summary on one count. The
test asserts the measured value (10), not the documented one, and says
why in its own comment.

**Rejected.**
- **Building a synthetic seed list "as if" it were real, to sidestep
  needing network access during development.** Network access was
  available (confirmed by fetching the real list directly); using it
  produces a strictly more valuable and more truthful seed than
  inventing one, per F1 §4.7's own stated reasoning for having a seed at
  all.

---

## D-019 — Display names are built from `givenName` + `surname`, never parsed from CN

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/trust/classify.parseSubject` builds
`Subject.DisplayName` only from the `givenName` (2.5.4.42) and `surname`
(2.5.4.4) attributes, falling back to raw `CommonName` only when one or
both are absent. No regular expression or string-splitting is ever
applied to CN.

**Why.** SPEC §11.7 gives three real CN values that make any CN-parsing
regex unreliable by construction: `"ВЕЉКО СТАНОЈЕВИЋ 011445479 Sign"`
(Cyrillic, trailing CA number, literal English word " Sign" appended),
`"Zoran Milovanović 246275"` (Latin name plus trailing number, no
suffix word), and `"Redžvel Mešković 200094362"` (same shape, different
issuer). No single pattern separates the name from the trailing content
across all three without issuer-specific special-casing, and a fourth
issuer would break it again. `givenName`/`surname` are separate,
unambiguous attributes on every certificate examined.

**Rejected.**
- **A per-issuer regex keyed off the certificate's policy OID.** Would
  work for the three known shapes but adds a maintenance burden that
  scales with the number of issuers, for a problem `givenName`/`surname`
  already solves generically.

---

## D-020 — `classify` defines its own `Reason` type instead of importing `internal/errs`

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/trust/classify.Reason` is a local `string` type
with constants (`ReasonExpired = "CERT_EXPIRED"`, etc.) whose *values*
match the corresponding `internal/errs.Code` constants exactly, but
`internal/trust/classify` does not import `internal/errs`. A caller that
needs an `errs.Code` converts with a plain type conversion,
`errs.Code(reason)`.

**Why.** F1 §5.1's own example code types `Info.NotUsableReason` as
`errs.Code` directly. But SPEC §4.2 rule 3 is unconditional: "`internal/trust`
MUST NOT import anything from `internal/`" — no carve-out for `errs`, and
`scripts/checkdeps`'s own test (`TestTrustImportingAnyInternalIsForbidden`)
confirms this is enforced literally, not just for "business logic"
packages. Importing `internal/errs` from `internal/trust/classify` would
make `go run ./scripts/checkdeps` fail. Per SPEC §0 ("if a rule makes
your implementation awkward, raise it in decisions.md rather than
bending it"), the dependency rule is treated as the authoritative
constraint and F1's example code is adapted to fit it, not the reverse —
weakening `checkdeps` to special-case one package would erode the exact
guarantee ("trust evaluation is pure and independently testable") the
rule exists to protect.

**Rejected.**
- **Adding an exception to `checkdeps` for `internal/errs`.** Considered
  and rejected: `internal/errs` has no dependencies of its own, so the
  purity argument for excluding it is real, but SPEC §4.2 rule 3 does not
  say "no dependency *except* errs," and inventing the exception now
  would be indistinguishable, to a future reader, from someone quietly
  weakening the rule the first time it was inconvenient.
- **Having `classify` import `internal/errs` and accepting the
  `checkdeps` failure as a known, documented violation.** Rejected
  outright — a failing dependency check is not an acceptable phase exit
  state under any framing.

---

## D-021 — Test certificates are synthetic, built to match SPEC §11's documented structure

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `testdata/certs/*.der` are generated by
`scripts/gencerts/main.go`, not extracted from a real card. They
reproduce SPEC §11's measured structural facts byte-for-byte where it
matters — multi-valued `serialNumber` RDNs (`PNORS-...` and `CA:RS-...`
in the same RDN, deliberately ordered PNORS-first so a naive
"first-match" parser would get the wrong value), Halcom's
`contentCommitment`-only KeyUsage, the Halcom-in-DN vs
MUP/Pošta-in-SAN email placement, and two certificates sharing one
identical `Subject`. Fabricated names, national ID numbers and email
addresses are used throughout. The three CA `Issuer` fields
(`mup_signing.der`, `posta_signing.der`, `halcom_signing.der`,
`halcom_auth.der`, `expired_signing.der`) are copied byte-for-byte from
the genuine CA certificates in the bundled real Trusted List (D-018), so
`classify`'s "qualified against the bundled TSL" test exercises real
trust-anchor bytes.

**Why.** This implementation environment has no access to a real
Serbian e-ID card or to genuine issued certificates, which by
construction belong to a real, identifiable person — exactly the
personal data SPEC §6.7/§11.6 says must be handled carefully, not
casually committed to a public repository's test fixtures "because it
was convenient." SPEC §0's instruction to pick the simplest option
applies once the "use real fixtures" option is unavailable: reproducing
the documented structure synthetically is the only way to test the
traps F1 §5.6 requires tested (multi-valued RDNs, the KeyUsage
exception, personal-data scrubbing) without possessing real personal
data at all.

**Rejected.**
- **Asking the user to supply real certificate exports from their own
  card.** Would work, but ties a repeatable, CI-runnable test suite to
  one person's hardware and personal data indefinitely, and the exact
  structural traps needed (e.g. an authentication certificate sharing a
  signing certificate's Subject) require two matched certificates from
  the same issuance, which is not guaranteed to be available on demand.
- **Fabricating the CA `Issuer` bytes too, alongside the end-entity
  certificate.** Would make the "qualified" test pass against a
  fabricated Trusted List entry instead of the real one, silently
  weakening what that test actually proves.

---

## D-022 — `.golangci.yml`'s v2 schema needed one fix: `goimports` moved out of `linters.enable`

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `goimports` is listed only under `formatters.enable`, not
under `linters.enable`, in `.golangci.yml`.

**Why.** D-009 (F0) flagged `.golangci.yml` as unverified because
`golangci-lint` was not installed in that environment, and explicitly
deferred verification to "the first real CI run." `golangci-lint` v2.13.2
was installed and run directly in this phase (`go install
github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`), and it
failed immediately with `can't load config: goimports is a formatter` —
in the v2 schema, `goimports` is a formatter only and is rejected if
listed under `linters.enable`. This is exactly the mechanical,
same-phase fix D-009 anticipated, not a new design decision.

**Rejected.** Nothing else was considered: this is a one-line schema
correction with a single valid fix.

---

## D-023 — Default-hidden certificates are exactly "unknown purpose and not qualified"

**Date:** 2026-08-31
**Phase:** F1

**Decision.** `internal/cli.CertRow.Hidden` returns true only when
`Purpose == PurposeUnknown && Qualification == QualificationNotQualified`.
An authentication-purpose certificate is always shown by default, even
when `Usable == false`.

**Why.** F1 §6.1's flag description paraphrases the rule loosely
("not qualified and not a signing certificate"), but F1 §6.1's own
worked example output shows an authentication certificate — not
qualified, not usable, `Purpose == authentication` — appearing in the
*default* view (no `--all`) as row `[2]`. Reading "not a signing
certificate" as "hide everything that isn't `PurposeSigning`" would hide
that exact example row, contradicting the transcript that states the
rule. F1 §5.4 gives the precise version directly: the Windows-internal
certificates noted in F1 §3.5 (self-signed, GUID subject, software KSP)
"fall out here as `QualificationNotQualified` with `PurposeUnknown`, and
are therefore hidden from the default list" — naming both conditions
together. This phase's own real-hardware run (§ manual verification)
found exactly this case on the development machine: two GUID-subject,
self-signed, software-backed certificates already present in the
Windows store, correctly hidden by default and correctly shown with
`--all`.

**Rejected.**
- **Hiding every certificate with `Purpose != PurposeSigning`.**
  Directly contradicted by F1 §6.1's own example transcript, which shows
  an authentication certificate in the default view.

---

## D-024 — SPEC §4.2 rule 3 amended to permit `internal/trust` importing `internal/errs`; [[D-020]] superseded

**Date:** 2026-09-01
**Phase:** F1/F2 cleanup

**Decision.** SPEC §4.2 rule 3 now reads: "`internal/trust` MUST NOT
import anything from `internal/` except `internal/errs`." `scripts/checkdeps`
carries exactly this one exception (`isUnder(d, "internal/errs")` is
skipped before the general `internal/trust` check), with a test proving
the exception is narrow (`TestTrustImportingAnyOtherInternalPackageIsForbidden`:
`internal/config`, `internal/keysource`, `internal/platform` and
`internal/pades` are all still forbidden) and a test proving the
exception itself (`TestTrustImportingErrsIsAllowed`).
`internal/trust/classify.Info.NotUsableReason` is now typed `errs.Code`
directly; the local `Reason` string type and its four constants
(`ReasonNone`, `ReasonNotUsable`, `ReasonExpired`, `ReasonCardNotPresent`)
introduced by [[D-020]] are deleted, along with the corresponding
switches in `internal/cli/render.go` (`reasonLabel`) and every test that
referenced them, updated to use `errs.CodeCertNotUsable`,
`errs.CodeCertExpired` and `errs.CodeCardNotPresent` directly. No mapping
function ever existed to convert `Reason` to `errs.Code` at a call site —
the only "mapping" was the parallel constant values themselves, which is
what made the workaround removable with no dead code left behind beyond
the type and its constants.

**Why.** [[D-020]] recorded, correctly at the time, that SPEC §4.2 rule 3
was unconditional and that F1 §5.1's example code (which typed
`NotUsableReason` as `errs.Code` directly) had to be adapted to fit the
rule rather than the reverse. Re-reading the rule against SPEC §7 exposes
the actual cost of that choice: `internal/errs` is deliberately a
dependency-free leaf — it imports nothing beyond `fmt` — carrying only
the `Code` vocabulary that SPEC §7 designates as the *entire* cross-boundary
error representation. A local, parallel `Reason` type whose values are
required to mirror `errs.Code`'s string values by convention (enforced by
nothing but a doc comment) is strictly worse than importing the real
type: it can drift silently (a new `errs.Code` added without a
corresponding `Reason` constant compiles fine and simply produces the
wrong string), and it forces every downstream consumer (`internal/cli`)
to either duplicate the same parallel-constant trick or convert at the
boundary — which nothing in the codebase actually did, meaning the
"conversion" SPEC's own F1 text anticipated was aspirational, not real.
Importing `internal/errs` does not weaken "trust evaluation is pure and
independently testable" (the property rule 3 exists to protect): `errs`
has no imports of its own, no I/O, and no behaviour beyond string
constants and an `Error` struct whose methods are excluded from
serialisation. Rule 3's purity guarantee is about `internal/trust` never
reaching into stateful or I/O-bearing packages (`internal/config`,
`internal/keysource`, `internal/platform`, `internal/pades`), not about
avoiding every internal identifier on principle.

**Rejected.**
- **Leaving [[D-020]]'s workaround in place.** Considered, since it does
  work and is already tested. Rejected because it is exactly the
  "invented requirement" SPEC §0 warns against in the other direction: an
  unconditional rule that was never load-bearing for the property it
  claims to protect, kept unconditional out of inertia, at the cost of a
  duplicate vocabulary that can silently drift from the real one.
- **Widening the exception to all of `internal/trust`'s siblings, or to
  any dependency-free leaf package.** Out of scope here: only the
  specific, measured conflict between rule 3 and SPEC §7's error model is
  being corrected. A general "dependency-free packages are exempt" rule
  would need its own justification and is not what was asked for.
- **Keeping `Reason` as a type alias for `errs.Code`
  (`type Reason = errs.Code`) instead of deleting it outright.** Would
  avoid touching call sites, but leaves a redundant name for the same
  type with no purpose once the import restriction that motivated it is
  gone — SPEC §0's "choose the simplest option" favours deleting it.

---

## D-025 — The PIN never enters this process; the OS smart card KSP shows its own dialog

**Date:** 2026-09-01
**Phase:** F2

**Decision.** `internal/keysource/windowscng` never sets
`NCRYPT_PIN_PROPERTY`, never passes `NCRYPT_SILENT_FLAG`, and no
struct or function signature anywhere in `internal/keysource`,
`internal/keysource/softtoken` or `internal/signing` carries a field
named after a PIN. Each of those three packages carries a test
(`pin_test.go`) that walks its own package's AST and fails if a struct
field or function parameter matching `(?i)\bpin\b` is ever added.

**Why.** SPEC §6.5 states plainly that the card caches the PIN in its
own state, independent of which process is talking to it — the PIN is
therefore not an access-control boundary between applications, only
the consent screen is. F2 §2.3 draws the direct consequence: since the
PIN buys the agent nothing as a security boundary, there is no reason
for it to ever be inside this process, where it could leak from logs,
crash dumps or memory. `NCRYPT_WINDOW_HANDLE_PROPERTY` is still set (to
0 in this phase — F5 supplies the real window handle), so the OS
dialog is at least parented once a window exists; that is the only
concession this code makes toward the PIN prompt's UX.

**Rejected.**
- **Collecting the PIN in our own UI for a nicer, branded prompt.**
  Rejected outright by F2 §2.3/§8: it would put a card PIN inside our
  process for no functional gain, since the card's own cached-PIN
  behaviour (SPEC §6.5) means our process being the one to collect it
  buys no additional security.
- **A grep-based text check instead of an AST walk** for the "no PIN
  field" test. Rejected because it cannot tell a real field from this
  package's own doc comments explaining *why* there is no PIN
  field — comments that necessarily use the word "PIN" — which would
  make the check permanently unusable without constant false positives.

---

## D-026 — `fCallerFree` is honoured exactly; a cached key handle is never freed

**Date:** 2026-09-01
**Phase:** F2

**Decision.** `internal/keysource/windowscng`'s session
(`session_core.go`) stores the `fCallerFree` flag `CryptAcquireCertificatePrivateKey`
returns and calls `NCryptFreeObject` in `Close` if and only if it was
true. `TestCloseHonoursCallerFreeFalse` is the failing test: it asserts
`freeKey` is never called when `fCallerFree` was false.

**Why.** F2 §2.1 states the consequence of getting this wrong
explicitly: when `fCallerFree` is false, Windows caches the handle
across every process using the same key, and freeing it "corrupts the
cache for every other process." That is not a memory leak in this
process's own accounting — the handle is owned and reused by Windows —
so `Close` correctly does nothing in that case, by design, not by
omission.

**Rejected.**
- **Always calling `NCryptFreeObject` in `Close` for simplicity.**
  This is precisely the mistake F2 §2.1 warns about: a signing session
  from a second process (or a second `Open` call in the same process)
  against the same key would then hold a handle Windows believes is
  still valid but has actually been freed, corrupting the cache.

---

## D-027 — Batch signing is strictly sequential, never parallel

**Date:** 2026-09-01
**Phase:** F2

**Decision.** `internal/signing.Sign` iterates a batch's items in a
plain `for` loop against one `*Session`, with no goroutines. A
`Session` (and the `keysource.Session` it wraps) is documented as not
safe for concurrent use, matching SPEC §8.5.

**Why.** F2 §4.2 states the reasoning directly: the smart card is a
single serial device, so a driver receiving concurrent requests only
queues them internally anyway — parallelism at this layer buys no
throughput — and concurrent access to smart card APIs is a known
source of driver-level failures. Sequential signing is therefore not a
simplification made for convenience; it is the actually-correct
design for this specific kind of device.

**Rejected.**
- **Signing items in a worker pool for speed.** Would not be faster
  (the card serialises the actual work regardless) and risks
  driver-level corruption or crashes that a single-threaded caller
  never exercises — exactly the failure mode F2 §4.2 names.

---

## D-028 — The `ALWAYS_AUTHENTICATE` detection threshold is 2000ms

**Date:** 2026-09-01
**Phase:** F2

**Decision.** `internal/signing.alwaysAuthenticateThreshold` is
`2000 * time.Millisecond`, used exactly as F2 §5.5 specifies: the
median of signatures 2 and 3 (or signature 2 alone, provisionally, if
there is no third) is compared against it to decide
`PINPolicyPerSignature` vs `PINPolicyPerBatch`.

**Why.** F2 §5.4/§5.5 give the measurement this threshold is built
around: a real e-ID card's subsequent-signature time is ≈413ms, with
under 2ms of variation across 90 operations. F2 §5.5's own reasoning is
quoted directly in `timing.go`'s comment: a PIN dialog cannot be shown,
filled and dismissed by a human in under two seconds, so 2000ms sits
roughly 5× above normal card latency and far below any plausible human
interaction — neither a slow card nor a fast typist lands in the wrong
bucket. `TestDetectPINPolicyTable` in `internal/signing/timing_test.go`
reproduces F2 §7.2's exact table, including the "sig2 over the
threshold, sig3 fast — the median decides, one slow signature does not
flip the result" row.

**Rejected.** Nothing else was considered — F2 §5.5 specifies both the
threshold and its reasoning directly; this decision exists to satisfy
F2 §8's instruction to record it, not because an alternative was
weighed.

---

## D-029 — Session lifetime: 90s idle, 30 minutes maximum, 120s approval window

**Date:** 2026-09-01
**Phase:** F2

**Decision.** `internal/signing.IdleTimeout` (90s), `MaxLifetime` (30
minutes) and `ApprovalWindow` (120s) are implemented exactly as F2
§4.1 specifies, each independently checked in `Session.checkLive`: idle
timeout resets on every successful `SignDigest` call, maximum lifetime
is measured from `Open` regardless of activity, and the approval
window only applies before the first signature has happened at all.
Exceeding any of them closes the session (freeing the underlying key
handle per D-026) and returns `ErrSessionExpired`.

**Why.** F2 §4.1 states each number is a specified value, not a
suggestion, with its own reasoning: 90s idle is long enough for a
phone call mid-batch and short enough that a walked-away-from machine
does not hold an authenticated card open; 30 minutes is an absolute
ceiling that a 1000-item batch at ~0.41s/signature (well under 7
minutes) never approaches; 120s is how long an already-approved batch
can plausibly take to actually start signing before something is
wrong. `TestMaxLifetimeClosesSessionEvenUnderContinuousActivity` is the
test proving the second number is a real ceiling, not just an
idle-timeout restatement: it keeps the session continuously active
(never idle) and still expects expiry once the absolute limit passes.

**Note on interpretation.** F2 §4.1 says only "if signing has not
begun in two minutes something is wrong" for the approval window,
without stating the consequence. This phase treats it the same as
idle/max-lifetime expiry — the session is closed and the next
operation must open a fresh one — for symmetry with the other two
limits and because F2 has no consent screen yet (F5) to attach a
softer, UI-level warning to. See D-036 for what "approval" means in a
phase with no consent screen.

**Rejected.**
- **Treating the approval-window overrun as a warning only, not a
  session-closing condition.** Plausible, but F2 §4.1 groups all three
  numbers under one lifetime-limit umbrella with no textual signal that
  this one is softer than the other two; closing is the simpler,
  more conservative reading (SPEC §0).

---

## D-030 — Timing constants are always measured at runtime, never hard-coded

**Date:** 2026-09-01
**Phase:** F2

**Decision.** No code path in `internal/signing` contains the literals
4900 or 413 (or their millisecond/second equivalents) as a timing
value. `TimingReport.FirstSignature` and `MedianSubsequent` are always
populated from `time.Duration`s actually measured around real
`SignDigest` calls (`buildTimingReport` in `timing.go`), and
`EstimatedTotal`'s ETA formula (F2 §5.6) takes `first` and `median` as
parameters supplied by the caller from that same measured data —
returning `ok == false` (the indeterminate "Preparing card…" state)
whenever `first <= 0`, i.e. before any real measurement exists yet.

**Why.** F2 §5.6 states the reasoning directly: Pošta is migrating to
RSA-4096, which will be slower, so a hard-coded estimate would
silently become a lie the day that migration lands. The two numbers
quoted in SPEC §12.9/F2 §5.4 (≈4.9s first signature, ≈0.41s
subsequent) are observations recorded in the document to explain *why*
the 2000ms `ALWAYS_AUTHENTICATE` threshold (D-028) is set where it is —
they are not meant to become constants in the code, and this codebase
does not treat them as such anywhere.

**Rejected.**
- **A `defaultFirstSignatureEstimate` constant to show *something*
  before the first signature completes.** Rejected: F2 §5.6 explicitly
  asks for an indeterminate state instead ("Preparing card…"), and any
  placeholder number would be exactly the lie the "never hard-code"
  rule exists to prevent, however clearly it were labelled as a guess.

---

## D-031 — The soft token is excluded by build tag; `IsTestKey` propagates to `classify.Info` and the CLI

**Date:** 2026-09-01
**Phase:** F2

**Decision.** Every file in `internal/keysource/softtoken` carries
`//go:build softtoken`. `keysource.Certificate.IsTestKey` is set `true`
by that package alone; `classify.Info` gained the same field
(additive; `Classify`'s signature is unchanged, so none of F1's
`classify_test.go`/`classify_real_test.go` call sites needed touching —
`internal/cli.Gather` sets `info.IsTestKey = c.IsTestKey` for a
soft-token row the same way it already overwrites `info.Thumbprint`
with the enumeration-supplied value). CI (`.github/workflows/ci.yml`)
builds the binary once without the tag and once with it, and inspects
each with `go tool nm`, asserting the softtoken package's symbols are
respectively absent and present — proving the exclusion the same way a
missing/present grep hit would, but immune to a compiler dead-code
pass silently making a string-based check pass for the wrong reason.

**Why.** SPEC §16.6 states both requirements as hard, non-negotiable:
the soft token must be impossible to enable in a shipped binary, and
every signature it produces must be visibly marked as a test signature
everywhere downstream. The two-directional CI check (absent without
the tag, present with it) is the "write the check so it fails if the
build-tag exclusion regresses" discipline this project has already
applied once (F0's `checkdeps` prefix-mismatch story) — checking only
the "absent" direction would not catch the tag itself silently
breaking (e.g. a typo turning `//go:build softtoken` into a plain
comment), which would make the "absent" check pass for the wrong
reason forever.

**Rejected.**
- **Changing `Classify`'s signature to accept `isTestKey bool`
  directly.** Would also work, but touches roughly twenty existing F1
  test call sites for a value that is easy to set as a plain field
  assignment after the call — exactly the pattern F1's own `report.go`
  already uses for `Thumbprint`. SPEC §0's "choose the simplest option"
  favours the additive field over the invasive signature change.
- **A single build/grep CI step instead of two.** Would satisfy F2
  §7.3's literal ask (absence without the tag) but not the general
  "write a test that actually fails" discipline this project holds
  itself to elsewhere — a broken tag and a working exclusion look
  identical from the "absent" side alone.

---

## D-032 — PKCS#12 encoding uses `software.sslmate.com/src/go-pkcs12`

**Date:** 2026-09-01
**Phase:** F2

**Decision.** Both `scripts/gentestkeys` (encoding) and
`internal/keysource/softtoken` (decoding) depend on
`software.sslmate.com/src/go-pkcs12` (BSD-3-Clause), pulling in
`golang.org/x/crypto` transitively.

**Why.** F2 §3.2/§3.3 explicitly require a PKCS#12 file, by name and
by the exact environment variable names a caller sets
(`LIRO_SOFTTOKEN_P12`, `LIRO_SOFTTOKEN_PASSWORD`) — this is not a
detail to substitute away even though the standard library has no
PKCS#12 support at all. `golang.org/x/crypto/pkcs12`, the obvious
first candidate, is explicitly "frozen" (its own package doc says so)
and decode-only — it cannot produce the file `gentestkeys` needs to
write. `software.sslmate.com/src/go-pkcs12` is a maintained fork of
that exact package adding `Encode`, is BSD-3-Clause (compatible with
this project's Apache 2.0 licence, D-005), and is used only by
test-only, build-tag-gated code (`softtoken`) and a developer tool
(`gentestkeys`) that is never part of a release binary — neither
touches the code path SPEC §12.1 requires to be hand-written (the PDF
and CMS layers), so this does not weaken that rule.

**Rejected.**
- **`golang.org/x/crypto/pkcs12`.** Cannot encode, only decode — would
  leave `gentestkeys` with no way to produce the `.p12` file F2 §3.3
  requires.
- **Hand-writing a PKCS#12 encoder/decoder.** SPEC §12.1's "write it
  yourself" rule is scoped specifically to the PDF and CMS layers that
  ship in the release binary, where an imported library's bug would be
  invisible in exactly the place this project cannot afford one. PKCS#12
  here is test-only scaffolding excluded from every release build by
  the `softtoken` tag; writing a parser for a legacy, rarely-touched
  format to avoid one well-known dependency is exactly the kind of
  unrequested scope SPEC §0 warns against.

---

## D-033 — `CertFindCertificateInStore` lookup failures always map to `CERT_NOT_FOUND`

**Date:** 2026-09-01
**Phase:** F2

**Decision.** In `internal/keysource/windowscng/conn_windows.go`,
`findAndAcquire` maps *every* `CertFindCertificateInStore` failure to
`errs.CodeCertNotFound`, rather than passing the raw status through
`mapStatus`'s general F2 §2.4 table.

**Why.** Found by the exit-condition check itself (F2 §6.1): the first
end-to-end run of `sign-digest` against the soft token failed with
`SIGN_FAILED` instead of succeeding, because the soft token's
thumbprint (correctly) does not exist in the real Windows "MY" store,
`CertFindCertificateInStore` fails with `CRYPT_E_NOT_FOUND`
(`0x80092004`) — a status F2 §2.4's table does not list, since that
table is about signing-*operation* failures (`NCryptSignHash`,
`CryptAcquireCertificatePrivateKey`), not lookup failures — and
`mapStatus`'s catch-all default (correctly, for that table's actual
scope) turned the unrecognised code into `SIGN_FAILED`. That silently
broke `cmd/liro-bridge`'s CNG-then-soft-token fallback (D-034's
sibling logic in `main.go`), which only falls back on
`CERT_NOT_FOUND`. This is exactly the class of bug SPEC §16's
"prove it against real verification, not just unit tests" discipline
exists to catch: every unit test in this phase passed while this bug
was present, because no unit test exercised the real
`CertFindCertificateInStore` call path — only the actual OpenSSL
round-trip against a real Windows store did.

**Rejected.**
- **Leaving the lookup failure on the general `mapStatus` table.**
  Directly caused the bug above; a "not found" condition from the
  *lookup* step is unambiguous regardless of the specific underlying
  status code, unlike a signing-operation failure where the specific
  code genuinely matters (wrong PIN vs. blocked vs. cancelled).

---

## D-034 — `certs --json` gains `pem` and `isTestKey`, and lists the soft token when configured

**Date:** 2026-09-01
**Phase:** F2

**Decision.** `internal/cli.Deps` gained an `ExtraCertificates` field
(nil unless a build wires it up); `Gather` classifies whatever it
returns exactly like a hardware certificate except `onHardware` is
always `false`, and copies `IsTestKey` onto the resulting row's
`classify.Info`. `CertRow` and the `--json` `jsonCert` shape both
gained the certificate's raw DER, PEM-encoded on output
(`certPEM` in `render_json.go`). `cmd/liro-bridge` wires
`ExtraCertificates` to the soft token's `List` only in a binary built
with the `softtoken` tag (`softtoken_enabled.go`/`softtoken_disabled.go`,
the same conditional-compilation split F1 already uses for
`windowscng.Enumerate` on non-Windows platforms).

**Why.** F2 §6 requires `certs --json` to expose the PEM certificate
("if F1 did not already provide it" — it did not), and F2's own exit
condition (§6.1) reads that PEM to build the OpenSSL verification
command. Making the soft token discoverable through the same `certs`
surface a real card uses — rather than inventing a second,
soft-token-specific listing command — means the exact same recipe in
the README (D-035) proves the exit condition against both hardware and
the soft token, and gives `IsTestKey` (SPEC §16.6) a real, exercised
path into `classify.Info` and a JSON field, rather than an untested
field that only theoretically "propagates."

**Rejected.**
- **A separate `sign-digest --list-softtoken` or similar command.**
  Would also satisfy the letter of F2 §7's tests but forks the
  certificate-listing surface in two, and the README's OpenSSL recipe
  (F2 §6.1) already centres on `certs --json` — duplicating it for one
  backend serves no one.
- **Leaving `IsTestKey` on `classify.Info` unset by any real call
  site**, satisfying only "the field exists." Rejected as hollow: SPEC
  §16.6 asks for the mark to be visible, not merely representable.

---

## D-035 — The README's OpenSSL recipe selects the certificate by thumbprint, from `.certificates[]`

**Date:** 2026-09-01
**Phase:** F2

**Decision.** The README's F2 §6.1 recipe reads
`.certificates[] | select(.thumbprint == $tp) | .pem`, not the literal
`.[0].pem` shown in F2 §6.1 itself. Every other command in the recipe
is unchanged.

**Why.** F1 already shipped `certs --json`'s top-level shape as
`{"readers": [...], "certificates": [...], "trustedList": {...}}`
(`internal/cli/render_json.go`, predating this phase) — not a bare
top-level array — so `.[0]` is not valid jq against this project's
actual output; F2 §6.1's `.[0].pem` is this document's own
illustrative shorthand, not a literal contract on the JSON's shape.
Re-shaping `certs --json`'s top level now, purely to make one example
command copy-pasteable, would be a gratuitous breaking change to an
already-shipped, already-tested F1 API for zero functional benefit —
exactly the kind of unrequested change SPEC §0 warns against making to
satisfy something adjacent to, but not actually required by, the
current phase. Selecting by thumbprint rather than by position `[0]`
is also strictly more correct on a machine with more than one
certificate installed (this phase's own manual verification run had
five: two Windows-internal self-signed certificates, two real Halcom
certificates, and the soft token's), where `.[0]` is not necessarily
the certificate `sign-digest` was just asked to use.

**Rejected.**
- **Changing `certs --json`'s top-level shape to a bare array,** to
  make F2 §6.1's example copy-pasteable verbatim. Rejected: this is F1
  API surface, out of F2's scope to change, and the SDKs/consumers F1
  already anticipated (SPEC §20) would be broken for a cosmetic gain.
- **Keeping `.[0].pem` literally and accepting it usually works.**
  Rejected: "usually" is not good enough for the one command sequence
  this whole phase's exit condition depends on, and this project's own
  manual verification run demonstrated `.[0]` picks the wrong
  certificate whenever more than one is installed.

---

## D-036 — The approval-to-first-signature window is measured from `Session.Open`

**Date:** 2026-09-01
**Phase:** F2

**Decision.** `internal/signing.Session`'s `ApprovalWindow` (D-029) is
measured from the moment the session was opened (`openedAt`), not from
a separate "user clicked Approve" timestamp — because no such event
exists yet in this phase.

**Why.** F2 explicitly excludes the consent screen (F5) and the whole
UI layer; "the user has approved" (F2 §4.1) has no concrete referent
in a phase whose only caller is a human directly invoking
`sign-digest` from a terminal. Treating the CLI invocation itself —
which is what causes `Open` to be called — as the approval event is
the simplest reading available (SPEC §0) until F5 introduces an actual
consent screen with its own timestamp, at which point that event
should replace `openedAt` as this window's start.

**Rejected.**
- **Deferring the approval window entirely to F5**, implementing only
  idle timeout and maximum lifetime in this phase. Rejected: F2 §4.1
  and its exit checklist list all three numbers as this phase's
  responsibility, with no phasing note suggesting the third is
  optional here.

---

## D-037 — `--repeat > 1` ignores `--out` and reports timing only

**Date:** 2026-09-01
**Phase:** F2

**Decision.** When `sign-digest --repeat N` is given with `N > 1`,
`RunSignDigest` never writes signature bytes anywhere (`--out` is
accepted but has no effect) and instead prints the batch's
`TimingReport` — first signature, median subsequent, PIN policy, total
— to stderr.

**Why.** F2 §6 says `--repeat N` exists "to exercise batching and
produce a timing report," and does not describe what should happen to
N signature values once produced. Materially, there is nothing useful
to do with them: RSA-PKCS#1v1.5 (F2 §2.2/§3.2) is deterministic for a
fixed key, digest and padding, so signing the same digest N times
produces N byte-identical signatures — there is no "which one" to
write to a single `--out` path, and writing N files for one `--out`
flag would invent a naming scheme F2 never specifies. Reporting timing
only is the simplest option that satisfies F2 §6's stated purpose for
the flag without inventing unrequested output shapes.

**Rejected.**
- **Writing the last (or first) of the N signatures to `--out`.**
  Would silently do something with a flag whose value, per the
  determinism argument above, is indistinguishable from any of the
  other N-1 — implying a meaningful choice where none exists.
- **Writing N numbered files.** Not asked for anywhere in F2, and adds
  a naming convention this phase has no reason to invent.

---

## D-038 — F3's three real fixtures are not in this repository; synthetic ones drive the parser and writer, `testdata/pdfs/local/` mirrors [[D-021]] for the real ones

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `testdata/pdfs/` contains no PDF files. `internal/pades/pdf`'s
parser and incremental-update tests are built against **synthetic**
fixtures constructed in Go (`internal/pades/pdf/fixtures_test.go`):
`buildClassicFixture` (a one-page document using a classic xref table)
and `buildStreamFixture` (the same document using a cross-reference
stream plus one object stream), each with every byte offset computed at
build time rather than hand-typed. `testdata/pdfs/local/` is a new,
git-ignored directory (`.gitignore`, mirroring `testdata/certs/local/`)
with a README asking for the three real signed PDFs (`mup.pdf`,
`posta.pdf`, `halcom.pdf`); tests that need them are written to skip
themselves, with a clear message, when a given file is absent.

**Why.** SPEC §12 and all of F3 were written against three real signed
PDFs, one from MUP, Pošta Srbije and Halcom — the source of every
measured fact in F3 §2.1's xref-mechanism table, the CMS sizes in §4.3,
the BER timestamp tokens in §12.5, and more. Those files were never
committed to this repository, for the same reason [[D-021]] gives for
certificates: a real Serbian qualified signature embeds the signer's
name, national ID number and email address in the signer certificate,
which is personal data SPEC §6.7/§11.6 forbids committing to a public
repository. This implementation environment has no access to them
either. [[D-021]] already established the exact pattern this decision
reuses without modification: synthetic, committed fixtures prove the
*shape* of the code; a git-ignored `local/` directory with a README
lets a developer with real material opt real-fixture tests back in, and
those tests skip cleanly when the directory is empty. Building the
synthetic PDFs in Go rather than as static committed files (unlike
`testdata/certs/`, which are static `.der` files written once by
`scripts/gencerts`) was chosen because a PDF fixture's value here is
purely structural (exercising the two cross-reference mechanisms) and
carries no personal data even hypothetically, so there is no reason to
freeze it as a binary blob instead of readable, byte-exact-by-construction
Go source that documents its own offsets.

**Consequence, stated plainly rather than glossed over.** Several F3
exit-condition items cannot be verified in this environment and are
reported as pending, not as passing on faith: parsing all three real
fixtures, the already-signed-document test against a genuine prior
signature (F3 §10.2 — this phase's single most important test — is
provable only structurally, via `bytes.HasPrefix` and byte-range
preservation, until a real prior signature exists to actually verify
against), parsing a real BER-encoded timestamp token, and the
independent verifier's or any manual validator's acceptance of real
three-CA output. `testdata/pdfs/local/README.md` states this
consequence in full for whoever supplies the real files later.

**Rejected.**
- **Fabricating "real-looking" signed PDFs**, inventing plausible names,
  certificates and signatures to stand in for the genuine article.
  Rejected outright: SPEC §0 forbids inventing requirements, and a
  fabricated document claiming to be "a real MUP-signed PDF" would be
  actively misleading to read later, unlike an honestly-synthetic
  fixture that documents itself as such. It would also not actually
  close the gap — SPEC §12's facts were measured from genuine CA output,
  and a fabrication embodies only this project's own (possibly wrong)
  understanding of that output, which is exactly the shared-assumption
  blind spot [[D-021]]'s rationale already identifies for certificates.
- **Blocking F3 entirely until real fixtures are supplied.** Rejected:
  the parser, incremental-update writer, and (per F3's own build order)
  everything through CMS, TSA and DSS can be built and unit-tested
  against synthetic fixtures and the soft token (F2), exactly as F2's
  own exit condition anticipated ("F2's soft token exists so that F3
  through F6 can be developed without a card"). Stopping the whole phase
  over three specific files would waste that groundwork.
- **Skipping the local-fixture directory and README entirely**, leaving
  the gap undocumented. Rejected: a future reader (or this project's own
  CI) needs to know *why* certain tests skip, not just that they do.

---

## D-039 — Both cross-reference mechanisms are implemented from the first commit, with recursion-depth and decompression-size bounds added during fuzzing

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/pdf.Parse` (`document.go`, `xref.go`)
supports classic `xref` tables and cross-reference streams (with object
streams) unconditionally from the start — there was never a
classic-table-only version of this parser. `TestParseClassicFixture` and
`TestParseStreamFixture` (`parser_test.go`) exercise both directly;
`TestObjectStreamExtractionMatchesDirectObject` proves the compressed
path resolves identically to a direct object. Running `FuzzParse` (12.6M
executions over two minutes) found and fixed three crashes, all in the
cross-reference-stream/object-stream path: an unvalidated xref-entry
byte offset passed straight to a slice index (`parseIndirectAt` now
range-checks before setting `p.pos`), a parser position that could go
negative from attacker-controlled arithmetic and defeat the existing
`eof()` guard (`eof()` now rejects negative positions, not only
too-large ones), and an object stream's attacker-controlled `/N` used
directly as a slice-capacity hint (now range-checked against the decoded
stream's own length before use). `flateDecode` also gained a
`maxDecodedStreamSize` cap (`io.LimitReader`, same 512 MB ceiling as
`MaxInputSize`) and `applyPredictor` gained bounds on `/Columns`,
`/Colors` and `/BitsPerComponent`, closing a decompression-bomb and an
integer-overflow-into-huge-allocation path that fuzzing would very
likely have found next. `parseArray`/`parseDictOrStream` gained a
500-level nesting cap against stack-overflow from a
`[[[[[...` -shaped input. None of this was reactive cleanup after
shipping — every fix landed before the parser's tests were reported
done, per F3 §2.5's fuzzing requirement and SPEC §16.5.

**Why.** F3 §2.1 states plainly that two of the three real fixtures use
cross-reference streams and object streams, so a table-only parser
"opens one document in three" — not a refinement to add later. [[D-038]]
records that this project has no access to the three real fixtures
themselves, so the specific counts in F3 §2.1's measurement table (3
classic sections for Halcom; 4 streams + 4 object streams for MUP; 4
streams + 1 object stream for Pošta) are recorded here as the
specification's own evidence, not independently re-measured — this
project's synthetic fixtures exist only to prove both *mechanisms* work,
not to reproduce those exact revision counts. The fuzzing crashes found
are exactly the class of bug F3 §16.5 anticipates for "a parser that
accepts foreign input": every one of them was reachable only through
binary, attacker-shaped xref-stream or object-stream fields (byte
offsets, `/N`, predictor parameters) that a classic-table-only code path
has no equivalent of, which is further evidence for building both
mechanisms together rather than bolting the stream path on afterwards —
the two share the bulk of their attack surface (arbitrary binary
integers driving allocation and indexing) precisely because they share
implementation, and testing them together is what surfaced these bugs
before any downstream phase code was built on top of them.

**Rejected.**
- **Building the classic-table path first and adding streams later, as
  a separate pass.** This is the exact ordering F3 §2.1 explicitly rules
  out ("Both mechanisms are required from the start; neither is a later
  refinement"), and the fuzzing evidence above is a concrete illustration
  of why: the bugs fuzzing found live specifically in the stream/object-
  stream code, which a "classic first" ordering would have shipped
  untested for however long "later" took.
- **Deferring the fuzz-found bounds-checking fixes to a follow-up pass**,
  reporting the parser as done and filing the crashes as future work.
  Rejected: SPEC §16.5 makes "no panic, no unbounded allocation, no
  infinite loop" a requirement of this phase's own exit condition, not
  an aspiration: a parser that panics on attacker input is not a
  finished parser, regardless of how its happy-path tests look.

---

## D-040 — `go-pkcs12` (D-032) now also backs the TSA client's TLS client-certificate authentication

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/tsa.LoadPKCS12ClientCert` decodes a PKCS#12
file with `software.sslmate.com/src/go-pkcs12` — the same dependency
D-032 introduced for the soft token and `scripts/gentestkeys` — and
builds a `tls.Certificate` from it for the TSA client's second
authentication mode.

**Why.** F3 §6.2 requires implementing both of Pošta's test TSA
endpoints: HTTP Basic (the easy case) and a TLS client certificate
supplied as a PFX with password `1234`, explicitly because "production
TSAs require client certificates, so exercising only the easy endpoint
leaves the harder path untested." A PFX is a PKCS#12 file — exactly the
format `go-pkcs12` already decodes. D-032's justification for the
dependency (no PKCS#12 support in the standard library;
`golang.org/x/crypto/pkcs12` is frozen and decode-only anyway;
BSD-3-Clause is compatible with Apache 2.0) applies identically here.
D-032 scoped the dependency to "test-only, build-tag-gated code
(softtoken) and a developer tool (gentestkeys)... neither touches the
[hand-written] code path"; this decision records, rather than silently
expanding, that the scope is now wider: `internal/pades/tsa` ships in
release builds. That is still consistent with SPEC §12.1's "write it
yourself" rule, which is scoped specifically to the PDF and CMS layers
where an invisible bug in an imported library is the actual risk being
guarded against — PKCS#12 decoding of a client authentication credential
is a different kind of code with a different risk profile (a decoding
bug here fails loudly, as a connection that cannot authenticate, not as
an invalid signature that silently looks valid).

**Rejected.**
- **Hand-writing a second PKCS#12 decoder for this one use, to keep
  `go-pkcs12` "test-only."** Rejected as pure duplication: the exact
  same format, decoded by the exact same trusted dependency, would now
  exist twice in the codebase for no benefit — SPEC §0's "choose the
  simplest option" favours reusing what D-032 already justified.
- **Requiring the TLS client certificate as a pre-parsed `tls.Certificate`
  from the caller, keeping `internal/pades/tsa` PKCS#12-free.** Would
  push the same dependency (or a hand-rolled decoder) onto every
  caller — `cmd/liro-bridge` at minimum — for no actual reduction in
  what ships in the release binary; the dependency is used either way.

---

## D-041 — `golang.org/x/crypto/ocsp` builds and parses OCSP requests/responses; CRLs use the standard library

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/dss` depends directly on
`golang.org/x/crypto/ocsp` (promoted from the indirect dependency D-032
already pulled in via `go-pkcs12`) for `ocsp.CreateRequest` and
`ocsp.ParseResponseForCert`. CRL parsing uses the standard library's
`crypto/x509.ParseRevocationList` (Go 1.19+) directly — no dependency
needed there at all.

**Why.** F3 §7 (DSS/B-LT) needs to build RFC 6960 OCSP requests and
parse OCSP responses to embed as revocation evidence. This is
meaningfully different from the case SPEC §12.1 forbids third-party
help for: `/ByteRange` and the CMS SignerInfo structure are where a
library bug produces a signature that *looks valid and is not* — the
entire reason that code must be small enough to read end to end.
`/DSS` content is the opposite shape of risk: it is *evidence attached
after* a signature already exists and already verifies on its own; a
bug in how it is built can make a document wrongly report B-T instead
of B-LT, but it cannot make an invalid signature appear valid, which is
the property SPEC §12.1 protects. `golang.org/x/crypto/ocsp` is
maintained by the Go team as a companion to the standard library
(unlike a general-purpose third-party PDF or CMS library), was already
present as an indirect dependency, and hand-rolling RFC 6960's request/
response ASN.1 would be exactly the "complexity with no corresponding
benefit" SPEC §0 warns against for a well-defined, non-signature-critical
protocol with no Serbian-CA-specific quirks measured anywhere in SPEC
§11/§12 the way certificate and CMS handling has.

**Rejected.**
- **Hand-writing OCSP request/response ASN.1**, matching the CMS
  layer's own discipline. Rejected: unlike CMS, nothing about OCSP in
  this project touches the property SPEC §12.1 exists to protect, so
  the cost (a second RFC 6960 implementation, complete with its own
  bug surface) is not offset by a corresponding integrity benefit —
  DSS content that fails to embed correctly degrades the level
  honestly (F3 §7.3), it does not silently corrupt a signature.
- **A third-party CRL library.** Unnecessary: `crypto/x509.ParseRevocationList`
  already covers exactly what this project needs (validating that
  fetched bytes are a parseable CRL before embedding them), so adding a
  dependency for it would violate SPEC §8.6's "prefer the standard
  library" outright.

---

## D-042 — /Contents reserves 32768 bytes; /ByteRange fields are fixed at 10 characters, space-padded on rewrite

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/pdf.DefaultReservedBytes` is 32768;
`BuildPlaceholder` writes it as 65536 hex characters. `byteRangeFieldWidth`
is 10 characters per number; `writeByteRangeValues` rewrites the real
`[0 b c d]` values right-aligned within that fixed width using `%*d`
(space padding), never changing the array's total textual length.

**Why.** F3 §4.3/§4.4 specify both numbers directly, with their own
reasoning: real CMS blobs measured against the specification's
reference fixtures were 11 917, 10 052 and 14 039 bytes (signer
certificate, up to two chain certificates, a ~7 KB signature timestamp
token) — 32 KB leaves the largest more than 2× headroom at a file-size
cost this project's own measurements confirm is trivial (this phase's
golden file, with no chain and no timestamp at all, is 9 357 bytes
total). 10 digits supports files up to 9.9 GB, far above `MaxInputSize`
(512 MB), so the length-neutral rewrite in `writeByteRangeValues` never
needs a wider field than was reserved — verified directly by
`TestPlaceholderByteRangeCoversWholeFileExceptContents` and the byte-exact
`TestGoldenFile`. Space padding, not zero padding, on the *final*
values is F3 §4.4's own instruction ("some validators dislike leading
zeros in the final values"); the *placeholder* content (before the real
offsets are known) uses zeros instead, matching F3 §4.2's own example
(`/ByteRange [0000000000 0000000000 0000000000 0000000000]`) — the two
are visually similar but serve different moments in the same field.

**Rejected.** Nothing else was considered for either number — F3 §4.3/
§4.4 specify both directly, with their own worked reasoning; this entry
records them and the evidence, per F3 §11's explicit instruction, not a
weighed alternative.

---

## D-043 — Encrypted PDFs are rejected with `CodePDFEncrypted`; decryption is never implemented

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/pdf.Parse` returns `errs.CodePDFEncrypted`
the moment the merged trailer contains an `/Encrypt` key, before any
further parsing of the document's content. No PDF decryption code
exists anywhere in this project.

**Why.** F3 §2.3 states this directly: none of this phase's fixtures
are encrypted, and "signing an encrypted document is a separate feature
with its own risks" that F3 does not ask for. Checking the trailer is
sufficient and correct — `/Encrypt`'s presence is exactly what marks a
PDF as encrypted (PDF 32000-1 §7.5.5) — and cheaper than attempting to
parse further into a document whose object and stream contents may not
even be readable without a key.
`TestEncryptedDocumentIsRejected` proves the check fires before
anything else in `Parse` can throw a different, more confusing error
first.

**Rejected.**
- **Implementing RC4/AES decryption to support signing encrypted
  PDFs.** Explicitly out of scope per F3 §2.3; would also be exactly
  the kind of unrequested scope SPEC §0 warns against for a phase this
  large and risky already.

---

## D-044 — The independent verifier is a second, from-scratch implementation: its own DER/BER reader, its own re-tagging step, its own OID table

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/verify` imports none of
`internal/pades/pdf`, `internal/pades/cms` or `internal/pades/tsa`. It
has its own byte-level `/ByteRange` scanner (`byterange.go`, independent
of `pdf.Parse`'s xref-based object model), its own BER/DER TLV reader
(`der.go`, a separate type from `internal/pades/tsa`'s identically-
purposed `berNode`), its own copy of the RFC 5652 §5.4 re-tagging step
(`0xA0` → `0x31`), and its own OID constants.

**Why.** F3 §8 states the reason directly: "Shared helpers between
signer and verifier defeat the purpose: a bug in a shared helper passes
both ways." This was not a theoretical risk in this phase —
`TestCMSVerificationFailsWithoutRetagging` and this package's own
`TestVerifySignatureTamperInsideSignedRangeIsDetected` /
`TestVerifySignatureTamperInsideContentsIsDetected` exist specifically
because a verifier sharing the signer's re-tagging or offset logic would
pass them for the wrong reason. The strongest evidence this
independence is real, not just structural: this project's own
`internal/pades/verify` output was cross-checked against `openssl cms
-verify` (a third, completely unrelated implementation) on a real
signature produced by the full pipeline through the shipped CLI binary
— `openssl cms -verify -binary` reported "CMS Verification successful"
against the same `/ByteRange`-derived detached content this package
itself extracted, confirming the digest and signature computation
independently at two removes, not one.

**Rejected.**
- **Reusing `internal/pades/cms`'s DER helpers (`der.go`) for parsing,
  keeping only the *comparison* logic independent.** Rejected: F3 §8's
  own reasoning is that a bug anywhere in the shared path — not only in
  the final comparison — passes both ways. The DER reader is exactly
  where a subtle length- or tag-handling bug would live.

---

## D-045 — TSA retry policy: 3 attempts, 15 s timeout, 1 s then 3 s backoff; only network errors, timeouts and 5xx are retried

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/tsa.Client.Timestamp` implements exactly
F3 §6.3's numbers (`maxAttempts=3`, `attemptTimeout=15s`,
`backoffAttempt1=1s`, `backoffAttempt2=3s`). Only a network error, a
timeout, or an HTTP 5xx (`retryableError`) triggers another attempt; an
HTTP 4xx or an RFC 3161-level rejection (`*RejectionError`) returns
immediately.

**Why.** F3 §6.3 gives every number directly, with its own reasoning
for the retry/no-retry split: 4xx and RFC 3161 rejections are
deterministic, so "a retry only wastes the user's time." This project
verified the policy against Pošta's real public test TSA, not only a
fake server: `TestIntegrationPostaBasicAuthTSA` succeeds against
`https://test-tsa.ca.posta.rs/timestamp1` end to end (a real
`genTime`, a real embedded TSA certificate, a 4196-byte real token),
and `TestClientTimestampDoesNotRetryOn4xx` /
`TestClientTimestampDoesNotRetryOnRejection` /
`TestClientTimestampRetriesOn5xxThenSucceeds` (asserting the 1 s+3 s
backoff actually elapses) prove the retry/no-retry split against a
controlled fake server, since a real TSA cannot be made to fail 4xx or
5xx on demand for a repeatable test.

**Rejected.** Nothing else was considered for the numbers themselves —
F3 §6.3 specifies them directly; this entry exists to record them with
evidence per F3 §11, and to record that the retry/no-retry classification
was tested against both a real TSA (the happy path) and a fake one (the
failure paths a real service cannot be coerced into on demand).

---

## D-046 — OCSP: 10 s / 2 attempts; CRL: 30 s / 1 attempt; revocation is collected only after the signature and timestamp exist

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/dss`'s `ocspTimeout`/`ocspAttempts` are
10 s/2; `crlTimeout`/`crlAttempts` are 30 s/1. `internal/pades.SignDocument`
calls `dss.CollectRevocation` only after `builder.Finish()` has already
produced the final CMS (signature and, if requested, timestamp both
already embedded).

**Why.** F3 §7.2 states both timeout/attempt pairs directly, with
"CRLs are large" as the reasoning for CRL's longer timeout and single
attempt. F3 §7.2 also states the ordering requirement directly:
revocation must be collected *after* the signature and timestamp exist,
"so the OCSP response postdates the signature — which is what a
validator expects." `internal/pades/sign.go`'s `applyDSS` is called
only once `SignDocument` already has `cmsDER` in hand (the finished,
timestamped CMS), never before — there is no code path in this project
that collects revocation evidence before a signature exists to attach
it to.

**Rejected.**
- **Collecting revocation evidence up front, in parallel with signing,
  to save time.** Would violate F3 §7.2's ordering requirement directly
  — an OCSP response that predates the signature it accompanies is
  exactly what a validator is right to be suspicious of.

---

## D-047 — Achieved level is always reported explicitly; nothing in this project's output can claim a level it did not reach

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades.Result.AchievedLevel` starts at `LevelBB`
and is only ever raised (never assumed) as each step actually succeeds:
to `LevelBT` only after a TSA response is received and validated, to
`LevelBLT` only when `dss.Result.Complete` is true. Every degradation
path (`OnTSAFailureAbort=false`, incomplete OCSP/CRL evidence, a DSS
embedding failure) appends a human-readable note to `Result.Notes`
rather than silently returning success. `internal/cli.RunSign` prints
`AchievedLevel` and every note on every successful invocation — there
is no flag or code path that suppresses this.

**Why.** F3 §7.3 states the rule directly ("Never claim a level that was
not reached") and SPEC §18.11 makes it a hard, unconditional
prohibition project-wide. `TestSignDocumentFallsBackToBBOnTSAFailure`
and `TestApplyIncompleteWhenCollectionFails` are the failing tests this
discipline requires: both assert the *lower* level and a populated
`Notes`/`!Complete`, not merely that signing "succeeded."

**Rejected.**
- **Reporting the requested level with a separate `warnings` field**,
  rather than changing `AchievedLevel` itself. Rejected: a caller (or a
  future UI) reading only the headline level field must see the truth
  without also having to notice and interpret a secondary field — F3's
  own example output (§7.3) shows the achieved level in the primary
  `Level:` line, with the reason parenthetically alongside it, not
  elsewhere.

---

## D-048 — BER (including indefinite length) is accepted wherever this project parses a timestamp token; every token it produces or embeds is passed through unmodified, never re-encoded

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades/tsa`'s BER reader (`ber.go`) and
`internal/pades/verify`'s independent one (`der.go`) both accept
indefinite-length constructed values (the `30 80 ... 00 00` shape) and
fragmented (constructed) OCTET STRINGs. `tsa.Response.TokenDER` and
`cms.Builder.AddUnsignedAttribute`'s caller
(`internal/pades.SignDocument`) both carry the TSA's response bytes
through verbatim — `berNode.raw` captures the *exact* bytes received,
EOC markers included, and nothing in this project's signing path ever
re-serialises a timestamp token it did not construct itself.

**Why.** SPEC §12.5/F3 §5.5 state the reason directly, from real signed
documents: tokens produced by the state's own signing tool are BER with
indefinite length, and "a DER-only parser fails on documents signed
through the state portal, which is a large share of Serbian
documents." Real evidence this matters, from this phase's own work:
`internal/pades/tsa.TestParseBERIndefiniteLength` proves the reader
handles the indefinite-length shape correctly (including reassembling a
fragmented OCTET STRING), and the *real* Pošta TSA response captured by
`TestIntegrationPostaBasicAuthTSA` was a 4196-byte definite-length DER
token — meaning this phase never got to observe a genuinely BER
real-world token directly (no real already-signed document was
available, see [[D-038]]), so the BER path's correctness rests on the
hand-built indefinite-length test vector, not a live example, and that
gap is recorded here rather than glossed over.

**Rejected.**
- **A DER-only parser, deferring BER support until a real BER token is
  in hand.** Rejected: SPEC §12.5 states plainly that this is not a
  refinement — a large share of real Serbian documents are BER, and a
  parser that cannot read them is broken for exactly the input this
  project exists to handle.

---

## D-049 — The golden-file test's RSA key is a fixed, embedded, non-secret PEM, not generated at test time; measured: `crypto/rsa.GenerateKey` is not reproducible from a seeded `math/rand` source, `x509.CreateCertificate` is

**Date:** 2026-09-01
**Phase:** F3

**Decision.** `internal/pades.goldenKeyPEM` (in `golden_test.go`) is a
fixed RSA-2048 private key, generated once with real `crypto/rand` and
embedded as a PEM literal directly in the test file. The self-signed
certificate built from it still uses a seeded `math/rand.New(rand.NewSource(42))`
reader passed to `x509.CreateCertificate`.

**Why.** F3 §4.5/§16.2 require a byte-exact golden file, "sign with a
fixed test key and a fixed timestamp." The first approach tried was
generating the key itself from a seeded `math/rand` source at test
time, matching this project's stated preference for never committing
key material (`testdata/softtoken/local/README.md`'s own reasoning: "a
private key, even a throwaway test one, ends up copied into something
real eventually"). That approach does not work: measured directly (a
throwaway program, `rsa.GenerateKey(rand.New(rand.NewSource(42)), 2048)`
run three times as separate processes) produced three *different* keys
from the identical seed — confirmed the seeded reader's own raw output
is byte-identical across runs, so the non-determinism is internal to
`GenerateKey`'s primality search, not the reader. `x509.CreateCertificate`,
given an already-fixed key and the same seeded reader, was measured
separately and found fully reproducible (`cmp` on two separate process
runs: identical). This is why the certificate step still takes a seeded
reader (no reason to also fix a second artifact for a step that is
already reproducible) while the key itself is embedded outright. The
embedded key is not the same category of risk `testdata/softtoken/local`'s
reasoning warns about: it lives inline in versioned Go test source, not
as a portable `.p12`/`.pem` file that looks like exportable credential
material, and it signs nothing outside this one deterministic fixture —
`TestGoldenFile` itself proved this key produces byte-identical output
across repeated `-update` runs before being committed.

**Rejected.**
- **Continuing to generate the key from a seeded `math/rand` source.**
  Directly contradicted by measurement, recorded above.
- **A 512-bit or smaller "obviously toy" key size**, to visually signal
  non-seriousness. Rejected: RSA-2048 is what every other RSA key in
  this project uses (D-032's soft token, every test fixture in
  `internal/pades/cms`/`tsa`/`dss`/`verify`); a smaller golden-file key
  would be the only place in the codebase using a different, weaker
  size for no functional reason.

---

## D-050 — Real-fixture tests written; a genuine classic-xref parser bug found and fixed; F3 §2.1's structural table corrected

**Date:** 2026-09-01
**Phase:** F3/F4 boundary — real-fixture test task

**Decision.** With `mup.pdf`, `posta.pdf` and `halcom.pdf` now present in
`testdata/pdfs/local/` (never committed — [[D-038]]), the real-fixture
tests [[D-038]] and [[D-039]] deferred as "pending" are now written and
green: `internal/pades/pdf/real_test.go` (parsing, cross-reference
mechanism measurement, incremental-update byte preservation),
`internal/pades/verify/real_test.go` (signature/ByteRange/CMS extraction
and Trusted-List classification, independent verification of the
existing signature, BER document-timestamp parsing), and
`internal/pades/real_test.go` (the already-signed-document test, F3
§10.2, against a genuine prior signature for the first time). All follow
the same t.Skip()-when-absent pattern `classify_real_test.go` established
for real certificates.

Writing `TestRealFixturesCrossReferenceMechanismMatchesMeasurement`
exposed a real bug in `internal/pades/pdf/xref.go`'s classic-table
parser, not a fixture problem: `parseClassicXrefAt`'s subsection-header
parsing called `p.parseNumber()` twice in a row —for the start object
number, then the count — with no `p.skipWhitespaceAndComments()`
between them. `parseNumber` does not itself skip leading whitespace (by
design — see its callers elsewhere in this same file, which explicitly
skip before each call). Every real subsection header in the wild is
written with a single space between the two numbers ("`xref\n0 2052\n`"),
per the PDF 32000-1 grammar and confirmed byte-for-byte in all three
real fixtures, so the second `parseNumber()` call always failed with
"malformed xref subsection header" on real input. `Parse` silently
recovered via its `rebuildXref` fallback (intended for a corrupt
`startxref`, F3 §2.2) on **every** classic-table section of **all three**
real documents, without ever surfacing an error to a caller.

This was invisible with only synthetic fixtures for a specific reason:
`buildClassicFixture` (`internal/pades/pdf/fixtures_test.go`) writes the
identical "`xref\n0 %d\n`" shape and triggers the identical parser
failure, but its documents are small enough, and have no object streams,
that `rebuildXref`'s brute-force "`N G obj`" regex scan happens to find
every object anyway and produce a document that looks correctly parsed.
The bug had zero effect on any synthetic-fixture test's outcome, which is
exactly the blind spot [[D-021]] and [[D-038]] already named for
certificates and PDFs respectively: a synthetic fixture built from this
project's own understanding of the format cannot contradict a bug that
understanding shares. On the real fixtures the same fallback produced a
much more consequential silent failure: `rebuildXref`'s regex only finds
objects written literally as "`N G obj`" text, so every object compressed
inside an object stream — 1908 of mup.pdf's 2071 live objects, 62 of
posta.pdf's 119 — was **completely absent** from the merged xref table,
with no error, no warning, and no crash; `Document.Get` simply returns
`nil` for an object number the map does not contain, indistinguishable
from "legitimately free." This is precisely the class of defect SPEC
§12.1 and F3 §4.1 warn about for `/ByteRange` — "no crash and no warning,
the only way to catch it is to verify externally" — except here it is
one layer down, in the object model everything else in this phase is
built on. Before this document, no test (including F3's own fuzzing,
[[D-039]]) had a way to notice, because fuzzing checks robustness against
malformed input, not silent correctness loss on well-formed input that
happens to exercise a code path synthetic fixtures never stress in a way
that matters.

**The fix.** One line: `p.skipWhitespaceAndComments()` inserted between
the two `parseNumber()` calls in `parseClassicXrefAt`
(`internal/pades/pdf/xref.go`). After the fix, `parseXrefChain` succeeds
on every real classic-table section (no more fallback to `rebuildXref`
for these documents), and every one of mup.pdf's 2072 and posta.pdf's
120 merged xref entries resolves, matching the object-stream contents'
own declared counts exactly. All pre-existing tests (`go test ./...
-count=1`, and separately `go vet` and `go build` for all three release
targets) remain green — the fix only makes the intended, spec-following
code path succeed instead of silently falling through to the recovery
path; no test anywhere depended on the buggy fallback behaviour.

**F3 §2.1's structural table is corrected, not merely re-stated**, based
on directly walking each real document's `/Prev` chain (the method
`TestRealFixturesCrossReferenceMechanismMatchesMeasurement` uses) rather
than trusting a summary count:

| Fixture | Revisions | Classic sections | Cross-reference stream sections | Object streams |
|---|---|---|---|---|
| halcom.pdf | 3 | 3 | 0 | 0 |
| mup.pdf | 4 | 4 (one is a hybrid `xref 0 0` stub) | 1 | 4 |
| posta.pdf | 4 | 4 (one is a hybrid `xref 0 0` stub) | 1 | 1 |

F3 §2.1's original table ("MUP: 4 xref streams, 4 object streams; Pošta:
4 xref streams, 1 object stream") conflated "number of revisions" with
"number of cross-reference-*stream* sections": both real documents have
four total revisions, but only **one** of the four upgrades to a genuine
cross-reference stream — reached via a backward-compatible hybrid
`/XRefStm` pointer (PDF 32000-1 §7.5.8.4) from a same-revision classic
"`xref 0 0`" stub — with the other three revisions (the original base
document plus two later ones) using plain classic tables. The object-
stream counts in the original table (4 for mup, 1 for posta) were
correct; only the "4 xref streams" figures were not. `Document.UsesXrefStreams()`
still correctly reports `true` for both — the trailer-merge semantics
that decide it (newest-first, first-to-set-a-key wins) mean that once
any revision in the chain sets `/Type /XRef`, the merged trailer carries
it even though the textually newest revision is a plain classic table.
Whether an incremental update should key its own mechanism choice off
"was a stream ever used in this chain" (current behaviour) or "what did
the single newest revision use" (F3 §3.2's literal wording) is a real,
observed nuance this task did not require resolving — it does not affect
any of this project's own generated output, since this project's own
incremental updates never mix mechanisms mid-chain the way these
real, externally-produced documents happen to — and is left here as a
documented open question for whoever next touches `UsesXrefStreams()`,
rather than fixed speculatively.

Each real document was also found to carry **two** signature slots, not
one: the main PAdES signature (`/SubFilter /ETSI.CAdES.detached`) and a
separate document-timestamp revision (`/SubFilter /ETSI.RFC3161`, SPEC
§12.2) appended afterwards, whose own CMS is the BER, indefinite-length
`TimeStampToken` SPEC §12.5 requires tolerance for. The main signature's
own embedded (unsigned-attribute) timestamp separately exhibits the
SHA-1 imprint defect SPEC §12.4 already documents from the state's
signing tool; this project's independent verifier correctly reports that
specific check as failed, which is expected and asserted precisely (not
merely tolerated) in `TestRealFixturesIndependentVerificationAcceptsExistingSignature`.

`testdata/pdfs/local/README.md` is updated with the corrected table and
the two-signature-slot fact.

**Rejected.**
- **Trusting F3 §2.1's original counts and writing tests to match them.**
  Would have produced a test asserting `xrefStreamSections == 4` for
  mup.pdf, which fails against the real file — exactly the "if a test
  fails, that is the valuable outcome" instruction this task was given.
  Reporting a passing-but-wrong number would have been worse than not
  writing the test at all.
- **Weakening `TestRealFixturesCrossReferenceMechanismMatchesMeasurement`
  to only check `UsesXrefStreams()`** (the one figure that happened to
  still match a simple true/false expectation) instead of measuring
  sections and object streams independently. Rejected: doing so would
  have hidden both the xref-stream miscount and the classic-table parser
  bug behind a single boolean that, as it happens, comes out right for
  the wrong reason (the trailer-merge quirk described above).
- **Fixing `UsesXrefStreams()`'s trailer-merge semantics to reflect only
  the newest revision.** Out of scope for this task, which is bounded to
  writing tests and fixing bugs a test exposes; no test written here
  requires that change, and no existing test's expectation depends on
  the current behaviour being wrong. Recorded above instead as an open
  question.

---

## D-051 — Font subsetting: a hand-written TrueType table writer, reading the source font through `golang.org/x/image/font/sfnt`

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `scripts/gensubsetfont` (a build-time-only developer tool,
never imported by anything that ships) parses the source NotoSans TTF
with `golang.org/x/image/font/sfnt` — the Go team's own sfnt decoder —
and calls `Font.LoadGlyph` to get each required glyph's outline, already
flattened (composite glyphs such as "č," base "c" plus a combining
caron, resolved into plain contours by the decoder itself). The actual
*subset* — a new, minimal, valid TrueType binary containing only the
required glyphs — is assembled by hand in `scripts/gensubsetfont/ttfbuild.go`:
`glyf`/`loca`/`cmap`/`head`/`hhea`/`hmtx`/`maxp`/`name`/`post` are all
written from scratch, including the OpenType checksum algorithm. `OS/2`
is deliberately omitted — PDF's `/FontDescriptor` supplies every metric
(`Ascent`, `Descent`, `CapHeight`, `ItalicAngle`, `StemV`, `FontBBox`)
a CIDFontType2 embedding actually needs; the embedded font's own `OS/2`
table is not consulted by a PDF viewer for glyph rendering, only this
project's own hand-written table writer's correctness is.

**Why.** F4 §3.2 requires the subset generated at build time and
committed as an asset, with no font parser shipped at runtime — met
exactly, since `golang.org/x/image/font/sfnt` is a dependency of
`scripts/gensubsetfont` only (a `go build ./cmd/liro-bridge/...`
dependency-graph check, `go list -deps`, confirms `golang.org/x/image`
does not appear anywhere in the release binary's dependency graph) and
of `internal/pades/appearance`'s own tests (excluded from the release
binary by definition — Go never links `_test.go` files into a normal
build). Using the Go team's own decoder for the *reading* half is the
same trust tier D-041 already extended to `golang.org/x/crypto/ocsp`:
a well-defined, non-signature-critical binary format with no
Serbian-CA-specific quirks, where hand-rolling a second TrueType parser
(composite-glyph resolution, hinting bytecode, cmap format variants)
would be exactly the "complexity with no corresponding benefit" SPEC §0
warns against. The *writer* half is hand-written specifically because it
is the part this project actually controls the shape of — one target
format (simple, uninstructed, unhinted TrueType outlines; a fixed,
known table set) — and because F4's exit condition ("renders correctly
in Adobe Reader... Cyrillic renders as Cyrillic") is a correctness bar
best met by code this project can read end to end, matching the same
reasoning SPEC §12.1 applies to the PDF/CMS layers, extended here on
this project's own initiative since F4 does not say one way or the
other for fonts.

**Verification.** Two independent self-checks run inside the generator
itself, both against the *regenerated* subset re-parsed cold by
`golang.org/x/image/font/sfnt` (never against this project's own
writer): `verifySubset` confirms every rune's GID and advance width
round-trip, and `verifyOutlineGeometry` compares every glyph's bounding
box against the *source* font's bounding box for the same rune — the
check that actually exercises `encodeSimpleGlyph`'s Y-axis handling
(`sfnt.LoadGlyph`'s Y axis increases down; TrueType's own `glyf` storage
and PDF text space both increase up — the single easiest sign error to
make, and the one a bounding-box mismatch would show immediately as a
flipped, implausible box, not a subtly wrong one). A third,
outside-the-generator check exists too: `internal/pades/appearance/font_test.go`'s
`TestEncodeCIDsCyrillicMatchesIndependentlyParsedFont` parses the
*committed* `notosans-subset.ttf` asset with the same independent
decoder and compares every CID `EncodeCIDs` produces for SPEC §11.7's
real MUP name against that decoder's own `GlyphIndex` — and a rendered
visual check (a 900×80 rasterisation of "ВЕЉКО СТАНОЈЕВИЋ čćđšž АБВ" via
`golang.org/x/image/vector`, done once during this phase's manual
development and not committed as a test, since it duplicates what the
geometry and CID-matching tests already assert byte-for-byte) confirmed
the glyphs are legible, upright and correctly shaped, not merely
present.

**Rejected.**
- **A third-party Go font-subsetting library** (e.g.
  `github.com/cdillond/gdf/subset`, surfaced while researching this
  decision). Rejected: an unaudited, low-usage dependency producing the
  one binary artifact this phase's entire exit condition depends on
  ("renders correctly in Adobe Reader") is a worse risk trade than a
  ~400-line hand-written writer this project can read end to end and
  has already independently verified twice over.
- **Embedding the entire, unsubsetted NotoSans font.** Would satisfy
  Identity-H/CIDFontType2 rendering correctness trivially (CIDToGIDMap
  /Identity works against any GID the font actually has), but
  contradicts F4 §3.2's explicit requirement ("the subset contains
  exactly the characters needed, which keeps the embedded file small")
  and SPEC §13.2 verbatim.
- **Keeping composite glyphs composite** (reproducing TrueType's
  composite-glyph table format) instead of flattening every glyph to a
  simple outline. Rejected: `sfnt.LoadGlyph` already resolves composites
  into flat contours as a normal part of reading a glyph, so
  reconstructing the composite mechanism in the writer would be
  unrequested complexity with no benefit — this subset's total glyph
  count (166) is small enough that flattening costs a negligible amount
  of file size.

---

## D-052 — The subset tag is the fixed string "LIROBR", not randomised per build

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `/BaseFont`'s six-uppercase-letter prefix (F4 §3.1,
`/AAAAAA+NotoSans`) is always `LIROBR+NotoSans` — a constant in both
`scripts/gensubsetfont` and the generated `subset_data.go` — not drawn
from a random or content-derived source.

**Why.** The convention's real-world purpose is collision avoidance
when several different subsets of the same font family, from different
producers, end up embedded in one PDF (e.g. after merging documents
from different tools). That scenario cannot happen here: this project
signs one document at a time and embeds, at most, the one font subset
`scripts/gensubsetfont` produces, once per signing operation that
requests a stamp. A fixed tag is simpler (SPEC §0) and has a second,
concrete benefit a random one would not: `scripts/gensubsetfont`'s own
output is then reproducible byte-for-byte across regenerations from the
same source font and character set (already relied on by
`createdModified`'s fixed timestamp, D-051's sibling choice), which
matters for reviewing a diff when the subset is regenerated later.

**Rejected.**
- **A random six-letter tag per generator run.** Standard PDF-producer
  practice, but solves a collision problem this project's own
  architecture does not have, at the cost of making the committed
  `notosans-subset.ttf`/`subset_data.go` pair non-reproducible from the
  same inputs — a real cost for a hypothetical benefit.

---

## D-053 — A missing glyph is `errs.CodeSignFailed` with structured `Details`, not a new error code

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `appearance.MissingGlyphError` (a plain Go error naming
the character and its code point, F4 §3.3) is wrapped, at the
`internal/pades` boundary (`wrapStampError` in `sign.go`), as
`errs.WithDetails(errs.CodeSignFailed, err, map[string]any{"character":
..., "codePoint": "U+XXXX"})` — reusing the code F3 already uses for
every other reason a signing operation cannot complete (e.g. an
oversized `/Contents` reservation, `TestSignDocumentOversizedReservationFailsLoudly`),
rather than introducing e.g. `STAMP_GLYPH_MISSING`.

**Why.** SPEC §7 says new codes are for new *situations*, extensible as
phases require — but a missing glyph is not a new kind of failure from
the API's perspective, it is one more reason the signing operation as a
whole did not produce a signature, which is exactly what
`SIGN_FAILED` already means. `Details` is precisely the mechanism SPEC
§7 provides for the structured, machine-readable specifics a generic
code cannot carry ("`{"reader": "Generic Smart Card Reader"}`, never
prose" is SPEC §7's own example) — "which character, which code point"
fits that shape exactly. The CLI's own `errMessage` (`internal/cli/sign.go`)
was extended to append any non-empty `Details` to its local, English-or-
localised diagnostic output, so `--stamp` on a caller-supplied
`--stamp-reference` containing an unsupported character still names the
character to whoever is debugging the failure, satisfying F4 §3.3's "a
clear error naming the character and its code point" at the point a
human actually reads it — this local CLI output is not the JSON API
boundary SPEC §7's "no human-readable message crosses the API" rule
governs (`internal/api` does not exist yet).

**Rejected.**
- **A new `STAMP_GLYPH_MISSING` code.** Would work, but a missing glyph
  is not usefully distinguishable, from a *caller's* perspective, from
  any other reason the signing operation failed to produce output — both
  mean "no signature, try something else" — so a second code buys
  nothing a `Details` field does not already provide, at the cost of
  one more permanent entry in SPEC §7's table.
- **Leaving `MissingGlyphError` unwrapped**, letting the CLI's existing
  fallback (`err.Error()` for a non-`*errs.Error`) show it. Considered,
  since the raw message already names the character. Rejected because
  it is inconsistent with how every other stamp/signing failure in this
  package surfaces (always through `errs.Error`), and would silently
  stop working the moment any future caller (a real `internal/api`, in
  particular) starts branching on `errors.As(err, &errs.Error{})`
  rather than string content.

---

## D-054 — `givenName`/`surname`/IDCRS extraction is duplicated locally in `internal/pades`, not imported from `internal/trust/classify`

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `internal/pades/signer_identity.go` re-implements the
~20 lines of DN scanning needed to pull `givenName` (2.5.4.42) and
`surname` (2.5.4.4) out of a certificate (`signerDisplayName`) and to
find an `IDCRS`-prefixed `serialNumber` value (`documentIDFromCertificate`),
rather than calling `internal/trust/classify`, which already solved the
identical `givenName`/`surname` problem for D-019.
`internal/pades/appearance.Options` never receives an `*x509.Certificate`
at all — only plain strings (`SignerName`, `DocumentID`, ...) that
`internal/pades` has already extracted.

**Why.** F4 §5.2 states the requirement in the strongest terms this
project uses anywhere: "the national identity number must be
unreachable from this package." `internal/trust/classify`'s exported
surface for this is `Classify(cert, tslList, ...)`, which requires a
`*tsl.List` this package has no reason to depend on, so it is not a
drop-in replacement regardless; but the deeper reason is architectural,
not merely convenient. Making `internal/pades/appearance` structurally
incapable of ever parsing a certificate's Subject DN — because it never
receives one, full stop — is a stronger guarantee than "it receives a
`classify.Subject` that has already had PNORS scrubbed out of it,
trust us": the former is checkable by reading `appearance.Options`'
field list, the latter depends on `classify`'s scrubbing logic staying
correct forever, in a package this one does not own. The ~20 duplicated
lines are a small, one-time cost for a boundary a future reader (or a
future security review) can verify by inspection, matching this
project's own stated preference (F0/F3's dependency-rule discipline)
for boundaries enforced by structure over boundaries enforced by
convention.

**Rejected.**
- **Exporting `classify.ParseSubject(cert) Subject`** as a
  TSL-independent entry point, then having `internal/pades` call it.
  Would remove the duplication, but reintroduces exactly the
  "trust the shared package's scrubbing" dependency this decision
  avoids, and touches F1 code SPEC §0's own top-level instruction here
  ("do not modify earlier phases' code except where F4 explicitly
  requires it") does not ask for.
- **Passing the full `*x509.Certificate` into `appearance.Options`** and
  doing the DN scan inside `internal/pades/appearance` itself. Rejected
  outright: this is the one thing F4 §5.2's "unreachable from this
  package" most directly forbids — a certificate handed to the
  appearance package is a certificate the appearance package *could*, in
  principle, mis-scan for PNORS, however carefully the actual scanning
  code is written today.

---

## D-055 — "Up to four lines" (SPEC §13.5) admits a fifth when both optional lines are present; the height table's 72pt entry is for exactly that case

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `appearance.buildLines` produces, in order: label, signer
name, reference (if `--stamp-reference` supplied), identity document
number (if `--stamp-show-document-id` and the certificate has one),
serial+time — up to **five** lines, not four. `heightsByLineCount`'s
five entries (`44, 44, 46, 56, 72`) are used with `heights[lines-1]`,
so a five-line stamp (both optional lines present) gets 72pt.

**Why.** SPEC §13.5 says "up to four lines: label, signer name,
optional identifier line, certificate serial and time" — naming one
optional line. F4 §5's own content table separately lists "Reference"
as the optional third line. F4 §5.2 then introduces a second,
independently-gated optional line (the identity document number),
without revising either "up to four" statement to say "up to five."
Taking "up to four" as a hard ceiling would require the reference and
the identity-document-number line to share one slot somehow — F4 never
says how, and the two are governed by unrelated flags
(`--stamp-reference` is free-text the caller supplies; `--stamp-show-
document-id` is a boolean gating certificate-derived data) with no
natural reason to conflict or overwrite one another. The geometry
table's fifth entry (72pt) has no other purpose anywhere in F4 if
"up to four" is read as an absolute ceiling — SPEC §13.1 and F4 §2 both
describe the table as "index = number of lines," and a five-entry table
for a four-line-maximum feature would be dead space. Treating the fifth
entry as evidence that five lines is an anticipated, valid case is the
simpler reading (SPEC §0) than either capping optional content
arbitrarily or leaving one array entry unexplained.

**Rejected.**
- **Folding the identity document number into the Reference line**
  (e.g. appending it, or preferring one over the other) to stay within
  four lines. Rejected: invents a merge rule F4 never specifies, and
  would silently drop or garble whichever of the two the caller actually
  wanted shown, for two flags that have no stated relationship to each
  other.
- **Treating `--stamp-show-document-id` as replacing the Reference
  line** when both are requested. Same objection: an invented priority
  rule with no textual basis, and a caller passing both flags
  legitimately wants both facts on the stamp (e.g. an ERP's own document
  reference *and*, separately, the signer's opted-in ID number).

---

## D-056 — The stamp's fourth line shows the /M signing date, not the RFC 3161 timestamp time

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `appearance.Options.SigningTime` (the "serial and time"
line's time component) is `signingDate.Format("2006-01-02 15:04 MST")` —
the same `time.Time` that becomes the signature dictionary's `/M` entry
— computed and drawn into the stamp's content stream *before*
`SignDocument` ever contacts a TSA.

**Why.** F4 §5's content table names the source as "timestamp time,"
matching SPEC §12.2's PAdES convention that the trustworthy time is the
RFC 3161 timestamp's, not the signer's own clock. But the visual
stamp's content stream bytes are fixed by `internal/pades.applyStamp`
as a *prior* incremental revision, appended before
`pdf.BuildPlaceholder` even reserves the `/Contents` slot the signature
and its timestamp will later occupy — the timestamp token (if any:
B-B never has one) does not exist yet at the point the stamp's pixels
are decided, and a B-B result never obtains one at all. There is no
value available to put on the stamp that is both "the RFC 3161
timestamp time" and available before signing starts, short of
timestamping first and signing the resulting stale placeholder
second — which would invert F3's whole signing pipeline for a cosmetic
label. SPEC §12.3's actual, load-bearing rule — "no `signingTime`
CMS attribute; PAdES validators must read the time from the
timestamp" — is completely unaffected by what text a *visual* stamp
displays: nothing about `SigningTime` here touches the CMS's signed
attributes, and `internal/pades/verify` (and any real PAdES validator)
still determines trust in the timestamp token itself, never in the
stamp's own drawn text.

**Rejected.**
- **Reordering `SignDocument` to timestamp before building the
  placeholder**, so the real TSA `genTime` is available for the stamp.
  Rejected: this is a materially different pipeline shape than F3
  built and tested (placeholder → digest → sign → timestamp → inject),
  entirely to make one line of cosmetic stamp text more literally
  accurate, at real risk to the much more important byte-range/digest
  correctness F3 spent its whole phase getting right.
  SPEC §0's "do not invent requirements" cuts against a change this
  large for a label.
- **Leaving the time line blank when no timestamp exists yet, filling
  it in only for B-T/B-LT.** Would produce a stamp whose fourth line's
  presence depends on the achieved level in a way F4 never asks for,
  and still could not show the *real* timestamp time without the
  pipeline reordering above.

---

## D-057 — The truncation mark is three ASCII periods, not U+2026

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `appearance.truncationMark` is the literal string `"..."`
(three U+002E FULL STOP characters), not U+2026 HORIZONTAL ELLIPSIS.

**Why.** F4 §5.4/SPEC §13's overflow rule says "truncate with an
ellipsis" without specifying the character. F4 §3.2 fixes the embedded
character set exactly: ASCII, the five Serbian Latin diacritics (both
cases), and the Serbian Cyrillic alphabet — U+2026 is in none of those
groups. Adding one character to the subset purely to spell the word
"ellipsis" literally would mean re-running `scripts/gensubsetfont`
against every regeneration henceforth for a single cosmetic glyph, and
would make this one line of stamp-overflow code the sole reason the
"exactly the characters needed" character set (F4 §3.2) is not exactly
what F4 §3.2 itself lists. Three ASCII periods are visually
indistinguishable from a typographic ellipsis at the stamp's small font
size and are already inside the committed character set with no changes
needed anywhere.

**Rejected.**
- **Extending the character set with U+2026.** Would work, but changes
  a build-time asset (regenerating `notosans-subset.ttf` and
  `charset.txt`) to support one specific punctuation mark this
  project's own truncation logic is free to spell differently — not
  worth the churn for a purely cosmetic difference.

---

## D-058 — Overflow font sizes: 7pt nominal, 6pt reduced (F4 §5.4's "one step")

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `appearance.nominalFontSize` is 7, `reducedFontSize` is 6
— every stamp line is drawn at 7pt unless it overflows the text
column's available width (`StampWidth - Padding*3 - LogoSize`, F4 §2),
in which case it is redrawn at 6pt, and only then truncated with the
mark from D-057 if it still overflows.

**Why.** Neither SPEC §13 nor F4 §5.4 specifies a font size anywhere —
only the overall stamp geometry (190pt width, 24pt margin, 36pt logo,
4pt padding, the five-entry height table) is fixed. 7pt is a
conventional small-print size that fits comfortably within the
smallest (44pt) stamp height alongside a 36pt logo and up to two lines
of text with visible line spacing; 6pt (F4 §5.4's literal "one step")
is chosen as a single, modest reduction — enough to noticeably shrink
long text without becoming illegible at typical print/screen
resolutions, matching SPEC §0's instruction to pick the simplest
workable value when a rule is silent about a specific number and record
it here.

**Rejected.**
- **Computing font size continuously to fit any given text exactly**,
  rather than one fixed "nominal" and one fixed "reduced" size. Directly
  contradicted by F4 §5.4's own wording — "reduce the font size in
  **one step**" describes a two-value scheme, not a continuous one.

---

## D-059 — The visible stamp is a separate, prior incremental revision, added before `pdf.BuildPlaceholder`'s own revision

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `internal/pades.applyStamp` parses the input PDF, appends
one incremental revision containing the stamp's font, logo and Form
XObject (via `internal/pades/appearance.Render`, given its own
`pdf.Update`), and returns the updated bytes. `SignDocument` then
re-parses those bytes and calls `pdf.BuildPlaceholder` exactly as it
always did, now passing `PageNumber` and an `Appearance` (Rect + a
reference to the already-appended Form XObject) so the widget
annotation `BuildPlaceholder` adds is visible instead of `Rect [0 0 0
0]`. A stamped, B-LT document therefore has at least three revisions
appended over the original: stamp objects, the signature placeholder,
and (if DSS applies) `/DSS`.

**Why.** `pdf.BuildPlaceholder` owns its own `*pdf.Update` internally
and calls `u.Apply()` before returning — there is no seam for a caller
to inject additional objects into that same revision without a more
invasive change to `BuildPlaceholder`'s own signature and internal
control flow, which F3 already got right and this phase should not
need to touch beyond adding the two new options fields (`PageNumber`,
`Appearance`) it actually needs. A second, prior revision is not a
workaround for that seam's absence so much as the natural shape once
noticed: `internal/pades.applyDSS` (F3) already adds a *further*
revision on top of a completed signature for exactly the same
reason — one operation, cleanly separated concerns, each producing its
own append-only revision — so a stamp revision *before* the signature
revision is the same pattern applied one step earlier in the pipeline,
not a new one. It also keeps `internal/pades/pdf` genuinely ignorant of
what a stamp looks like (F4 §6): `PlaceholderOptions.Appearance` is
just a `Rect` and a `Reference`, built and populated entirely by
`internal/pades/appearance` and `internal/pades`, never by
`internal/pades/pdf` itself.

**Rejected.**
- **Changing `BuildPlaceholder`'s signature to accept a callback that
  adds objects to its own internal `*pdf.Update` before `Apply()`.**
  Would produce one revision instead of two, saving a small amount of
  file size, but requires exposing `BuildPlaceholder`'s internal
  `*pdf.Update` and object-allocation order to a caller in a new way
  that F3 never needed and that couples `internal/pades/pdf` to the
  existence of a stamp-building step it should not need to know about.
- **Building the stamp's objects as part of the *same* revision by
  having `internal/pades` construct the whole `pdf.Update` itself and
  passing it into a lower-level `BuildPlaceholder` variant.** Same
  objection, with more surface area: this reshapes `internal/pades/pdf`'s
  public API for one caller's convenience.

---

## D-060 — `/Rotate` handling: the Form XObject's `/Matrix` counter-rotates; `Rect` is computed per rotation × corner from a 16-entry lookup table

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `appearance.PlaceCorner` (`geometry.go`) maps every
`(/Rotate ∈ {0,90,180,270}) × (corner ∈ {four corners})` combination to:
which physical MediaBox corner to anchor at (which differs from the
*requested visual* corner once rotation is not zero), whether the
stamp's width/height swap in content-space, and a Form XObject `/Matrix`
that rotates the stamp's own content — always authored upright, in its
own natural reading orientation — by the angle that cancels out the
page's `/Rotate` once a viewer applies it. `internal/pades/appearance.Render`
resolves `/MediaBox` and `/Rotate` (via the new `internal/pades/pdf.ResolveMediaBox`/
`ResolveRotate`, walking the `/Parent` chain per F4 §2.1) and passes
them straight into `PlaceCorner`.

**Why.** PDF defines `/Rotate` as the clockwise angle a viewer rotates
the page *for display* (PDF 32000-1 §7.7.3.3) — the content stream's own
coordinate system is never rotated. Placing a stamp's `Rect` using
un-rotated bottom-right arithmetic on a `/Rotate 90` page therefore
lands it at a corner that is not the bottom-right *of what a person
looking at the rendered page actually sees* — exactly the failure mode
F4 §2.1 names directly. The derivation (recorded in full in
`geometry.go`'s own doc comment, since it is the kind of reasoning a
future reader will want at the point of the code, not only here):
physically rotating a rectangle 90° clockwise moves its top-left corner
to its top-right, top-right to bottom-right, bottom-right to
bottom-left, bottom-left to top-left; walking that cycle backwards
gives, for each requested *visual* corner, the *physical* MediaBox
corner to anchor at before rotation is applied. The companion `/Matrix`
(a pure rotation, verified by `TestPlaceCornerLandsInsidePageBoxEveryCornerEveryRotation`'s
determinant check) pre-rotates the stamp's own content the opposite way,
so the two rotations cancel and the stamp reads upright regardless of
the page's `/Rotate`. This was verified visually, not only
geometrically: signing a synthetic `/Rotate 90` and a `/Rotate 270`
page and rendering the result with `pypdfium2` (Google's PDFium,
independent of this project's own code) during this phase's manual
development showed the stamp upright at the intended corner in both
cases, with the page's own body text visibly rotated as expected — the
strongest evidence available in this environment short of Adobe Reader
itself (SPEC's manual-acceptance item, still pending human
verification).

**Rejected.**
- **Ignoring `/Rotate` and always computing `Rect` against the raw
  MediaBox.** This is F4 §2.1's explicitly named failure mode
  ("a stamp placed in the bottom-right of an unrotated coordinate
  system lands somewhere else on a rotated page") — not a candidate,
  recorded here only because it is the naive baseline every other
  option improves on.
- **Rotating the *annotation* (`Rect`) instead of the *content*
  (`/Matrix`).** A `Rect` is an axis-aligned rectangle in content space;
  it has no rotation of its own to set — only its position and, via
  swapped width/height, its aspect ratio can encode a rotation's effect.
  The actual counter-rotation has to live somewhere that can express an
  angle, which is what `/Matrix` is for.

---

## D-061 — The logo is a generated, deliberately generic mark (a disc with a checkmark), not a Liro brand asset

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `scripts/genlogo` draws the stamp's fixed 36×36pt logo
programmatically — a solid-colour disc (an arbitrary, unbranded blue,
`#2A4D8F`) with a white checkmark stroke, computed directly with
`image`/basic distance-to-segment math, no font or external asset
involved — and pre-compresses the result (zlib/RFC 1950, matching what
PDF's `/FlateDecode` filter requires) into two committed assets,
`logo-rgb.flate` and `logo-alpha.flate`, embedded via `//go:embed`.

**Why.** F4 §4 fixes the logo's geometry (36×36pt, RGB + `/SMask` alpha,
both Flate-compressed) and says "the logo is fixed; do not make it
configurable in this phase," but does not say what it depicts — and
SPEC §10 states directly that "the agent has no dependency on the Liro
Design System," which is a separate React monorepo this implementation
environment has no access to and no authority to invent brand assets
for. Drawing a small, genuinely unbranded mark is the option that
satisfies F4 §4's structural requirements (a real RGB+alpha image pair,
correctly embedded) without fabricating something that looks like an
official Liro logo but is not one — which would be actively misleading
to whoever next looks at a signed document expecting the eventual real
mark. Pre-compressing at build time (rather than encoding raw pixels at
runtime) keeps `internal/pades/appearance`'s runtime path doing nothing
but copying static bytes into the PDF stream, matching this phase's
broader "generate once, embed the result, parse/encode nothing at
signing time" discipline (D-051) applied to the one other binary asset
this phase introduces.

**Rejected.**
- **Leaving the logo out entirely, or as a blank/placeholder rectangle.**
  Rejected: F4 §4 requires an actual RGB image plus alpha mask,
  referenced through `/SMask` — an empty or degenerate image would not
  exercise (or prove) that code path at all.
- **Asking the user to supply a real Liro logo asset before proceeding.**
  Considered, but F4 §4's own instruction ("the logo is fixed; do not
  make it configurable") reads as "commit *something* fixed now," and
  this project's established pattern for missing real-world material
  it has no access to (D-021, D-038) is to build the structurally
  correct synthetic stand-in and say so plainly, not to block the phase.

---

## D-062 — "No 13 consecutive digits" is checked against the stamp's plain-text option fields, not the finished PDF's raw bytes

**Date:** 2026-09-01
**Phase:** F4

**Decision.** The security-relevant test this project's engagement rules
require ("no stamp output may contain 13 consecutive digits") is
implemented as `TestBuildAppearanceOptionsNeverContainsThirteenConsecutiveDigits`
(`internal/pades/stamp_test.go`), which scans the plain-text strings
`internal/pades.buildAppearanceOptions` produces — `Label`,
`SignerName`, `Reference`, `DocumentID`, `SerialHex`, `SigningTime` —
against a certificate whose Subject deliberately carries a real,
13-digit, JMBG-shaped `PNORS` value in the same multi-valued RDN as an
`IDCRS` value (SPEC §11.6 Trap 1's worst case), with every optional
stamp line turned on. It does not scan the finished, signed PDF's raw
bytes for a 13-digit run.

**Why.** Scanning raw output bytes was tried first and produces false
failures with no connection to personal-data leakage: Identity-H text is
stored as two-byte glyph indices written as hexadecimal (F4 §3.3), and
small glyph indices routinely hex-format using only the characters
0-9 — CID 84, for instance, is the four characters `"0054"`. Ordinary,
entirely safe stamp text (a caller's own `--stamp-reference`, this
project's own generated label and date text) can therefore produce runs
of 13 or more digit *characters* in the encoded `Tj` operand with no
13-digit *number* anywhere in the underlying content — confirmed
directly: the first version of this test, scanning `result.Bytes`,
failed against a completely unrelated CID sequence
(`"...004F0046004500010043005A..."`) the very first time it ran, on
output that did not contain the test's PNORS value in any human-readable
form at all. Checking the *source strings* before CID encoding is the
check that actually matches what the rule is protecting against — and
is provably equivalent to checking what a human or an accessibility
tool would read from the finished stamp, since `internal/pades/appearance`'s
own `TestToUnicodeCoversEveryGID`/`TestEncodeCIDs*` tests already
establish that the `/ToUnicode` mapping and the CID encoding are
lossless and exact.

**Rejected.**
- **Scanning `result.Bytes` for `\d{13}` directly**, accepting the false
  positives and hand-tuning the pattern to exclude hex-CID-shaped runs
  (e.g. requiring the digits not be immediately preceded by `<` or
  followed by `>`/another hex digit). Rejected: fragile — the exact
  shape of a false positive depends on which glyphs happen to be drawn,
  which is a property of *this* generator run's GID assignment, not
  something a test should need to special-case around. A test that
  passes by accident of GID numbering is not a check that would actually
  fail if a real leak were introduced.
- **Stripping all `<...>` hex-string spans from `result.Bytes` before
  scanning**, to approximately exclude CID data. Rejected as more
  complex than checking the source strings directly, for a weaker
  guarantee (it would still scan CMS/DER binary data and the
  `/Contents` placeholder's own hex digits, both irrelevant to the
  stamp).

---

## D-063 — `golang.org/x/image` is scoped to the font-subset generator and this project's own tests; verified absent from the release binary's dependency graph

**Date:** 2026-09-01
**Phase:** F4

**Decision.** `go.mod` gained `golang.org/x/image v0.45.0` (BSD-3-Clause,
the standard `golang.org/x/*` licence, compatible with this project's
Apache 2.0 licence per D-005's own reasoning) as a direct dependency.
It is imported by exactly two places: `scripts/gensubsetfont` (a
developer tool, D-051) and `internal/pades/appearance/font_test.go`
(a `_test.go` file, never compiled into a normal build). `go list -deps
./cmd/liro-bridge/...` — the actual release binary's import graph — was
checked directly and does not contain `golang.org/x/image` anywhere in
it.

**Why.** SPEC §8.6 requires recording what a new dependency does, why
the standard library is insufficient, and its licence, for every
dependency added — the standard library has no TrueType/OpenType font
support at all, so some decoder was unavoidable for D-051's approach;
`golang.org/x/image/font/sfnt` is the Go team's own, already justified
at the same trust tier as `golang.org/x/crypto/ocsp` (D-041). Recording
the *scope* explicitly, with a reproducible verification command rather
than an assertion, follows D-031's own precedent (the softtoken
build-tag exclusion is checked two-directionally in CI, not merely
claimed) — a dependency that is genuinely test/tool-only is worth
proving, not just stating, given how easily an unrelated future change
could accidentally start importing it from runtime code.

**Rejected.** Nothing else was considered for the dependency itself —
D-051 already justifies choosing it over the alternatives (a third-party
subsetting library, or hand-writing a second TrueType parser). This
entry exists specifically to record its scope and licence per SPEC
§8.6, which D-051 (about the subsetting *design*) does not itself
cover.

---

## D-064 — F4 §9's remaining recorded facts: missing glyphs fail loudly by construction, `/ToUnicode` is unconditional, and the name/identity rules are SPEC's own, not a choice made here

**Date:** 2026-09-01
**Phase:** F4

**Decision, restated for the record (F4 §9 asks each of these to be
recorded explicitly, even where SPEC leaves no real alternative to
weigh):**

- **A missing glyph fails loudly, unconditionally.** `appearance.EncodeCIDs`
  (`font.go`) has exactly one path for a rune not in `runeToGID`: return
  `*MissingGlyphError`. There is no fallback branch, no default glyph,
  no `.notdef` substitution anywhere in the call chain from
  `EncodeCIDs` up through `fitLine`, `buildContentStream`, `Render`,
  `applyStamp`, to `SignDocument` — the error simply propagates,
  wrapped only for its `errs.Code` (D-053), until the whole signing
  operation fails. `TestEncodeCIDsMissingGlyphFailsLoudly` and
  `TestRenderRejectsMissingGlyphInReference` are the tests this
  project's own engagement rules require for a check like this — proof
  it actually fires, not just that the code path exists.
- **`/ToUnicode` is present on every stamp, never conditionally.**
  `addFontObjects` (`font.go`) always allocates and writes the
  `ToUnicode` CMap stream as part of the font object graph — there is
  no code path that embeds the font without it, and no option field
  controls whether it is written. `TestToUnicodeCoversEveryGID` asserts
  every GID the subset defines has a `bfchar` entry, not merely that
  the stream exists.
- **The signer name is `givenName` + `surname`, never CN**, exactly as
  D-019 established for F1's certificate listing, applied here to the
  stamp: `internal/pades.signerDisplayName` reproduces SPEC §11.7's
  three real, measured CN values directly in
  `TestSignerDisplayNameUsesGivenNameAndSurname` (`ВЕЉКО СТАНОЈЕВИЋ
  011445479 Sign`, `Zoran Milovanović 246275`, `Redžvel Mešković
  200094362`) and asserts the built name is exactly the given/surname
  pair, containing neither the CA number nor MUP's literal " Sign"
  suffix. See D-054 for why this logic lives in `internal/pades` rather
  than being imported from `internal/trust/classify`.
- **The identity document number defaults off; the national identity
  number is unreachable, not merely undisplayed by default.** These are
  two different strengths of guarantee and this project implements
  both at their stated strength. `StampOptions.ShowDocumentID`'s zero
  value is `false` (Go's own default), so "opt-in, never the default"
  (F4 §5.2) holds without any extra code — the CLI's own
  `--stamp-show-document-id` flag likewise defaults to `false`
  (`internal/cli/sign.go`). Separately, and stronger,
  `documentIDFromCertificate` (`internal/pades/signer_identity.go`)
  never recognises the `PNORS` prefix at all, under any option or code
  path — proven by `TestDocumentIDFromCertificateNeverReturnsPNORS` and
  `TestDocumentIDFromCertificateIgnoresPNORSEvenInsideSameMultiValuedRDN`,
  and reinforced project-wide by
  `TestBuildAppearanceOptionsNeverContainsThirteenConsecutiveDigits`
  (D-062).
- **`/Rotate` is respected.** Recorded in full, with its derivation, as
  D-060.

**Why recorded together.** F4 §9 lists these as six separate bullet
points to record; five of the six (all but `/Rotate`, large enough to
warrant its own entry, D-060) are cases where SPEC or F4 itself dictates
the outcome directly and unambiguously, leaving nothing to weigh — the
same situation D-042/D-045 were already in for F3's fixed numbers.
Per those entries' own precedent, this records the fact, the evidence
(the specific test that would fail if the discipline regressed), and
the exact SPEC/F4 citation, rather than a weighed decision with
rejected alternatives that were never actually live options.

**Rejected.** Nothing else was considered for any of the five points
above — each is SPEC's or F4's own unconditional rule, not a design
space this phase had genuine latitude in.

---

## D-065 — The signature field name is chosen at signing time, not hard-coded: "Signature1" collided with a real document's own field

**Date:** 2026-09-01
**Phase:** F4/F5 boundary — fix-and-polish pass (Task 1)

**Decision.** `pdf.BuildPlaceholder` no longer defaults an empty
`PlaceholderOptions.FieldName` to the literal `"Signature1"`. It now
calls `uniqueFieldName(doc, fields)`, which walks the document's entire
existing `/Fields` tree — the top-level array and every `/Kids` subtree
beneath it, not only the top level, since a field's `/T` can be a
partial name qualified by its `/Parent` chain — collects every `/T`
value found at any depth, and returns the first of
`"Liro-Signature-1"`, `"Liro-Signature-2"`, ... not already present.
`internal/pades.SignDocument` still never sets `FieldName` itself (it
never has); the new default logic lives entirely in
`pdf.BuildPlaceholder`, which is where the collision this document
records was introduced. `testdata/golden/minimal-signed-bb.pdf` was
regenerated (`go test ./internal/pades/ -run TestGoldenFile -update`):
the golden document has no pre-existing fields, so its own field is
still the first name tried, `"Liro-Signature-1"`, seven bytes longer
than the old literal — the only byte-level effect of this fix on a
document with no prior signature.

**Why.** Confirmed by independent analysis of a real MUP-signed PDF
resigned by this project: the output carried three signature fields —
`/T "Signature1"` (the document's original CAdES signature),
`/T "Signature2"` (its original document timestamp), and `/T
"Signature1"` again (this project's own, freshly added). Per PDF
32000-1, `/T` must be unique within the AcroForm field tree; two fields
sharing a name are one field with two conflicting `/V` values to any
reader that assembles the tree — which is exactly what Adobe Acrobat
does, reporting "At least one signature is invalid" and rendering an
empty Signature Panel for every one of this project's outputs against a
real, already-signed document. Every cryptographic property of that
same output was independently verified correct: the original bytes are
a literal prefix of the new file, the original CAdES signature's
`messageDigest` still matches its `/ByteRange` digest, the original
document timestamp still verifies, this project's own new signature
verifies, and the appended revision correctly uses a cross-reference
stream — this is a one-string bug in a project whose cryptography and
incremental writer are otherwise sound, the same class of failure as
[[D-050]]'s classic-xref parser bug: everything a test checked was
green, because every existing test signs a document previously signed
by this project's own code, which always emitted the same field name —
so the collision reproduced identically in the fixture and in the
signature it was compared against, and no test asserts anything about
the AcroForm field *tree*, only about cryptographic properties, which
are unaffected either way. `internal/pades/fieldname_real_test.go`'s
`TestRealFixturesSignatureFieldNameIsUnique` is the test that closes
this: it signs each of the three real fixtures in
`testdata/pdfs/local/` and asserts every `/T` in the resulting document
is distinct. Run against the code exactly as it stood before this
fix (`FieldName` defaulting to the literal `"Signature1"`), it failed
on all three fixtures with "field name \"Signature1\" appears 2
times" — confirmed directly before the fix landed, not assumed. After
the fix, the same test passes on all three.

`"Liro-Signature-N"` rather than continuing the `"SignatureN"`
convention: Adobe, NexU and the eUprava applet — the tools that produce
the vast majority of already-signed Serbian PDFs this project will ever
be asked to re-sign — all emit `"SignatureN"`, so a document already
signed in Serbia is disproportionately likely to already contain those
exact names. A convention this project does not share with the tools
most likely to have signed the document first is the only choice that
actually avoids the collision rather than merely relocating it (e.g.
`"Signature2"` would have collided with the same real document's
*document-timestamp* field in this exact case).

**Rejected.**
- **Numbering only from the count of existing top-level `/Fields`
  entries** (e.g. `len(fields)+1`), instead of collecting every `/T`
  value. Rejected: a document's `/Fields` array can contain non-terminal
  fields whose own `/T` is a partial name with no relation to the
  array's length, and — more directly — this undercounts whenever a
  field's kids (not the array itself) are what actually carries a
  clashing name, which is exactly the "field names can be hierarchical"
  trap this task named explicitly.
- **A GUID or random suffix** (e.g. `"Liro-Signature-" +
  uuid.New()`), to make collision structurally impossible rather than
  merely checked-for. Rejected: this project's own incremental updates
  never mix mechanisms unpredictably (D-050's own note on
  `UsesXrefStreams()`), a checked, deterministic, human-readable name is
  simpler to read in a PDF viewer's field list and simpler to reproduce
  in a test, and SPEC §0 favours the simplest option that actually
  solves the stated problem — collision avoidance here is fully solved
  by checking the tree, since this project only ever adds one field per
  signing operation.
- **Keeping `"SignatureN"` but starting the count above whatever the
  document's own highest `SignatureN` suffix is.** Rejected: still
  collides with any tool (this project's own prior output included)
  that names fields non-sequentially or restarts its own counter, and
  buys nothing over checking the actual existing names directly.

---

## D-066 — `STAMP_GLYPH_MISSING` replaces the `SIGN_FAILED` mapping for a missing stamp glyph; [[D-053]] superseded

**Date:** 2026-09-01
**Phase:** F4/F5 boundary — fix-and-polish pass (Task 2)

**Decision.** `errs.CodeStampGlyphMissing` (`"STAMP_GLYPH_MISSING"`) is
a new code. `internal/pades.wrapStampError` now wraps
`appearance.MissingGlyphError` as
`errs.WithDetails(errs.CodeStampGlyphMissing, err, map[string]any{"character":
..., "codePoint": "U+XXXX"})`, replacing the `errs.CodeSignFailed`
mapping [[D-053]] chose. All three `internal/i18n` catalogues gained
`error.stamp_glyph_missing`, a template ("The stamp contains a
character the font does not support: %s (%s)." in English) that
`internal/cli.errMessage` formats directly with the error's
`character`/`codePoint` `Details` — the one error code whose local CLI
message is built by filling a template rather than by appending
`Details` generically after a fixed sentence, because SPEC §13.2's own
required wording ("a clear error naming the character and its code
point") only reads correctly as one sentence, not as a sentence plus an
appended `key=value` fragment.

**Why.** [[D-053]] reasoned that a missing glyph "is not a new kind of
failure from the API's perspective" and reused `SIGN_FAILED`, on the
theory that `Details` already carries whatever a caller needs to
distinguish it. That reasoning did not survive contact with what a
person actually sees: running this project's own CLI against a stamp
containing an unsupported character prints "Kartica nije uspela da
napravi potpis" — "the card failed to produce a signature" — for a
problem that has nothing to do with a card, a reader, or the signing
operation at all. `SIGN_FAILED`'s catalogue text is not a neutral code
name to a person reading it; it is a sentence that sends them to check
hardware, for a problem that lives entirely in the stamp's own text.
[[D-053]]'s own objection to a new code — "not usefully distinguishable
from a caller's perspective... both mean 'no signature, try something
else'" — is true of a *machine* caller branching on the code, but this
project has no such caller yet (`internal/api` does not exist); the
only caller today is a human reading `internal/cli`'s localised
message, for whom "check your card" and "one character in your stamp
text isn't supported, try a different one" are not the same instruction
at all — a genuinely different *situation* in SPEC §7's sense, not a
cosmetic distinction invented for its own sake.

**Rejected.**
- **Keeping [[D-053]]'s mapping and fixing only the catalogue text
  under `error.sign_failed`.** Would fix this one symptom but leaves
  `SIGN_FAILED` meaning two genuinely different things (a hardware/card
  failure and a stamp-content problem) behind one code, which is exactly
  what a future `internal/api` caller would need to branch on and could
  not — the same failure mode [[D-053]]'s own choice produced, just
  moved one layer earlier.
- **A generic `VALIDATION_FAILED`-style code covering every
  caller-supplied-content problem.** Rejected as inventing an
  unrequested abstraction (SPEC §0) for a project with exactly one
  known case of this shape so far; `STAMP_GLYPH_MISSING` names the
  actual, specific, already-measured situation.

---

## D-067 — `sign` fails, by default, when a level is requested and no TSA is configured — rather than shipping a hard-coded default TSA endpoint

**Date:** 2026-09-01
**Phase:** F4/F5 boundary — fix-and-polish pass (Task 7)

**Decision.** `internal/pades.SignDocument` no longer skips its whole
timestamp step when `opts.TSA == nil`. It now attempts the step
whenever `opts.RequestedLevel != ""` — the CLI always sets one (`--level`
defaults to `"b-lt"` and accepts only `"b-t"`/`"b-lt"`, never empty) —
treating "no TSA configured" as a `tsaErr` exactly like a TSA that was
contacted and failed: `opts.OnTSAFailureAbort` (default `true`, i.e.
`--on-tsa-failure abort`) still decides whether that aborts the whole
operation or degrades to B-B with an explanatory `Result.Notes` entry.
No default timestamp authority was added anywhere in this project.

**Why.** Established directly, not assumed: `liro-bridge sign` with no
`--level` and no `--tsa` produced `Nivo: B-B` with no warning, because
no TSA is configured by default — `--tsa` defaults to the empty string,
so `var client *tsa.Client` in `internal/cli.RunSign` stays `nil`
unless the user passes it — and `SignDocument`'s old
`if opts.TSA != nil { ... }` guard meant the entire timestamp step,
including the `Result.Notes` degradation message every other TSA
failure path already produces, was skipped outright. This is exactly
SPEC §12.8/§18.11's "never silently downgrade" — the CLI's own
documented default level (`b-lt`) was silently unreachable in the
factory-default configuration, and nothing said why. Of the two options
this task posed, configuring a default TSA endpoint was rejected in
favour of failing loudly: SPEC §12.7 does name a "suggested default"
(the Office for IT and eGovernment), but hard-coding a live, government-
operated production endpoint as this project's own default — one this
CLI would silently start contacting on every unconfigured invocation —
is a materially bigger, riskier change than this fix-and-polish pass's
own stated scope, and is exactly the kind of unrequested feature SPEC
§0 warns against inventing to route around an inconvenient constraint.
Failing loudly by default is also simply *true* to what already exists:
`--on-tsa-failure`'s own default is `"abort"`, and this fix makes that
default finally apply to the "no TSA at all" case the same way it
already applied to "TSA contacted, then failed" — it completes an
already-half-implemented invariant rather than inventing a new one.
Six existing CLI tests (`internal/cli/sign_test.go`) that signed without
configuring a TSA and asserted success were updated to pass
`--on-tsa-failure b-b` explicitly, since none of them were actually
testing TSA behaviour (output-suffix naming, overwrite/`--force`, batch
skip-and-continue, the two stamp tests) — each was, incidentally,
relying on the exact silent-degradation bug this task fixes.

**Rejected.**
- **Configuring a default TSA URL** (SPEC §12.7's suggested Office for
  IT and eGovernment endpoint). Rejected for the reasons above: a live
  production dependency contacted by default is a bigger change than
  this task's bounded scope, and is more invasive than the CLI already
  being explicit about what it needs (`--tsa`) — a user who wants B-LT
  by default can already get it with one flag, or a future phase can
  make this a persisted `Settings` value (F5) once that concept exists.
- **Only failing when `RequestedLevel == LevelBLT`**, leaving `B-T`
  requests to silently degrade as before. Rejected: `B-T` requires a
  TSA exactly as much as `B-LT` does (`B-LT` is `B-T` plus `/DSS`), and
  the CLI never lets a caller request `B-B` directly in the first place
  (`parseLevel` only accepts `b-t`/`b-lt`) — there is no requested level
  this fix should exempt.

---

## D-068 — The Trusted List staleness warning is measured from the last successful fetch, not the list's own issue date

**Date:** 2026-09-01
**Phase:** F4/F5 boundary — fix-and-polish pass (Task 9)

**Decision.** `internal/cli.renderTSL`'s staleness warning now calls
`tslNeedsStaleWarning(prov, now)`, which returns true when
`prov.Source == tsl.SourceEmbedded` (no fetch has ever succeeded at
all) or when `now.Sub(prov.FetchedAt) > staleWarningAge`. It no longer
compares `now` against `prov.IssuedAt`. The heading line
(`certs.tsl_heading`, "issued %s (%d days old, %s)") is unchanged and
still reports the published list's own age for information — that
figure was never wrong, only the warning that used to key off it was.
All three catalogues' `certs.tsl_stale_warning` text was reworded from
"this list is more than 30 days old" to "the trusted list has not been
successfully refreshed from the network in the last 30 days," so the
warning's own wording matches what it now actually measures.

**Why.** The reported output — "izdata 2026-05-20 (stara 104 dana,
upravo osvežena)" immediately followed by "upozorenje: ova lista je
starija od 30 dana" — contradicts itself: a list that was *just
refreshed* was, in the same breath, reported stale. The Ministry
publishes a new Trusted List only when something in it changes (SPEC
§11.1 says nothing about a refresh cadence tied to a fixed calendar), so
a list issued 104 days ago can easily still be the current, correct
one; its *issue* date says nothing about whether this agent can still
reach the Ministry. What the warning is actually meant to protect
against — this agent silently running on stale trust data because it
cannot reach the network — is exactly what `Provenance.FetchedAt`
already recorded on every path (`SourceCache`'s `FetchedAt` is the
cache file's own mtime, set at the moment of its last successful
`Refresh`; `SourceNetwork`'s is `time.Now()` at the moment of that
call) without needing a new field. `SourceEmbedded` is the one case
with no `FetchedAt` set at all — this agent has never successfully
fetched anything, running entirely on the seed bundled at build time —
which this task's own wording calls out as needing a warning
unconditionally, independent of the seed's own issue date.
`TestRenderTextNoStaleWarningWhenRecentlyFetchedDespiteOldIssueDate`
reproduces the task's own reported example (`IssuedAt` 104 days in the
past, `FetchedAt` now) and asserts no warning; run against the code as
it stood before this fix, it failed, warning exactly as the bug
report describes.
`TestRenderTextStaleWarningOnEmbeddedSeedRegardlessOfIssueDate` likewise
failed before the fix (an embedded seed with a recent `IssuedAt` produced
no warning) and passes after.

**Rejected.**
- **Dropping the "days old" figure from the heading entirely**, since
  it is not what the warning measures. Rejected: the figure itself was
  never inaccurate or the source of the contradiction — it truthfully
  reports how old the published list is, which is legitimate
  information for a technical user — only its use to *decide the
  warning* was wrong. Removing true, requested information (SPEC §11.1
  requires displaying the list's age) to avoid a warning-logic bug in a
  different part of the same function would be fixing the wrong thing.
- **Warning whenever `Source != SourceNetwork`** (i.e. the moment a
  list did not come from *this run's own* fetch), instead of comparing
  `FetchedAt` against a threshold. Rejected: this would warn on every
  single invocation that reuses a same-day cached list from an earlier
  successful refresh — `SourceCache` is the normal, expected state for
  most invocations (the agent does not re-fetch on every `certs` call)
  — which is exactly the kind of noisy, uninformative warning this task
  is asking to eliminate, not multiply.

---

## D-069 — `/ByteRange`'s real values are written compact and left-aligned, all padding moved after the last number

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — second fix-and-polish pass (Task 1)

**Decision.** `internal/pades/pdf.writeByteRangeValues` no longer
formats each of the four `/ByteRange` numbers into its own fixed-width,
right-aligned field (`%*d`, giving each number up to nine leading
spaces of its own). It now renders all four numbers together as
`"%d %d %d %d"` — real digits, single-space separators, nothing else —
and pads only the far end of the fixed span (the new
`byteRangeTotalWidth` constant, `4*byteRangeFieldWidth + 3`, unchanged
from what the placeholder already reserved) with spaces after the
fourth number. A `panic` guards the case where the compact form would
not fit — unreachable in practice, since `byteRangeFieldWidth` (10
digits) was already sized in [[D-042]] for files far larger than
`MaxInputSize`, but SPEC §0 asks that a constraint like this be guarded
rather than silently trusted.

**Why.** Independent analysis of a real document (`testdata/pdfs/local/mup.pdf`,
signed with the soft token at B-LT with a real Pošta test-TSA
timestamp) reproduced Adobe Acrobat's exact failure: "At least one
signature is invalid", empty Signature Panel, "Unexpected byte range
values defining scope of signed data." Everything cryptographic was
independently confirmed correct first — `/ByteRange` arithmetic,
`messageDigest`, both pre-existing signatures in the document, the DSS
revision's structure, `pypdf` and PDFium parsing cleanly — narrowing
the fault to rendering alone. The one structural difference from every
reference implementation (the document's own MUP-produced signature,
iText, PDFBox, pyHanko, and Adobe itself) was exactly this: they all
write `/ByteRange [0 603053 668591 615                        ]` —
numbers packed together, padding trailing at the end — while
[[D-042]]'s right-aligned-per-field rendering produced
`/ByteRange [         0     603053     668591        615]`. Adobe does
not read `/ByteRange` through its general object parser; PDF 32000-1
§7.7.5 requires signature handlers to scan it from raw bytes before the
document is otherwise resolved (so that the exact signed byte span is
known before anything else is trusted), and that raw scanner is strict
about the shape real tools produce. After the fix, signing the same
real fixture the same way (soft token, B-LT, real Pošta timestamp)
produces `/ByteRange [0 582950 648488 572                        ]` —
43 bytes, matching the placeholder's original width exactly — and all
three signature slots in the output (the document's original CAdES
signature, its original document timestamp, and this project's new
signature) independently re-verify (`ByteRangeDigestOK`/`SignatureOK`/
`SigningCertificateOK` all true for the two PAdES signatures; the
document-timestamp slot's `ByteRangeDigestOK=false` is expected and
already documented in `internal/pades/verify/real_test.go`'s
`mainSignatureSlot` helper — its own `messageDigest` covers the
embedded TSTInfo, not this document's `/ByteRange`). The new
`TestPlaceholderByteRangeIsLeftAlignedForAdobesRawScanner` in
`internal/pades/pdf/placeholder_test.go` pins the exact byte shape —
compact numbers, single-space separators, no run of two or more spaces
before the fourth number — so this cannot silently regress back to
per-field padding. `testdata/golden/minimal-signed-bb.pdf` was
regenerated (`go test ./internal/pades/ -run TestGoldenFile -update`):
its total byte length is unchanged (9369 bytes both before and after,
since [[D-042]]'s length-neutrality property is exactly what this fix
preserves), only the `/ByteRange` field's own bytes differ.

**Rejected.**
- **Reducing `byteRangeFieldWidth` instead of changing the alignment.**
  Would not have helped: the failure was the *shape* of the padding
  (leading spaces before three of the four numbers), not the total
  field width, which [[D-042]] had already sized correctly and which
  this fix deliberately leaves unchanged — every offset after
  `/ByteRange` in the file depends on that width staying fixed.
- **Guessing at further changes without a comparison against a
  known-good signer.** The task that produced this entry explicitly
  asked for exactly that discipline: confirm this specific fix against
  the concrete evidence gathered (Adobe's own reference format,
  reproduced byte-for-byte) rather than iterating blindly if it had not
  worked. It did work — verified directly against the real fixture, not
  only against this project's own synthetic test.

---

## D-070 — The real Liro logo (256×256, turquoise `#038387`) replaces the generated placeholder mark; [[D-061]] superseded

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — second fix-and-polish pass (Task 2)

**Decision.** `assets/signature-logo.js` — the real Liro mark exported
from the previous Bridge SDK, `256×256`, turquoise `#038387`, as raw
RGB and 8-bit-alpha pixel data already zlib-compressed (RFC 1950, the
form PDF's `/FlateDecode` filter and `/SMask` require) and base64-
encoded — is committed as the source of truth. `scripts/genlogo` no
longer draws anything: it reads that file, extracts and base64-decodes
`rgbFlateBase64`/`alphaFlateBase64`, sanity-checks (by decompressing,
read-only) that each decodes to exactly `256×256×3` and `256×256`
bytes, and writes the still-compressed bytes straight to
`internal/pades/appearance/logo-rgb.flate` and `logo-alpha.flate` — no
re-encoding, resampling or colour conversion anywhere in the path.
`internal/pades/appearance.LogoImagePixels` (256) is a new constant,
used for the image XObjects' `/Width`/`/Height` in `logo.go`;
`LogoSize` (36, unchanged) still governs only the stamp's placement —
the `cm` matrix that scales *any* image into a 36×36pt box in the
content stream (`stamp.go`'s `buildContentStream`), which is exactly
why a 256px source needs no resampling to be placed correctly at 36pt.

**Why.** [[D-061]] recorded, correctly at the time, that this project
had no access to a real Liro Design System asset and built a
structurally-correct synthetic stand-in (a disc with a checkmark)
rather than inventing something that looked official but was not. That
constraint no longer holds: the real asset, previously exported from
the Bridge SDK for exactly this purpose, is now available, and [[D-061]]
itself named "asking for a real asset" as the preferred path once one
existed. `TestLogoAssetIs256x256WithAlphaApplied`
(`internal/pades/appearance/logo_test.go`) decompresses both committed
`.flate` files directly and checks: the pixel counts match `256×256`
exactly; the alpha channel has both fully-opaque and fully-transparent
pixels (proving `/SMask` has something to mask, not a uniform fill);
and at least one fully-opaque pixel is exactly `#038387`, the specified
brand colour — not merely the right size. `TestLogoXObjectDeclaresRealPixelDimensions`
(`internal/pades/appearance/stamp_test.go`) renders a stamp end to end
through `Render`, re-parses the output, and confirms both `/Image`
XObjects (the RGB image and its `/SMask`) declare `/Width 256 /Height 256`
— what the runtime path actually writes, not just the source bytes.
Signing a real fixture with `--stamp` (`testdata/pdfs/local/mup.pdf`,
soft token, B-T) and inspecting the output directly confirms the same:
both image objects declare `256`/`256`, and every signature slot in the
result still independently verifies — the stamp's presence does not
touch the signed byte range, matching F4 §6's separation.

**Rejected.**
- **Resampling or re-encoding the asset to 36×36 to match `LogoSize`.**
  Explicitly ruled out by the task supplying this asset: "No
  re-encoding, no resampling, no colour conversion — the data is
  already in the target format." It is also unnecessary — a PDF image
  XObject is always mapped into the unit square by the content stream's
  `cm` matrix (already how `LogoSize` places the logo, see `stamp.go`),
  so the asset's pixel resolution and its placement size in points are
  independent by construction; conflating them (as the old placeholder
  did, being authored 1:1 at 36×36) was incidental, not required.
- **Keeping `LogoSize` as both the placement size and the image pixel
  size, and hard-coding `256` only where `logo.go` needs it.** Rejected
  in favour of a named `LogoImagePixels` constant: a bare `256` at the
  two call sites would read as another instance of the same placement
  geometry `LogoSize` already names, inviting exactly the conflation
  this decision's own reasoning above warns against.
- **Generating the `.flate` files at test time from the JS source,
  rather than committing them.** Rejected: `scripts/genlogo`'s existing
  contract (F4 §4, [[D-061]]) is "pre-compressed at build time,
  embedded via `//go:embed`" — the runtime path must never encode or
  decode image data — and the committed `.flate` files are what
  `//go:embed` actually reads; committing them (not just the JS source)
  is what keeps that contract true, exactly as it already was for the
  placeholder mark.

---

## D-071 — `testdata/pdfs/blank.pdf` is a committed, script-generated fixture with no pre-existing AcroForm

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — third fix-and-polish pass (Task 1)

**Decision.** `scripts/genblankpdf` writes `testdata/pdfs/blank.pdf`: a
single A4 page (595×842pt), empty `/Resources`, no `/Contents`, no
`/AcroForm`, no existing signature, classic xref, and a fixed `/ID`
derived from `sha256("liro-bridge-blank-fixture-v1")[:16]` — deterministic
so re-running the generator reproduces the committed file byte for byte.
Unlike `testdata/pdfs/local/*.pdf` (real signed documents, personal
data, never committed — see that directory's README), this fixture is
small (427 bytes unsigned) and contains nothing this project did not
write itself, so it is committed directly, the way
`internal/pades/golden_test.go`'s in-memory minimal PDF already is —
except this one is a real file on disk other tests and manual
inspection can reach without running Go code first.

**Why.** Task 1 asked for exactly this: a minimal, reproducible
fixture that signs to "roughly 40KB rather than 670KB" so future
diagnosis of writer-level bugs is fast, and — its stated second purpose
— a discriminator for Task 2's fix. A document with **no** pre-existing
`/AcroForm` forces every AcroForm key in the signed output (`/DA` is
absent entirely; `/Fields`, `/SigFlags`, the widget's `/T`) to be
something this project's own code created, not something copied forward
from a real producer's own bytes. That isolation matters because the
bug Task 2 fixes only manifests in code this project's own writer
executes — a test against a real fixture (which already carries a
correctly-formed `/DA` and `/Lang` from its original producer) cannot
tell the difference between "this project preserved the original
formatting" and "this project's own writer independently produces
correct formatting," since both would look identical from the outside
if the incremental-update path never re-serialised those bytes at all.
Signing this fixture (`TestSignBlankFixtureProducesSmallVerifyingOutput`,
`internal/pades/blank_test.go`) measured 66714 bytes with the default
`pdf.DefaultReservedBytes` (32768 raw bytes — 65536 hex characters
alone, [[D-042]]) and a self-signed test certificate with no chain and
no TSA — well under an order of magnitude below a real fixture's
~670KB, though not the literal "~40KB" the task's own motivating
sentence estimated; that estimate assumed a smaller reservation than
this project's actual default, and the real, load-bearing point it was
making — fast local diagnosis instead of picking through a 670KB file —
holds regardless of which exact number the default reservation lands
on. Recorded here rather than silently rounded to fit, per SPEC §0.

The `/ID` value is deliberately derived from a hash rather than typed
out as memorable hex digits: an early draft used
`hex("LiroBridgeBlank")`, which decodes to printable ASCII and was
therefore indistinguishable, by content, from ordinary text — exactly
the property [[D-072]]'s content-based hex/literal decision keys off of.
A hash's output bytes are not printable, so this fixture's `/ID` is
genuinely binary the same way a real producer's MD5-derived `/ID` is,
letting `TestSignBlankFixturePreservesIDAsHexString` exercise the "hex
is still correct for genuinely binary content" half of [[D-072]] without
special-casing the `/ID` key anywhere in the fixture generator or the
test.

**Rejected.**
- **A hand-typed binary literal pasted into the repository.** Explicitly
  ruled out by the task: "generated by a script so it is reproducible,
  not a binary someone pasted in." A script that can be re-run and diffed
  against the committed output is itself part of the fixture's value.
- **Reusing `internal/pades/sign_test.go`'s `buildMinimalPDF`/
  `internal/pades/golden_test.go`'s `goldenMinimalPDF` in-memory
  builders instead of a new committed file.** Considered, since both
  already build a small synthetic PDF with no AcroForm. Rejected because
  the task specifically asked for a *committed* fixture at
  `testdata/pdfs/blank.pdf` — a real file other tests, a future golden
  file, or a human with a PDF viewer can reach directly, not bytes that
  only exist inside one Go test function's closure.
- **Typing the /ID hex digits out as a readable ASCII string
  ("LiroBridgeBlank").** Tried first; rejected once it defeated its own
  test — see "Why" above.

---

## D-072 — PDF strings are written as literal `(...)` by default; hex is reserved for content that is not printable ASCII text

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — third fix-and-polish pass (Task 2)

**Decision.** `internal/pades/pdf.writeObject`'s `String` case now
writes a literal `(...)` string — `(`, `)` and `\` backslash-escaped,
`\n`/`\r`/`\t` escaped the same way, every other byte outside printable
ASCII (0x20–0x7E) as a `\ddd` octal escape (PDF 32000-1 §7.3.4.2) — for
any string whose bytes are all printable ASCII plus `\n`/`\r`/`\t`
(`isBinaryString` returns false). A string containing any other byte is
still written as a hex `<...>` string, exactly as before. The decision
is made from the string's own content, in one place
(`internal/pades/pdf/write.go`), for every string this project writes —
both ones it constructs itself (a signature widget's `/T`) and ones
copied forward unmodified from a parsed document during an incremental
update (an existing AcroForm's `/DA`, a catalog's `/Lang` — F3 §3.3:
"copy each object being modified in full").

**Why.** Independent, from-scratch analysis of a real, already-verified-
correct document (ByteRange arithmetic, `messageDigest`, all three
signature slots, pyHanko's `SignatureCoverageLevel.ENTIRE_REVISION` for
every signature — SPEC §16.8, added by this same task, records the full
validator picture) had already ruled out every cryptographic and
structural cause, including [[D-069]]'s `/ByteRange` alignment fix,
which did not by itself resolve Adobe's "At least one signature is
invalid." The one remaining byte-level divergence from every real
producer (the document's own MUP-produced signature, iText, PDFBox,
pyHanko) was that this project's writer hex-encoded *every* PDF string
unconditionally (the previous doc comment on `writeObject` said so
directly: "Strings are always written as hex... hex encoding has no
escaping edge cases"), while every reference implementation writes text
fields as literal strings. Two of those fields are not ordinary text to
Acrobat: `/DA` is executed as a content-stream fragment when Acrobat
builds a field's appearance, and `/T` is read while Acrobat assembles
the AcroForm field tree. A failure in either explains the observed
symptom exactly — an empty Signature Panel, meaning Acrobat's own field-
tree/appearance code, not its cryptographic verifier, is what is
rejecting the document. `internal/pades/pdf/write_test.go` pins the
literal encoding, its escaping, a full 0–255 byte round trip through the
real parser, and the binary/hex content boundary directly.
`internal/pades/literal_strings_real_test.go`
(`TestRealFixturesFormStringsAreLiteral`) confirms the fix against all
three real fixtures' actual output — scoped specifically to
`result.Bytes[len(in):]`, the newly appended revision, because a naive
whole-file check would have passed even with the bug still present: the
*original* document's own untouched bytes (still present as the file's
unmodified prefix, F3 §3.1) already write `/DA` and `/Lang` literally on
their own. The measured, byte-exact new AcroForm dictionary this
project itself now writes for mup.pdf is
`<</DA (/Helv 0 Tf 0 g )/DR 2066 0 R/Fields [2053 0 R 2065 0 R 2073 0 R]/SigFlags 3>>`
and the new widget is
`<</F 4/FT /Sig/P 3 0 R/Rect [0 0 0 0]/Subtype /Widget/T (Liro-Signature-1)/Type /Annot/V 2072 0 R>>`
— both literal throughout, matching the original document's own
formatting for the keys it carried forward. `testdata/golden/minimal-signed-bb.pdf`
was regenerated (`go test ./internal/pades/ -run TestGoldenFile -update`):
its total length changed from 9369 to 9353 bytes (the field name
`Liro-Signature-1`, hex-encoded, cost 36 bytes; literal, it costs 19),
confirming the fix is length-*neutral in kind* but not
length-*identical* — exactly the class of change SPEC §16.2's golden
test exists to catch and this task explicitly authorised regenerating
deliberately.

Whether this is Acrobat's actual, complete objection is not something
this project can confirm without Acrobat itself, which is not available
in this environment (SPEC §16.7 already documents that manual
acceptance against real applications is checked before release, not
during development). It is recorded here, plainly, as this task's
instructions required: this is the one remaining byte-level divergence
from every reference producer that this analysis found, it is now
fixed, and if it turns out not to be Acrobat's whole objection, the next
step is a byte-level diff of this project's output against a document
signed by a known-good tool — not another guess.

**Rejected.**
- **Threading a per-key "this one is binary" flag through every caller
  that builds a `Dict`.** Would work, but requires every call site that
  writes `/ID` (currently one: `internal/pades/pdf/incremental.go`'s
  `updateTrailer`) to remember to set it, and silently produces the old,
  wrong behaviour for any future binary-content key that forgets to. A
  content-based decision, made once in the writer, cannot be forgotten
  at a call site because there is no per-call-site decision to make.
- **Always writing literal strings, with no hex case at all.** Rejected:
  `/ID` values are raw digest bytes, and every reference implementation
  this project compared against (including the document's own original
  producer) writes that specific kind of content as hex, not as a string
  full of `\ddd` octal escapes. [[D-071]]'s fixture-generator note
  explains the concrete test this content-based choice is verified
  against.
- **Recording, in the parsed `String` type itself, which syntax
  produced it, and re-emitting parsed strings in their original form
  unconditionally.** This is what the task's own wording called "better
  still," and it was considered: threading a `wasHex bool` (or a
  `literal`/`hex` enum) through `internal/pades/pdf.String` would let a
  string parsed from hex round-trip as hex even if it happens to look
  like text, and vice versa. Rejected for this pass because it changes
  the shape of `String` itself — every constructor and every comparison
  (`String(fieldName)`, `dict.Get(Name("T")).(String)`, and so on,
  throughout `internal/pades`) would need to either carry the new field
  or default it correctly — which is a broader change than this task's
  "do not restructure anything" instruction permits for a fix whose
  content-based version already produces byte-identical output to what
  origin-preservation would have produced for every case this project's
  own real fixtures and golden test actually exercise (a parsed literal
  string stays literal because it is printable text; a parsed hex `/ID`
  stays hex because it is not). If a future real document is found whose
  original producer hex-encoded a printable-ASCII string for its own
  reasons, this decision's content-based choice would re-emit it as
  literal instead of preserving the original hex form — a real,
  narrower gap than the one this decision closes, and one to fix when
  and if a real document actually exhibits it, not preemptively.

---

## D-073 — SPEC §16.8 records what each validator actually proves, and that four independent implementations accepted a document Adobe rejected

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — third fix-and-polish pass (Task 3)

**Decision.** SPEC gained §16.8, "The validator landscape": a table
naming what each of the eUprava validator, DSS Demo, PKS, Inception,
pyHanko and Adobe Acrobat actually establishes, plus a paragraph
recording the specific measured case that motivated it — pyHanko,
pypdf, PDFium and this project's own independent verifier all accepted
a document Adobe Acrobat rejected, and the actual fault ([[D-069]],
[[D-072]]) was real but specific to two things Adobe's own parser does
that PDF 32000-1 permits but does not require (a raw-byte `/ByteRange`
scanner ahead of general parsing; executing `/DA` and reading `/T` while
assembling the field tree), not evidence that Adobe was simply wrong.

**Why.** This task's own diagnosis needed exactly this distinction
twice in a row: [[D-069]]'s `/ByteRange` alignment fix was validated
against Adobe's own documented raw-byte scanning behaviour specifically,
*not* against pyHanko's acceptance (pyHanko already accepted the
misaligned form); this task's own string-encoding fix needed the same
move, because pyHanko, pypdf, PDFium and this project's own verifier all
already accepted the hex-encoded `/DA`/`/T` before this fix existed —
none of them execute or structurally interpret those two keys the way
Adobe's field-tree and appearance code does. Without a durable record of
*why* four acceptances did not mean the document was actually fine, the
natural next engineering instinct — "four independent tools accept this,
the fifth must be an outlier, simplify the formatting back" — would
silently reintroduce the exact defect this task fixed twice. SPEC §16.7
already names Adobe's AATL trust-list gap as a known, undocumented-as-a-
bug divergence; §16.8 is the general version of that same discipline,
covering every validator this project checks against, not only Adobe.

**Rejected.**
- **Recording this only in `docs/decisions.md`.** Decisions are
  append-only history of what was tried; SPEC is "the rules that never
  change" (§0) and is what a future phase is required to read in full
  before starting. A rule this load-bearing — do not let validator
  acceptance count as proof against Adobe's stricter behaviour — belongs
  where SPEC §0's own instruction guarantees it gets read again, not
  only in a decision entry a future reader might not scroll back to.
- **Ranking the validators by authority instead of describing what each
  proves.** Considered, since the eUprava validator is explicitly named
  as "more authoritative... than Adobe" for legal purposes. Rejected as
  the organising principle for the whole table: authority is the right
  frame for the eUprava/PKS/Inception row specifically (legal validity
  is what matters for this project's users), but Adobe's row is not
  about legal authority at all — it is "what the user actually sees,"
  which is a different and equally real reason it stays in the list
  regardless of its legal standing.

---

## D-074 — Incremental update preserves a modified object's direct/indirect structure; a catalog's direct /AcroForm is no longer promoted to a new indirect object

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — fourth fix-and-polish pass

**Decision.** `internal/pades/pdf.BuildPlaceholder` no longer promotes a
direct (inline) `/AcroForm` dictionary in the catalog to a new indirect
object. When the catalog's own `/AcroForm` entry is a direct `Dict`, the
rewritten catalog keeps it direct, with `/Fields` extended in place to
include the new signature widget alongside whatever fields the document
already had; no standalone `AcroForm` object number is allocated for
that case. When `/AcroForm` was already an indirect reference, or absent
entirely, behaviour is unchanged from before this fix: an existing
indirect object is redefined under its own number ([[D-038]]'s "modifying
an object means appending a full copy under its existing number, never
editing the earlier bytes" rule, F3 §3.3), and a document with no
`/AcroForm` at all still gets a freshly created indirect one, exactly as
`testdata/pdfs/blank.pdf` continues to exercise
(`TestSignBlankFixtureAcroFormIsIndirect`,
`TestSignBlankFixtureProducesSmallVerifyingOutput`). `/Annots` on the
signed page was never affected by this bug: `doc.Resolve(...).(Array)`
already resolves whichever shape the original held and writes the result
back as a direct array in the page's own rewritten copy, matching every
real fixture's own direct-array shape — this fix brings `/AcroForm` in
line with a rule `/Annots` already followed correctly, rather than
inventing a new one.

**Why.** Adobe Acrobat showed an empty Signature Panel — "At least one
signature is invalid" — for every one of the three real, already-signed
fixtures this project re-signs, while accepting a signed
`testdata/pdfs/blank.pdf`. Direct, byte-level inspection of each
fixture's own original catalog object found an exact pattern:

| Fixture | Original `/AcroForm` in the catalog | This project emitted | Acrobat |
|---|---|---|---|
| halcom.pdf | direct dictionary | `43 0 R` (new indirect object) | rejects |
| mup.pdf | direct dictionary | `2074 0 R` (new indirect object) | rejects |
| posta.pdf | direct dictionary | `122 0 R` (new indirect object) | rejects |
| blank.pdf | no `/AcroForm` at all | `6 0 R` (new indirect object, nothing to lift) | accepts |

All three real documents carry `/AcroForm` as a direct dictionary
embedded in the catalog — measured directly, not assumed — and the
`case Dict:` branch `BuildPlaceholder` had used since F3 explicitly
promoted it: "An inline (non-indirect) `/AcroForm` is unusual but
legal; promote it to an indirect object like every other object this
update modifies (F3 §3.3: never edit in place)." That comment's
premise was wrong for this specific key: F3 §3.3's "never edit in
place" rule is about not mutating the *bytes of an existing object in
place* — it says nothing about changing an object's *identity* between
direct and indirect, and doing so here is itself an edit the previous
signer never made. blank.pdf has no pre-existing `/AcroForm` to lift, so
it was never exposed to this and was the only one of the four fixtures
Acrobat accepted, which is what made it useless as a discriminator for
this specific bug even though [[D-071]] built it to discriminate a
different one.

Every other structural property of the rewritten catalog, AcroForm and
page objects had already been ruled out by the same byte-level analysis,
against halcom.pdf specifically: the original bytes are an exact literal
prefix; every xref offset resolves correctly; the `/Prev` chain and
`startxref` are correct; classic xref entries are exactly 20 bytes; the
trailer preserves the original `/ID`'s first element; the catalog,
AcroForm and page objects lose no keys and are semantically identical
apart from the intended additions; `/DA`, `/T` and `/Lang` are literal
strings matching the originals ([[D-072]]); `/ByteRange` is compact and
arithmetically correct ([[D-069]]); pyHanko reports every signature as
`SignatureCoverageLevel.ENTIRE_REVISION`, with only `FORM_FILLING` and
`LTA_UPDATES` differences between revisions. The one remaining
divergence was `/AcroForm`'s identity itself. For a document that
already contains a signature, Acrobat's incremental-update analysis
compares what the new revision changed against the previous one — not
merely what the final, fully-resolved document now contains. An
`/AcroForm` that was inline before and a separate indirect object after
is not legible to that comparison as "one field added to the existing
form"; it reads as the entire form having been replaced, which is
consistent with SPEC §16.8's own explanation of why pyHanko, pypdf and
PDFium (all of which resolve final state, none of which diff revisions)
accepted the same bytes Acrobat rejected.

`TestPlaceholderPreservesDirectAcroForm`
(`internal/pades/pdf/placeholder_test.go`, against a new synthetic
fixture, `buildClassicFixtureWithDirectAcroForm`, so this is covered
independently of the gitignored real fixtures) and
`TestRealFixturesAcroFormDirectnessMatchesInput`
(`internal/pades/acroform_direct_real_test.go`) pin the fix: for each of
the three real fixtures, direct/indirect in the output matches
direct/indirect in the input; when direct, the new revision's own
catalog object bytes contain `/AcroForm <<...>>` inline with `/Fields`
extended by exactly one entry over the input; the original bytes remain
a literal prefix; and both the original signature and the new one still
fully verify. The measured, byte-exact new catalog object for a signed
halcom.pdf is:

```
1 0 obj
<</AcroForm <</DA (/Helv 0 Tf 0 g )/DR 36 0 R/Fields [24 0 R 35 0 R 42 0 R]/SigFlags 3>>/Extensions <</ADBE <</BaseVersion /1.7/ExtensionLevel 8>>>>/OutputIntents [23 0 R]/Pages 2 0 R/Type /Catalog>>
endobj
```

`/AcroForm` is inline, `/Fields` carries the document's two pre-existing
fields plus this project's new widget as its third entry, and no fourth
object number was spent on a standalone AcroForm object.
`TestSignBlankFixtureAcroFormIsIndirect` keeps the blank fixture's
existing, correct behaviour pinned: with no `/AcroForm` to preserve the
shape of, a fresh indirect object is still created exactly as before.

**A general principle, applied narrowly here.** An incremental update
should not change whether a value is direct or indirect where doing so
costs nothing — that is a structural change to something the previous
signer signed over, and it is exactly the kind of change a
signature-aware analyser like Acrobat's inspects. This fix applies that
principle only to `/AcroForm`, the one place this project's own code
violated it; `/Annots` already followed it correctly (see "Decision"
above), and no other object this update touches (the page, the widget,
the signature dictionary) had the same problem, so no other call site
needed changing. This is a narrow, one-key fix, not a rewrite of the
incremental-update engine.

**A test-scoping trap, recorded again because it bit twice.** [[D-072]]
already recorded this once for the string-encoding fix, and the same
trap applies here with equal force: an assertion checked against the
*whole* signed file can pass even when this project's own writer is
still wrong, because a real fixture's *original, untouched* bytes
already contain a perfectly correct `/AcroForm`, `/DA`, `/Lang` and so
on — they were written by the document's genuine original producer, not
by this project. `TestRealFixturesAcroFormDirectnessMatchesInput` scopes
its inline-dictionary check to `result.Bytes[len(in):]` specifically
(the newly appended revision) for exactly this reason: checking the
whole file for `/AcroForm <<` would have passed even with the promotion
bug still in place, because the *original* halcom.pdf/mup.pdf/posta.pdf
catalog objects (still present, byte-for-byte, in the unmodified prefix)
already contain their own inline `/AcroForm`. Every test written against
a real fixture in this project must scope its assertions to the bytes
this project's own code newly wrote, never to the file as a whole — the
whole-file version proves nothing about what this project's writer just
did.

**Rejected.**
- **Threading a "was this direct or indirect" flag through every object
  this update might touch, generically.** Considered, since the general
  principle above applies beyond `/AcroForm` in theory. Rejected because
  no other object this update writes is ever direct in a real fixture
  today (`/Annots` is handled correctly already by construction, not by
  a flag — see "Decision"), so a generic mechanism would be untested
  machinery built for a case that does not currently exist. SPEC §0's
  "choose the simplest option" favours the narrow, one-key fix actually
  exercised by real, measured input.
- **Rewriting the catalog and AcroForm structural logic in
  `BuildPlaceholder` to be shape-preserving in general, rather than
  fixing this one branch.** Explicitly out of scope: the task instructed
  "this one change only," and the byte-level analysis found exactly one
  structural divergence (`/AcroForm`'s identity), not a class of bugs
  needing a general redesign.
- **Leaving the previous `case Dict:` comment's reasoning
  ("promote it to an indirect object like every other object this
  update modifies") uncorrected and only fixing the code.** Rejected:
  that comment's own stated premise is what led to the bug, and a future
  reader hitting the same F3 §3.3 citation without this decision's
  correction could reintroduce the same promotion for the same
  mistaken reason.

## D-075 — Cross-reference mechanism detection now reads only the last revision (via startxref), never the whole `/Prev` chain

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — fifth fix-and-polish pass

**Decision.** `Document.UsesXrefStreams()` (`internal/pades/pdf/document.go`)
no longer answers "does /Type /XRef appear anywhere in the merged
trailer." It resolves the document's final `startxref` directly and
inspects only the bytes found there: the `xref` keyword means the last
revision is a classic table; an indirect object with `/Type /XRef` means
it is a cross-reference stream. This is `xrefMechanismAtStartxref`, a
single lookup at one offset — it does not scan the file and does not
walk `/Prev`. `Update.Apply` was unchanged; it already called
`UsesXrefStreams()` to choose between `appendClassicXref` and
`appendXrefStream` (F3 §3.2), and now gets the right answer from it.

The one path this cannot help is the rebuild-by-scanning fallback
(`rebuildXref`, taken when `startxref` itself is unusable): with no real
revision boundaries left to point at, `UsesXrefStreams()` falls back to
the merged trailer's own `/Type` key, same as before this fix. That
fallback is already a degraded, best-effort path and is not what any of
the three real fixtures exercise.

**Why.** The direct-`/AcroForm` fix ([[D-074]]) made Acrobat accept a
signed halcom.pdf, but it kept rejecting mup.pdf and posta.pdf. Checking
which cross-reference mechanism each original's *last* revision uses —
the one its own `startxref` points at — against what this project
appended, found an exact discriminator:

| Fixture | Last revision (measured) | This project appended | Acrobat |
|---|---|---|---|
| halcom.pdf | classic table | classic table | accepts |
| mup.pdf | classic table | cross-reference stream | rejects |
| posta.pdf | classic table | cross-reference stream | rejects |

At each original's final `startxref` offset the bytes are literally
`xref\n0 2\n0000000000 65535 f\r\n…` in all three documents — a classic
table, full stop. Yet `UsesXrefStreams()` reported `true` for mup.pdf
and posta.pdf. The reason is `mergeTrailer` (`internal/pades/pdf/xref.go`):
it walks the `/Prev` chain newest-first and, for each key, keeps the
first (i.e. newest) definition it finds, but a key a newer section never
defines at all is not cleared — it simply falls through to whatever an
older section sets. mup.pdf and posta.pdf are not "purely stream-based,"
despite `testdata/pdfs/local/README.md`'s description (also corrected by
[[D-039]]): each is a classic-table base revision, one revision that
upgrades to a genuine cross-reference stream (reached via a
backward-compatible hybrid `/XRefStm` pointer, PDF 32000-1 §7.5.8.4), and
two further plain classic-table revisions on top of that. The newest
(last) revision's trailer is a classic `trailer` dictionary, which has no
`/Type` key at all — so the merge, finding no newer definition, let the
middle stream revision's `/Type /XRef` leak through into the merged
trailer untouched. `UsesXrefStreams()` read that merged trailer and
concluded, wrongly, that the document's mechanism was a stream.
halcom.pdf is classic in every one of its three revisions, so it never
had a `/Type` key anywhere to leak, and happened to get the right answer
by accident — which is exactly why this bug survived past [[D-074]]'s
fix undetected.

This is precisely what F3 §3.2 already named: "If the previous revision
used a classic xref table, append a classic table. If it used a
cross-reference stream, append a cross-reference stream. Do not mix. A
classic table appended to a stream-based document is accepted by some
readers and rejected by others, which is the worst kind of bug." The
same reasoning runs in the other direction — a cross-reference stream
appended to a document whose last revision is a classic table — and
Acrobat is one of the readers that rejects it.

**The appended cross-reference streams were themselves well-formed.**
Byte-by-byte inspection of the streams this project wrote for v-mup.pdf
and v-posta.pdf (before this fix) found every entry's offset resolving
to the correct object, an entry for the stream object itself, a correct
`/Prev` chain, and `/Size`, `/W` and `/Index` all internally consistent.
The bug was never in how the stream was built — `appendXrefStream`
(`internal/pades/pdf/incremental.go`) is unchanged by this fix — only in
*whether* it should have been built at all for these two documents. A
correct object in the wrong place is still a bug.

**Mechanism detection must read the last revision, never scan the
file.** This is the second time this project's own measurement of these
three fixtures has been wrong in a way a test then had to correct
([[D-039]] first corrected "mup.pdf and posta.pdf are purely
stream-based" to "mixed, ending in a stream" — itself now further
corrected here to "mixed, ending in a *classic table*"). The lesson
generalises: a `/Prev` chain's mechanism is a per-revision property,
not a per-document one, and any future code asking "what mechanism does
this document use" must resolve one specific revision — almost always
the last one, via `startxref` — rather than aggregating across the
chain the way `mergeTrailer` correctly does for ordinary key/value
content but cannot correctly do for "does this key exist in the newest
section," which is a different question than "what is this key's
value in the newest section that defines it."

**Verification.** `TestRealFixturesCrossReferenceMechanismMatchesMeasurement`
(`internal/pades/pdf/real_test.go`) now also measures, independently of
`Document` (via `measureXrefMechanism`, which was already walking the
chain by hand for the section-count assertions this test already made),
which mechanism the *first* section in the chain — the one `startxref`
resolves to — actually is, and asserts `UsesXrefStreams()` matches that
`lastIsStream` value for all three real fixtures: `false` for all three,
`mergeTrailer`'s leak notwithstanding.
`TestRealFixturesIncrementalUpdatePreservesOriginalBytes` goes one step
further and closes the loop the previous test alone could not: it signs
each real fixture, then resolves the *appended* revision's own
`startxref` (guarded to fall strictly after `len(in)`, so the check is
scoped to bytes this project's own code just wrote — the same
scoping discipline [[D-072]] and [[D-074]] already record, since a
check that tolerated a match anywhere in the whole file would also have
passed with the bug still in place) and confirms its mechanism matches
the input's independently-measured last revision.
`TestIncrementalUpdateMatchesMechanism` (`internal/pades/pdf/incremental_test.go`)
is now table-driven over three synthetic fixtures instead of two:
`buildClassicFixture`, `buildStreamFixture`, and a new
`buildMixedHistoryFixture` (`internal/pades/pdf/fixtures_test.go`) built
byte-by-byte to reproduce mup's and posta's exact shape — a classic base
revision, a middle revision with a genuine cross-reference stream, and a
further classic revision on top, `/Prev`-chained throughout. Because the
real fixtures are gitignored, this synthetic fixture is the only thing
that keeps this exact regression pinned in CI. Signing it asserts the
appended revision is a classic table, matching its last revision, not a
stream. All three cases also re-parse the signed output and confirm
`UsesXrefStreams()` still reports the same mechanism afterward. Every
pre-existing test in `internal/pades` and `internal/pades/pdf` — golden
file included — passed unchanged; the golden fixture is single-revision
and classic throughout, so it was never able to exercise this bug and
did not need regenerating.

**Unresolved.** Whether Acrobat now accepts the re-signed mup.pdf and
posta.pdf has not been confirmed in this pass — that requires opening
the output in actual Acrobat, which this environment cannot do. What is
confirmed, by this project's own independent verifier
(`internal/pades/verify`) and by direct byte inspection of the appended
`startxref`: the appended revision's mechanism now matches the input's
last revision for all three real fixtures, both the pre-existing and
newly-added signatures still fully verify, and the original bytes remain
a byte-for-byte prefix. If Acrobat still rejects either file, the
mechanism mismatch this decision fixes is no longer the explanation, and
the next step is a byte-level diff against the same document signed by a
known-good tool, per the task that produced this fix.

**Rejected.**
- **Clearing trailer keys a newer section doesn't mention, so an older
  section's `/Type` can never leak through `mergeTrailer`.** Considered,
  since it would fix `UsesXrefStreams()` too. Rejected: `mergeTrailer` is
  used for the *whole* merged trailer, which legitimately carries
  forward keys like `/Info` or `/ID` from whichever revision last set
  them even when a newer revision's trailer doesn't repeat them — that
  carry-forward is correct merge behaviour for those keys and is exactly
  what F3 §2.1 asks for. `/Type` is not like `/Info` or `/ID`: its
  presence or absence is itself the signal, not just its value when
  present. Special-casing `/Type` inside `mergeTrailer` would fix this
  one symptom while leaving the general trap (a per-revision existence
  question answered by a per-document aggregate) ready to bite the next
  key with the same shape. Resolving the mechanism directly from
  `startxref`, independent of the merged trailer entirely, is the
  narrower and more honest fix — it answers the actual question asked
  ("what did the last revision do") instead of patching the aggregate to
  approximate it.
- **Scanning the whole file for the last `xref`/`/Type /XRef` occurrence
  by byte position, rather than resolving `startxref`.** Rejected: the
  task explicitly forbids it, and it is also simply the wrong question —
  a later byte offset is not guaranteed to be the newest revision in a
  file with unusual object placement, whereas `startxref` is the
  document's own, authoritative statement of where its last revision
  begins.
- **Storing the mechanism as a field on `Document`, computed once during
  `Parse`.** Considered for the minor efficiency of not re-finding
  `startxref` on every `UsesXrefStreams()` call. Rejected as unnecessary
  complexity: `Update.Apply` already calls `findStartxref` a second time
  independently for its own `/Prev` chaining, so the document was never
  free of this small redundant scan, and `UsesXrefStreams()` is called at
  most a handful of times per signing operation — not a hot path SPEC §0
  would ask to optimise pre-emptively.

---

## D-076 — B-LT caps embedded revocation evidence at 5 MB per artefact; MUP's OCSP responder is unreachable at the network level, confirmed independently of this project's client

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — sixth fix-and-polish pass (Task 1)

**Decision.** `internal/pades/dss.DefaultMaxArtefactSize` (5 MB, `5 *
1024 * 1024`) caps how large a single OCSP response or CRL may be before
`CollectRevocation` will hand it back for embedding into `/DSS`.
`CollectRevocation` now takes a `maxArtefactSize int64` parameter (zero
or negative means the default); `fetchOCSP`/`fetchCRL` still download and
parse the artefact exactly as before, but return `(nil, skippedSize)`
instead of the bytes once `skippedSize > maxArtefactSize` — the size
check happens after successful validation, not instead of it, so a
genuinely malformed response is still rejected as it always was.
`dss.Entry` gained `TooLarge bool`/`SkippedBytes int64`; `dss.Result`
gained the same two fields (`TooLarge`/`LargestSkippedBytes`), computed
in `Apply` from the entries. `internal/pades.Options` gained
`MaxRevocationArtefactSize int64`, threaded through to
`CollectRevocation`; `internal/pades.Result` gained
`RevocationTooLarge`/`LargestSkippedBytes`, set in `applyDSS` when
`dssResult.TooLarge`. The CLI's `sign` command gained
`--max-revocation-size` (bytes; 0 means the built-in default).

**Why — the measurement.** MUP's end-entity certificates carry
`http://ocsp.mup.gov.rs/MUPGradjaniCAocsp` in their AIA extension (SPEC
§11.9). Probed directly, independent of this project's own HTTP client,
from this development environment (which does have working general
internet access — confirmed by `https://www.google.com` returning `200`
in 0.38s, and by DNS resolving `ocsp.mup.gov.rs` to `195.222.96.163`
without trouble): a raw TCP connection to `ocsp.mup.gov.rs` on **port 80
times out** after 10s, 15s and 20s across repeated attempts (`curl -v`
and a bare `/dev/tcp` connect both agree), and the same host on **port
443 also times out**. In the same session, a TCP connection to
`ca.mup.gov.rs:80` — the CRL host on the same domain — **succeeds
immediately** (`exit=0`), and `curl -L` following its redirect to
`http://crl.mup.gov.rs/MUPGradjaniCA4.crl` downloaded the CRL
successfully. **The OCSP responder does not accept a connection at all;
this is not an HTTP-level or request-shape problem this project's
client could have avoided** — no TCP handshake completes, so no HTTP
request this project or any other client sent could ever have reached
it. `internal/pades/dss.fetchOCSP`'s existing behaviour (two attempts,
10s timeout each, per SPEC/F3 §7.2) is therefore already correct: it
times out and falls back to the CRL exactly as designed. **The size cap
below is the real fix for the oversized-output symptom, not a
workaround for a client bug** — there was no client bug to work around.

The downloaded CRL, `http://crl.mup.gov.rs/MUPGradjaniCA4.crl`, measured
**exactly 30,136,214 bytes** — the same figure this task's own report
names, confirming the earlier measurement independently: "the revocation
list for every Serbian identity card" is not a loose description, it is
this project's own repeated, reproducible download. Embedding a CRL that
size into `/DSS` produced a **30,813,989-byte PDF** that **Adobe Acrobat
Reader cannot open at all** (confirmed against the real signed document
this task's report describes); the same document signed at `--level b-t`
(no `/DSS`) was **673,111 bytes** and opened normally in Acrobat. The
code that produced the oversized file was not wrong about *what* B-LT
requires — SPEC §12.6 does call for embedding revocation evidence — the
national CRL itself is simply too large a thing to embed in a PDF that a
reader can open, at any correctness level. Capping the artefact size
(rather than, say, refusing MUP certificates B-LT unconditionally) keeps
every other issuer's evidently smaller CRLs and OCSP responses (SPEC
§11.9: Halcom and Pošta both have working OCSP; nothing here has ever
measured either of their CRLs as remotely this size) embeddable, while
making exactly the one measured pathological case degrade honestly
instead of producing an unopenable file.

5 MB was chosen, not a tighter or looser number, because it sits
comfortably above every real OCSP response and any CRL scoped to
something short of "every card the state has ever issued" (Halcom's and
Pošta's own CRLs, per SPEC §11.9's confirmed-working OCSP for both, have
never been observed anywhere near this size), and comfortably below the
point where a `/DSS` dictionary makes a document unusable — the measured
30 MB case is roughly 6× over the cap, leaving margin rather than a
line drawn exactly at the one failure observed.

**Degradation is honest and visible (Task 1c).** When a CRL or OCSP
response is skipped for being too large, `dssResult.TooLarge` is true
(not merely `Complete == false`, which also covers plain unavailability
— a caller needs to tell the two apart, since "MUP's list is enormous"
and "nobody's OCSP answered and there's no CRL URL either" call for
different sentences). `pades.Result.AchievedLevel` in this case is
`LevelBT`, never `LevelBLT` (SPEC §7.3/§18.11: no silent overclaim).
`internal/cli.levelLine` gives `RevocationTooLarge` its own localised
message — `error.stamp_glyph_missing`'s established pattern of a
template filled with a specific fact, not the generic English `Notes`
join every other degradation still uses (`Notes` itself stays
English-only per SPEC §9.2, since it doubles as this package's local/log
output) — added to all three catalogues as `sign.revocation_too_large`:

```
en:      "Revocation data was too large to embed (%s); saved at B-T."
sr-Latn: "Podaci o opozivu su bili preveliki za ugrađivanje (%s); sačuvano na nivou B-T."
sr-Cyrl: "Подаци о опозиву су били превелики за уграђивање (%s); сачувано на нивоу B-T."
```

`internal/cli.formatBytesApprox` renders the size as whole megabytes
(30,136,214 bytes → "29 MB"), matching this task's own example message
shape ("Revocation data was too large to embed (32 MB); saved at
B-T.") closely enough that the wording is traceable directly to the
task's own report.

**Rejected.**
- **Refusing B-LT for MUP certificates unconditionally**, since MUP is
  the one issuer whose CRL is known to be this large. Rejected: MUP's
  OCSP responder being unreachable today does not mean it always will
  be, and a size cap handles that case automatically (once OCSP starts
  answering, B-LT is reached the normal way) without a special case keyed
  on issuer identity, which SPEC §11.3 already warns against building
  fragile heuristics around.
- **Raising `maxCRLSize` (the 64 MB network *download* cap,
  unchanged by this task) instead of adding a separate embedding cap.**
  These are different concerns: the download cap bounds how much this
  project will read off the wire before giving up; the new embedding cap
  bounds what ends up inside a PDF. Conflating them would mean either
  refusing to even measure a large-but-real CRL (if lowered), or
  continuing to embed one exactly as large as the CRL that broke Acrobat
  (if left as the only cap).
- **Truncating the CRL to fit under the cap instead of omitting it
  entirely.** A truncated CRL is not a valid CRL — it would either fail
  to parse (caught by the existing `x509.ParseRevocationList` check,
  which runs before the size check and is unaffected by this task) or,
  worse, silently misrepresent which certificates it actually covers.
  Omitting it and reporting B-T honestly is the only choice consistent
  with SPEC §7.3/§18.11.

---

## D-077 — Hardware presence is determined per certificate, by attempting to open its own key; SPEC §11.10 amended to state this at the granularity it was always meant to cover

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — sixth fix-and-polish pass (Task 2)

**Decision.** `internal/keysource/windowscng` gained
`ncryptConn.probePresence(thumbprintHex string) (present bool, err
error)`, implemented in `conn_windows.go` by reusing `findAndAcquire`'s
own certificate-lookup step (`findCert`, newly factored out so both
functions share it) and then attempting
`CryptAcquireCertificatePrivateKey` — the same call `findAndAcquire`
makes, which resolves to `NCryptOpenKey` for a CNG-backed key, and which
never prompts for a PIN (only `NCryptSignHash` does, F2 §2.3). Unlike
`findAndAcquire`, `probePresence` does not route a failure through
`mapStatus` (the signing-path error table in `errors.go`, unchanged by
this task): it inspects the raw Windows status directly via the new,
pure, platform-independent `isCardAbsentStatus(status uint32) bool`,
which is `true` exactly for `NTE_BAD_KEYSET` (`0x80090016`) and
`SCARD_W_REMOVED_CARD` (`0x80100069`) — this task's own two named codes
— and `false` for everything else, including `NTE_NO_KEY`, which
`mapStatus` already treats as a different situation
(`CERT_NOT_USABLE`). A key opened successfully by the probe is
immediately closed again (`NCryptFreeObject`, only when `fCallerFree`
said this caller owns it, per D-026's already-established rule) rather
than kept open — a presence check is not a signing session.
`windowscng.Source` gained `Presence(ctx, thumbprint)
(bool, error)`, mirroring `Open`'s own `conn`-or-`newConn()` pattern.

`internal/cli.Deps.AnyCardPresent` (a single, machine-wide
`func(ctx) (bool, error)`) is replaced by `Deps.PresenceCheck
func(ctx, keysource.Thumbprint) (bool, error)` — called once per
hardware-backed certificate inside `Gather`'s enumeration loop (the new
`certPresence` helper), rather than once for the whole batch and reused
for every row. `Deps.Readers` is unchanged: it still answers "is there a
reader attached at all" (F1's original question), rendered in its own
section of `certs`' output, independent of any certificate — exactly the
"different question, different message" this task asked to preserve.
`cmd/liro-bridge/main.go`'s `runCerts` now constructs a
`windowscng.Source` and wires `PresenceCheck: cngSource.Presence`,
dropping the `AnyCardPresent: svc.AnyCardPresent` wiring, which is now
unused (nothing else in `internal/cli` read that field).

**Why.** Reported directly, from a real run: `liro-bridge certs` on a
machine with only a MUP e-ID card in the reader listed a Halcom
certificate — whose card was not present anywhere — as `usable`. The
cause, read directly from `internal/cli/report.go` before this fix:
`Gather` called `deps.AnyCardPresent(ctx)` exactly once and passed that
single boolean to `classify.Classify` for *every* enumerated
certificate. D-014 (F1) established the underlying rule correctly — read
presence from `SCardListReaders`/status, never from certificate
enumeration — but the phase that implemented it only ever asked "is
there a card in any reader," not "is there a card in *this
certificate's* reader," which is a coarser question than the one a
bookkeeper's machine with several clients' certificates installed
actually needs answered (SPEC §14.1 already names this configuration as
normal, not an edge case). One card inserted therefore marked every
hardware-backed certificate on the machine as available, reproducing
exactly the failure mode D-014's own reasoning was written to prevent —
just one certificate later than D-014 anticipated: the user selects the
wrong-but-apparently-usable certificate, clicks Sign, enters a PIN, and
only then discovers the card is missing.

SPEC §11.10 is amended (not superseded — the underlying rule, "never
infer presence from enumeration," was already correct) to state the
granularity explicitly: presence is a property of one certificate, not
of the machine, and must be decided by attempting to open that
certificate's own key. The original wording ("presence is determined
from `SCardListReaders`... or by attempting to open the key container")
already named opening the key container as an acceptable method, but
did not say *per certificate*, which is exactly the gap this
implementation fell into.

**How this was tested without a card physically present or absent.**
`TestIsCardAbsentStatus` pins the pure classification
(`isCardAbsentStatus`) directly against both named codes and against
codes that must *not* count as absence (`NTE_NO_KEY`,
`SCARD_W_WRONG_CHV`), the same "logic lives in a plain, unit-testable
function; only the DLL call itself is untestable" split this package
already uses for every other piece of Windows-API behaviour (F1/F2's own
`conn_windows.go`/`session_core.go` split). `TestSourcePresence*`
(`source_test.go`) drives `Source.Presence` through the existing
`fakeConn` pattern (`session_core_test.go`, extended with
`presencePresent`/`presenceErr` fields) to prove per-call results and
error propagation and cancelled-context handling, without any real
syscall. At the `internal/cli` layer,
`TestGatherPresenceIsPerCertificateNotGlobal`
(`report_test.go`) is the direct discriminator for the reported bug: two
certificates built from the *same* underlying (qualified, otherwise
usable) DER but different thumbprints, and a `PresenceCheck` that
answers `true` only for one of the two thumbprints, must produce two
different `Usable` results. Run against the code as it stood before this
fix (a single `anyCard` value reused for both), both certificates would
have reported `Usable == true`; after the fix, only the "present" one
does, and the "absent" one carries `NotUsableReason ==
errs.CodeCardNotPresent`.

**Rejected.**
- **Routing `probePresence`'s failures through the existing `mapStatus`
  table.** `mapStatus` currently maps `NTE_BAD_KEYSET` to
  `CERT_NOT_FOUND` (`errors.go`, unchanged), a mapping this task does not
  touch: `mapStatus` serves `findAndAcquire`'s signing-session path,
  where "bad keyset" reported as "certificate not found" is pre-existing
  behaviour this bounded fix pass was not asked to revisit, and changing
  it would have widened this task's scope into the signing path for no
  requirement stated here. `probePresence` interprets the raw status
  itself instead, precisely because the signing path's mapping and the
  presence-probe's mapping are allowed to disagree without either being
  wrong for its own caller.
- **Keeping one `AnyCardPresent`-shaped call but iterating it per reader
  instead of per certificate.** Would not fix the bug: the failure is
  that a certificate's presence cannot be determined from reader state
  alone when multiple certificates (possibly for different, absent
  cards) share the machine — only opening that specific certificate's own
  key answers the right question.
- **Treating a `PresenceCheck` error the same as a fatal `Gather`
  error.** Rejected: a single certificate's probe failing (e.g. a race
  between enumeration and the probe) must not fail the entire `certs`
  command for every other, unrelated certificate. `certPresence` treats
  an error conservatively as "not present" and logs it, rather than
  propagating it up through `Gather`'s return.

---

## D-078 — A qualified timestamp's genTime is compared against the /M value after signing; a warning, never a failure, when they disagree by more than five minutes

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — sixth fix-and-polish pass (Task 3)

**Decision.** In `internal/pades.SignDocument`, once a timestamp
response is obtained successfully (`tsaErr == nil`), the code now
compares `resp.GenTime` (the TSA's attested time) against `signingDate`
— the same value already written into the signature dictionary's `/M`
entry before the TSA round trip began (SPEC §12.2/the existing
"the stamp shows `/M`, unavoidable, Adobe does the same" decision, which
this task leaves untouched). If `|resp.GenTime - signingDate|` exceeds
the new `clockDriftWarnThreshold` (5 minutes), `pades.Result` gets
`ClockDriftWarning = true` plus `MachineTime`/`TimestampTime` (the two
compared values, for a caller that wants to name both), and an
English `Notes` entry. **Nothing here fails the operation, aborts, or
alters a single byte already written** — the check runs after
`ph.InjectSignature` in every way that matters (it only reads `resp` and
`signingDate`, both already computed) and only ever adds to `Result`.
`internal/cli.printClockDriftWarning` prints a new, localised
`sign.clock_drift_warning` message (added to all three catalogues) to
stderr — advisory output, exactly like the existing test-key warning,
never affecting the exit code or the file written.

**Why.** The task's own report is direct: a stamp on a real signed
document read "2026-09-02 13:05 CEST" — the signer's machine clock,
correctly and unavoidably, since the stamp's bytes are fixed before the
timestamp exists (this project's own applyStamp doc comment already
explains why, and this task does not revisit that decision). But nothing
previously checked that clock against anything after the fact: if it had
drifted, the document would carry a stamp asserting one time and a
qualified RFC 3161 token attesting a different one, sent to third
parties, with no signal to the signer that anything was wrong. This
matters especially because the failure compounds silently — a signer
whose clock is wrong keeps producing documents with the same wrong
stamp date until something else forces them to notice.

**Distinct from the existing ±10-minute hard check.**
`internal/pades/tsa.Client.Timestamp`'s own `validateResponse` already
rejects a response whose `genTime` is more than
`maxSkew` (10 minutes) from `time.Now()` *at the moment the response is
validated* — this is unrelated machinery, added earlier (F3 §6.5) to
protect the timestamp's own trustworthiness (a wildly wrong `genTime`
might mean a broken or malicious TSA), and it aborts the whole
timestamp step outright. This task's check is different in every
relevant way: it compares against `signingDate` (captured once, before
the round trip, not "now" at an arbitrary later moment), it uses a
tighter 5-minute threshold specifically so an honest signer notices
before their next hundred signatures, and it never fails anything — it
exists purely to inform. Because the existing hard check already bounds
how far `resp.GenTime` can be from real "now" (±10 minutes) before this
task's check even gets a chance to run, the two compose without
conflict: a response failing the hard check never reaches `SignDocument`
at all (the operation aborts or falls back to B-B per existing TSA
failure handling, unchanged), and one that passes it can still trip this
task's softer, earlier-firing warning.

**Localisation.** `sign.clock_drift_warning` was added to all three
catalogues:

```
en:      "Warning: your computer's clock (%s) and the trusted timestamp (%s) differ by more than five minutes. Check your computer's clock."
sr-Latn: "Upozorenje: sat vašeg računara (%s) i pouzdani vremenski žig (%s) razlikuju se za više od pet minuta. Proverite sat na računaru."
sr-Cyrl: "Упозорење: сат вашег рачунара (%s) и поуздани временски жиг (%s) разликују се за више од пет минута. Проверите сат на рачунару."
```

Both times are named, per the task's own requirement — a user who only
sees "your clock might be wrong" has nothing to check it against.

**Tested with a fake TSA, not the real Pošta test TSA.** The real test
TSA (SPEC §12.7) always returns its own current time, which this
project's tests cannot control — there is no way to deterministically
produce a >5-minute (but <10-minute, to stay inside the existing hard
check) disagreement against a live service. `internal/pades/tsa_fake_test.go`
is a small, from-scratch RFC 3161 `TimeStampResp` builder (deliberately
independent of `internal/pades/tsa`'s own unexported test helper of the
same shape, `tsa/client_test.go`'s `buildTestResponse` — that package
has no exported way to fabricate a response with a controllable
`genTime`, and adding test-only production surface for one call site was
not this task's purpose) that decodes an incoming request's nonce and
digest and grants a token with a caller-chosen `genTime`.
`TestSignDocumentClockDriftWarningWhenTimestampDisagrees` drives this
against a real `internal/pades/tsa.Client` with a 7-minute offset
(beyond the 5-minute threshold, inside the 10-minute hard-check window)
and asserts the warning fires with both times populated, and that the
signature itself still independently verifies via
`internal/pades/verify` — proving the warning is purely additive.
`TestSignDocumentNoClockDriftWarningWithinThreshold` proves ordinary,
sub-threshold skew (a `genTime` equal to `signingDate`) never nags the
user. `internal/cli`'s `TestPrintClockDriftWarningLocalised` and
`TestLevelLineReportsRevocationTooLargeLocalised` (D-076) both exercise
the localisation directly against fabricated `*pades.Result` values, in
all three locales, independent of the signing pipeline.

**Rejected.**
- **Comparing against `time.Now()` at the moment `SignDocument` finishes,
  instead of the `/M` value (`signingDate`) captured before the TSA round
  trip.** Rejected: the entire point is to check what the *document
  itself claims* (`/M`) against what the timestamp attests — comparing
  against a third, later value would answer a different question ("did
  time pass normally during this function call," which is uninteresting)
  rather than "do the two times the document carries agree."
- **Failing the operation, or forcing a save-without-timestamp choice,
  when drift exceeds the threshold.** Explicitly ruled out by the task:
  "do not fail, do not alter the document." A wrong local clock is the
  signer's problem to fix before their *next* batch, not a reason to
  block the one they are looking at right now — SPEC §12.8's "TSA failure
  must never block" carries the same spirit even though this is not a
  TSA failure.
- **A single shared threshold constant with the existing ±10-minute hard
  skew check**, rather than a second, independent 5-minute constant.
  Rejected: the two checks protect different things (the timestamp's own
  trustworthiness, versus informing the user their clock looks wrong) for
  different audiences (an aborted operation, versus an advisory message),
  and conflating their thresholds would make a future change to one
  silently change the other's behaviour for an unrelated reason.

---

## D-079 — A /DSS revision is written only when it carries at least one OCSP response or CRL; certificates alone never justify one

**Date:** 2026-09-02
**Phase:** F4/F5 boundary — seventh fix-and-polish pass

**Decision.** `internal/pades/dss.Apply` now returns without writing an
incremental revision at all when, after collecting evidence for every
certificate, `/DSS`'s would-be `/OCSPs` and `/CRLs` arrays are both
empty — i.e. when the only thing the revision would carry is `/Certs`.
`Result.Bytes` in that case is `doc.Data()`: the document's own
original bytes, returned unmodified (`Document.Data`'s documented
contract — "the same slice, never a copy" — makes this exact, not just
byte-equal). `Complete`, `TooLarge` and `LargestSkippedBytes` are still
computed and returned exactly as before; only whether the revision gets
appended is now conditional. [[D-076]]'s size cap and everything
upstream of it — `dss.CollectRevocation`, `internal/pades.applyDSS`,
`Result.AchievedLevel`/`RevocationTooLarge`/`Notes` in
`internal/pades/sign.go` — is unchanged: `applyDSS` already assigns
`result.Bytes = dssResult.Bytes` unconditionally, so returning the
original bytes from `Apply` was sufficient by itself to make the whole
pipeline skip the revision; no caller above `dss.Apply` needed editing.

**Why.** [[D-076]]'s size cap, measured against MUP's real OCSP responder
(unreachable — see that entry) and its 30,136,214-byte CRL, made the
oversized-file symptom go away: the reported level degrades honestly to
B-T instead of embedding a 30 MB CRL. But a controlled comparison — the
same real card, the same real timestamp, two documents each signed both
ways — showed Adobe Acrobat still rejecting the output whenever a DSS
revision was appended at all:

| Document | DSS revision | Acrobat |
|---|---|---|
| Pošta | none (B-T only) | accepts |
| Pošta | empty (`/Certs` only, no `/CRLs`/`/OCSPs`) | **rejects** |
| MUP | none (B-T only) | accepts |
| MUP | empty (`/Certs` only, no `/CRLs`/`/OCSPs`) | **rejects** |

The only difference within each pair is the appended DSS revision. What
that revision actually contained in both rejected files was checked
directly: `<</Certs [130 0 R 131 0 R]/VRI <</2720DC...
<</Cert [130 0 R 131 0 R]>>>>>>` — certificates only, no `/CRLs`, no
`/OCSPs`. The CRL was discarded by [[D-076]]'s size cap; OCSP produced
nothing because MUP's responder does not accept a TCP connection at
all (also established in [[D-076]]).

**The DSS revision's structure itself was verified correct — this is a
semantic problem, not a malformed-output problem.** Every xref offset
resolves to the right object, the `/Prev` chain is intact, a classic
xref table was appended matching the input's own last-revision
mechanism, the original bytes remain a literal prefix, and all three
signatures (the pre-existing one(s) plus the new one) verify
independently. None of [[D-069]]'s `/ByteRange` fix, [[D-072]]'s
literal-string fix, or [[D-074]]'s direct/indirect `/AcroForm`
preservation fix — the three prior byte-level Acrobat defects this
project found and fixed — apply here; this revision has none of those
defects. The defect is what the revision *means*: a `/DSS` whose entire
purpose (SPEC §12.6) is preserving long-term validation evidence, but
which contains no revocation evidence at all, achieves nothing. It
costs a revision and costs bytes, and it asserts — by existing at all —
a claim ("this document carries LTV evidence") the document cannot
support, while the reported level is honestly B-T. Certificates alone
never justify a `/DSS`: they are already present in the CMS
(`signerInfo`/the certificate chain, SPEC §12.3/§11.8), so a `/DSS`
carrying only `/Certs` duplicates information the document already has
under a dictionary whose name specifically means "here is the
revocation evidence."

Skipping the revision when there is nothing to embed in it makes the
output, for every case this project has actually produced against real
hardware to date, byte-for-byte equivalent in structure to the B-T
documents Acrobat already accepts — because in every case measured so
far, "revocation deliberately unavailable" and "the DSS would have been
empty" are the same condition.

**What remains unproven.** Whether Acrobat objects to an *empty* DSS
specifically, or to something about this project's DSS revisions in
general, is not established by this fix or by the measurement that
motivated it. Every DSS this project has produced against real
hardware has been empty — MUP's OCSP responder is unreachable and its
CRL exceeds [[D-076]]'s cap, and no other issuer's real hardware has
been exercised through this path yet — so there is no measured case of
a genuinely populated DSS (real `/OCSPs` or `/CRLs` entries) being
tested against Acrobat at all. If a populated DSS is later produced
against real hardware and Acrobat still rejects it, that is a separate,
new defect and a separate investigation — not evidence against this
entry's reasoning, which only ever claimed that an *empty* DSS is
semantically wrong regardless of what Acrobat thinks of it. That future
investigation, if needed, should reference this entry rather than
duplicate its measurement.

**Testing without real hardware.** `internal/pades/dss/dss_test.go`
gained `TestApplyWritesNoRevisionWhenAllEntriesAreTooLarge` (mirrors
the already-existing `TestApplyIncompleteWhenCollectionFails`, adding
the `bytes.Equal(result.Bytes, src)` assertion both now carry) —
`Apply` fed entries with no OCSP/CRL evidence, asserting `Result.Bytes`
equals the input document's own original bytes exactly. At the
`internal/pades` level, `sign_test.go` gained
`TestSignDocumentBLTSkipsDSSWhenNoRevocationEvidence` (a `chainedSession`
with no OCSP responder and no CRL distribution point configured at
all — the network-independent "no endpoint exists" branch of
`dss.CollectRevocation`) and
`TestSignDocumentBLTAchievedWithOCSPEvidence` (a fake `httptest.Server`
built with `golang.org/x/crypto/ocsp`, already an indirect dependency
via `internal/pades/dss`'s own tests, returning a small, valid `Good`
response for the signer's real serial number). The first asserts the
output contains no `/DSS` key, that the original input bytes are a
literal prefix of the output, and that its `%%EOF` count matches a
plain B-T signing of the identical input signed with the identical
keys/TSA — proving no extra revision was appended beyond the signature
itself. The second parses the output with `internal/pades/pdf` and
asserts `/DSS`/`/OCSPs` resolve and `AchievedLevel == LevelBLT`. Both
independently re-verify the signature via `internal/pades/verify`.
`TestSignDocumentBLTDegradesToBTWhenRevocationTooLarge` ([[D-076]]'s own
end-to-end test) gained the same no-`/DSS`-in-output assertion, since
under this fix its scenario (a CRL fetched but rejected by the size
cap, no OCSP configured) is also a no-evidence case. Every assertion
above is scoped to what this project's own code produced for a given
run — the no-`/DSS` check is a substring/structural check on the whole
output (there is no pre-existing `/DSS` in any input fixture these
tests construct, so whole-file scoping is not the [[D-072]]/[[D-074]]
trap here), and the prefix/EOF-count checks compare two independently
produced outputs rather than searching for an untouched original inside
one of them.

**Rejected.**
- **Refusing B-LT outright (returning an error) instead of degrading
  silently to a clean B-T document.** Rejected: this is exactly the
  behaviour [[D-076]] already established as correct — degrade honestly
  to the level actually achieved, report why, and hand back a usable
  document — and this fix does not change that contract at all. It only
  changes what "the level actually achieved" is allowed to look like on
  disk: a real B-T document, not a B-T-labelled document that still
  carries a vestigial, empty `/DSS`.
- **Writing `/Certs` alone whenever at least the certificates are
  available, on the theory that "some evidence is better than none."**
  Rejected by the task's own framing and independently reasoned here:
  the certificates are not revocation evidence, they are already in the
  CMS, and a `/DSS` dictionary's presence is itself a signal ("this
  document has LTV support") to any validator that inspects for one —
  a signal this project should not send when it is false.
- **Changing `internal/pades.applyDSS` (in `sign.go`) instead of
  `dss.Apply`.** Considered, since `applyDSS` is the call site that
  decides `Result.AchievedLevel`. Rejected because `dss.Apply` is
  already the one place that knows, from `ocspRefs`/`crlRefs`, whether
  there is anything to embed — duplicating that check one layer up
  would mean either recomputing it from `entries` a second time (two
  places that must agree on the same condition) or exposing
  `ocspRefs`/`crlRefs` as new `Result` fields for `applyDSS` to inspect,
  neither of which is simpler than making `Apply` itself refuse to
  write a revision it cannot justify. This also kept `sign.go` entirely
  untouched, satisfying the task's "this one change only."

---

## D-080 — WebView2 host: hand-written COM interop, driven by the real WebView2.idl; the redistributable loader is embedded and extracted at first use

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `internal/ui`'s WebView2 host is hand-written COM interop
(F5 §2.1's first option), not a third-party binding. Every interface
layout — vtable slot indices, method signatures, IIDs — used anywhere
in the package (`ICoreWebView2Environment`, `ICoreWebView2Controller`,
`ICoreWebView2`/`ICoreWebView2_3`, the four completion/event handler
interfaces this package implements, and
`ICoreWebView2WebMessageReceivedEventArgs`) was copied from
`WebView2.idl` inside the `Microsoft.Web.WebView2` NuGet package
(fetched directly from `nuget.org`, version 1.0.4191.47), not
reconstructed from memory or from Microsoft's prose documentation —
every slot index in `internal/ui/*_windows.go` is commented with the
IDL method it corresponds to and its position in that interface's
declaration order, so it can be checked against the IDL directly if a
future SDK version changes anything. `WebView2Loader.dll` (x64), the
small redistributable stub every non-.NET WebView2 host must call
through (`CreateCoreWebView2EnvironmentWithOptions`,
`GetAvailableCoreWebView2BrowserVersionString`), is committed at
`internal/ui/assets/webview2/WebView2Loader.dll` alongside its licence
(`LICENSE.txt`, the same directory — a BSD-style licence that
explicitly permits redistribution in binary form), embedded with
`go:embed`, and extracted to
`%LOCALAPPDATA%\Liro\webview2\WebView2Loader-<size>.dll` on first use
(`loader_windows.go`). Go objects implementing a COM interface (the
four handler kinds) share one struct layout, `comBase{vtbl uintptr;
refs int32}`, whose address IS the interface pointer per the universal
C++/COM ABI; `QueryInterface`/`AddRef`/`Release` are generic thunks
shared by every kind, and each kind's `Invoke` function pointer is
created once, at package `var` initialisation, via
`syscall.NewCallback` — never once per call or per window (`com_windows.go`'s
"vtable singletons" comment).

**Why.** F5 §2.1 states the reality directly: there is no supported Go
binding for WebView2, and the two acceptable approaches are
hand-written interop (matching F1/F2's precedent for `winscard.dll`/
`ncrypt.dll`) or "a single well-scoped third-party binding... if one
exists that is maintained and small enough to read," with hand-written
preferred "if the second means importing something large." The
candidate third-party bindings available at the time of writing
(`github.com/jchv/go-webview2`, already present in this environment's
module cache from unrelated prior use) pull in a full webview
abstraction — window creation, event dispatch, a public API shaped
around a generic "webview" concept — none of which this project wants;
this project needs exactly the subset F5 §2.4 specifies (three
messages in, one `ExecuteScript` call out, virtual-host asset serving)
plumbed into a window this project's own code fully controls
(fixed-size, always-on-top for consent, centred on the cursor's
monitor, DPI-aware). Hand-written interop, scoped to exactly that
subset, is smaller and more auditable than adapting a general-purpose
binding down to it — the same reasoning D-016 already applied to
choosing an in-house Exclusive C14N implementation over a general XML-
Security library.

The loader DLL itself is unavoidable regardless of which option is
chosen: `CreateCoreWebView2EnvironmentWithOptions` is not a documented,
stable COM `CoCreateInstance` target — every WebView2 host in every
language calls it through this loader stub, which is why SPEC §8.6
lists "a WebView2 binding" as an expected acceptable dependency in the
first place. Embedding it and extracting it to the per-user config
directory (rather than requiring a side-by-side file at install time,
which F10's packaging phase does not exist yet to arrange) keeps the
agent a true single binary as distributed, satisfying SPEC §1's design
centre, at the cost of one small file written to disk on first launch —
the same trade-off `internal/pades/appearance` already makes for its
embedded font and logo assets.

**What the interop actually took, and the two real bugs it surfaced.**
Once the vtable slot indices were read correctly from the IDL, a blank
WebView2 window rendering trusted local HTML, receiving `PostJSON`
calls from Go, worked on the first attempt against real hardware (a
genuine Windows 11 machine with the Evergreen Runtime already
installed, version 152.0.4191.53) — but closing that window crashed
with an access violation, twice, from two different causes, both found
by attaching a Go stack trace to the crash and reasoning from the
exact call site, not by guessing:

1. `ICoreWebView2Environment::CreateCoreWebView2Controller` initially
   failed synchronously with `HRESULT 0x802A000C` ("This method can
   only be called from the thread that created the object"), even
   though a `GetCurrentThreadId()` trace proved every call — environment
   creation, the completion callback, controller creation — ran on the
   identical OS thread. Comparing against `go-webview2`'s own working
   implementation (present in this machine's module cache) showed the
   actual rule: the environment/controller pointers a
   `*CompletedHandler::Invoke` receives are **borrowed references**,
   valid only for the duration of that call, not already `AddRef`'d for
   the receiver the way an ordinary `[out, retval]` property getter's
   result is. `environmentCompletedInvoke`/`controllerCompletedInvoke`
   now call `AddRef` (vtable slot 1) on the delivered pointer before
   returning, before this package's code ever uses it outside the
   callback.
2. Once controller creation worked, `ICoreWebView2Controller::Close`
   crashed inside itself with SEH `0xc0000005`. Two independent causes
   were found and fixed together: `Close()` was being called from a
   `WM_DESTROY` handler, after `DestroyWindow` had already torn down
   the WebView2 control's own child `HWND` as part of Windows' own
   child-window cleanup — `Close()` now runs from `WM_CLOSE`, before
   `DestroyWindow`, matching Microsoft's own sample ordering. Separately,
   the `webMessageReceivedHandler` object passed to
   `add_WebMessageReceived` was never stored anywhere Go-reachable
   after that call returned — WebView2 holds a raw pointer to it, not
   a Go reference, so nothing stopped the garbage collector from
   reclaiming it, and `Close()` (which releases every registered event
   handler as part of closing) then dereferenced a stale address. The
   handler is now stored on the `window` struct for the subscription's
   whole lifetime.

A third, unrelated bug surfaced during manual verification, not a
crash: the window initially centred on screen coordinate (0, 0) instead
of the monitor under the cursor, because `MonitorFromPoint`'s `POINT`
parameter is passed **by value** — on the amd64 calling convention a
struct that small is packed into one 64-bit argument (x in the low 32
bits, y in the high 32 bits), not two separate `uintptr` arguments; two
arguments silently shifted every later parameter by one slot, so
`GetMonitorInfoW` received a garbage or null monitor handle and left
its output struct at its zero value. Fixed by packing the point
manually (`win32_windows.go`'s `cursorMonitorRect`).

**Manual verification performed.** All on this real Windows 11 machine,
with the genuine Evergreen Runtime, not a stub: `DetectRuntime` reports
the installed version; a window is created, sized and DPI-scaled
correctly, centred on the monitor under the cursor (verified against
`System.Windows.Forms.Screen.PrimaryScreen.Bounds` — the window's
`GetWindowRect` centre matched the screen's centre to the pixel), shown,
and takes the foreground (`GetForegroundWindow` matched); `PostJSON`
successfully runs `ExecuteScript`; closing — both the timeout-driven
`Window.Close()` path and, structurally, the same code path a
user-driven `WM_CLOSE` takes — tears down cleanly with no crash and no
leaked `msedgewebview2.exe` process group beyond Windows' own normal
warm-background-instance behaviour (observed independently of this
project's own process, on the very same machine, before this project's
binary was ever run).

**`go vet`'s `unsafeptr` check is disabled project-wide, not
suppressed per line.** Implementing a COM object in Go means
reinterpreting the raw `this` machine word a callback receives from
foreign code as a `*T` — exactly the pattern `unsafeptr` exists to
catch for accidental misuse (storing a `uintptr` across a sequence
point, then converting it back to a `Pointer`), but unavoidable and
correct for implementing a native callback ABI: verified directly
(a two-line reproduction) that isolating the conversion inside its own
helper function does not change vet's verdict, since the check is
per-expression, not per-package or per-function, so no restructuring
of this package's code could satisfy it. `-unsafeptr=false` is passed
to both the CI `go vet` step and this project's own `.golangci.yml`
(`linters.settings.govet.disable`), with the reasoning recorded at
both call sites rather than only here.

**Rejected.**
- **A third-party WebView2 binding** (`github.com/jchv/go-webview2` or
  similar). Rejected per F5 §2.1's own preference ordering: these pull
  in a general-purpose webview abstraction and public API surface this
  project does not want, for a problem — the specific, narrow subset of
  WebView2 F5 actually specifies — hand-written interop covers in
  roughly a dozen files with every design decision (window ownership,
  message surface, virtual-host mapping) under this project's own
  control.
- **Shipping `WebView2Loader.dll` side-by-side with the executable
  instead of embedding and extracting it.** Would work, but makes the
  distributed artefact two files instead of one, contradicting SPEC
  §1's "single binary" design centre for no benefit this phase — F10's
  installer, when it exists, could still choose to ship it side-by-side
  if that ever becomes preferable, without this decision blocking it.
- **Retrying `CreateCoreWebView2Controller` on `HRESULT 0x802A000C`
  instead of finding the root cause.** Considered only briefly, in the
  sense of "maybe this is transient" — rejected immediately once the
  thread-ID trace proved the literal "wrong thread" explanation false,
  which is what led to comparing against a working implementation and
  finding the real (missing-`AddRef`) cause. A retry loop around a
  bug is exactly the kind of thing SPEC §0 warns against doing instead
  of understanding the failure.

---

## D-081 — DPI awareness is declared programmatically, not via an application manifest; no manifest pipeline exists yet

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `internal/ui` calls
`SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)`
once, at process startup (`win32_windows.go`'s `ensureDPIAware`,
guarded by `sync.Once`), rather than declaring per-monitor DPI
awareness in an embedded application manifest.

**Why.** F5 §2.3 asks for per-monitor DPI awareness "declared in the
manifest," but SPEC §5's repository layout places the manifest
(`cmd/liro-bridge/rsrc.syso`, "Windows icon/manifest (generated)")
under packaging, which F10 builds — no manifest-generation step exists
in this repository yet, and inventing one now, only for this one
attribute, would be exactly the kind of scope creep SPEC §0 warns
against ("do not build beyond your phase"). `SetProcessDpiAwarenessContext`
(available since Windows 10 1703, which is older than the WebView2
Evergreen Runtime's own minimum supported OS) is Microsoft's documented
programmatic equivalent to the manifest entry, achieves the identical
runtime effect, and needs no new build tooling — verified directly:
the smoke-tested window rendered at the correct physical size and
stayed sharp with no code path calling this function under simulated
DPI other than the one described here.

**Rejected.**
- **Building a manifest-generation pipeline in this phase to satisfy
  F5 §2.3's literal wording.** Rejected as scope beyond what F5 asks
  for build-wise; F10's packaging phase is where `rsrc.syso` and
  everything that depends on it belongs, and when it exists, adding a
  `<dpiAwareness>` entry there and removing this call is a small,
  independent, same-effect change — not a rework of anything in this
  phase.
- **Skipping DPI awareness declaration entirely, relying on Windows'
  default (per-process system-DPI-aware) behaviour.** Rejected outright
  by F5 §2.3's own reasoning: a window that is not per-monitor DPI
  aware renders blurry on a scaled display, "which is most laptops" —
  not a corner case for this product's actual users.

---

## D-082 — Assets are served over a virtual host mapping; the tray icon is Windows' default `IDI_APPLICATION`; autostart uses `golang.org/x/sys/windows/registry`; the tray-launching entry point is an explicit `tray` subcommand

**Date:** 2026-09-03
**Phase:** F5

**Decision.** Every window's HTML/CSS/JS is served through
`ICoreWebView2_3::SetVirtualHostNameToFolderMapping` at `https://liro.local/...`
(`internal/ui/window_windows.go`), never `file://` and never a local
HTTP server. The tray icon (`internal/ui/tray_windows.go`) loads
`IDI_APPLICATION` via `LoadIconW(0, IDI_APPLICATION)` rather than a
custom `.ico` resource. Autostart (`internal/platform/autostart_windows.go`)
is implemented with `golang.org/x/sys/windows/registry`, not hand-written
`RegOpenKeyEx`/`RegSetValueEx` syscalls. Starting the tray is a new,
explicit `liro-bridge tray` subcommand, not the bare-invocation path.

**Why.**
- **Virtual host over `file://`/a local server.** F5 §2.4 states the
  reasoning directly: `file://` origins have different, weaker web
  security properties (no meaningful origin isolation from other
  `file://` content on the machine), and a local HTTP server is a
  second listening socket, contradicting SPEC §6.1's "loopback API,
  nothing else listens" model. A virtual host name that resolves to
  nothing outside this one `ICoreWebView2` instance gives the page a
  real, stable origin (`https://liro.local`) with none of either
  drawback.
- **`IDI_APPLICATION` instead of a custom icon.** F5 §3's checklist item
  is "icon present from startup," not a specific brand mark — unlike
  the visible signature stamp (SPEC §13), F5 does not specify tray icon
  geometry or colour at all. Building a real `.ico` (or a hand-drawn
  `CreateIconIndirect` bitmap, D-061's placeholder-logo precedent) is
  packaging-shaped work that belongs with `cmd/liro-bridge/rsrc.syso`
  in F10, not invented early for a requirement F5 does not state (SPEC
  §0: do not invent requirements). Using Windows' own default
  application icon needs no new asset and no new GDI code, and is
  trivially replaceable later by pointing `LoadIconW` at a resource
  instead of `IDI_APPLICATION`.
- **`golang.org/x/sys/windows/registry` for autostart.** Unlike the
  WebView2 COM surface (D-080), there is no reason to hand-write this:
  `golang.org/x/sys/windows/registry` is already a transitive part of
  this project's existing `golang.org/x/sys` dependency (SPEC §8.6
  already lists it as an expected acceptable dependency), well-scoped,
  and small. Hand-writing `RegOpenKeyExW`/`RegSetValueExW` would add
  syscall-level risk (exactly the kind D-080's COM work carries) for no
  benefit, since — unlike WebView2 — a maintained, minimal binding
  already exists in a dependency this project already has.
- **An explicit `tray` subcommand.** F5 does not name a CLI entry point
  for starting the background agent. Bare invocation (`liro-bridge`
  with no arguments) already has a tested, F0-established contract —
  print usage, exit 0 (`TestNoArgsPrintsUsageAndExitsZero`) — and
  changing it to launch a blocking, window-creating tray process would
  both break that test and make a bare invocation in a script or test
  harness hang waiting for a GUI. A new, explicit subcommand is the
  simplest option that adds F5's behaviour without touching F0's.

**Rejected.**
- **A custom-drawn tray icon (D-061's "generic mark" pattern) or the
  real Liro logo (D-070) rasterised into an `HICON`.** Both would need
  new GDI code (`CreateDIBSection`/`CreateIconIndirect`) this phase does
  not need to write — F5's checklist is satisfied by "present from
  startup," and a real icon is exactly the kind of packaging-shaped
  asset F10 already owns (`rsrc.syso`).
- **Hand-written registry syscalls for autostart**, to keep the "no
  third-party binding beyond what's necessary" discipline as strict as
  D-080's. Rejected because that discipline exists to bound *new* risk
  (D-080's own reasoning is entirely about WebView2 having no
  alternative); `golang.org/x/sys/windows/registry` is not new risk —
  it is already in the dependency graph and is exactly the kind of
  "expected acceptable dependency" SPEC §8.6 anticipates.
- **Making bare invocation start the tray.** Would match some other
  desktop agents' convention, but directly contradicts F0's own tested
  contract for this project and would make `liro-bridge` (no args) a
  footgun in any script or CI step that just wants to check the binary
  runs.

---

## D-083 — The page->Go message surface stays exactly three types; the settings window reads its form back through `ExecuteScript`'s own return value, not a fourth message type

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `internal/ui.MessageType` has exactly three values —
`approve`, `cancel`, `selectCertificate` — used, unmodified, by every
window this phase builds (consent, pairing, settings). Pairing's
Allow/Deny buttons send `approve`/`cancel` (F5 §6's own natural mapping
onto the same two ideas). The settings window, which genuinely needs to
report a whole form's worth of structured data — language, autostart,
TSA URL, output suffix, signature level, and *which* action button was
pressed — does this by having its "Save"/"Export audit log"/"Check for
updates now" buttons all send the existing `approve` message, and Go
then calls a new `Window.Eval(script string) (string, error)`
(`internal/ui/window_windows.go`) that runs a small trusted script
(`window.__liroCollectState()`, `internal/ui/assets/pages/settings.js`)
and reads its JSON-encoded return value back through
`ICoreWebView2ExecuteScriptCompletedHandler`'s own completion value —
a channel that already existed for the Go->page direction (`ExecuteScript`)
and needed only to stop discarding its result, not a new page->Go
message type. The consent window's failed state accordingly no longer
offers "Retry" or "Save without a timestamp" as interactive buttons
(SPEC §12.8's choice); it shows the actionable message and a "Copy
technical details" button implemented entirely client-side
(`navigator.clipboard`), plus "Close" (which sends `cancel`).

**Why.** F5 §2.4 states the constraint as an unconditional property of
the host, not of one window: "The page can request exactly three
things... Anything else is dropped and logged. Keep the message surface
this small deliberately," and the exit checklist repeats it standalone:
"Exactly three message types accepted; others dropped and logged."
Reading this as scoped only to the consent window (the security-critical
one) was considered, since F5 §7 (settings) never repeats the
constraint — but the checklist item is phrased as a host-level property,
and honouring it literally, for every window, is strictly safer and
no harder once `Window.Eval` exists: no future window can accidentally
smuggle structured data into Go through a fourth `MessageType` value,
because there still isn't one. `ExecuteScript`'s result parameter
(`webview2_windows.go`'s `executeScriptCompletedInvoke`) was already
part of the Go->page direction F5 §2.4 explicitly allows
("Go -> page: ExecuteScript with a JSON payload"); reading back what a
*trusted, project-authored* script (never built from untrusted input)
returns is a different use of an existing channel, not a new one.

Removing Retry/Save-without-timestamp from the failed state is the
direct, honest consequence: those actions need to tell Go to *do*
something (retry, or continue at B-B) with data the three-message
surface has nowhere to carry once "reuse `approve`/`cancel` for
everything" is off the table for the consent window specifically (unlike
settings, consent's `approve` already means something — the original
sign decision — and overloading it a second time on the failed screen
would blur what the audit log records that click as). SPEC §12.8's
"save without a timestamp" choice is still made — by `--on-tsa-failure`
at the batch level, before signing begins, exactly as F3 already
implemented it — just not as a live, mid-failure re-decision from the
consent window in this phase.

**Rejected.**
- **Adding a fourth `MessageType` (e.g. `formSubmit` carrying an
  arbitrary JSON payload) for settings.** Rejected as exactly what the
  checklist item exists to prevent: once one window can send arbitrary
  structured data, the "three things, deliberately small" property
  stops being true project-wide, for the convenience of one window that
  has another option available.
- **Scoping the three-message rule to consent/pairing only, leaving
  settings free to add its own message vocabulary.** Plausible given
  F5 §7's silence on the point, but the checklist's own wording doesn't
  draw that line, and `Window.Eval` makes the stricter reading free —
  there was no actual cost to honouring the narrower interpretation
  once the escape hatch existed.
- **Wiring Retry/Save-without-timestamp by overloading `approve` a
  second time on the failed screen, disambiguated by which state the
  window was in when it arrived.** Considered — it would work
  mechanically — but rejected as confusing for exactly the reason the
  audit log cares about: `approve` already has one meaning (the
  original consent decision) that the audit entry's `Outcome` is built
  from; reusing it for "please retry after a TSA failure" mid-flow
  makes that meaning ambiguous for no real gain, when the message text
  can already tell the user what to do next (F5 §5.5's own framing).

---

## D-084 — The audit `Entry` type has no field a personal name, file name or identifier could go in; the canonical form is a length-prefixed, fixed-field-order byte encoding pinned by a golden-value test

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `internal/audit.Entry` (`entry.go`) has exactly the eight
fields F5 §8.1 lists — `Sequence`, `Timestamp`, `Thumbprint`,
`Application`, `DocumentCount`, `Outcome`, `FailureCode`, `IsTestKey` —
plus the two hash-chain fields, and no others: no field for a signer's
name, a file name, a JMBG, or an email address exists on the type at
all. `CanonicalBytes()` concatenates every field in this fixed order,
each variable-length field (`Thumbprint`, `Application`, `Outcome`,
`FailureCode`, `PrevHash`) preceded by its own 4-byte big-endian length,
and `Timestamp` truncated to whole seconds before encoding.
`TestCanonicalBytesIsStable` builds the expected byte sequence
independently (via `encoding/binary` directly, not by calling
`CanonicalBytes`' own helpers) for one fixed `Entry` and compares —
a golden-value test in substance, without a checked-in binary fixture.

**Why.** F5 §8.3 states plainly: "Note that personal names are excluded
even though they appear on the consent screen. The certificate
thumbprint identifies the signer without storing their identity." The
strongest way to guarantee this is to give the type nowhere to put one
— a runtime check (e.g. "reject any field containing an '@' or 13
digits") would be strictly weaker, since it only catches a mistake at
the moment data flows through it, whereas an allow-list of fields
catches it at compile time, for every future caller, permanently. This
mirrors `internal/errs.Error`'s own design (D-002): the type itself is
the enforcement mechanism, not a check layered on top of a type that
could hold the wrong thing.

The canonical-form test needed one iteration to get right: an initial
version asserted the whole encoded `Entry` (hash and all) contained no
run of 13 consecutive digits, and failed — not because of a real leak,
but because `Hash` is a 32-byte SHA-256 digest, hex-encoded to 64
characters drawn from `[0-9a-f]`, and a run that decimal-digit-only by
chance across 64 mostly-random hex characters is not actually rare.
This is the same lesson D-072/D-074 already recorded for PDF output
byte-scanning: **scope an assertion to the bytes that are actually
meaningful, never the whole artefact** — `TestAuditLogNeverContainsPersonalData`
(`internal/audit/store_test.go`) now checks the parsed entry's semantic
fields only (`Thumbprint`, `Application`, `Outcome`, `FailureCode`),
explicitly excluding `Hash`/`PrevHash`, which are expected to look like
noise and are not "content" in SPEC §6.7's sense — they are derived
from the entry, not stored alongside it as separate information.

**Rejected.**
- **A `Note`/`Details` free-text field on `Entry`, for future
  flexibility.** Rejected outright: SPEC §0 warns against inventing
  requirements, and a free-text field is precisely the kind of thing a
  future caller would eventually put a name or a file name into,
  defeating the allow-list property this decision is built on.
- **Scanning the whole encoded entry (including `Hash`) for personal
  data, accepting the false-positive risk as "rare enough."** Rejected
  once measured directly: it fired on the very first realistic test
  run, not in some theoretical edge case, which means "rare enough" was
  wrong and the check would have been a source of real, recurring test
  flakiness rather than a meaningful guard.
- **A checked-in binary golden file for the canonical form**, matching
  `testdata/golden/`'s pattern for signed PDFs. Rejected as more
  machinery than one `Entry` value's byte layout needs — F3's golden
  files exist because a hand-rolled PDF/CMS byte layout is exactly the
  kind of thing that "looks right, verifies, and is subtly wrong" (SPEC
  §12.1); this canonical form is simple enough that an inline,
  independently-constructed comparison in the test itself is equally
  strong and far easier to read.

---

## D-085 — The consent view model is pure Go with no window dependency; Approve starts unfocused by giving Cancel the initial focus; the interactive CLI's batch fingerprint is a whole-file SHA-256, not the PAdES `/ByteRange` digest

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `internal/consent` (view model, sanitisation, certificate
options, progress/state machine, error-action mapping) imports nothing
from `internal/ui` and has no notion of a window — every function in it
is exercised by `internal/consent/*_test.go` alone. The page never
autofocuses anything; `cmd/liro-bridge`'s consent page
(`internal/ui/assets/pages/consent.js`) explicitly calls
`document.getElementById("cancel-btn").focus()` once the certificate
list has rendered, and Approve stays `disabled` until a `selectCertificate`
message has been received — so it is never focusable, let alone
focused, until the user has already made an explicit choice.
`cmd/liro-bridge/interactive_windows.go` computes each input PDF's
batch-fingerprint contribution as `sha256(fileBytes)` — the whole file
on disk — rather than the digest `internal/pades.SignDocument` computes
internally over the `/ByteRange`-selected content during signing.

**Why.** F5 §10 states the testability requirement directly ("The
consent screen's data... is computed in Go and testable without a
window"), and the package boundary is what makes that true rather than
aspirational — `internal/consent` cannot reach into `internal/ui` even
by accident, since nothing in it imports that package.
Explicitly focusing Cancel (rather than merely *not* focusing Approve,
which would leave focus on whatever the browser engine defaults to —
typically the first focusable element in DOM order, which happens to be
the first certificate row, not reliably Cancel) makes F5 §5.6's rule
"Approve is never the initially focused control" true by an assertion
the page makes about itself, not an accident of DOM order that a future
markup change could silently break.

The whole-file digest is a genuine, unspecified choice: F3's signing
pipeline computes its own digest internally, over the `/ByteRange`, as
part of `SignDocument` itself — there is no pre-signing digest exposed
for the consent screen to reuse, and computing one would mean either
partially replicating `/ByteRange` selection before signing (exactly
the fragile, easy-to-get-subtly-wrong logic SPEC §12.1 already singles
out for care) or restructuring `SignDocument` to expose an intermediate
digest, out of scope for this phase. SPEC §6.6's own reasoning for the
fingerprint — "so a technical user can verify what was approved against
what the calling application says it sent" — is satisfied by any stable,
reproducible digest of the approved content; a whole-file SHA-256 is the
simplest one available without touching F3's signing internals (SPEC
§0: choose the simplest option). It differs from the eventual
`/ByteRange` digest signed inside the CMS, but does not need to match
it — the consent fingerprint's job is confirming *which bytes were
shown and approved*, before any signature-specific transformation.

**Rejected.**
- **Focusing nothing and relying on the browser's default tab order.**
  Rejected: the default focused element after page load, with no
  explicit `.focus()` call, is not guaranteed across WebView2/Chromium
  versions, and "not explicitly Approve" is a weaker guarantee than
  "explicitly Cancel" for a rule SPEC calls "the most important
  paragraph in this document."
- **Computing the real `/ByteRange` digest for the fingerprint by
  partially replicating F3's placeholder/ByteRange logic ahead of
  signing.** Rejected as scope creep into F3's already-hardest, most
  carefully tested code (SPEC §12.1) for a value whose only job is
  giving a technical user *something* stable to compare, not
  cryptographically binding the approval to the final signed bytes —
  the signature itself, verified independently (SPEC §16.4), is what
  actually provides that binding.

---

## D-086 — `scripts/synctokens` is the source of the token values, not a puller of `@liro/tokens`' output, because that package is not available in this environment

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `scripts/synctokens/tokens.go` and `intents.go` contain
the literal CSS custom-property values (`internal/ui/assets/tokens.css`)
and component classes (`internal/ui/assets/intents.css`) directly in Go
source, written to those two files when the script runs. `--liro-color-brand`
and `--liro-color-brand-hover` are the real Liro turquoise already
established in this project (D-070's `#038387`, the visible signature
stamp's logo colour); every other value is a reasonable placeholder.

**Why.** F5 §4.1 describes the intended pipeline: "`@liro/tokens`
generates a `tokens.css`... Add `scripts/synctokens` which pulls the
generated `tokens.css` and the intent classes into `internal/ui/assets/`."
`@liro/tokens` is a React-monorepo package (SPEC §10 states directly
that the agent "has no dependency on the Liro Design System — that is a
React monorepo and cannot run here") and is not present anywhere in
this environment — there is nothing for `synctokens` to pull from. F5
§4.2's actual, checkable requirement — every colour, spacing and
typography value the agent's pages use is a `var(--liro-*)` custom
property in one committed file, with a hex literal anywhere else in
agent CSS failing the build (`scripts/checkcss`, tested) — does not
depend on where the values originally came from, so `synctokens`
satisfies it by being the source of record itself, exactly as SPEC §0
directs when a stated mechanism's prerequisite is unavailable: choose
the simplest option that still meets the actual, checkable requirement,
and record why.

**Rejected.**
- **Hand-writing `tokens.css`/`intents.css` directly, with no
  `scripts/synctokens` at all.** Rejected: F5 §4.1 and the exit
  checklist both require the script to exist ("`scripts/synctokens`
  exists; output committed"), and keeping the values in Go source
  behind a script — rather than editing the generated CSS files by
  hand, which their own header comment explicitly warns against — is
  what actually makes them mechanically regenerable once a real
  `@liro/tokens` package exists: replacing this script's body with a
  real fetch-and-copy step is a self-contained change that touches
  nothing downstream.
- **Blocking F5 on the design system becoming available.** Not a
  choice available in this environment, and contrary to SPEC §0's "do
  not silently work around a constraint... raise it in `decisions.md`"
  — raised here, not worked around silently, and every value is clearly
  a placeholder pending the real package.

---

## D-087 — What the F5 report claimed versus what the first real run showed, and why the verification approach changes as a result

**Date:** 2026-09-03
**Phase:** F5 (post-shipping-run fixes; not F6)

**What was claimed.** The F5 report stated the consent flow was
verified through "window creation, certificate list rendering, cancel
handling, and a correctly-chained audit entry," that the three windows
were trilingual, and that `sign --interactive` existed and worked.

**What the binary actually did, measured directly.** Every window
opened with correct native chrome (title bar, turquoise buttons, tray
tooltip) but every `data-i18n` label was blank in all three locales —
only the native window title and one hard-coded `<option>` rendered.
`liro-bridge sign --interactive` appeared to fail outright.
`liro-bridge certs` and `sign --interactive` both hung indefinitely
with no output. Clicking Approve, Cancel (the button, not the window's
close box), Settings' Save, and every other page-driven button did
nothing observable in the running program. Two tray menu items
(Certificates, View audit log) did nothing at all, and the third
(Open) was indistinguishable, by appearance, from a working one.

**Root causes, each verified independently against the running
binary before being called a fix:**

1. **Empty window text.** `NewWindow`'s `setUpWebView2`
   (`internal/ui/window_windows.go`) called `Navigate` and returned
   immediately; the caller's first `PostJSON` (carrying every
   localised string) then ran `ExecuteScript` before the page's own
   `bridge.js`/`<page>.js` had loaded, so `window.__liroReceive`
   did not exist yet and `bridge.js`'s `window.__liroReceive &&`
   guard silently swallowed the entire init message. Fixed by
   subscribing `ICoreWebView2::add_NavigationCompleted` before
   `Navigate` and pumping the message loop until it fires
   (`navigationCompletedHandler`, `webview2_windows.go`) —
   `NewWindow` now never returns until the page has actually
   finished loading. Verified by rendering the real Settings window
   in all three locales (screenshots taken of the running binary)
   before and after the fix, and by the new
   `TestSettingsWindowRendersLocalisedText`
   (`cmd/liro-bridge/render_windows_test.go`), which creates a real
   WebView2 window, posts the real init payload, and reads
   `textContent` back through `Window.Eval` — the class of test F5
   §10 called for and never had.

2. **`sign --interactive` "not found."** The flag *was* implemented —
   `cmd/liro-bridge/interactive_windows.go` and `interactive_other.go`
   exist, uncommitted but present in the working tree, and `main.go`
   already dispatches `sign --interactive` to it. It was not lost.
   Running it, though, hung before any window could appear:
   `internal/keysource/windowscng`'s per-certificate presence probe
   (`probePresence`, `conn_windows.go`, D-077's own SPEC §11.10
   fix) called `CryptAcquireCertificatePrivateKey` without
   `CRYPT_ACQUIRE_SILENT_FLAG`. SPEC §11.10 states plainly that
   "opening a key never prompts for a PIN, only signing does" — that
   claim does not hold for every certificate on this machine: for two
   of the four enumerated certificates, the call silently blocked
   waiting on an OS credential/insert-card prompt that nothing was
   watching for, hanging `certs` and `sign --interactive` identically
   and indefinitely (a scratch program built to call
   `windowscng.Enumerate`/`Presence` directly, bypassing the CLI,
   reproduced the same hang against the same certificate, isolating it
   from anything UI-related). The historical log line
   `"CryptAcquireCertificatePrivateKey: The action was cancelled by
   the user"` from an earlier manual session is that same UI having
   been shown once before and dismissed by a person. Fixed by adding
   `cryptAcquireSilentFlag` to the probe's flags and treating both
   `NTE_SILENT_CONTEXT` and the newly-observed `SCARD_E_NO_SMARTCARD`
   as "not present" rather than an error or a block (`errors.go`,
   `session_core.go`, `conn_windows.go`). Verified by rerunning the
   same reproduction (now returns in single-digit milliseconds for
   every certificate) and by running `certs` and
   `sign --interactive` to completion in the built binary.

3. **Approve, Cancel-by-button, Save and every other page-initiated
   action did nothing.** `bridge.js`'s `liroSend` called
   `window.chrome.webview.postMessage(JSON.stringify(msg))`. WebView2's
   `postMessage` already serialises an object argument to JSON on the
   native side (`WebMessageAsJson`); handing it an already-stringified
   payload makes the native side see a JSON *string* and re-encode
   that, so `internal/ui.ParseMessage` received
   `"{\"type\":\"approve\"}"` (a JSON string containing escaped JSON)
   instead of `{"type":"approve"}`, failed to unmarshal it into
   `rawMessage`, and silently dropped it — logged only as "not one of
   approve/cancel/selectCertificate," never surfaced anywhere a person
   would see it. This affected every page-driven action in every
   window: Approve, Cancel via its button (window-close still worked,
   because that path is a synthetic Go-side event, `OnClosed`, that
   never goes through `postMessage` at all — which is likely why the
   report's "cancel handling" claim looked true to whoever tested it),
   Settings' Save/Close/Export/Check-updates buttons, and the new
   Certificates/Audit-log windows' Close buttons. Fixed by passing the
   object directly: `postMessage(msg)`. Verified two ways: a new
   permanent test, `TestConsentWindowRendersAndRoundTrips`
   (`cmd/liro-bridge/consent_render_windows_test.go`), which creates a
   real consent window, clicks a certificate row and Approve via
   `Window.Eval`, and asserts Go actually receives
   `selectCertificate`/`approve` (it failed with the bug present,
   caught it directly, and passes now) — and a real mouse click on the
   real Cancel button in the running binary, which produced a genuine
   `audit.Store` entry with `outcome: "denied"`.

**What changed in the verification approach.** Every fix above was
confirmed against the *running binary* — a real WebView2 window,
launched and driven with real Win32 input or `Window.Eval`, its actual
rendered DOM or actual audit-log output inspected — not only against a
test that exercises Go-side data construction. That is a deliberate,
permanent change, not a one-off: `TestSettingsWindowRendersLocalisedText`
and `TestConsentWindowRendersAndRoundTrips` are now committed as
regression tests that create real windows and would have caught both
the empty-text bug and the dropped-message bug before either shipped.
This is the second time in this project that green tests accompanied a
broken product — the first was the xref parser silently losing 92% of
a real document's objects (an earlier phase) — and in both cases the
common thread is the same: a test that exercises the logic beneath a
boundary is not evidence about what crosses the boundary itself. The
boundary here is Go code executing inside an embedded browser engine;
nothing short of actually running it proves anything crossed correctly.

**Rejected.**
- **Trusting the F5 report's claims and only re-running its unit
  tests.** This is exactly what let all of the above ship. `go test
  ./...` was green throughout; none of it caught any of the four
  defects above, because none of the existing tests created a real
  window.
- **Assuming the F5 report's testing claims described a deliberate lie
  or fabrication.** Nothing found supports that: `interactive_windows.go`
  is real, substantial, correctly-structured code, and the underlying
  bugs are exactly the kind that manual testing under time pressure
  plausibly misses (a hang that looks like "the flag doesn't exist" if
  the tester's patience or timeout ran out before minutes had passed;
  a "cancel works" claim that is true for window-close and silently
  false for the button). The more useful lesson is procedural, not
  characterological: CI's own construction (D-088, next) made it
  structurally impossible for automated testing to have caught any of
  this, so a human's manual pass was the *only* check standing between
  these bugs and a report calling them fixed — and manual passes miss
  things reliably, which is the entire reason this project's other
  phases lean so heavily on automated, binary-level verification
  (golden files, an independent CMS verifier, OpenSSL round-trips).

---

## D-088 — CI never built or ran a single `*_windows.go` file; added a `windows-latest` job

**Date:** 2026-09-03
**Phase:** F5

**Decision.** Added a second CI job, `windows`, running on
`windows-latest`, alongside the existing `ubuntu-latest` job. It runs
`go vet` and the full test suite with `-tags softtoken` — the same tag
the ubuntu job's sign-digest/PDF-signing steps already depend on to
test signing with no hardware present (F2 §3) — so the new
`TestSettingsWindowRendersLocalisedText` and
`TestConsentWindowRendersAndRoundTrips` (D-087) can exercise a real
consent window, a real certificate list, and (via the soft token) a
real completed signature, entirely inside CI.

**Why.** Every one of D-087's four defects lived in a `*_windows.go`
file or a page asset only a Windows-built binary exercises.
`internal/ui` is a Windows-only package (SPEC §11.11's Windows-only
scope for phases 1–10); on `ubuntu-latest`, `go build ./...` and
`go test ./...` silently skip every file carrying a `windows` build
constraint — not a partial check, a *complete absence* of one. The
existing CI's "build windows/amd64" step cross-compiles the package
(`GOOS=windows go build`) but a cross-compiled binary is never
executed by anything, so it proves only that the Go compiler accepts
the syntax — the exact gap that let a race condition in `Navigate`
timing, a`CRYPT_ACQUIRE_SILENT_FLAG` omission, and a
`JSON.stringify` double-encoding bug all ship with a fully green CI
run. This is the direct, mechanical answer to "what changed in the
verification approach" (D-087): it is not only that fixes are now
checked against a running binary by hand, but that two of those
checks are now permanent, automated, and run on every push.

**Rejected.**
- **Leaving CI as ubuntu-only and relying on manual runs before every
  release.** This is what produced the F5 report's false claims in the
  first place (D-087) — a manual pass is exactly the check that missed
  all four defects once already.
- **Running the full existing ubuntu job's steps a second time on
  Windows (golden-file signing, fuzzing, cross-compilation).** None of
  those exercise anything Windows-specific — they already run
  correctly on ubuntu and gain nothing from repetition on a slower,
  more expensive runner. The new job is deliberately narrow: vet plus
  the full test suite (which now includes the two new window-driving
  tests), nothing duplicated.
- **Requiring real smart-card hardware in CI to test the consent/sign
  window end to end.** Not available in a hosted runner, and not
  necessary: the soft token (F2 §3) already exists precisely so the
  signing pipeline can be exercised with no hardware, and
  `windows-latest` ships the WebView2 Evergreen Runtime (bundled with
  Microsoft Edge) every window in this project depends on.

---

## D-089 — Certificates and Audit log tray items are built, not stubbed; Open is disabled rather than silently inert

**Date:** 2026-09-03
**Phase:** F5

**Decision.** The tray's "Certificates" and "View audit log" items
(previously a `slog.Info` placeholder each, doing nothing visible) now
open real windows: `certificates.html`/`.js` lists every enumerated
certificate using the same `consent.BuildCertificateOptions` view
model and the same per-row rendering (name, role, issuer, thumbprint
tail, qualified badge, disabled reason, test-key badge) the consent
window already uses — factored into a shared `buildCertOptions`
(`cmd/liro-bridge/ui_payloads.go`) so the two never drift apart —
and `auditlog.html`/`.js` is a plain, newest-first list of
`audit.Store.All()`'s entries (timestamp, outcome, application,
document count, thumbprint tail, test-key marker), reusing the same
`*audit.Store` the Settings window's "Export audit log" button already
opens. "Open" remains a genuine F6 placeholder (the main drag-and-drop
window does not exist yet) but is now added to the tray menu with
`MF_GRAYED | MF_DISABLED`
(`appendMenuItemDisabled`, `internal/ui/tray_windows.go`) instead of
`appendMenuItem` — visibly present, visibly inactive, rather than
indistinguishable from a working item that happens to do nothing.

**Why.** The F5 review states the requirement directly: a menu item
that "appears enabled and produces no response reads as a broken
program," and names Certificates specifically as "a small window over
data that is already computed" — true, since every field it needs
already exists as `classify.Info`/`consent.CertificateOption`. Building
both real windows, rather than disabling them too, follows the
review's explicit preference ("Building them is preferred") and cost
almost nothing beyond the certificates window given the existing
`consent` view-model code; the audit window is new but small (SPEC
§6.7's `audit.Entry` already carries every field the plain list needs).

**Rejected.**
- **Disabling Certificates and Audit log in the menu instead of
  building them**, the review's own fallback option. Rejected because
  it was strictly more expensive than building them: disabling still
  requires touching `showMenu`, and the certificate/audit data was
  already fully computed and rendering-ready via existing view models.
- **A richer audit log window** (filtering, search, export button
  inline). Rejected as scope beyond "a plain list" (the review's own
  phrase) and beyond what Settings' existing Export button already
  covers.

---

## D-090 — The tray icon is generated from `assets/signature-logo.js` via a new `scripts/genicon`, not hand-drawn or left as `IDI_APPLICATION`

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `scripts/genicon` decodes the same real Liro mark
`scripts/genlogo` already extracts for the PDF visual stamp
(256×256, turquoise `#038387`, raw RGB + 8-bit alpha, zlib-compressed)
and writes a standard multi-image `.ico`
(`internal/ui/assets/icon.ico`, embedded and committed) containing that
mark downsampled to 16/20/24/32/40/48/64/256 px — every size Windows
asks a tray or shell icon for across 100–200% DPI scaling — each frame
PNG-encoded (the format every icon-loading Windows API has accepted
inside `.ico` since Vista). Downsampling uses a hand-written
area-weighted box filter (`boxResize`): correct, alias-free downscaling
for a flat-shaded mark, needing nothing beyond `image`/`image/png`
from the standard library. `internal/ui/icon_windows.go` extracts the
embedded bytes to a stable per-user path (mirroring
`loader_windows.go`'s `ensureLoaderExtracted` exactly) and loads it
with `LoadImageW(..., LR_LOADFROMFILE)` at the current DPI's
`SM_CXSMICON`/`SM_CYSMICON` size, falling back to the previous
`IDI_APPLICATION` placeholder only if extraction or loading fails.

**Why.** F5 shipped `IDI_APPLICATION` — the generic Windows application
icon — with a comment explicitly deferring the real asset to F10's
packaging phase; the review asks for it now, from the same source
`genlogo` already uses, so there remains exactly one place the mark's
pixels live. Reproducing it from `assets/signature-logo.js` rather than
drawing a new icon by hand keeps the tray icon and the PDF stamp's logo
provably the same mark, and keeps the asset regenerable the same way
`genlogo`'s own doc comment already establishes as this project's
convention for this source file.

**Verified**, not merely built: `LoadImageW` (the exact API
`loadTrayIcon` calls) was used directly, outside the running agent, to
load every frame size from the generated file and render it to a
bitmap — confirming the `.ico` is well-formed and Windows' real icon
loader accepts every frame, independently of any bug in
`icon_windows.go` itself. The running tray process was then confirmed
to register the icon under the tooltip "Liro Bridge dev" via UI
Automation against the live notification-area overflow flyout (a more
reliable check than locating the icon's pixels in a screenshot, which
Windows' overflow behaviour made unreliable during testing).

**Rejected.**
- **`System.Drawing.Icon`'s multi-size constructor as the verification
  tool.** It threw `ArgumentOutOfRangeException` on the 256×256 PNG
  frame specifically — a known .NET/GDI+ legacy-loader limitation with
  large PNG-compressed ICO frames, unrelated to the file's validity:
  `LoadImageW`, the actual Win32 API the tray uses, loads every frame,
  including 256×256, without error. Verifying against the wrong API
  would have reported a false failure.
- **Embedding the icon as a Windows resource (`rsrc.syso`) instead of
  extracting the embedded bytes to disk.** SPEC §5 already names
  `rsrc.syso` for the *executable's own* icon/manifest, generated at
  F10's packaging step — out of scope here, and the tray icon
  (`Shell_NotifyIconW`) needs a loadable `HICON` at runtime regardless
  of what the .exe's own resource section carries, so the
  embed-then-extract approach `loader_windows.go` already established
  for `WebView2Loader.dll` was reused rather than introducing a second
  mechanism.
- **Bilinear or nearest-neighbour resizing.** Rejected in favour of
  the box filter: both alias visibly on a mark with hard geometric
  edges at the largest downscale ratios (256→16), and a correct area-
  weighted average costs nothing extra in a dependency-free
  implementation.

---

## D-091 — TSA Basic-auth and client-certificate credentials are configuration fields, stored and transmitted the same way the CLI's equivalent flags already were

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `config.Config` gains four fields — `TSAUser`,
`TSAPassword`, `TSAClientCertPath`, `TSAClientCertPassword` — mirroring
the CLI's existing `--tsa-user`/`--tsa-password`/`--tsa-client-cert`/
`--tsa-client-cert-password` flags (F3). Settings gains four
corresponding fields (`tsa-user`, `tsa-password`, `tsa-client-cert-path`,
`tsa-client-cert-password`), the two password fields as
`<input type="password">`. A new `buildTSAClient` in
`cmd/liro-bridge/interactive_windows.go` reads them into a `tsa.Auth`
exactly the way `internal/cli.RunSign` already does for the CLI path,
replacing `sign --interactive`'s previous `tsa.Auth{}` — always empty,
so the interactive path silently sent no credentials at all even when
a TSA required them.

**Why.** The review's own reasoning: Pošta's public test TSA — the
only timestamp authority actually reachable for testing (SPEC §12.7)
— requires HTTP Basic credentials on one endpoint and a client
certificate on the other, and production TSAs generally require a
client certificate too. The CLI already had all four flags; Settings
and `config.Config` did not, so a person using the agent's own window
rather than the CLI had no way to reach a timestamp authority beyond
Pošta's non-existent unauthenticated option. Storing them as plain
`config.Config` fields (not behind `internal/platform`'s DPAPI-backed
`SecretStore`) matches how `TSAURL` and every other Settings-owned
value already persists — SPEC §6.4's DPAPI requirement is scoped
explicitly to the API pairing device secret, not every credential the
agent ever holds, and introducing a second, differently-secured
storage mechanism for these four fields alone (while leaving the rest
of `config.Config` as plain JSON) was judged more inconsistent than
useful without a stated requirement to do so — raised here rather than
silently assumed either way (SPEC §0).

**Verified in the running binary**, not only by code inspection: typed
a password into the Settings window, confirmed it renders masked
(WebView2's native reveal-password eye icon present, dots shown),
clicked Save, confirmed `config.json` on disk contains the value
verbatim, and grepped the log file for the literal password —
zero matches, confirming SPEC's "never appear in any log line"
requirement holds for the actual save path, not just by inspection of
which `slog` calls exist today.

**Rejected.**
- **Routing the TSA client-certificate password (or the Basic-auth
  password) through `internal/platform.SecretStore`.** Rejected per
  the "why" above: no other Settings-owned value uses it, and SPEC
  §6.4 scopes it to the pairing secret specifically; adding it here
  alone, with nothing else in `config.Config` following the same
  pattern, would be a larger and more inconsistent change than the
  task asked for.
- **A native file picker (`GetOpenFileNameW`) for the client
  certificate path.** Rejected as scope beyond "add all four to the
  configuration and to Settings" — a plain text field for a path is
  consistent with every other Settings field's own input style
  (`tsa-url`, `output-suffix`) and required no new Win32 surface.

## D-092 — `--help`/usage text is English only; SPEC §9.2's en/localised split gains a third category

**Date:** 2026-09-03
**Phase:** F5

**Decision.** `liro-bridge --help` (and the bare, no-argument
invocation, which prints the same thing) is English, unconditionally —
it no longer calls `i18n.Load` at all. The top-level command list and
synopsis in `cmd/liro-bridge/main.go`'s `topLevelUsage` are now plain
string literals instead of catalogue keys, and the seven `cli.help_*`
keys are removed from all three locale catalogues (`en.json`,
`sr-Latn.json`, `sr-Cyrl.json`) — they had no other caller. SPEC §9.2
is amended: it previously drew one line ("CLI output is localised; log
files are not"). The line now reads:

> Code, comments, documentation, help and usage text are English.
> Everything a non-developer reads at runtime — CLI *output* (certs
> listings, error messages), window text — is localised in all three
> catalogues. Log files are English regardless, for the reason already
> given (read by developers).

This also removes the second, hand-rolled `"Usage of %s:\n"` block
(`fs.PrintDefaults()`, listing only `-version`) that `topLevelUsage`
printed after the command list — it was a second, differently
formatted "usage" header in the same output, and everything it added
over the synopsis above it was one flag a user can just as well
discover by running `liro-bridge --version`. `TestHelpFlag` and
`TestNoArgsPrintsUsageAndExitsZero` (`cmd/liro-bridge/main_test.go`)
are updated to assert on `"Usage: liro-bridge <command> [flags]"`
instead of the removed `"Usage of liro-bridge"` string.

**Why.** The owner's own framing, from an actual run: `--help` mixed
Serbian ("Komande:", localised one-line descriptions) with the
standard library's English `flag` package output in the same block —
readable to no single audience. Every subcommand's own `--help`
(`sign --help`, `certs --help`, ...) was already English by construction
(none of them set `fs.Usage`, so they fall through to `flag`'s own
English default) — only the top-level command was inconsistent with
itself. Help and usage text is developer-facing in the same sense code
comments are: it explains *how to invoke the program*, not something a
signer reads mid-signature. SPEC §9.2 previously had no place to say
this because it only ever distinguished "CLI output" (assumed to mean
everything the CLI prints) from "log files" — this decision narrows
"CLI output" to mean the certs listing, error messages and other
*runtime* text a user reading their own certificates or a failure
sees, and gives help/usage its own, English-only rule alongside code
and documentation.

**Verified in the running binary**, not only by the updated tests:
built the binary and ran `liro-bridge --help`, `liro-bridge`, and
`liro-bridge sign --help` with `LIRO_LOCALE`/config locale set to
`sr-Cyrl` — every line of `--help`/bare-invocation output is English,
every line of `sign --help` was already English, and `liro-bridge
certs` (with no certificates present) still prints in Cyrillic,
confirming runtime CLI output is untouched by this change.

**Rejected.**
- **Keeping the command descriptions localised and only fixing the
  duplicate flag block.** Rejected: the owner's decision was explicit
  that help/usage is English "like everything else developer-facing,"
  not merely that the two usage blocks look inconsistent with each
  other — leaving the command list localised would still mix English
  and Serbian in the same `--help` output.
- **Leaving `cli.help_flags_heading` and `cli.help_flag_version` in the
  catalogues, unused, in case a future flags section wants them.**
  Rejected: they were already dead (never referenced by any code path
  even before this change) and would now additionally contradict the
  "help text is English" rule if anyone ever wired them back in;
  removed rather than left as a trap.

## D-093 — Outcome colours come from `IntentFamilyColor`, a single generated map, never chosen per call site

**Date:** 2026-09-03
**Phase:** F5

**Decision.** The certificates and audit log windows render an outcome
(and, more generally, any future status this project wants to colour
the same way) as a single word, right-aligned, in one of exactly four
colours: positive, warning, negative, caution. The mapping from those
four names to tokens lives in exactly one place —
`IntentFamilyColor` (`scripts/synctokens/intents.go`), a
`map[string]string` from intent name to `--liro-color-*` token name —
and `intentOutcomeCSS` generates the four `.liro-outcome-<name>` CSS
rules directly from it (sorted by name for a stable diff), appended to
the generated `intents.css`. Four new tokens back it:
`--liro-color-positive` (green), `--liro-color-negative` (the existing
danger red, aliased under its intent name), `--liro-color-caution`
(orange, distinct from warning), and the existing `--liro-color-warning`
(amber) is reused rather than duplicated.

`internal/ui` gains a small, dependency-free `Intent` string type
(`IntentPositive`/`IntentWarning`/`IntentNegative`/`IntentCaution`) so
Go call sites — `cmd/liro-bridge/auditlog_windows.go`'s new
`outcomeIntent(audit.Outcome) ui.Intent` — have a typed value to pass
through the JSON payload (`jsAuditEntry.OutcomeIntent`) rather than a
bare string assembled ad hoc at each call site. The mapping
`approved`->positive, `denied`->warning, `failed`->negative,
`partial`->caution is fixed by `outcomeIntent` alone; nothing else in
`cmd/liro-bridge` or the page JS decides an outcome's colour.

**Why.** Two independent problems, one cause. First, every button in
the agent's windows was rendered in `--liro-color-brand`, which was set
to the real Liro turquoise (`#038387`, D-070's logo colour) — turquoise
read as "green" to an actual person clicking Approve/Save, even though
no literal hex anywhere said so (`scripts/checkcss` was never the gap;
the *token itself* was wrong). `--liro-color-brand` is now `#0078D4`,
the design system's actual brand blue; the turquoise stays exactly
where D-070 put it (`internal/pades/appearance`, `scripts/genicon`,
`scripts/genlogo`) and is never read by this token pipeline again.
Second, the audit log window coloured every non-`approved` outcome
identically (`liro-badge-warning`, regardless of whether the batch was
merely denied, partially failed, or failed outright) — a user could
not tell "cancelled, no big deal" from "failed, something is wrong" at
a glance, which is the entire point of colouring an outcome at all.
Naming the mapping `IntentFamilyColor` and generating the CSS from it,
rather than hand-writing four `color: var(...)` rules and a parallel
Go `switch`, keeps the two from drifting apart the way the four
outcomes and one badge class already had.

**Verified in the running binary**, not only by
`scripts/synctokens`'s own new tests (`TestIntentFamilyColorHasExactly...`,
`TestIntentOutcomeCSSGeneratesOneRulePerIntent`): built four audit log
entries (approved, denied, failed, partial — `internal/audit.Entry`
written directly to a test profile's audit store) and opened the Audit
log window; each outcome renders as one right-aligned word in a
visibly different colour (green/amber/red/orange), and every button in
every window (consent, pairing, settings, certificates, audit log) is
the same blue, with no green anywhere. `go run ./scripts/checkcss`
stays green.

**Rejected.**
- **Reusing `--liro-color-danger` for `negative` instead of adding
  `--liro-color-negative` as its own token.** Rejected: `danger` is
  already load-bearing elsewhere (`.liro-error-box`, disabled-reason
  text) for a general "something is wrong" meaning unrelated to the
  four-way outcome taxonomy; giving the outcome-negative case its own
  name in `IntentFamilyColor` keeps that map self-describing even
  though the two tokens happen to share a value today.
- **A hand-written `switch` in JS mapping `entry.outcome` strings
  straight to CSS class names, with no Go-side `Intent` type.**
  Rejected: it would have put the outcome -> colour-family decision in
  the one layer (page JS) explicitly forbidden from choosing its own
  colours, and duplicated the same four-way mapping a second time
  instead of computing it once in Go and handing the page an
  already-resolved intent name.

## D-094 — Visual verification never simulates mouse or keyboard input on this machine; a screenshot needing an interaction asks the owner to perform it

**Date:** 2026-09-03
**Phase:** F5

**Decision.** Verifying a window "by looking at it" means launching the
real (or a throwaway preview) binary and taking a screenshot of it —
nothing more. `SetCursorPos`/`mouse_event` (or any other
programmatic-input API) are never used to drive a window during
verification on this machine, for any reason, at any level of care.
When a screenshot genuinely needs an interaction to be meaningful — a
selected certificate row so the enabled Approve button's colour is
visible, for instance — the assistant pauses and asks the owner to
perform that one click, then screenshots, rather than simulating it.
This applies beyond this task: the same judgement — stop and ask
before any action that would reach outside the project's own files and
windows — extends to any future verification step with similar reach.

**Why.** While verifying Task 4's consent-window layout, a
`mouse_event` call intended to click a certificate row in a throwaway
preview window instead landed on the owner's real, live desktop — the
foreground window at the moment the click actually fired was a browser
tab playing an unrelated YouTube video, not the preview window.
`SetCursorPos`/`mouse_event` are global on Windows: there is no
mechanism that confines them to one process's window, because which
window is in the foreground depends on a race against everything else
running on the machine at that instant. This was a browser tab; the
same race could as easily land a synthetic click on a delete
confirmation, a "send" button, or — directly relevant to this
project — the operating system's own PIN dialog (Task 2), where a
synthetic Approve/OK means a qualified signature the certificate's
owner never actually authorised. No amount of "target only the test
window's HWND" care closes that gap, because the click is delivered to
whatever the OS foreground is at the moment it fires, not to whatever
this process most recently called `SetForegroundWindow` on — the two
can and did diverge. The risk is categorically unacceptable, not a
matter of degree, so it is not something a more careful implementation
of the same approach can fix.

**Verified.** N/A — this is a process decision about how verification
itself is done, not a code change.

**Rejected.**
- **Simulating input more carefully (e.g. re-checking the foreground
  window immediately before each `mouse_event` call).** Rejected per
  the "why" above: the race is inherent to global input injection on
  Windows, not a bug in any particular calling pattern, so no amount of
  additional care around it removes the risk.
- **Confining verification to Go-level/WebView2 `Eval`-driven clicks
  only (as the existing `*_windows_test.go` suite already does via
  `Window.Eval("...click()")`), and calling that sufficient without
  ever screenshotting.** Rejected: an `Eval`-driven click is safe (it
  runs inside the target page's own DOM, never touches the real
  cursor) and remains the right tool for automated tests, but the
  actual task at hand — confirming what a window *looks like* — still
  requires a real screenshot; the fix is "screenshot without
  synthetic input," not "skip the screenshot."

---

## D-095 — No default timestamp authority; the consent window asks the user instead of refusing; RS-GOV TSA's endpoint is `https://tsa.gov.rs/`, contract-gated, and was unreachable when probed

**Date:** 2026-09-04
**Phase:** F5 — second real-run review (Task 1)

**Decision.** Three parts, one subject.

*(a) There is no default timestamp authority.* `config.Default().TSAURL`
stays empty, and nothing in this project contacts a timestamp authority
that the user did not configure. [[D-067]]'s reasoning for not
hard-coding a live government endpoint stands unchanged; what changes
is what happens next.

*(b) The consent window asks.* When a batch is about to be signed and
either no TSA is configured, or a configured one did not answer after
`internal/pades/tsa`'s existing retry policy ([[D-045]]: three
attempts, 15 s each, 1 s then 3 s backoff), the consent window enters a
new state (`consent.StateTSAChoice`) offering exactly three actions:

- **Sign without a timestamp** — proceeds at B-B.
- **Configure a timestamp authority** — opens the Settings window,
  re-reads the configuration when it closes, rebuilds the TSA client,
  and continues at the requested level if one is now configured; asks
  again if not.
- **Cancel** — nothing is signed; the audit entry records `denied`.

The achieved level is then reported in all three places a user could
look: on the consent window's done screen (`consent.level_bb`, "Nivo
B-B — bez vremenskog žiga", in the warning intent family), in
`pades.Result.AchievedLevel` as it always was, and in the audit entry,
which gains an `AchievedLevel` field carrying the *weakest* level any
document in the batch reached. The audit-log window renders it, marked
in the warning family when it is B-B.

The question is asked *before* the card session is opened, so a user
who cancels never enters a PIN for a batch that was never going to be
saved. A TSA that fails mid-batch asks the same question, then retries
that document — the batch is not abandoned (SPEC §12.10).

`--on-tsa-failure`'s command-line default stays `abort`. The finding
this fixes is "the user is given no way forward from inside the
application"; on the command line the flag *is* the way forward, it is
documented in `--help`, and a script that silently starts emitting B-B
where it used to emit B-LT is a worse outcome than one that stops and
says why.

*(c) Settings offers two named presets plus the free-text URL.*
Neither is preselected: a preset radio is checked only when the saved
URL is byte-identical to that preset's own, so the factory-default
(empty) configuration selects nothing, and a hand-typed URL never
silently claims to be one of them. The URL field remains the only thing
actually saved; a preset is a shortcut for filling it in.

- **freetsa.org** — `https://freetsa.org/tsr`, labelled exactly "nije
  kvalifikovan u Srbiji" / "not qualified in Serbia" / "није
  квалификован у Србији" and nothing more.
- **Office for IT and eGovernment (RS-GOV TSA)** —
  `https://tsa.gov.rs/`, with the note that it requires an approved
  request to the Office and a client certificate.

**Verified.** Measured, not assumed:

*The authority that timestamped `testdata/pdfs/local/mup.pdf`.* Both of
that document's CMS blobs were extracted and every embedded certificate
parsed. The timestamp token's signer is
`CN=RS-GOV TSA-3 200103828, O=Kancelarija za informacione tehnologije i
elektronsku upravu, SERIALNUMBER=CA:RS-200103828`, `extendedKeyUsage`
`timeStamping`, issued by `CN=Pošta Srbije CA 1`, certificate policies
`0.4.0.194112.1.3` and `1.3.6.1.4.1.15672.10.812.1.0`. Both tokens
carry the TSTInfo policy OID `1.3.6.1.4.1.55016.1.1.0` — exactly the
OID SPEC §12.7 names for this provider, confirmed by searching the
decoded DER for its encoding `06 0B 2B 06 01 04 01 83 AD 68 01 01 00`.
No certificate in the document carries a URL for the TSA *service*;
the AIA and CRL URLs all point at Pošta's repository, so the endpoint
could not be derived from the fixture.

*The endpoint.* The Office's own eUprava service page for the service
(euprava.gov.rs/usluge/1635) states the address as `https://tsa.gov.rs/`,
that client applications must support certificate authentication, and
that the service is for "државних органа, органа локалне самоуправе и
јавних служби" — state bodies, local self-government bodies and public
services — which must submit a request through the eUprava portal. The
Office's own pages (ite.gov.rs) describe the RS-GOV TSA infrastructure
but publish no endpoint.

*What one RFC 3161 request actually returned.* A well-formed
`TimeStampReq` (version 1, SHA-256 imprint, 8-byte nonce, `certReq`
true — the same shape `internal/pades/tsa.buildRequest` produces) was
POSTed to `https://tsa.gov.rs/` with
`Content-Type: application/timestamp-query`, from this machine, on
2026-09-04. Result:

- The host is live. TLS completes; the server certificate is
  `CN=tsa.gov.rs`.
- The server **does** request a TLS client certificate, and the CA list
  it will accept is every Serbian qualified issuer: Pošta Srbije CA
  Root/CA 1/CA 2/CA Root 2026, MUPCA Root 3 / Resursi 3 / Sluzbenici 3,
  Halcom BG Root CA and its two e-signature CAs, PKS CA Root/Class1,
  E-Smart ESS RQCA/IQCA1, and the Office's own EID RS PRIV Root /
  EID RS Infr Mgt.
- The response was **HTTP 400 Bad Request** with
  `Content-Type: text/html` and a 769 141-byte eUprava-branded error
  page reading "Поштовани корисници, У току је унапређење система.
  Доступност сервиса очекује се убрзо." ("Dear users, a system upgrade
  is in progress. Service availability is expected shortly.") —
  `Response Code: 400`, application `VS-tsa.gov.rs`.
- Not a timestamp token, not an RFC 3161 rejection, not a network
  failure: an HTTP error page from the gateway in front of the service.
- Identical for `GET /`, `POST /`, `POST /tsa/timestamp` and
  `POST /TSAServer`, and identical again on a repeat 20 s later, so it
  is not a transient or a wrong-path artefact.

So: **it does not accept requests without a contract, and at the moment
of probing it was not answering RFC 3161 requests at all.** The preset
still ships, because the task's condition for leaving it out was being
unable to establish the URL — the URL *is* established, by its operator's
own published service description and by a live host presenting a
`CN=tsa.gov.rs` certificate and asking for exactly the client
certificates that page says are required. It ships with a label saying
what it needs, and it is not preselected, so no user reaches it by
accident.

*freetsa.org, for contrast.* The same request to
`https://freetsa.org/tsr` returned HTTP 200,
`Content-Type: application/timestamp-reply`, 4642 bytes, beginning
`30 82 12 1E 30 03 02 01 00 …` — `PKIStatus 0`, granted.

**Rejected.**
- **Configuring `https://tsa.gov.rs/` (or any other authority) as the
  default TSA URL.** Rejected on the owner's own instruction and on
  [[D-067]]'s reasoning, now reinforced by measurement: the endpoint
  requires an approved request to the Office and a Serbian qualified
  client certificate, so as a default it would fail for essentially
  every user of this program, and the failure would be an HTML error
  page rather than anything a TSA client can explain.
- **Leaving the RS-GOV preset out because the probe did not return a
  token.** Considered seriously — the instruction says to ship nothing
  that does not work. Rejected because what the probe established is
  that the *endpoint* is right (correct host, correct certificate,
  correct client-certificate demand) and the *service* is gated and
  currently in maintenance. A user who has the contract and the card
  needs this URL; withholding it would help nobody, and the note on the
  preset states the precondition plainly.
- **Keeping SPEC §12.8's choice at the `--on-tsa-failure` level only,
  as [[D-083]] decided.** Superseded, for this one screen. D-083's
  reasoning was that a mid-failure re-decision had nowhere to travel
  through a three-message page->Go surface without overloading
  `approve` and blurring what the audit log records that click as. That
  is now solved the same way the settings window solved it: the two
  proceeding actions send `approve` and record *which* they were in
  `window.__liroTSAChoice()`, which Go reads back through
  `Window.Eval`'s own return value. The disambiguation is explicit data
  reported by the page, not an inference from which state the window
  happened to be in — which is precisely the thing D-083 rejected. The
  message surface is still exactly three types.
- **Making the timestamp choice a checkbox on the waiting screen ("sign
  without a timestamp") shown up front.** Rejected: it asks every user,
  every time, about a situation most of them will never be in, and a
  pre-ticked or easily-ticked box is how a downgrade becomes routine.
  The question is only worth asking at the moment it actually matters.
- **Recording only a boolean "downgraded" in the audit entry instead of
  the level string.** Rejected: it would lose the B-T/B-LT distinction,
  which is the same question one step further up, and the field costs
  nothing extra to carry as a string.
- **Appending `AchievedLevel` to `audit.Entry.CanonicalBytes`
  unconditionally.** Rejected: it changes the canonical bytes of every
  entry ever written, so every audit log already on disk would read as
  tampered with. Appending it only when set — it is the last field, and
  length-prefixed like every other — leaves a pre-change entry
  canonicalising to exactly the bytes it always did.
  `TestCanonicalBytesWithoutAchievedLevelIsUnchanged` pins that.

---

## D-096 — The batch fingerprint is shown as 16 hex characters plus a Copy action; no value in these windows may widen its container

**Date:** 2026-09-04
**Phase:** F5 — second real-run review (Task 2)

**Decision.** `consent.ViewModel` now carries two forms of the batch
fingerprint: `Fingerprint` (the full 64 hex characters, unchanged) and
`FingerprintShort` (`ShortFingerprint`: the first 16 characters plus
`...`, the same three-ASCII-period mark [[D-057]] established). The
consent window renders only the short form, in monospace, with a
compact "Copy" button that puts the full value on the clipboard. The
full value never appears in the DOM.

Separately, every value in these windows that has no length bound now
wraps rather than overflows: `.liro-row > *` gets `min-width: 0` (a
flex item's default `min-width: auto` is what actually refused to
shrink), and `overflow-wrap: anywhere` is applied to certificate names
and meta lines, the error box, the fingerprint, the output path and the
done summary.

**Why.** SPEC §6.6 says the fingerprint exists "so a technical user can
verify what was approved against what the calling application says it
sent". A clipboard copy serves that comparison exactly; sixty-four
characters rendered on screen do not — nobody compares a hex digest by
eye, and the string has no break opportunities, so it pushed the
Details card, the row, and the window's whole layout apart.

**Verified.** `TestConsentDetailsFingerprintDoesNotWidenItsContainer`
renders the Details section in a real WebView2 window at the size
`runSignInteractive` uses, measures the card's
`getBoundingClientRect().width` with no fingerprint and again with the
full 64-character one from the report
(`d54aeba8571c16922cb7cd1f6b758824bc7b26e6865daa4bba47be47c906135e`),
and asserts they are equal — plus that neither the body nor the card
scrolls horizontally, that the rendered text is the elided form, and
that the full value appears nowhere in `document.body.textContent`.
Confirmed to fail against the pre-fix rendering: with the CSS and the
short form reverted, the same test reports "body scrollWidth 583
exceeds clientWidth 520" and "Details card scrollWidth 566 exceeds
clientWidth 486".
`TestConsentLongFileNameDoesNotWidenTheWindow` does the same for a
300-character file name, and
`TestSettingsWindowFitsWithoutHorizontalOverflow` for a 200-character
TSA URL and client-certificate path.

**Rejected.**
- **CSS truncation (`text-overflow: ellipsis`) instead of truncating in
  Go.** Rejected: the full string would still be in the DOM, one
  stylesheet change away from overflowing again, and a Copy action
  would then be copying something the page is pretending is shorter
  than it is. Truncating where the value is computed means the page
  never holds a form it must not render.
- **Wrapping the fingerprint across lines instead of eliding it.** It
  fits, but four lines of hex is noise in a section whose other content
  is the thing the user actually reads (the file names).

---

## D-097 — Export audit log writes to a folder the user chooses and says where; Check for updates says the update channel is not built yet

**Date:** 2026-09-04
**Phase:** F5 — second real-run review (Task 3)

**Decision.** Both Settings buttons now produce visible output.

*Export audit log* opens the OS folder chooser
(`ui.ChooseFolder`, `SHBrowseForFolderW` with
`BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE`, on its own OS thread with
its own OLE apartment), writes `liro-audit-<stamp>.jsonl` and
`liro-audit-<stamp>-report.json` there, and reports the destination
folder on a status line in the window. A cancelled chooser reports
nothing — the user withdrew the request. A chain that fails
verification is reported as its own distinct message, because "the
export failed" and "the export succeeded and the log is broken" call
for different reactions.

*Check for updates* states, on the same status line, that automatic
updates are not available in this version yet. SPEC §15.2's update
channel — the embedded public key, the GitHub Releases check, the
release-signature verification and the ask-before-installing prompt —
is F10's work, built with the packaging it belongs to.

The status line is a new Go->page payload type (`"status"`), not a new
page->Go message type: the three-message surface is untouched.

**Why.** Both handlers already existed and both already ran. Export
wrote its two files into `%LOCALAPPDATA%\Liro\audit\exports\` and
logged the paths; the update check logged a line saying it was not
implemented. On screen, both were indistinguishable from a dead button
— the same objection that applied to the tray menu items in the
previous round ([[D-089]]). A button that appears enabled and produces
no response reads as a broken program.

**Verified.** `TestSettingsStatusLineRendersWhatAnActionDid` posts a
real status through `postSettingsStatus` into a real WebView2 window
and reads the DOM back: the line is hidden before any action, carries
the catalogue's own text afterwards, has a non-zero rendered height,
and takes its colour from the intent family ([[D-093]]) rather than a
colour chosen at the call site. The folder chooser itself was opened
and photographed — parented to the Settings window, with its localised
prompt — see [[D-100]] for how. `audit.Store.Export` writing both files
was already covered by `TestExportWritesEntriesAndReport`.

Confirming that the export *completes* — files on disk, destination
named on screen — needs someone to press OK in that dialog, which
[[D-094]] means is the owner's click, not a simulated one.

**Rejected.**
- **`IFileOpenDialog` with `FOS_PICKFOLDERS`.** The modern API, but
  this package's COM interop is hand-written vtable work ([[D-080]]);
  it would mean several more interfaces implemented by hand for a
  chooser with no requirement `SHBrowseForFolderW` does not already
  meet.
- **Keeping the fixed `exports/` directory and just naming it on
  screen.** Rejected: the task asks for a location the user chooses,
  and an export the user cannot put on a USB stick or attach to an
  email is most of the way to not being an export.
- **Building enough of the update channel to make the button do
  something real.** Rejected as F10's work, and as exactly the
  build-beyond-your-phase move SPEC §0 warns against; saying plainly
  that it is not there yet is what this round was asked for.

---

## D-098 — Every window carries the real Liro mark, at both icon sizes, set with WM_SETICON

**Date:** 2026-09-04
**Phase:** F5 — second real-run review (Task 4)

**Decision.** `internal/ui`'s window creation sends `WM_SETICON` for
both `ICON_SMALL` (the title bar) and `ICON_BIG` (Alt+Tab and the task
switcher), loading the already-embedded `assets/icon.ico` at each
size's own `GetSystemMetricsForDpi` dimensions. `loadTrayIcon` is now a
two-line call into the same helpers.

**Why.** The window class registered in `win32_windows.go` carries no
`hIcon`/`hIconSm`, and nothing ever sent `WM_SETICON`, so every window
showed the Windows placeholder even though the tray icon had come from
the real mark since [[D-090]]. Both sizes are set explicitly because
Windows derives neither from the other: setting only `ICON_SMALL`
leaves Alt+Tab on the placeholder, and only `ICON_BIG` leaves the title
bar on it.

**Verified.** `TestWindowsCarryTheLiroIcon` (cmd/liro-bridge) creates a
real window and reads the icons back out of it with `WM_GETICON` for
both sizes, rather than asserting that `setWindowIcons` was called — a
`LoadImageW` failure returns 0 and is deliberately silent, so "the call
happened" proves nothing about what the user sees.
`TestSystemIconSizeIsPositive` (internal/ui) guards the size
arithmetic, because `LoadImageW` treats a zero dimension as "use the
resource's own size" and silently produces a wrong-sized icon rather
than an error.

The window half of that pair lives in `cmd/liro-bridge`, with every
other window test, and not next to the code it exercises. Written first
in `internal/ui`, it deadlocked `go test ./...`: `go test` runs
packages in parallel, every window in this project opens WebView2
against the one user data folder under `%LOCALAPPDATA%\Liro`, and
WebView2 will not open a second instance of a user data folder from
another process — so `internal/ui`'s window and `cmd/liro-bridge`'s
window blocked on each other inside
`CreateCoreWebView2Controller`, with no error, until the test binary's
own timeout fired. Keeping every window test in one package is what
keeps them serialised.

**Rejected.**
- **Putting the icon on the window class (`WNDCLASSEX.hIcon` /
  `hIconSm`) instead.** It works, but the class is registered once per
  process before any window's DPI is known, so both sizes would be
  resolved at whatever the first monitor's scaling happened to be.
  `WM_SETICON` per window resolves each at that window's own DPI.

---

## D-099 — `ui.NewWindow` intermittently never completes; measured, unfixed, and the window-test suite reshaped around it

**Date:** 2026-09-04
**Phase:** F5 — second real-run review (found while verifying Tasks 1–4)

**Decision.** Recorded as a known defect, not fixed in this pass. Two
things were done about it: every test that opens a WebView2 window now
lives in `cmd/liro-bridge` and shares one consent window and one
settings window between them, and this entry states the measurement so
the next person does not rediscover it from scratch.

**Why — what was measured.** `go test ./cmd/liro-bridge/` intermittently
hangs. Every hang has the same shape: a goroutine parked in
`pumpUntil` inside `environmentCreateController` (occasionally
`createEnvironment` or `coreWebView2ExecuteScript`), waiting for a
WebView2 completion handler that never fires, until the test binary's
own deadline kills it. No error, no HRESULT, no log line.

Rates measured on this machine, this build, with nothing else of ours
running:

| What ran | Windows opened | Result |
|---|---|---|
| `TestConsentWindowRendersAndRoundTrips` alone, ×8 | 1 | 7 ok, 1 hung |
| `TestSettingsWindowRendersLocalisedText` alone, ×6 | 1 | 6 ok |
| Consent + settings tests, ×6 | 2 | 5 ok, 1 hung |
| Whole package, ×8 | 2 | 6 ok, 2 hung |

So it is roughly one window creation in twenty to thirty, and it is
present with a single window — it is not caused by opening several.
More windows per run simply means more chances to hit it.

Two hypotheses were tried and both were wrong, which is worth
recording so they are not tried again:

- **"The pump is asleep in GetMessage with the flag already set."**
  Plausible — the completion handlers are raw hand-written COM vtables
  ([[D-080]]) handed to the runtime as bare pointers, so nothing
  guarantees the runtime invokes them on the waiting thread. Replacing
  `GetMessage` with a `PeekMessage` + `MsgWaitForMultipleObjectsEx`
  loop that re-checks the flag every 50 ms, and making the flags
  atomic, changed nothing: 2 hangs in 4 runs, the same as before. The
  completion genuinely never arrives. Both changes were reverted rather
  than left in on a theory that measurement had disproved.
- **"A per-run WebView2 user data folder isolates the tests."** A
  `TestMain` pointing `LOCALAPPDATA` at a fresh temp directory made it
  *much* worse — every run then hung on the very first window. Removed.

A second symptom of the same fragility, seen once: after a run in which
every test passed, the process crashed during teardown with an access
violation inside the WebView2 runtime, reached from
`(*window).closeWebView`'s `ICoreWebView2Controller::Close` — a
controller pointer that was no longer valid. `closeWebView` now zeroes
every pointer it releases, so a second WM_CLOSE from any source is a
no-op instead of a call on freed memory. That is correct regardless of
what sent the second close, which was not established.

One real bug was found on the way and kept: `coreWebView2ExecuteScript`
ended `_ = sBuf` after handing the script's UTF-16 buffer to an
asynchronous call. That is not a use the compiler has to honour, so the
buffer could be collected while ExecuteScript was still reading it; it
is now `runtime.KeepAlive(sBuf)`, along with the same fix for
`coreWebView2Navigate`'s URI and `coreWebView2SetVirtualHost`'s two
strings. It did not fix the hang and was not its cause — it is a latent
defect found by inspection.

**What was actually changed.** The window tests were moved into one
package and reduced from thirteen window creations per run to two, both
shared. That cut the package from 27.7 s to 5.6 s and cut the hang rate
roughly in proportion, without hiding anything: a hang still fails the
run, loudly, at the test binary's deadline.

Not attempted here, and the two candidates for whoever fixes this
properly: creating every window on **one** UI thread with one COM
apartment and one WebView2 environment, instead of this project's
thread-per-window model (F5 §2.1) — the environment is the object the
runtime deduplicates per user data folder, and one environment per
process is what every WebView2 sample does; or giving each completion
wait a deadline so a stuck call reports an error instead of hanging,
which is what SPEC §12.8's "never hang" asks for everywhere else in
this program.

**This is a user-facing defect, not only a test one.** The same code
opens the tray's Settings, Certificates and Audit log windows, and the
consent window. One in twenty-odd of those will not open, and the
program will sit there. It has not been seen in manual use because
manual use opens a handful of windows, not hundreds.

**Rejected.**
- **Retrying `NewWindow` on a timeout inside the test helpers.** That
  hides a defect a user will hit, in the one place that would otherwise
  keep reporting it.
- **Fixing the thread model as part of this pass.** It is a rewrite of
  `internal/ui`'s concurrency design, and this pass is four bounded
  fixes to what the second real run exposed. Recorded here instead.

---

## D-100 — Verification screenshots come from the real binary where a window can be reached without a click, and from a throwaway harness where it cannot

**Date:** 2026-09-04
**Phase:** F5 — second real-run review

**Decision.** [[D-094]] forbids simulating mouse or keyboard input on
this machine, and most of the screens this pass changed are reachable
only by clicking. They were verified like this:

- **The real binary, screenshotted directly.**
  `liro-bridge.exe sign --interactive --in <file>` was run and its
  consent window photographed. That is where the title-bar icon
  (Task 4) was confirmed: the window shows the Liro mark, not the
  Windows placeholder. The certificate list showed the owner's two
  real Halcom certificates, correctly marked unusable with no card in
  the reader.
- **A throwaway test file, created, used and deleted in the same
  session**, holding each window open for six seconds so it could be
  photographed, and driving the page only through `Window.Eval` — which
  runs inside the page's own DOM and never touches the real cursor
  ([[D-094]]'s own carve-out for `Eval`-driven clicks). It used the
  real page assets, the real payload builders
  (`buildConsentInit`, `consentTSAChoicePayload`, `consentDonePayload`,
  `buildSettingsInit`, `buildAuditLogInit`) and the real window host —
  everything except the CLI dispatch that would have required a click
  to reach. Screens confirmed this way: the Details section with an
  elided fingerprint and its Copy action; the timestamp choice screen;
  the done screen stating B-B; the settings window's two presets and
  its status line; the audit log window's level lines; and the folder
  chooser, opened parented to the settings window with its localised
  prompt.

Two windows were resized as a direct result of *looking* at them, which
no test would have caught: Settings from 480 to 880 points (the preset
group and the status line had pushed Save and Close below the fold —
the window scrolled) and the audit log from 440 to 520 (four entries
plus their new level lines no longer fitted).

**Why.** The alternative was to ask the owner for six or seven separate
clicks across four windows, most of which prove nothing a screenshot of
the same rendered page does not. The one thing this approach cannot
prove — that the whole flow works end to end from a real card, with a
real PIN — is asked of the owner explicitly instead, as one run rather
than seven interruptions.

**Rejected.**
- **Adding a permanent `preview` subcommand or build tag to the
  shipped binary.** Verification scaffolding does not belong in the
  product; a file created and deleted in one session leaves nothing
  behind.
- **Screenshotting only what the real binary reaches without a click.**
  That is the consent window's waiting state and nothing else — every
  screen this pass actually changed would have gone unlooked-at, which
  is the exact failure mode [[D-087]] recorded.

---

## D-101 — Both window-layer defects were one bug: Go memory handed to WebView2 as a bare address, with nothing keeping it where it was put

**Date:** 2026-09-04
**Phase:** F5 — third real-run review

**Decision.** Every Go object and buffer whose raw address crosses into
WebView2 or Win32 is pinned with `runtime.Pinner` for as long as the
callee may use it — the completion and event handler objects for as
long as their COM reference count says so, everything else for the
length of the call. `runtime.KeepAlive`, which is what this code used
before, is not enough and never was.

Separately, and independently correct: the WebView2 controller is
released by exactly one thread, the one that created it, exactly once,
guarded by a flag set on entry to the teardown rather than inferred
from pointers zeroed on the way out.

**Why — the two symptoms turned out to have one cause.**

[[D-099]] recorded `ui.NewWindow` hanging roughly one call in
twenty-five, always parked in `pumpUntil` inside
`environmentCreateController`, waiting for a completion that never
came. It also recorded an access violation during teardown, reached
from `closeWebView`'s `ICoreWebView2Controller::Close`, and guessed at
a second WM_CLOSE arriving after teardown. Both guesses about *why*
were wrong, and the two symptoms are the same defect wearing different
clothes: **a Go value handed to foreign code as a `uintptr`, which the
Go runtime is under no obligation to leave where it was.**

Two separate runtime mechanisms invalidate such an address:

- **The stack moves.** When a goroutine needs more stack, the runtime
  allocates a bigger one and *copies every frame to a new address*. It
  rewrites the Go pointers it can find; it cannot rewrite a number, and
  it cannot reach into the WebView2 runtime at all.
- **The collector frees.** A `uintptr` is not a reference. An object
  whose only remaining holder is WebView2 is, as far as Go is
  concerned, garbage.

`go build -gcflags=-m` named the first one outright:

```
webview2_windows.go:211:36: &controllerCompletedHandler{} does not escape
webview2_windows.go:345:39: &executeScriptCompletedHandler{} does not escape
```

Those two handlers were **on the stack**. `pumpUntil` is a deep call
chain that runs foreign callbacks on the same goroutine, so it grows
the stack routinely — and when it did, WebView2 set `done = true` in
the abandoned copy while the loop read the moved one. Forever. That is
the hang, and the two functions escape analysis named are exactly the
two [[D-099]] measured it in.

**Measurements.** All on this machine, this build, `-count=1`.

| What | Before | After |
|---|---|---|
| `ui.NewWindow`, plain runs | 2 hangs in 25 | **0 hangs in 300** |
| `ui.NewWindow`, stack moved on every call | 10 hangs in 10 | **0 hangs in 25** |
| Close racing a user close, real page and handlers | crashed 0xc0000005 within 40 | **0 crashes in 150** |
| `sign --interactive`, real binary, closed by a real WM_CLOSE | — | 3 runs, exit 0 |

The second row is the proof rather than the fix. Building with
`-gcflags=all=-d=maymorestack=runtime.mayMoreStackMove` makes the
runtime move the stack at *every* function entry, which turns a
one-in-twenty-five race into a certainty; before the fix that build
could not create a single window — it failed instantly and differently,
in `controllerGetCoreWebView2`, whose stack-allocated out-parameter
moved between the address being taken and the call using it, the same
bug in its synchronous form. After the fix it opens twenty-five without
complaint. That command is the reproduction recipe for whoever touches
these files next:

```
go test -gcflags=all=-d=maymorestack=runtime.mayMoreStackMove \
    ./internal/ui/ -run TestNewWindowAlwaysCompletes
```

**The teardown crash was the collector, not two threads.** The reported
dump showed the main goroutine blocked in `(*window).Close` and the
window thread crashing in `wndProc -> closeWebView -> controllerClose`,
which reads as two threads racing over one controller. It is not.
`ICoreWebView2Controller::Close` releases every event handler
registered against the controller — the codebase already knew this,
which is why `webMessageHandler` was kept on the `*window` — and the
navigation-completed handler, added later, was **dropped on the floor
the moment `setUpWebView2` returned**. WebView2 kept its address for
the window's whole life and called `Release` on it during teardown. If
a collection had run in between, that call landed on reclaimed memory.

Confirmed by re-creating exactly that state — un-pinning the navigation
handler, keeping every other fix — and running the new close-race test,
which reproduced the reported stack character for character:

```
Exception 0xc0000005
ui.comCall -> ui.controllerClose -> ui.(*window).closeWebView -> ui.wndProc(0x10)
```

With the pin restored, 150 iterations pass. The test forces a
collection before each close, which is what turns a lifetime bug from
an occasional mystery into something a test can be relied on to catch.

**The ownership fix is still made, on its own merits.** Two WM_CLOSEs
for one window are ordinary — the title bar sends one, `Close` sends
one — and `ICoreWebView2Controller::Close` runs a nested message loop
that dispatches whatever is queued straight back into `wndProc` on the
same thread, from inside the first close. Idempotency-by-zeroing cannot
help there: the second entry reads the pointers before the first entry
has finished with them and zeroed them. So `tearingDown` is set on
entry; `wmRunFunc` stops running queued closures once teardown starts,
since a queued `Eval` would otherwise call into pointers being
released; `closedCh` is closed under a `sync.Once`; and the window
records the thread that owns it, so `Close` can tell "post and wait"
from "I am that thread, dispatch it here" rather than deadlocking.
Measurably this changed nothing on its own — with the lifetime bug
fixed, disabling these guards still passed 100 close races — and it is
kept because "usually correct" is not a property to build F6's extra
windows on.

**What else the audit found.** Every remaining pointer handed to a COM
or Win32 method was converted to the same discipline: the UTF-16
script, URI, virtual-host and user-data-folder buffers (pinned across
the whole asynchronous call, not merely `KeepAlive`d across the
synchronous half); `queryInterface`'s GUID and out-parameter;
`controllerSetBounds`'s RECT; `controllerGetCoreWebView2`'s
out-parameter; both event registration tokens; `WebMessageAsJson`'s
out-parameter; `SHBrowseForFolderW`'s BROWSEINFO and the two buffers it
points at, which sit under a modal message loop for as long as the
dialog is open; `LoadImageW`'s path. `IUnknown::Release` also stopped
being a decrement whose result nothing read: reaching zero is now what
unpins an object, so a handler lives exactly as long as something
outside Go can still call it and not one reference longer — checked
directly by `TestHandlerObjectsAreUnpinnedWhenReleased`, because the
opposite mistake is a leak that grows with every `ExecuteScript`.

**Rejected.**
- **A mutex around the teardown.** It is the wrong shape: two threads
  politely taking turns at an apartment-threaded object is still two
  threads touching it. Ownership is the property that makes it correct;
  the flag is only there because the owning thread can be re-entered by
  its own nested pump.
- **Keeping `runtime.KeepAlive` and adding more of it.** KeepAlive
  stops memory being *collected*. It says nothing about it being
  *copied*, which is what was actually happening. Every KeepAlive on
  this path is now a pin.
- **Pinning every handler forever and never unpinning.** Simpler, and a
  slow leak for the one handler allocated per `ExecuteScript` call —
  which is to say per `PostJSON`, in a tray process meant to run for
  weeks.
- **The one-UI-thread rewrite [[D-099]] proposed as the likely fix.** A
  reasonable guess at a defect that turned out to have nothing to do
  with thread count: the hang reproduces on a single window in a fresh
  process, and now does not reproduce at all. Rewriting the concurrency
  model would have been a large change made for a wrong reason.

---

## D-102 — On the timestamp-choice screen, signing without a timestamp is the primary action and Cancel is quieter than the secondary

**Date:** 2026-09-04
**Phase:** F5 — third real-run review

**Decision.** The three actions get three distinct weights, not two:
*Sign without a timestamp* is `liro-btn-primary` (brand blue),
*Configure a timestamp authority* is `liro-btn-secondary` (neutral,
bordered), and *Cancel* is a new `liro-btn-quiet` — no fill, no border,
secondary text colour, taking the neutral fill back only on hover.

**Why.** This project ships no default timestamp authority ([[D-095]]),
so this screen exists precisely because none is configured, and signing
without one is what most people who reach it will choose. It was styled
as the secondary while *Configure* — the action that sends the user off
to the settings window in the middle of a signature — wore the brand
colour.

The third weight exists because two neutral buttons stacked together
read as equals, and Cancel is not the equal of either real choice.
`liro-btn-quiet` joins `scripts/synctokens/intents.go` next to the
other button variants, so it is generated into `intents.css` with the
rest and takes its colours from tokens; `checkcss` stays green.
Verified by looking at the rendered screen, by [[D-100]]'s method.

**Rejected.** *Leaving Cancel as a second secondary and only swapping
the other two.* That fixes the wrong emphasis and leaves the ambiguity
the task actually described: the screen would still offer two
equal-looking neutral buttons, one of which does nothing.

---

## D-103 — The consent window offers a visible stamp, on by default, in one of four corners, and remembers both answers

**Date:** 2026-09-04
**Phase:** F5 — fourth real-run review (Task 1)

**Decision.** The consent window gains a checkbox, *Add a visible
stamp*, **ticked by default**, and — while it is ticked — a corner
selector offering exactly SPEC §13.1's four corners, `bottom-right`
first because that is SPEC's own default. Both answers are read back
when Approve is pressed (through `window.__liroStampChoice()` and
`Window.Eval`'s return value, the same channel the settings form and
the timestamp choice already use — [[D-083]]), saved to
`config.json` as `visibleStamp` and `stampPosition`, and used as the
window's starting state on the next run.

Nothing else about the stamp is exposed here. The reference line and
the identity-document-number line stay command-line capabilities
(`--stamp-reference`, `--stamp-show-document-id`); SPEC §13.5 is
explicit that the identity document number is never a default, and it
is not offered where it could become one by accident. Explicit x/y
coordinates stay on the command line too, since SPEC §13.1 defers a
visual placement picker to a later phase.

The command line is unchanged: `--stamp` is still the only thing that
draws a stamp there, and a `sign` run without it still produces the
byte-identical invisible signature the F3 golden file pins.

**Why the default is on.** A completed `sign --interactive` run
produced a correct signature the owner could not see, and their first
reaction was that the signature had not been applied. The evidence in
the output was

```
field 2073 | T = Liro-Signature-1 | Rect = [0, 0, 0, 0] | no appearance stream
```

which is SPEC §13.4's invisible default, correctly implemented — and
unreachable from the window, because the window never asked. A
signature nobody can see is indistinguishable, to the person who just
signed, from no signature at all.

The direction of the default follows from who is on each side of it.
Someone who wants an invisible signature is making a specialist choice
about a document that will be checked by a validator, and they know it;
they untick a box. Someone who wants to see their signature on the page
is the ordinary case — a contract, an invoice, a form that a human will
open and look at — and they should not have to discover a setting to
get it. SPEC §13.4's rule is about the *code path* ("stamp generation
is a separate module, invoked only when a visual stamp is requested"),
which is unchanged: `interactiveStampOptions` returns `nil` when the box
is unticked, and every line that reads `opts.Stamp` is skipped exactly
as before. What changed is who does the requesting.

Persisting both answers follows from the same reasoning as the choice
itself: this is a preference about how the user's own documents should
look, not a per-batch security decision. SPEC §18.15 forbids
remembering a *certificate* across sessions — because on a machine
holding several clients' cards a remembered default becomes a
wrong-signer incident — and that reasoning does not reach a stamp: no
choice here can sign anything with the wrong key, and the certificate
is still chosen explicitly, every time, with nothing preselected.

**Verified.** Not by reading the payload builder — by the bytes that
come out of it, which is what the finding was about:

- `TestInteractiveSignatureIsVisibleWhenTheStampIsAskedFor` signs
  `testdata/pdfs/blank.pdf` through `signInteractiveOne` with exactly
  the options `runSignInteractive` builds, and asserts the produced
  document has a `/Rect` that is not `[0 0 0 0]`, an `/AP` appearance
  stream, and the embedded `LIROBR` font subset. Against the previous
  build's behaviour (no stamp options at all) every one of those three
  assertions fails.
- `TestInteractiveSignatureStaysInvisibleWhenTheStampIsOff` asserts the
  opposite for an unticked box — every `/Rect` degenerate, no font
  subset — so SPEC §13.4's default path is provably still there.
- `TestInteractiveStampCornerReachesTheOutput` signs the same document
  twice, bottom-left and top-right, and asserts the two rectangles
  differ: the corner is not decorative.
- `TestConsentStampCheckboxIsOnByDefaultAndOffersFourCorners` and
  `TestConsentStampPositionHidesWhenTheStampIsOff` measure the real
  WebView2 DOM: the box is checked out of the box, the selector holds
  exactly `bottom-right,bottom-left,top-right,top-left` with localised
  labels, and the corner selector is hidden while no stamp is being
  drawn.
- `TestReadStampChoiceReadsWhatThePageHolds` proves the answer travels
  back to Go; `TestSettingsSavePreservesTheStampChoice` proves saving an
  unrelated setting does not silently switch it off again (see below).
- In the running binary: `liro-bridge sign --interactive --in
  testdata\pdfs\local\mup.pdf` shows the checkbox ticked and the corner
  selector on `Dole desno`, above the Details disclosure.

**A defect this exposed.** `handleSettingsAction` built a fresh
`config.Config` from the form's own fields, so every field the settings
window does not show was reset on save — the log level and the port
range to hard-coded literals, and, once this task added them, the stamp
choice to its zero value (invisible, no corner). Saving any setting
would have quietly undone the stamp choice. It now starts from the
configuration the window was opened with and overwrites only what the
form holds. `TestSettingsSavePreservesTheStampChoice` pins it.

**Rejected.**
- **Leaving the stamp off by default and only offering the checkbox.**
  This is the literal reading of SPEC §13.4, and it fixes the
  discoverability half of the finding while leaving the substance: the
  first signature a new user makes is still invisible, and they still
  conclude nothing happened. §13.4's rule is about the code path, not
  about which way a checkbox points, and the code path is untouched.
- **Making the visible stamp unconditional in the window (no
  checkbox).** Rejected: the invisible signature is a legitimate,
  specified default with real users — a document that will only ever be
  validated by a machine gains nothing from a stamp, and a stamp
  overlays whatever is under it. Removing the choice trades one
  complaint for another.
- **Putting the choice in Settings instead of on the consent window.**
  Rejected: the position and the visibility are decisions about *this
  document* as often as they are standing preferences — the corner
  depends on where the page's own content is. Settings would be the
  right home for a default, but the moment of signing is where the
  question is actually being asked. Persisting the answer gives the
  Settings behaviour anyway, without the trip.
- **Offering a full placement picker, or x/y coordinates, in the
  window.** Out of scope by SPEC §13.1's own words ("a visual position
  picker comes in a later phase"), and four corners cover what a corner
  stamp is for.
- **Changing the command line's `--stamp` default to match.** Rejected:
  a script that has been producing invisible signatures must not start
  producing stamped ones because the desktop default changed. The two
  front doors are allowed to have different defaults precisely because
  one has a human looking at it and the other does not — the same
  reasoning [[D-095]] applied to `--on-tsa-failure`.

---

## D-104 — An existing output file is a choice with its own error code, not an "unexpected error"; every code now has a message in all three catalogues

**Date:** 2026-09-04
**Phase:** F5 — fourth real-run review (Task 4)

**Decision.** Three parts.

*(a) The consent window asks.* When the file a signature would be
written to already exists, the window shows a new state
(`consent.StateOutputExists`) naming the existing file and offering
three actions:

- **Save as `<name>-2.pdf`** — the primary action, naming the file it
  would actually write, computed by `nextFreeOutputPath` (the first
  free numeric suffix, counting from 2, bounded at 1000).
- **Overwrite** — the neutral secondary.
- **Cancel** — `liro-btn-quiet`, the third weight [[D-102]] introduced.

Saving under a different name is the primary because it is the only one
of the three that loses nothing. The answer is remembered for the rest
of the batch: a hundred-document batch signed a second time asks once,
not a hundred times. Cancel ends the batch, nothing is written, and the
audit entry records `denied` exactly as any other cancellation does.

SPEC §12.11 and §18.10 forbid *silently* replacing a file. Overwriting
one the user has just been shown and asked about is not silent, and the
rule is not softened: with no window (the command line) the refusal
stands unchanged, and its message already names `--force` in all three
catalogues.

The question is asked **before the card session is opened**, for the
same reason [[D-095]] moved the timestamp question there: every output
path is known before signing begins — it comes from the input name and
the configured suffix, not from anything the signature produces — so a
user who answers Cancel has not spent a PIN entry on a batch that was
never going to be saved.

*(b) The condition gets its own code.* `errs.CodeOutputExists`
(`OUTPUT_EXISTS`) replaces the bare `fmt.Errorf("output file already
exists: %s", …)` that `codeOfInteractive` was mapping to
`errs.CodeInternal` — and so to "Дошло је до неочекиване грешке", the
message reserved for genuinely unclassified failures. Alongside it,
three more conditions that had the same problem: `OUTPUT_WRITE_FAILED`
(the signature succeeded, the disk write did not — `SIGN_FAILED` would
send the user to check the card for a problem that is on the disk),
`TSA_CLIENT_CERT_UNREADABLE` and `TSA_CLIENT_CERT_INVALID` (a wrong
path and a wrong password: two clear causes needing two different
corrections, so two codes rather than one).

*(c) Every code has a message, and the failed screen renders Details.*
Seven codes had no catalogue entry at all — `NOT_PAIRED`,
`AUTH_FAILED`, `CONSENT_TIMEOUT`, `PIN_REQUIRED`, `CERT_EXPIRED`,
`CERT_REVOKED`, `VERSION_TOO_OLD` — and would have rendered their own
key ("error.cert_revoked") to the user. `errs.AllCodes` now enumerates
the code set and `TestEveryErrorCodeHasAMessageInEveryCatalogue` fails
the build if a future code arrives without one. Separately, the consent
window's `pushFailure` rendered the bare catalogue string, so
`STAMP_GLYPH_MISSING` — whose message is a sentence with two
placeholders — would have reached the screen as "…does not support: %s
(%s)". It now goes through `cli.ErrorMessage`, the same renderer the
command line uses. [[D-103]] makes that path reachable by turning the
stamp on by default, which is why it is fixed in the same round.

**Why.** The owner signed to a path that already held a signed
document and was told the program had failed unexpectedly. It had not:
it had protected their file, which is the behaviour SPEC asks for. The
whole cost of the defect was in the presentation — a deliberate refusal
wearing the words reserved for a bug — and the remedy the user was left
with was to delete the file by hand and run the command again, which is
the work the program should have offered to do.

`CodeInternal` is the bucket for failures nothing understands. Every
condition that is understood, and that a user can act on, belongs
outside it; that is the general rule this round applies, and the new
test is what keeps it applied.

**Verified.**
- `TestResolveOutputConflictAppliesTheChoice` drives the whole loop
  against a real WebView2 window: a free path asks nothing, `--force`
  answers without asking, Overwrite returns the original path with
  permission to replace it, Save-as returns `…-2.pdf`, each answer is
  remembered so the next conflicting document is settled silently, and
  Cancel returns "do not proceed". The clicks are `Eval`-driven inside
  the page's own DOM — [[D-094]]'s carve-out; nothing touches the real
  cursor.
- `TestConsentOutputExistsScreenShowsBothPaths` measures the rendered
  screen: the existing path, a rename button naming
  `ugovor-potpisan-2.pdf`, the localised Overwrite label, initial focus
  on Cancel (nothing that writes a file is focused first), and each
  button recording its own choice for Go to read back.
- `TestOutputExistsIsItsOwnErrorCode` asserts `signInteractiveOne`
  returns `OUTPUT_EXISTS`, never `INTERNAL`, and that all three
  catalogues have a message for it that is not the unexpected-error
  message.
- `TestNextFreeOutputPathCountsFromTwo` covers the suffix search.
- `TestResolveInteractiveOutputsSettlesTheWholeBatchBeforeSigning`
  covers the pre-session pass: three documents, two of them already
  having a signed output, produce exactly one question, and the single
  Save-as answer settles all three (`prvi-potpisan-2.pdf`,
  `drugi-potpisan-2.pdf`, and the unconflicted `treci-potpisan.pdf`
  untouched).
- `TestEveryErrorCodeHasAMessageInEveryCatalogue` covers (c); it fails
  against the pre-change catalogues with seven missing keys.

**Rejected.**
- **Asking once per conflicting document rather than remembering the
  answer for the batch.** Rejected: SPEC §12.10 already establishes
  that a batch is not to be turned into an obstacle course, and a
  hundred identical questions is not a hundred choices.
- **Making Overwrite the primary action.** Rejected: the primary weight
  belongs to the action that cannot destroy anything. A user who means
  to overwrite still reaches it in one click.
- **Silently writing `…-2.pdf` with no question at all.** Rejected: it
  is not destructive, but it leaves the user with two files and no idea
  why, and the next run makes a third.
- **Mapping the existing-file refusal onto an existing code
  (`SIGN_FAILED`).** Rejected for the reason [[D-066]] rejected the
  same shortcut for a missing glyph: a code whose message names the
  card sends the user to look at hardware for something that has
  nothing to do with hardware.
- **Changing the command line to overwrite, or to auto-rename, when
  `--force` is absent.** Rejected: SPEC §18.10 stands, the flag is
  documented in `--help`, and a script that starts overwriting files
  because a desktop window learned to ask is a worse failure than the
  one being fixed.

---

## D-105 — B-B is a signature level the user can choose in Settings, and choosing it silences the timestamp question

**Date:** 2026-09-04
**Phase:** F5 — fourth real-run review (Task 3)

**Decision.** Settings' signature-level group gains **B-B** as its
first option, and all three options say in words what they contain:

```
B-B    signature only, no timestamp
B-T    signature + timestamp
B-LT   signature + timestamp + revocation evidence
```

`config.Config.SignatureLevel` accepts `"b-b"` alongside `"b-t"` and
`"b-lt"`; the default is unchanged (`b-lt`). When B-B is the configured
level, `runSignInteractive` never builds a TSA client, never contacts an
authority, and never shows [[D-095]]'s timestamp-choice screen — the
question it asks has already been answered. `internal/pades` treats a
*requested* level of B-B the same way: no timestamp is attempted and
none is reported as missing, because nothing failed.

The achieved level is still reported exactly as [[D-047]] requires — on
the done screen as "Nivo B-B — bez vremenskog žiga" in the warning
intent family, in `pades.Result.AchievedLevel`, and in the audit entry
— because a B-B signature carries no proof of when it was made whether
the user chose that or fell into it, and the log is where they find out
weeks later.

The command line is untouched: `--level` still takes `b-t` or `b-lt`,
and B-B is reached there by not configuring a TSA or by
`--on-tsa-failure=b-b`, which is where a script's version of this
decision already lived.

**Why.** SPEC §12.6 calls B-B "fallback only, on explicit user choice"
— and until now there was nowhere to make that choice explicitly. A
user who has decided against timestamps (no contract with an authority,
an internal document, a machine with no network) met [[D-095]]'s
choice screen on every single signature and answered it the same way
every time. A question that is always answered identically is not a
choice; it is a toll. Settings is where a standing decision belongs,
and the screen remains for the case it was built for: a level that
*needs* a timestamp and cannot get one.

Spelling the levels out is the same judgement [[D-095]] made for the
audit log's own B-B line. "B-T" tells a specialist everything and a
signer nothing, and this is the screen where someone decides what their
signatures will legally be.

**Verified.**
- `TestSettingsOffersThreeLevelsWithBBFirst` reads the real DOM: three
  radios in the order `b-b,b-t,b-lt`, B-B checked when it is what the
  configuration holds, its label the catalogue's own, and
  `__liroCollectState` reporting `"b-b"` back — the value that is
  actually saved.
- `TestSignatureLevelBBIsValid` proves `config.validate` no longer
  replaces it with the default on the next load (it did: the old check
  accepted only two strings).
- `TestInteractiveLevelMapsConfiguredLevels` maps all three plus the
  empty string.
- `TestConfiguredBBSignsWithoutATimestampAndReportsBB` signs a real
  document at a configured B-B with no TSA client at all, with
  `allowBB` deliberately false — proving nothing was attempted that
  could fail — and asserts the achieved level is B-B and that no
  degradation note was produced.

**Rejected.**
- **Adding `b-b` to the command line's `--level` too.** Considered; not
  done. The flag's two values are what F3 specified, the outcome is
  already reachable there two other ways, and a third spelling of the
  same thing is how a CLI accumulates surface. If a script ever needs
  it, it is one line — and it will arrive with a reason.
- **Keeping the timestamp-choice screen even at B-B, "so the user is
  reminded".** Rejected: that is exactly the toll being removed, and
  the reminder already exists where it belongs — the done screen and
  the audit entry both say B-B, in the warning family, every time.
- **Labelling the levels with their ETSI names alone.** Rejected: see
  the "why" above.

---

## D-106 — Every window is a fixed-height page with one scrolling region and pinned actions; settings labels sit above their inputs

**Date:** 2026-09-04
**Phase:** F5 — fourth real-run review (Tasks 2 and 5)

**Decision.** Two changes with one shape behind them.

*(a) Labels above inputs.* Every labelled text input and select in the
settings window is now a `.liro-field`: the label on its own line,
left-aligned, the input full width beneath it. Checkboxes and radios
keep their label beside them (`.liro-check`) — there the label *is* the
control's name, and it is not competing with an input for the same
line. The window's size is unchanged (520 × 880).

*(b) One scrolling region per window.* `body` carries `.liro-page`
(`height: 100vh`, a flex column), `main` fills it and cannot grow past
it, exactly one descendant is a `.liro-scroll-region` (`flex: 1 1 auto;
min-height: 0; overflow-y: auto`), and everything else — headers,
status lines, action rows — is fixed. Per window: the certificate list
(consent, certificates), the entry list (audit log), the form
(settings). One exception, deliberate: the consent window's file list
keeps its own 120-point cap and scrolls inside the Details card — it is
capped at ten names by SPEC §6.6 precisely so "a long name cannot push
the Approve/Cancel buttons off screen", and the disclosure holding it
is closed by default, so it is never a second scrollbar competing for
the same glance. `TestConsentWindowFitsWithTenLongFileNames` holds that
case to the same standard: ten names at the 120-character display cap,
Details open, six certificates behind it, page still fixed and buttons
still on screen. The fixed `max-height`s those lists used to carry are gone:
a fixed height is only correct while nothing else on the page changes
size, and things did.

Two windows were resized as a direct result of measuring them. The
certificates window grew from 440 to 640 points: with six certificates
— the realistic case for a machine holding several clients' cards, SPEC
§14.1 — 440 showed 305 of the list's 701, under three rows of six. It
scrolls correctly at either size; this is simply enough of a reference
list to read without scrolling.

The consent window grew from 720 to 860 points for the same reason: the
stamp checkbox and its corner selector joined the
fixed content below the list, and six certificate rows plus their gaps
are 544 points, of which 720 showed 408. At 860 the list is 548 and all
six fit without scrolling. 860 plus a title bar still sits inside this
machine's 1032-point work area.

**Why.** Serbian labels run considerably longer than their English
equivalents — "Klijentski sertifikat za servis za vremenske žigove
(PKCS#12)" against "TSA client certificate (PKCS#12)" — so a
side-by-side row that fits in English wraps to two lines in Serbian
however wide the window is made. Widening is not a fix, it is a delay;
stacking gives the label the whole width and ends the competition.

The scrolling shape exists because "the buttons are visible" cannot be
a property that holds for the content someone happened to test with. A
page that can scroll at all can scroll its Save button off the bottom,
and no test of the payload will ever notice. Making the page itself
unscrollable turns that from something to check into something that
cannot happen; the one region that genuinely grows scrolls, and the
actions are outside it.

**Verified.** Measured in real WebView2 windows at each window's own
fixed size, with realistic content — six certificates (two usable
signing certificates, their identical-subject authentication twins per
SPEC §11.5, an expired one, one with its card out), twenty audit
entries across all four outcomes and all three levels, and every
settings field populated with values of the length real ones have:

| Window | Size | Page scrolls | Scrolling region | Actions |
|---|---|---|---|---|
| Consent (waiting) | 520 × 860 | no | `#cert-list`, 548 of 548 — all six rows fit | Cancel/Approve visible |
| Consent (details open) | 520 × 860 | no | `#cert-list` shrinks and scrolls | Cancel/Approve visible |
| Consent (done, timestamp choice, output exists, failed) | 520 × 860 | no | none needed | all visible |
| Certificates | 460 × 640 | no | `#cert-list`, 505 of 701 | Close visible |
| Audit log | 460 × 520 | no | `#entry-list` scrolls | Close visible |
| Settings | 520 × 880 | no | `.settings-form`, 715 of 954 | Save/Close visible |

`assertPageDoesNotScroll` additionally walks every element in the
document and fails if any element outside the named scrolling region
has `overflow-y: auto|scroll` *and* actually overflows — which is the
double-scrollbar defect an earlier round reported, now ruled out by
construction rather than by inspection.
`TestSettingsLabelsSitAboveTheirInputs` asserts, for every field, that
the label's bottom edge is at or above the input's top edge, that the
input spans the field's full width, and that no label wraps onto a
second line in `sr-Cyrl`. Before the change the same assertion fails on
the TSA client-certificate and password labels.

**A defect the screenshots caught that no test had.** The stamp's
corner selector is hidden with `el.hidden = true` while the checkbox is
unticked, and the property was correctly `true` — while the selector
was plainly on screen in the running binary. `[hidden] { display:
none }` comes from the *user-agent* stylesheet, and an author rule as
ordinary as `.liro-field { display: flex }` outranks it in the cascade
regardless of specificity — the same trap `consent.css` had already
documented for `main[hidden]` and solved locally. `intents.css` now
carries a global `[hidden] { display: none !important }`, so every page
in this project can keep using `el.hidden` and mean it. The test that
missed it read the property; it now reads `getComputedStyle(...).display`
and the element's height, and fails against the pre-fix stylesheet with
"the corner selector still renders with the stamp off (display: flex)".

**Rejected.**
- **Widening the settings window further.** The task's own instruction,
  and correct: 880 was already the second widening, and the next long
  Serbian label would have needed a third.
- **Keeping `max-height` on the lists and simply making the numbers
  bigger.** Rejected: it is the same defect one round later. A fixed
  height is a guess about everything else on the page; `flex: 1` is not
  a guess.
- **Letting the settings page scroll as a whole and accepting that Save
  scrolls with it.** Rejected — that is the defect being fixed, in the
  window where it was first seen.
- **Truncating long labels with `text-overflow: ellipsis`.** Rejected
  for the reason [[D-096]] rejected it for the fingerprint: a label the
  user cannot read in full is not a label, and a settings form is
  exactly where the full words matter.

---

## D-107 — The embedded Trusted List is stored byte-for-byte as the Ministry publishes it; `text=auto` had normalised its CRLF line endings away

**Date:** 2026-09-05
**Phase:** F6 — Part 0 (repair 0a)

**Decision.** `.gitattributes` marks `internal/trust/tsl/seed/TSL-RS.xml`
— and every other byte-exact artefact in the repository — as `-text`, so
git stores and checks it out verbatim on every platform. The seed itself
is replaced with the Ministry's own bytes, BOM stripped and CRLF intact,
which hash to `3f574484…` again. `scripts/genseed` is a new developer
tool that is the only supported way to replace it: it fetches (or reads)
a candidate, strips the BOM, **verifies the XML-DSig signature against
`tsl.PinnedSigners` and refuses to write anything that does not verify**,
refuses a sequence number older than the committed one, and prints the
sequence, issue date, provider count, line-ending style and digest of
both the old and the new list before touching anything.

**What the discrepancy actually was.** Three numbers were reported as
disagreeing. Measured directly, before changing anything:

| Reading | Sequence | Digest |
|---|---|---|
| The Ministry's live publication, as served | 36 | `ea573c30…` (BOM + CRLF) |
| The same, BOM stripped — what [[D-018]] committed | 36 | `3f574484…` |
| The committed seed, as it stood | 36 | `d83bac89…` (BOM stripped, **LF**) |
| The on-disk cache, freshly fetched | 36 | `ea573c30…` |
| `liro-bridge certs` | 36 | — |
| The `currentSequence=37` in the test log | — | — |

**Every one of them says 36.** There is no sequence 37 and there never
was one: `currentSequence=37` is a value `TestRefreshRejectsRollback`
writes into a store by hand (`s.current.Sequence = 37`) so that the real,
validly signed sequence-36 list reads as a rollback attempt. It is a
fabricated number in the log line of a *passing* test — the anti-rollback
warning firing exactly as designed — and it appears in the output of
`go test ./...` beside the digest failure, which is what made the two
look related. They are not.

The digest failure has one cause, and it is not the Trusted List at all.
`.gitattributes` read `* text=auto eol=lf`. The Ministry serves this
document with CRLF line endings; git classified it as text and
normalised all 4 772 of them to LF at commit time. The result is a
byte-different copy of a semantically identical document: same sequence,
same issue date, same ten providers, and — measured, not assumed — a
signature that still verifies against the pinned signers, because XML
parsing normalises `\r\n` to `\n` before canonicalisation ever sees it.
So nothing was ever *wrong* with what the agent trusted; what broke was
the one check that proves the embedded list is the published list, and
that check was right to break. A seed whose digest cannot be compared
against the Official Gazette is a seed nobody can audit.

**Why this is worth a `.gitattributes` entry rather than a note.** The
digest is the only mechanism connecting this file to the Ministry's
publication. Restoring the bytes without disabling the normalisation
would put them back exactly as they were within one commit, on any
machine, silently. The same protection is extended to the other
byte-exact artefacts — `testdata/golden/**` (SPEC §16.2's byte-for-byte
comparison), `testdata/pdfs/*.pdf` ([[D-071]]'s reproducible fixture),
the `.der` certificates, the font subset, the two `.flate` logo halves,
the icon and `WebView2Loader.dll`. The `.der`/`.ttf`/`.flate`/`.ico`/
`.dll` files were already safe by accident — git's binary heuristic
found NUL bytes in them — but `blank.pdf` and `minimal-signed-bb.pdf`
were classified as **text**, and are byte-exact artefacts one
CR-containing regeneration away from the identical bug. Relying on a
heuristic to protect a golden file is not protection.

**Verified.** `go run ./scripts/genseed` fetched the live list and
printed the candidate's digest as `3f574484…`, its signature as verified
against a pinned Ministry signer, sequence 36, issued 2026-05-20, ten
providers — before writing, and independently of the constant it was
about to satisfy. `git cat-file blob` on the staged file confirms the
*index* holds `3f574484…`, not just the working tree, which is the half
that would otherwise regress on the next clone.

Three tests, each confirmed to fail against the pre-fix artefact and pass
against the fixed one:

- `TestSeedDigestMatchesExpected` (pre-existing) — the failure that
  started this.
- `TestSeedIsTheMinistrysOwnBytes` (new) — fails with "the embedded
  seed's CRLF line endings have been normalised to LF; check
  .gitattributes marks it -text", so the next person to see it is told
  the cause rather than a hex mismatch.
- `TestSeedSequenceAgreesWithWhatTheCodeReports` (new, F6 §0a's required
  check) — compares four independent readings of the sequence number:
  the `seedSequence` constant, the `<TSLSequenceNumber>` element found by
  regexp in the raw bytes (deliberately not through this package's own
  parser), `Parse`'s result, and `Provenance.Sequence`, which is the
  number `certs` actually prints. It cannot be satisfied by a value that
  exists only inside a test's own fixture, which is precisely what 37
  was.

**Rejected.**
- **Updating `seedSHA256` to `d83bac89…`.** The obvious way to make a
  red test green, and the one that had to be refused: it would pin the
  digest of a file git happened to rewrite, permanently severing the
  connection to the digest published in the Official Gazette. SPEC §11.1
  makes the Trusted List the primary authority for qualification; a seed
  that can no longer be checked against its publisher is exactly the
  "unverified seed is worse than a stale one" case F6 §0a names.
- **Committing the file with its BOM, since that is literally what the
  server sends.** Rejected: [[D-018]] stripped it deliberately and the
  digest F1 §4.7 and the Gazette record is the stripped one. A BOM is
  transfer encoding, not content, and everything downstream
  (`encoding/xml`, the C14N implementation) would treat it as content.
- **`* -text` for the whole repository.** Would fix this and create a
  worse problem: Go source, JSON catalogues and CSS committed from a
  Windows machine would start carrying CRLF into the repository, which
  is what `text=auto` is there to prevent. The exception belongs on the
  files whose bytes are the point, not on all of them.
- **Re-seeding to a newer list while in here.** There is no newer list:
  the Ministry's live publication is still sequence 36. If there had
  been, `scripts/genseed` is what would have taken it, and it would have
  verified the signature first — which is the shape F6 §0a asks for
  whether or not it was needed this time.

---

## D-108 — Windows-internal certificates are recognised by what they are, not by their KeyUsage; one rule in `classify`, used by every listing

**Date:** 2026-09-05
**Phase:** F6 — Part 0 (repair 0b)

**Decision.** `classify.Info.IsWindowsInternal()` is the single rule
deciding what a listing hides by default. It returns true for a
certificate the Trusted List does not know that is *either* of no
recognised purpose ([[D-023]]'s original rule, kept intact) *or*
self-signed with a GUID for both its subject and issuer common name. A
soft-token certificate is excluded explicitly. `classify.Info` gains
`SelfSigned`, set by `Classify` from
`bytes.Equal(cert.RawSubject, cert.RawIssuer)`.
`internal/cli.CertRow.Hidden()` now delegates to it and holds no logic of
its own; the consent window and the Certificates window already went
through `CertRow.Hidden()`, so all three surfaces share one
implementation rather than three copies.

**Why the rule had to change rather than be applied more widely.** F6
§0b reads as "the filter exists in one place and not the other". It does
not: `internal/cli/render.go`, `cmd/liro-bridge/interactive_windows.go`
and `cmd/liro-bridge/certificates_windows.go` all called
`CertRow.Hidden()` already. The rule itself was the problem. Measured on
the owner's machine against the real Windows store, before any change:

```
2414ebcc-b68a-461c-9a69-bca3af581969  purpose unknown         hidden
5a26d334-110e-4468-910f-34313774f081  purpose authentication  SHOWN
```

Both are self-signed, software-backed, GUID-named and unknown to the
Trusted List — the same artefact. The second carries KeyUsage
`digitalSignature + keyEncipherment`, so `purposeFromKeyUsage`
(correctly, per SPEC §11.4) calls it "authentication", and [[D-023]]'s
purpose-keyed test let it through. Purpose was never what made these
certificates internal; on the machine F1 was measured on it just happened
to be a reliable proxy, and the proxy stopped holding the moment Windows
issued itself a certificate with different bits set.

[[D-023]]'s first limb is kept rather than replaced because it is not
wrong, only incomplete — it is what hides `selfsigned_unrelated.der` and
the `2414ebcc…` certificate, and dropping it would be a regression
against behaviour F1 §5.4 measured and F1 §6.1's transcript shows.

[[D-023]]'s other property is preserved deliberately and tested for: a
real *authentication* certificate — MUP's or Halcom's, qualified, on a
card — is still shown, disabled, with its reason. F1 §6.1's own example
transcript shows one in the default view, and F5 §5.2's "hiding them
makes the user think the card is broken" is the same instruction from
the other side.

**Why `isGUID` is strict.** It accepts the bare canonical form and the
braced form Windows also writes (which `selfsigned_unrelated.der`
already reproduces), and nothing else — not `urn:uuid:…`, not a GUID
with a word appended. A looser match is a way to hide a real certificate
whose name merely contains one, and hiding a certificate a person needs
is a worse failure than showing one they do not.

**Verified against the real store, through the rebuilt binary**, not
only through tests: `liro-bridge certs` now lists 2 certificates (the
owner's two real MUP ones, correctly marked unusable with the card out);
`liro-bridge certs --all` lists all 4, both GUID certificates included.
`certs --json` continues to emit every row with its own `hidden` flag, as
it always has — `--all` was only ever about the text view.

Tests: `TestReportedWindowsInternalCertificateIsHidden` uses the exact
GUID from the owner's output and fails against [[D-023]]'s rule;
`TestWindowsInternalRuleAcrossCertificateShapes` covers both hidden
shapes and four that must stay visible, including a self-signed
certificate with an ordinary name (being self-signed is not on its own
disqualifying — a company's internal certificate has a name a person
recognises); `TestSoftTokenCertificateIsNeverHidden` guards SPEC §16.6;
`TestIsGUID` covers the boundary cases.

**Rejected.**
- **Filtering in each of the three listing call sites.** That is what
  F6 §0b's wording suggests and it is the wrong shape: three copies of
  one rule is how the consent window and `certs` come to disagree in the
  first place. The rule belongs where classification lives.
- **Keying the rule on `OnHardware`.** Available at the `CertRow` layer
  and tempting, since these certificates are all software-backed.
  Rejected as adding nothing: a hardware-backed certificate never has a
  self-signed GUID subject, so the condition is already implied, and
  putting it in the rule would push the rule out of `classify.Info` for
  no gain.
- **Hiding anything not `PurposeSigning`.** Rejected for the reason
  [[D-023]] rejected it, unchanged: F1 §6.1's transcript shows an
  authentication certificate in the default view.
- **A new committed `.der` fixture for the reported certificate.**
  `scripts/gencerts` draws fresh keys and serials from `crypto/rand` and
  rewrites every file it owns, so adding one would churn five unrelated
  fixtures. The test builds its certificate in Go instead —
  [[D-038]]'s own reasoning for synthetic PDF fixtures, and this one
  carries no personal data and no real CA bytes worth freezing.

---

## D-109 — Autostart creates the Run key rather than assuming it; the CI failure was a missing registry key, not a missing executable

**Date:** 2026-09-05
**Phase:** F6 — Part 0 (repair 0c)

**Decision.** `windowsAutostart.SetEnabled` uses `registry.CreateKey`
when enabling — which opens an existing key unchanged and creates one
otherwise — and treats a missing key as "already disabled" when
disabling, rather than an error. `windowsAutostart` gains a `keyPath`
field so a test can point it at a subkey that genuinely does not exist;
`NewAutostart()`'s behaviour and the exported surface are unchanged.
`TestWindowsAutostartRoundTrip` skips, with a message naming the key,
when the real Run key cannot be opened or created at all.

**What the failure actually was.** Reported as

```
TestWindowsAutostartRoundTrip
  SetEnabled(true): The system cannot find the file specified.
```

and read, reasonably, as the executable path not existing on the runner.
It is not: the registry stores that string verbatim and never resolves
it. "The system cannot find the file specified" is
`ERROR_FILE_NOT_FOUND` from `registry.OpenKey` — the *key* is absent. A
fresh GitHub runner profile has no
`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`. `IsEnabled`
already handled that case (`registry.ErrNotExist` → not enabled);
`SetEnabled` returned it raw.

So this was a product defect, not only a test one: on any machine
without that key — a fresh profile, a locked-down or newly provisioned
account — turning autostart on from Settings failed, with a message
pointing at the wrong thing. Skipping the test would have hidden it.

**Confirmed by reproduction, both directions.** With `SetEnabled`
reverted to `OpenKey`, the new
`TestWindowsAutostartCreatesTheKeyWhenItDoesNotExist` fails with
`SetEnabled(true) with no key present: The system cannot find the file
specified.` — the CI message, character for character, on a machine
where the real Run key exists. With `CreateKey`, it passes. The test
uses a scratch key under `HKCU\Software\LiroBridgeTestScratch`, asserts
the key is absent before it starts, and deletes both it and its parent
afterwards, so it does not depend on the environment and leaves nothing
behind.

`TestWindowsAutostartLeavesNeighbouringValuesAlone` is new for a
different reason: the round-trip test's comment has always claimed this
code touches only its own value name, and nothing checked it. It runs
against the user's real Run key, where a neighbour is somebody else's
startup entry.

**Rejected.**
- **Skipping the test on CI and leaving the code alone.** F6 §0c offers
  this, and it would have been the wrong half of the choice: the test
  was reporting a real defect in a real code path, in the one
  environment that happened to exercise it. The skip is kept as a
  fallback for an environment with no writable profile at all, but it is
  no longer what makes CI green.
- **Making the test write to a fake or redirected registry hive.**
  Windows has no per-process registry redirection short of
  `RegOverridePredefKey`, which is more machinery than a `keyPath` field
  and would test something other than what ships.
- **Validating `exePath` in `SetEnabled`.** Rejected as inventing a
  requirement: nothing asks for it, the path is legitimately allowed to
  be a not-yet-installed location, and the reported error would still
  not have been about that.

---

## D-110 — `golangci-lint-action` is pinned to v9 at the exact version used locally

**Date:** 2026-09-05
**Phase:** F6 — Part 0 (repair 0d)

**Decision.** CI's lint step uses `golangci/golangci-lint-action@v9`
with `version: v2.13.2` — the exact version installed on the
development machine — instead of `@v6` with `version: latest`.

**Why.** `.golangci.yml` targets the v2 configuration schema
([[D-009]], corrected by [[D-022]]). Action v6 installs golangci-lint
v1, which cannot read it; the action's own v7 release notes state "The
GitHub Action v7 supports golangci-lint v2 only", and v9 (current, and
what this pins) requires golangci-lint ≥ v2.1.0. That mismatch is the
whole of the 36-second Ubuntu failure — the job never got as far as
linting anything.

The version is pinned rather than left at `latest` for a reason beyond
this fix: `latest` means CI's verdict can change overnight with no
commit, so a new lint rule arrives as a red build on an unrelated
change, at the worst possible moment for diagnosing it. Pinning to the
version a developer actually runs means `golangci-lint run ./...`
locally and the CI step are the same check.

**Verified.** `golangci-lint run ./...` at v2.13.2 reports `0 issues.`
across the repository, including everything Part 0 added. Whether the
*hosted* job goes green cannot be observed from this machine — it needs
a push — and is reported as such rather than claimed.

One real finding came out of running it: `misspell` reads the backslash
in a Windows path literal `C:\other\app.exe` as an escape and sees
"ther" inside it. The value in the test was changed rather than
suppressed, since F6 forbids `//nolint` and the path was arbitrary.

**Rejected.**
- **Pinning to `v9` with `version: latest`.** Fixes the schema mismatch
  and leaves the "CI's verdict changes with no commit" half in place.
- **Downgrading `.golangci.yml` to the v1 schema to suit action v6.**
  Backwards: v2 is the current schema, [[D-022]] already verified this
  config against a real v2 binary, and v1 is what would need replacing
  again next.

---

## D-111 — Windows-only files carry the `_windows` suffix; CI lints both the Linux and the Windows view

**Date:** 2026-09-05
**Phase:** F6 — Part 0 (found by 0d's own fix)

**Decision.** `cmd/liro-bridge/ui_assets.go` and `ui_payloads.go` are
renamed `ui_assets_windows.go` and `ui_payloads_windows.go` (and
`ui_payloads_test.go` to `ui_payloads_windows_test.go`), which is the
build-constraint mechanism SPEC §8.2 already specifies and every other
file in that directory already uses. `fprintln`/`fprintf` — the only two
things in either file that are genuinely cross-platform — move to a new,
unconstrained `cmd/liro-bridge/print.go`. CI gains a second lint step
that runs the same pinned `golangci-lint` with `GOOS=windows`.

**Why — what fixing the lint action actually uncovered.** [[D-110]]
pinned the action so it could read the v2 config. The very first run
that got as far as analysing anything failed with ten `unused` findings,
all in F5 code, none of it touched by this phase. Confirmed pre-existing
rather than introduced, by linting the parent commit (`568567a`) in a
detached worktree with `GOOS=linux`: **the identical ten issues**. The
previous lint step had never reported them because it died loading the
config, every run, since F0.

The cause is a real property of the code, not a linter quirk.
`golangci-lint` on `ubuntu-latest` analyses the `GOOS=linux` view.
`ui_payloads.go` and `ui_assets.go` carried no build constraint, so they
compiled on Linux — but every caller of what they define, in production
(`interactive_windows.go`, `certificates_windows.go`,
`auditlog_windows.go`, `tray_windows.go`) and in tests
(`consent_render_windows_test.go` and its siblings), is behind a
`windows` constraint and therefore absent there. On Linux those symbols
genuinely are dead code. `unused` was right; the filenames were wrong.

Naming them for what they are is the fix rather than silencing the
linter: these files build the JSON payloads a WebView2 window consumes
and map the virtual host its assets are served from. There is nothing in
either that could run on another platform, and when F12/F13 bring macOS
and Linux windows, those platforms will need their own payload code
beside this one — exactly what the suffix convention is for.

**The second lint step is the more important half.** Until now CI
linted only the Linux view, which excludes almost everything this
project currently is: `internal/ui`'s entire hand-written COM and
WebView2 layer, the tray, the icon loading, every window in
`cmd/liro-bridge`. None of it had ever been linted, by any run, ever.
That is precisely the blind spot [[D-088]] found and closed for *tests*
— "CI never built or ran a single `*_windows.go` file" — surviving one
layer over in the lint step, and it is why a ten-symbol pile of dead
code sat unreported through an entire phase. Both views are linted
because each compiles code the other cannot: `GOOS=linux` is the only
one that sees the `*_other.go` fallbacks, `GOOS=windows` the only one
that sees the rest.

**Verified.** `golangci-lint run ./...` at v2.13.2 reports `0 issues.`
for `GOOS=windows`, `GOOS=linux` and `GOOS=darwin`; `go build` and
`go vet -unsafeptr=false` succeed for all three; the full test suite
passes with and without the `softtoken` tag.

**Rejected.**
- **Deleting the ten symbols as dead code.** They are not dead — they
  are the consent window's progress, done, timestamp-choice,
  output-exists and failure payloads, all reached on Windows, several
  of them added only last phase ([[D-095]], [[D-104]]). Only the Linux
  view thinks otherwise, and the Linux view is wrong about this file
  because the file lied about which platform it was for.
- **Excluding `unused` from `.golangci.yml`, or excluding
  `cmd/liro-bridge` from it.** Turning off the check that correctly
  identified a real filing error, so that the filing error can stay. It
  would also disarm `unused` for every future genuine case.
- **A `//nolint:unused` on each symbol.** Ten suppressions for one
  misnamed pair of files, and forbidden outright by F6's own rules.
- **Linting only with `GOOS=windows` and dropping the Linux pass.**
  Would have fixed the failure without renaming anything, and left the
  `*_other.go` fallbacks — the code that runs when someone builds this
  for a platform it does not support yet — as the only unlinted files
  in the repository. Both passes cost one step.
- **Bumping `actions/checkout@v4` and `actions/setup-go@v5` to v6 while
  in here.** The run's annotations warn that both target Node 20 and are
  being forced onto Node 24. Real, and worth doing — but it is not one
  of F6 §0's four repairs, it cannot be verified from this machine, and
  a workflow change made speculatively is how a green build becomes a
  red one for a reason unrelated to anything being worked on. Recorded
  here so the next person does not have to rediscover it.

---

## D-112 — `Run`'s refresh test waits for the refreshes instead of racing a 250 ms budget; the Ubuntu test step had never once executed

**Date:** 2026-09-05
**Phase:** F6 — Part 0 (found by 0d's own fix, second round)

**Decision.** `TestRunRefreshesImmediatelyThenOnInterval`
(`internal/trust/tsl/store_test.go`) no longer gives `Run` a fixed
250 ms context and then counts how many refreshes fit inside it. The
fetcher announces each call on a buffered channel; the test waits for
two of them — the immediate refresh plus at least one the ticker drives
— with a 30-second-per-refresh ceiling that exists only to turn a hang
into a failure. It then cancels the context and requires `Run` to
return, which the old test never checked at all: it let a deadline
expire and assumed the rest.

**Why — the failure, and why it had never been seen.** With
[[D-110]]'s and [[D-111]]'s lint fixes in, the Ubuntu job reached its
`test` step, and this one test failed:

```
--- FAIL: TestRunRefreshesImmediatelyThenOnInterval (0.65s)
    store_test.go:175: Refresh called 1 times in 250ms with a 40ms
    interval, want at least 2 (immediate + at least one tick)
```

Everything else in the run — every package, on Linux, under `-race`,
including all of Part 0's new tests — passed.

The test had never run in CI before. **This repository has three CI runs
in total**, the first of them on F5's final commit, and every one of them
failed at the lint step, which is sequential and skips everything after
it. So `go test ./... -race` on Ubuntu had never completed once in the
project's life, and this test's assumption had never been tested against
anything but a fast developer machine with no race detector.

The assumption is that `Run` fits at least two refreshes into 250 ms at a
40 ms interval. Each refresh is a full XML-DSig verification of the
577 KB Trusted List — exclusive C14N over the whole document, then
RSA-SHA512 — plus a parse. Measured directly on the development machine
(AMD Ryzen 5 4500, 12 threads, no race detector):
**28.4 ms per verify-and-parse**. That leaves the old test roughly one
order of magnitude of headroom, which sounds ample and is not: a
two-core hosted runner with `-race` instrumentation on allocation-heavy
XML DOM work loses far more than that. The runner's own numbers say so —
the test reports `0.65s` elapsed for a 250 ms context, meaning the single
in-flight refresh overran the deadline by ~400 ms, so one refresh there
cost something like 600 ms. At 600 ms per refresh and a 250 ms budget,
one call is all that can ever happen. The test was not flaky on that
machine; it was deterministically wrong.

**The restored seed is not the cause, measured rather than assumed.**
[[D-107]] grew the embedded list by 4 772 bytes (the CRs it put back),
which is an obvious suspect for a timing test that started failing in the
same phase. Benchmarked both encodings through the same
`verifyAndParse`: **28.6 ms for the CRLF seed against 29.5 ms for the
LF one**, 20 iterations each — within noise, and if anything faster.
The old test would have failed on that runner at either size.

**The general point.** A test that asserts "N things happen inside a
fixed wall-clock window" is asserting how fast the machine is, which is
not a property of this code and not something anyone intended to pin.
Waiting for the events proves the actual claim — refresh once
immediately, then keep ticking, then stop when told — on any machine, and
fails only when the behaviour is genuinely absent. The new interval is
1 ms rather than 40 ms for the same reason: with the test waiting on
events, a shorter interval only means less idling, never less rigour.

The rewrite also removes a latent data race the old shape had been
lucky to avoid: `Run` was called synchronously, so the `calls++` inside
the fetcher happened on the test's own goroutine. Moving `Run` onto a
goroutine — needed to observe cancellation — would have made that
counter a genuine race, which is exactly what the channel avoids.

**Verified.** Passes 20 consecutive runs locally in 3.0 s total (~150 ms
each). `-race` cannot be run on this machine at all (no C compiler, the
condition [[D-012]] already recorded), so the runner is the only place
that check happens — which is the whole reason this had to be fixed
rather than tuned by eye.

A scan for other tests with the same shape found none: every remaining
millisecond-scale duration in a `_test.go` file is either a value handed
to a pure function (`internal/signing`'s timing tables,
`internal/consent`'s progress arithmetic) or a sleep, neither of which
can fail for being on a slow machine.

**Rejected.**
- **Raising the budget — 250 ms to 2 s, or the interval to 5 ms.** The
  first thing to try and the wrong thing to keep: it re-picks a number
  that happens to work on the two machines anyone has looked at, and
  leaves the next slower runner, or the next `-race`-instrumented
  change, to rediscover this. There is no budget that is both large
  enough to be safe and small enough to mean anything.
- **`t.Skip` under `-race`, or a `testing.Short()` guard.** Skipping the
  test in the only environment that actually runs it with the race
  detector is not a fix.
- **Making `Refresh` cheaper so more of them fit — e.g. caching the
  verification result.** A real optimisation to consider on its own
  merits some day, but changing production code to satisfy a test's
  arbitrary stopwatch is backwards, and the test would still be
  measuring the machine.

---

## D-113 — On a platform with no CNG store, "not found" is the literal truth and the code the soft-token fallback needs; F2's and F3's CI exit conditions had never executed

**Date:** 2026-09-05
**Phase:** F6 — Part 0 (found by 0d's own fix, third round)

**Decision.** Two changes, both in service of CI steps that had never
once run.

*(a)* `internal/keysource/windowscng`'s non-Windows stub
(`conn_other.go`) returns `errs.New(errs.CodeCertNotFound, …)` from
`findAndAcquire` instead of a bare
`fmt.Errorf("windowscng: not supported on this platform")`.

*(b)* CI's "sign a PDF and verify it with OpenSSL and the independent
verifier" step passes `--on-tsa-failure b-b`.

**Why (a).** `cmd/liro-bridge`'s `runSignDigest` and `runSign` fall back
to the soft token **only** when CNG answers `CERT_NOT_FOUND`, and
deliberately on nothing else — [[D-033]] established that narrowness so a
real failure (card removed, PIN blocked) is never masked by a confusing
second attempt against an unrelated backend. On Linux and macOS the stub
returned an error carrying no `errs.Code` at all, so `errors.As` found
nothing, the fallback could not fire, and both `sign-digest` and `sign`
failed outright with `windowscng: not supported on this platform` — for
a soft token that has nothing to do with CNG.

`CERT_NOT_FOUND` is not a euphemism here. There is no CNG certificate
store on these platforms, so no thumbprint is in it, and "not found" is
the literally correct answer to the question `findAndAcquire` was asked.
It is also the same code the Windows implementation gives for the same
question ([[D-033]]).

**Why (b).** [[D-067]] made "a level was requested and no TSA is
configured" a failure rather than a silent downgrade (SPEC §18.11), and
`--level` defaults to `b-lt`. The CI step deliberately configures no
timestamp authority — its own comment says so — so since D-067 it has
been asking for B-LT and correctly being refused. The flag is how a
caller says "B-B is what I want here", which is what the step means. The
workflow simply predates the decision.

**Why neither was ever noticed.** This repository has had four CI runs.
The first three failed at the lint step ([[D-110]]), which is sequential
and skips everything after it, so **F2's exit condition (§6.1, the
OpenSSL round trip) and F3's (the PDF signed and independently verified)
have never executed in CI, not once, in the project's life.** Both were
verified by hand at the time, on Windows, where CNG exists and the
fallback is never needed. Neither defect could reach a developer's
machine: both are specific to a platform this project does not yet
support and only cross-compiles for.

**Verified on real Linux, before pushing.** Running the two steps in CI
and reading the result is a slow way to learn this, and it had already
cost three rounds. Both steps were instead reproduced locally, on a real
Ubuntu kernel, by cross-compiling `liro-bridge` (with the `softtoken`
tag), `gentestkeys` and the workflow's own two helper programs
(extracted verbatim from `ci.yml`) for `linux/amd64` and running them
under WSL with the host's own OpenSSL. Nothing was installed to do it.

Both directions were measured:

| Step | Before | After |
|---|---|---|
| sign-digest + OpenSSL | `sign-digest: windowscng: not supported on this platform` | `Verified OK` |
| sign a PDF + verifier + `openssl cms` | `sign: … Servis za vremenske žigove ne odgovara.` | `independent verifier: OK`, `CMS Verification successful` |

The signed output reports `Nivo: B-B (no timestamp (saved at B-B):
pades: level B-LT requested but no TSA is configured)` — the achieved
level and its reason, exactly as [[D-047]] requires and never the
requested one.

**Rejected.**
- **Widening the fallback to fire on any CNG error.** It would have
  fixed the symptom in one line and destroyed the property [[D-033]]
  built: a removed card or a blocked PIN would silently become a
  soft-token signature attempt. The narrowness is the point; the stub
  was what was wrong.
- **Special-casing "not supported on this platform" by string, or
  adding a `PLATFORM_UNSUPPORTED` code for the fallback to also
  accept.** The first is string-matching an error message across a
  boundary, which SPEC §7 exists to prevent. The second invents a code
  for a situation that already has a correct one, and would still need
  every fallback site to learn about it.
- **Skipping the two CI steps on Linux, or moving them to the Windows
  job.** They are F2's and F3's stated exit conditions and their whole
  value is being run by something other than the author's own machine.
  Moving them to Windows would reproduce, one job over, exactly the
  blind spot [[D-088]] and [[D-111]] each had to close.
- **Configuring a TSA for the CI step instead of asking for B-B.** It
  would make the step depend on a live external service for something
  that is testing the PDF and CMS layers, not the timestamp client —
  which `internal/pades/tsa`'s own integration test already covers
  against Pošta's real test TSA ([[D-045]]).

---

## D-114 — Dropped files reach the native frame because WebView2's own external-drop handling is switched off

**Date:** 2026-09-05
**Phase:** F6 — Part 1

**Decision.** A window that sets `ui.Options.OnFilesDropped` does two
things at once: it queries `ICoreWebView2Controller4` and calls
`put_AllowExternalDrop(FALSE)`, and it calls `DragAcceptFiles` on its
own `HWND`. `WM_DROPFILES` then arrives at `wndProc`, `DragQueryFileW`
reads the paths and `DragFinish` releases the drop. Paths are handed to
the callback exactly as the shell supplies them, directories included —
deciding what is a PDF and what to look inside is `internal/jobs`'s
business, not `internal/ui`'s.

**Why both halves are needed.** Either alone produces a window that
looks like a drop target and does nothing with a drop. WebView2's
control creates its own child windows, which Chromium registers as drop
targets; a drop lands there and the parent frame never sees it. And what
the page receives is a `File` object, which by design does not carry a
path — the web platform does not expose one, and no amount of page code
recovers it. `AllowExternalDrop` exists precisely for this: its own IDL
comment says the property is "used to configure the capability that
dragging objects from outside the bounds of webview2 and dropping into
webview2 is allowed or disallowed", and turning it off is what lets a
host application handle the drop itself.

**The IID and the vtable slot were read from the real IDL**, not from
documentation or memory, which is the discipline [[D-080]] set for every
identifier in this package. `Microsoft.Web.WebView2` 1.0.4191.47 was
fetched again and `WebView2.idl` extracted from it:
`ICoreWebView2Controller4` is `97d418d5-a426-4e49-a151-e1a10f327d9e`,
and `put_AllowExternalDrop` is vtable slot **37** — IUnknown's three,
then `ICoreWebView2Controller`'s 23 (3–25), `Controller2`'s two (26–27),
`Controller3`'s eight (28–35), and Controller4's `get` at 36. That
count was cross-checked against the four controller slots this package
already uses and knows work: `put_IsVisible` 4, `put_Bounds` 6, `Close`
24, `get_CoreWebView2` 25 — all four fall exactly where the same
enumeration puts them.

**Verified in the running binary**, which is the only place this can be
verified at all: `liro-bridge open` with a folder and a file, and the
window listing what the shell handed it.

**Rejected.**
- **Reading `dataTransfer.files` in the page and sending the names to
  Go.** Names are not paths, and F5 §5.3 forbids treating a name as one
  for exactly the reasons that would then apply. There is no path to
  recover; this is not a limitation to work around but the web
  platform's deliberate design.
- **Leaving `AllowExternalDrop` on and registering an `IDropTarget` on
  the parent.** Same problem: the child window is the registered target
  and consumes the drop first.
- **Making the WebView2 control smaller than the client area so a strip
  of the frame can receive drops.** A drop target the user has to aim
  at is not a drop target.

---

## D-115 — The queue, the runner and the shell handover are a package with no window in it

**Date:** 2026-09-05
**Phase:** F6 — Parts 1, 3, 4, 5

**Decision.** `internal/jobs` holds the document list (`Queue`), the run
(`Runner`, `SignFunc`, `Progress`, `Report`) and the cross-process
handover the Explorer menu needs (`Inbox`, `CollectBatch`). None of it
imports `internal/ui`, and none of it knows a window exists. The main
window turns a `Queue` into a payload and a click into a call; it makes
none of the decisions.

**Why.** F5 §10 required this of the consent screen and D-085 recorded
it; F6 adds the surface that actually has decisions in it — what a
dropped folder contributes, when a duplicate is one document, what
happens at document fifty when the card is gone, what Stop means, which
level a mixed batch reports. Every one of those is a question with a
right answer that does not depend on pixels, and every one of F6 §7's
cases is a `SignFunc` that fails in a particular way at a particular
document. They are ordinary tests, on any platform, with no card and no
browser engine.

The split earns itself immediately: `internal/jobs`'s tests cover a
hundred-document batch with one corrupt file, a card removed at fifty,
a disk filling at seventy, Stop landing mid-document, a window closing
mid-batch, two hundred files at once, and the same file listed twice —
none of which would be practical to drive through a window, and all of
which run in seconds.

**Three rules it implements that are not the window's to interpret.**
Skip and continue (SPEC §12.10) with two exceptions only —
`CARD_NOT_PRESENT` and `PIN_LOCKED` end a run, and those come from
`signing.AbortsBatch` rather than a second list here, so F2's own rule
has one implementation. Stop never interrupts a signature in flight,
because that is how a half-written file gets left (F6 §3, SPEC §18.10).
And a batch's reported level is the **weakest** any document reached,
never the best (SPEC §18.11).

**`internal/signing` gained four thin exported wrappers** —
`AbortsBatch`, `CodeOf`, `DetectPINPolicy`, `BuildTimingReport`,
`MedianOf` — each delegating to the unexported function F2 already had.
Reimplementing D-028's measured 2000ms threshold, or F2 §5.3's abort
list, in a second place is how two parts of one program come to disagree
about the same batch.

**Rejected.**
- **Putting the runner in `cmd/liro-bridge` beside the window.** It is
  where the F5 signing loop already lived, and it is why F6 §7's cases
  would each have needed a real window, a real card session and a real
  PDF to exercise a rule about ordering.
- **Naming it `internal/batch`.** "Batch" is SPEC §3 glossary for one
  user approval covering N documents, and `internal/signing` already has
  a `Batch` type for exactly that. Two `Batch`es in one program is a
  worse problem than a slightly duller package name.

---

## D-116 — The main window is a new window; the consent screen is the existing one, opened from it

**Date:** 2026-09-05
**Phase:** F6 — Parts 1, 3, 5

**Decision.** `/pages/main.html` is a new window with three screens —
the document list, the queue, the report. When Sign is pressed it calls
`askForConsent`, which opens **the consent window that already exists**,
unchanged: the same page, the same certificate list, the same timestamp
question, the same output-file question, the same audit entry, the same
code `sign --interactive` has been using since F5. That window closes as
soon as the card session is open, and the batch is then watched in the
main window.

`askForConsent` is the first half of `runSignInteractive`, factored out
so both entry points run the same one.

**Why not draw the consent screen into the main window.** SPEC §6.5
calls the consent screen "the only real gate" and the most important
paragraph in the specification. A second implementation of it — however
carefully written — is a second thing that has to stay right about
certificate choice, about never preselecting one, about asking the
timestamp and output questions before the card is touched, and about
what the audit log records. The two would drift, and the direction they
drift in is the one that matters.

The cost is that two windows appear in sequence rather than one changing
screens. That is a smaller price than two consent screens.

**Why the queue and report are new rather than reusing the consent
window's own progress and done screens.** Those show one line of
progress and a two-number summary, which is what F5 needed. F6 §3 and
§5 ask for per-document state, a Stop button, and a report listing
failures by name — a different screen, in the window that owns the
list.

**Rejected.**
- **One window with a consent screen inside it.** Above.
- **Keeping the consent window open behind the queue.** Two windows both
  claiming to be in charge of one batch; the consent window's own
  progress screen would sit there, stale, behind the real one.
- **Reusing `runSignInteractive` wholesale from the main window.** It
  owns its own window, its own progress rendering and its own exit
  code. Extracting the consent phase was the smaller change and left
  the command line's behaviour byte-identical.

---

## D-117 — Documents are read one at a time, at the moment each is signed

**Date:** 2026-09-05
**Phase:** F6 — Part 3

**Decision.** `interactiveInput` no longer carries a document's bytes.
It carries the path and the SHA-256 the consent screen's batch
fingerprint is built from, computed by streaming the file
(`io.Copy` into the hash) rather than reading it. `signInteractiveOne`
opens and reads the document itself, at the moment it signs it, and the
bytes are released when it is written.

**Why.** The F5 shape read every input in full before the consent window
even opened, and held all of them for the whole run. For the one or two
files a command line is given that is fine. F6 §7 asks for two hundred
at once, and a two-hundred-document batch of ordinary Serbian contracts
is hundreds of megabytes held from before the person has decided to sign
until after the last one is written — most of it long before and long
after the moment it is needed.

A file that cannot be read at that moment is now a named condition
rather than a fatal error before the window opens: `INPUT_UNREADABLE`, a
new code (below), skipped and reported like any other bad document.

**Rejected.**
- **Reading lazily but caching.** The same memory, spent less
  predictably.
- **Keeping the up-front read and capping the batch size.** A cap is a
  refusal wearing a number, and F6 §7 names two hundred as a case to
  handle rather than to refuse.

---

## D-118 — `INPUT_UNREADABLE` is its own code: a document that cannot be read is not a card problem

**Date:** 2026-09-05
**Phase:** F6 — Part 7

**Decision.** `errs.CodeInputUnreadable` (`INPUT_UNREADABLE`) is a new
code, returned when the document to sign cannot be opened. All three
catalogues carry a message for it.

**Why.** F6 §7 lists four situations that reach this: a file open
exclusively in another program (Acrobat holds a document that way while
it is open for editing), a file deleted or moved after it was added to
the list, a network drive that has gone away mid-batch, and a folder
where a file was expected. None of them is a problem with the card,
which is where `SIGN_FAILED`'s own message sends the user, and none is
unclassified, which is what `INTERNAL` means. This is the same argument
[[D-066]] made for `STAMP_GLYPH_MISSING` and [[D-104]] for
`OUTPUT_EXISTS`, applied to the one remaining condition in F6 §7's list
that had no code of its own.

**Verified.** Each of the four situations has a test that asserts the
code, asserts it is not `INTERNAL`, and asserts every locale renders it
as a sentence rather than a key — including one that takes a real
exclusive `CreateFile` handle on the document, which is the only way to
reproduce what an open editor does.

**Rejected.**
- **Reusing `PDF_INVALID`.** A document that cannot be opened has not
  been found to be invalid; saying so would send someone to check a file
  that is fine.
- **One code for input and output problems.** `OUTPUT_WRITE_FAILED`
  already exists ([[D-104]]) and asks for a different correction: one is
  "close it in the other program", the other is "the disk would not take
  it".

---

## D-119 — The Explorer entry hands its file to a shared inbox; one invocation opens a window and keeps collecting

**Date:** 2026-09-05
**Phase:** F6 — Part 2

**Decision.** The context-menu command is
`"<exe>" --shell-verb "%1"`, registered under
`HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign`
with `MultiSelectModel=Player` and the executable as the icon source.
Every invocation appends its path to a per-user inbox file and then
tries to claim a named mutex. The one that claims it waits for the
inbox to go quiet, opens a window for what is there, **and keeps
draining the inbox for as long as that window is open**, so a late
arrival joins the list already on screen. The others exit immediately.

**Why a file and a mutex rather than a socket or a pipe.** This is a
per-user, local, short-lived handover between processes that are all
this same program, and F6 is explicit that no server of any kind is
built this phase. The mutex is in the `Local\` namespace, so two users
signed in over RDP each have their own (SPEC §14.1).
`ERROR_ALREADY_EXISTS` from `CreateMutexW` is the whole signal — waiting
on the mutex would make the other nineteen processes queue up and each
open a window in turn, which is the opposite of what is wanted.

**Why the window keeps collecting — and how that was found.** The first
design was a quiet period alone, at 750ms, justified in a comment that
described a measurement nobody had taken. Running the real binary with
twenty real invocations produced the actual numbers: the arrivals spread
over **2390 ms**, with a typical gap between consecutive ones of about
**50 ms** — and **one gap of 1070 ms**, a stall somewhere in process
creation. That stall ended the quiet period early. Ten documents opened
a window; **the other ten were left in the inbox with nobody to open
them.** A silent half-batch, which is the worst outcome this product
has, and every unit test passed while it happened, because a fake clock
has no stalls in it.

Sizing the window to swallow a 2390 ms spread was rejected: it makes a
single right-clicked document wait two and a half seconds for a window,
the common case paying for the rare one. The window is 600 ms — more
than ten times the ordinary gap — and what makes the arrangement correct
is that it no longer has to be right. The open window drains the inbox
every 250 ms, so a straggler appears in the list; the collector drains
once more before it exits, so nothing is left behind if one arrives as
the window closes; and an inbox untouched for five minutes is discarded
rather than merged into an unrelated batch, which is the only remaining
way a path could be stranded (a collector that died).

**Verified against the real binary, twice.** Before: twenty
invocations, a window with eleven documents, nine left in the inbox.
After: twenty invocations, one process, one window, "20 dokumenata", the
inbox drained — with the log showing the window opening with ten and the
other ten joining 2.6 seconds later, which is the design working rather
than the defect recurring.

**One real bug the tests caught on the way.** `ShellMenuCommand` used
`strconv.Quote`, which produces *Go source* syntax and escapes every
backslash: an ordinary path came out as
`"C:\\Users\\Petar Petrović\\..."` and would have reached Windows with
every separator doubled. It is a pair of literal quotes now. A Windows
path cannot contain a double quote, so nothing needs escaping.

**The menu icon is the extracted Liro mark, not the executable.** The
obvious value for Explorer's `Icon` is the binary itself, whose first
icon resource the shell would use — except the binary has no icon
resource until F10 builds one (SPEC §5 puts `rsrc.syso` under
packaging), so that shows the generic Windows application icon.
`ui.IconFilePath` hands back the same extracted `.ico` the tray already
loads, so the menu entry and the tray show one mark from one asset.
Verified by asking the shell itself what it offers for a `.pdf` rather
than by trusting that a registry key implies a menu entry: `Verbs()`
lists "Potpiši koristeći Liro Bridge" second, immediately after Open.

**Registration happens whenever the agent runs, not only from the
tray.** It began as a tray-startup step, which leaves a chicken and egg
the entry cannot solve for itself: a person who reaches the agent any
other way — `liro-bridge open`, a shortcut, the first run before
autostart has ever fired — finds no entry to right-click. `runOpen`
applies it too. It is idempotent, and it is also what refreshes the
label after a language change.

**Rejected.**
- **A longer quiet window instead of the watcher.** It trades a rare
  wrong answer for a constant slow one, and it is still only a guess
  about the slowest machine anyone will run this on.
- **`HKCR` or `HKLM`.** Both need administrator rights the target user
  (a bookkeeper on a machine they do not administer) does not have, and
  a per-user program writing there is one its own uninstaller cannot
  clean up.
- **The `.pdf` progid rather than `SystemFileAssociations`.** A progid
  belongs to whichever application currently owns PDFs; the entry would
  vanish the day someone installs a different reader.
- **Omitting `MultiSelectModel`.** Without it Explorer hides the entry
  as soon as more than one file is selected — which is the case Part 2
  exists for.

---

## D-120 — The stamp gets its own window; explicit coordinates are clamped into the page, never refused

**Date:** 2026-09-05
**Phase:** F6 — Part 6

**Decision.** `/pages/stamp.html` is a new window carrying every stamp
decision: on or off, which of SPEC §13.1's four corners, which page
(first, last, or a number), the reference line, and whether to show the
identity document number. It is reached from the main window's summary
line and from Settings. The answers persist in `config.Config`
(`StampPage`, `StampReference`, `StampShowDocumentID` join the existing
`VisibleStamp` and `StampPosition`), and the main window shows the
current answer in one line.

Separately, `appearance.ClampToPageBox` moves a stamp requested at
explicit coordinates so that it sits inside the page box with the same
24 pt margin every corner placement keeps, and `Render` uses it.

**Why a window.** The owner's request after using F5: the stamp
controls were squeezed onto the consent screen, which is the screen a
person reads in two seconds to decide whether to sign. A reference line
is not a thing to configure there. [[D-103]] put them there because
there was nowhere else; now there is.

**Why clamping rather than refusing.** F6 §6 states it and gives the
reason: "a stamp nudged inside is better than a refusal, and a stamp
hanging off the page is not acceptable output". Refusing turns a
slightly-wrong coordinate into a failed batch; drawing it where it was
asked produces a document with a signature appearance half over the
edge, which the original Bridge would not allow either. A page too small
to hold the stamp and both margins is the one case where the margin
cannot be honoured on both sides; the stamp is pinned to the
bottom-left inset rather than shrunk, because its size is fixed by SPEC
§13.1 and a predictable corner is easier to reason about than a stamp
that silently changed size.

**The identity document number stays off by default**, and nothing here
changes that: SPEC §13.5 calls it personal data on a document that will
be sent to third parties. The window offers it with a sentence saying
so. The *national* identity number remains unreachable from the stamp
entirely, which is a different and stronger guarantee
([[D-054]], [[D-064]]).

**The reference line is sanitised** through F5 §5.3's own pipeline
before it is stored. It is free text a person types and this project
draws into a PDF other people read; a direction-override character has
no business there for the same reason it has none in a file name.

**Two things the window itself taught.** It was built at 460×560 and
that was too short: with every control shown, the form scrolled and the
margin note — the one line here a person reads once and needs to have
seen — was what fell below the fold. It is 680 now, with a test that
fails if the form scrolls again. And the page's `saved` flag survived a
fresh `init`, so a second posting into the same window reported a Save
nobody pressed; the page now resets it, because a page holds no state of
its own beyond what Go has told it.

**Rejected.**
- **Leaving the controls on the consent screen and adding the rest of
  them there.** More of exactly what was reported as wrong.
- **A visual placement picker.** SPEC §13.1 defers it to a later phase
  in as many words.
- **Refusing out-of-page coordinates.** F6 §6 rules it out directly.

---

## D-121 — A new batch clears the consent window's certificate choice

**Date:** 2026-09-05
**Phase:** F6 — Part 7

**Decision.** `consent.js`'s `renderWaiting` resets
`selectedThumbprint` to null and disables Approve, every time an `init`
payload arrives.

**Why.** SPEC §18.15 forbids remembering a certificate across sessions,
and F6 §7 asks for a test that the selection does not persist. Writing
that test found the rule failing in the one place it is implemented:
`renderWaiting` rebuilt the certificate rows — clearing every row's
`aria-selected` — but left the page's own `selectedThumbprint` and left
Approve enabled. A second batch posted into the same window arrived with
Approve already pressable, for a certificate chosen for different
documents.

No user could reach it: every consent window in production is created
fresh, and the F5 window flows post other *states* into a live window,
never a second `waiting`. It became visible the moment a test posted two
batches into one window — which is also the shape any future re-render
would take, and is why the fix belongs in the page rather than in the
test.

This is the same class as the stamp window's `saved` flag ([[D-120]]),
found the same way in the same session: page state that is not a
function of what Go last told it.

**Rejected.**
- **Giving the test its own window instead.** It would have made the
  test pass and left the defect. The property being tested is that a
  new batch is a new decision, and the window is the same window either
  way.

---

## D-122 — What F6 changed about how the work was checked

**Date:** 2026-09-05
**Phase:** F6

**Decision, recorded because the phase turned on it.** Three defects in
this phase were invisible to a green test suite and visible within
seconds of running the real binary and looking at the result:

1. **Nine of twenty documents silently dropped** by the Explorer
   coalescing ([[D-119]]). Every unit test passed. A fake clock has no
   process-creation stalls in it, so every arrival landed inside the
   quiet period by construction.
2. **The failed row squeezed its file name to two words** and put the
   reason beside it, because a `flex-basis: 100%` only moves an element
   to its own line if the row is allowed to wrap. The payload was
   correct; the rendering was not.
3. **The stamp summary read "strana Prva strana"** — "page First page" —
   because both the format string and the page label supplied the noun.

None of these is a subtle bug. All three were obvious on sight and
invisible to assertions about the data behind them. That is [[D-087]]'s
finding again, and this phase's contribution is only that it happened
three more times, in three different layers, in one phase.

What was done about it, beyond fixing them: the window-driving tests
now cover what is *rendered* rather than what was posted — computed
styles rather than the `hidden` property ([[D-106]]'s trap), the
geometry of a wrapped row, whether a scrolling region actually scrolls
— and the real binary was run and photographed for every screen this
phase added, including the two that a person can only reach by clicking
through a batch.

**The screenshots are taken with `PrintWindow`, not by capturing the
screen.** The first attempt used `CopyFromScreen` over the window's
rectangle and produced a photograph of an unrelated video playing on the
owner's desktop, because the window was not in front and
`SetForegroundWindow` does not simply grant that. `PrintWindow` with
`PW_RENDERFULLCONTENT` asks the window to render itself, so the capture
is that window's own pixels wherever it sits and whatever is on top —
and, more to the point, taking it does not require taking the
foreground away from whoever is using the machine. That is the same
judgement [[D-094]] made about input, applied to output.

**Processes started for verification were stopped by exact PID**, never
by image name, per F6's own rule — including the twenty-invocation runs,
where the surviving process was identified from its own log line before
being stopped.

**Rejected.**
- **Treating the three defects as ordinary bugs not worth an entry.**
  The pattern is the point: this is the third phase in a row where the
  test suite was green and the product was visibly wrong, and each time
  the gap was the same one.

---
## D-123 — What was actually refusing the drop: nothing under the cursor was a drop target, and the frame's own registration is never consulted

**Date:** 2026-09-05
**Phase:** F6 — first-real-use fix pass (Task 1)

**Decision.** A window that sets `ui.Options.OnFilesDropped` now does
three things, not two: `put_AllowExternalDrop(FALSE)` as before, then
`RegisterDragDrop` with this package's own `IDropTarget` on the frame
**and on every window WebView2 has created underneath it**
(`internal/ui/droptarget_windows.go`), then `DragAcceptFiles` as the
`WM_DROPFILES` fallback. The registration happens after the page has
finished loading, so the browser's window tree has stopped changing
shape. `initApartment` (`com_windows.go`) calls `OleInitialize` rather
than `CoInitializeEx`, because `RegisterDragDrop` is OLE.

**What was measured, in the order it was measured, because two
plausible explanations were wrong before the third one held.**

A window hosting WebView2 is not one window. Walking the tree with
`EnumChildWindows` and reading each window's class, extended style and
`OleDropTargetInterface` property:

```
LiroBridgeWindow             ex=0x00000110  ACCEPTFILES=YES  -
  Chrome_WidgetWin_0         ex=0x00000000  ACCEPTFILES=no   -
    Chrome_WidgetWin_1       ex=0x00200000  ACCEPTFILES=no   -
      Chrome_RenderWidgetHostHWND  ex=0x00000020  ACCEPTFILES=no  -
      Intermediate D3D Window      ex=0x00280024  ACCEPTFILES=no  -
```

1. **Integrity level, ruled out first.** This process and `explorer.exe`
   both measure medium (`0x2000`), read from each token's
   `TokenIntegrityLevel`. UIPI was never blocking the drop.
2. **`DragAcceptFiles` was called on the right window.** The frame's
   `WS_EX_ACCEPTFILES` bit is set — `ex=0x…110` above, read back with
   `GetWindowLongPtr`. [[D-114]] got that half right. It does not help,
   because that bit is consulted for the window the drop lands on and
   the drop does not land on the frame.
3. **Where the drop does land.** With `AllowExternalDrop` left at its
   default, exactly one window in the tree carries a registered
   `IDropTarget`: `Chrome_WidgetWin_1`. `put_AllowExternalDrop(FALSE)`
   makes Chromium revoke precisely that, which leaves **nothing
   anywhere under the cursor that accepts anything at all**. That is
   what Windows draws the no-entry cursor for. Both halves of [[D-114]]
   were individually correct and together produced a window that could
   not receive a drop.
4. **Registering an `IDropTarget` on the frame alone was tried next**,
   on the theory that the drop-target search walks up the parent chain
   — which is what every WinForms and WPF host of this control appears
   to rely on. Measured against a real drag by the owner: it does not.
   The registration was confirmed present on the frame (the property
   appeared where it had not been before) and the cursor stayed
   no-entry.
5. **Registering one on every window in the tree works.** The owner
   dragged one file, then several, then a folder; every one arrived,
   and the log says which window received it:

   ```
   ui: drag entered a registered window  hwnd=0x15a0488 class=Chrome_RenderWidgetHostHWND carriesFiles=true
   ui: drop received                     hwnd=0x15a0488 class=Chrome_RenderWidgetHostHWND paths=3
   ```

   `Chrome_RenderWidgetHostHWND` — four levels below the frame, and
   `WS_EX_TRANSPARENT`, so not even the window `WindowFromPoint`
   returns. No theory this project could have reasoned its way to.

**So the rule is: do not have a theory about which window receives the
drop.** Register on all of them and let every one answer the same way.
Which window Windows picks is its business; nothing here depends on
knowing.

**`OleInitialize` is explicit rather than inherited.** Measured:
WebView2 already calls it on this thread as part of its own setup, so
`RegisterDragDrop` happens to succeed either way today — with
`CoInitializeEx` only, Chromium's own drop target still appeared. That
is depending on the order and the internals of somebody else's
initialisation for a guarantee this package needs for its own call.
`shutdownApartment` undoes the matching one; the two keep separate
counts.

**Both delivery paths log.** `IDropTarget::Drop` and `WM_DROPFILES`
each say so, with the window and its class. Which of the two delivered
a drop is the difference between reading a bug report and guessing at
one — and this pass spent two rounds of the owner's time on exactly
that guess.

**What a test cannot prove here, stated plainly.** No test in this
project can drag a file. [[D-094]] forbids synthetic input on this
machine, and a test that drives `IDropTarget`'s methods directly proves
this package's own code and nothing about which window Windows hands
the drop to — which is the entire defect. The verification is the
owner's drag and the log line it produced. The window tree, the
extended styles and the drop-target properties above were all measured
by a throwaway program created and deleted in the same session (D-100's
pattern).

**A known limit, not fixed here.** The registration is a snapshot of the
tree at the moment the page finished loading. Each of this project's
pages navigates exactly once and never again, so the tree measured then
is the tree that lives for the window's life. A window that navigated
elsewhere later could grow a child nobody registered, and would refuse
drops over it with no error — the same silence this entry exists to
close. If a future window navigates more than once, this needs a
re-scan.

**Rejected.**
- **`DragAcceptFiles` alone ([[D-114]]).** Measured not to work, twice:
  the flag is set on the frame and the drop lands four windows below it.
- **An `IDropTarget` on the frame alone.** Measured not to work.
- **Reading `dataTransfer.files` in the page.** Names are not paths, and
  the web platform does not expose one — [[D-114]]'s reasoning here is
  unchanged and correct.
- **Making the WebView2 control smaller so a strip of frame shows.** A
  drop target the user has to aim at is not a drop target ([[D-114]]).
- **Leaving `CoInitializeEx` and relying on WebView2 having called
  `OleInitialize` first.** It does today. That is not a contract.

---

## D-124 — Three steps, each asking one thing; step 3 is a step, not a fixture

**Date:** 2026-09-05
**Phase:** F6 — first-real-use fix pass (Task 2)

**Decision.** Signing is three windows in sequence, and no question is
asked twice:

1. **The main window** — which documents. Its footer keeps the output
   folder and nothing else; the stamp summary and its Change button are
   gone.
2. **The consent window** — who is signing, how many documents, the
   certificate, the fingerprint and file list behind Details, and the
   approval. SPEC §6.5's gate, and nothing about stamps.
3. **The stamp window** — how to sign: visible or invisible, and when
   visible, one of SPEC §13.1's four corners. Its primary action is
   Sign.

Then the timestamp question if there is one, then the output-file
question if there is one, then the card session, then signing —
unchanged, and all still before the PIN, for the reasons [[D-095]] and
[[D-104]] give.

`consentRequest` gains `stamp *consent.StampChoice`. When it is
non-nil the answer is already given and **step 3 does not open a window
at all**; `askHowToSign` folds the supplied answer into the
configuration and returns. Nothing sets it yet: both front doors leave
it nil.

`stampOptionsFor(c, cfg)` is the single place a configuration becomes
what the engine draws, used by the main window and by `sign
--interactive` alike.

**Why step 3 must be skippable, and why that is designed now rather
than later.** When a program asks the agent to sign, the request
carries its own answer to how — visible or not, and where. A person
answering it again is being asked to re-decide something already
decided, and a flow that cannot skip the question would have to be
unpicked to allow it. So the skip is the request's, not the
configuration's: `req.stamp` is what the caller supplies, and with it
supplied the person sees the approval and nothing else — one window,
one click. `TestAskHowToSignSkipsTheWindowWhenTheAnswerIsSupplied`
pins it, raced against a deadline because a regression here would hang
rather than fail: `runStampWindow` waits for a click.

**Why the order is certificate, then how to sign.** The approval and
the certificate are the same screen, and that screen must be the one
thing a caller-driven signature shows. Putting step 3 before it would
give the same three windows in a different order, and would leave the
skipped case showing the approval second rather than alone. Step 3
after Approve is also where the timestamp and output-file questions
already are, and the consent window already opens Settings on top of
itself mid-flow ([[D-095]]) — a second window over it is a shape this
flow already has.

**Why the stamp window has two sizes and two labels.** The same window
is Settings' way in, where the answer is a standing preference rather
than the last step before a signature. Step 3 shows the mode and the
corner and is 440 × 310; Settings shows those plus the page, the
reference line, the identity-document toggle and the margin note, and
is 440 × 700. Both measured in a real window at their own size:
`TestStampWindowFitsBothRolesWithoutScrolling` creates one window per
role rather than measuring in a shared window of some other size,
because a size constant checked against a differently-sized window
proves nothing about the window a person opens.

A single window at a single size was built first and looked wrong both
ways round — photographed, not reasoned about. At the step's size the
preferences scrolled inside a hundred-point region, a sliver at a time.
At the preferences' size step 3 was a mostly-empty window asking one
question.

**Window sizes, all measured rather than adjusted by eye.**

| Window | Was | Now | Why |
|---|---|---|---|
| Consent | 520 × 860 | 520 × 760 | The stamp block was 96 points of the fixed content below the certificate list. Header and footer take 216 points between them; six certificate rows plus their gaps are 544 (six rows of content measured at 504.3, five 8-point gaps making up the rest), and 760 gives the list exactly that. Six is SPEC §14.1's bookkeeper. |
| Main | 560 × 720 | 560 × 690 | The stamp summary row left the footer, which measured 34 points shorter for it. |
| Stamp | 460 × 680 | 440 × 310 / 440 × 700 | One size per role, above. |

Settings, Certificates and the audit log are untouched: nothing in this
pass changed what they hold, and [[D-106]] measured each of them for
its own content.

**What the consent screen keeps, and a test that says so.**
`TestConsentScreenAsksNothingAboutStamps` asserts the three stamp
elements are absent from the DOM, that the approval screen carries no
`select` or checkbox at all, that the six things SPEC §6.5/§6.6 require
are present, and that Cancel still has initial focus (F5 §5.6).

**Two catalogue entries died with the controls** —
`consent.stamp_visible` and `consent.stamp_position_label`, along with
the four `consent.stamp_position_*` corner names, `main.stamp_change`,
`main.stamp_summary_on`/`_off`, `main.stamp_page_number` and
`stampwindow.visible`. Their Go callers (`jsStampPosition`,
`stampPositionText`, `stampPositionOptions`, `stampPageLabel`,
`readStampChoice`, `persistStampChoice`) went with them rather than
staying as unused surface.

**The command line asks the same three questions.** `sign
--interactive` reaches the same step-3 window the main window does, so
the two front doors do not diverge. It is worth recording that they are
still two implementations of the consent *loop* — [[D-116]] says
"`askForConsent` is the first half of `runSignInteractive`, factored
out", and that is true of the main window but not of the command line,
which kept its own copy because it holds its window open through the
whole batch for progress. Unifying them is a real piece of work and not
this pass's; what is shared today is the page, the payload builders,
step 3 and `stampOptionsFor`.

**Rejected.**
- **Drawing step 3 into the consent window as another screen.** It is
  the screen [[D-116]] and SPEC §6.5 protect from exactly this; the
  previous round put two stamp controls on it and this pass is the
  result.
- **Leaving the stamp summary on the main window as a read-only line.**
  Half the complaint: the answer would still be shown in step 1 and
  asked in step 3, which is how "asked twice" started.
- **Keeping the standing preferences behind a disclosure in step 3's
  own window.** Built, photographed, rejected — see above.
- **Driving the skip from the configuration rather than the request.**
  Then step 3 would be skipped always, for everyone, and the question
  would have no home at all.

---

## D-125 — The stamp is four lines: label, name in capitals, serial, date; only the label follows the interface language

**Date:** 2026-09-05
**Phase:** F6 — first-real-use fix pass (Task 3)

**Decision.** `appearance.buildLines` produces four lines, always, each
on its own line, left-aligned beside the logo:

```
Дигитално потписано
ВЕЉКО СТАНОЈЕВИЋ
SN 20F048A768F56F099E
04.09.2026. 15:04:33
```

then the caller's reference line and the identity document number, each
only when supplied — six at most.

- **Line 1** is the label, sentence case, from the catalogue:
  `Дигитално потписано` / `Digitalno potpisano` / `Digitally signed`.
  It used to read "…potpisao", with a "by" that had nothing after it.
- **Line 2** is the signer's name, upper-cased, built from `givenName`
  + `surname` as SPEC §11.7 requires and never parsed out of CN
  ([[D-019]], unchanged).
- **Line 3** is the certificate serial with an `SN ` prefix, truncated
  by `fitLine` like any other line if it does not fit.
- **Line 4** is the `/M` signing date as `DD.MM.YYYY. HH:MM:SS`.

**The label transliterates; the name does not.** This is the rule the
task names and it is SPEC §9.3's, applied to the one place the two
kinds of text sit on adjacent lines. The label is interface text: it is
whatever the catalogue holds for the locale the window is running in,
so a Cyrillic interface draws a Cyrillic label and Latin and English
draw Latin ones. The name is *data* — a certificate subject field —
and passes through in whatever script the certificate carries it,
whatever language the interface is in. A MUP certificate reads Cyrillic
and a Halcom one Latin in all three interfaces. Rendered and looked at,
both:

- MUP, Cyrillic interface: `Дигитално потписано` / `ВЕЉКО СТАНОЈЕВИЋ`
- Halcom, English interface: `Digitally signed` / `ZORAN MILOVANOVIĆ`

The second is also what proves the upper-casing is a change of case and
not of script: `strings.ToUpper` is Unicode-aware, so `Milovanović`
becomes `MILOVANOVIĆ` with its Ć intact — a character the font subset
already carries (F4 §3.2).

**The date is not localised either.** `02.01.2006. 15:04:05` in every
locale: the same digits in the same order, so a document signed in an
English interface and read in a Serbian one says the same thing. It is
still the `/M` value the signature dictionary carries rather than the
timestamp token's `genTime`, for the reason [[D-056]] gives — the
stamp's bytes are fixed before a TSA is contacted — and that is
unchanged. The seconds are new: a signature timestamp is read to the
second, and the previous form (`2026-09-01 12:00 CET`) had neither
seconds nor a shape a Serbian reader writes a date in.

**The height table is sized to the content, not the other way round.**
The serial and the time used to share one line — two facts crowded onto
one, neither readable at a glance — so the base stamp was three lines
and SPEC §13.1's table (`[44, 44, 46, 56, 72]`) was indexed for that.
Four base lines and up to six total need entries the table did not
have, and two it did have are no longer reachable. The first three
entries are SPEC's own, untouched; four and up are computed:

```go
const stampLineHeight = 10
const logoBlockHeight = LogoSize + 2*Padding // 44

func heightForTextLines(n int) float64 {
	h := float64(2*Padding + n*stampLineHeight)
	if h < logoBlockHeight { return logoBlockHeight }
	return h
}

var heightsByLineCount = [6]float64{
	44, 44, 46,
	heightForTextLines(4), heightForTextLines(5), heightForTextLines(6),
} // 44, 44, 46, 48, 58, 68
```

The derivation is executable rather than commented so it cannot drift
from the numbers it produced. 7pt type in a 10pt slot is ordinary tight
typesetting; the floor is the logo's own height, below which the box
would be shorter than the mark inside it. A four-line stamp is
therefore 48 points tall rather than SPEC's 56: 56 was sized for four
lines of which one was the *optional* reference, and spreading four
mandatory lines over it leaves 12-point gaps that read as a paragraph
rather than a stamp.

**Verified by looking at it, at the real size, with the real
certificate.** The 190 × 48 box was rendered from
`testdata/pdfs/blank.pdf` signed with the real MUP certificate's
Subject and serial — the fields the stamp draws — and rasterised with
PDFium at 10× and cropped to the stamp's own rectangle, so what was
inspected is the stamp and not a page containing one. Four lines, each
legible, the logo left and the text block beside it. Repeated for the
Halcom subject in the English locale. The harness was a throwaway
created and deleted in the same session (D-100).

**Rejected.**
- **Keeping "SN <serial>  <time>" on one line and adding the date as a
  fifth.** The task's own point: four lines, each on its own line.
- **`…` (U+2026) for the truncation mark.** [[D-057]] chose three ASCII
  periods and is not re-litigated here; the character is in the subset
  now, but the reasoning about a purely cosmetic difference stands and
  changing it would churn a build-time asset for nothing.
- **Localising the date format.** A trailing ordinal period is Serbian
  convention and would read as a typo in the English interface; the
  task's rule scopes the script to the label alone, and a date is not
  the label.
- **Vertically centring the logo now that the box is taller.** Measured
  at 48 points the logo's block is 44 of them; there is 4 points to
  centre in, and moving it 2 is not a change anyone would see.

---

## D-126 — A chosen output folder is reversible, and the chooser no longer answers "the Desktop" for a stray OK

**Date:** 2026-09-05
**Phase:** F6 — first-real-use fix pass (Task 4)

**Decision.** Two changes, neither to the rule.

*(a) `ui.ChooseFolder` takes a starting folder* and preselects it
(`BFFM_INITIALIZED` → `BFFM_SETSELECTIONW`, through one
package-level `syscall.NewCallback` singleton rather than one per
call — [[D-080]]'s trampolines are never released and this dialog opens
on a tray process that runs for weeks). The main window passes the
folder it already holds.

*(b) The main window's output row offers the way back.* A "Beside each
document" button appears next to the path only when a folder is chosen,
and clears it.

**What the rule was, and was already.** F6 §4's rule — each signature
beside its own input, a chosen folder overriding it, per input for a
batch gathered from several folders — was implemented correctly and
still is: `jobs.OutputPathFor` joins the output name to
`filepath.Dir(inPath)` when the configured folder is empty, and
`config.Default().OutputFolder` is empty.
`TestOutputPathsFollowEachInputsOwnFolder` pins it for three inputs in
three different folders.

**What actually happened.** The owner's `config.json` held
`"outputFolder": "C:\\Users\\Veljko\\Desktop"`, so the report saying
`Sačuvano u: C:\Users\Veljko\Desktop` was telling the truth about what
the agent had been told to do. How it came to be told that is the
defect, and it is two defects meeting:

- `SHBrowseForFolderW` with no starting selection opens on the Desktop
  **with the Desktop itself selected**. Pressing OK — which is what a
  person does when they meant to look around and changed their mind —
  silently answers "the Desktop".
- Nothing in the main window could unset it again. Choosing a folder
  was a one-way door: the only ways back were the Settings window's
  text field or editing `config.json` by hand.

Either alone is survivable. Together they are a program that quietly
starts writing every signed document to the Desktop and offers no way
to stop it.

**Verified in the window**,
`TestMainWindowOffersTheWayBackToBesideEachDocument`: the button does
not render with no folder chosen, renders when one is, sends
`clearOutputFolder`, and Go's handler empties the configuration and
re-renders the row as "the same folder as each document".

**Rejected.**
- **Changing the default.** There was nothing wrong with it.
- **Refusing the Desktop as an output folder.** It is a perfectly good
  place to put a signed document if that is what someone means. The
  defect is that they did not mean it.
- **Leaving the reset to the Settings window's text field.** It is
  where the value can be cleared, and it is two windows away from where
  the mistake is made and seen.

---

## D-127 — Two rounds of the owner's hands were spent on a guess; what that changes

**Date:** 2026-09-05
**Phase:** F6 — first-real-use fix pass

**Decision, recorded because the pass turned on it.** Task 1 could not
be verified by this project at all. [[D-094]] forbids synthetic input,
so the only way to know whether a drag works is for the owner to drag,
and each attempt costs an interruption. Two were spent:

- The first shipped an `IDropTarget` on the frame, on a hypothesis
  ("the drop-target search walks up the parent chain") that had never
  been measured, only inferred from how WinForms and WPF hosts of this
  control are documented to behave. It was wrong.
- Only the second round carried instrumentation — a log line naming the
  window each `DragEnter` and `Drop` reached — and that log is what
  identified `Chrome_RenderWidgetHostHWND` as the window Windows
  actually hands the drop to. It would have identified it a round
  earlier at no extra cost.

**One further thing was measured, and it matters more than the fix.**
The owner's first answer to "did the drop work" reported success. The
log showed no run of that binary at all, at any time in the relevant
window. The claim was not checked against the answer alone — it was
checked against the log, which did not support it, and so was not
recorded as verification. The second round left evidence: a process
whose start line appears in the log at 15:43:40, and drop lines at
15:48 naming paths and counts.

**So: an answer about what happened on screen is a lead, not evidence.**
Where the program can leave a trace, arrange for it and read it. This
project already learned that green tests are not evidence about what a
window does ([[D-087]], [[D-122]]); this is the same lesson pointed the
other way, at a report rather than a test.

**What was done about it.** Both drop delivery paths log which window
received the drop and how many paths came with it. Registration logs
each window it took. A window that ends up with no drop target at all
logs an error rather than looking like a drop target and refusing every
drop, which is what the previous build did in silence.

**Rejected.**
- **Treating the first answer as a false report.** Nothing supports
  that and it is not the useful reading. A person asked "did it work"
  while doing something else answers about what they remember; the
  program's job is to have left something that settles it.
- **Adding a permanent diagnostic subcommand.** Verification
  scaffolding does not belong in the product ([[D-100]]); the window
  tree, extended styles and drop-target properties were measured by a
  program created and deleted in the same session. What stayed is the
  logging, which is not scaffolding — it is what a bug report from a
  user's machine will need.

---

## D-128 — The font subset wrote zero for every left side bearing; the owner saw it as two touching letters

**Date:** 2026-09-05
**Phase:** F6 — first-real-use fix pass (found by the owner's acceptance run of Task 3)

**Decision.** `scripts/gensubsetfont`'s `buildHmtx` writes each glyph's
real left side bearing — its own outline's `xMin` — instead of zero, and
`buildHhea`'s three horizontal-extent fields (`minLeftSideBearing`,
`minRightSideBearing`, `xMaxExtent`) are computed from the glyphs rather
than left at zero. The committed `notosans-subset.ttf` is regenerated.

**What the owner saw.** Accepting the reworked stamp on a real signed
document: everything correct except that in `ВЕЉКО СТАНОЈЕВИЋ` the Ј and
the Е were too close together. Blown up from the rendered stamp, they
touch.

**What it was.** `buildHmtx` wrote `0` for every glyph's bearing, with a
comment saying the field is "not used by this project's rendering path".
It is used, and not by this project — by the rasteriser. A TrueType
renderer positions a glyph by shifting its outline horizontally by
`hmtx.leftSideBearing - glyf.xMin`; the outline is authored wherever the
designer put it, and `hmtx` is what says where its origin actually is. A
bearing of zero therefore moves **every** glyph left by its own `xMin`,
which is a different amount for every letter. Measured on the committed
asset before the fix, reading the glyph and metric tables directly:

| Glyph | `glyf.xMin` | advance | bearing written | displacement |
|---|---|---|---|---|
| Ј U+0408 | −78 | 273 | 0 | **+78** |
| О U+041E | 60 | 761 | 0 | −60 |
| Е U+0415 | 97 | 556 | 0 | −97 |
| С U+0421 | 60 | 640 | 0 | −60 |

All 177 glyphs were displaced. Ј is one of the few Serbian letters drawn
with a hook reaching *left* of its own origin, so it moved the opposite
way from its neighbours and by the most: right into the Е, while leaving
a gap after the О before it. That is the reported symptom exactly, and
it is why this letter is where it showed.

**Every test in the package was green, and could be.** They ask about
advance widths (`TextWidth1000`, `gidWidths`, the `/W` array) and about
glyph indices (`EncodeCIDs` against an independently parsed font).
Neither was wrong. The generator's own `verifySubset` checks that every
rune's GID and advance round-trip, and `verifyOutlineGeometry` checks
each glyph's bounding box against the source font's — a bounding box in
the glyph's own coordinates, which the bug does not touch. Nothing
compared the two tables that have to agree with each other. This is
[[D-087]] and [[D-122]] once more: what was checked was a layer below
the one that was wrong.

**The check that would have caught it, added.**
`internal/pades/appearance/font_metrics_test.go` reads the committed
font's own `head`, `maxp`, `loca`, `glyf`, `hmtx` and `hhea` tables — not
through any decoder, not through this package's own lookup tables — and
asserts, for all 177 glyphs, that `hmtx.leftSideBearing` equals
`glyf.xMin`, and that an outline-less glyph has a bearing of zero.
Confirmed to fail against the asset as it stood (every glyph reported,
starting `glyph 2: bearing 0, xMin 72`) and to pass after. A second test
names Ј and asserts it still overhangs to the left and that its two
tables agree, so the reason this letter showed first is recorded where
someone will read it.

**An assertion that was tried and is wrong, recorded so it is not tried
again.** The first version of that second test walked `СТАНОЈЕВИЋ` glyph
by glyph and required each letter's ink to start after the previous
letter's ink ended. It fails on a *correct* font: Ј's ink legitimately
begins 18 units before О's ends, because the two overlap horizontally
and not vertically — О's bowl and Ј's descender hook. Horizontal ink
overlap is normal typography, not a defect, and a test that would fail
on a correct font is worse than no test.

**A second, separate defect in the same writer, fixed while here.** The
format-4 `cmap` subtable declared its own length as `14 + subtableLen`
where `subtableLen` already included the 14-byte header — 238 bytes
declared, 224 present. `fontTools` refuses to parse the table at all
("corrupt cmap table format 4"). Nothing in this project's rendering
path reads it — Identity-H addresses glyphs by CID (F4 §3.3), which is
exactly why a self-contradicting length could sit in a shipped font
asset unnoticed — but an embedded font that contradicts itself is the
class of thing this project has already been bitten by four times
([[D-069]], [[D-072]], [[D-074]], [[D-075]]), always by one reader being
stricter than the rest. After the fix the table parses strictly and maps
all 176 characters.

**The source font, recorded because it was not before.** F4 §3.2 has the
subset generated at build time from a NotoSans-Regular.ttf the
repository does not carry, and nothing said which one. Regenerating to
apply this fix therefore had to start by finding it, and **it was not
found**: three fetchable NotoSans releases were tried and none
reproduced the committed `glyf` table. So the asset was, until now, not
reproducible from any stated input — a generated artefact nobody could
regenerate.

What was used instead, recorded so the next regeneration starts from
something rather than searching:

```
https://github.com/notofonts/notofonts.github.io/raw/main/fonts/NotoSans/unhinted/ttf/NotoSans-Regular.ttf
SHA-256 f3961a9cde016d41a4879aecda1474d3a36d6bf54fa0e4643de029cc2248b0e8
```

Unhinted rather than hinted because the writer strips hinting bytecode
anyway; both variants of that release produce a byte-identical `glyf`,
which is itself evidence they are one release.

**What changed in the asset, measured table by table rather than
asserted.** `charset.txt` and `subset_data.go` are **byte-identical** to
what was committed — the same 176 characters, the same GID assignment,
the same advance width for every glyph, so the `/W` array a PDF carries
and every width this project computes are unchanged. `cmap`, `name` and
`post` are byte-identical. `hmtx`, `hhea` and `head` differ by this fix.
`glyf`, `loca` and `maxp` differ because the outlines come from a
different Noto Sans release: 28 800 bytes against 32 706, fewer control
points for the same letters. That is a real change and it is stated
rather than glossed: the same letters at the same widths, redrawn by
their own designers.

**Verified by looking at it, before and after.** The 190 × 48 stamp,
signed with the real MUP certificate's Subject and rasterised at 10× and
cropped to the stamp's own rectangle: before, `СТАНОЈЕВИЋ` has a visible
gap between О and Ј and none between Ј and Е; after, the word is evenly
spaced. Both images were looked at, not measured — the defect was
reported by eye and is settled by eye.

**Rejected.**
- **Patching `hmtx` in the committed `.ttf` by hand.** It would have
  changed fewer bytes and left the asset exactly as unreproducible as it
  was, which is half of what let this survive.
- **Blocking on finding the original source font.** Three releases were
  tried. Continuing to hunt for a font whose only distinguishing property
  is that its outlines carry more points, in order to avoid a change that
  leaves every metric identical, is not a good use of the time — and the
  search is itself the argument for recording a source.
- **Leaving the `cmap` length wrong because nothing reads it.** That
  reasoning is what produced the bearing bug in the first place: a field
  dismissed as unused by "this project's rendering path", in an artefact
  handed to other people's readers.

## D-129 — Settings went dead because a window opened from it was created underneath it, and the window behind then froze on a channel send

**Date:** 2026-09-05
**Phase:** F6 — second-real-use fix pass (Task 1)

**Decision.** Two changes in `internal/ui`, both of them properties of
the package rather than of any one window.

*(a) `Options.Owner`.* A window opened from another window names it.
The owner goes into `CreateWindowExW`'s `hWndParent` slot, which for a
`WS_POPUP` window makes it the *owner*: Windows then keeps the new
window above that one in z-order unconditionally, whatever either one's
topmost flag says. The new window is centred on its owner rather than
on the monitor under the cursor, and the owner is disabled
(`EnableWindow(owner, FALSE)`) for as long as it is up, re-enabled on
the way out — before `DestroyWindow`, so activation returns to it
rather than to whatever else is on the desktop. Three call sites pass
one: Settings opening the stamp window, the consent window opening step
3, and the consent window opening Settings from the timestamp question.

*(b) No caller's callback runs on a window's message-loop thread.*
`OnMessage`, `OnFilesDropped` and `OnClosed` are queued by the thread
that produces them and delivered, in order, by one goroutine per window
(`dispatchEvents`). The queue is a slice under a mutex, not a channel,
because it must never make the producer wait — a channel of any fixed
size eventually blocks, and blocking the producer is the whole defect.

**What the state actually was — measured before anything was changed.**
The owner reported that Settings makes the program stop responding, and
asked which of a hang and a crash it was. It is a hang, and nothing was
wrong in COM: the process was alive, no exception was raised, both
windows' threads existed, and the WebView2 controller was never touched
from the wrong thread. Neither of the two candidates this project's own
history offered was involved — no `runtime.Pinner` site was missed
([[D-101]]), and the teardown ownership rule ([[D-101]] again) held.

*Save and Close are not it.* `handleSettingsAction` for a save returns
in **6 ms**; `Window.Close` on the settings window returns in **13 ms**.
Driven through the real production nesting — a real tray, its
`WM_COMMAND` calling `runSettingsWindow`, and a `WM_CLOSE` posted to the
settings window as a person's click on the title bar would —
`runSettingsWindow` returned in **15 ms** and the tray answered the next
menu command immediately afterwards.

*What is it.* Pressing **Podešavanja pečata** in Settings calls
`runStampWindow`, which opened a window with `AlwaysOnTop: false`
(that role is not a step) and no owner, while the settings window is
`WS_EX_TOPMOST`. Both windows are centred on the same monitor, and 440
× 700 fits entirely inside 520 × 880, so the new window was created
**exactly underneath** an always-on-top window. Measured rather than
reasoned about: `WindowFromPoint` at the centre of the new window
returned the settings window's HWND, not its own. Go was then blocked
in `runStampWindow` waiting for a click on a window nobody could see or
reach.

*And then the window behind it froze.* Every window in this project
delivers page messages by sending on a channel of eight, and
`runSettingsWindow`'s loop — the only thing draining that channel — was
inside the wait above. The ninth click filled the channel and the
callback blocked **inside `wndProc`**, on the settings window's own
message-loop thread. Reproduced deterministically and dumped:

```
goroutine 9 [chan send, locked to thread]:
  ...OnMessage
  internal/ui.dispatchMessage (messages.go:79)
  internal/ui.webMessageReceivedInvoke (webview2_windows.go:169)
  syscall.syscalln
  ...DispatchMessage
```

From that point the settings window did not repaint, did not answer
`SendMessageTimeoutW(WM_NULL)`, and could not be closed — `Close`'s
posted `WM_CLOSE` had no loop left to dispatch it, so it waited out its
five-second teardown timeout and returned with the window still on
screen. The tray was gone too, for a reason that predates this pass:
`OnSettings` runs inside `trayWndProc`, so the tray's own loop is
blocked for as long as a settings window is open. Three things dead,
one cause.

**Why both halves are fixed, not just the visible one.** Making the
stamp window always-on-top would have put it in front and looked like a
fix. It would have left the second half untouched — any future window,
folder chooser, or slow handler that keeps a caller busy for nine
clicks would freeze the window it was opened from — and it would have
put two topmost windows in a fight neither wins predictably. Ownership
is the property that actually says "this window came from that one",
and it also gives the disabled owner, which stops the clicks being made
at all rather than making them harmless.

**Verification.** `TestAWindowOpenedFromSettingsIsReachable`
(`cmd/liro-bridge`) presses the stamp button through the page's own DOM
([[D-094]]'s `Eval` carve-out — nothing here touches the real cursor),
waits for the window it opens to actually be *shown* (not merely
created: the frame exists about two seconds before it is raised, and
asserting z-order in between measures nothing), and then asks what a
person asks — `WindowFromPoint` at its own centre must be itself, and
Settings must be disabled. Run with `Options.Owner` removed it fails
with "the window opened from Settings is covered: a click at its own
centre would reach 0x1c0682, not 0x2770454". It then puts twenty clicks
into Settings while Go is not reading, which the pre-fix build could
not survive past nine.

`TestBlockingCallbackDoesNotFreezeTheWindow` (`internal/ui`) is the
general form: a window whose `OnMessage` never returns still answers
`WM_NULL`, still runs `Eval`, still closes inside its teardown timeout,
and delivers all twenty queued callbacks in order once released. Run
against the pre-fix dispatch it hangs to the test's own 90-second
deadline.

`TestSettingsOpensAndClosesRepeatedly` is the volume the task asked
for: a hundred cycles of open, Save through the page, read the form
back through `Eval`, close — alternating Go's close with the title
bar's, since they take different paths. Three runs of a hundred, back
to back: 3m36s, 3m35s, 3m41s, all green — and `LIRO_SETTINGS_CYCLES`
takes a longer one. It deliberately stops short
of the save's side effects: `os.Executable()` inside a test binary is
the test binary, and a hundred saves would point the developer's own
autostart at a temporary file.

**Rejected.**
- **Making the stamp window `AlwaysOnTop`.** Above: fixes the symptom
  by accident, leaves the freeze, and makes two topmost windows
  contend.
- **A bigger message channel.** Any fixed size is a number of clicks
  after which the window dies. Sixteen would have moved the report from
  nine clicks to seventeen.
- **Dropping messages when the channel is full.** It keeps the window
  alive by throwing away a person's click, which is worse than the
  freeze in one specific way: the freeze is at least obvious.
- **A timeout on `runStampWindow`'s wait.** It would return control
  eventually, to a person who has no idea why, having closed a window
  they never saw.
- **Moving the tray's callbacks off `trayWndProc` in this pass.** Real
  — the tray is inert for as long as any window it opened is open — but
  it is a separate change to the tray's own threading, it was not what
  made the program dead, and this pass is five bounded fixes. Recorded
  here for whoever takes it.

---

## D-130 — The duplicate report was two documents with one name, not a check whose result was thrown away

**Date:** 2026-09-05
**Phase:** F6 — second-real-use fix pass (Task 2)

**Decision.** `jobs.Item` carries the folder its document is in, and
`jobs.NeedsFolder` says, per item, whether its name alone identifies it
in this list. The main window renders the folder on exactly those rows
and no others. `Queue`'s duplicate rule is unchanged: full path,
case-folded, `filepath.Clean`ed — two files called `ugovor.pdf` in two
folders are two documents and both are kept, two drops of one path are
one document and the second is refused out loud.

**What was measured, and what was not found.** The report is that the
message appears and the file joins the list anyway, with a list showing
`TEST 1..4` and then `TEST 4` and `TEST 3` a second time. Taken at face
value that means `Queue.appendItem` returned false — producing the
notice — while the item still reached `items`, which the code cannot
do. So each link was measured instead of read:

- `Queue.Add` refuses a repeat, whether it arrives as a second call, as
  the same path twice in one call, or as a folder alongside a file
  inside it. Case and separators do not defeat it.
- The window's own path was driven end to end: a real main window with
  a real `OnFilesDropped`, its real loop on its own goroutine, and a
  real `WM_DROPFILES` carrying a real `HDROP` — four documents, then
  each of two dropped again. Four rows, two duplicate notices, nothing
  added twice.
- The paths the shell hands over are exact. A genuine shell
  `IDataObject` for two real files (`SHCreateDataObject` over their
  pidls — the same object Explorer gives a drop target) read back
  through this project's own `dataObjectFiles` returns
  `C:\Users\Veljko\Desktop\TEST 3.pdf` and `…\TEST 4.pdf`, byte for
  byte what the files are called on disk.
- The owner's own log for the reported session shows one process, one
  main window, and one `drop received` line per gesture — no double
  delivery, and the binary that wrote it is the current tree (it
  contains `output-beside-btn` and not `stamp-change-btn`).

**What does reproduce the screenshot**, exactly: two documents with one
name, from two folders. Both are kept, correctly, and the list then
shows the same name twice with nothing to tell them apart — beside a
notice about a *third* drop being a genuine repeat. From outside, that
is indistinguishable from a check that ran and was ignored, and it is
how it was reported. The report is right about what it looked like and
about what to do next; it is wrong about the cause, and this entry
records that rather than fixing something that is not broken.

**The honest limit.** No sequence of drops was found that puts one path
into the queue twice, and this pass could not reproduce one. If the
owner sees two rows that are the *same* folder as well as the same
name, that is a different defect and this entry is not it — which is
why `addPaths` now logs, on every add, how many paths arrived, how many
were added, how many were duplicates and how many unreadable, plus the
queue length before and after. Counts only: SPEC §18.3 forbids a file
name in any log file, and the counts are what settle the question
anyway.

**The test was the other half of the report, and it was right.** F6 §7
asked for "the same file listed twice" and the test that existed handed
all three paths to the queue in one call, as the command line's initial
paths. That is not what a person does. It now drops the file, looks at
the list, and drops it again — one `Add` per drop, through the window,
with a render in between — and covers the two other shapes (one drop
naming it twice; a folder and a file inside it) alongside.
`TestTwoDocumentsWithOneNameAreBothKeptAndBothLegible` covers the case
that actually happened: three documents, two sharing a name, three
rows, exactly two of them carrying a folder, and no notice, because
nothing was refused. The queue screen follows the same rule and the
same test checks it — watching two rows called `ugovor.pdf` and being
told that one of them failed is this defect happening two seconds
further on. The report's failure list is left alone: naming a failure
unambiguously would mean carrying the folder through `jobs.Failure` as
well, which is a wider change than what was reported, and the queue
screen above it already says which is which while the run is
happening.

**Rejected.**
- **Comparing by file name.** It would have made the report go away by
  losing a document, which is the one outcome worse than showing two
  rows that look alike.
- **Resolving each path to its file identity** (a handle plus volume
  and index, or `GetFinalPathNameByHandle`) so that a junction or an
  8.3 short name cannot produce two entries for one file. It is the
  strictly more correct comparison and [[D-115]] already weighed and
  declined it; nothing measured here changes that trade — a handle per
  path is a real cost on a network share for a case still not observed.
- **Showing the folder on every row.** A batch gathered from one
  folder — the ordinary case — would carry the same path on every line
  to make legible a case that is not happening.
- **Truncating the folder with an ellipsis.** [[D-096]]'s rule: a value
  with no length bound wraps, it does not widen its container, and a
  folder a person cannot read in full is not an answer to "which one is
  this".

---

## D-131 — The wait before the consent window is the window, not the certificate work

**Date:** 2026-09-05
**Phase:** F6 — second-real-use fix pass (Task 3)

**Decision.** The main window's Sign button takes itself out of service
on the first press and says `Otvaranje…` / `Отварање…` / `Opening…`
until the list is rendered again. Nothing about the order of the
consent phase changed.

**Why not reorder it — the measurement.** The task asked whether the
per-certificate presence probe ([[D-077]]) is the cost, and whether the
consent window should open first and fill its certificate list as it
arrives. Measured on the machine this was reported from, three runs
each:

| Step | Cold | Warm |
|---|---|---|
| `tsl.NewFileStore` | 54 ms | 36–38 ms |
| `windowscng.Enumerate` (4 certificates) | 60 ms | 1–2 ms |
| presence probe, all four | 44 ms | 10–13 ms |
| the whole `cli.Gather` | 905 ms | 191–322 ms |
| **`ui.NewWindow` for the consent page** | **2.29 s** | **2.12–2.15 s** |
| first `PostJSON` into it | 9 ms | 11 ms |

Per certificate the probe costs between 1.5 ms and 14.6 ms. So the
answer to the question as asked is no: the probe is one or two per cent
of the wait, and the whole certificate gather is under a fifth of it.
**The wait is the WebView2 window**, which costs a little over two
seconds every time and would still cost it if the list arrived
afterwards. Showing the window first would move about a quarter of a
second and add a second rendering state to the screen SPEC §6.5 calls
the only real gate.

**What would actually make it faster, recorded rather than done.** Each
`ui.NewWindow` builds its own WebView2 *environment*; one environment
per process, shared by every window, is what Microsoft's own samples do
and is what [[D-099]] already noted in passing. That is a change to
this package's lifetime model, not a button, and it is not one of this
pass's five fixes.

**Verified** by `TestSignSaysItIsOpeningAndCannotBePressedTwice`: the
button reads Sign and is pressable, one press disables it and relabels
it, a second press produces no message at Go at all, and a later render
of the list restores both.

**Rejected.**
- **A separate spinner.** The task's own preference, and correct: the
  button is where the person is looking and where the press happened.
- **Disabling the whole window.** Browse and Clear are harmless while
  the consent window opens, and a window that greys out entirely reads
  as the freeze this pass is otherwise removing.

---

## D-132 — Step 3 is named for the question it asks, and asks only it

**Date:** 2026-09-05
**Phase:** F6 — second-real-use fix pass (Task 4)

**Decision.** `stampwindow.title` is `Način potpisivanja` / `Начин
потписивања` / `Signing method` in the three catalogues, and
`stampwindow.subtitle` — "Poslednji korak. Sve ostalo je već odlučeno."
— is deleted from all three, with the Go that read it. Step 3 posts no
subtitle and the page hides the element rather than leaving it empty,
because an empty paragraph still takes its line box and its gap.
Settings' way into the same window keeps its own subtitle: there the
window is a standing preference and the line says which.

The window's step height goes from 310 to 280 points. Measured in a
real window in `sr-Cyrl`, the longest of the three catalogues, the form
needs 143 points and starts to scroll below 270; the ten points above
that are so a one-word label change does not immediately put it back,
and `TestStampWindowFitsBothRolesWithoutScrolling` is what says so.

**Why.** The subtitle told a person what they could already see, on a
window whose whole value is being small — [[D-124]] built it at one
question per step and then spent a line saying that. "Kako potpisati"
names the act of asking; "Način potpisivanja" names the answer, which
is what the window is for.

**Verified** by `TestStepThreeIsTitledForWhatItAsksAndSaysNothingElse`,
in all three locales: the title is the new one, the deleted key is gone
from the catalogues rather than merely unused, and the subtitle element
is hidden with zero rendered height in step 3 and present in Settings.

---

## D-133 — Export is where the log is

**Date:** 2026-09-05
**Phase:** F6 — second-real-use fix pass (Task 5)

**Decision.** The audit log window gains an **Izvezi** / **Извези** /
**Export** button beside Close, and a status line under it. It calls
`exportAuditLogNow` — the same function Settings has called since
[[D-097]], not a second copy — which asks for a folder, writes
`liro-audit-<stamp>.jsonl` and `liro-audit-<stamp>-report.json` there,
and names the folder on screen. `postSettingsStatus` is renamed
`postWindowStatus`, because two windows now render that payload.

The page sends `approve` for the one action this window has, so the
page→Go surface stays at the three types F5 §2.4 fixes ([[D-083]])
without a fourth message type or a form to read back.

**Why the format is not revisited.** One JSON object per line plus a
verification report is what a log meant to be kept for years should be:
append-only, one entry per line, readable without this program, and
checkable against its own hash chain by anyone. Nothing about moving
the button is a reason to touch it.

**Verified.** `TestAuditLogWindowOffersExportBesideClose` reads the real
window in all three locales: the button carries the catalogue's own
text, sits in the same action row as Close with both on screen, and its
click reaches Go as `approve`.
`TestAuditLogWindowSaysWhereTheExportWent` posts a real status through
`postWindowStatus` and reads the DOM back — hidden before anything
happens, then carrying the destination folder, with a non-zero rendered
height and its colour from the intent family ([[D-093]]) rather than
one chosen at the call site.

What no test here can do is press OK in the folder chooser: it is a
native modal dialog and [[D-094]] forbids simulating the click, so the
last step — the two files on disk, the destination named on screen — is
the owner's to confirm, exactly as [[D-097]] already recorded for
Settings' own Export.

---

## D-134 — Settings was never losing the value: it wrote it correctly and then never read it again

**Date:** 2026-09-05
**Phase:** F6 — third-real-use fix pass (Task 1)

**Decision.** The configuration file is the only authority on what the
configuration is. Three consequences, all of them in
`cmd/liro-bridge`:

*(a)* `settingsOnOpening` — new, and what `runSettingsWindow` now calls
— reads the file at the moment the window opens and takes both the
form's values *and the window's own interface language* from what it
finds. `runSettingsWindow`'s `locale` parameter is gone: a window that
has just read the configuration does not need to be told what language
it is in.

*(b)* A save folds the form onto `currentConfig(cfg)` — the file as it
stands at that moment — not onto the `config.Config` the window was
opened with.

*(c)* Every window the tray opens, and the tray's own menu labels, ask
`currentConfig` rather than being handed the copy the process read at
startup. `ui.TrayOptions.Labels` is now a `func() TrayLabels`, called
each time the menu is built, so the menu that opened Settings is in the
language chosen there the next time it is opened.

**Which of the four steps loses it — measured in order, before anything
was changed.** The report was that a language changed to Serbian
Cyrillic and saved comes back as Latin on reopening, and that the same
happens to every other field. The previous pass had timed
`handleSettingsAction("save")` at 6 ms and treated Save as innocent;
returning quickly says nothing about what was written. So each link was
measured separately, through a real settings window and the real
handler:

| Step | Result |
|---|---|
| 1. the page sends the changed value | **yes** — all thirteen fields, exactly as typed |
| 2. Go receives it and builds the right Config | **yes** |
| 3. Save writes it to disk | **yes** — every field correct in `config.json` immediately afterwards |
| 4. the reopened window reads it back | **no** |

Step 4 alone. `runSettingsWindow` rendered the `config.Config` it was
handed, and its only production caller is the tray, which reads the
file once in `run()` and holds that value for the life of the process.
So the reopened window showed what was true when the agent started.
The value was never lost in the sense the report suggested — it was
written correctly and then never read again.

The second half is worse than the first and is what makes it look like
nothing sticks at all: because a save folded the form onto that same
stale copy, the *next* save wrote the pre-change values back over the
file. Measured, one run:

```
step 3 (config.json immediately after Save):   "locale": "sr-Cyrl"
step 4 (what the reopened window renders):     locale = "sr-Latn"
step 4b (config.json after a second Save):     "locale": "sr-Latn"
```

The owner's own `config.json` is consistent with this: `signatureLevel`
was `b-b`, which is not a default and can only have been saved, while
`locale` was back at `sr-Latn`.

**Confirmed in the shipped binary, without simulating any input.**
[[D-094]] forbids driving the real cursor and Settings is only
reachable by clicking a tray icon, so the tray was opened by posting
`WM_COMMAND(trayCmdSettings)` to its own message-only window — a window
message, the same instrument [[D-129]]'s test uses for `WM_CLOSE`, not
a synthesised click. With `config.json` saying `sr-Cyrl`, the agent was
started, Settings opened and photographed (Cyrillic, correct), the
window closed, the file changed on disk to `en` underneath the running
agent, and Settings opened again. Before the fix the second window was
still in Cyrillic with "Srpski (ćirilica)" selected while the file said
`en`. After it, the second window is entirely in English with "English"
selected. That is step 4, in the real binary, both ways round.

One thing that measurement taught on the way: the first attempt wrote
`config.json` from PowerShell, which added a UTF-8 BOM, and
`config.Load` returned `Default()` because the file no longer parsed —
so the window looked stale when it was in fact reading a file it could
not use. Worth knowing on its own: a hand-edited `config.json` saved
with a BOM is silently replaced by defaults, with only a warning in the
log.

**The same fault one step removed, inside a single window's lifetime.**
The stamp window is reachable from Settings and saves its own answer to
disk while Settings is still open. A save that folds the form onto the
Config Settings was opened with therefore wrote the pre-stamp values
straight back over it — the visible stamp switched off again, the
corner, the page, the reference line and the identity-document toggle
all reverted, seconds after being set. This is the *same* defect
[[D-129]]'s comment claims to have fixed by replacing a freshly-built
`config.Config` with the caller's copy: it moved the staleness one step
away rather than removing it. `TestSettingsSaveKeepsWhatTheStampWindowWrote`
pins it.

**Why the existing test did not catch it.** The comment on the save
path was right about the defect it described, and the test that pinned
it — `TestSettingsSavePreservesTheStampChoice` — was structurally
unable to see this one: it called `handleSettingsAction` with the very
`config.Config` it then wrote its assertions from. That can only ever
prove "a save folds the form onto whatever it is handed", which is
exactly what the code did and exactly what was wrong. It never touched
the file between a save and a reopen, and it never opened a second
window at all, so there was no step 4 in it to fail. It is rewritten to
put the unshown fields on disk and *nowhere else*, and to hand in a
Config that deliberately disagrees with them.

The general shape, which this project has now hit three times
([[D-087]], [[D-101]], here): a test that supplies both the input and
the expectation from the same value measures the function's internal
consistency, not the product's behaviour. The new tests go through the
file.

**Verification.**
`TestEverySettingsFieldSurvivesSaveCloseAndReopen` is the report as a
test: it opens a real settings window through the real payload, changes
all twelve controls through the page's own DOM ([[D-094]]'s `Eval`
carve-out), presses Save through the page, runs the real
`handleSettingsAction`, reads `config.json`, then reopens through
`settingsOnOpening` with the stale Config a tray would still be holding
and reads every control back — and then saves a second time and checks
the file again. Against the pre-fix build every one of the twelve fails
twice, once on the reopen and once on the second save.
`TestSettingsWindowOpensInTheLanguageOnDisk` asserts the same property
against the window `runSettingsWindow` itself opens, using the only
thing a window shows that is readable from outside its own page: with
`sr-Cyrl` on disk and `sr-Latn` in the caller's Config, a window titled
"Подешавања" must appear. Pre-fix it never does.

Full suite green on Windows with `-count=1`, with and without the
`softtoken` tag; `gofmt`, `go vet` and `golangci-lint` clean in both the
Linux and the Windows view; `checkdeps` and `checkcss` OK.

**Found while measuring, and fixed: one press of the stamp button sent
seven messages.** `syncPresetSelection` in `settings.js` had a copy of
the stamp button's click handler pasted inside it, so it registered
another listener every time it ran — once on init and once per
keystroke in the TSA URL field. Measured through a real window: five
keystrokes then one press produced **seven** `approve` messages at Go,
which is seven stamp windows opened one after another. One paste
deleted; `TestOneStampButtonPressSendsOneMessage` measures it as one.

**Also found, deliberately not fixed here.** `internal/platform`'s own
`TestWindowsAutostartRoundTrip` and its shell-menu tests write to the
real `HKCU\...\Run` value and the real Explorer verb, and restore only
whether the entry is *present*, not what it pointed at — so a full
`go test ./...` on a developer's machine leaves their autostart entry
reading `C:\test\liro-bridge.exe` and their context-menu label in
whichever language ran last. It is a defect in a package neither of
this pass's two tasks touches, so it is recorded rather than changed.
What is changed is that no test *this pass* adds makes it worse:
`keepThisMachinesAutostartAndMenu` snapshots both registrations and
puts them back verbatim, and the existing save test now uses it too —
without it, `os.Executable()` inside a test binary pointed this
machine's autostart at a deleted file in a temporary directory, which
was measured, not assumed.

**Rejected.**
- **Passing the saved `config.Config` back up to the tray so it can
  update its copy.** It fixes the tray and leaves every other holder of
  a Config — the consent window, a future caller — with the same
  problem, and it makes the file no longer the authority. Reading the
  file is one line and cannot be got wrong twice.
- **Caching the file's contents behind a modification-time check.** The
  configuration is read when a window opens, which is a few times an
  hour at most, and `config.Load` of a 500-byte file is not a cost
  worth a cache's failure modes.
- **Reloading the whole tray — icon, menu, tooltip — after a save.**
  The menu's labels are the only language-dependent part, and they are
  now asked for at the moment the menu is built, which is strictly less
  machinery than a rebuild and cannot leave the icon half torn down.
- **Making Settings apply the language to windows already open.** No
  other window is open while Settings is: [[D-129]] gives Settings its
  owner and disables it, and the tray is inert for as long as a window
  it opened is up. There is nothing on screen to re-render.

---

## D-135 — The export names both files and says what each one is; the chain check speaks in words, not in `BrokenAt`

**Date:** 2026-09-05
**Phase:** F6 — third-real-use fix pass (Task 2)

**Decision.** The export confirmation carries a heading and one line per
file:

```
Dnevnik revizije je izvezen u C:\Users\Veljko\Desktop\Izvoz
  liro-audit-20260905-205112.jsonl        20 zapisa
  liro-audit-20260905-205112-report.json  provera ispravnosti: u redu
```

`postWindowStatusFiles` carries a `files` array alongside the existing
status text; `exportedFileLines` builds it. Four new catalogue keys in
all three locales — `settings.export_entries`,
`settings.export_entries_one`, `settings.export_check_ok`,
`settings.export_check_broken` — and `settings.export_chain_broken`,
which said one sentence about the folder and named no file, is deleted
from all three rather than left unused ([[D-132]]'s rule).

When the hash chain does not verify, the second line says so in words
and names the entry:

```
  liro-audit-…-report.json   provera ispravnosti: NIJE PROŠLA —
                             dnevnik je izmenjen kod zapisa 13
```

and the whole confirmation takes the negative intent family
([[D-093]]), heading included. Both files are still written and the
heading still says where they went, because they were.

**Why.** The owner opened the report and asked what it was:

```json
{ "entryCount": 137, "result": { "OK": true, "BrokenAt": -1 } }
```

That is correct output and it belongs with the log — it is the
hash-chain check SPEC §6.7 requires the log to be provable against, and
the thing that makes an export worth keeping rather than a copy of a
file. What was missing was any sentence on screen saying which file was
which. Two files appearing in a folder with nothing said about either is
how a correct verification report comes to look like debris.

The failing case is the one the report exists for, and it was the worse
of the two: `settings.export_chain_broken` said "exported to «folder»,
but the verification report says the chain is broken" — a sentence about
a *report*, naming neither the file nor the entry, which reads as a
technical footnote rather than as "this log has been altered". The
entry is now named.

**Which number is shown.** `VerifyResult.BrokenAt` is a zero-based index
into the exported entries. The exported log holds one JSON object per
line, so it is reported as `BrokenAt + 1` — the line a person can
actually count to in the file they have just been handed. Deliberately
not `Entry.Sequence`: rotation means the first exported entry is not
necessarily sequence 0, and a number that does not match the file in
front of them is worse than no number.

**Rendering.** `liroRenderStatusFiles` lives in `bridge.js` and
`.liro-status-files` in the generated `intents.css` ([[D-086]]), because
two windows have an Export button and render the same payload in the
same place — the settings window at 520 points and the audit log window
at 460. A two-column grid, the name in `--liro-font-family-mono` like
every other technical value in this project, both columns `min-width: 0`
with `overflow-wrap: anywhere` so a long name wraps rather than widening
the window ([[D-096]]). Neither column sets a colour of its own: the
status line is already coloured by its intent, and the line saying the
chain is broken is precisely the one that must not be quieter than the
heading above it. Every value goes in through `liroSetText`, never
`innerHTML` (SPEC §6.6) — these are file names.

The status element became a container, which had a side effect worth
recording: `settings.js` used to set `el.className` to the intent class
alone, throwing away `liro-fixed-region` — the one thing keeping the
status line from being squeezed off a growing form. It now composes.

**Verified.** `TestExportNamesBothFilesAndSaysWhatTheyAre` checks the
Go half in all three catalogues, including that one entry reads as a
sentence of its own, that a broken chain names entry 42 for a
`BrokenAt` of 41, that it does not read the same as an intact one, and
that the deleted key is gone from the catalogues rather than merely
unused. `TestExportConfirmationRendersBothFiles` posts a real payload
into both real windows in all three locales and reads the DOM back: the
folder, both file names and both sentences are on screen, the grid has
a non-zero rendered height and exactly four cells, neither window
scrolls horizontally, and a status that wrote no files shows no grid.
`TestExportConfirmationSaysPlainlyWhenTheChainIsBroken` adds that the
report's own line is not greyed down relative to the heading.

Photographed, from a real export of a real audit log through
`store.Export` — twenty entries, a real report file — rendered in the
real windows: `sr-Latn`, `sr-Cyrl` and `en`, intact and broken, in both
the audit log window and Settings. What no test and no screenshot can
do is press OK in the folder chooser, which is a native modal [[D-094]]
forbids simulating; that step remains the owner's, exactly as
[[D-097]] and [[D-133]] already recorded.

**Rejected.**
- **Putting the two sentences into the report JSON.** The file is
  machine-readable evidence, read years later by whoever is checking
  the log; a localised sentence in it would be neither.
- **A single pre-formatted line with padding spaces.** It aligns only in
  a monospace font at one width, and the two windows are different
  widths in three languages.
- **Stacking each detail under its file name.** Five lines instead of
  three for the ordinary case, to solve a wrapping problem the grid
  solves when it actually occurs.
- **Leaving the heading positive and colouring only the report's line
  when the chain is broken.** A person reads the first line; a green
  "exported to…" above a red line about alteration is a mixed signal on
  a screen that must not have one.

---
