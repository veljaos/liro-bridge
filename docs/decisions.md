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

> **Partly superseded by [[D-209]]**, which changes the label's wording,
> sets line 2 in a bold face, groups line 3's serial in fours and centres
> the block against the logo. What stands: four lines in this order, the
> name upper-cased from `givenName` + `surname`, only the label following
> the interface language, and the date not localised.

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

## D-136 — The page is drawn by a rasteriser written for this project; WebView2's own PDF viewer cannot be dragged over

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** `internal/pades/render` is a from-scratch PDF page
rasteriser: a content-stream interpreter, a signed-area scanline
rasteriser, colour spaces and PDF functions, image XObjects with their
masks, a TrueType glyph reader, and a CCITT Group 3/4 decoder. It draws
a page — and each annotation's normal appearance stream — into an
`image.RGBA` at a caller-chosen number of pixels per point. The
placement window shows those images.

**Why not WebView2's viewer.** F6b §2.1 rules it out and the reason is
structural rather than aesthetic: that viewer owns its own scrolling,
zoom and page navigation, and there is no way to put a rectangle over it
and read where the rectangle is in *page* coordinates. The window's
whole job is answering "where on this page is this stamp", so a viewer
that will not answer it is not a viewer this window can use.

**Why write one rather than embed one.** The alternatives were weighed
and are recorded below. The argument that settles it is not "we write
things ourselves here" — it is that this is the one hand-written layer
in this project where a bug cannot do harm. SPEC §12.1's rule exists
because a `/ByteRange` mistake produces a signature that looks valid and
is not. Nothing this renderer produces is written into a document,
compared against a golden file, or trusted by any validator. The worst a
bug here can do is draw a page wrongly, on screen, in front of the one
person who is looking at it. That is the whole risk, and it is visible
on sight.

**What it supports, measured against the documents this project has.**
All three real fixtures (`testdata/pdfs/local`) render correctly and
completely: justified Cyrillic body text, bold and italic serif
headings, page furniture, and — from their annotation appearance
streams — the signature stamps they already carry. Measured on this
machine: mup.pdf page 1 in 12.7 ms at 1.5 px/pt, page 5 (dense body
text) in 32.8 ms, and the same page at 5.33 px/pt (the 400 per cent zoom
step) in 112 ms into a 3175 × 4491 image. Opening a 200-page document
costs 0.8 ms; the whole document is parsed once and pages are drawn one
at a time.

What all three fixtures happen to contain is worth recording, because it
is what "supported" was aimed at: `/FontFile2` TrueType, both simple
(WinAnsi) and Type0/Identity-H with CIDFontType2 descendants; the
standard fonts named but not embedded; `FlateDecode` and nothing else;
no images, no shadings, no patterns. Those are the shapes a Serbian
business document takes. Images, CCITT-compressed scans, JPEG, colour
keys, soft masks on images, axial and radial shadings, Type 3 fonts and
inline images are implemented too, because a scanned contract is an
ordinary thing to be asked to sign and none of the fixtures is one.

**What it does not do, stated rather than discovered later.** CFF and
Type 1 font *programs* are not parsed — a font embedded in either is
substituted (D-137), keeping the document's own widths. JBIG2 and
JPEG 2000 images are not decoded and are skipped. `/SMask` in an
ExtGState (a soft mask on a whole group) is ignored, so a shape masked
that way draws at full strength. Tiling patterns become a flat mid grey.
Mesh shadings (types 4–7) become the middle of their own colour ramp.
Text rendering modes that stroke are filled. Each of these increments a
counted note on the result, which goes to the log; the one that means
the page is visibly incomplete — the operator budget running out — also
puts a line on the window saying so.

**The page shown is the /MediaBox, not the /CropBox.** A reader shows
the CropBox. This shows the MediaBox, because the MediaBox is the
coordinate system a stamp's position is measured in — `ClampToPageBox`,
`PlaceCorner` and `internal/placement` all work in it — and a preview
whose edges are not the edges of the coordinate space would put the
stamp somewhere other than where it was dropped on any document where
the two differ. They are the same box in every document this project has
looked at. A document whose CropBox is genuinely smaller will preview
larger than a reader shows it.

**Rejected.**
- **WebView2's built-in PDF viewer.** F6b §2.1's own reason, above.
- **PDFium through cgo** (`go-fitz`, `go-pdfium`'s cgo mode). Would
  render everything, and would end the "single binary, no C toolchain"
  property F0 §10's `CGO_ENABLED=0` trap exists to protect and phase 11
  is the first place to give up.
- **PDFium compiled to WebAssembly, run under a pure-Go runtime**
  (`go-pdfium`'s wazero mode). Genuinely pure Go, genuinely no cgo, and
  it would draw every document perfectly. Rejected because it puts a ten
  megabyte binary blob nobody in this project can read into a signing
  agent, and a WASM runtime to execute it. SPEC §8.6's rule about
  recording what a dependency does and why the standard library is
  insufficient is answerable here; "and what it is" is not.
- **Rendering only what the phase's own fixtures need and refusing the
  rest.** Considered as a way to make the job smaller. Rejected: the
  documents this project cannot draw are exactly the ones a person most
  needs to look at before signing — a scan, a form, something from a
  system nobody here has seen.

---

## D-137 — Shapes may be substituted; positions and widths never are

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** When a document does not embed a font program, or embeds
one in a form `internal/pades/render` does not read, the glyphs are
drawn from a substitute face this project embeds — but every metric
comes from the document: which codes a string decodes into, and the
advance width of each one, are read from `/Widths` or `/W` and nothing
else. Two substitutes are embedded, a sans and a serif, chosen by the
base font's own name; bold is synthesised by stroking the outline and
italic by shearing it.

Both are subsets of Noto (SIL Open Font License 1.1) generated by
`scripts/gensubsetfont --set preview`, whose character sets are recorded
beside them in `internal/pades/render/preview-sans-charset.txt` and
`preview-serif-charset.txt`: printable ASCII, Latin-1, Latin Extended-A,
Greek, Cyrillic, the punctuation a word processor emits without being
asked, currency, and a handful of mathematical and geometric signs. 579
glyphs each, 124 KB and 175 KB.

**Why substitution is safe here and would not be elsewhere.** The thing
this window measures is *where things are on the page*. A word drawn in
Noto Sans instead of Helvetica begins and ends exactly where the real
one does, because the position of every glyph is the sum of the
document's own advance widths; only the letterforms differ. That is the
one property that has to hold, and it holds by construction. Getting the
width from the substitute font instead would break it a word at a time
across a line — which is why the widths are read from the document even
though the font program sitting right there has its own.

**Measured, and the one case where the document does not say.** A
document that names a standard font and supplies no `/Widths` array at
all is entitled to: a reader is expected to know the fourteen standard
fonts' metrics. This project does not carry those tables. The fallback
is the substitute font's own advances, with Courier and its relatives
given a flat six tenths of an em, and it is honest about being a few per
cent long on a line of Helvetica. It was found by looking: a synthetic
200-page fixture written without `/Widths` first rendered with every
glyph half an em wide, which is visibly wrong on sight and would have
been invisible to any test of the payload behind it.

**Two faces rather than one.** Serbian legal documents are set in a
serif face far more often than not, and a contract previewed in a
sans-serif when it will print as Times reads as a different document.
The choice is made from the base font's name rather than the
`/FontDescriptor`'s Serif flag, which real producers set wrongly often
enough that the name is the better evidence.

**Rejected.**
- **Drawing nothing for a font that cannot be read.** A page of blank
  lines where the text is, which is worse than the wrong letterforms in
  every way that matters here.
- **Embedding the standard fourteen fonts' metric tables.** Six tables
  of a couple of hundred entries each, to make a case right that no real
  producer emits. Recorded as the honest gap instead.
- **Embedding one face and shearing or emboldening it for everything.**
  That is what bold and italic already do; serif from sans is not a
  transformation.

---

## D-138 — The margin is 12 pt, superseding F6's 24, for every placement

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** `appearance.Margin` is 12. It was 24. The change applies
to every placement, not only to the new one: a corner chosen by name, an
explicit coordinate from the command line, and a position dragged in the
placement window all keep the same twelve points clear of every page
edge.

**Why.** F6b §2.5 states the value and the owner's reasoning: not too
large, because someone may genuinely want the stamp close to the edge —
but never flush against it, which the original Bridge did not allow
either. Twelve points is about four millimetres, past any printer's
unprintable border and any binding, and small enough that a stamp
deliberately tucked into a corner looks tucked in rather than floated.

**Why it applies to corners too, rather than only to dragged
positions.** The placement window snaps to the four corners, and the
positions it snaps to are the ones `appearance.PlaceCorner` computes.
Two margins would mean a stamp dragged to the bottom right landing
twelve points from where choosing "bottom-right" puts it — two ways of
asking for the same thing giving different answers, which is the kind of
disagreement this project has spent three phases removing rather than
adding. `TestCornersAgreeWithTheSignedDocument` compares the two
directly.

**What changes on disk.** Every corner-placed stamp moves twelve points
towards its corner. No golden file changes: `testdata/golden/minimal-signed-bb.pdf`
is an invisible signature with no stamp at all, and the stamp tests
assert against `Margin` rather than against 24. The catalogues' own
margin note said "24 pt" in all three languages and now says 12.

**Rejected.**
- **Twelve points for a dragged position and twenty-four for a corner.**
  Above: it makes the snap land somewhere other than the corner it
  claims to be.
- **Keeping twenty-four and letting the window clamp closer.** Then the
  window would let a person place a stamp the command line refuses to
  place, which is the same disagreement pointing the other way.

---

## D-139 — The position is held in points; zoom is anchored on the cursor and cannot move the stamp

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** The placement window holds the stamp's position as two
numbers in the page's own coordinates, in points. Screen pixels are
derived from it to draw and read back from a drag; they are never what
is kept. Zoom changes the scale used for that conversion and nothing
else.

Zooming is anchored on the pointer, not on the page's centre: the
content point under the cursor is computed before the scale changes and
the viewport is scrolled afterwards so that the same content point is
under the cursor again. Zooming with the buttons anchors on the centre
of the visible area, which is where a person clicking a button is
looking.

**Why the anchoring.** F6b §2.3 asks for it and gives the case:
zooming in to read fine print near a signature line has to keep that
line under the pointer, or the thing being aimed at leaves the window at
the moment of aiming. Centre-anchored zoom is what makes a viewer feel
like it is fighting back.

**Why points rather than pixels, and how it is proven.** A position kept
in pixels has to be recomputed on every zoom change, and every
recomputation is a rounding. Kept in points, there is nothing for zoom
to round. That is easy to claim and easy to get wrong, so it is checked
two ways rather than asserted:

- `TestZoomNeverMovesTheStamp` (`internal/placement`) takes a position,
  draws it at one zoom, reads it back from the drawn rectangle, draws it
  at the next, and so on through every step in both directions, for
  three page shapes and all four rotations — which is what the window
  actually does — and requires the position afterwards to be the one it
  started at.
- `TestZoomNeverMovesTheStampInTheWindow` (`cmd/liro-bridge`) does the
  same through the real window, the real page and the real zoom buttons,
  reading the position out of the page itself.

Holding the position in a variable and never touching it would have
passed neither: both go through the display and back.

**The other half: the window never resizes.** F6b §2.3 is explicit and
`TestZoomChangesTheImageAndNotTheWindow` pins it — the canvas changes
width between 100 and 200 per cent and the window's own client area does
not.

**Rejected.**
- **Anchoring on the page centre.** Simpler, and wrong for the one thing
  zoom is for here.
- **Scaling the already-rendered image with CSS instead of re-rendering
  at the new zoom.** Instant, and blurry at exactly the moment sharpness
  is the point: someone zooms to 400 per cent to see where a printed
  line is.

---

## D-140 — One remembered position, not a named list of them

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** The configuration remembers exactly one placement —
`stampPlacedPage`, `stampX`, `stampY`, in use when `stampPosition` is
`"custom"` — and `internal/placement.Saved` is that one position. There
is no list, no name, and no picker.

**Why.** F6b §3 leaves the choice open and asks for the reasoning. The
need it describes is specific: the owner signs documents of the same
shape a thousand times and wants the stamp under the same printed
initials every time. One position answers that completely.

A named list answers a different need — several document shapes, each
with its own place — that nobody has asked for, and it brings three
things with it that the window is better without: a naming step at the
moment of placing, a picker at the moment of signing, and a
wrong-preset failure mode where a batch is stamped in the place meant
for a different kind of document. The last is the one that decides it:
this window's whole value is that it asks one thing.

It is also the easy direction to grow in. A list of these is a list of
these; the position type, the fitting, the clamping and the reporting
are all per-position already and would not change.

**What one position has to survive, and does.** A remembered position is
reused across documents that are not all the same length or the same
size, so it is fitted to each one rather than applied blindly: a page
number past the end falls back to that document's last page, and a
position off the edge of a smaller page is brought inside its margin.
Both are reported afterwards — `pades.Result.StampPageFellBack` and
`StampMoved`, counted into `jobs.Report.StampAdjusted`, shown on the
report screen and printed by the command line — because a stamp
somewhere other than where it was put is a surprise if nothing says so.

**Rejected.**
- **Named positions.** Above.
- **One position per document shape, matched automatically by page
  size.** A guess dressed as a convenience, and F6b's own instruction
  about not guessing at occupied corners applies with more force here:
  it would be wrong silently.
- **Remembering nothing, and asking every time.** That is the corner
  selector this window replaces, with more work.

---

## D-141 — Page images are files on a second virtual host, allowed cross-origin, deleted with the window

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** `ui.Options` gains `ScratchHost` and `ScratchDir`: a second
virtual host mapped onto a directory the caller owns. The placement
window creates a temporary directory under the agent's own per-user
configuration directory, writes each rendered page into it as a PNG, and
hands the page a URL. The directory is removed when the window closes.

The mapping is made with `COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND_ALLOW`,
unlike the asset host, which stays `DENY`.

**Why files rather than payloads.** An A4 page at the 400 per cent zoom
step is 3175 × 4491 pixels. Handing that to the page as a data URI means
base64 through `ExecuteScript` — several megabytes of UTF-16 string per
page change, on the same call this project has already had to make
reliable twice (D-099, D-101). As a file, the browser fetches it
asynchronously, decodes it off the UI thread and caches it, and a page
change costs one short JSON payload naming a URL.

**Why the second host, and why ALLOW.** The asset host is
content-addressed and shared between every window in the process, which
is exactly wrong for files that change while one window is looking at
them. A second host is the smallest thing that separates them.

`ALLOW` was not a preference: it was measured. With `DENY` the window
came up with its page and its stamp both showing a broken-image icon and
nothing else wrong. `DENY` refuses access from other origins, and a page
served from `liro.local` loading an image from `liro.pages` is another
origin. "Other origins" inside one WebView2 instance means the agent's
own page and nothing else; the only two hosts that resolve at all in
that instance are the two mapped there, and neither is reachable from
outside it.

**Why the directory is removed.** The images are pictures of the
document being signed. Nothing in SPEC forbids a temporary file on the
user's own machine — the WebView2 loader and the UI assets are already
extracted the same way — but a rendered page of somebody's contract is a
different kind of thing from a stylesheet, and it has no reason to
outlive the window that needed it. `placeUI.close` removes the directory
on every path, including the ones that end in an error.

**Rejected.**
- **`WebResourceRequested`, serving the image from memory.** The clean
  answer, and the one that touches no disk at all: three more COM
  interfaces implemented by hand, plus an `IStream`, on the layer whose
  hand-written vtables have already cost this project two crash
  investigations. Worth doing if the disk ever becomes a problem;
  not worth doing first.
- **Data URIs.** Above.
- **Mapping the scratch directory as the asset host.** One host cannot
  be two folders, and the asset host is shared.

---

## D-142 — The placement window keeps the three-message surface: every click reports what it meant

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** The placement window sends `approve` and `cancel` and
nothing else, like every other window in this project. A page change and
a zoom change are requests for Go to draw something — which is a message
the three-type surface has no room for — so they send `approve`, and Go
reads `window.__liroPlacementRequest()` back through `Eval` to find out
what the click meant: `{action: "render"|"done", page, scale, x, y}`.
Step 3's window does the same for its three buttons (`save`, `place`,
`reset`).

**Why.** F5 §2.4 makes the three-type surface a property of the host
rather than of one window, and D-083 recorded it as such. D-095 then
established the way out when a window genuinely has more than two things
to say: the page reports what the click meant as explicit data, read
back through the channel `ExecuteScript` already provides, rather than
Go inferring it from which state the window was in. This is that
pattern, applied to a window that asks for a page rather than to one
that answers a question about a timestamp.

The alternative — a fourth message type carrying an arbitrary payload —
is exactly what the rule exists to prevent: once one window can send
structured data, the "three things, deliberately small" property stops
being true for the whole program, for the convenience of one window that
had another way.

**What it costs, and why that is acceptable here.** One extra round trip
per page or zoom change, and a page that debounces its own requests by
ninety milliseconds so a person dragging the zoom does not queue a
dozen. A drag costs nothing at all: the stamp moves inside the page,
with no Go involvement, which is also what F6b §6 asks for.

**Rejected.**
- **A fourth message type.** Above.
- **Polling the page from Go on a timer.** Keeps the surface at three
  and turns an idle window into twenty `ExecuteScript` calls a second on
  the layer this project has twice had to make reliable.

---

## D-143 — Explicit coordinates now respect /Rotate, and a page past the end is clamped rather than refused

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** Two fixes in `internal/pades`, both found by building the
placement window on top of the explicit-coordinate path that existed
since F4 and had only ever been reached by a command-line flag.

*(a) A stamp placed by coordinate on a rotated page is drawn upright.*
`appearance.Render`'s `UseXY` branch used the identity matrix and the
unswapped footprint. `PlaceCorner` had done the counter-rotation
correctly since F4 (D-060) — the coordinate path simply never got it. On
a `/Rotate 90` page the stamp was therefore drawn on its side, inside a
rectangle of the wrong aspect. It now takes the same
`RotationMatrix(rotate)` and the same width/height swap as a corner
placement does. `TestPlacedPositionOnARotatedPageIsDrawnUpright` covers
all four rotations.

*(b) A page number past the end of a document is clamped to its last
page.* `config.Config.StampPage`'s own doc comment has said since F6
that "a number past the end of a document is clamped to its last page
rather than refused". `applyStamp` did not do that: `pdf.FindPage`
returned an error and the whole document failed. It now clamps and
records `stampAdjustment.pageFellBack`, which reaches
`pades.Result.StampPageFellBack` and from there the batch report.

**Why they were invisible until now.** `--stamp-xy` and `--stamp-page N`
are command-line flags a person passes deliberately for one document
they are looking at; the rotated case and the past-the-end case both
need a *saved* position reused across documents nobody chose it for,
which is what F6b §3 introduces. The comment in (b) was written for a
behaviour that was never implemented, which is the more instructive
half: a doc comment describing what the code should do reads exactly
like one describing what it does.

**Rejected.**
- **Refusing a page past the end, and making the batch report it as a
  failed document.** It is not a failure — it is a position being reused
  across documents of different lengths, which is the whole point of
  remembering one. Failing would turn a placement that is right for most
  of a batch into an error for the rest of it.
- **Fixing (a) only in the placement window's own path, leaving
  `--stamp-xy` as it was.** Two answers to the same question again.

---

## D-144 — The CCITT decoder is checked against an encoder this project did not write

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** `internal/pades/render/ccitt.go` implements ITU-T T.4 and
T.6 (Group 3 one- and two-dimensional, Group 4) with the code tables
written out. It is tested against a Group 4 and a Group 3 stream
produced by Pillow 12.2.0, compared pixel by pixel against the bitmap
that encoder was given.

**Why the independence matters here specifically.** These tables are
about two hundred numbers, and a single transposed bit produces a
decoder that works on most images and garbles some. A test built from
this decoder's own output would agree with any such mistake. This is F3
§8's rule for the CMS verifier — "shared helpers between signer and
verifier defeat the purpose" — applied to a codec.

**What the fixture is, and one thing it taught.** A 137 × 71 image,
deliberately not a multiple of eight wide so the ragged end of every row
is exercised, containing filled rectangles, horizontal and diagonal
rules, an ellipse outline and a row of single black pixels: long runs,
short runs, and runs of one. The first version decoded perfectly and
came out inverted, which turned out to be Pillow writing its Group 4
TIFF with the opposite polarity from the fax convention
(`PHOTOMETRIC_MINISBLACK`). The generator inverts the image before
encoding so the *stream* follows the convention PDF's `CCITTFaxDecode`
defaults to; a separate test then drives `/BlackIs1` and requires the
output to be the exact complement.

**Rejected.**
- **Skipping CCITT and letting scanned documents preview blank.** A
  scanned contract is an ordinary thing to be asked to sign, and it is
  precisely the document where a person needs to see the page before
  placing a stamp on it.
- **Testing the decoder against a stream this project encoded.** Above.

## D-145 — A window closed before its first payload landed is a cancellation, not a failure

**Date:** 2026-09-06
**Phase:** F6b

**Decision.** `ui.ErrWindowClosed` is a new exported sentinel, returned
by `Window.PostJSON` and `Window.Eval` when the window has already
closed. `runSettingsWindow` treats it, on its own first `PostJSON`, as
the cancellation it is and returns nil.

**Why, and how it surfaced.** `ui.NewWindow` does not return until the
page has finished loading, which is about two seconds; the window's
title bar exists from the moment it is created. A person who closes the
window in that gap — or a test that finds it by title and closes it,
which is what `TestSettingsWindowOpensInTheLanguageOnDisk` does —
produces a window that is gone before the first payload can be posted.
The settings window then reported "ui: window is closed" as a *failure*
from a function whose every other cancellation path returns nil.

It surfaced as a deterministic test failure once this phase added a
seventh long-lived window to `cmd/liro-bridge`'s test package and moved
the timings: the test passed alone and failed in the package, every
time. Widening the test's timing would have hidden a real behaviour, so
the behaviour is what changed. Closing a window is how a person cancels
every other window in this program (F5 §2.3, `Options.OnClosed`);
closing it a second earlier means the same thing.

**Rejected.**
- **Making the test wait longer before closing the window.** It would
  go green and leave a window that reports a person's cancellation as
  an error in the log.
- **Returning nil for every `PostJSON` failure.** A window that is open
  and will not take a payload is a real fault and should still say so.
  The sentinel is what separates the two.

---

## D-146 — Step 3 is one choice with three outcomes; the checkbox is gone and the method a person used last is what the configuration already says

**Date:** 2026-09-06
**Phase:** F6b — step 3 rework

**Decision.** Step 3 is a single screen, *Metod potpisivanja*, offering
three options one under another, each a button-sized target, in this
order:

```
Potpiši birajući poziciju potpisa    → the F6b placement window, on the
                                       batch's first document
Potpiši sa definisanim pozicijama    → the four corners, revealed inline
                                       beneath this option
Potpiši bez vizuelnog prikaza        → nothing is drawn on the page
```

Each option reveals the rest of its own answer under itself and only
while it is chosen: the remembered position for the first, the four
corners for the second, and a sentence saying that nothing is drawn
anywhere for the third. Nothing is on a second screen.

The on/off checkbox is removed outright. It is what made this three
questions instead of one — a control whose only job was to change which
other controls existed, which is exactly the nesting the three options
replace. "Visible or invisible" is no longer asked at all: it is the
difference between the third option and the other two.

**How a returning user's method is preselected, and why it needs no new
setting.** The method is not a fourth thing on disk. It is read from
the two fields that were already there (`stampMethodOf`):

| Configuration | Method |
|---|---|
| `visibleStamp: false` | nothing shown |
| `stampPosition: "custom"` with a position actually placed | choosing the position |
| anything else | a set corner |

So the option a person used last time is the one already selected this
time, and confirming is one click. A fourth field would have been a
second answer to a question the first two already answer, free to drift
from them — the failure this project has recorded whenever one fact is
stored twice ([[D-020]], [[D-093]], [[D-134]]). Writing back is the same
mapping in reverse, with one guard: choosing the first option when
nothing has ever been placed leaves the corner underneath it alone
rather than writing a position nobody has chosen, which
`config.validate` would replace on the next load anyway.

Choosing "nothing shown" likewise leaves the corner alone. Turning the
stamp back on afterwards must not have lost the corner that was picked
for it.

**The first option opens the placement window as part of the same
click.** Pressing *Potpiši* with it chosen opens the placement window on
the batch's first document, at whatever position is remembered; using
that position signs, cancelling it comes back to this screen and signs
nothing. The alternative — place, return here, press Sign again — is the
errand F6b's own Place button already was, and the option's words
("sign, choosing where the signature goes") promise one act, not two.
A document the renderer cannot draw falls back exactly as F6b §5 says:
the window comes back showing the corners, with the sentence saying
why, and the remembered position is left untouched on disk. That is
why `buildStampInit` takes the chosen method as a parameter rather than
deriving it — the method being offered and the method the configuration
holds are genuinely different things in that one case, and a window
that re-derived it would offer the option that had just failed.

**Two clicks for a corner.** The four corners are four targets laid out
two by two, in the order they sit on a page — gore levo, gore desno,
dole levo, dole desno — rather than a dropdown. Choosing one and
confirming is two clicks; through a dropdown it was three. The grid
also means picking a corner is aiming rather than reading.

**The third option had to be unmistakable.** "No visible mark" and "a
mark somewhere I cannot see" are the same words to someone who has just
signed, and the second reading is precisely how F5's invisible default
was first reported ([[D-103]]): a correct signature the owner concluded
had never been applied. The option says *bez vizuelnog prikaza* and,
when chosen, adds a line saying that no stamp is drawn anywhere in the
document and the signature is still fully valid.

**Step 3 is still skippable, and the skip is unchanged.** A caller that
supplies its own answer (`consentRequest.stamp`) still sees no window at
all ([[D-124]]). The screen it now skips is a cleaner one.

**Settings: the same three options in the same words, not a second
vocabulary.** The instruction for this pass was that Settings "keeps its
own shape — it sets a default rather than making a choice for one batch
— but its wording should match". That is read here as its *role*: it
still saves rather than signs, and it still holds what a standing
preference owns and a batch does not — which page, the reference line,
the identity-document toggle, the margin note, and the two buttons that
change or give back a remembered position. What it does not keep is the
two dropdowns. Rendering the same one choice through a second, parallel
control is the thing this project has spent three phases removing rather
than adding ([[D-108]]'s three copies of one rule, [[D-124]]'s "two
answers to the same question", [[D-138]]'s two margins), and it would
mean every future change to the three methods had to be made twice and
kept agreeing. Raised here rather than done silently, per SPEC §0; if
the owner wants the dropdowns back in Settings specifically, that is a
one-file change to the page and nothing else.

**Sizes, measured in a real window in sr-Cyrl — the longest of the three
catalogues — in each role's own size, in all three methods.**

| Window | Was | Now | Form needs |
|---|---|---|---|
| Step 3 | 350 | **345** | 208, against 215 before |
| Settings' way in | 730 | **775** | 597, against 551 before |

Step 3 is five points shorter and its form seven points shorter, which
is honest rather than impressive: three button-sized targets are most of
the height, and they are the point of the screen — the saving is in what
the screen no longer asks, not in what it takes up. Settings grew,
because it now carries the same three options rather than two dropdowns;
775 is well inside what this project already ships (the settings window
itself is 880) and the layout test measures every method in both roles.

**Verified by looking at it, in a real window at each role's own size,
and by running the built binary.** Photographed with each of the three
options chosen (`PrintWindow`, so taking the picture does not take the
foreground from whoever is using the machine — [[D-122]]) and driven
only through `Eval` inside the page's own DOM ([[D-094]]); the harness
was created and deleted in the same session ([[D-100]]). The built
`liro-bridge.exe` was launched with a document, its window photographed,
and the process stopped by its own exact handle — never by image name.
What no test and no screenshot here can do is reach step 3 the way a
person does: that needs a card, a certificate and a click, and it is the
owner's acceptance, exactly as [[D-097]], [[D-133]] and [[D-135]] already
record for the folder chooser.

**Catalogue.** `stampwindow.method_placed`, `method_corners`,
`method_none` and `method_none_hint` are new in all three catalogues;
`stampwindow.title` becomes *Metod potpisivanja* / *Метод потписивања* /
*Signing method*. `stampwindow.mode_label`, `mode_visible`,
`mode_invisible` and `position_placed` are deleted from all three rather
than left unused, along with the Go that read them ([[D-132]]'s rule),
and `TestStepThreeIsTitledForWhatItAsksAndSaysNothingElse` asserts each
one is gone from the catalogues rather than merely unreferenced.

**Rejected.**
- **A `stampMethod` field in `config.Config`.** The obvious way to
  remember the method, and unnecessary: the two fields already there say
  it exactly, and a third that must agree with them is a third that can
  disagree.
- **Keeping the checkbox and only re-ordering what it revealed.** It is
  the nesting itself that was reported, not its order.
- **A dropdown for the four corners.** Three clicks where two will do,
  and it hides the choice until it is opened — on a screen whose whole
  job is showing what the choices are.
- **Keeping Settings' two dropdowns.** Above.
- **Making the first option sign with the remembered position without
  reopening the placement window.** It would make one option mean two
  different things depending on whether anything had been placed
  before, and the option's own words say what it does.
- **Explaining the third option permanently, under the option, whether
  or not it is chosen.** It would make one of the three targets taller
  than the other two for a sentence that only matters once that option
  is being considered.

---

## D-147 — Finish is the completion screen's primary action, and it closes the window

**Date:** 2026-09-06
**Phase:** F6b — step 3 rework (second task)

**Decision.** When a batch finishes, *Završi* / *Заврши* / *Finish* is
the primary action, in brand blue, and it closes the window. *Potpiši
još dokumenata* is the neutral button beside it. The button that opens
the output folder is unchanged, and so is Save report.

`finish` is a recorded action like every other button on this page, not
a fourth page→Go message type ([[D-083]]): `handleAction` returns true
for it, which is the same path a closed window already took, so a run
still in flight is stopped between documents and nothing half-written is
left behind.

**Why.** Signing more was the primary action — the uncommon case dressed
as the expected one. Someone who has just signed a hundred documents is
finished; someone who wants more will look for it, and it is still one
click away.

**Two rows rather than four buttons on one line.** Measured, not
assumed: four buttons do not fit on one line at the main window's 560
points in any of the three languages, and left to wrap on their own they
put Finish alone on a line of its own in sr-Latn and in English while
pairing it with signing-more only in sr-Cyrl. So the pairing is decided
in the markup instead — what to do with the documents just written on
one row, what to do next on the row below — and the test asserts that
Finish and signing-more share a line, with Finish last, in all three
locales. Widening the window was not worth taking: the report is one of
three screens in a window sized for the other two.

**Verified.** `TestFinishIsThePrimaryActionOnTheReport` reads the
rendered screen in all three locales — the words, the weights, that the
two buttons resolve to different background colours rather than merely
carrying different classes, that they are on one line with Finish last,
and that all four actions are inside the window.
`TestFinishClosesTheWindow` covers the half a rendering test cannot see:
signing more clears the batch and stays open, Finish ends the loop.
Photographed in the real window in sr-Latn and sr-Cyrl.

**Rejected.**
- **Leaving signing more as a primary alongside a second primary.** Two
  blue buttons is no emphasis at all.
- **Making Finish close the whole agent.** It closes the window, which
  from the tray returns to the tray and from `liro-bridge open` ends the
  process — the same thing closing the window has always done.
- **Dropping Save report or shrinking the folder button to make one row
  fit.** The folder button stays as it is, and a quieter Save report is
  already as quiet as this project's weights go.

---

## D-148 — One window, whose content changes; the approval is a step of it and loses nothing by being one

**Date:** 2026-09-06
**Phase:** F6b — one window

**The complaint.** Signing one document opened six windows in sequence.
The owner counted them, and counted the buttons with them: *Potpiši,
Potpiši, Izaberi, Potpiši, Dalje, Potpiši.* Each window was defensible
alone — [[D-116]] put the approval in its own window precisely so there
would be one implementation of it, [[D-124]] gave the signing method its
own step, [[D-136]] gave the placement picker a window because it needs
the room — and together they were a maze: every step took the
foreground from the one before it, arrived somewhere else on the screen,
and left nothing to press but a button that opened the next window.

**Decision.** One window. Its content changes:

```
1  Documents     drop, browse, list, remove
2  Certificate   the list, and the approval
3  Method        three cards
4  Position      only for the first method
   -> Sign
```

Then the progress and the report, in the same window, as they already
were. The placement picker is the one thing that still opens
separately, and only because it has to show a page of the document at a
size worth dragging on; closing it comes back to step 4.

**How "content changes" is implemented: three pages, one window.** The
steps are three HTML pages the one window navigates between
(`ui.Window.Navigate`), not one page holding every screen's markup. The
alternative — folding all four steps into a single page — was rejected
for the reason [[D-116]] gives about the consent screen: it is the
product's only real gate, and a second copy of it, however carefully
merged, is a second thing that has to stay right. Keeping
`consent.html` as its own page keeps that copy count at one, keeps each
page's stylesheet and its own layout tests, and keeps the three files
readable. Two of the four steps (the method and the position) share
`stamp.html`, so moving between them costs no navigation at all.

**A navigation is not a second window, and the difference is measured.**

| | Before | After |
|---|---|---|
| Windows opened to sign one document | 4 (documents, approval, method, picker) | 1, plus the picker only when no position is remembered |
| Cost of moving a step | 2.06 s — a second WebView2 window | 16–34 ms — a navigation |

The 2.06 s was not WebView2 being slow. See [[D-150]]: the virtual host
was named `liro.local`, and `.local` is the multicast-DNS TLD, so every
page load paid a fixed 2 s of name resolution. Fixing that made both
numbers small; collapsing the windows is what made the *number* of them
small.

**How the certificate step keeps every property SPEC §6.5 requires.**
This is the part that was not allowed to soften, and each property is
asserted by
`TestApproveIsNotTheDefaultFocusAndNeedsADeliberatePress`:

- **The window is the agent's own.** It always was and still is; a
  caller supplies data, never markup, never a page, never a size.
- **Approve is not the initially focused control.** Cancel is —
  unchanged from F5 §5.6, and deliberately *not* the new Back button:
  the way out of a decision is refusing it, not stepping away from it.
- **Approve cannot be pressed until a certificate has been chosen**, and
  pressing it without one sends nothing to Go.
- **The approval is a press of a button, by a human.** Nothing about
  being step 2 of 4 makes it automatic, skippable or implied by the step
  before it: `next` from the documents step lands *on* the approval, and
  only `approve` leaves it.
- **A new batch is a new decision.** The page clears its selection on
  every arrival — including an arrival by pressing Back from the step
  after it, which is the case this now has that it did not before
  ([[D-121]]'s rule, in the one new place it can be broken).
- **What it shows is unchanged**: who is signing, how many documents,
  the application, and the fingerprint and file list behind Details
  (SPEC §6.6).
- **A request that carries its own answers still sees the approval and
  nothing else.** `stepsFor` returns exactly one step then, so there is
  no header, no Back, and nothing to press but Cancel and Approve —
  [[D-124]]'s "one window, one click", now literally one window.

**Back.** Available on every step but the first, and never while
signing. Someone who picked the wrong certificate goes back one step
instead of closing a window, re-opening it and re-enumerating a smart
card. It is not a decision and writes nothing to the audit log; Cancel
is a decision — a refusal — and still records `denied`, as it always
has. Escape stays Cancel rather than becoming Back: "dismiss" is what
Escape means everywhere else in this program, and a second meaning for
it on four screens is worse than one meaning on all of them.

**The step is visible as four small dots, not as words.** Both forms
were offered; the dots read more quietly, which is right for something
that is context rather than the subject of the screen. The words are
still there for anyone who needs them — the row's `aria-label` is
*Korak 2 od 4*, so a screen reader says it while the eye sees dots. The
current dot is larger as well as coloured, because colour alone is not a
distinction everyone can see (SPEC §10.3).

**The count of dots is the count of steps this run actually has**, which
is not fixed: a caller that brings its own documents has no first step,
one that brings its own method has no third or fourth, and the position
step exists only for the first method. The method screen therefore
carries both readings — three steps if a corner or nothing is chosen,
four if the position is — and the page picks between them as the radio
changes. It is rendering, not deciding: both readings came from Go with
the payload. Getting that pair the wrong way round showed four dots for
a three-step run, which is what
`TestTheMethodScreenCountsTheStepItsChoiceAdds` now prevents.

**The window resizes to its content, keeping its own centre.**

| Step | Size | Why |
|---|---|---|
| Documents | 560 × 690 | eight rows without scrolling, footer and actions always on screen ([[D-106]]) |
| Certificate | 520 × 760 | six certificate rows — a bookkeeper's machine, which SPEC §14.1 calls normal |
| Method | 440 × **380** | three button-sized targets and whatever the chosen one reveals |
| Position | 440 × 270 | one line, one button, and Sign |

The method step was 345 when it was a window of its own; the step
header costs it 35. That was found the hard way and is worth recording:
the layout test passed at 345 because it was posting a payload with no
header in it while the real screen had one, and the third option was
cut off on screen. The layout tests now post the header the real screen
carries, and the certificate step's do too.

Keeping the window's own centre rather than re-centring on a monitor is
what stops it walking across the screen: measured across the four steps,
the centre stayed at (960, 539) throughout. The result is clamped into
the monitor's work area so a taller step cannot push its own buttons
under the taskbar.

**The command line is the same window, entered one step in.**
`liro-bridge sign --interactive --in x.pdf` brought its own documents,
so it has no documents step; everything after that — the approval, the
method, the position, the timestamp question, the output paths, the
progress and the report — is the same code the tray's window runs. That
removed a whole second implementation of the batch loop, the progress
screen and the completion screen. The consent page's own progress,
done, timestamp, output-exists and failure screens went with it: the
three questions that are asked *after* the approval now live on the page
that carries the progress and the report, which is where the flow is by
the time they are asked.

**What one saving cost, honestly.** Clicks from an empty window to a
signed document are counted in the report; they are essentially
unchanged. What collapsed is windows, waiting and recovery: four
windows became one, a step costs 20 ms instead of two seconds, a wrong
certificate costs one press instead of three, and a remembered position
no longer reopens the picker at all — which is the case F6b §3 says the
owner is in a thousand times over.

**Also this pass, and smaller** (the owner's second task): the line
under the third method — *"Na dokumentu se ne crta nikakav pečat.
Potpis je i dalje potpuno važeći."* — is deleted, because the option's
own title says it, and it made one of three equal targets taller than
the other two for a sentence that only matters once that option is
being considered ([[D-146]] rejected doing this permanently and was
right; the fix was to delete it, not to reveal it). And *Sačuvano:
strana 1, x 371, y 79* is gone from under the first method: someone
choosing to place a stamp is about to place it, and coordinates from
last time are noise at the moment of deciding. They are said where they
mean something — on the position step, which is *about* the position,
and inside the picker, where the same line is also a button that puts
the stamp back on it.

**Rejected.**
- **One page holding all four steps.** Above: it would put a second
  implementation of the consent screen in the tree, which is the one
  thing [[D-116]] exists to prevent.
- **Making the method cards advance the flow by themselves**, saving one
  press per signature. A mis-click would then advance, and a returning
  user whose method is already chosen would have to click it anyway —
  no saving where it was wanted, and a new way to go somewhere by
  accident.
- **Escape as Back.** Above.
- **A `Korak 2 od 4` label instead of dots.** Louder than the thing it
  labels. It is the accessible name instead.
- **Signing straight from the picker when a position was just chosen**,
  saving one press the first time a position is set. The task asks for
  the picker to return to step 4, and it is right to: what was just
  dragged is worth seeing confirmed before it is signed with.
- **Keeping `sign --interactive` on its own path.** It is how two
  implementations of one gate come back.

**Verified.** Photographed in a real window at each step's own size
(`PrintWindow`, so taking the picture does not take the foreground —
[[D-122]]), and driven only through `Eval` inside the page's own DOM
([[D-094]]); the harness was created and deleted in the same session
([[D-100]]). The built `liro-bridge.exe` was launched with a document,
its window photographed, and the process stopped by its own exact PID —
never by image name. What no screenshot here can do is press the
buttons: that needs a card, a certificate and a hand, and it is the
owner's acceptance, exactly as [[D-097]], [[D-133]] and [[D-146]]
already record.

---

## D-149 — A certificate that is not for signing is not shown at all; F1 §6.1's rule is reversed

**Date:** 2026-09-06
**Phase:** F6b — one window

**Decision.** A listing shows only signing certificates. The rule lives
in one place, `classify.Info.HiddenByDefault`, and every surface that
lists certificates goes through it: `liro-bridge certs`, the
certificate step of the signing flow, and the Certificates window.
`certs --all` still shows everything, exactly as before.

**What was there before.** F1 §6.1 asked for a non-signing certificate
to be shown and disabled with its reason, and F5 §5.2 repeated it, on
this reasoning: *"Hiding them makes the user think the card is
broken."* Both are amended.

**Why that reasoning was wrong.** It was about a card, and the person is
looking at a list. Every Serbian card carries an authentication
certificate beside the signing one; on a Halcom card their Subject DN is
byte-for-byte identical (SPEC §11.5), and on a MUP card the name and the
issuer match too. So what the list actually showed was this:

```
ВЕЉКО СТАНОЈЕВИЋ
za prijavu — MUP Gradjani CA 4          DA534AC6
Ovaj sertifikat se ne može koristiti za potpisivanje.
```

— the person's own name, twice, the second time struck through under a
sentence about a distinction they have no vocabulary for. It is not a
choice. It cannot become a choice. And it makes every list twice as
long: two entries on a one-card machine, twelve on the bookkeeper's six
(SPEC §14.1).

**What is still shown, disabled, with its reason.** A *signing*
certificate that cannot be used right now: the card is out, or the
certificate has expired. That is a real choice temporarily unavailable,
and hiding *that* is what would actually make a card look broken —
which is the true form of the concern F1 was reaching for.
`TestHiddenKeepsASigningCertificateThatCannotBeUsedNow` is the guard.

**Nothing else is hidden.** The rule is exactly two shapes: a purpose
that is not signing, and the Windows-internal artefact [[D-108]] named
(self-signed, GUID subject, unknown to the Trusted List). A soft-token
certificate is never hidden whatever its purpose — SPEC §16.6 requires a
test key to be loudly visible wherever it appears.

**Rejected.**
- **Hiding it only when a signing certificate from the same card is
  present.** It sounds careful and is not: it makes the list depend on
  what else is installed, so the same certificate appears or does not
  depending on the machine, and the case it protects — a card carrying
  only an authentication certificate — is one where the answer is still
  "nothing here can sign", which `consent.no_usable_certificate`
  already says in words.
- **Keeping it and shortening its sentence.** The sentence was never the
  problem; the second row was.
- **A per-listing flag.** One rule, one place ([[D-108]]'s own lesson
  about three copies of one rule).

---

## D-150 — The virtual host was named `.local`, and that cost two seconds at every page load

**Date:** 2026-09-06
**Phase:** F6b — one window

**Found while measuring [[D-148]].** A navigation between two pages of
one window was taking 2.02 seconds — every time, to the millisecond,
whichever page, including navigating to the page already showing.
`Eval` immediately afterwards took 1 ms, so the page was ready; the
`NavigationCompleted` callback itself was firing two seconds after
`Navigate` returned. It was not the message pump and not the page.

**The cause.** The virtual host serving the embedded assets was named
`liro.local`. `.local` is reserved for multicast DNS (RFC 6762), and
Windows resolves such a name through mDNS/LLMNR before WebView2's
virtual-host mapping is consulted. Every page load paid that timeout.

**Decision.** The hosts are `liro.invalid` and
`preview.liro.invalid`. `.invalid` is reserved by RFC 2606 precisely for
names that must never resolve, which is exactly what a virtual host is,
and it can never be delegated to anybody.

**Measured, on the machine this was reported from:**

| | `liro.local` | `liro.invalid` |
|---|---|---|
| Opening a window (environment, controller, first page) | 2.30 s | 0.37–0.42 s |
| Navigating to another page | 2.02 s | 16–34 ms |
| The package's window test suite | 266 s | 52 s |

The window cost is the one this project has been quoting at itself since
F5 — "creating a WebView2 window is measured at a little over two
seconds", which is why the main window's Sign button had to say
*Otvaranje…* so it would not be pressed twice. Nearly all of it was
this.

**SPEC §10.2 carries the rule** so the name cannot drift back: a virtual
host name must not end in `.local`.

**Rejected.**
- **Leaving it and calling the two seconds WebView2's.** It was ours.
- **`liro.assets` or any other invented TLD.** Measured equally fast,
  but nothing stops `.assets` being delegated one day; `.invalid` cannot
  be.
- **Keeping `.local` and pre-warming a second window.** Hiding a
  two-second stall behind a thread, instead of removing it.

---

## D-151 — The position step is deleted; the first method opens the picker, and the picker takes the step's name

**Date:** 2026-09-06
**Phase:** F6b — one window, second fix pass (Tasks 1 and 2)

**Decision.** The signing flow is three steps, not four. `stepPosition`
is gone, along with the screen it drew (`state-position` in
`stamp.html`), its size constants, its payload fields (`screen`,
`canSign`), and the two readings the method screen carried because the
number of steps used to depend on which method was chosen (`stepAlt`,
`primaryLabelPlaced`). `stepsFor` no longer takes a method, because the
answer no longer varies with one; `headerFor`, `isLastStep` and
`primaryLabelFor` lost the same parameter.

Choosing *Potpiši birajući poziciju potpisa* and pressing **Potpiši**
opens the placement picker directly, on the batch's first document, with
the certificate just approved, at the remembered position if there is
one. Using a position from it signs. Closing it comes back to the method
step and signs nothing — the method screen is what the picker opened
over, and it is still there behind it. A document the renderer cannot
draw still falls back to the corners with the sentence saying why (F6b
§5, unchanged).

`placementOutcome` therefore answers *whether the batch can be signed*
(`sign bool`) rather than *which step to go to*, which is the shape the
question actually has now.

**The picker is renamed.** `place.title` was "Gde ide pečat" / "Где иде
печат" / "Where the stamp goes"; it is now **"Pozicija potpisa" /
"Позиција потписа" / "Signature position"** — the exact wording the
deleted step used, so a person who pressed Sign lands somewhere they
recognise as the answer to what they just chose. That is the same
decision, not a second one: the step's name moves to the thing the step
was a preface to.

**Why.** The step asked nothing. It said

```
Pozicija potpisa
Sačuvano: strana 1, x 209, y 364
```

and then, if nothing was remembered, opened the picker by itself. A
screen between a decision and its consequence, whose only content was a
fact the picker shows anyway — and shows better, since there the same
line is also the button that puts the stamp back on that position
(`place.reset_to_saved`, "Vrati na sačuvanu"). [[D-146]] built the three
methods as "one choice with three outcomes"; the fourth step made one of
those three outcomes arrive in two acts.

**Two catalogue keys die with it**, in all three locales rather than
being left for someone to wire back ([[D-132]]'s rule):
`stampwindow.position_step_title` (whose words moved to `place.title`)
and `stampwindow.placed_none` — the "nothing saved yet" line, which only
the deleted screen ever showed. `stampwindow.placed_at` stays, because
the picker still writes the position out; `placedSummary` folded into
`placedSummaryIfSet` beside its one remaining caller.

**What it costs and what it saves, counted rather than asserted.**
Presses from the method step to a signature, with the method already the
one the configuration holds — the returning user, which is the case
[[D-140]] says the owner is in a thousand times over:

| Method | Before | After |
|---|---|---|
| Choosing the position, one remembered | Dalje, Potpiši = **2** (picker not opened) | Potpiši, Koristi ovu poziciju = **2** |
| Choosing the position, nothing remembered | Dalje, [picker opens itself] Koristi ovu poziciju, Potpiši = **3** | Potpiši, Koristi ovu poziciju = **2** |
| A set corner | Potpiši = **1** | Potpiši = **1** |
| Nothing drawn | Potpiši = **1** | Potpiši = **1** |

Add one press to any row for choosing a different method card, and one
more for a different corner.

So the first row costs nothing and gains the page: the position is now
confirmed against the document being signed rather than against two
numbers. The second row loses a press. Nothing else moves. Recorded
this way because "fewer clicks" was not the point and would have been
the wrong thing to claim — what came out is a screen, and the screen is
what was reported.

**Verified by looking at it**, in a real window at the step's own size,
driven only through `Eval` inside the page's own DOM ([[D-094]]) and
photographed with `PrintWindow` so taking the picture does not take the
foreground ([[D-122]]); the harness was created and deleted in the same
session ([[D-100]]). The method screen shows three dots with the last
one current and **Potpiši** as the primary action for all three methods
— it said *Dalje* for the first one before. The picker opens titled
*Pozicija potpisa*, with the stamp already on the remembered position
and "Sačuvano: strana 1, x 371, y 79 — Vrati na sačuvanu" along its
foot. The built binary was run on a document and its window
photographed; the process was stopped by its own exact PID, never by
image name.

Tests: `TestTheFlowIsOneWindowWithThreeSteps` (three steps for all three
methods, and the picker is not one of them),
`TestEveryMethodEndsTheFlowOnTheSameWord` (three dots and *Potpiši* for
each method, read off the real DOM),
`TestTheMethodScreenSaysNothingAboutCoordinates` (the coordinates are
nowhere on that screen in either role, `state-position` is gone from the
page entirely, and the picker's own line names them),
`TestThePickerTitleIsThePositionOfTheSignature` (the new title in all
three catalogues, and the deleted key gone rather than unused),
`TestTheThreeEndingsOfThePicker` (chosen signs, cancelled signs nothing
and changes nothing, undrawable offers the corners).

**Rejected.**
- **Keeping the step and only removing the picker's automatic opening.**
  That leaves a screen whose whole content is two numbers, which is what
  was reported.
- **Signing straight from the picker with the remembered position, with
  no picker at all when one is remembered.** It would keep the one-press
  case at one press and make the option mean two different things
  depending on whether anything had been placed before — the objection
  [[D-146]] already recorded against exactly this shape.
- **Leaving the picker titled "Gde ide pečat".** The person pressed
  *Potpiši birajući poziciju potpisa*; the window that opens should
  answer in the same words, and the words the deleted step used are
  already those.

---

## D-152 — The progress screen covers the card session; a page that has just been navigated to shows nothing until it is told what to show

**Date:** 2026-09-06
**Phase:** F6b — one window, second fix pass (Task 3)

**Decision.** Three changes, one defect.

*(a)* `mainWindow.gotoPage` posts the page's strings and nothing else —
`{"type": "strings", "strings": …}`, handled by `main.js` as
`liroApplyStaticStrings()` with no screen chosen. It used to post
`filesPayload("init")`, which carries the strings *and renders the
document list*.

*(b)* Every screen in `main.html` starts hidden, `state-files` included.
A fresh document shows nothing until the step that navigated to it says
which screen it wants.

*(c)* `startSigning` posts the progress screen itself
(`postPreparingCard`) immediately after it reaches the main page, and
again after the timestamp and output questions, before the card session
is opened.

`bridge.js` now stores `payload.strings` from whichever payload carries
them rather than only from an `init`, since the payload that refills the
table after a navigation is no longer always the one that renders the
first screen.

**Why — what was actually happening.** Pressing *Potpiši* showed the
documents screen for about a second before the card was used. The cause
is (a) and only (a): `startSigning` navigates to the main page, that
navigation posted the document list's whole payload, and nothing
replaced it until `jobs.Runner`'s first progress hook — which is on the
far side of `openInteractiveSession`. So for the length of the card
being opened, the window showed step 1.

That is not cosmetic and this entry says so plainly, because the fix
looks small enough to be mistaken for one. SPEC §12.9 requires a
distinct "Preparing card…" state precisely because the measured ~4.9 s
of card initialisation reads as a hang otherwise; the window was instead
showing a screen that says nothing is happening at all, and saying it in
the shape of the flow having gone backwards. The state built to prevent
that confusion existed and was not on screen.

**Why the page starts blank rather than starting on the document list.**
Posting the progress screen immediately after the navigation would have
left the document list visible for the length of one `ExecuteScript`
round trip — a frame or two rather than a second, but the same defect
made smaller, and the sort of thing that grows back the first time
something slow is added between the two. A page that shows nothing until
Go names a screen cannot show the wrong one at all. What is visible in
that gap is the window's own background, which is what any page load
looks like and is not a step of anything.

SPEC §10.2 carries both halves as rules — no step visible on the way to
another, and the screen that covers a wait is the one that describes the
wait — so the next screen added to this window inherits them.

**Verified.** `TestTheDocumentsScreenIsNotShownOnTheWayToSigning` does
what `startSigning` does: navigates a real window from the stamp page to
the main page and asserts every one of the six screens has a computed
`display` of `none` afterwards, that the strings did arrive, and that
`postPreparingCard` then shows the queue with the catalogue's
"Priprema kartice…", the indeterminate bar, and no percentage. It was
confirmed to fail against each half of the fix reverted separately:
with `state-files` visible by default, and with `gotoPage` posting the
files payload again — both times reporting that state-files is on screen
with a computed display of "flex" after navigating, before any screen
was asked for.

Photographed too, in a real window at the flow's own size: the main page
immediately after the navigation (nothing), and immediately after
`postPreparingCard` ("Priprema kartice…", the document listed as
*čeka*, the indeterminate bar, *Zaustavi*). The built binary was run on
a document and its documents step photographed, which is the other half
of (b) — a page that starts hidden must still render the document list
when that is what is asked for.

**Rejected.**
- **Posting the progress screen straight after the navigation and
  leaving `state-files` visible by default.** Above: the same defect,
  one round trip long instead of one second, and ready to grow back.
- **Keeping `filesPayload("init")` in `gotoPage` and posting the
  progress screen over it.** Same objection, and it leaves the page's
  strings arriving in a payload that also decides a screen — which is
  what tied the two together in the first place.
- **Opening the card session before showing anything, and showing the
  progress screen when it returns.** That is the interval the screen
  exists to cover.

---

## D-153 — A test that writes to the machine puts back what it found, not the shape of what it found

**Date:** 2026-09-06
**Phase:** FTEST

**Decision.** `internal/platform`'s `keepAutostartValue` snapshots this
project's own `Run` value **verbatim**, plus whether it was present at
all, and restores exactly that — deleting the value again when there was
none. `TestWindowsAutostartRoundTrip` uses it.
`TestAutostartTestsPutBackWhatTheyFound` and
`TestAutostartTestsDoNotInventAnEntry` observe what a `t.Cleanup`
actually restores, through a nested `t.Run`, which is the only way to
see it.

**Why.** The round-trip test recorded a bool from `IsEnabled()` and
restored it with `SetEnabled(before, testExePath)`, where `testExePath`
is `C:\test\liro-bridge.exe` — a path deliberately chosen not to exist.
On any machine where autostart was on, one `go test ./...` therefore
rewrote the developer's real autostart entry to point at nothing, and the
agent silently stopped starting with Windows. Measured on the owner's
machine before and after the first suite run of this pass:

```
before:  LiroBridge = "C:\Users\Veljko\Desktop\liro-bridge\liro-bridge.exe"
after:   LiroBridge = "C:\test\liro-bridge.exe"
```

[[D-134]] recorded this as a known defect in a package that pass did not
touch, and left it. It is not a test-hygiene nicety: the value it
destroys is the one that makes the product start.

The general form, which is what this entry is for: **restoring the
*shape* of what was there is not restoring what was there.** A boolean
says whether an entry existed; it does not say what it pointed at, and
the pointing-at is the whole content.

**Rejected.**
- **Skipping the round-trip test on a machine with a real Run key.** It
  is the only test that exercises the real registry path the product
  uses, and the fix costs twenty lines.
- **Writing to a scratch key instead of the real one.** That is what the
  other tests in the file already do, and it is why they were never the
  problem — but the round-trip test exists specifically to prove
  `NewAutostart()`'s own default key path works.

---

## D-154 — The renderer counts a substituted font and a guessed width, because a note it does not make is a property it silently does not have

**Date:** 2026-09-06
**Phase:** FTEST

**Decision.** `internal/pades/render` emits two notes it did not:
`substituted the shapes of a font the document does not embed` and
`advance widths guessed: the document declares none`. Both fire once per
font per page, from `showText` rather than from `loadProgram`, so a font
declared in `/Resources` and never drawn reports nothing and the
guessed-width case — which only becomes known once a code has been
looked up — is reported the same way as the substituted-shape one.

**Why.** [[D-136]] states that everything the renderer cannot draw
faithfully "increments a counted note on the result", and [[D-137]] that
"every metric comes from the document". Measured over 380 documents and
2 472 pages, exactly four notes fired in the whole sweep — the mesh
shading, the tiling pattern, JBIG2 and JPEG 2000 — and **every document
drawn entirely in substituted glyphs reported an empty `Notes` map**,
including the ones whose advance widths the renderer had to invent. The
existing font note fires only when a program is present and unreadable,
which is the rarer half by a wide margin: the common case is a
standard-14 font named with no `/FontDescriptor` at all, which PDF
32000-1 §9.6.2.2 entitles a producer to write.

The two are kept separate because they are different facts. Substituted
shapes are harmless for the thing this renderer exists for — every word
still begins and ends where the real one does. Guessed widths are the one
case where [[D-137]]'s load-bearing property does not hold, and against
PDFium the page it produced correlated at 0.896 where every other text
page was at 0.96 or better.

**Rejected.**
- **One note covering both.** They call for different reactions: one is
  cosmetic, the other means the positions on the page are approximate,
  which is what a placement window is measuring.
- **Noting at load time.** A font can be declared and never drawn, and
  whether widths had to be guessed is not knowable until a code is
  looked up.

---

## D-155 — The command line's own I/O and parse failures carry codes, like the window's

**Date:** 2026-09-06
**Phase:** FTEST

**Decision.** `internal/cli.signOneFile` wraps `os.ReadFile` as
`INPUT_UNREADABLE` and `os.WriteFile` as `OUTPUT_WRITE_FAILED`; the seven
structural failures in `internal/pades/pdf` and `internal/pades` —
`/Root` not a dictionary, no `/Page` under a node, no `startxref` to
chain from — are wrapped as `PDF_INVALID` at the point the fact is known.
`sign.output_exists` is deliberately left as it is: it is already
localised and it names `--force`, which is what a person needs
([[D-104]]).

**Why.** Driven against the shipped binary over a corpus of broken
documents, `liro-bridge sign` printed the operating system's own English
in a Serbian interface — `read C:\…: Incorrect function.`,
`open C:\…: Access is denied.` — and the parser's own English for three
structural failures. `INPUT_UNREADABLE` ([[D-118]]),
`OUTPUT_WRITE_FAILED` ([[D-104]]) and `PDF_INVALID` all already existed,
with a sentence in all three catalogues, and the window path already used
them. Two front doors answering one question differently is what
[[D-108]], [[D-124]] and [[D-138]] each had to remove once already.

Wrapping in `internal/pades/pdf` rather than at the CLI boundary fixes
both doors at once: the window path maps an unclassified error to
`INTERNAL` — "an unexpected error occurred" — for a document that is
simply broken, which is the same defect [[D-104]] fixed one case over.

**Rejected.**
- **Mapping unclassified errors to a code at the CLI boundary.** It
  would fix the door that was measured and leave the other one, and it
  would have to guess which code an error deserves at a point that has
  lost the context to know.

---

## D-156 — A column's width comes from the catalogue, never from the format string

**Date:** 2026-09-06
**Phase:** FTEST

**Decision.** `internal/cli.certFieldFormatter` computes the certificate
listing's value column from the widest of the seven labels *in the locale
being rendered*, and pads with `%-*s`. The seven literal runs of spaces
that used to do it are gone.

**Why.** They were each the right length for the *English* label beside
them. In English every value started at column 13; measured in `sr-Latn`
the seven started at 12, 16, 16, 13, 12, 9 and 13 — on the one screen the
command line actually shows a signer, in the two languages almost all of
them read. Go's `fmt` pads `%s` by runes rather than bytes, so "Važi" and
"Vazi" occupy the same column and no width arithmetic of our own is
needed.

`TestCertificateFieldsLineUpInEveryLocale` asserts the property rather
than the numbers, in all three locales, so a catalogue change cannot
reintroduce it.

**The general form, which is why this is an entry rather than a
one-line fix.** Any layout constant written next to a translated string
is a layout constant that is right in one language. This project already
learned it for windows — [[D-106]]'s "widening is not a fix, it is a
delay" — and the command line had the same defect in the simplest
possible shape.

**Rejected.**
- **Padding to a fixed width large enough for every catalogue.** It
  makes the English listing needlessly wide to serve the longest Serbian
  label, and the next catalogue change breaks it again.

---

## D-157 — A UTF-8 byte-order mark is stripped from `config.json`, and nothing else is

**Date:** 2026-09-06
**Phase:** FTEST

**Decision.** `config.Load` strips a leading `EF BB BF` before
`json.Unmarshal`. Nothing else changes: plain text, UTF-16, a truncated
file and a lone BOM are all still rejected and still fall back to
defaults with a warning.

**Why.** `encoding/json` rejects a BOM at offset 1, so a `config.json`
carrying one was silently replaced by the defaults — the language, the
signature level, the remembered stamp position, the output folder, all
reverted, with only a line in a log file to say so. Every ordinary way of
editing that file on Windows writes one: PowerShell's `Set-Content
-Encoding utf8` does, and Notepad offers it in a dropdown. This was found
by doing exactly that while checking the three locales, and it cost a
confusing half-hour before the bytes were looked at.

[[D-134]] ran into it once and recorded it as "worth knowing on its
own". It is a defect against [[D-134]]'s own rule that the file is the
single authority on the configuration, and this project already strips
the same three bytes from the one other outside text file it reads (the
Trusted List seed, [[D-018]]/[[D-107]]).

**Rejected.**
- **Accepting UTF-16 too.** A BOM is transfer encoding on a document
  that is otherwise exactly what it claims to be; a UTF-16 file is a
  different encoding of the whole thing, and silently accepting it would
  be inventing a format nobody writes on purpose.

---

## D-158 — `errs.AllCodes` is proven complete by reading the source, because a list kept in step by hand is a check that quietly stops checking

**Date:** 2026-09-06
**Phase:** FTEST

**Decision.** `TestAllCodesListsEveryDeclaredCode` parses `errs.go`'s own
syntax tree and requires every `X Code = "…"` constant to appear in
`AllCodes()`, and nothing else to. `TestEveryCodeValueIsScreamingSnakeCase`
pins SPEC §7's naming rule and that no two constants share a value.

**Why.** `TestEveryErrorCodeHasAMessageInEveryCatalogue` — the check
[[D-104]] added so that no code can reach a user as its own key
("error.cert_revoked") — walks `AllCodes()`, and is exactly as strong as
that list is complete. Nothing checked that. A constant declared and
forgotten there compiles, passes every test in the repository, and
reaches a user as its key. The list has already had to be kept in step by
hand four times (`INPUT_UNREADABLE`, `OUTPUT_WRITE_FAILED` and the two
TSA client-certificate codes), and this pass added none — it was already
correct, which is exactly when a missing check is cheapest to add and
hardest to notice you need.

Reading the source rather than reflecting over values is [[D-025]]'s own
method for the "no PIN field" check, for the same reason: the property is
about what is *declared*, and a declaration nothing references is
invisible to anything but the syntax tree.

**Rejected.**
- **Counting the constants and comparing the count.** It catches an
  omission and not a substitution, and it says nothing about which one.

---

## D-159 — B-LT is a claim about what the document contains, not about what collection attempted

**Date:** 2026-09-06
**Phase:** FTEST

**Decision.** `dss.Apply` returns `Complete: false` whenever it writes no
revision at all. `internal/pades.applyDSS` is unchanged and therefore
leaves `AchievedLevel` at `LevelBT` with its existing note.

**Why.** Measured through the shipped binary:

```
liro-bridge sign --level b-lt --tsa https://freetsa.org/tsr
  Nivo: B-LT
66714 bytes   /DSS=False  /OCSPs=False  /CRLs=False  /VRI=False
```

B-LT is B-T plus a `/DSS` carrying revocation evidence (SPEC §12.6), so a
document with no `/DSS` has not reached it, and saying it has is the
overclaim SPEC §18.11 and [[D-047]] forbid outright.

`Apply`'s `case i+1 < len(certs)` only expects evidence for a certificate
whose issuer is also in the list, so a chain of **exactly one
certificate** expects none at all and comes out "complete" having
collected nothing. [[D-079]] then correctly skips the revision while
`applyDSS` raises the level on that same flag.

One certificate is not a corner case. MUP embeds only the signer
certificate in its CMS (SPEC §11.8), so a failed AIA fetch with nothing
matching in the bundled trust store leaves exactly one — and that is the
same outage that stops OCSP answering, so the two arrive together. It is
also the soft token's own shape, which is why every `--level b-lt` run in
this pass hit it.

**Why the existing test did not catch it, which is the part worth
keeping.** `TestSignDocumentBLTSkipsDSSWhenNoRevocationEvidence` asserts
exactly this property and passes, because `chainedSession`'s chain is two
certificates long — as is every other session in that package. The
single-certificate path had never been signed at all. A test that asserts
the right property against the wrong fixture is a test that will keep
passing.

**Rejected.**
- **Checking it in `applyDSS` instead.** `Apply` is the one place that
  already knows, from `ocspRefs`/`crlRefs`, whether there is anything to
  embed — which is [[D-079]]'s own reasoning for putting the skip there.
  Duplicating the condition one layer up is two places that must agree.

---

## D-160 — A batch that could not be recorded is said out loud; what the user is told about it is not decided here

**Date:** 2026-09-06
**Phase:** FTEST

**Decision.** `recordInteractiveAudit` logs, at error level, both a store
that could not be opened and an append that failed, carrying the outcome
and the document count and nothing else (SPEC §18.3). Refusing to append
onto a chain whose last entry cannot be read is unchanged, and correct.

**Why.** Both errors were discarded outright — a bare `return` and
`_, _ = store.Append(...)`. Measured by breaking the log deliberately: a
single unparseable line anywhere in it makes **every subsequent append
fail forever**, because the chain's last entry cannot be read and so the
next `PrevHash` cannot be computed. One truncated last line is exactly
what a power cut leaves. From that moment the agent goes on signing and
goes on not recording, with nothing anywhere — while SPEC §6.7 makes the
audit log the record of every signature.

**What is deliberately not decided here.** Whether the *person* should be
told, and what should happen if they are, is a product decision with at
least three defensible answers — refuse to sign until it is dealt with,
warn on the report screen and continue, or start a new chain file beside
the broken one and record the discontinuity. SPEC §6.7 does not say, so
this pass does not either; it is in `docs/ftest-report.md` as J-9 for the
owner. That the program should not lose the fact in silence needed no
such call.

**Rejected.**
- **Recovering by starting a fresh chain automatically.** It is probably
  the right answer and it is not this pass's to choose: a hash chain that
  silently restarts is a hash chain whose gap nobody was told about,
  which is the property the chain exists to prevent.

---

## D-161 — What this testing pass changed about what counts as evidence

**Date:** 2026-09-06
**Phase:** FTEST

**Decision, recorded because the pass turned on it.** Nine defects were
found. **Not one of them was found by a failing test.** Every one was
found by running the shipped binary or the real pipeline over real input
and then looking at what came out — and in four cases, by looking at
something other than the thing the program said:

| Found by | Defect |
|---|---|
| reading the registry before and after `go test ./...` | B-1, the autostart entry rewritten to a path that does not exist |
| counting the notes on 2 472 rendered pages and finding four | B-2, a substituted font never counted |
| reading the error messages, in Serbian, of 40 deliberately broken documents | B-3, the operating system's English in a Serbian interface |
| looking at a certificate listing in three languages instead of one | B-4, columns aligned in English only |
| writing `config.json` the ordinary Windows way | B-5, a byte-order mark silently discarding every setting |
| asking what the catalogue check is as strong as | B-6, a list kept in step by hand |
| noticing five files were all exactly 66 714 bytes | **B-7, B-LT claimed for a document with no `/DSS`** |
| counting handles across a thousand enumerations | B-8, a leak and a 855 ms probe |
| truncating an audit log's last line as a power cut would | B-9, every later signature unrecorded, in silence |

B-7 is the one worth keeping. The program said `Nivo: B-LT`. The
independent verifier said the signature was good. Every test in the
repository passed, including one asserting *exactly the property that was
broken* — it just used a two-certificate fixture, and the defect only
exists with one. What gave it away was a file size: five outputs at five
different levels, all exactly 66 714 bytes, when a `/DSS` revision has to
add bytes.

This is [[D-087]], [[D-122]] and [[D-128]] a fourth time, and the
sharpest statement of it so far: **a test that asserts the right property
against the wrong fixture will keep passing forever.** The fixture is
part of the assertion.

**What was done about it, beyond the nine fixes.** Every new test in this
pass was confirmed to fail against the old code, in both directions where
there are two. The layout tests now measure all three locales rather than
the one assumed to be longest — the assumption does not survive checking,
since "Sign, choosing where the signature goes" is longer than either
Serbian spelling. And `errs.AllCodes` is now proven complete by reading
the source ([[D-158]]), because that check was the one thing standing
between a new code and a user seeing `error.cert_revoked`.

**Rejected.**
- **Reporting the nine as ordinary bugs without this entry.** The
  pattern is the finding. Three previous phases recorded the same lesson
  about windows, about fonts and about settings; this pass says it about
  fixtures.

---

## D-162 — Revocation failures are remembered for the length of one batch; successes are not

**Date:** 2026-09-07
**Phase:** FTEST decisions — J-10

**Decision.** `dss.EndpointMemory` is one batch's record of revocation
endpoints that did not answer. `CollectRevocation` takes one, skips any
URL already in it, and adds a URL that produced no answer after every
attempt. `pades.Options.RevocationMemory` carries it; `internal/cli`'s
`RunSign` creates one before its loop and `cmd/liro-bridge`'s `runBatch`
one per run. Nil — the zero value, and what a single-document signature
passes — remembers nothing.

**A successful response is deliberately not cached across documents.**
[[D-046]]'s rule stands: revocation is collected after the signature
exists so that the response postdates it, and a response fetched after
document 1 predates document 100's signature. Whether that matters for a
qualified signature is the owner's call and he has not made it. Only
failures are remembered.

**The scope is one batch, never the process.** A new batch starts with
no assumptions, because a responder that was down five minutes ago may
be up now.

**Measured, on this machine, ten documents against an OCSP responder
that accepts the request and never answers:**

| | Before | After |
|---|---|---|
| Ten documents | **3m20s** (200.2 s) | **20.1 s** |
| Per document | 20.0 s each | 20.0 s, then 0 s x9 |
| Requests that reached the responder | 20 | 2 |

Twenty seconds is [[D-046]]'s own policy working — two attempts of ten
— and it is now paid once for the batch instead of once per document. A
hundred-document B-LT batch against MUP's responder, which [[D-076]]
measured as dropping connections (exactly this case), goes from about 33
minutes of timeouts to twenty seconds.

**What counts as "did not answer", precisely.** An endpoint that
returned *something* is not remembered, even when what it returned was
unusable: a responder that is plainly up is not the twenty-second cost
this exists to remove, and giving up on it after one document would turn
a transient server-side problem into a whole batch with no revocation
evidence. `TestAResponderThatAnswersBadlyIsStillAsked` pins that.

**Not remembered, and recorded rather than done: an artefact that was
fetched and then refused for being too large.** [[D-076]]'s size cap
discards a 30 MB CRL after downloading it, and that download repeats per
document. Remembering it would make documents 2..100 report "no
evidence" instead of "too large", which is the specific, honest reason
[[D-076]] built `Result.TooLarge` to carry. Remembering both the URL and
the skipped size would keep the message and remove the download; it is a
larger change than J-10 asked for, and it is noted here rather than made.

**Verified both ways.** The counting tests
(`TestASilentOCSPResponderIsContactedOnlyTwice` and its siblings) were
run against the code with `mem.remember` disabled and report "a silent
responder was contacted 20 times across ten documents, want 2"; with it,
2. The timing above is from a throwaway harness created and deleted in
the same session ([[D-100]]), not from a test that spends 200 s in the
suite.

**Rejected.**
- **Caching successful responses too, turning 33 minutes into 20
  seconds *and* saving the successful fetches.** That is the change
  [[D-046]] forbids without the owner deciding, and this pass was told
  to implement the smaller one only.
- **Remembering per certificate rather than per endpoint URL.** Two
  certificates from one issuer share a responder; keyed on the URL, the
  second one benefits from the first one's timeout. Keyed on the
  certificate it would not.
- **A process-wide memory.** It would turn one bad afternoon into a
  permanently degraded agent, and nothing would ever find out the
  responder came back.

---

## D-163 — The presence probe is answered once per certificate per listing; the cheaper question exists, is measured, and is not taken

**Date:** 2026-09-07
**Phase:** FTEST decisions — J-7

**Decision.** `internal/cli.Gather` builds a `presenceMemo` — a
thumbprint-keyed map of probe answers — and throws it away with the
listing. A certificate enumerated more than once in one listing is
probed once; a failed probe's conservative "not present" is remembered
too, so it is not paid for twice either. Nothing is cached across
listings, and nothing is probed concurrently.

**Why not across listings.** A card really can be inserted between one
`certs` and the next, and SPEC §11.10 exists because reporting a
certificate as available when it is not produces the worst outcome this
product has: the user clicks, enters a PIN, and fails five seconds later.

**Why not concurrently.** [[D-027]] rejected concurrent smart-card
access for signing — the card is a single serial device whose driver
queues requests anyway, and concurrent access is a known source of
driver-level failures. A probe goes through the same middleware and the
same card.

**Measured, on this machine, one card present.**

| | Before | After |
|---|---|---|
| This machine's real store (5 rows, 3 hardware-backed) | Gather 2.76 s, **3 probes**, 2.43 s of probing | **identical** |
| A six-row listing whose three hardware certificates are each enumerated twice | 3.58 s, **6 probes** | **1.97 s, 3 probes** |

Per probe, measured again here: **451–590 ms** when the card is present,
**873–1273 ms** when it is not. `liro-bridge certs` through the rebuilt
binary: 2.31 s, three runs, unchanged.

**The honest reading of the first row.** On a store where every
certificate is enumerated once — which is this machine, and which is
also SPEC §14.1's bookkeeper with six *distinct* certificates — the memo
saves nothing, because there is no repeat to remove. It costs nothing
and makes a repeat free, and that is the whole of what it does. The five
seconds a bookkeeper pays is not what this removes, and saying otherwise
would be the kind of claim this project's own FTEST pass exists to stop.

**The cheaper question exists, and here is what it is.**
`NCryptEnumKeys` on the "Microsoft Smart Card Key Storage Provider"
enumerates the key containers on *currently inserted* cards. Measured
directly, three runs, with one card in the reader and one certificate
whose card is absent:

```
NCryptEnumKeys: 2 keys in 913 ms / 921 ms / 915 ms
    "da552541b5504c418eb0ab4eb55eb0e6"   <- the present card
    "ab5fdde7ec79468281adbcf4feadd778"   <- the present card
                                         <- the absent card's container
                                            BD60C020...  is not listed
```

It answers the question exactly: the two containers it lists are the two
certificates the per-certificate probe reports present, and the absent
card's container is absent from the list. It costs **one call for the
whole machine**, about 0.9 s, *independent of how many certificates there
are* — against 2.43 s for three and about 5 s for the bookkeeper's six.
It also never opens a key, which is what J-7 asked about.

**It is not taken, and that is the owner's call to make, not this
pass's.** SPEC §11.10 states the mechanism — "determine it per
certificate by attempting to open that certificate's own key" — and
[[D-014]] and [[D-077]] are recorded decisions behind it. Switching to
container enumeration is a change to that rule, with its own questions:
it depends on every middleware registering through the Microsoft KSP
(all three Serbian issuers do, SPEC §11.11, but the PKCS#11 platforms in
F11+ will not), and a container name that matches is evidence about a
key, not about the certificate that names it. Recorded here, measured,
for the owner.

**Rejected.**
- **Caching the probe for a few seconds across listings.** Named in J-7
  as one of the three options and rejected outright: a card inserted in
  those seconds is invisible, and this is the one place where being
  wrong costs a PIN entry.
- **Skipping the probe for certificates the listing will hide anyway.**
  It would remove one of this machine's three probes (the MUP
  authentication twin, hidden by [[D-149]]) — but `certs --all` and
  `certs --json` show every row with its own state, so the answer is
  needed whether or not the default view shows it.

---

## D-164 — An input that already carries the output suffix is asked about in the window and skipped, with a count, on the command line

**Date:** 2026-09-07
**Phase:** FTEST decisions — J-3

**Decision.** `jobs.LooksLikeOutput(path, suffix)` is the one rule:
does this input's own name, before its extension, already end in the
configured output suffix. Both front doors ask it and neither guesses
what the answer means.

*The window* asks the person, once, for the whole batch:
`consent.StateAlreadySigned` says how many there are and offers **Skip
them and sign the rest** (primary — it loses nothing), **Sign them too**
(neutral), **Cancel** (quiet, and the initially focused control, like
every other question this window asks). The skipped documents are not
removed from the queue: they run through the batch as
`jobs.ErrSkipDocument`, so they show as *skipped* rather than *failed*
and the report says how many were left alone and why.

*The command line* reports the count and does the safe thing: inputs
whose names already end in the suffix are skipped unless `--resign` is
given, and either way a sentence names how many there are and what is
happening to them. If skipping leaves nothing to sign, that is said and
the exit code is 1.

**Why a question and not a rule.** The previous session declined to skip
these on the grounds that guessing intent could be wrong, and that
reasoning is right: counter-signing a `ugovor-signed.pdf` that arrived
from somebody else is an entirely ordinary thing to want, and nothing in
the file says which of the two this is. So nothing guesses. The window
asks; the command line names what is about to happen and requires a flag
to do the other thing.

**Why a dedicated flag rather than `--force`.** `--force` already means
"replace the file that is there". Overloading it would mean that
somebody who passes it to overwrite last week's outputs *also* silently
starts signing them again — which is precisely how
`ugovor-signed-signed.pdf` was reached in the first place. `--resign`
says the one thing it means.

**Measured in the shipped binary**, a folder holding `ugovor.pdf`,
`racun.pdf` and a `prethodni-signed.pdf` left by an earlier run:

```
run 1:  1 ulaz je vec potpisan dokument (ime se zavrsava na -signed) i preskocen je.
        Koristite --resign da bude potpisan.
        Potpisano 2/2 dokumenata
run 2:  3 ulaza su vec potpisani dokumenti ... i preskoceni su.
        izlazni fajl vec postoji; koristite --force za prepisivanje.  (x2)
        Potpisano 0/2 dokumenata
run 3:  identical to run 2
```

No `-signed-signed.pdf` at any point, where three runs used to produce
`-signed`, `-signed-signed` and `-signed-signed-signed`. `--force` alone
does not change that. `--resign` produces
`prethodni-signed-signed.pdf` and says so.

**The window path stays what it was.** A person who put a list together
deliberately still signs exactly that list; the addition is a sentence
and a choice that appears only when there is something to say, before
the card session opens — so cancelling has not cost a PIN entry
([[D-095]]'s and [[D-104]]'s own reasoning for where a question goes).

**Rejected.**
- **Skipping them silently in the window too.** The window's list was
  assembled by hand; dropping something from it without a word is worse
  than the doubled suffix.
- **Removing the skipped documents from the queue before the approval,
  so the consent screen counts only what will be signed.** It is the
  tidier count, and it puts a question in front of SPEC §6.5's gate for
  something that is not about consent. Signing a subset of what was
  approved is never a weakening; the report says which and why.
- **Comparing against the output path rather than the input's name.**
  That is [[D-104]]'s question ("does the output already exist"), which
  is separate and still asked. An input named like an output is a
  different fact, and it is the one J-3 is about.

---

## D-165 — The signed document is written beside its destination and renamed over it; a destination held open is refused, with its own code

**Date:** 2026-09-07
**Phase:** FTEST decisions — J-8

**Decision.** `platform.WriteFileAtomic` writes to a temporary file in
the destination's own directory, flushes it, closes it, and renames it
over the target. Both signing paths use it —
`internal/cli.signOneFile` and `cmd/liro-bridge.signInteractiveOne` —
in place of `os.WriteFile`. On any failure the temporary file is removed
and the destination is untouched. `errs.CodeOutputInUse`
(`OUTPUT_IN_USE`) is a new code with a message in all three catalogues.

**What it fixes.** `os.WriteFile` opens with `O_CREATE|O_TRUNC` and then
writes, so between those two moments the destination exists at the wrong
length. Measured with four pollers watching a destination while a 4 MB
file replaced an existing 300 KB one, the sizes another program observed
were 0 (x3 — neither the old file nor the new one), 4194304 and 307200.
A program watching the folder can pick up an empty or partial signed
document, and a write that fails partway destroys a previously good
signed file and leaves a truncated one.

**The temporary file is in the same directory** so the rename is within
one volume and therefore atomic. A temporary directory elsewhere would
make it a copy, which is the same half-written window again.

**The cost is accepted and is the point.** Reproduced here: with a
reader holding the destination open the way a C runtime's `fopen("rb")`
does — share read and write, not delete — `os.WriteFile` **replaced the
file**, and `os.Rename` refuses. Refusing is better than destroying, and
it is what careful tools do.

**Why a new code.** `OUTPUT_WRITE_FAILED` means "the disk would not take
it" and sends a person to look at the folder and the free space. A
destination somebody has open needs one thing and nothing else will do:
close it. The message says so, in all three catalogues. Measured in the
shipped binary:

```
liro-bridge: sign: ...\ugovor.pdf: Potpisani dokument nije mogao da zameni
postojeci fajl jer je taj fajl otvoren u drugom programu. Zatvorite ga i
pokusajte ponovo. (path=...\ugovor-signed.pdf)
exit code: 1
target after: 66714 bytes, SHA-256 unchanged
```

**One ambiguity had to be measured rather than assumed.** Windows
returns `ERROR_ACCESS_DENIED` for a rename onto a destination another
program holds *and* for a rename onto a read-only file — confirmed
directly with a throwaway program. The two need opposite answers, so
`isSharingViolation` asks which it is (`FILE_ATTRIBUTE_READONLY`) rather
than guessing; a read-only destination stays `OUTPUT_WRITE_FAILED`.
`ERROR_SHARING_VIOLATION`, `ERROR_LOCK_VIOLATION` and
`ERROR_USER_MAPPED_FILE` are unambiguous and need no such question. The
folder is never in question at that point: a temporary file was created,
written and closed in it moments earlier.

**Tests.** `TestWriteFileAtomicLeavesTheOriginalIntactWhenTheDestinationIsHeldOpen`
holds the destination open, asserts the original survives byte for byte,
that the temporary file is gone and that the code is `OUTPUT_IN_USE`;
run against `os.WriteFile` it reports "WriteFileAtomic replaced a
destination held open by another program".
`TestWriteFileAtomicNeverTruncatesTheDestination` watches the
destination throughout and requires every size a watcher sees to be
either the whole old file or the whole new one; against `os.WriteFile`
it reports "a watcher observed the destination at 0 bytes".
`TestWriteFileAtomicTellsAReadOnlyFileFromAHeldOneApart` pins the
ambiguity above. `TestOutputHeldOpenIsNamedAndTheOriginalSurvives`
covers the same through the real signing step.

**Rejected.**
- **Keeping `os.WriteFile` and writing an empty marker first**, or any
  other way of narrowing the window without closing it. The window is
  the defect.
- **Falling back to `os.WriteFile` when the rename is refused.** That is
  the destruction this change exists to prevent, restored under a
  condition nobody would notice.
- **Reporting a held-open destination as `OUTPUT_WRITE_FAILED` and
  putting the specifics in `Details`.** [[D-066]] and [[D-104]] rejected
  the same shortcut twice: a code whose message names the disk sends the
  person to look at the disk.

---

## D-166 — A chain that cannot be continued is left where it is and a new one is started beside it, recording the break; SPEC §6.7 amended

**Date:** 2026-09-07
**Phase:** FTEST decisions — J-9

**Decision.** A store holds one or more chains. When `Store.Append`
finds that the current chain cannot be continued — its last entry cannot
be read, because a line is not a whole entry or because a file cannot be
read at all — it leaves that chain's files exactly as they are and
writes into a new chain beside them, whose first entry carries a
`Discontinuity`: which file preceded it, at which sequence and line it
stopped, and why. SPEC §6.7 gains a subsection stating all of this,
because it did not say what happens when a chain cannot be continued.

**File naming.** Chain 1 keeps the names it always had
(`2026-09-001.jsonl`), so an existing audit directory reads exactly as
it did; later chains carry their number before the extension
(`2026-09-001.c2.jsonl`). Grouping is done by reading names, never by
moving files — the broken file must not be touched, and that includes
renaming it.

**The record is enumerated, not prose.** `Discontinuity` has named
fields and a `BreakReason` of `unparseable` or `unreachable`.
`audit.Entry`'s field set is a deliberate allow-list ([[D-084]]): a
free-text note is where a file name eventually ends up. The one file
name it does carry is this package's own generated `YYYY-MM-NNN.jsonl`,
which is produced from a date and a rotation number and cannot become a
document's name.

**It is part of what the entry hashes**, behind a one-byte presence
marker appended after `AchievedLevel`. A record of a break that could be
edited without breaking the chain it starts would be worth nothing. An
entry with neither optional field canonicalises to exactly the bytes it
always did, so a log written before either existed still verifies —
`TestTheDiscontinuityIsPartOfWhatIsHashed` pins all three properties.

**Told once.** Only the entry that opened the new chain carries the
record, so "not on every subsequent signature" is a property of the data
rather than a flag somebody has to clear. `recordInteractiveAudit`
returns it; the report screen shows one sentence in the caution family —
a notice, not an error — naming the file that broke, the file the log
continued in, and the folder. `TestTheNextSignatureAfterABreakSaysNothing`
is the guard.

**Reading became tolerant, and that is a behaviour change worth naming.**
`All` used to return an error for the whole store if any line anywhere
was unparseable, so a log with one truncated last line showed *nothing*
in the audit window and exported *nothing*. It now returns what
survives; the break is reported by `Chains`/`Verify` instead. A chain
that breaks at line 400 still has 399 entries worth keeping.

**Verification reports each chain separately**, because a new chain's
first entry has no `PrevHash` by construction and walking the store as
one sequence would report the discontinuity itself as tampering.
`StoreVerification` carries one `ChainVerification` per chain — files,
count, first and last timestamps, its own walk, its discontinuity, and
where reading stopped — plus a store-wide `BrokenAt` counted across the
exported file's own lines, so [[D-135]]'s "entry 42" message still
points at a line a person has in front of them. Tampering inside a chain
is still detected at exactly the entry that was altered
(`TestTamperingIsStillDetectedWithinAChain`).

**Export writes every chain**, and the exported log carries the
discontinuity records, so it is self-describing without the report
beside it. The confirmation line says how many chains there are, whether
each is intact, and when and why each break happened:

```
integrity check: the log could not be read to its end - 2 chains,
not all intact, breaks: break on 06.09.2026. (an entry could not be read)
```

A two-way ok/broken split produced "integrity check: passed - not all
intact" for that state, which is a contradiction on the one screen that
must not have one ([[D-135]]); "could not be read to its end" is a third
sentence for a third finding. The chain count takes Serbian's two plural
stems (`2 lanca`, `5 lanaca`, with the teens on the larger form), because
a machine-shaped plural on the screen that reports the integrity of an
audit log is not the impression to give.

**The unreachable case is the same shape.** A chain whose file cannot be
read at all — a permissions change, a network drive that has gone away —
starts a new chain whose first entry says `unreachable`. Refusing to
sign over a log is worse than recording that the log moved. What this
does *not* recover from is the whole audit directory being gone, which
`NewStore` fails on: in this product that directory is
`%LOCALAPPDATA%\Liro\audit` and is local by construction, and that path
still reports at error level and records nothing ([[D-160]]'s behaviour,
unchanged).

**Nothing ever overwrites, truncates or deletes a broken chain file.**
Every write is `O_APPEND|O_CREATE` into the *new* chain's own file.
`TestTheBrokenFileIsNeverTouchedByAnythingTheWindowDoes` reads, verifies,
exports and signs again, then compares the broken file byte for byte.

**Verified in the shipped binary**, against a scratch profile holding a
chain truncated mid-entry and then continued: the tray's audit-log
window lists both chains' entries newest-first and shows, on the entry
that opened chain 2, "Ovde pocinje novi lanac: 2026-09-001.jsonl nije
mogao da se nastavi (zapis nije mogao da se procita)." The window was
opened by posting the tray's own `WM_COMMAND` to its message-only window
— a window message, never synthetic input ([[D-094]]) — and photographed
with `PrintWindow` ([[D-122]]). The process was stopped by its own exact
PID.

**Rejected.**
- **Refusing to sign until the log is dealt with.** One of J-9's three
  options, and the one that stops a bookkeeper's afternoon over a file
  that is not the signature.
- **Warning and carrying on without recovering.** That is what [[D-160]]
  already did, and it leaves every later signature unrecorded.
- **Recovering by truncating the broken file back to its last whole
  line.** It would let the chain continue in place, and it destroys the
  evidence of exactly what happened — which is the one thing an
  append-only log is for.
- **A subdirectory per chain.** Cleaner to read, and it would mean
  moving the broken file, which is forbidden.
- **Restarting the sequence numbering from where the broken chain left
  off, so numbers never repeat across chains.** It suggests a
  continuation that does not exist. Each chain verifies on its own from
  sequence 0, and the discontinuity record is what ties them together.

---

## D-167 — The one defect in this pass was found by looking at the shipped window, and it was a class this project has a rule about

**Date:** 2026-09-07
**Phase:** FTEST decisions

**Decision, recorded because the pass turned on it.** Every test written
for these five tasks passed, in both build configurations, before the
binary was rebuilt and looked at. The audit-log window then showed this:

```
Ovde pocinje novi lanac: 2026-09-001.jsonl nije mogao da se nastavi (zap|
[------------------- horizontal scrollbar -------------------]
```

The break line ran off the right edge and the entry list had grown a
horizontal scrollbar. The cause was one class name: the sentence was
given `.liro-outcome`, which carries the colour — and, because it exists
for the one-word outcome badge on the right of a row, also
`white-space: nowrap` and `text-align: right`. The fix is to take the
colour class alone, plus `min-width: 0` on the flex items above it.

**Why no test caught it.** `assertPageDoesNotScroll` checks the page's
own scrollWidth and, for every other element, only whether it scrolls
*vertically*. The list is the window's designated scrolling region, so
its horizontal overflow was inside the one place nothing was looking.
`TestTheChainBreakLineWrapsRatherThanWideningTheWindow` now measures the
list's own `scrollWidth` against its `clientWidth`, the line's right edge
against the window's, and the computed `white-space`; run against the
`.liro-outcome` version it reports "the entry list scrolls sideways:
scrollWidth 601 exceeds clientWidth 413" in sr-Cyrl, the longest of the
three catalogues. `TestTheReportNoticeWrapsToo` does the same for the
report screen's own notice, which is the longest sentence either screen
carries.

**This is [[D-096]]'s rule for the third time** — a value with no length
bound wraps, it does not widen its container — and [[D-087]], [[D-122]],
[[D-128]] and [[D-161]] for the fifth: *a green suite is not evidence
about what a window shows.* The specific trap is worth naming on its
own, because it will catch the next person too: **`.liro-outcome` is a
badge class, not a colour class.** The colour lives in
`.liro-outcome-<intent>`; taking both gives a sentence the geometry of a
one-word label.

**Rejected.**
- **Widening the window.** [[D-106]]'s answer, unchanged: widening is
  not a fix, it is a delay.
- **Truncating the sentence with an ellipsis.** [[D-096]] rejected it
  for the fingerprint and [[D-106]] for a settings label; a sentence
  about the integrity of the audit log is not one to cut short.

## D-168 — A BER length is accumulated in a `uint64` and refused if it cannot be a positive `int`; both independent readers had the same defect

**Date:** 2026-09-07
**Phase:** FTEST Group 3 — fuzzing

**Decision.** `internal/pades/tsa.readTagAndLength` and
`internal/pades/verify.readDER` accumulate a long-form BER length into a
`uint64` and return an error when it exceeds `math.MaxInt`, instead of
shifting it into an `int` and comparing the result against a buffer
length.

**Why.** `FuzzParseResponse` found this in its second minute:

```
30 88 30 30 30 30 30 30 30 30
panic: runtime error: slice bounds out of range [:-8633347502144212944]
```

BER permits up to eight length octets and eight octets set the sign bit
of an `int`. Every check below the length compares it against a buffer
size — `if length > len(body)` — which a negative value passes, and the
slice that follows panics. The reader is fed by two things neither this
agent nor its user controls: an HTTP response from a timestamp
authority, and the unsigned attribute inside the CMS of a document
somebody else signed.

**The part worth keeping.** The identical defect was in
`internal/pades/verify`'s reader, which [[D-044]] made a second,
from-scratch implementation *specifically* so that "a bug in a shared
helper passes in both directions and the test proves nothing." Nothing
was shared and nothing was copied; two readings of the same RFC both
reached for `int` and neither thought about eight octets.

Independence protects against a bug being *propagated*. It does nothing
against a bug being *reinvented*, and a length field that overflows a
machine word is exactly the kind of thing two careful people get wrong
the same way. So the rule this adds is not architectural, it is a habit:
**when one of a pair of deliberately independent implementations falls
over, go and look at the other one before doing anything else.** That is
what found the second one, in about a minute, and no amount of
independence would have.

**Rejected.**
- **Fixing only the reader the fuzzer found.** The other one is reached
  from `VerifySignature` on every document this project verifies,
  including in CI, and it had the same panic waiting behind the same
  ten bytes.
- **Rejecting a length larger than the input rather than larger than an
  `int`.** Tighter, and it would have worked — but it changes which
  error message a merely-too-large length produces, for no gain: the
  existing "declared length exceeds available bytes" check already
  rejects those, correctly, one line later. Only the impossible needed a
  new answer.
- **Capping the octet count at four instead of eight.** It would make
  the overflow unreachable on any machine, and it would also refuse
  encodings BER permits. Refusing what the standard allows, to avoid
  thinking about a conversion, is the kind of narrowing that comes back
  as a real document this project cannot read.

---

## D-169 — A window owns the two icons it sets, and destroys them after the window is gone

**Date:** 2026-09-07
**Phase:** FTEST Group 3 — window lifetime

**Decision.** `setWindowIcons` returns the two `HICON`s it loaded; the
`window` struct keeps them; `wndProc`'s `WM_CLOSE` case destroys them
with `DestroyIcon` **after** `DestroyWindow` has returned.

**Why.** Measured, by opening each of the seven windows a hundred times
and reading the process's own GDI counter: **6.00 GDI objects and 2.10
USER objects per window, monotonically, never returned.**

`LoadImageW` with `LR_LOADFROMFILE` and without `LR_SHARED` creates a
new icon on every call and the caller owns it. `WM_SETICON` does not
take ownership — the window stores the handle and paints from it — and
`DestroyWindow` does not free it either. Measured directly, outside the
agent: one icon is 3 GDI objects and 1 USER object, two icons per
window, and `DestroyIcon` gives all of them back (600 of 604, 200 of
201).

A process's default GDI quota is 10 000, so this is roughly 1 600
windows before a window cannot be drawn — a long afternoon rather than
an immediate failure, which is why nothing noticed. The failure it
eventually produces is a window that will not open, with nothing in any
log to say why.

**After `DestroyWindow`, not before.** Until the window is destroyed it
is still using both icons to paint its own title bar and its Alt+Tab
entry; destroying an icon a live window holds is a window drawing from
freed memory. The teardown order is therefore: re-enable the owner,
close the WebView2 controller, revoke the drop targets, `DestroyWindow`,
*then* the icons.

**Why no test saw it.** Every test in `cmd/liro-bridge` borrows one
shared window per page and closes it once, at the end — which
[[D-098]]'s own note explains is necessary, because thirteen WebView2
environments in one process stopped completing at all. One window that
leaks is indistinguishable from one that does not. It takes a second
window to see a slope, and the suite was built to have exactly one.

This is [[D-161]]'s lesson on a different axis. There the fixture was
the wrong shape (a two-certificate chain for a one-certificate defect);
here the fixture is the right shape and there is only ever one of it.
**A per-instance cost is invisible to a test that makes one instance**,
however carefully that test asserts.

**Two tests, because the property has two halves.**
`internal/ui.TestAnIconLoadedForAWindowIsGivenBack` proves the primitive
frees what it allocates — fifty icon pairs, no window, milliseconds.
`cmd/liro-bridge.TestOpeningAndClosingWindowsDoesNotLeakGDIObjects`
opens ten real windows and proves one calls it: 6.00 GDI per window
against the old code, 0.00 against this one. Ten rather than a hundred
because the property is linear, and ten is four seconds — a price worth
paying on every run for a defect whose entire symptom is that nothing
ever noticed it.

`TestEveryWindowSurvivesAHundredOpenAndCloseCycles` is the measurement
that found it, kept behind `LIRO_WINDOW_CYCLES` because seven hundred
window creations is six minutes and does not belong in every
`go test ./...`.

**The tray's own icon is deliberately not changed.** It is loaded once
per process rather than once per window, so it is not a per-cycle cost,
and its fallback is `IDI_APPLICATION` — a shared system icon that must
never be destroyed. Adding a `DestroyIcon` there would replace a
non-existent leak with a real bug.

**Rejected.**
- **`LR_SHARED` instead of destroying.** It applies only to images
  loaded from a module's own resources, not from a file, so it would
  silently do nothing here — the worst kind of fix, one that looks
  right and changes nothing.
- **Setting the icons on the window class instead.** [[D-098]] already
  rejected that, and its reason is unchanged: the class is registered
  once per process before any window's DPI is known, so both sizes
  would be resolved at whatever the first monitor's scaling happened to
  be.
- **Destroying the icons in `closeWebView`.** It runs before
  `DestroyWindow`, which is exactly the window of time in which the
  window is still painting from them.

---

## D-170 — The window layer's remaining leak is a process handle per WebView2 environment; recorded against J-6 rather than fixed

**Date:** 2026-09-07
**Phase:** FTEST Group 3 — window lifetime

**Decision.** Recorded, measured, not fixed. `ui.NewWindow` leaks
**about one kernel handle per window**, two thirds of them handles to
`msedgewebview2.exe` processes that have already exited. The change that
closes it is the one J-6 already names — one WebView2 environment per
process rather than one per window — and that is a change to this
layer's lifetime model, not a bounded fix.

**Why it is a leak and not a lag, measured.** A hundred windows, then
two minutes of doing nothing at all, sampled every five seconds: +127
handles over baseline immediately, +126 at thirty seconds, +126 at
sixty, +128 at a hundred and twenty. Nothing comes back.

**What kind of handle, measured.** This process's own handle table
snapshotted before and after forty windows, via
`NtQuerySystemInformation` with `SystemExtendedHandleInformation` — so
nothing has to call `NtQueryObject` and risk hanging on a synchronous
pipe — with each object type index resolved by creating one object of
that kind and reading the index it got, rather than from a table that
would be wrong on the next Windows build:

```
40 windows: 280 -> 319 handles (+39, 0.97 per window)

Process        27 appeared,  0 of the old ones closed,  net +27  (0.68/window)
Event          15 appeared,  9 closed,                  net  +6  (0.15/window)
type index 26  12 appeared,  8 closed,                  net  +4  (0.10/window)
Thread          5 appeared,  5 closed,                  net   0
```

This project never calls `CreateProcess` or `OpenProcess`. The handles
are created inside WebView2, in this process's address space, and
releasing `ICoreWebView2Environment` does not close them. A handle to a
dead process keeps its process object alive; the processes themselves
do exit, which was checked separately — after roughly nine hundred
window creations the machine held thirteen `msedgewebview2.exe`
processes, every one of them started an hour before this work began.

**Why it is not fixed here.** Each `ui.NewWindow` builds its own
WebView2 environment, and an environment is what starts a browser
process group. [[D-099]] noted one-environment-per-process as the likely
shape of a fix and was wrong about what it would fix; [[D-131]] named it
as "what would actually make it faster, recorded rather than done"; J-6
records it as still the right next change and still bigger than a
bounded pass. Three passes have declined it for the same reason, and an
unattended pass with nobody to accept a rewrite of this layer's
concurrency is the worst of the four moments to attempt it.

What this entry adds is a second reason to do it. It is not only the
0.37–0.42 s a window costs; it is a kernel handle per window that never
comes back. Whoever takes J-6 should know it closes two things.

**The exposure, stated so it is not overstated.** It is bounded by the
process's own life, and this agent's process is short-lived by the
owner's own account — people start it when they want to sign. A thousand
windows is a thousand handles, which is nowhere near any limit. This is
a thing to fix when the layer is next opened, not a thing to open the
layer for.

**Rejected.**
- **Closing the process handles directly.** They are not this project's
  handles to close. Guessing which of a foreign library's handles are
  safe to close, from the outside, is how a process crashes at teardown
  for a reason nobody can reconstruct.
- **Reusing one environment across windows as a narrow change, without
  the rest of J-6.** The environment is created on the window's own
  thread and this package's model is a thread per window ([[D-101]]'s
  ownership rule: exactly one thread may touch a controller). Sharing an
  environment across threads is precisely the part of J-6 that makes it
  a lifetime-model change rather than a smaller one.

## D-171 — The rename retries for a second; "held open" means held, not glanced at

**Date:** 2026-09-07
**Phase:** FTEST Group 3 — flake runs

**Decision.** `platform.WriteFileAtomic`'s rename is wrapped in
`renameWithRetry`: up to one second, in doubling waits from 20 ms, for
as long as the only thing refusing it is another program having the
destination open. Everything else about [[D-165]] is unchanged — write
to a temporary file beside the destination, flush, close, rename over
it, and refuse with `OUTPUT_IN_USE` when the destination is genuinely
held. A read-only destination, and a directory in the destination's
place, are still refused at once: neither can ever succeed, and
retrying them would spend the whole budget proving it.

**Why.** Found as a flake — one of twenty suite runs failed in
[[D-165]]'s own `TestWriteFileAtomicNeverTruncatesTheDestination`, whose
watcher goroutine polls the destination with `os.Stat` while the write
runs. It was tempting to call that a test racing itself and move on. It
is that, and the mechanism it exposes is a product defect: **`os.Rename`
over a destination that anything at all has open fails, including
something that opened it for thirty microseconds to read its size.**
Measured: 30 failures in 200 runs, 15 per cent.

[[D-165]] weighed one kind of held-open destination — a person has the
previous signed document open in a PDF reader — and chose refusing over
destroying, which was right. It did not weigh the other kind, and on
Windows the other kind is everywhere: an antivirus scanner reading a
file that has just appeared in a folder, a search indexer, a backup
agent, Explorer's preview pane, a folder-watching sync client. Against
those, giving up at the first refusal turns a passing glance into a
refused signature — and one nobody can act on, because by the time the
person reads "close it and try again" the file is already closed.

**This is not a softening of [[D-165]].** Write-then-rename is
untouched, the code and the message are the same, and a destination
somebody is really holding is still refused. What changes is only how
long "held open" has to last before it counts: a second rather than an
instant.

**Why a second.** Long enough to outlast anything that opened a file to
look at it, short enough that a person waiting on a signature does not
notice, and far shorter than the several seconds it takes a human to
close a document in another program — so a genuinely held file still
reaches the message that tells them to.

**A second decision, about the test.** The first version of the
regression test span two unthrottled `os.Stat` loops, which is how the
defect was originally measured. It then failed once in a twenty-run
suite pass, on a machine with six fuzzers on it — and that failure was
*correct while the test was wrong*. Two saturating stat loops on a busy
machine do not model something glancing at a file; they model a file
that is open essentially all the time, which no bounded retry can or
should survive.

A test whose verdict depends on how busy the machine is measures the
machine, which is [[D-112]]'s finding exactly, arriving from the other
direction: D-112's test asserted that N things fit in a fixed window,
and this one asserted that a race would go a particular way. The
rewrite says how long the destination is held — 300 ms, released while
the write is under way — and asserts against the stated budget instead
of racing for it. It also checks the write did *not* finish before the
reader let go, so it cannot pass for the wrong reason on a fast machine.

**Rejected.**
- **Calling it a test problem and leaving the product alone.** The test
  is a stand-in for a scanner, and the 15 per cent was measured against
  the shipped function.
- **Falling back to `os.WriteFile` when the rename keeps failing.**
  That is the destruction [[D-165]] exists to prevent, restored under a
  condition nobody would notice.
- **A longer budget — five seconds, or until it works.** Every second
  spent retrying is a second before a person who really does have the
  file open is told so, and a batch of a hundred documents against a
  folder somebody has open would spend eight minutes finding that out
  one document at a time.
- **Retrying on any rename failure rather than only a sharing
  violation.** A read-only destination reports the same Windows status
  ([[D-165]] measured that), and retrying it is a second of certain
  failure for every document in a batch.

---

## D-172 — What Group 3 changed about where defects are looked for

**Date:** 2026-09-07
**Phase:** FTEST Group 3

**Decision, recorded because the pass turned on it.** Six defects were
found. **None of them was found by a failing test, and none of them was
found by looking at what the program showed.** Group 1/2 recorded that
its nine were found by running the shipped binary and looking at the
output ([[D-161]]). This pass found nothing that way — the output was
right every time. What found these was **counting something across
repetitions**:

| Found by | Defect |
|---|---|
| feeding a parser ten bytes it had never seen | C-1, a length that went negative in both independent readers |
| opening one window a hundred times instead of once | C-2, six GDI objects per window, never returned |
| opening one window a hundred times and then waiting | C-3, a process handle per window, and it is not a lag |
| running the suite twenty times instead of once | C-4, a signature refused because something glanced at the file |
| running the suite twenty times instead of once | C-5, one window in a few hundred that does not open |
| counting what was left on the machine at the end | C-6, 1.26 GB of temporary directories the suite could not delete |

Every one is a **rate**, or the sum of one. Not one of them is visible in a single
instance, and every earlier pass in this project looked at single
instances: a screenshot, a signed document, a green run. A signed
document is right or wrong; six GDI objects are neither until you have
a hundred of them to divide by.

This is [[D-161]]'s lesson one step further out. There the finding was
that a test asserting the right property against the wrong fixture keeps
passing. Here it is that **a test asserting the right property against
one instance of the fixture cannot see a per-instance cost at all** —
`cmd/liro-bridge`'s suite makes exactly one of each window, deliberately
and for a good reason ([[D-098]]), and that is precisely why six GDI
objects per window survived four phases of window work.

**What was done about it, beyond the six fixes.** Three of the
measurements are now things anyone can run rather than things this
session did:

- `TestEveryWindowSurvivesAHundredOpenAndCloseCycles`, behind
  `LIRO_WINDOW_CYCLES`, which is what found C-2 and C-3.
- `TestOpeningAndClosingWindowsDoesNotLeakGDIObjects`, which runs in the
  ordinary suite because ten windows is four seconds and this class of
  defect is invisible below two.
- Seven fuzz targets, six of them new, which is what found C-1 and is
  what will find the next one.

`scripts/gencorpus` is committed for the same reason: the corpus this
pass needed had to be rebuilt from a prose description because the pass
that built it deleted it, and a measurement nobody can repeat is a
number rather than a check.

**Rejected.**
- **Reporting the five as ordinary bugs without this entry.** The
  pattern is the finding, and it is a different pattern from the four
  entries before it ([[D-087]], [[D-122]], [[D-128]], [[D-161]]), which
  were all about *looking* at one thing properly. This one is about
  counting many.

---

## D-173 — The canonical string is carried forward unchanged, and its worked example is pinned in three places at once

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** `api.CanonicalString` is

```
METHOD \n PATH \n TIMESTAMP \n NONCE \n sha256hex(BODY)
```

with a single `\n` between the parts, no trailing newline, the method
upper-cased and everything else used exactly as it arrived. `PATH` is
`r.URL.EscapedPath()` — the path as it appeared in the request line,
without the query string. `TIMESTAMP` and `NONCE` are the header
strings themselves rather than re-formattings of what they parse to.

`api.EmptyBodySHA256` is a named constant for sha256 of no bytes, and
`docs/PROTOCOL.md`'s worked example is checked against this package by
`TestTheProtocolDocumentsWorkedExampleIsTrue`, which reads the document
and compares the secret, the timestamp, the nonce, the body, the body
hash, the signature and the escaped canonical string against what the
code produces.

**Why.** The previous implementation of this protocol got this right and
F7 §1 says to carry it forward, so there is nothing to weigh about the
shape. What there is to decide is how it is kept honest, and this
project already knows the answer to that: a fact written down twice is
a fact that can disagree with itself. Three places state this one —
`CanonicalString`, the worked example, and whatever an integrator
writes — and only the first two are ours, so the two that are ours are
pinned to each other by a test.

The three details that are easy to get wrong and are asserted
individually: the timestamp is the *string* that was sent
(`TestCanonicalStringUsesTheHeaderValuesVerbatim` — "1757260800" and
"01757260800" are the same instant and different canonical strings),
only the method is upper-cased
(`TestCanonicalStringUpperCasesTheMethodAndNothingElse`), and an empty
body hashes to something rather than to nothing
(`TestEmptyBodyHashIsTheDocumentedConstant`, which recomputes the
constant rather than trusting the literal).

**The comparison is constant time.** `hmac.Equal` over the decoded
bytes. An ordinary string comparison leaks how much of a forged
signature was correct, which turns one search of 2^256 into 64 searches
of 16. A presented value that is not hexadecimal, or is the wrong
length, is simply wrong rather than an error worth distinguishing:
[[D-175]] is why the caller's answer is the same either way. Uppercase
hexadecimal is accepted even though this project only ever produces
lowercase — leniency in what is read costs nothing, and rejecting the
other spelling would be a rejection nobody could diagnose from the
response.

**Rejected.**
- **Comparing the hex strings rather than the decoded bytes.** Works,
  and makes the comparison case-sensitive for no reason; decoding first
  is both more lenient about the spelling and the same amount of
  constant-time code.
- **`r.URL.Path` rather than `EscapedPath()`.** `Path` is the
  percent-decoded form, and a client signs what it sent. Every path this
  protocol defines is ASCII with nothing to escape, so the two are
  identical today — but a client that percent-encodes something anyway
  should still be able to authenticate.

---

## D-174 — A nonce is spent only after the signature verifies, is scoped to the application that sent it, and the cache sweeps as it goes

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** `api.NonceCache.Use` both checks and records, in one call
under one lock, and `Authenticate` calls it **last** — after the
pairing has been found, the timestamp accepted and the signature
verified. Entries are keyed on `(appID, nonce)`. Every use sweeps
whatever has aged out, under the same lock. The cache has a hard cap
(`maxNonceEntries`, 100 000); reaching it is reported as `RATE_LIMITED`
rather than by evicting anything.

**Why the ordering.** Consuming the nonce first lets an unauthenticated
caller fill the replay cache with fabricated values — and, worse, lock a
real application out of a nonce it is about to use. F7 §1 names this
among the things the previous implementation got right. Verify, then
spend. `TestAFailedSignatureDoesNotSpendTheNonce` is the test that
would fail if the two were ever swapped: it sends a forged signature
carrying the nonce the real application is about to use, asserts the
cache is still empty afterwards, and then asserts the real request with
that same nonce goes through.

**Why check and record are one method.** Splitting them invites a
caller to check early, which is the ordering above undone. There is no
`Seen()` on this type.

**Why per application.** Two applications independently choosing the
same random nonce is otherwise a refused request for one of them, for a
reason neither could ever diagnose from a response that deliberately
says nothing (D-175). Scoping cannot weaken replay protection: a replay
carries the original signature, which verifies only under the original
application's secret, so it only ever reaches its own scope.

**Why the sweep walks the whole map.** That is what the previous
implementation did and it is the right shape here: the map holds only
the nonces of authenticated requests made in the last five minutes,
which for a hundred-document batch is a few hundred entries. A sweep on
a schedule instead needs a goroutine, a shutdown path and a test for
both. The cap bounds the worst case at a walk of a hundred thousand
entries — about a millisecond — and only a caller flooding the agent can
put it there.

**Why the cap reports rather than evicts.** Evicting the oldest entry
would silently re-open exactly the replay window the cache exists to
close. `TestNonceCacheReportsWhenItIsFull` asserts both halves: past the
cap a new nonce is refused, and the oldest one is still remembered.

**A test that had to be made cheap rather than slow.** The first version
of that test built `maxNonceEntries` real entries, which with a full
sweep on every use is quadratic: the package's tests went from under a
second to 77. `newNonceCacheWithLimit` takes the cap as a parameter so
the test reaches it in thirty-two entries. The production constant is
unchanged; what changed is that the test measures the behaviour rather
than the machine ([[D-112]]'s rule).

**Rejected.**
- **A global nonce scope.** Above: a false rejection with no diagnosis
  available, for no gain.
- **Evicting the oldest entry at the cap.** Above.
- **A background sweeper.** More machinery than the problem has.

---

## D-175 — Every authentication rejection is `AUTH_FAILED` with no details; `NONCE_REUSED` and `TIMESTAMP_SKEW` are deliberately not added

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** `Authenticate` returns exactly two codes:

- `NOT_PAIRED` when there is no live pairing for the `appId` — it was
  never paired, or it has been revoked.
- `AUTH_FAILED`, with no `Details` at all, for every other refusal: a
  missing header, an over-long nonce, an unparseable timestamp, a
  timestamp outside the skew window, a wrong signature, a reused nonce,
  an `Origin` header that does not match the bound one.

F7 §8 lists `NONCE_REUSED` and `TIMESTAMP_SKEW` among the codes to add.
**They are not added.** This is the one place in F7 this phase declines
to do what the phase document asks, and it is recorded here rather than
done quietly.

**Why.** F7 §3 states the rule and its reason — "A rejection returns
`401` and a code. **It never says which check failed** — that is a hint
to whoever is probing" — and the exit checklist repeats it as a
checkable item: "Rejections say `401` and a code, never which check
failed." A code that means "your nonce was reused" says that the
timestamp and the signature were both fine, which is precisely the
information §3 exists to withhold: it tells someone working through a
forgery how far they got.

The two instructions cannot both be honoured. Of the two, §3 states a
property with a reason attached and the checklist makes it a test; §8's
list is a sentence naming codes to add. So the property wins, and the
two codes are not added — because a code that can never be returned is
dead surface, and this project's own rule ([[D-132]], [[D-146]],
[[D-151]]) is to delete a catalogue key or constant nothing reads
rather than leave it as a trap for the next person to wire back in.

**What the caller loses, and what replaces it.** An integrator whose
machine clock is three minutes out gets `AUTH_FAILED` and, from the
response alone, cannot tell that from a wrong secret. That is a real
cost. It is paid for in the agent's own log instead: see [[D-176]].

**Why `NOT_PAIRED` is exempt.** It is not one of §3's three checks; it
is the precondition to all of them, and it is the only distinction a
caller can act on — re-pair. Folding it into `AUTH_FAILED` would leave
an application whose pairing a person revoked in Settings with no way to
tell "you were disconnected" from "your signing code is wrong", which is
the one question it most needs answered. What it reveals is which
`appId` values the agent knows, and an `appId` is an identifier the
agent issued to that caller, not a secret.

**Rejected.**
- **Adding the two codes and returning them.** Breaks §3 and the exit
  checklist, for the benefit of whoever is probing.
- **Adding them and never returning them.** Satisfies §8's letter and
  leaves two constants nothing can produce, in a vocabulary SPEC §7 says
  is permanent once written.
- **Returning `AUTH_FAILED` with `details: {"reason": ...}`.** The same
  leak wearing a different field name, and a second vocabulary beside
  the first, which §8's own first sentence exists to prevent.

---

## D-176 — Which check refused a request goes to the agent's own log and no further

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** `Authenticate` logs one line per refusal carrying an
`authReason` — `missing_header`, `nonce_too_long`,
`unparseable_timestamp`, `clock_skew`, `signature_mismatch`,
`nonce_reused`, `origin_mismatch` — alongside the code and the `appId`.
The `authReason` type is unexported and is never serialised.
`docs/PROTOCOL.md` §3.3 tells integrators the log is there and what it
says.

**Why.** [[D-175]] makes every authentication refusal look identical
from the outside, which is right and which leaves an integrator with
nothing to work from. The agent's own log is the one place where saying
costs nothing: it is on the machine the person is already sitting at,
it is read by developers, and it is already English by SPEC §9.2's own
rule for log files. An `appId` is an identifier, not a secret, and no
secret, nonce, body or file name goes near this line (SPEC §18.3).

**It paid for itself in the same session it was written.** The
PowerShell client used to drive this group against a real socket
computed its Unix timestamp with `[double]::Parse((Get-Date -UFormat
%s))`, which on this machine's Serbian locale reads `.` as a group
separator: the timestamp went out as `175726123412345`. Every
authenticated request was refused, correctly, as `AUTH_FAILED` with no
details — and the *table of failure modes the client printed looked
entirely right*, because "a stale timestamp" and "a wrong signature"
both produce `AUTH_FAILED`. What said otherwise was one glance at the
agent's log: fourteen lines of `reason=clock_skew`, including for the
cases that were supposed to be testing something else.

That is the point worth keeping. A uniform answer is the correct thing
to give a caller and a useless thing to debug against, and the log is
what makes the second half survivable. Without it this verification run
would have reported a passing table for a client that was wrong in a way
the table could not show.

**Rejected.**
- **Not logging the reason at all.** The uniform answer would then be
  the only thing anyone ever sees, including the person who has to fix
  the integration.
- **Logging the canonical string the agent computed.** Tempting, and it
  would make the commonest mistake obvious — but the canonical string
  carries the body hash and the path of a signing request, and a log
  line is not the place to start accumulating those. Naming the check is
  enough to find every mistake this session actually hit.

---

## D-177 — The pairing window has no Allow button: the code is the approval

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** The pairing window shows the application's name, its
origin, and six digits, and its only button is **Deny**. There is no
Allow. `pairing.allow` is deleted from all three catalogues.

Once the application supplies the right code, the window switches to a
second screen — the name, "Connected", and one sentence: *the
application can now ask to sign; you still approve every signature
yourself* — with a Close button, and shrinks from 420×330 to 420×210 to
fit it. The spent code is cleared from the page rather than merely
hidden behind the new screen.

**Why there is no Allow.** SPEC §6.2 sketches a window with Approve and
Deny; F7 §2.1 says the older design is better and replaces it, and the
replacement is not "a code as well as a button" — the code *is* the
approval, and a button beside it would be a second, weaker one. A click
proves somebody was at the machine. The code proves somebody was at the
machine **and** is talking to the application that asked, which is the
thing actually worth proving, and the reason the code is not in the
response. Keeping an Allow button would mean a person could approve a
pairing without ever reading the code out, which is the mechanism not
happening.

Deny stays, because refusing has to be one press and not a hunt for the
close box.

**Why the window says something when it succeeds.** The alternative is
that the window a person is reading a code out of vanishes at the moment
they finish reading it. This project has now fixed the same shape of
thing four times ([[D-089]], [[D-097]], [[D-133]], [[D-146]]): a window
or a button that produces no visible response reads as a broken program,
and the fix is always to say what happened. The sentence chosen says the
one thing a person needs to know about what they just did — that it did
not buy the application a signature, only the right to ask.

**Why it resizes.** Photographed at the full height, the connected
screen was half empty under one sentence. The window keeps its own
centre while it shrinks ([[D-148]]'s rule for the signing flow's steps),
so it does not walk across the screen at the moment somebody is looking
at it.

**Verified by looking at it**, in the real window opened by the real
adapter, driven by a real HTTP client, photographed with `PrintWindow`
so taking the picture does not take the foreground from whoever is using
the machine ([[D-122]]). Both screens, in `sr-Latn`. The code on screen
was checked against the code the agent said it had generated.

**Rejected.**
- **Keeping Allow alongside the code.** Above: it is the mechanism not
  happening, available in one click.
- **A Copy button for the code.** The fingerprint has one ([[D-096]])
  because sixty-four hex characters are not readable by eye and six
  digits are. Nothing here needs it.
- **Closing the window the instant the code is confirmed.** Above.
- **A countdown to expiry on the window.** Not asked for anywhere, and
  it would be the page holding state of its own — the thing [[D-120]]
  and [[D-121]] each had to remove once. The window closes when the code
  expires, which says the same thing at the only moment it matters.

---

## D-178 — An application's name is sanitised and the sanitised form is bound; its origin is refused rather than sanitised

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** `validatePairingRequest` puts `applicationName` through
the same pipeline every other piece of untrusted display text in this
project goes through — `consent.SanitizeDisplayText` then
`consent.TruncateMiddle` at `consent.MaxDisplayLength` — and **binds the
sanitised form**. An `origin` is not altered at all: one carrying a
control character, whitespace or a Unicode direction override, or longer
than 255 bytes, is answered `REQUEST_INVALID` with
`details.field = "origin"`.

**Why the name is sanitised and the sanitised form is what is stored.**
SPEC §6.6 requires the display name shown above a signature to be the
one bound at pairing, so that an application cannot pair as "Test" and
later present itself as "Liro". If pairing bound the raw name and each
screen sanitised it separately, the two screens would agree only for as
long as two call sites stayed in step. Binding the sanitised form makes
them the same bytes by construction.

**Why the origin is refused instead.** SPEC §6.2 and F7 §2.1 both
require it to be shown **verbatim** — "no prettifying, no stripping of
the scheme... A user who sees `http://` instead of `https://` must be
able to notice." A value that has been altered on its way to the screen
is not verbatim. So the only two honest options are to display exactly
what arrived or to decline it, and an origin with a right-to-left
override in it is not an origin. Refusing also means the bound value,
the compared value and the displayed value are one string with no
transformation anywhere between them, which is what makes the check on
confirm mean something.

**What the origin binding actually buys, stated rather than implied.** A
program on this machine declares its own origin and can declare
anything; the value is that a person sees it at pairing time and that it
is fixed thereafter. The case where it cannot be forged is a browser,
which sets `Origin` itself — so where the header is present it is
authoritative and must match, at pairing and on every later request.
That defence is real but secondary to [[D-179]]'s, which is that a
browser cannot make one of these calls at all.

**Rejected.**
- **Sanitising the origin the way the name is sanitised.** It would
  silently turn a hostile origin into an innocuous-looking one, which is
  the opposite of letting a person notice.
- **Requiring the origin to parse as a URL.** F7 §0 says any program on
  the machine may pair, and a local program's own idea of its origin is
  not necessarily a URL. Inventing that requirement would refuse honest
  callers to no benefit.
- **Binding the raw name and sanitising at each screen.** Two call sites
  that have to agree forever, where one value would do.

---

## D-179 — The protocol is unreachable from a browser, by construction rather than by instruction

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** Two rules, applied to every endpoint before any handler
sees a request:

- No `Access-Control-*` header is ever sent, and an `OPTIONS` request is
  answered `403` with `REQUEST_INVALID` rather than being treated as a
  preflight.
- Every endpoint requires `Content-Type: application/json`; anything
  else is `REQUEST_INVALID` with
  `details.expectedContentType`.

Every response also carries `Cache-Control: no-store`.

**Why.** F7 §2.3 says the device secret belongs on the integrator's
server and never in a browser — "a secret in a browser is a secret every
visitor has" — and asks for that to be documented prominently. It is,
in `docs/PROTOCOL.md` §2.4. Documenting it is not the same as enforcing
it, and enforcing it here costs two rules:

A request carrying `X-Liro-*` headers, or `application/json`, is not a
"simple request", so a browser must preflight it — and the preflight is
refused. The only thing a browser can send without asking permission
first is a form-shaped POST, and the content-type rule refuses that. The
two together mean page JavaScript cannot make any call in this protocol,
which is a stronger statement than "please do not put the secret in a
page."

`TestTheProtocolIsNotReachableFromABrowser` drives a real preflight
through a real socket and asserts both the status and the absence of
each of the four `Access-Control-*` headers; the real-client run made
the same two calls and got `403` and `400`.

**Rejected.**
- **Answering preflight with a narrow allow-list of origins.** It would
  make the protocol reachable from a page, which is the thing being
  prevented, in exchange for a convenience nobody asked for.
- **Accepting any content type and relying on the signature.** Pairing
  is not signed — it cannot be, there is no secret yet — so the content
  type is the only thing standing between `/v2/pair/request` and a form
  post from a page a person happens to have open.

---

## D-180 — The device secret is base64 in the one response that carries it, and DPAPI-encrypted beside its own entropy on disk

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** `platform.SecretStore` is SPEC §6.4's interface: `Get`,
`Set`, `Delete`, and nothing above it knows what DPAPI is. On Windows it
is a file of `CryptProtectData` blobs (`secrets.json`) beside a file of
32 random bytes (`secrets.entropy`) that every blob is bound to through
`pOptionalEntropy`. On every other platform `NewSecretStore` returns
`ErrSecretStoreUnsupported` — macOS is phase 12 and Linux phase 13 — and
does not fall back to an unencrypted file.

A pairing's metadata (name, origin, timestamps) lives in a plain
`pairings.json`; its device secret lives only in the secret store, under
`pairing.<appId>`. `api.Pairing` has no field a secret could go in.

Over the wire the secret is base64, in the response to
`/v2/pair/confirm` and nowhere else, ever.

**Why base64 and not hex.** The signature and the body hash are hex
because they are text in a text format that an integrator compares by
eye; a 32-byte key is not, and every other binary value this protocol
carries (a digest, a document's content) is base64. `EncodeDeviceSecret`
and `DecodeDeviceSecret` are exported so that a future SDK written in Go
uses this package's own idea of the encoding rather than its own.

**Why the metadata and the secret are stored apart.** The metadata is
exactly what Settings shows and what a person is entitled to look at;
the secret is the one thing that must be encrypted at rest. Keeping them
in different files means no code path that reads a pairing in order to
display it can carry a secret by accident, and
`TestThePairingFileHoldsNoSecret` asserts the metadata file contains
neither the raw bytes nor their base64.

**Why the store refuses on macOS and Linux instead of falling back.** A
device secret in plain JSON is the exact outcome §6.4 exists to prevent,
and a fallback nobody notices is how it would get there.

**What this does not defend against, stated plainly because it would
otherwise look like an oversight.** Another process running as the same
user can read both files and can call DPAPI with the same entropy.
Nothing on the machine stops that and nothing is meant to: SPEC §6.5 is
explicit that the human at the consent window is the only boundary that
holds, precisely because the card caches its own PIN independently of
which process is talking to it. What the encryption buys is what §6.4
claims for it and no more — the blob alone, copied to another machine or
another account, is worth nothing.
`TestSecretStoreBlobAloneDoesNotDecryptWithDifferentEntropy` and
`TestDPAPIRefusesTheWrongEntropy` are what say so, the second against
the real API rather than a stand-in.

**How the file store is tested on a platform that has no protector.**
The encrypting half is one small interface; everything else — the
format, the entropy file's lifecycle, the atomic write, the not-found
behaviour — is shared and is exercised on every platform with a
stand-in. Two tests use the real DPAPI, and they are the ones that would
notice if `CryptProtectData` were never called at all.

**Rejected.**
- **Putting the encrypted secret in `pairings.json` alongside the
  metadata.** One file is simpler and removes the property above: every
  reader of a pairing would then be holding a secret it does not need.
- **An unencrypted fallback on macOS and Linux until phases 12 and 13.**
  Above.
- **Hex for the device secret.** Above.

---

## D-181 — Six new codes for pairing, and one for a malformed request

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** `internal/errs` gains `REQUEST_INVALID`, `RATE_LIMITED`,
`PAIRING_IN_PROGRESS`, `PAIRING_EXPIRED`, `PAIRING_CODE_INCORRECT`,
`PAIRING_ORIGIN_MISMATCH` and `PAIRING_DENIED`, each with a message in
all three catalogues.

**Why each is its own situation and not a `Details` field on one code.**
SPEC §7's rule is that new *situations* get new codes, and this
project's own reading of it ([[D-066]], [[D-104]], [[D-118]],
[[D-165]]) is that two conditions are one situation when they need the
same thing from whoever receives them. These need six different things:

| Code | What the caller must do next |
|---|---|
| `REQUEST_INVALID` | fix the request |
| `RATE_LIMITED` | wait the stated number of seconds |
| `PAIRING_IN_PROGRESS` | wait; somebody else's window is open |
| `PAIRING_EXPIRED` | start a new pairing request |
| `PAIRING_CODE_INCORRECT` | ask the person to read the code again |
| `PAIRING_ORIGIN_MISMATCH` | fix the integration; the two calls disagree |
| `PAIRING_DENIED` | stop asking; this is an answer |

**Why `REQUEST_INVALID` is one code and not one per field.** The
opposite of the above: every malformed field needs the same thing, which
is for the caller to fix its request, and a code per field is a second
vocabulary growing without limit beside the first. `details.field`
carries which one — a structured fact, which is what `Details` is for.

**Why an unknown request identifier is `PAIRING_EXPIRED` and not its own
code.** Expired, voided by wrong codes, already confirmed and never
existing all need the same thing — start again — and folding them
together means guessing identifiers tells a caller nothing about which
ones exist.

**Why `PAIRING_ORIGIN_MISMATCH` is separate even though it looks like a
security answer.** It is almost always an integrator declaring two
different origins in the two calls, and it reveals nothing: whoever
receives it supplied the request identifier it is about. Folding it into
`PAIRING_EXPIRED` would cost a real person an afternoon to save an
attacker nothing.

**Rejected.**
- **Reusing `CONSENT_DENIED` for a refused pairing.** Its catalogue
  message is about an operation being cancelled, and a person reading it
  after a refused *connection* is being told about the wrong thing —
  [[D-066]]'s finding, which is what `STAMP_GLYPH_MISSING` exists for.
- **A single `PAIRING_FAILED` with a `reason` in `Details`.** A second
  vocabulary beside `errs.Code`, which is what F7 §8's first sentence
  rules out.

---

## D-182 — One pairing store per process, because revoking has to be immediate

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision.** `openPairings` is called once per process, in `runTray`,
and the resulting `*api.Pairings` is passed to the settings window. The
settings window revokes through it; the protocol will authenticate
against the same one. `sign --interactive` opens its own, because it is
a different process with nothing else in it holding one.

The settings window shows every pairing — name, origin verbatim, when it
was paired, when it last asked for anything — and one **Disconnect**
button per row. Revoking deletes the device secret first and the
metadata second.

**Why one.** F7 §2.4 says revoking is immediate. Two stores over one
file would each hold their own idea of what is paired: a pairing revoked
in Settings would go on authenticating until whichever store served the
request happened to be reopened. That is the same "one fact stored
twice" failure this project has recorded for a rule ([[D-108]]), for a
question asked in two places ([[D-124]]) and for a margin ([[D-138]]).

**Why the secret is deleted before the metadata.** Of the two possible
half-finished states, a secret that outlives its pairing authenticates
nothing and is invisible; a pairing that outlives its secret also
authenticates nothing but leaves a row a person can see and wonder
about. `Pairings.Secret` additionally reports "no" for a pairing whose
secret has gone, so a half-written state can never be authenticated
against — `TestAPairingWithNoStoredSecretIsNotUsable`.

**Why "last used" is written to disk at most once a minute.** Every
authenticated request updates it, and a hundred-document batch is a
hundred requests; persisting each would be a hundred rewrites of a file
whose only reader is a settings window nobody has open. In memory it is
always current, so a window opened now shows the truth.
`TestLastUsedIsPersistedNoMoreThanOncePerMinute` pins both halves.

**Why revoking re-renders only the list.** Re-posting the whole init
payload would put every unsaved edit in the settings form back to what
is on disk, which is not what disconnecting an application asked for.
`TestRevokingDoesNotDiscardUnsavedSettings` is the guard.

**Rejected.**
- **Opening a store per window.** Above.
- **Passing the pairing list to Settings as data and letting it revoke
  through a callback.** The same thing with more indirection; the store
  is already the thing that knows how.

---

## D-183 — `scripts/synctokens` had drifted from the CSS it generates, and running it would have deleted a live stylesheet

**Date:** 2026-09-07
**Phase:** F7 — group 1 (found while adding a token)

**Decision.** The `.liro-steps*` block — the step header every screen of
the signing flow carries — is folded back into
`scripts/synctokens/intents.go`, and
`TestGeneratedFilesMatchWhatIsCommitted` now reads
`internal/ui/assets/tokens.css` and `intents.css` off disk and requires
them to be byte-identical to what the generator produces.

**What was wrong.** Adding two typography tokens for the pairing code
meant running `go run ./scripts/synctokens` for the first time since the
one-window signing flow was built. It rewrote `intents.css` **1 507
bytes shorter**: the step header's five classes had been written
directly into the generated file and never into the generator. Running
the generator — which its own header tells you to do, and which nothing
had done in between — would have taken the step indicator off every
screen of the signing flow, silently, in a commit about a font size.

**Why this is worth an entry rather than a one-line fix.** A generated
artefact nobody regenerates is a generated artefact in name only, and
the failure mode is not "the generator is stale" — it is "the generator
is a loaded gun". The file says "Do not edit by hand"; that is the rule,
and there was no check that the rule had been followed. There is now,
and it is confirmed to fire in both directions: run against the
generator as it stood before this phase it reports `committed: 14520
bytes, generated: 13013 bytes`, and passes after.

This is [[D-161]]'s and [[D-172]]'s lesson at one remove. Those are
about a test asserting the right property against the wrong fixture, and
about a per-instance cost being invisible to a single instance. This one
is about a check that did not exist at all for a file two things claim
to own.

**Rejected.**
- **Deleting the hand-written block and accepting the loss.** It is live
  CSS that four screens depend on.
- **Dropping the "generated" header from `intents.css` and treating it
  as hand-maintained.** It is genuinely generated in part — the outcome
  colour rules come from `IntentFamilyColor` ([[D-093]]) — so it would
  then be half-generated with nothing saying which half.

---

## D-184 — Group 1 was driven by a client written from the specification, over a real socket, before anything claimed to work

**Date:** 2026-09-07
**Phase:** F7 — group 1

**Decision, recorded because the group turned on it.** Every property
this group claims was checked twice: once by Go tests inside the
package, and once by a PowerShell script that shares no code with the
agent — .NET's own `SHA256` and `HMACSHA256`, its own canonical string,
its own nonce — talking to the real `internal/api` handler over a real
loopback socket, with the real pairing store, the real DPAPI secret
store and the real pairing window on screen.

It is the check F7's own rules ask for: "a protocol that passes its
tests and refuses a real request from a real program is this project's
recurring failure mode." The client was written from
`docs/PROTOCOL.md`, which is the same document an integrator gets.

**What it found.** Nothing wrong with the agent — and one thing wrong
with the *client*, which is the more useful result. Its first version
computed the Unix timestamp with `[double]::Parse((Get-Date -UFormat
%s))`, and on this machine's Serbian locale `.` is a group separator, so
every request went out with a timestamp of `175726123412345`. Every one
was refused. The table the script printed of "every way authentication
can fail" was entirely green, because a stale timestamp and a wrong
signature are the same answer by design ([[D-175]]) — a passing table
for a client that was wrong in a way the table could not show.

The agent's own log said `reason=clock_skew` fourteen times, which is
what [[D-176]] is for and what turned a plausible-looking result into a
found bug in about ten seconds.

**The second thing the harness got wrong, also worth keeping.** The
script read error bodies from the response stream and got empty strings,
so every error code printed blank — PowerShell had already read the body
by the time it threw, and the body is in `ErrorDetails.Message`. The
first run therefore showed a table of eleven refusals with no codes in
it, and it would have been easy to read that as "the codes work" rather
than "the client cannot see them."

Both are the same lesson pointed at the verification instead of the
product: **a harness is code too, and a green result from a harness
nobody has checked is worth what a green test against the wrong fixture
is worth** ([[D-161]]).

**What is verified this way, and what is not.** Verified: pairing end to
end over HTTP, the code not being in any response, the "one at a time"
rule, wrong codes with their remaining count, origin mismatch, confirm
twice, an authenticated request that the agent accepts, eleven ways
authentication can fail and what each returns, a replayed request, the
preflight refusal and the content-type refusal. Not verified here, and
not claimed: the agent binding its own port and writing `bridge.json` —
that is group 2, and until it exists there is nothing for a client to
discover. The listener the client talked to was the harness's own.

**Rejected.**
- **Waiting until group 2 to drive anything with a real client.** The
  canonical string is the thing most likely to be subtly wrong and most
  expensive to find wrong later, and it needed a second implementation
  now rather than after three more groups were built on it.
- **Reporting the first run's green table.** It was green for the wrong
  reason and the log said so.

---

## D-185 — Loopback is a constant, not a parameter; the port range is configurable and the address is not

**Date:** 2026-09-07
**Phase:** F7 — group 2

**Decision.** `api.Listen(start, end)` takes a port range and nothing
else. The host it binds is `loopbackHost`, a package constant
(`127.0.0.1`), and there is no parameter, configuration field,
environment variable or flag anywhere in this project that can change
it. A range that is zero or out of order falls back to SPEC §14's own
17580–17590 rather than refusing.

**Why.** SPEC §6.1 does not say the agent listens on loopback by
default; it says binding to a non-loopback interface must be
*impossible*, not merely off by default. The way to make something
impossible is to leave no way to ask for it — so the address is not an
input, and the one function that binds a socket builds it from the
constant.

That is a property of the source rather than of one call, so a test
that checks what `Listen` returns cannot see it: it would say nothing
about a second `net.Listen` somebody adds next year.
`TestNothingInThisPackageBindsAnywhereButLoopback` walks the package's
own syntax tree instead and requires exactly one socket-binding call
site, in `listen.go`, whose arguments reach `loopbackHost`. That is
[[D-025]]'s method for "no PIN field anywhere" and [[D-158]]'s for
"AllCodes is complete", applied to the one line SPEC §6.1 is about.

**Why the range falls back rather than refusing.** `PortRangeStart` and
`PortRangeEnd` come from `config.json`, where [[D-006]] validates each
field independently and deliberately does not check one against the
other. So a nonsensical pair can reach here, and an agent that will not
listen at all is a worse answer than one that listens where the
specification says it should.

**Rejected.**
- **A configurable bind address, defaulting to loopback.** This is the
  exact shape SPEC §6.1 rules out. A default is something somebody can
  change; the specification asks for something nobody can.
- **Testing the property by connecting from a non-loopback address.**
  It proves one listener at one moment, and needs a second interface to
  connect from. The syntax tree proves it for every listener this
  package will ever have.

---

## D-186 — The discovery file carries three fields; the minimum client version is health's answer and not the file's

**Date:** 2026-09-07
**Phase:** F7 — group 2

**Decision.** `bridge.json` is `{port, agentVersion, protocolVersion}`
and nothing else. It is written atomically
(`platform.WriteFileAtomic`, [[D-165]]) when the agent starts and
removed when it stops. `GET /v2/health` answers `{agentVersion,
protocolVersion, minimumClientVersion}` and needs no authentication.

**Why the minimum client version is not in the file.** An SDK wants it
early, which is an argument for putting it there — and the argument
against is the one this project has recorded three times: a fact stated
in two places is a fact that can disagree with itself ([[D-108]] for a
rule, [[D-124]] for a question, [[D-138]] for a margin). The file's job
is to say *where the agent is*; everything else is one request away
once you know. If the two ever disagreed, an SDK reading the file would
refuse to talk to an agent that would happily have served it.

**Why the file is atomic.** An SDK polling for the agent must never
read a half-written file and conclude the port is `1758`. It costs
nothing: `WriteFileAtomic` already exists for the signed documents.

**Why a stale file is not a problem worth solving.** An agent that
crashed leaves one behind, pointing at a port nothing is listening on.
The SDK's connection is refused, which means exactly what no file at
all means: the agent is not running. A PID in the file, or a lock, or a
heartbeat would each be machinery for a case the existing failure
already answers correctly.

**Rejected.**
- **A `pid` field**, so a stale file can be recognised. Above: the
  refused connection already says so, and a PID is one more thing an
  unauthenticated reader learns about the machine.
- **Keeping the file when the agent stops**, so an SDK can report "the
  agent was here". It cannot tell that from "the agent is here and
  busy", which is worse than nothing.

---

## D-187 — The job registry lives in `internal/jobs`, beside the queue and the runner; `Runner` gains `RunItems`

**Date:** 2026-09-07
**Phase:** F7 — group 3

**Decision.** `internal/jobs` gains `Registry`, `Job`, `JobState` and
`Update` — the whole of F7 §7's job lifecycle, with no HTTP in it. It
also gains `Runner.RunItems(ctx, []Item, SignFunc, Hooks)`; `Run`
becomes one line that calls it with the queue's own items.
`internal/api` turns a `Job` into a 202, an event stream and a 404, and
`cmd/liro-bridge` drives the run.

**Why here and not in `internal/api`.** A job is one batch being
signed, seen by a caller who is not sitting at the window — which is
the same thing the queue and the runner are about. Everything the
registry needs is already in this package's vocabulary: `Progress`,
`Phase`, `Report`, the ETA from measurement. Putting it in
`internal/api` would have meant a second progress vocabulary beside the
first and a second place that knows what a batch is.

**Why `RunItems` rather than a second runner.** A batch that arrived
over the protocol has no queue: its documents are digests a caller
computed or bytes it sent, and a `Queue` is about files from end to end
— `Add`, folder expansion, duplicate paths, output paths. But
everything a *run* decides is identical for both: one document at a
time in order, skip-and-continue (SPEC §12.10), the two codes that end
a batch, Stop between documents and never during one, and an ETA from
the measured first signature. That is one function, and both front
doors go through it rather than each having their own — which is what
the instruction to build on `internal/jobs` rather than beside it
actually asks for.

The change to F6's code is one line: `Run` delegates. Every existing
test passes unchanged, which is the point.

**Rejected.**
- **A second runner for the protocol.** Two implementations of
  skip-and-continue is how the window and the command line come to
  disagree about what a batch did — the failure this project has
  removed for a rule ([[D-108]]), a question ([[D-124]]) and a margin
  ([[D-138]]).
- **Making `Queue` able to hold documents that are not files.** It
  would put `Add`, folder scanning and output paths in front of a
  caller that has no files, and would give `Item` a meaning that
  depends on where it came from.

---

## D-188 — A follower always sees the newest state and the terminal one; the states in between may be coalesced

**Date:** 2026-09-07
**Phase:** F7 — group 3

**Decision.** `Job.Follow` reads the job's state, calls the follower
with it, and then waits on a channel that `Publish` closes and
replaces. A follower therefore always lands on the *newest* state
rather than working through a backlog, always sees the state as it
stands when it connects, and always sees the terminal one.

**Why not a queue of updates.** A hundred-document batch publishes a
hundred `signing` updates. A follower that has fallen behind wants to
know where the batch *is*, not to be walked through where it was; and a
buffered channel that fills has to choose between blocking the run and
dropping something — and the something it drops could be the terminal
state, which is the one update a caller cannot do without.

The close-and-replace broadcast has neither problem: nothing is
buffered, so nothing can be dropped, and the terminal state is
delivered because it is what the follower reads next whatever else it
missed. It is also what makes reconnecting mid-batch correct with no
extra machinery — F7 §12 asks for exactly that case, and a reconnected
stream simply starts from where the job is now.

**What this cost, and the test that had to change because of it.** The
first version of the event-stream test published every state and then
read them, and failed: the fake signer published six states in
microseconds and the reader saw two. That test was asserting something
the design deliberately does not promise. It now hands off — the signer
publishes a state and waits until the stream has reported it before
publishing the next — which is both the honest property (a stream
connected throughout sees every state a run passes through) and a test
that measures behaviour rather than how fast this machine is
([[D-112]]).

**Rejected.**
- **A buffered channel per follower.** Above: it can drop the one
  update that matters, and only under load, which is when a caller most
  needs it.
- **Blocking the run until every follower has read.** A caller that
  stops reading would then stop the signing.

---

## D-189 — The result is handed over once; the "one job" slot frees when the job finishes, not when its result is collected

**Date:** 2026-09-07
**Phase:** F7 — group 3

**Decision.** `Job.TakeResult` returns the result once and never again
— and returns nothing for a job that finished with nothing to give, so
a refused batch and an already-collected one are the same answer.
`GET /v2/jobs/{id}/result` forgets the job before it writes the body.
The owner's one-job slot is released the moment the job reaches a
terminal state, whether or not anybody came for the result. A finished
job nobody collected is discarded after ten minutes.

**Why the slot frees on finish rather than on collection.** F7 §10 says
"a paired application may have one job at a time" and §7.3 says an
uncollected result is discarded after ten minutes. Holding the slot for
those ten minutes would lock a caller out of signing over a batch that
is already done — a caller that crashed between the 202 and the result
would be unable to sign anything for ten minutes. "One at a time" is
about what the agent is doing, not about what the caller has read.

**Why the job is forgotten before the body is written.** A caller that
reads the response and immediately asks again must find nothing, and a
write that fails halfway does not entitle anyone to a second copy of a
qualified signature.

**Two defects the tests found, both real.** `Submit` refused a second
job while the first was merely still *known* rather than still
*running*, so an application could not submit again until its previous
result had been collected or had expired — the ten-minute lockout
above, present in the first implementation. And `TakeResult` reported
success for a job that finished with no result at all, which would have
handed a caller an empty success where a refusal belonged. Both were
found by tests written from the rule rather than from the code.

**Rejected.**
- **Keeping the result for a second collection "in case the first was
  lost".** F7 §7.3 is explicit, and the reason is not caution about
  memory: signatures are the output of a qualified signing operation
  and do not linger waiting to be collected twice.
- **Discarding unfinished jobs on the same deadline.** A job that has
  not finished is bounded by its own consent timeout and by the run it
  is in; forgetting it from under a signature in flight would leave the
  caller with no way to learn what happened to a batch that is still
  happening.

---

## D-190 — What the seven states are, in this agent's own terms

**Date:** 2026-09-07
**Phase:** F7 — group 3

**Decision.** F7 §7.2's seven states map onto this agent's own moments
like this, and the mapping lives in exactly one place
(`mainWindow.publishProgress` and the two calls around it):

| State | The moment |
|---|---|
| `queued` | The request is accepted. Nothing is on screen — another job's window may be open (D-193). |
| `awaiting_consent` | The agent's own window is up and nobody has answered. Carries the remaining time, republished every second. |
| `awaiting_pin` | Approve was pressed; the card session is being opened, which is where the operating system's own PIN dialog appears if the card asks for one. |
| `preparing_card` | `jobs.PhasePreparingCard` — the first signature is in flight. |
| `signing` | `jobs.PhaseSigning` — every signature after the first. |
| `completed` / `failed` | What `internal/api` publishes from the result. |

**Why `awaiting_pin` is the card session and not the first signature.**
The PIN never enters this process ([[D-025]]): the operating system's
smart card provider shows its own dialog, and this agent only knows
that it asked the card for something. Opening the session is the first
moment such a dialog can appear, and `preparing_card` already has a
meaning of its own — SPEC §12.9's measured first-signature cost, which
is card initialisation rather than a person typing.

**Why `jobs.PhaseFinished` publishes nothing.** The run being over is
not a state a caller should see: what it produced is the answer, and
`internal/api` is what turns that into `completed` or `failed`. A
`signing` event published after the last signature would be the last
thing the stream carried before it ended, saying the batch was still
going.

**Rejected.**
- **Publishing `awaiting_pin` for the whole of the first signature.**
  It would make `preparing_card` unreachable, and SPEC §12.9 is
  specific that the ~4.9 s is card initialisation a caller needs to be
  told is expected.

---

## D-191 — The certificate a caller names is binding on the hash path and a suggestion nowhere

**Date:** 2026-09-07
**Phase:** F7 — group 4

**Decision.** `POST /v2/sign` requires `certificateThumbprint`, and the
consent window then offers that certificate and no other. `POST
/v2/sign/pdf` accepts it and does not require it; without one the
person is offered every certificate they would be offered locally. A
named certificate that is not on the machine fails the job
`CERT_NOT_FOUND` **before any window opens**.

**Why the hash path's thumbprint is load-bearing.** On that path the
caller has already built the PDF and the CMS, around one particular
signer certificate. A signature made with a different key produces a
document that verifies against nothing — so letting the person choose a
different one would be letting them produce an invalid signature by
making an ordinary-looking choice.

**Why this does not weaken SPEC §6.5 or §18.15.** The person still sees
the window, still selects the row, and still presses Approve; Approve
is still not the initially focused control and is still disabled until
they have chosen. Nothing is remembered across sessions. What has
changed is only the length of the list — and F7 §9's "certificate
selection is explicit" is about the person choosing deliberately rather
than about how many things they choose between.

**Why a window is not opened when the certificate is absent.** Asking a
person to approve a batch that nothing on this machine can sign is
asking a question with no useful answer, and the failure is the same
either way. Measured in the real-client run: a request naming a
thumbprint no certificate has fails in about half a second, with the
enumeration in the log and no window on screen.

**Rejected.**
- **Treating the thumbprint as a hint on both paths.** It makes the
  hash path able to produce an invalid signature through the person's
  own correct-looking choice.
- **Requiring it on both paths.** On the document path the agent builds
  the CMS, so any usable certificate produces a valid document — and
  requiring one would mean a caller had to learn a thumbprint from
  somewhere, which this protocol deliberately gives it no way to do.

---

## D-192 — A protocol batch is the same window, the same step and the same audit entry; it is not a second consent screen

**Date:** 2026-09-07
**Phase:** F7 — group 4

**Decision.** A request from a paired application runs through
`mainWindow` — the same type, the same certificate step, the same
Approve button, the same audit log — with one field set: `remote`, the
protocol request it is serving. What that field changes is only what
the batch is made of and where its output goes.

- The documents are digests or bytes in memory rather than files, so
  the run uses `jobs.Runner.RunItems` over a slice this flow owns
  ([[D-187]]) rather than a `Queue`.
- The two questions about files — J-3's already-signed question and
  [[D-104]]'s output-file question — are not asked, because neither has
  anything to be about: a protocol batch writes nothing.
- On the hash path there is no timestamp step either, so SPEC §12.8's
  question is not asked: the caller built the CMS and timestamps it
  itself.
- The signatures go back to the caller instead of onto the disk.

**Why not a second flow.** [[D-116]] and [[D-148]] both turned on this:
the consent screen is the product's only real gate (SPEC §6.5), and a
second implementation of it is a second thing that has to stay right
about certificate choice, about never preselecting one, about what the
audit log records, and about Approve not being focused. They would
drift, and the direction they drift in is the one that matters. F7 §9
says the same thing from the other side — "a request from a paired
application is not more trusted than a person dropping files. It is the
same window."

**What this cost.** Four small conditionals in `mainWindow` and one new
file. The window's own layout tests, its step logic, its audit
recording and its progress screen were not touched.

**The application's name comes from the pairing.** `applicationName()`
returns `remote.req.Application` — bound at pairing, sanitised there
([[D-178]]) — or `"local"`. There is no path by which a name in a
signing request reaches a screen (SPEC §6.6, F7 §2.2), and
`TestTheApplicationNameComesFromPairing` sends `applicationName` in the
request body to prove it is ignored.

**Rejected.**
- **A separate protocol consent window sharing only the page.** The
  page is not the gate; the code around it is — which is what F5's own
  measured defects were about ([[D-087]]).
- **Making `jobs.Queue` able to hold in-memory documents.** [[D-187]].

---

## D-193 — One consent window at a time; a job waits in `queued` and its 120 seconds start when its window opens

**Date:** 2026-09-07
**Phase:** F7 — group 4

**Decision.** `protocolSigner` holds a mutex for the whole of one job's
window. A second job — from a different application, since one
application may only have one job (F7 §10) — stays in `queued` until
the first window is answered, and its own 120 seconds begin at the
moment its window is actually on screen: not when the request was
accepted, and not when the flow was entered.

**Why one at a time.** Two consent windows stacked on top of each other
is the maze F6b spent a whole pass removing ([[D-148]]), and the second
one would take the foreground from a person mid-decision on the first.
`queued` exists in F7 §7.2's own list of states, and this is what it is
for.

**Why the clock starts at the window and not at acceptance.** A job
that waited two minutes behind another one's window would otherwise
expire without anybody ever having been asked. The deadline is set in
`mainWindow.open`, after `ui.NewWindow` has returned — so it excludes
enumerating the smart card (measured at 0.2–0.9 s) and creating the
WebView2 instance (0.37–0.42 s since [[D-150]]) as well.

**Rejected.**
- **Refusing a second job outright while a window is open.** F7 §10
  bounds a caller to one job and says nothing about the agent; two
  applications each having one is ordinary, and a refusal would make
  the second one's success depend on the first one's timing.
- **Queueing with no bound.** There is one: each application may have
  one job, and each job expires in 120 seconds if nobody answers.

---

## D-194 — The consent countdown is drawn from Go, once a second, in the last thirty; the page owns no clock

**Date:** 2026-09-07
**Phase:** F7 — group 4

**Decision.** `ConsentTimeout` is 120 seconds and
`ConsentCountdownFrom` is 30. One goroutine per protocol window ticks
once a second: it republishes `awaiting_consent` with the remaining
time — which is what a caller's own countdown is built from — and, once
under thirty seconds, posts `{"type":"countdown","seconds":N,"text":…}`
to the page. The page renders what it is given and computes nothing.

**Why 120.** F7 §7.4 gives both the number and the reason, and the
reason is the one that matters: F2 §4.1 already established a
120-second approval-to-first-signature window
(`signing.ApprovalWindow`), and inventing a second number for the same
human decision would be two answers to one question — the pattern
[[D-108]], [[D-124]] and [[D-138]] each had to remove once.

**Why the page holds no clock.** [[D-120]] and [[D-121]] are both
defects of the same shape: page state that is not a function of what Go
last told it. A `setInterval` in the page would be a second clock,
drifting from the one that actually decides when the job expires, and
would keep counting after a payload that says something else arrived.

**Why the last thirty seconds and not the whole time.** A number on
screen from the first second would make every ordinary signature look
like a race. Someone answering normally never sees a clock at all.

**Verified by looking at it**, in the real binary, with a real request
from a real client: photographed with `PrintWindow` so taking the
picture does not take the foreground from whoever is using the machine
([[D-122]]). At 117 seconds remaining the window shows the application
name bound at pairing (`Knjigovodstvo doo`), the document count, the
certificate list and no clock; at one second it shows *"Ovaj zahtev
ističe za 1 s."* in the warning family above the actions, with the
window the same size and nothing overflowing. The caller's own stream,
over the same two minutes, carried 119 events counting from 117 999 ms
to 999 ms.

**One defect this found.** `consentExpired` published a final
`awaiting_consent` with no remaining time, so the last thing a caller
saw before `failed` was a countdown with no number in it. It publishes
nothing now: the terminal state is the next thing, and it is
`internal/api`'s to publish.

**Rejected.**
- **A countdown from the first second.** Above.
- **Letting the page run its own timer from a single "expires at"
  payload.** One clock, in Go, is what makes the number on screen and
  the moment the job actually expires the same fact.

---

## D-195 — The whole-document path is a setting, and switching it off is its own code

**Date:** 2026-09-07
**Phase:** F7 — group 4

**Decision.** `config.Config.DocumentSigningEnabled` (on by default) is
read on **every** request rather than captured when the listener
started; Settings has a checkbox for it with one line of explanation.
`POST /v2/sign/pdf` answers `DOCUMENT_SIGNING_DISABLED` (403) when it
is off — after authentication, deliberately. `POST /v2/sign` has no
such switch.

**Why a setting and not an install option.** F7 §6 calls the
whole-document path "optional at install time", and the installer that
would offer to leave it out is F10's. Until then the setting is the
whole of that choice, and it is the more useful half anyway: a
deployment can change its mind without reinstalling.

**Why it is read per request.** [[D-134]]'s rule — the configuration
file is the authority, and a copy taken at startup is how a value saved
a moment ago comes back as the old one. A setting that needed the agent
restarted to take effect would be a setting that silently did nothing
until the next reboot.

**Why the check is after authentication.** Whether this machine offers
the path is a fact about how it is set up, and an unpaired caller has
no business learning it.

**Why the hash path has no switch.** It is SPEC §4.3's most secure
arrangement — the agent never possesses the document — and there is
nothing to turn off about an endpoint that cannot see a document in the
first place.

**Three new codes, and why each is its own situation** (SPEC §7, and
this project's own reading of it in [[D-066]], [[D-104]], [[D-118]],
[[D-165]] — two conditions are one situation when they need the same
thing from whoever receives them):

| Code | What the caller must do next |
|---|---|
| `DOCUMENT_SIGNING_DISABLED` | Use the hash path, or ask the person to turn it on |
| `JOB_IN_PROGRESS` | Wait for its own previous job |
| `JOB_NOT_FOUND` | Submit again |

**Rejected.**
- **Folding `JOB_NOT_FOUND` into `REQUEST_INVALID`.** The request is
  not invalid; the job is gone, which is a different thing and needs a
  different reaction.
- **Answering `NOT_PAIRED` when the document path is off.** It is
  paired. Saying otherwise would send an integrator to re-pair, which
  changes nothing.

---

## D-196 — The audit entry records which front door a batch came through, as a fourth optional canonical field

**Date:** 2026-09-07
**Phase:** F7 — group 4

**Decision.** `audit.Entry` gains `Channel`: empty for a batch a person
started here (`ChannelLocal`), `api-digests` for `POST /v2/sign`,
`api-documents` for `POST /v2/sign/pdf`. It is part of
`CanonicalBytes`, behind its own marker byte (2), appended after the
discontinuity's (1).

**Why it is recorded at all.** F7 §6 requires the whole-document path
to be recorded distinctly from the hash path, and the reason is a real
difference rather than bookkeeping: on the hash path the agent never
possessed the document (SPEC §4.3), which is a different fact about a
signature from the other two.

**Why the local channel is the empty string.** So that every entry any
existing audit log holds canonicalises to exactly the bytes it always
did, and every hash in every log already on disk still verifies. That
is the same property [[D-095]] built for `AchievedLevel` and [[D-166]]
for the discontinuity, and it is why `Channel` is emitted only when it
is not `ChannelLocal`.

**Why a marker byte rather than another bare field.** After `PrevHash`
the buffer either ends, or continues with a 4-byte length whose first
byte is 0 (the level), or with 1 (a discontinuity), or with 2 (a
channel), each after the last. Two entries differing in any of them
cannot produce the same bytes, and the next optional field is one more
marker and nothing else.
`TestAChannelIsPartOfWhatIsHashedAndCostsNothingWhenAbsent` pins all
three properties.

**Rejected.**
- **Recording the channel in `Application`** — "My ERP (documents)".
  It would put a computed suffix inside a value that is otherwise
  exactly what a person approved at pairing, and would make the two
  paths indistinguishable for a batch whose application name happens to
  end the same way.
- **A boolean "came over the protocol".** It would lose the
  hash/document distinction, which is the one F7 §6 actually asks for.

---

## D-197 — A request with no body does not declare a content type; what keeps a browser out is the preflight and the absence of any CORS header

**Date:** 2026-09-07
**Phase:** F7 — group 2, corrected by the real-client run

**Decision.** `Content-Type: application/json` is required of every
request that has a body, and of no request that does not. `GET
/v2/health`, `GET /v2/jobs/{id}/events` and `GET /v2/jobs/{id}/result`
require none.

**What was built first, and what refused it.** The first version
required the JSON content type on every endpoint including the GETs, on
the reasoning that a request declaring `application/json` is never a
"simple request", so a page must preflight it, and the preflight is
refused — closing the last door a page could knock on. That reasoning
is correct about browsers and wrong about clients: **.NET's
`HttpClient` puts `Content-Type` on the *content*, so a `GET` with no
body has nowhere to put it at all.** Measured directly, with a client
written from `docs/PROTOCOL.md` against the built binary: every GET
came back `REQUEST_INVALID`, including `/v2/health`, which is the first
call any SDK makes. F9's own .NET SDK could not have made a single
request.

That is precisely the failure F7's own rules name — "a protocol that
passes its tests and refuses a real request from a real program is this
project's recurring failure mode" — and it would have passed every test
in this repository, because Go's `http.Request` lets a caller set the
header on a GET and this project's tests did.

**What actually keeps a browser out, stated exactly.** The
authenticated endpoints require the four `X-Liro-*` headers of F7 §3; a
request carrying those is never a simple request, so a page must
preflight it, and the preflight is answered `403` with no
`Access-Control-*` header of any kind. That covers every endpoint that
does anything. `GET /v2/health` is the one call a page can make, and it
cannot read a word of the answer, because no CORS header is ever sent —
what it could learn is that something is listening on a loopback port,
which it can learn from the connection succeeding.

`TestAPageCannotReadWhatHealthSays` and
`TestAPageStillCannotReachTheJobEndpoints` pin both halves;
`TestHealthAnswersAGETWithNoContentTypeAtAll` and
`TestTheJobEndpointsWorkWithNoContentTypeAtAll` pin the shape a real
client sends.

**Rejected.**
- **Keeping the rule and telling integrators to work around it.** For
  many clients there is no workaround: the header cannot be attached to
  a request with no content.
- **Requiring it only on `/v2/health`**, since that is the one endpoint
  a page can reach. It is also the one endpoint an SDK calls before it
  has anything else, so it is the worst possible one to make hard.

---

## D-198 — A request is authenticated before its body is parsed

**Date:** 2026-09-07
**Phase:** F7 — group 3

**Decision.** The two signing endpoints read the raw body (content
type, declared length, bytes), authenticate against it, and parse it
only afterwards. `readJSONBody` — read and decode in one step — is kept
for the two pairing endpoints, which have no secret to authenticate
against yet.

**Why.** The body has to be *read* before authentication either way:
the signature covers the hash of the bytes as received (F7 §3). Parsing
is a different step, and doing it first meant an unauthenticated caller
sending nonsense got `REQUEST_INVALID` where it should have got
`AUTH_FAILED` — a second thing to work from, in a protocol whose
refusals deliberately say nothing ([[D-175]]).

Found by the real-client run: a case meant to send an empty body signed
as if it had content came back `REQUEST_INVALID` rather than
`AUTH_FAILED`, because the empty body was not JSON and the parser said
so first.

**Rejected.**
- **Parsing first and calling the difference harmless.** It is a small
  leak and it costs nothing to close: the split is four lines.

---

## D-199 — `INTERNAL` is the only 5xx; everything else is a refusal or a 422

**Date:** 2026-09-07
**Phase:** F7 — group 3, corrected by the real-client run

**Decision.** `statusFor`'s default is `422 Unprocessable Content`, not
500. `INTERNAL` maps to 500 explicitly and nothing else does.
`CONSENT_DENIED` and `CONSENT_TIMEOUT` join `PAIRING_DENIED` on 403.

**Why.** The real-client run collected a job the person had simply not
answered and got **HTTP 500** with `CONSENT_TIMEOUT`. Nothing failed:
a window opened, nobody was there, and it expired — which is the
behaviour F7 §7.4 asks for. Answering 500 tells an integrator the agent
is broken and tells their monitoring the same, and it is the sort of
thing that produces a support ticket about the wrong component.

Almost every code in this protocol means "understood, and it did not
happen": the card was not there, the person said no, the document was
not a PDF, the timestamp authority did not answer. 422 is what that is.
500 belongs to `INTERNAL`, which is what `INTERNAL` means (SPEC §7:
"anything unclassified — always accompanied by a local log entry").

Making 422 the *default* rather than enumerating the codes that get it
is deliberate: a code added later lands there unless somebody thinks
about it, and that is the right way round. A new condition nobody has
classified is far more likely to be one of these than to be this agent
failing.
`TestOnlyINTERNALIsAServerError` walks `errs.AllCodes()` and holds the
whole vocabulary to it ([[D-158]]'s method), so this cannot regress one
code at a time.

**Rejected.**
- **200 with a failure body for a failed job.** An SDK's natural shape
  is "2xx means I have a result"; a failed job under 200 makes every
  caller check twice.
- **Enumerating the 422 codes and leaving the default at 500.** It is
  the same list kept by hand that [[D-158]] exists to stop, and it
  fails in the direction that hurts.

---

## D-200 — Group 4 was driven by a client written from the specification, against the built binary; it found two defects in the product and one in itself

**Date:** 2026-09-07
**Phase:** F7 — groups 2, 3 and 4

**Decision, recorded because the phase turned on it.** Everything these
three groups claim was checked twice: by Go tests inside the packages,
and by a PowerShell client sharing no code with the agent — .NET's own
`SHA256` and `HMACSHA256`, its own canonical string, its own nonces —
talking to `liro-bridge.exe tray` over a real loopback socket, with a
real pairing store, the real DPAPI secret store, and the real consent
window on screen.

**What it found in the product.** Two defects, neither of which any
test in this repository could have caught:

1. **Every GET was refused.** The JSON content type was required on
   endpoints with no body, which .NET's `HttpClient` cannot send at all
   ([[D-197]]). Go's own client can, and this project's tests used Go's.
2. **A job the person did not answer was HTTP 500** ([[D-199]]).

Both are the shape F7's own rules name and this project has recorded
five times ([[D-087]], [[D-122]], [[D-128]], [[D-161]], [[D-172]]): a
green suite beside a product that refuses a real request.

**What it found in itself, which is the more useful half.** The
client's first run produced a table of fifteen authentication failures
that was entirely green — and was green for the wrong reason.
PowerShell gives an unbound `[string]` parameter `""` rather than
`$null`, so `if ($null -eq $SignedBody) { $SignedBody = $Body }` never
fired and **every request with a body was signed as if the body were
empty**. Every case in the table failed, including the ones that were
supposed to fail for some other reason, and `AUTH_FAILED` looks the
same either way by design ([[D-175]]).

What said otherwise was the agent's own log ([[D-176]]): fourteen lines
of `reason=signature_mismatch` where four of them should have said
`clock_skew`, `nonce_too_long` and `origin_mismatch`. That is the
second time in this phase a uniform answer has been correct for a
caller and useless to debug against, and the second time the log has
been what made the difference.

This is [[D-184]] happening again, in the same language, to a different
line of it. The lesson stands and is now twice measured: **a harness is
code too, and a green result from a harness nobody has checked is worth
what a green test against the wrong fixture is worth.**

**What is verified this way, and what is not.** Verified against the
built binary: the discovery file and the port it names; `/v2/health`;
the preflight refusal and the absence of every CORS header; fifteen
ways authentication can fail and what each returns; a replayed request;
submission answering 202 with a fingerprint the client recomputed for
itself; one job per application; a job belonging to another application
being invisible; the event stream over a real socket for 117.6 seconds
and 119 events; the consent window opening, showing the name bound at
pairing, and counting down in its last thirty seconds; expiry as
`CONSENT_TIMEOUT`; the result delivered once and 404 after; three
malformed requests with the field named. And, with two agents each in
its own per-user home: two ports, two discovery files, and each one's
application refused by the other as `NOT_PAIRED`.

**Not verified, and not claimed.** A signature. This machine has no
card reader attached — measured, in the agent's own log:
`CryptAcquireCertificatePrivateKey: Cannot find a smart card reader`
for all three enumerated certificates — so no protocol request could
reach a card, and pressing Approve would need a hand at the machine
that [[D-094]] does not allow to be simulated. What stands in for it:
the flow's own tests sign a hundred documents through the real
`runBatch`, the real PAdES engine and a real RSA key, and check the
signatures against the signer's public key. The last step — a person
approving and a card signing — is the owner's, exactly as [[D-097]],
[[D-133]], [[D-146]] and [[D-148]] already record.

**Two sessions were two per-user homes, not two logged-in users, and
that is stated rather than glossed.** A second Windows session cannot
be created from here. What was run is two agents, each with its own
`%LOCALAPPDATA%` — which is the whole of what two signed-in users
differ by as far as this agent is concerned, since the discovery file,
the pairing store and the DPAPI secret store are all under it. What
that does not exercise is anything about session isolation Windows
itself provides.

**The machine was put back.** `config.json` hashes identically to its
snapshot, the audit directory is byte-for-byte unchanged, the autostart
value was never touched, and the Explorer verb — which the agent
re-registers at every start, and which two runs under a temporary home
therefore pointed at the temporary binary — was restored from a `reg
export` taken before any of this began and verified against it
afterwards. Every process started for verification was stopped by its
own exact PID, never by image name.

**Two more, found by the linter and by asking what a screen would show.**
`golangci-lint`'s `unused` reported `batchItems` as dead, which it was —
the progress screen was still rendering `m.queue.Items()`, empty for a
protocol batch, so a person would have watched a hundred documents sign
with nothing on the list. And the report screen said the signatures went
to "each document's own folder", for a batch that wrote nothing anywhere;
it now says they were returned to the application that asked. Both are
[[D-087]]'s class again — a payload that is right about the data and
wrong about what the person sees — and both are now pinned by tests that
read the rendered payload rather than the value behind it.

**Rejected.**
- **Reporting the first run's green table.** It was green for the wrong
  reason and the log said so.
- **Driving the client from Go, sharing this package's own
  `CanonicalString`.** It would have been green for a third wrong
  reason — the one [[D-044]] names for verifiers: a bug in a shared
  helper passes both ways.

---

## D-201 — Three CI failures, all one defect: a test asserting a property must observe the property, not time how long the machine took

**Date:** 2026-09-08
**Phase:** F7 — CI repair

**Decision, and the rule the three share.** A test that asserts a
property must observe that property. It may wait on a channel, an
event, or a state it can read back; it may never wait on a duration,
and it may never assume one goroutine outruns another. Three tests were
red on the CI runner and green here, none of them because the product
was wrong, and all three for that one reason. This is [[D-112]]'s rule,
recorded again because three more instances of it arrived at once and
that entry had recorded only its own.

None of the three assertions was weakened. Each was already asserting
something true; each was asserting it by a means that measured the
machine instead.

**Failure 1 — `TestASuccessfulPairingShowsTheConnectedScreen/en`: a
race, and the numbers say so.**

```
pairing connected (en): document.body scrollHeight 330 exceeds clientHeight 210
```

The window shrinks from 420x330 to 420x210 when a pairing succeeds, and
the test measures after the shrink. Two readings were possible — the
content is genuinely taller on the runner (a substituted font, a
different line height), or the measurement raced the resize and read the
new window height against the old content — and the whole of the fix
depends on which, so it was established rather than assumed.

Measured, in the real window, at the size each screen is really shown at:

| screen | window | body scrollHeight | body clientHeight |
|---|---|---|---|
| connected | 420x330 | 330 | 330 |
| connected | 420x210 | 210 | 210 |
| connected | 420x180 | 195 | 180 |
| code | 420x330 | 330 | 330 |
| code | 420x210 | 270 | 210 |

The connected screen's content needs **195 points** and is given 210,
identically in all three locales — its text wraps to the same number of
lines in each. Nothing on it is 330 tall and nothing could be: 330 is
what `body { height: 100vh }` measures in the window as it stood
*before* the resize. So the reported 330 was never a content height. It
was the old viewport.

That was then reproduced rather than left as an inference. A single
atomic snapshot taken immediately after `Resize(420, 210)` returned
reported `window.innerHeight 330, body.scrollHeight 330,
body.clientHeight 330` — the entire pre-resize layout, after Resize had
returned. And the old test, run 100 times at `GOMAXPROCS=2` with every
core of this machine kept busy, **failed 8 times out of 100** with the
runner's own message.

The mechanism: `Window.Resize` returns once the native window has been
moved and the WebView2 controller's bounds have been set, both on the
window's own OS thread. The page learns its new size down the browser's
own path to the renderer, which is not ordered against `ExecuteScript`,
the path `Eval` uses. `assertPageDoesNotScroll` then asked four separate
`Eval` questions, and the resize landing between two of them compared a
content height measured against one viewport with a client height
measured against another.

Both halves were fixed. `resizeAndSettle` waits until the page itself
reports the new viewport — an observable state, with a 30-second ceiling
that exists only to turn a window that never resizes into a failure
instead of a hang — and `assertPageDoesNotScroll` now takes its four
numbers in one `Eval`, so a pair can never straddle anything. The second
half matters on its own: a fully stale snapshot is self-consistent and
would have asserted the layout of a window nobody was looking at, which
is a false green rather than a false red.

**This is not a product defect. A different one, in the same window, was
found while establishing that — at the end of this entry.**

**Failure 2 — `TestETAIsMeasuredNeverConstant`: a premise that cannot
hold on a loaded machine.**

```
ETA did not grow with measured signature time: fast 41.6492ms, slow 31.7008ms
```

The test signed one batch through a `SignFunc` sleeping 30 ms and
another through one sleeping 2 ms, and required the first batch's
estimate to be the larger. On a two-core runner under `-race` the 2 ms
sleep came back in 41.6 ms and the 30 ms one in 31.7 ms, so the premise
was false and the test failed for being right about a machine it was
never asserting anything about.

The property SPEC §12.9 asks for is that the estimate comes from
measurement and never from a constant — the rule exists because a
hard-coded 4900 ms is wrong for a Pošta card measured at 12.7 s. That is
a property of a function, so it is now asserted over one. `RunItems`'
four lines of estimate arithmetic became `etaFor`
(`internal/jobs/run.go`), unchanged in behaviour, and the test hands it
the durations two different cards would have produced and checks the
arithmetic F2 §5.6 states — including that halving every measurement
shrinks the estimate, so the number is a function of the measurement in
both directions rather than merely larger for a larger input.

A second test keeps the runner itself honest, since a unit test over
`etaFor` says nothing about what `RunItems` feeds it. It asserts an
identity rather than a comparison: the first estimate a run reports
equals `Report.Timing.FirstSignature`, the run's own measurement of its
own first signature. That holds however long the machine took, and no
constant can satisfy it, because a measured duration is never exactly
one.

Its `SignFunc` waits for the process clock to visibly advance before
returning. That is a state being waited for, not a period being slept: a
loaded machine reaches it later and never sooner. It is needed because
`time.Since` across an instant call is exactly **0** on Windows — the
clock's granularity here measured 512 µs — and a first signature of zero
is F2 §5.6's "no measurement yet" ([[D-030]]), so a run of instant
signatures reports no ETA at all. That is correct behaviour and not a
defect: a real signature takes ~0.41 s.

**Failure 3 — `TestTheAwaitingConsentEventCarriesTheRemainingTime`: a
race against a design that is right.**

```
jobs_test.go:398: the stream never reported awaiting_consent
```

The signer published `awaiting_consent` and was released the moment the
test had opened its stream, which assumed the reader was attached and
scheduled before the job left the state. Under `-race` it was not. The
stream is not at fault: a follower lands on the newest state rather than
working through a backlog (PROTOCOL.md §6.2, [[D-188]]), because a
hundred stale `signing` events are worth less than the one saying where
the batch actually is. A state a run passes straight through is
therefore a state a follower may legitimately never see.

So the job is held in `awaiting_consent` until the stream has reported
it, and only then released — the same hand-off
`TestTheEventStreamReportsEveryStateAndEnds` already uses, for the same
reason, recorded in [[D-188]] when that test had to learn it. Nothing
about the stream changed, no sleep was added, and no history is
replayed. Holding the job there is what makes the state observable; a
sleep would only have made it likely.

**A fourth site, found by scanning for the same shape.**
`TestTheWindowResizesToItsContent` (`signflow_windows_test.go`) read
`window.innerWidth`/`innerHeight` once, immediately after `Resize`, and
required them to equal the step's size — the identical defect, one file
away, still green only because nothing had ever loaded this machine
enough. It now waits through `resizeAndSettle` and asserts exactly what
it asserted before.

**A genuine product defect, found while establishing that Failure 1 was
not one — reported here, not fixed.**

Measuring what the connected screen really needs (195 of 210 points)
raised the question of what happens to a name longer than the
26-character one every pairing test uses. A pairing name is
caller-supplied and reaches the window at up to
`consent.MaxDisplayLength` = 120 characters. Measured at that length, in
all three locales:

- **Code screen, 420x330:** content is 354 points. `#app-name`'s
  rendered box runs from **-8 to 104** — its first line is above the top
  of the window. The page overflows, and `deny-btn.focus()` scrolls that
  overflow, taking the application's name off the top of the screen.
- **Connected screen, 420x210:** content is 279 points. `#connected-name`
  runs from **-53 to 59** — a third of the name is off-screen.

This is the class [[D-096]], [[D-106]] and [[D-167]] each name, and the
one `pairing.css`'s own comment already describes for the origin: a
caller-supplied field with no length bound, in a window with a fixed
height. It is worse here than for the origin, because the name is the
one thing on that screen a person is meant to read before typing a code
into somebody else's application — SPEC §6.2's reason for showing the
origin verbatim applies to the name at least as strongly.

It is recorded rather than fixed because the remedy is a design choice
on a security screen, not a mechanical repair: make the identity block
the page's one scrolling region (the shape `.origin-row` already has,
which hides nothing and moves neither the code nor the buttons), clamp
the name to a fixed number of lines with a visible ellipsis, or make
both windows taller for a case that is rare.
The first is recommended.
`TestALongOriginWrapsRatherThanWideningThePairingWindow` covers the
origin and has no counterpart for the name; whichever remedy is chosen
needs one.

**Verified.** Each of the three, one hundred consecutive runs on this
machine at `-count=100`: 100/100 idle, and 100/100 again with every core
kept busy by two load generators — at `GOMAXPROCS=2` for the pairing
test, which is the condition under which the old one failed 8 times in
the same 100, and at `GOMAXPROCS=1` for the two that need no window. The
full suite passes with `-count=1`, with and without the `softtoken`
tag, alongside `gofmt`, `go vet -unsafeptr=false`, `golangci-lint` in
both the Linux and the `GOOS=windows` views, `checkdeps` and `checkcss`.
No `//nolint` was added.

**`-race` was not run, and that is not a formality.** This machine has
no C compiler — verified again here, `cgo: C compiler "gcc" not found` —
which is [[D-012]]'s condition and the same one [[D-112]] had to record.
`-race` is exactly what turned all three of these red, so the runner
remains the only place that check happens, and the load substitute above
is a substitute, not the thing itself.

**The machine was put back.** `config.json`, the audit directory, the
log and the extracted UI assets all hash identically to a snapshot taken
before any of this ran; the only files under `%LOCALAPPDATA%\Liro` that
changed are inside WebView2's own browser profile, which every run of
the UI tests touches. `HKCU\...\Run` exports byte-for-byte identically
and carries no `LiroBridge` value, and the Explorer verb key is still
absent, both as they were. Both load generators were stopped by their
own exact PIDs.

**Rejected.**
- **A sleep, a retry, or a wider budget in any of the three.** Every one
  of them re-picks a number that works on the two machines anyone has
  looked at, which is what [[D-112]] already rejected and the reason
  this entry exists.
- **Making the event stream replay history so Failure 3's state could
  not be missed.** That trades the guarantee that matters — a follower
  always lands on where the batch actually is — for the convenience of a
  test. PROTOCOL.md §6.2 is right.
- **Keeping Failure 2 as an integration test with the sleeps made
  longer.** A ten-to-one ratio was already there and was lost anyway.
  The property is arithmetic over measurements, and a test that cannot
  be defeated by a busy machine is worth more than one that observes the
  arithmetic through a scheduler.
- **Fixing the long-name overflow while in here.** Real, recommended,
  and described above — but it is a layout decision about a window whose
  job is to be read carefully, and making it silently inside a change
  meant to repair three tests is how a design nobody chose ships.
---

## D-202 — The identity block is the pairing window's one scrolling region; a 120-character name now starts inside the window

**Date:** 2026-09-08
**Phase:** Pre-F8 fixes (Task 1)

**Decision.** On both screens of the pairing window, the block that says
who is asking — the application's name, the "wants to connect" line and,
on the code screen, the origin card — is one `.identity` block, and it
is the page's only scrolling region. `.origin-row` keeps its own layout
and gives up the scrolling it used to do alone. The block shrinks and
never grows (`flex: 0 1 auto` over `.liro-scroll-region`'s
`min-height: 0; overflow-y: auto`), so an ordinary name and origin leave
the code exactly where it was.

The connected screen grows from 420×210 to 420×226 — see the defect at
the end of this entry, which is the price the remedy would otherwise
have charged for every ordinary name.

**Why the name and not just the origin.** [[D-201]] measured this and
recorded it rather than fixing it, correctly: the remedy is a design
choice on a security screen. A pairing name reaches the window at up to
`consent.MaxDisplayLength` — 120 characters — and at that length, in all
three locales, the page overflowed its own window and `deny-btn.focus()`
scrolled that overflow, putting the first line of the name above the top
of the screen.

That name is the one thing on the screen a person must read before
typing a code into somebody else's application. An application calling
itself 120 characters of padding followed by its real name puts the real
name out of sight, and the person approves what they cannot see. SPEC
§6.2's reason for showing the origin verbatim — so that somebody who
sees `http://` where they expected `https://` can notice — applies to the
name at least as strongly.

Of the three remedies [[D-201]] listed, this is the one it recommended
and the one that costs nothing: it hides nothing (everything is still
reachable), it moves neither the code nor the buttons, and it stops the
page itself from scrolling at all.

**What it does, measured in a real window at each screen's own size, in
all three locales — the numbers are identical in each.**

| | code screen 420×330 | connected screen 420×226 |
|---|---|---|
| ordinary name | page 330 of 330; identity 123 of 123 — nothing scrolls; name 16..44; code at 177; Deny at 271 | page 226 of 226; identity 46 of 46 — nothing scrolls; name top 16; Close at 167 |
| 120-character name | page 330 of 330; **identity 207 of 132, scrolls**; name **16**..128; code at 187; Deny at 271 | page 226 of 226; **identity 130 of 61, scrolls**; name top **16**; Close at 167 |

Before, at 120 characters: the code screen's content was 354 in a
330-point window and `#app-name` rendered at **−8**..104; the connected
screen's was 279 in 210 and `#connected-name` at **−53**..59. Both
first lines are now at 16.

The one thing that moves is the code, ten points down the code screen,
into space that was empty: the identity block grows into the slack
between the code and the actions rather than pushing anything. Deny and
Close do not move at all — `.actions` keeps `margin-top: auto`, so they
are pinned to the bottom whatever the block above does.

**The test.** `TestALongApplicationNameStaysReadableInThePairingWindow`
posts a name of exactly 120 characters, through
`consent.SanitizeDisplayText` and `TruncateMiddle` so it is the longest
one that can reach this window at all, and asserts on both screens in
all three locales that the page does not scroll, that the buttons are
inside the window, and that the name's first line is inside it. Run
against the page as it stood it reports the finding's own numbers back —
"scrollHeight 354 exceeds clientHeight 330", "#app-name renders at
-8..104 of a 330-point window", "#connected-name renders at -53..59 of a
210-point window" — in each of the three locales.
`TestALongOriginWrapsRatherThanWideningThePairingWindow` is unchanged
except for the region it names.

**The consent window has the same shape, and had the same defect one
field over.** The application name above a signature comes from the
pairing and is bound at the same 120 characters ([[D-178]]), so it was
checked the same way. The page does not scroll and the buttons stay on
screen — the certificate list gives way, which is what [[D-106]] built it
to do — but `.liro-row` lets *every* child shrink, and at 120 characters
the child that gave way was the **label**: "Aplikacija:" squeezed to 33
points of the 50 it needs, and clipped, in all three locales. The
caller's own text taking the space from the word that says what it is.
The label now keeps its intrinsic width and the name wraps inside what
is left. `TestALongApplicationNameFitsOnTheConsentScreen` covers it,
including a check that no element anywhere on the page is wider than the
space it has — which is what caught this.

**A defect the remedy itself introduced, found by looking at the shipped
window.** With the identity block scrollable, the connected screen at
420×210 had **no slack**: measured, its content needed 46.19 points and
the block was given 45.41, so it scrolled by eight tenths of a point and
Windows drew a scrollbar beside an ordinary two-line company name. Every
layout test passed — the page did not scroll, the buttons were on
screen, the name was inside the window — and the shipped window had a
scrollbar in it. That is [[D-167]]'s pattern for the third time and
[[D-087]]/[[D-122]]/[[D-161]]/[[D-172]]'s for the sixth: **a green suite
is not evidence about what a window shows.** It was found by starting
the real binary, pairing with it from a real client and photographing
the window.

The fix is sixteen points of slack (210 → 226, one `--liro-space-4`, so
a one-word label change does not put the scrollbar straight back) and,
more importantly, the assertion that was missing:
`TestAnOrdinaryNameLeavesTheIdentityBlockUnscrolled` measures the
region's own `scrollHeight` against its `clientHeight` for an ordinary
name, on both screens, in all three locales. At 210 it fails with
"#state-connected .identity scrolls for an ordinary name — 45 of 46".

**Rejected.**
- **Clamping the name to a fixed number of lines with a visible
  ellipsis.** [[D-201]]'s second option. It hides part of the one value
  on the screen that must be read in full, and the part it hides is the
  end — which is exactly where an application that pads its name puts
  its real one.
- **Making both windows taller for a case that is rare.** [[D-201]]'s
  third. A 120-character name needs 354 points of content on a 330-point
  screen; sizing for it means every ordinary pairing looks at a window
  two thirds empty, and the next longer catalogue string starts the
  argument again ([[D-106]]: widening is not a fix, it is a delay).
- **Truncating the name in Go before it reaches the window.** It is
  already truncated, at 120 characters, by the pairing itself
  ([[D-178]]); truncating further would mean the value the person
  approves and the value bound at pairing are different bytes, which is
  the property [[D-178]] exists to hold.
- **Leaving `.origin-row` as a second scrolling region inside the
  identity block.** Two scrollbars in a 420-point window, one inside the
  other, is the double-scrollbar defect [[D-106]] rules out by
  construction.

---

## D-203 — `GET /v2/certificates`: the names, never the certificates; no new consent, and a Settings switch

**Date:** 2026-09-08
**Phase:** Pre-F8 fixes (Task 2)

**Decision.** `GET /v2/certificates` is a new authenticated endpoint. It
answers with one entry per certificate a person would be offered:

```json
{"certificates":[{"thumbprint":"52EA0BAB…","displayName":"Zoran Milovanović",
  "issuer":"Halcom BG CA PL e-signature","purpose":"signing","qualified":true,
  "usable":false,"notUsableReason":"CARD_NOT_PRESENT","isTestKey":false}]}
```

`internal/api` gains a `CertificateSource` interface, for the same
reason it has `Signer`: SPEC §4.2 rule 4 forbids it importing
`internal/cli`, and `cmd/liro-bridge` is the one place that knows a
certificate comes from Windows CNG. The implementation
(`gatheredCertificates`) is `cli.Gather` — the same enumeration
`liro-bridge certs` prints and the same one the agent's own certificate
step shows — narrowed by the same one rule every listing in this program
goes through, `classify.Info.HiddenByDefault` ([[D-108]], [[D-149]]).
Three copies of that rule is how a listing and a window come to disagree
about what a machine holds.

**Why it exists.** `/v2/sign` requires `certificateThumbprint` and
PROTOCOL.md §5.1 explains why: the caller has already built a CMS around
one signer certificate, so any other key produces a document that
verifies against nothing ([[D-191]]). Nothing in the protocol told a
caller where to get a thumbprint. That is a hole, and F8's SDK walks
straight into it — an SDK must be able to offer the person a choice.

**What is in it, and what the shape follows from.**

*Nothing the classification layer strips.* A Serbian qualified
certificate carries the holder's national identity number in its Subject
DN and their email address in the DN or the SAN (SPEC §11.6).
`internal/trust/classify` scrubs both before anything is displayed or
logged, and this endpoint reports what that layer produced. There is no
field on `api.Certificate` a Subject DN could travel in — the same
allow-list guarantee `audit.Entry` gets its safety from ([[D-084]]),
rather than a check on the way out.

*`isTestKey`, because SPEC §16.6 is unconditional.* A soft-token
signature must be visibly marked everywhere it appears, and a caller
that could not tell one apart would be the one place it was not.

*Not the validity dates.* The task named six things and this is not one
of them; `notUsableReason: CERT_EXPIRED` already covers the case a
caller can act on. SPEC §0's "do not invent requirements" applies to a
field as much as to a feature.

*`purpose`, even though every row a listing returns today says
"signing".* The authentication twin every Serbian card carries is
hidden by the rule above ([[D-149]]), so the value is currently
constant. It is carried anyway because SPEC §11.5 requires every row to
state the role KeyUsage gives it, and because a caller told "signing"
has been told rather than left to assume.

**The DER, decided rather than deferred.** A caller building a CMS needs
the signer certificate itself — for `issuerAndSerialNumber`, for
`signingCertificateV2`, and for the certificates set (SPEC §12.3). It is
**not** here, and the reason is the first rule above: the DER *is* the
national identity number and the email address, in full, wrapped in
ASN.1. Putting it on a listing that is answered without any window, at
any moment a paired application chooses, turns "which certificates
exist" into "here is the identity document of the person at this
machine". Whatever else is true, that cannot be the endpoint whose whole
justification is letting an SDK show a name.

**So the CMS path keeps a hole, and it is stated rather than glossed.**
A caller that has never seen the certificate still cannot build a CMS
around it; today it has to come from wherever the integration already
gets it — a copy the person exported, or a document they have already
signed. The answer is not to widen this endpoint but to make the
certificate arrive with something the person approved: the smallest such
change is for `/v2/sign`'s own result to carry the signer certificate,
which is a signature the person pressed Approve for. That is a decision
about the signing path, not about a listing, and it is recorded here for
whoever takes it up rather than made in passing. PROTOCOL.md §5.5 says
the same to an integrator.

**Consent: no new pairing-time question, and a Settings switch.**

*No pairing-time consent.* [[D-177]] is explicit that the pairing window
has no Allow button because the code is the approval, and that a button
beside it would be a second, weaker one. A permission checkbox there is
worse than a button: it asks a person to evaluate a capability at the
one moment they have no context for it, on the screen whose whole value
is that it asks one thing.

*A Settings switch, on by default* (`certificateListingEnabled`, read on
every request the way `documentSigningEnabled` is, checked after
authentication for the same reason [[D-195]] checks that one after it).
The argument for a switch at all is SPEC §14.1's bookkeeper: on a
machine holding several clients' cards, this listing tells an
application paired by *one* client the names on the *other* clients'
certificates. Nothing in a pairing implies that, and the person at the
machine is who should be able to decline it.

The argument for it being on by default is that an SDK which cannot
enumerate cannot offer a choice, which is the endpoint's entire purpose;
and switching it off costs nothing that is actually signed — on
`/v2/sign/pdf` a caller never needed a thumbprint, and on `/v2/sign` it
has already built a CMS around a certificate it therefore already knows.

`errs.CodeCertificateListingDisabled` (`CERTIFICATE_LISTING_DISABLED`,
403) is its own code rather than `NOT_PAIRED`, for [[D-181]]'s test: two
conditions are one situation when they need the same thing from whoever
receives them, and "you are not paired" would send an integrator to
re-pair, which changes nothing. All three catalogues carry a message.

**Verified against the built binary, with a client that shares no code
with the agent** — a PowerShell script using .NET's own `SHA256` and
`HMACSHA256`, its own canonical string and its own nonces, over a real
loopback socket to `liro-bridge.exe tray` running under its own
`%LOCALAPPDATA%`. Against this machine's real certificate store it
returned exactly one row — a real Halcom signing certificate, `usable:
false`, `notUsableReason: CARD_NOT_PRESENT`, with no card in the reader
— which is the listing rule working: the machine's Windows-internal
certificates and the authentication twin are not in it. No PEM, no `@`,
no thirteen consecutive digits anywhere in the response, and no
`Access-Control-Allow-Origin` header. Unauthenticated: `401
AUTH_FAILED`. With `certificateListingEnabled` set to false in the
configuration file while the agent was running: `403
CERTIFICATE_LISTING_DISABLED`, and `200` again the moment it was set
back — no restart, which is [[D-134]]'s rule.

**Rejected.**
- **Returning the DER or a PEM.** Above.
- **Returning it only for a certificate the person has already approved
  a signature with.** Closer to right, and still the wrong endpoint: it
  makes a listing's answer depend on history, and the place for a
  consented certificate is the consented operation.
- **No switch at all.** Considered seriously, on the argument that a
  pairing already grants more than this. It does not, on the machine
  that matters: a pairing is one client's decision, and the listing
  answers about all of them.
- **Refusing rather than hiding the machine's internal certificates.**
  Nothing to refuse — `HiddenByDefault` already answers exactly this
  question for `certs`, for the certificate step and for the
  Certificates window, and a fourth answer would be a fourth thing to
  keep in step.

---

## D-204 — Both pairing calls carry `origin`, and the document now says so in prose

**Date:** 2026-09-08
**Phase:** Pre-F8 fixes (Task 3)

**Decision.** `docs/PROTOCOL.md` §2.1 states, beside the confirm
example, that both calls carry `origin`, that the two must be
byte-for-byte equal, that it is a required field of the confirm body,
and that a confirm which declares a different origin — or leaves it out
— is answered `PAIRING_ORIGIN_MISMATCH` with the request still live. §2.2's
rule is reworded from "confirm must come from the same origin as the
request" to "confirm carries the same `origin` as the request, and it
must match"; the error table's action becomes "send the same `origin`
field in both calls (§2.1)"; and §9's walkthrough says the origin in
step 5 is the same string as in step 3.

**Why, and what was actually wrong.** The example already showed the
field — this is not a case of the document omitting it. What it did not
have was a sentence. The only prose was §2.2's "confirm must come from
the same origin as the request", which reads as a statement about *where
the call comes from* — a transport property, or the `Origin` header —
rather than about a field in the body that the agent compares. The owner
lost time on exactly this writing the first client, and the only other
hint was the error table's "send the same origin in both calls", which
an integrator reaches after they are already stuck.

An example shows what a correct request looks like; it does not say
which parts are required, what happens when one is missing, or that two
of them have to be equal to each other. Those are the three things that
cost the time, so those are what the prose now says. The §3.4 pointer is
there because "origin" now means two different things on the same page —
a body field the agent compares and an HTTP header a browser sets — and
an integrator who conflates them looks for the bug in the wrong place.

**Verified against the built binary, with the same independent client**
that verified [[D-203]]: confirm with no `origin` answers `403
PAIRING_ORIGIN_MISMATCH`, confirm with a different one answers the same,
and confirm with the same one answers `200` — the behaviour the document
now describes, in that order, against a real pairing whose six-digit
code was read off the real window.

**Rejected.**
- **Making `origin` optional on confirm, defaulting to the request's.**
  It would remove the mistake by removing the check, and the check is
  half of what binds a pairing to an origin (SPEC §6.2). An integrator
  who declares two different origins has a bug this answer tells them
  about.
- **Leaving it to the error table.** That is where it was.

---

## D-205 — `POST /v2/echo` returns the canonical string this agent built, and nothing else

**Date:** 2026-09-08
**Phase:** Pre-F8 fixes (Task 4)

**Decision.** `POST /v2/echo` is a new endpoint, authenticated exactly
like every other, that returns

```json
{"canonicalString": "POST\n/v2/echo\n1757260800\n9f2c…\n7f24…", "bodySha256": "7f24…"}
```

and nothing else. At most 64 KB of body. The body is hashed, never
parsed: it must be declared as JSON like every other body, but it does
not have to be valid JSON, so an integrator can send exactly the bytes
they are about to sign.

**Why.** Every way authentication can fail returns `401 AUTH_FAILED`
with no detail. That is right — [[D-175]] settled it and it stands: a
code that says "your nonce was reused" tells whoever is probing that the
timestamp and the signature were both fine. The cost is that a first
integration is a hunt. Writing the first client produced four failures
in a row, each giving the same answer: a wrong field name, a missing
origin, a body altered between hashing and sending, and a `Content-Type`
.NET cannot put on a GET.

[[D-176]] answered half of that — the agent's own log names which check
refused each request, and PROTOCOL.md §3.3 points at it. This is the
other half: the log says *which check*, and this says *what the agent
hashed*. An integrator prints their own canonical string beside it and
the difference is visible in one line. Each of those four failures would
have taken a minute.

**The reasoning was confirmed before it was implemented, not after.**

- *It reveals nothing.* Every part of what it returns is something the
  caller supplied moments earlier and still has: the method, the path,
  the timestamp, the nonce and its own body. The signature is not
  returned, and nothing here would let one be derived — that needs the
  device secret, which this endpoint never touches.
- *A request that reaches the handler has already authenticated.* The
  string it echoes is the string of a request that was accepted, so a
  caller that could not authenticate learns nothing here it could not
  learn from any other endpoint. `TestEchoIsAuthenticatedLikeEveryOtherEndpoint`
  asserts that a refused request is told nothing at all — no
  `canonicalString` in a 401 body.
- *It answers about the one request that carried it and nothing else.*
  There is no way to ask it about somebody else's request, or about an
  earlier one.
- *A nonce is spent here as anywhere else*, so a replayed echo is
  refused (`TestAnEchoIsNotReplayable`).

`bodySha256` is repeated on its own even though it is the canonical
string's last line, because the commonest single mistake is a body that
changed between being hashed and being sent, and comparing one
64-character value is easier than finding it inside a five-line string.

**Verified against the built binary, with the independent client.** The
client computed its own canonical string from the four header values and
the bytes it sent; the agent's answer matched it character for
character, and the body hash matched the client's own SHA-256. A request
signed for one body and sent with another — the mistake the endpoint
exists to make visible — was still refused `401 AUTH_FAILED`, which is
the property that must not soften: this is a comparison tool, not a way
past the gate.

**Rejected.**
- **Returning the signature the agent computed.** It would turn a
  debugging aid into an oracle: a caller could then check a guessed
  secret against a known canonical string offline.
- **Returning which check refused a request.** That is [[D-175]]'s
  answer and it does not change. An echo is only reachable by a request
  that already passed every check.
- **Echoing on a GET, so it can be called with no body at all.** The
  mistake most worth catching is about a body, and a GET has none.
  `/v2/echo` with an empty body still works and still reports
  `EmptyBodySHA256`, which is the constant §3.1 states outright.
- **A larger body limit.** 64 KB is generous for a `/v2/sign` request
  naming five hundred digests, which is the body an integrator most
  wants to compare, and nothing here holds a document.

---

## D-206 — A remembered stamp position never applies to a request that arrived over the protocol

**Date:** 2026-09-08
**Phase:** Pre-F8 fixes (Task 5)

**Decision.** `newProtocolWindow` drops the remembered placement from
the configuration this run uses — the corner falls back to
`config.DefaultStampPosition` and the placed page and coordinates to
zero — before anything else about the stamp is decided. Nothing is
written to disk: the remembered position is the person's own standing
answer and stays exactly as they left it.

The method question is otherwise unchanged. A caller that supplied its
own `stamp` still sees the approval and nothing else ([[D-124]]); a
caller that did not still leaves the question with the person, who
chooses in the window exactly as they do locally; and with no choice
made, the default corner applies.

**Why.** A saved position is an answer to "where on *this* document",
given by a person who was looking at the page when they gave it
([[D-140]], [[D-151]]). The documents in a protocol batch are not that
document and nobody has looked at them: they arrived over a socket, from
a program, possibly a hundred at a time.

The owner's first protocol signature landed the stamp on top of the
document's existing MUP signature. He saw it, because he looked. An ERP
sending a hundred documents has nobody looking.

**A second way the same error could arrive, closed with it.** With the
placement dropped, the method screen preselects a corner rather than
"choose the position" — but a person can still choose the first method
deliberately. On a protocol batch there is no document on disk to open
the picker on, and `placeStamp`'s no-path branch opens a *file chooser*:
the person would have browsed for some other document and placed the
stamp by looking at that. That is the same error one step further on, so
`signAtAChosenPosition` refuses it the way F6b §5 already refuses a
document the renderer cannot draw — it falls back to the corners and
says why, through the existing `place.unavailable` message and the
existing path.

**Occupied-corner detection stays out of scope**, on the owner's own
ruling: a person can see the page and will move the stamp themselves.
Nothing here looks at what is already on a page.

**Why the flow's construction moved into its own function.**
`runProtocolFlow`'s setup — what makes a protocol batch differ from a
person's own — was duplicated by the test helper that exercised it, so a
difference could exist in one and not the other. That is [[D-134]]'s
finding exactly: a test that supplies both the input and the expectation
from the same value measures its own consistency. It is now
`newProtocolWindow`, and the tests go through it.

**Verified.** `TestAProtocolBatchNeverSignsAtTheRememberedPosition`
gives the run a configuration with a real placed position (page 2, x
371, y 79 — the shape a person actually leaves behind), asserts that a
*local* batch with that configuration does sign at those coordinates
(so the fixture is known to reproduce the defect), and then asserts that
a protocol batch does not — `UseXY` false, no placed page, no
coordinates — both with a stamp supplied and without one. Against the
code as it stood it reports "a protocol batch signs at the remembered
coordinates (page 2, 371, 79)".
`TestAProtocolBatchWithNoStampOffersACornerNotAPlacedPosition` covers
what the person is then offered, and
`TestAProtocolBatchLeavesTheRememberedPositionOnDisk` covers the half
that would be a worse defect than the one being fixed: the file is
byte-identical afterwards.

**Rejected.**
- **Dropping it only when the request supplies no stamp.** The narrower
  fix, and it leaves the placed page and coordinates in the run's
  configuration for the supplied case too. They are unreachable there
  today only because a supplied stamp always names a corner; a rule that
  holds by accident is a rule waiting to stop holding.
- **Making the placement picker work on a protocol batch's own
  documents.** They are in memory and the picker reads a path, so this
  is a real feature — showing a caller's document to the person and
  letting them place a stamp on it — and it is not this pass's.
- **Refusing a protocol batch that has no stamp answer at all.** It
  would make an ERP's ordinary request fail for want of a corner nobody
  asked it to choose.

---

## D-207 — One WebView2 environment, one apartment and one thread per process; measured before and after, and two of the three things it was meant to close do not reproduce

**Date:** 2026-09-08
**Phase:** Pre-F8 fixes (Task 6, J-6)

**Decision.** `internal/ui` has one UI thread for the whole process
(`uithread_windows.go`): one goroutine locked to one OS thread, one STA
apartment taken with `OleInitialize` and never given back, one
`ICoreWebView2Environment` created at the first window and kept, and one
message loop that runs for the process's life. Every window is created
on it, pumped by it, and torn down on it. `ui.NewWindow` hands a closure
to that thread through a message-only window and waits.

Everything else about the model is deliberately unchanged. Exactly one
thread touches a controller and it is the thread that created it
([[D-101]]'s ownership rule) — now the same thread for every window,
which satisfies the rule more simply rather than less. `PostJSON`,
`Eval`, `Navigate`, `Resize` and `Close` still marshal by posting
`wmRunFunc` to their *own* window, so `wndProc`'s per-window
`tearingDown` guard still decides whether a queued closure may run. No
caller callback ever runs on it: `OnMessage`, `OnFilesDropped` and
`OnClosed` are still delivered by one goroutine per window
(`dispatchEvents`).

Two things had to go with the per-window thread. `wmDestroy` no longer
posts `WM_QUIT` — that would end the loop every other window depends on
— and no longer shuts the apartment down, which would take the
environment and any open window's controller with it. `closeWebView` no
longer releases the environment: it is the process's, not the window's.

**Why a message-only window rather than `PostThreadMessage`.** A thread
message has no window to be dispatched to, so any nested modal loop that
retrieves it discards it — and
`ICoreWebView2Controller::Close` runs a nested loop ([[D-101]]). The
closure it was announcing would never run and `NewWindow` would wait
forever. A window message is routed to the work window's procedure by
whichever loop retrieves it, nested or not.

**The three measurements, on this machine, before and after.** Each
number is from the same harness against the same pages, warm (one window
opened and closed before the baseline, so no one-time cost is counted as
a per-window one).

| | before | after |
|---|---|---|
| **Window creation, warm** (30 creations) | mean **207 ms**, best 177, worst 434 | mean **214 ms**, best 186, worst 426 |
| **Window creation, cold** (first three of a fresh process) | 411 ms, 202, 190 | 429 ms, 200, 197 |
| **Kernel handles** across open/close cycles | 271 → 287 over **200** cycles (+16), wandering: 279, 285, 291, 301, 270, 278, 278, 290 | 276 → 320 over **200** cycles (+44), **flat**: 322 by cycle 25, then 324, 324, 324, 325, 325, 325, 325 |
| **Creations that failed outright** | **0 of 600** | **0 of 600** |

**Two of the three things J-6 was recorded to close do not reproduce on
this machine today, and that is reported rather than smoothed over.**

*Creation time was never the environment.* J-6's premise — "window
creation costs what it costs because each window builds its own
environment" — does not hold. WebView2 already deduplicates an
environment per user-data folder, so the second
`CreateCoreWebView2EnvironmentWithOptions` in a process was already
cheap: the cold measurement shows the first window costing ~410 ms and
the second ~200 ms *before* the change, and exactly the same after it.
The ~200 ms is the controller and the navigation, and this change does
not touch either. Anyone reading [[D-131]]'s "what would actually make
it faster, recorded rather than done" should read this line with it: it
would not have.

*C-3's one-handle-per-window does not reproduce.* [[D-170]] measured
0.97 handles per window and confirmed they did not come back after two
minutes. Measured again here on the same instrument's newer sibling
(`GetProcessHandleCount`, sampled after a GC and a two-second grace):
+16 over 200 cycles, wandering up and down as browser process groups
came and went. Whether the difference is the WebView2 runtime, the
instrument, or something about how the two harnesses opened their
windows was not established — what is established is that the leak this
change was supposed to close is not measurable here in either shape.

*C-5's one-in-a-few-hundred failure does not reproduce either*: 0 of 600
before and 0 of 600 after.

**What the change does do, measured.** After it, handle growth is *flat*
— +44 paid within the first 25 cycles and then 324→325 across the next
175 — where before it wandered by tens with no settled value. The +44 is
the UI thread, the environment and the browser process group behind it,
and it is paid once. The browser processes themselves are not held:
counted around one window, `msedgewebview2.exe` went 12 → 18 → 12, and
stayed at 12 twenty seconds after the last window closed, with the
environment still alive. It also removes an OS thread, a COM apartment
and an environment from every window's lifetime, in a layer with four
recorded lifetime defects ([[D-099]], [[D-101]], [[D-114]], [[D-129]]),
and makes [[D-101]]'s ownership rule a statement about one thread rather
than about many.

**What it costs, stated because it is new.** One wedged window creation
now stops every other window from being created, where before it stopped
only its own. That is accepted on measurement rather than on faith: the
hang [[D-099]] recorded was fixed by [[D-101]] and did not reappear in
600 consecutive creations either side of this change. It is the price of
the environment being shared at all, and it is why the alternative
[[D-170]] rejected — sharing an environment without sharing the thread —
is not available: an environment belongs to the apartment that created
it.

**Tests.** `internal/ui/uithread_windows_test.go` pins the property
rather than the defect, because the property cannot exist in the old
shape: two windows share one environment and one thread, closing a
window leaves the environment for the next, a second window opens while
the first is up, and closing one window leaves the others answering —
that last being what `WM_QUIT` in `wmDestroy` would have broken.
Every one of them opens at least two windows, because a per-instance
cost is invisible to a fixture there is only one of ([[D-172]]).
`TestOpeningAndClosingWindowsDoesNotLeakGDIObjects` and
`TestEveryWindowSurvivesAHundredOpenAndCloseCycles` (behind
`LIRO_WINDOW_CYCLES`) are unchanged and green; across seven windows and
25 cycles each the handle count converges — 276, 299, 309, 311, 312,
318, 320, 320 — and the last two subtests add nothing at all.

**Rejected.**
- **Reverting it because the numbers do not show what J-6 predicted.**
  Considered, and it is the owner's call rather than this pass's to
  reverse. What the measurements say is that the *reasons* recorded for
  the change were wrong, not that the change is: flat is better than
  wandering, one environment is what every WebView2 host does, and the
  lifetime model is smaller than it was. The numbers are here so that
  the decision can be made on them.
- **Sharing the environment while keeping a thread per window.**
  [[D-170]] rejected it and was right: `CreateCoreWebView2Controller`
  must be called on the apartment that created the environment.
- **A deadline on the wait in `uiThread.do`.** It would turn a wedge
  into an error, which is what SPEC §12.8 asks for elsewhere — and it
  needs a story about what the caller does next that this pass does not
  have. Recorded as still open, exactly as [[D-099]] left it.
- **Shutting the apartment down at process exit.** There is nothing to
  gain: the operating system reclaims it, and any code that ran early
  would be shutting it down under a window still using it.

---

## D-208 — The pairing window is a two-field table under one typeface, and the connected screen is a mark, a word and a name

**Date:** 2026-09-08
**Phase:** Pre-F8 redesign (Tasks 1 and 2)

**Decision.** Both screens of the pairing window are redrawn. Nothing
either of them does changes: no new strings arrive, none of them are
sanitised differently, the origin is still shown verbatim, there is
still no Allow button, and the identity block is still the page's one
scrolling region ([[D-177]], [[D-178]], [[D-202]]).

*The code screen* is a two-field table — a small quiet label above a
larger, stronger value, twice — with the code centred below it and Deny
alone in the bottom right:

    Zahtev
    Knjigovodstvo d.o.o. — ERP

    Poreklo
    https://test.local


                     Kod
                 0 3 7 8 6 8
          Unesite ovaj broj u aplikaciju.
             Ovaj kod važi 5 minuta.

                                         [ Odbij ]

`pairing.wants_to_connect` is deleted from all three catalogues and
`pairing.request_label` — *Zahtev / Захтев / Request* — takes its place.
The label says what the sentence said, above the value it is about,
which is one line of screen instead of two and one less thing to read.

*The connected screen* is a green circle-check, **Povezano**, and the
application's name, centred, one under the other, with Close where it
was. `pairing.connected_explain` — "the application can now ask to sign;
you still approve every signature yourself" — is deleted from all three.

**Why one typeface.** Before this the screen had four treatments: the
name at `--liro-font-size-display`, the phrase under it at body size,
the origin in `--liro-font-family-mono` inside a bordered card, and the
code in mono again. Four for two facts about one caller, and the two
facts did not look like the same kind of thing — the name read as a
title and the origin as a technical detail, when the origin is the one
SPEC §6.2 says a person must be able to notice `http://` in. A label
above a value, twice, in one face, says they are two fields of one
record. The distinction between a label and its value is carried by
size, weight and colour, which is what type is for; a second family
would have said the two were different in kind, which they are not.

The code keeps `--liro-font-size-code` and `--liro-letter-spacing-code`
and gives up the monospace family with everything else. It is six digits
with generous letter spacing at thirty-four points: nothing about
reading them across a desk needed the family, and the family was the
last thing on the screen making the page look like two pages.

**Why the code block is separated and centred.** Two things say the six
digits are not a third field about who is asking. The gap above them is
`--liro-space-5` against the `--liro-space-3` between the fields, and
the block — label, digits, note — is centred on the window's own axis
while the table above it stays left-aligned.
`TestTheGapAboveTheCodeIsLargerThanTheGapBetweenTheFields` and
`TestTheCodeIsCentredOnTheWindow` assert both in all three locales
rather than leaving them to whoever next edits the stylesheet.

Centring the digits needs one correction that is invisible until it is
missing. `letter-spacing` puts its gap after the *last* character too,
so a centred run of six spaced digits sits half a letter-space left of
where it looks like it should; `text-indent: var(--liro-letter-spacing-code)`
puts it back. Measured, in the real window: with the correction the ink
spans 136.0..284.0 in a 420-point window and is centred on 210.0; without
it, on 206.9. The test measures the ink rather than the paragraph's box,
because the box is shrink-wrapped and centred by the flex container
whatever the type inside it does — it reads 210.0 either way, which is
exactly how this would have shipped unnoticed.

**Why the note under the code is two lines.** It says two things — what
to do with the number and how long it lasts — and the sentence a person
has to act on should not have to be found inside a paragraph.
`pairing.code_explain` becomes *Unesite ovaj broj u aplikaciju.* and a
new `pairing.code_validity` carries *Ovaj kod važi 5 minuta.*, in all
three catalogues. Two elements rather than one string with a line break
in it: a catalogue holds sentences, not layout, and a translator who is
handed one string containing a newline will eventually be handed one
without it. `TestTheCodeIsCentredOnTheWindow` also asserts each of the
two renders as exactly one line in every locale, so a longer translation
that quietly wraps to three is a failure rather than a surprise in the
window.

**Why the connected screen loses its sentence.** [[D-177]] put it there
to say the one thing a person needs to know about what they just did —
that pairing did not buy the application a signature, only the right to
ask. That is still true, and it is still the reason the screen exists at
all; what is no longer true is that a paragraph is how to say it. The
screen is shown for the few seconds between a code being confirmed and
somebody pressing Close, and a mark, a word and a name are read in one
glance where three lines of prose are read by nobody. The sentence's
content is not lost: SPEC §6.5 makes the consent window the gate, and
that window is where a person meets the claim again, with a document in
front of them.

The keys are **deleted from all three catalogues**, not merely
unreferenced, and
`TestTheStringsTheRedesignRemovedAreGoneFromEveryCatalogue` asserts both
are gone in all three — the check
`TestThePairingWindowShowsTheCodeAndNoAllowButton` already makes for
`pairing.allow`. An unused message is one edit away from being wired
back in.

**The mark is drawn in the page, and both of its numbers are tokens.**
Inline SVG: a circle and a check, `stroke="currentColor"`, inside an
element whose `color` is `var(--liro-color-positive)` and whose width
and height are `var(--liro-icon-size-lg)`. `--liro-icon-size-lg: 40px`
is a new token in `scripts/synctokens`, because SPEC §10.1's answer to a
value that does not exist as a token is to add one, and a spacing token
standing in for an icon size would have been the number written down
twice under a name that means something else. `scripts/checkcss` stays
green; there is no literal in the CSS and none in the HTML either.

An icon library was not added. One glyph is not a dependency, and a font
that has to load is a mark that is missing for the first frame of a
screen whose whole content is that mark.

**The sizes, and the content heights behind them.** Measured in the real
window at each screen's own size, in all three locales — the numbers are
identical in each, because the only text that differs between them is
three short labels.

| | code screen 420×369 | connected screen 420×228 |
|---|---|---|
| room the identity block can be given | 140 | 61 |
| one-line name | identity 98 — nothing scrolls; code at 138, note 34 tall, Deny at 310 | identity 22 — nothing scrolls; mark 16..56, Povezano 64..92, name 100..122, Close at 169 |
| ordinary two-line company name | identity 121 — nothing scrolls, 19 points spare | identity 45 — nothing scrolls, 16 points spare |
| 120-character name | identity 143 of 140, **scrolls**; name's first line at 16; code at 180, Deny at 310 | identity 67 of 61, **scrolls**; name's first line at 100; mark and Povezano do not move; Close at 169 |

**Why the code screen grew, 330 → 369.** Not for the 120-character
case — for the ordinary one, and the height is set by one rule: the
identity block must not scroll for an ordinary two-line company name,
with one `--liro-space-4` of slack. Everything below that block grew.
The fields are taller than the old prose-and-card — the origin is
sixteen points rather than twelve — the gap above the code is
twenty-four rather than sixteen, and the note under the code is two
lines rather than one. Measured at 330 with the new fields, that name
needed 121 points in a block that could be given 118; at 352, with the
note still one line, 123. Three points short, and then two, is a
scrollbar beside an ordinary name — exactly the defect [[D-202]] found
by looking at the shipped connected screen, and exactly the one every
layout test passes through: the page does not scroll, the buttons are on
screen, the name is inside the window, and there is a scrollbar in the
picture. 369 gives the block 140.

**Why the connected screen grew, 226 → 228.** The sentence came off and
a forty-point mark went on, and what sets the height is unchanged: an
ordinary two-line company name must not make the identity block scroll,
with one `--liro-space-4` of slack so that a one-word label change in
any of the three catalogues does not put the scrollbar straight back.
That name needs 45; at 228 the block can be given 61. The number was
measured again rather than carried over, which is the whole reason it
moved by two points instead of staying where it was.

**What a 120-character name costs, stated plainly.** Deny and Close do
not move at all — `.actions` keeps `margin-top: auto`. The code does: it
sits at 138 for a one-line name and at 180 for a 120-character one,
because the identity block grows into the slack between the code and the
actions rather than pushing anything. That is [[D-202]]'s mechanism
unchanged (`flex: 0 1 auto` over `.liro-scroll-region`), and the
movement is larger than the ten points [[D-202]] measured only because
the slack it grows into is larger. Pinning the code absolutely would
mean giving the identity block a fixed height — dead space under every
ordinary name, to hold still for a case that is padding.

**Verified by looking at it.** The binary was built and started in tray
mode, a real HTTP client posted `/v2/pair/request` and then
`/v2/pair/confirm` with the code read off the screen — which is the
mechanism [[D-177]] describes, performed rather than simulated — and
both screens were photographed with `PrintWindow`, so taking the picture
did not take the foreground from whoever was using the machine
([[D-122]]). Twelve pictures: both screens, an ordinary name and a
120-character one, in `sr-Latn`, `sr-Cyrl` and `en`. No synthetic mouse
or keyboard input anywhere ([[D-094]]); the one window that had to be
dismissed between runs was sent `WM_CLOSE`, a window message. The
process was stopped by its own exact PID.

The machine was left as it was found, checked by hash: `config.json`,
the audit directory, `HKCU\...\Run` and the Explorer verb. One of the
four had actually changed — starting the agent from `dist/` made
`applyExplorerMenu` re-point the `.pdf` verb at the binary that was
running, which is that function working correctly and is why the hash is
taken rather than assumed. It was restored.

**Rejected.**
- **Keeping the origin in monospace.** It is the argument for the
  redesign in miniature: monospace says "technical detail", and SPEC
  §6.2 wants the origin read as carefully as the name.
- **`text-transform: uppercase` on the labels.** The sketch this was
  drawn from writes KOD in capitals; it also writes the application's
  name in capitals, and the name is caller-supplied text this window may
  not alter. One rule for all three labels, and the catalogues already
  hold them in the case Serbian writes them in.
- **One catalogue string with a newline in it for the two-line note.**
  A catalogue holds sentences; a translator handed one string with a
  line break in it will eventually hand one back without it, and the
  layout would then depend on a character nobody can see in a JSON
  file. Two keys, two elements.
- **Leaving the note as the single sentence it was** ("Dajte ovaj kod
  aplikaciji. Važi pet minuta."). Two instructions in one line, and the
  one a person has to act on — type this number over there — was the
  half nobody read.
- **An icon font or library for the check mark.** Above.
- **Reusing `--liro-space-6` as the mark's size.** 32 points is the
  right order of magnitude and the wrong name: a spacing token used as a
  size is the value written down twice, once under a name that means
  something else.
- **Sizing either window for the 120-character case.** [[D-201]]'s third
  option and [[D-202]]'s rejection, unchanged: a name that long needs
  143 points on a screen whose ordinary name needs 98, and sizing for it
  means every ordinary pairing looks at a window a third empty.
- **Leaving the connected screen at 226 because the content got
  shorter.** It did not: a sentence came off and a forty-point mark went
  on. The height was measured again, which is the point.

---

## D-209 — The stamp says "Elektronski potpisano", sets the signer's name in a real bold face, takes its ink from the tokens, and groups the serial in fours

**Date:** 2026-09-08
**Phase:** Pre-F8 redesign (Task 3)

**Decision.** Four changes to the visible signature stamp, and nothing
else. The width is still 190 points, the margin still 12, the logo is
still the full-colour mark on the left, the SN line and the seconds in
the timestamp both stay, and the height still comes out of the table
`geometry.go` computes from `stampLineHeight` rather than out of
anybody's hand. [[D-125]]'s wording and its four-line arrangement are
superseded; the rest of it — the label follows the interface, the name
follows the certificate, the date is not localised — stands unchanged.

### 1. The label is "Elektronski potpisano"

`sign.stamp_label` in all three catalogues:

| Locale | Was | Is |
|---|---|---|
| `sr-Latn` | Digitalno potpisano | **Elektronski potpisano** |
| `sr-Cyrl` | Дигитално потписано | **Електронски потписано** |
| `en` | Digitally signed | Digitally signed |

"Digitalno" describes how the signature was made. "Elektronski" is what
the thing *is* — the term the Serbian law on electronic documents uses,
the term the MUP certificate's own policy text uses ("Ovo je
kvalifikovani sertifikat za elektronski potpis," which is in the
certificate this was verified with), and therefore the term a person
holding the printed document already has a meaning for. English is
untouched: "Digitally signed" is the established English form, capital
D and lower-case s, and changing it to match the Serbian would be
translating in the wrong direction.

The key name did not change, so no key was added; what the change
required was that the *old strings* be gone rather than merely unused,
and `TestStampLabelIsTheElectronicWordingInEveryCatalogue` searches
every value of every catalogue for all four old forms (both scripts,
both the "-ano" and the earlier "-ao" endings) rather than only checking
`sign.stamp_label`. A phrase left behind under some other key would
still reach a screen.

### 2. The signer's name is bold, and the bold is real

The name is the one thing a person looks at a signature stamp to find,
and in a four-line block of 7pt type the only thing that says so is
weight.

**What it cost, measured before choosing.** The stamp embeds a
subsetted NotoSans as Type0/CIDFontType2 (SPEC §13.2, [[D-051]]), and
that subset has no bold face — a bold face means generating and
committing a second one:

| | Regular | Bold | |
|---|---|---|---|
| Subset `.ttf` | 31 052 B | 31 184 B | committed asset |
| `/FontFile2` stream (Flate) | 13 142 B | 13 514 B | in every stamped document |

- **The binary grows by 31 232 bytes** — measured, by building
  `./cmd/liro-bridge` with and without the asset: 15 472 128 against
  15 440 896. That is 0.2% of the binary.
- **Every stamped document grows by 14 934 bytes** — measured, by
  applying the incremental revision with one face and with two: 17 936
  bytes against 32 870. The extra beyond the font stream is the second
  `/W` array, the second Type0/CIDFont/FontDescriptor dictionaries and
  their xref entries.
- Against a real signed document that is small. The one this was
  verified on — a blank page, real MUP card, B-B — is 104 708 bytes, of
  which the certificate, the chain and the CMS are most; a B-LT document
  with revocation data is larger again.

So it was affordable, and it was taken. **The name is set in a genuine
NotoSans Bold, not in a synthetic stroke.** That matters enough to be
tested rather than asserted:
`TestBoldFaceIsAGenuineBoldNotTheRegularFaceRenamed` reads the committed
asset with an independent decoder and
measures the width of the capital I's outline, which is a stem and
nothing else — 258 design units regular, 325 bold, a quarter again
heavier. A renamed copy of the regular subset, or a `Tr 2` fill-and-
stroke fake, passes every other test in the file and fails that one.

**Two faces, one of everything else.** Both subsets are generated by
`scripts/gensubsetfont` from the same sorted `charset()`, and it assigns
GIDs sequentially in that order, so **a character's glyph index is the
same number in either face**. Three things follow, and all three are the
reason the second face is as cheap as it is:

- `runeToGID` is not duplicated. `subset_data_bold.go` carries advance
  widths and vertical metrics and nothing else — a second copy of the
  rune table would be a second thing to drift, and a drifted one would
  not fail: it would draw the signer's name in the wrong letters.
- The `/ToUnicode` CMap is written once and referenced by both Type0
  fonts. That saves 2 790 bytes per document and, more to the point,
  leaves one answer to "what text is this" rather than two that have to
  agree.
- The bold face's character coverage cannot be narrower than the regular
  face's, so no name that could be stamped before can fail to be stamped
  now. Restricting the bold subset to the capitals a name upper-cases
  into was considered and rejected below.

`/StemV` is 160 for bold against 80 for regular. Both remain the
conventional placeholders the descriptor comment describes — this subset
carries no OS/2 data to measure from — but a bold face declaring the
regular face's stem width would be a statement this project knows to be
false.

### 3. The ink comes from the design tokens

`stampInkR/G/B` is `#16211F`, which is `--liro-color-text-primary`, and
`TestStampInkIsTheDesignTokensTextPrimary` reads
`internal/ui/assets/tokens.css` and checks it — the same standard the
logo's `#038387` is held to against the logo asset ([[D-070]]). Reading
the file from a test, rather than importing anything, is what keeps
`internal/pades` clear of the window layer (`scripts/checkdeps`).

**An honest note on "darker".** The task asked for darker text, and the
measurement says the text was already as dark as a PDF gets: the content
stream set *no* fill colour at all, so it drew in the initial graphics
state, which is pure black. Rendered at 4× before the change, the
darkest text pixel is `#000000`. `#16211F` is very slightly *lighter*
than that in absolute terms.

It was applied anyway, and not as a technicality. Black-by-omission was
never a decision anyone made; it meant the stamp's colour was whatever
graphics state a viewer happened to be in when it drew the appearance
stream, and a viewer that left a fill colour set would have tinted the
text. Naming the colour makes it deterministic, and makes it the same
ink as every window in the product. What actually makes the block read
darker on paper is the second change above — weight, not hue — and if
more is wanted the lever is size or weight, not a darker grey, because
there is no darker grey to reach for.

### 4. The serial is grouped in fours, and it fits

`SN 20F0 48A7 68F5 6F09 9E`, from the left, so the short group falls at
the end where a reader comparing digit by digit has already stopped
counting. Eighteen unbroken hexadecimal digits is not something a person
can read off a page, check against a certificate viewer, or read down a
telephone; groups of four are the length at which people reliably do all
three, and it is how a card number and an IBAN are already set.

It is **longer**, not shorter — 25 characters against 21 — and the
stamp's width is fixed, so it was measured rather than assumed:

| Line | Width at 7pt | Text column |
|---|---|---|
| `SN 20F048A768F56F099E` (was) | 82.30 pt | 142 pt |
| `SN 20F0 48A7 68F5 6F09 9E` (is) | **89.58 pt** | 142 pt |

The text column is `190 − (4 + 36 + 4) − 4 = 142` points. The grouped
line is 89.58 of them at the nominal 7pt, with 52 points to spare.
**Nothing was reduced and nothing was changed to make it fit**, and
`TestGroupedSerialFitsTheTextColumnAtNominalSize` measures the real MUP
serial through `fitLine` so that what is asserted is what is drawn.
Halcom's serial is eight digits and less than half as wide. A serial at
X.509's full 20-byte limit would be 52 characters and would still be
reduced then truncated by `fitLine`, exactly as it was before grouping;
neither issuer this project has certificates from comes near that.

### 5. The text block is centred against the logo

It used to start at the top of the box and run down from there. In a
48-point four-line stamp that put the text's centre at 24 and the mark's
at 22 — the logo was pinned at the bottom padding, leaving 8 points of
white above it and 4 below, and the block sat visibly higher than the
mark it stands beside.

Both are now centred on the box: `logoY` puts the mark at
`(h − 36) / 2`, and `textBlockTop` centres a block of `n ×
stampLineHeight` on the same point, clamped into the padding — bottom
edge first, so the top edge wins for a six-line stamp that cannot fit
either way. `TestLogoAndTextBlockShareOneCentre` checks the two centres
coincide for every line count the height table defines, not only for the
four-line case that prompted it.

The line spacing itself is unchanged (`stampLineHeight` is still 10, and
still the only number the height table is computed from), so no height
in the table moved: a four-line stamp is still 48 points. The lines also
no longer *stretch* to fill whatever height the box has, which they did
before — for one to three lines that spread four lines' worth of
leading over the box and read as a paragraph rather than a stamp.

### How this was verified

- **A real document, signed with a real card.** The MUP certificate on
  this machine (`…B3D1ECCE`, ВЕЉКО СТАНОЈЕВИЋ, MUP Gradjani CA 4),
  B-B — the command line was given no `--tsa`, so B-T degraded under
  `--on-tsa-failure b-b` and said so, which is [[D-067]] and [[D-047]]
  and not this change's business. The result verifies:
  `ByteRangeDigestOK`, `SignatureOK` and `SigningCertificateOK` all
  true through `internal/pades/verify`, the independent reader.
- **Rendered at 4×** through `internal/pades/render` — the project's own
  rasteriser, which is not the code that wrote the stamp — from the
  finished signed document, and from both real certificates (MUP,
  Cyrillic; Halcom, Latin) in all three interfaces. All six say the
  right thing: the label follows the interface, the name follows the
  certificate and is never transliterated.
- **The renderer is also the test.**
  `TestStampPreviewDrawsBothEmbeddedFacesWithoutSubstituting` puts the
  stamp through that
  rasteriser and fails if it had to *substitute* a face — because a
  substituted face still draws a picture with letters in it that looks
  broadly right, so the picture is not the evidence; the absence of the
  note is. Proved to fail: with the bold asset replaced by 31 184 random
  bytes it reports "substituted a FontFile2 font program", and the asset
  was restored and its digest checked.

**Rejected.**
- **Faking the bold with a synthetic stroke** (`2 Tr` plus a small
  `w`). Free, and it does thicken the letters — but it thickens them
  outward from the same outlines, so the counters fill in at 7pt and the
  advance widths stay the regular face's, which means the line measures
  as one width and draws as another. The cost of the real face was
  measured first, at 0.2% of the binary and 15 KB per document, and it
  was not high enough to accept a fake. Had it been, this entry would
  have said so out loud rather than shipping a stroke quietly.
- **A bold subset of only the capitals a name upper-cases into.** About
  70 glyphs instead of 177, so perhaps 40% of the cost. Rejected because
  `strings.ToUpper` leaves uncased characters alone: a name containing a
  digit, or any character outside that list, would then fail to stamp in
  bold although it stamps today in regular — a name that could be signed
  becoming a name that cannot, to save 8 KB.
- **Bolding the label as well, or instead.** The label is the least
  informative line on the stamp; emphasis on every line is emphasis on
  none, and it would spend the same 15 KB per document to say nothing.
- **Reducing `stampLineHeight` so four lines fit inside the logo's 36
  points.** It would make the block and the mark exactly the same
  height, which is tidy, and it would tighten 7pt type below the 1.4×
  leading the existing reasoning arrived at. The instruction was to
  compute the height from the line height, not to choose a line height
  that produces a pleasing height.
- **Setting the ink to a chosen dark grey instead of the token.** That
  is the value written down in a second place, which is the thing this
  project's token rule exists to prevent — and, as measured above, every
  candidate grey is lighter than what was already being drawn.
- **Grouping the serial from the right,** so the short group leads. Card
  numbers and IBANs group from the left, and a leading short group reads
  as a prefix rather than as the beginning of the number.

---

## D-210 — The repository root is the npm package; `sdk/typescript/dist` is committed, and a CI step proves it is what `src/` produces

**Date:** 2026-09-09
**Phase:** F8

**Decision.** There is a `package.json` at the repository root. It is
named `@liro/bridge`, its `exports` point into
`sdk/typescript/dist/{esm,cjs,types}`, and its `files` list carries the
SDK's built output, its source, its README and `docs/PROTOCOL.md`. The
SDK's own `sdk/typescript/package.json` is unchanged in intent — same
name, same version, same exports relative to itself — and is what a
developer works in.

`sdk/typescript/dist/` is committed. `npm run check-build` rebuilds into
a scratch directory and diffs the two file by file; CI runs it, and
`test/package.test.mjs` runs it again from the test suite.

**Why the root and not `sdk/typescript`.** F8 §2 gives the install
command — `npm install github:veljaos/liro-bridge#main` — and npm has no
way to install a subdirectory of a GitHub repository. What that command
installs is the repository's *root*, so the manifest that describes the
package has to be there. The alternative is telling every integrator to
clone and point a `file:` dependency at a subdirectory, which is not
what F8 asked for and is not what anybody does.

Two things make the root manifest honest rather than a trick. `files`
lists exactly what a consumer needs, so `npm pack` produces a 97 KB
tarball rather than the whole repository — measured. And
`TestTheRepositoryRootIsInstallableAs@liro/bridge`'s equivalent
(`test/package.test.mjs`) reads every path the root manifest names and
fails if one of them is not there, so a manifest that points at output
somebody deleted cannot ship.

**Why `dist/` is committed.** Installing from GitHub runs no build step;
there is no `prepare` script that could run one, because a `prepare`
would need the TypeScript compiler in the integrator's own install. So
what is committed is what an integrator gets, exactly.

**Why the check matters more than the convention.** This project has
already had one generated file drift from its generator, and running the
generator would have silently deleted a block five screens depended on
([[D-183]]). A committed artefact nobody regenerates is a committed
artefact in name only. `check-build` is what makes "do not edit `dist/`
by hand" true rather than hopeful, and it is confirmed to fire: run
against a `dist/` with one character changed it names the file and both
byte counts.

**Verified end to end, because a package that passes its own tests and
cannot be installed is this project's recurring failure mode one layer
over.** `test/package.test.mjs` packs the repository root exactly as npm
would, installs the tarball into an empty project, and imports
`@liro/bridge` both ways:

```
esm: typeof LiroBridge.connect === 'function'   ✓
cjs: require('@liro/bridge').LiroBridge         ✓
node_modules after installing: @liro            (and nothing else)
```

CI does the same on a clean runner.

**Rejected.**
- **A `package.json` only in `sdk/typescript`.** The natural place, and
  it makes F8 §2's own install command not work.
- **npm workspaces at the root.** Workspaces are for developing several
  packages together; they do nothing for a consumer installing this one
  from GitHub, because what they install is still the root.
- **Building on install with a `prepare` script.** It would keep `dist/`
  out of the repository, and it would put a TypeScript compiler into
  every integrator's dependency tree — in a package whose whole
  discipline is that it adds nothing to theirs.
- **Publishing to npm.** The owner's decision, recorded in F8 §0: not
  publishing removes a class of risk (name squatting, an abandoned
  package taken over) that a signing SDK should not carry.

---

## D-211 — `secretStore` is required with no default; the secret is in a `WeakMap`, and `JSON.stringify` of a pairing leaves a marker that names the fix

**Date:** 2026-09-09
**Phase:** F8

**Decision.** `LiroBridge.connect` refuses without a `secretStore`, and
there is no default path anywhere in the SDK. `StoredPairing` keeps the
device secret in a module-level `WeakMap` keyed by the instance, not in
a field; `toJSON()` puts `REDACTED_SECRET` where the secret would be;
`util.inspect` and `toString` are given their own implementations;
`serialise()` is the one way the bytes come out, and
`StoredPairing.deserialise()` the one way back.

`FileSecretStore` writes one JSON file at a caller-named path,
atomically, and restricts it: `0600` on POSIX, and on Windows one
`icacls` call that breaks inheritance, grants this user full control,
and removes Everyone, Authenticated Users, Users and Interactive — by
SID, not by name, because those groups are localised and this product's
users are on Serbian Windows. A failure to restrict is an error, not a
warning.

**Why no default.** F8 §3 is explicit and the reasoning is not
stylistic: the device secret is an application's whole authority to ask
for a signature, and a library that picks a location has made that
decision in every deployment that never thought about it. The type
system is what makes somebody decide.

**Why a `WeakMap` rather than a `#private` field.** A `#private` field
is still a property: some Node versions print them under
`util.inspect(..., { showHidden: true })`, and the set of things that
can reach one has grown before. Nothing outside this module can reach a
`WeakMap` entry at all, and no future change to how Node prints objects
can make it visible. Measured across four `inspect` depths with
`showHidden` and `getters` both on, and through `JSON.stringify` nested
two objects deep.

**Why a marker rather than silence.** A `SecretStore` implementation
that persists `JSON.stringify(pairing)` — the first thing anybody writes
— would otherwise store a record with no secret in it, and the failure
would surface later as `AUTH_FAILED`, which is the least diagnosable
answer this protocol has by design ([[D-175]]). `deserialise` recognises
the marker and throws `SECRET_STORE_INVALID` with a sentence naming
`serialise()`, at `connect()`, on the next run. `test/pairing.test.mjs`
drives exactly that sequence through a deliberately naive store.

**What the Windows half actually guarantees, stated precisely.**
Measured on Windows 11 26200, with the Users group granted on the
containing directory so the file genuinely inherits access for other
accounts: after the call the file has exactly one ACL entry, this user.
That is stronger than what is claimed. What is *claimed* — and what
holds from a zero exit code without parsing anything — is that
inheritance is broken and those four well-known SIDs have no access.
`icacls /inheritance:r` was measured to convert inherited entries to
explicit ones when used alone, and to remove them when a `/remove` is
present in the same call; relying on that quirk would be relying on
undocumented option ordering, so the claim is the narrower one.

SYSTEM and Administrators may survive on a machine whose directory grants
them, and that is not worth chasing: an administrator can take ownership
of any file whatever its DACL says.

**Rejected.**
- **A default under `%LOCALAPPDATA%` or `~/.liro`.** Convenient, and it
  is the decision F8 §3 says the SDK does not get to make.
- **Throwing from `toJSON()` so the mistake is louder.** It would make
  `JSON.stringify({ context: { pairing } })` throw inside somebody's
  logger, which takes down a request handler for a logging call. The
  marker is loud enough and lands at `connect()`.
- **Storing the secret in a `#private` field.** Above.
- **Warning rather than failing when the permissions cannot be set.** A
  signing secret in a world-readable file is the outcome this class
  exists to prevent, and SPEC §14.1 names the multi-user machine as
  ordinary rather than exceptional.
- **Removing SYSTEM and Administrators explicitly.** Above.

---

## D-212 — `LiroError.code` is an exhaustive union, and a code newer than this SDK is `UNKNOWN` with the agent's own string beside it

**Date:** 2026-09-09
**Phase:** F8

**Decision.** One error class, `LiroError`, with `code` typed as a union
of every code `internal/errs` declares, plus eight the SDK raises
itself, plus `UNKNOWN`. `agentCode` always carries the exact string that
arrived. A `switch` over `code` with a `never` default is checked by the
compiler, which is what F8 §6 asks for.

**Why one class and not thirty-nine.** F8 §6's own example is
`e instanceof LiroError && e.code === 'CARD_NOT_PRESENT'`. A class per
code would make that read `e instanceof CardNotPresentError`, which is
more to import, more to remember, and no more checkable.

**Why `UNKNOWN` exists.** `PROTOCOL.md` §7 says codes are stable and new
situations get new ones — and adding a code does not change the protocol
version, because a caller written against the old one still works. So an
agent newer than this SDK can answer with a code the union does not
name. The SDK does not guess and does not fold it into `INTERNAL`, which
would mislabel a condition the caller might well know about: `code` is
`UNKNOWN`, `agentCode` is verbatim, and the message says the agent is
probably newer.

**The union is checked against the agent's own source, not against a
list somebody keeps in step.** `test/errors.test.mjs` parses
`internal/errs/errs.go` for every `Code = "…"` constant and requires the
two sets to be equal in both directions — a code the agent can send that
this SDK does not name, and a code this SDK names that the agent does
not declare, are both failures. That is [[D-158]]'s method applied
across the language boundary, and it is the check that stops a new agent
code reaching an integrator as `UNKNOWN` with no sentence.

**Two messages are load-bearing rather than descriptive**, and both have
their own test. `AUTH_FAILED` names the agent's own log, the seven
reasons that log records, and `POST /v2/echo` — because the response
says nothing and always will ([[D-175]]), and those two are what is
left. `PIN_INCORRECT` says never to retry, and why. A third test asserts
that *no other* message claims to know which authentication check
failed, so the SDK cannot start leaking through its own prose what the
protocol withholds.

**Rejected.**
- **An error class per code.** Above.
- **Folding an unknown code into `INTERNAL`.** It reads as "the agent
  broke", which is exactly what `INTERNAL` means and exactly what a new
  code most likely is not.
- **Typing `code` as `string`.** Then `switch` is not checkable, which
  is the one thing F8 §6 asks for.

---

## D-213 — What this SDK retries is a short list with a reason each, and a submission is not on it

**Date:** 2026-09-09
**Phase:** F8

**Decision.** `Transport.send` retries only when the caller marked the
request `retryable`, and only for a failure that left no response at all
or a `5xx`. Three requests are marked: `GET /v2/health`,
`GET /v2/certificates`, `POST /v2/echo`. Everything else is sent once.
Three attempts, 200 ms then 600 ms apart. **Every attempt builds a new
nonce and a new timestamp**, from `crypto.randomUUID()`.

**Why the default is not to retry.** A retry is a decision about what a
repeat costs, and the costs here are not symmetric:

| Not retried | What a repeat costs |
|---|---|
| any `401` | Nothing gained: a request that did not authenticate does not authenticate the second time, and every authentication failure looks identical from the outside ([[D-175]]), so the caller learns nothing new either. |
| `POST /v2/sign`, `POST /v2/sign/pdf` | A submission that timed out may already have created a job. A second one is a hundred documents signed twice, and a second window in front of a person for a batch they have already approved. |
| `POST /v2/pair/request` | A second window on somebody's screen, or `PAIRING_IN_PROGRESS` from the first — and the pairing rate limit counts refused requests too. |
| `POST /v2/pair/confirm` | One of five code attempts, spent for a code that may already have been accepted. |
| `GET /v2/jobs/{id}/result` | It is delivered once and the job is then forgotten. |

**Why a new nonce every attempt.** A reused one is `AUTH_FAILED` — so a
retry meant to recover from a hiccup instead produces the least
diagnosable answer the protocol has. `test/retries.test.mjs` asserts
three distinct nonces and three distinct signatures across three
attempts, against an agent that verifies both.

**Why the backoff is short.** This is a loopback socket. A request that
is going to connect connects in well under a millisecond; the only
things worth waiting out are a listener a moment mid-restart and a
transient `5xx`. F3's TSA client waits 1 s then 3 s ([[D-045]]) because
it is talking to a timestamp authority over the internet, which is a
different question.

**`PIN_INCORRECT` is never retried by anything here**, and the SDK has
no code path that could: it arrives as a job failure, `signPdf` throws,
and nothing above catches it. The README says so in the strongest terms
the language allows, because three wrong entries block a card and
unblocking a national identity card means a visit to the Ministry.

**Rejected.**
- **Retrying everything idempotent-looking.** `GET /v2/jobs/{id}/result`
  looks idempotent and is not, by design.
- **Retrying a submission after a timeout with the same nonce, so the
  agent's replay cache would refuse the duplicate.** It would work,
  and it makes the SDK's correctness depend on the agent's nonce cache
  still holding the entry — a five-minute window that a slow batch can
  outlast, and a coupling the SDK has no business having.

---

## D-214 — The default origin is `"local"`

**Date:** 2026-09-09
**Phase:** F8

**Decision.** `ConnectOptions.origin` defaults to the string `"local"`.

**Why there is a default at all.** F8 §1's target integration is
`LiroBridge.connect({ applicationName: 'Moj ERP' })`, and `origin` is a
required field of both pairing calls. Something has to be sent.

**Why `"local"` and not something more specific.** The origin is shown
**verbatim** on the pairing window so that a person can judge it — SPEC
§6.2's reason is that somebody who sees `http://` where they expected
`https://` must be able to notice. What is truthful to show for a
program on the machine with no web origin of its own is that it is a
program on the machine. Three candidates were weighed and each says
something false or nothing:

- **The application name**, so the window would read "Moj ERP" twice.
  It says nothing the row above it does not.
- **`local:<application name>`**, which the agent refuses outright the
  moment the name contains a space — and "Moj ERP" does. Mangling the
  name to fit would be altering a value on its way to a screen, which
  is the thing §6.2 is about.
- **A synthesised URL** such as `app://moj-erp`. It looks like an origin
  and is not one; a person reading it would be judging a fiction.

`"local"` gives no separation between two applications that both take
the default, and that is fine: what separates them is the `appId` and
the device secret. The origin is for the person, and for an application
that has a real one the README says to pass it.

**Rejected.** All three above, and leaving `origin` required — which
would make F8 §1's three-line target impossible for exactly the callers
it is aimed at.

---

## D-215 — `console.log(bridge)` printed a device secret, and the fix is that a `LiroBridge` is not a path to the store

**Date:** 2026-09-09
**Phase:** F8

**Decision.** `LiroBridge` has its own `util.inspect` implementation and
its own `toJSON()`, both showing the application name, the origin, the
`appId`, the agent version and the protocol version, and nothing else.

**Why — measured, not anticipated.** F8 §4.2 asks for a test that pairs,
exercises every error path and greps every string the SDK produced. The
first run of that test failed, on a path nothing had been written to
guard:

```
the device secret reached the bridge object (as an encoding)
  ...secretStore: MemoryStore { records: Map(1) { 'Moj ERP' =>
  { appId: '…', applicationName: 'Moj ERP', origin: 'local',
    deviceSecret: 'bGlyby1zZGstbGVhay10ZXN0LXNlY3JldC0zMmJ5dGU=' } } }...
```

`StoredPairing` was already careful ([[D-211]]) and printed
`deviceSecret: <32 bytes, withheld>` exactly as intended. The leak was
one object further out: a `LiroBridge` holds the caller's own
`SecretStore`, a store holds device secrets — that is its whole job —
and Node's inspector walks object graphs. One `console.log(bridge)` put
a base64 device secret on screen.

Nothing about that is the store's fault, and nothing about it is
avoidable from the store's side. What is avoidable is this object being
the path to it.

**What this does not claim.** A caller who logs *their own store* still
logs whatever it holds. That is theirs, and the `SecretStore`
documentation says what a store contains. What is fixed is that the SDK
does not hand it to them by accident.

**The general shape, because it is the third time this project has met
it.** [[D-084]] and [[D-062]] both record scoping a secret-scan to the
bytes that are actually meaningful. This is the opposite error: a scan
scoped too *narrowly*, at the object everyone expected to be the risk,
while the risk was one reference away. The test that found it walks
`String`, `util.inspect` at infinite depth with hidden properties and
getters, `JSON.stringify` over every own property name, `message`,
`stack` and the whole `cause` chain, for forty-odd error paths — and
captures everything written to stdout and stderr for the whole run
besides. It also asserts that the same search over the secret itself
*does* find it, so a fixture that could never fail would fail.

**Rejected.**
- **Making `options.secretStore` non-enumerable on the bridge.** It
  works for `inspect` and not for `JSON.stringify` with an own-property
  allow-list, and it is a property of one field rather than a statement
  about what the object shows.
- **Documenting "do not log the bridge".** A rule an integrator has to
  remember, protecting the thing they are most likely to log while
  debugging.

---

## D-216 — Node 18 is the floor, `@types/node` is a development dependency, and no Node type reaches the public surface

**Date:** 2026-09-09
**Phase:** F8

**Decision.** `engines.node` is `>=18.0.0` and `connect()` refuses an
older one with a sentence naming the version it found. The package has
exactly two development dependencies, both pinned to an exact version:
`typescript` and `@types/node`. It has no runtime dependencies, no peer
dependencies and no optional ones, and a test asserts all three are
empty.

Nothing in the generated `.d.ts` files names `Buffer`, `NodeJS.*` or a
`node:` import — a test reads every declaration file and fails if one
does. A document is a `Uint8Array` throughout the public API.

**Why 18.** F8 §2 names it, and it is where `fetch`, web streams and
`crypto.randomUUID` are all present without a flag. The SDK uses all
three.

**Why the version check is at `connect()` and not at import.** A module
that throws while being imported takes down a process at load, before a
`try` can catch it, and the message ends up in a stack trace with no
context. `connect()` is the first thing anybody calls and is already
`async`, so the refusal arrives as a rejected promise a caller can
handle — with `UNSUPPORTED_ENVIRONMENT` and the version it found.

**Why `@types/node` is acceptable and a runtime dependency would not
be.** F8 §2's rule is about what ends up in the integrator's
application: "Every dependency added here is a dependency in the
integrator's application, in a package that handles a signing secret."
A development dependency is not installed transitively — proven, not
assumed: the CI step and `test/package.test.mjs` both install the packed
tarball into an empty project and assert `node_modules` contains
`@liro` and nothing else.

The reason the *public surface* is kept free of Node types is separate
and is the one that matters to a consumer: a `.d.ts` that referred to
`NodeJS.ErrnoException` would make `@types/node` a compile-time
requirement for anybody type-checking against this package.

**Rejected.**
- **Hand-writing declarations for the `node:` builtins to avoid
  `@types/node` entirely.** Considered, and it would work. Rejected as a
  few hundred lines of type surface this project would then own and
  would have to keep true against a Node it does not control — for a
  dependency that ships to nobody.
- **Testing only on the newest Node.** The CI job pins Node 18, because
  a suite that only runs on the newest one is not evidence about the
  version the package says it accepts.

---

## D-217 — `echo()` is on the SDK's surface, and it is the only thing there F8 §5 does not list

**Date:** 2026-09-09
**Phase:** F8

**Decision.** `LiroBridge` has one method beyond F8 §5's list:
`echo(body)`, which is `POST /v2/echo` and returns the canonical string
the agent built plus the body hash.

**Why, given SPEC §0's instruction not to invent requirements.** F8 §6
requires the `AUTH_FAILED` message to point at `/v2/echo`. An SDK whose
error message names an endpoint its user then has to hand-roll — the
canonical string, the four headers, the HMAC — has told them where to
look and left them to build the torch. Fifteen lines close that.

**What it is honestly for.** Not for users of this SDK's happy path:
this SDK builds the canonical string itself and a caller of `signPdf`
will never need it. It is for the two cases where somebody is debugging
across an implementation boundary — writing a second client in another
language beside a working one, or chasing a machine clock — and for
making §6's own advice actionable from inside the package.

It is authenticated like every other call, so it answers only for a
request that already authenticated. It diagnoses a body hash or a path,
never a wrong secret; the agent's log is what covers that half
([[D-176]]).

**Rejected.**
- **Leaving it out and letting the message point at an endpoint the SDK
  does not expose.** Defensible, and it is the reading of SPEC §0 that
  keeps the surface smallest. Rejected because the message is required
  to name the endpoint either way, and a named endpoint with no way to
  reach it is worse surface than a small method.
- **Exposing the raw transport so a caller could build any request.**
  That is a general escape hatch, which is a much larger thing than one
  documented diagnostic, and it would let a caller construct requests
  this SDK's retry and nonce rules do not cover.

---

## D-218 — Five of the seven examples were run against a real agent and a real signature; Java and PHP were not, and the folder says so

**Date:** 2026-09-09
**Phase:** F8

**Decision.** `sdk/examples/` holds seven clients:
`sign-with-sdk.mjs`, `liro-test-client.ps1` (moved from the repository
root, as F8 §8 asks), `sign.py`, `sign.go`, `Sign.cs`, `Sign.java` and
`sign.php`. The README's ~20-line fragments are extracts of those files,
and a test compares each fragment against the region its file marks, so
the two cannot drift.

**Five were run against a real agent, end to end, including a real
signature.** A build of `liro-bridge tray` with the `softtoken` tag, a
loopback socket, a real pairing window on screen, a real consent window,
and a person pressing Approve. Every one of them received a signed PDF
back, and this project's own independent verifier
(`internal/pades/verify`) says every one of the five documents carries a
valid signature — `ByteRangeDigestOK`, `SignatureOK` and
`SigningCertificateOK` all true, no errors:

| Example | Signed document |
|---|---|
| `sign-with-sdk.mjs` | 104 888 bytes, B-B |
| `sign.py` | 104 888 bytes, B-B |
| `sign.go` | 104 888 bytes, B-B |
| `Sign.cs` | 104 888 bytes, B-B |
| `liro-test-client.ps1` | 104 890 bytes, B-B |

B-B because the agent was given no timestamp authority, which since
[[D-067]] is reported rather than silently downgraded.

**Two were not run, and that is stated where somebody opening the folder
sees it.** This machine has no JDK and no PHP. F8 §8 asks for four
languages *and* says every example must be run before it is committed;
on this machine those two instructions cannot both be satisfied for Java
and PHP. Raised with the owner rather than decided quietly, and the
owner's ruling was to ship them with specific labelling.
`sdk/examples/README.md` therefore carries a table saying which examples
were executed, what they produced, and that `Sign.java` and `sign.php`
were written from `docs/PROTOCOL.md` and never executed — not in a
decision entry alone.

**Running them found three defects that reading them did not.** This is
the point of F8 §8's rule and it earned itself immediately:

1. **`sign.py` died on a Serbian name.** `UnicodeEncodeError: 'charmap'
   codec can't encode character 'ć'` — printing `Milovanović` to a
   Windows console under cp1252, on the certificate listing, first run.
   Every real certificate has such a name; the example would have failed
   for every user of this product. Fixed by reconfiguring `sys.stdout`,
   and the same one-line fix applied to `Sign.cs` and `Sign.java` where
   the symptom is question marks rather than a crash.
2. **`Sign.cs` looked for the discovery file in the wrong place.** It
   used `Environment.GetFolderPath(SpecialFolder.LocalApplicationData)`,
   the shell's known folder; the agent writes to the `LOCALAPPDATA`
   *environment variable* (`internal/platform/paths.go`). The two are
   not always the same path, and the example found nothing. Fixed, and a
   test now asserts all six hand-written clients read the environment
   variable.
3. **`Sign.cs` needed C# 7.1 for an `async Main`.** The compiler that
   ships inside every Windows installation is C# 5. Rewritten with a
   synchronous entry point that waits on the asynchronous one — which is
   also the better example, because a Serbian ERP is as likely to be on
   .NET Framework as on .NET 8.

`sign.go` and `sign.php` write UTF-8 straight out, which a legacy
console renders as mojibake rather than refusing. Neither carries a
platform-specific workaround: `chcp 65001` is the console's business,
and the examples' subject is the protocol. Said in the folder's README
rather than fixed.

**Rejected.**
- **Leaving Java and PHP out entirely**, following "an example that does
  not run is worse than no example" literally. Put to the owner as one
  of three options; they chose to ship them labelled, and a Java
  integration is a real audience for this product.
- **Installing a JDK and PHP to run them.** Offered and declined: it
  cuts against leaving the machine as it was found, for two files.
- **Writing the README fragments by hand.** They would drift from the
  files beside them, which is [[D-183]]'s finding, and a fragment that
  has drifted is worse than none: the reader copies it, it does not
  work, and they have lost more than writing it themselves would cost.

---

## D-219 — What F8 changed about where defects were looked for, and the one gap this phase leaves

**Date:** 2026-09-09
**Phase:** F8

**Decision, recorded because the phase turned on it.** Five defects were
found in this phase. **Two were found by a test that had just been
written to look for exactly that thing, and three were found by running
a program against a real agent.** None was found by a test that already
existed, because none of this existed before.

| Found by | Defect |
|---|---|
| the secret-grep test's first run | `console.log(bridge)` printed a device secret out of the caller's own store ([[D-215]]) |
| the ACL test, run against a directory that granted `Users` | (none — the fixture was built to be capable of failing and the code passed it) |
| running `sign.py` against the real agent | a Serbian name killed it, on the certificate listing ([[D-218]]) |
| running `Sign.cs` against the real agent | it read the shell's known folder and looked in the wrong place ([[D-218]]) |
| compiling `Sign.cs` with the compiler Windows ships | `async Main` is C# 7.1 ([[D-218]]) |

The first is the one worth keeping. Every test in the suite passed
before it was written, including the ones about `StoredPairing`, which
was careful and correct. The leak was one object reference further out
than anybody had looked — and it was found because F8 §4.2 asked for a
*grep of every string the SDK produced*, not for a check of the object
everyone expected to be the risk. **A secret-scan scoped to the thing
you thought of is a scan that finds what you thought of.**

The three from running are [[D-161]]'s and [[D-122]]'s lesson in a new
language: a green suite is not evidence about what a program does when
somebody runs it. What is new is only the shape of the evidence — this
phase's fake agent (`test/fake-agent.mjs`) speaks the protocol from the
document with its own HMAC and its own canonical string, deliberately
sharing no code with the SDK ([[D-044]]'s rule for verifiers), and it
still could not have caught any of the three, because none of them is
about the protocol.

**The gap this phase leaves, stated rather than glossed.** The
multi-document path was proved against a protocol-accurate fake agent at
two and three documents, and against the *real* agent at one. Every
batch needs a person to approve it, and asking for six approvals had
already been asked for once; a hundred-document batch through the real
agent is the case an ERP actually has, and it has not been run. What
stands in for it: `internal/api`'s and `internal/jobs`' own tests
already sign a hundred documents through the real runner (F7), and the
SDK's count and ordering checks — `SignedDocument[]` never closing up
over a failure, the returned count matching the sent count — are
asserted at 2 and 3 against an agent that verifies every signature.

**Rejected.**
- **Reporting the five as ordinary bugs without this entry.** Four
  previous entries record the same lesson about looking rather than
  asserting ([[D-087]], [[D-122]], [[D-161]], [[D-172]]); what this one
  adds is that a security check's *scope* is part of its fixture, and
  that scoping one to the obvious object is how it passes while the
  thing it protects is on screen.

---

## D-220 — Three things `docs/PROTOCOL.md` does not say that an implementation needs; recorded rather than changed

**Date:** 2026-09-09
**Phase:** F8

**Decision.** F8 forbids changing the agent and asks for anything in
`PROTOCOL.md` that turned out to be wrong, missing or misleading when
something other than PowerShell tried to use it. Three things, all
gaps rather than errors, none of them fixed here because a protocol
document is not this phase's to edit.

**1. A failed document's shape on `/v2/sign/pdf` is undocumented.**
§5.1 is explicit about the digests path: `signatures` has one entry per
digest, a failure is `null` in place, and the array never closes up.
§5.2 shows only a fully successful `documents` array and an empty
`failures`, and never says what an entry looks like for a document that
did not sign. The agent omits `content` and `achievedLevel` and adds a
`failures` entry with the index — read out of
`internal/api/jobhandlers.go`'s `signedDocumentBody`, not out of the
document. An integrator who assumed `content` is always present would
write `Buffer.from(doc.content, 'base64')` and get a crash on the one
path that matters. **Suggested:** one sentence in §5.2 and a `failures`
entry in its example.

**2. `minimumClientVersion` has no stated format.** §4.1 says to compare
your own version against it and tell the person to update the agent if
yours is lower. It does not say the comparison is semver, and `"0.0.0"`
only suggests it. This SDK carries its own version and does not compare,
because a comparison rule this SDK invented would be a rule the agent
had not agreed to. **Suggested:** say "semantic version, compared by
precedence", or say what else it is.

**3. §4 gives only the Windows discovery path.** SPEC §14 gives all
three, and they differ from what a naive reading would guess on Linux
(`$XDG_RUNTIME_DIR/liro/`, not `~/.config`). The SDK implements SPEC
§14's three. Harmless today, since the agent is Windows-only until F12;
worth a line when it is not.

**A fourth thing, which is a compliment rather than a gap.** §2.5's
paragraph about not sending `Content-Type` on a request with no body is
the single most valuable sentence in the document for somebody writing a
client, and it was written because a real .NET client could not obey the
opposite rule ([[D-197]]). Two of the seven examples in `sdk/examples/`
would have been wrong without it.

**Rejected.**
- **Fixing §5.2 while here.** F8 is explicit: "no changes to the agent's
  own behaviour. If the SDK cannot do something because the protocol
  will not let it, that is a finding to report — not a licence to change
  the protocol." A documentation gap is the same shape of thing, and the
  document is the specification this phase implements *against*.
- **Guessing a semver comparison and implementing it.** It would be
  right, and it would be a rule this SDK invented on behalf of an agent
  that has not stated one — which is how two implementations come to
  disagree about the same field.

---

## D-221 — `npm-cli.js` sits beside the node binary only on Windows; the packaging test assumed that everywhere, and only the Windows job ever ran it

**Date:** 2026-09-09
**Phase:** F8

**The defect.** `sdk/typescript/test/package.test.mjs` runs npm as a
script through this Node rather than through a shell — `execFileSync('npm',
args, { shell: true })` concatenates its arguments into a command line
without escaping them, which is not a habit to have in a repository about
signing. That part was right. What was wrong was how it found the script:

```js
const NPM_CLI = join(dirname(process.execPath), 'node_modules', 'npm', 'bin', 'npm-cli.js');
```

under a comment that stated as fact that "`npm-cli.js` ships beside the
node binary on every installation". It does not. There are two layouts:

| Platform | node | `npm-cli.js` |
|---|---|---|
| Windows | `<prefix>\node.exe` | `<prefix>\node_modules\npm\bin\npm-cli.js` |
| POSIX, including `actions/setup-node` on `ubuntu-latest` | `<prefix>/bin/node` | `<prefix>/lib/node_modules/npm/bin/npm-cli.js` |

On POSIX the constant therefore resolved to
`<prefix>/bin/node_modules/npm/bin/npm-cli.js`, one directory too deep and
on the wrong branch. `execFileSync` failed with `MODULE_NOT_FOUND`.
104 of the 105 tests passed; the one that failed was *installing the
repository the way the README says produces a working import* — the one
test that proves the whole packaging argument, and the reason the
`sdk-typescript` job exists at all. Because the job died there, the two
steps after it — the runtime-dependency check and the pack-and-install
check at the repository root — never ran either.

**The fix.** Resolve `npm-cli.js` instead of assuming a layout, in three
steps, and say where you looked if none of them is there:

1. `process.env.npm_execpath`, when it is set and names an existing `.js`
   file. This is npm's own answer to the question, and CI runs the suite
   through `npm test`, so it is set exactly where it matters.
2. The Windows layout.
3. The POSIX layout.

Failing all three, throw an error naming every path that was tried. The
`MODULE_NOT_FOUND` this replaces named a path that appeared nowhere in
the test's own output, assembled from a constant nobody had read; a
resolution failure has to say where it looked.

**Why this was invisible, which is [[D-219]]'s lesson one layer out.**
Before the `sdk-typescript` job existed, this suite had run in exactly
one place: a Windows development machine. It was written there, run
there, and reviewed there, and every one of those observations was
consistent with a constant that is wrong on every other operating
system. The first time it ran anywhere else, it failed. **A check that
only ever runs where it was written is not evidence about where it will
run.** D-219 recorded that a secret-scan scoped to the object you
thought of finds what you thought of; this is the same defect along the
axis of *platform* rather than *scope*. What makes it worth its own
entry is that the check in question was the packaging check — the one
whose entire job is to answer "does this work somewhere other than
here?" — and it was the one carrying a here-only assumption.

**A gap this leaves, stated rather than glossed.** The repository's
`windows` job runs Go only: `go vet` and `go test -tags softtoken`. No
CI job runs the TypeScript suite on Windows. Both layouts are now
resolved and both were verified by hand for this entry, but a future
Windows-only regression in `sdk/typescript` would be found by a
developer, not by CI — which is the same shape of gap this entry is
about, pointing the other way. Adding a Node step to the `windows` job
would close it and is not done here.

**Closed, 2026-09-09 (F9).** The `windows` job now runs the SDK suite
too: `actions/setup-node@v4` pinned to Node 18 — the version
`sdk-typescript` pins, so the two jobs test the same Node — then
`npm ci`, `npm run check-build` and `npm test`, in `sdk/typescript`.

Pinned rather than `latest`, for the reason the `golangci-lint` comment
in `ci.yml` already gives: `latest` means CI's verdict can change
overnight with no commit, so a new rule arrives as a red build on an
unrelated change, at the worst possible moment for diagnosing it.

Folded in here rather than opened as its own entry because it is the
same finding. This entry's own sentence is that a check which only runs
where it was written is not evidence about where it will run, and the
gap it left was that sentence pointing at itself: the Windows branch of
`npm-cli.js` resolution — the branch this entry added — was exercised by
a person and the POSIX branch by CI. It is now exercised by both.

Verified before pushing rather than by reading the runner's log
afterwards: the three steps exactly as written were run on this Windows
machine against `sdk/typescript` — `check-build` reports "dist/ matches
src/", `npm test` reports 105 passed and 0 failed. Node here is 24, not
the 18 the job pins, so what this proves is the steps' own shape on
Windows; the first `windows` run of the job on Node 18 is what closes
the rest.

The two remaining `sdk-typescript` steps — the runtime-dependency check
and the pack-and-install at the repository root — deliberately stay on
`ubuntu-latest` alone. Neither has an operating system in it: one reads
`package.json`, and the other packs a tarball and imports it, which is
what an integrator does on whatever they run. Duplicating them would buy
a second copy of the same answer on a slower runner.

**Verified, on both operating systems and through every branch.**

| Where | What ran | Result |
|---|---|---|
| Windows, Node 24.11.1 / npm 11.6.2 | all four `sdk-typescript` steps, transcribed from `ci.yml` | `check-build` green, 105/105, no runtime dependencies, `esm ok` / `cjs ok` |
| Linux (x86-64, Node 18.20.8 / npm 10.8.2 — the Node the CI job pins) | the same four steps | `check-build` green, `# pass 105 / # fail 0`, no runtime dependencies, `esm ok` / `cjs ok` |

Each resolution step was reached on purpose rather than incidentally:
`npm_execpath` on both, since `npm test` sets it; the Windows layout by
running the suite on Windows with `npm_execpath` unset; the POSIX layout
by running it on Linux with `npm_execpath` unset, where
`<prefix>/bin/node_modules` does not exist, so nothing but the third
candidate can have answered. The failure path was exercised too, by
running the suite under a `node` copied into a directory with no npm
beneath it: it names both paths it tried and stops, which is the whole
point of the change.

One honest note about the evidence: the Linux leg ran on a musl x86-64
Linux rather than on `ubuntu-latest`'s glibc, because no glibc Linux was
available on the machine this was fixed on. The axis under test is the
directory layout, which is the same on both — `<prefix>/bin/node` and
`<prefix>/lib/node_modules/npm/bin/npm-cli.js`, exactly what
`actions/setup-node` unpacks — and the two steps that had never run
before, the runtime-dependency check and the root pack-and-install, ran
there and passed. The first `ubuntu-latest` run of the job is still the
thing that closes this.

**Rejected.**
- **`shell: true`.** The comment above the constant already explains why
  not, and it is still the reason: unescaped concatenation of paths into
  a command line, in this repository, on a test whose arguments include a
  temporary directory name.
- **Skipping the test on POSIX, or on any platform.** The test's entire
  claim is that an integrator can install this package. An integrator on
  Linux is the common case, not the exotic one; a skip there would make
  the suite quieter and the claim smaller without making it truer.
- **Hardcoding the POSIX layout and keeping Windows as the special
  case.** That is the same defect with the platforms exchanged. Both
  layouts are guesses; `npm_execpath` is an answer, and it goes first.
- **Calling `npm` off `PATH` with `execFileSync` and no shell.** It works
  on POSIX and fails on Windows, where `npm` is `npm.cmd` and needs a
  shell to be found — which is the rejected option above wearing a
  different hat.

---

## D-222 — There is one `sign` and it shows the consent window; the path with no window keeps its own name and only a softtoken build has it

**Date:** 2026-09-09
**Phase:** F9

**The defect, and its age.** `liro-bridge sign` signed without asking
anybody. The consent window appeared only behind `--interactive`.

Measured against the binary rather than read out of the source. A build
of the commit this phase started from (`58a6a23`), with the soft token
configured and `LOCALAPPDATA` pointed at a scratch directory:

```
> old-bridge.exe sign --in ugovor.pdf --thumbprint E0BEC835... --on-tsa-failure b-b
Potpisano: ...\ugovor-signed.pdf
  Nivo: B-B
  Sertifikat: …B4AA99A9
exit=0
```

One process invocation, one signed document, no window, no pairing, no
origin binding and no human. Any process running as that user could do
it.

The code is F3-era. It predates F5's consent window by three phases and
was never reconciled with either of the two rules that govern it, and no
entry in this log had reconciled it either:

- **SPEC §18.2**: "No signature without human approval. No flag, no
  configuration, no header bypasses the consent screen."
- **SPEC §4.3**: "All four entry points go through the same consent
  screen, the same session, the same audit log."

**A second thing the same run establishes.** After that signature the
scratch `%LOCALAPPDATA%\Liro` held `logs\` and `tsl-cache.xml` and **no
`audit\` directory at all**. `internal/cli.RunSign` never touched the
audit log — it had no `*audit.Store` anywhere in it. So `sign` was not
only signing without approval, it was signing without a record, against
SPEC §6.7's rule that the log records every signature. The consent fix
closes that as a side effect: the window flow records every batch,
including a refused one.

**Decision.**

*(a) There is one `sign`, in every build, and it shows the window.*
`run`'s `sign` branch calls `runSignCommand` — what `runSignInteractive`
was — unconditionally. `--interactive` is not a distinction any more and
`parseSignArgs` refuses it with a sentence saying so rather than
accepting and ignoring it: a flag that does nothing advertises a
distinction that does not exist, and this project deletes surface like
that rather than leaving it as a trap ([[D-132]], [[D-146]], [[D-151]],
[[D-175]]).

*(b) The path with no window survives only under the `softtoken` build
tag, under its own name.* `internal/cli/sign.go` is now
`internal/cli/sign_noconsent.go`, carrying `//go:build softtoken`, and
`RunSign` is `RunSignWithoutConsent`. `cmd/liro-bridge` reaches it
through a `sign-no-consent` subcommand that exists only in the same
build (`noconsent_softtoken.go`; `noconsent_release.go` is what a
release compiles instead and knows no command name at all).

*(c) A release binary contains neither.* Proved by reading the symbol
table, not by trusting the tag.

**Why `sign-no-consent` rather than letting a tagged `sign` mean
something different.** Both arrangements keep the path out of a release
binary, and the second would have left CI's own command line untouched.
It was rejected for the reason [[D-221]] recorded four days earlier: a
check that only runs where it was written is not evidence about where it
will run. If a tagged `sign` meant "sign with no window", the regression
test for this very defect could only run in an untagged build — and the
Windows CI job builds the suite **with** the tag, so the one automated
check standing between this defect and its return would never have run
in CI at all. With `sign` meaning the same thing in every build, the
test is untagged, runs in both configurations, and runs on the Windows
runner.

**Why the path survives at all.** CI signs a PDF and verifies it with
OpenSSL and this project's own independent verifier on every push (SPEC
§16.4, F3's exit condition) and must keep doing that with no human
present. That is a test build's need, so it lives behind the tag that
already means "this is a test build" (SPEC §16.6). No flag was added: a
`--no-prompt` in this program is a defect whatever motivates it, and a
flag would also be reachable in a release binary, which is the whole
thing being prevented.

The renamed command still satisfies F3's exit condition end to end,
checked rather than assumed: `sign-no-consent --in ... --out ...
--thumbprint ... --force --on-tsa-failure b-b` against the soft token
produced a document whose signature `internal/pades/verify` accepts
(`ByteRangeDigestOK`, `SignatureOK`, `SigningCertificateOK`) and which
`openssl cms -verify -binary` reports as "CMS Verification successful".

**What this broke.** Everything that was relying on `sign` not opening a
window, which is the flag surface the two paths never shared. A release
binary's `sign` now takes `--in` and `--force` and nothing else. Gone
from it: `--out`, `--thumbprint`, `--level`, `--tsa`,
`--tsa-client-cert`, `--on-tsa-failure`, `--reserve`,
`--max-revocation-size`, `--resign`, `--stamp`, `--stamp-position`,
`--stamp-xy`, `--stamp-page`, `--stamp-reference` and
`--stamp-show-document-id`.

Each of those has a home already — the certificate is chosen in the
window, the level, the timestamp authority and the output folder are
`config.json`, the stamp is the method screen ([[D-146]], [[D-151]]) —
except two, stated rather than glossed: **`--out` and `--resign` have no
equivalent in the window flow.** `--out` is nearly answered by the
output-file question ([[D-104]]) and the chosen output folder
([[D-126]]), but neither is "write this one document to exactly this
path"; `--resign` is answered by J-3's own question ([[D-164]]), which a
person answers rather than a script. Nobody has asked for either back,
and the owner's ruling for this phase is that there is no CLI contract
for legacy callers — an integrator who is not on Node uses the protocol
directly. If one of them is wanted, it is a flag on the window flow and
it will arrive with a reason.

`sign --interactive` in any script is now an error with a sentence
saying why.

**One mechanical consequence, recorded so it is not read as scope
creep.** `caCertificatesFromTSL` lived in `cmd/liro-bridge/main.go` with
no build constraint. Its two callers are now a Windows-only file and a
softtoken-tagged one, so in a plain non-Windows build it became a
function nothing could use, and `golangci-lint`'s `unused` said so
against the `darwin` and `linux` views. `//nolint` is forbidden this
phase and would have been the wrong answer anyway ([[D-111]]): it is a
pure question about a Trusted List with no wiring in it, so it is now
`tsl.List.CACertificates()`, in the package whose job is answering
questions about Trusted Lists.

**Tests.**

- `TestTheSignCommandOpensAWindowAndSignsNothingUntilItIsAnswered`
  (`cmd/liro-bridge`, no build tag) drives `run([]string{"sign", "--in",
  ...})` — the dispatch is half of what broke, so it goes through the
  same function `main` does — waits for a *visible* window of this
  process's own class that was not open before, asserts no output file
  exists at that moment, closes the window with `WM_CLOSE` (a window
  message, never synthetic input — [[D-094]]), and asserts none exists
  afterwards either. Confirmed to fail against the commit this phase
  started from, in a worktree at that commit: "no new window titled
  \"Liro Bridge\" appeared within 1m30s", with the old path's
  `--thumbprint je obavezan.` on stderr beside it. Six consecutive runs,
  three in each build configuration, pass.

  Waiting for the window to be *visible* rather than merely to exist is
  not decoration. Built the first way it hung once under the softtoken
  tag and passed without it: `ui.NewWindow` creates the frame about two
  seconds before it shows it, and a `WM_CLOSE` posted into that gap is
  dispatched into a half-built window that then waits for a navigation
  which never completes. Waiting for an observable state rather than for
  a moment is [[D-201]]'s rule, arriving in a new place.
- `TestTheNoConsentPathIsAbsentFromAReleaseBinary` builds the agent both
  ways and reads each symbol table with `go tool nm`, requiring
  `internal/cli.RunSignWithoutConsent` and
  `internal/keysource/softtoken` to be absent from one and present in
  the other. Both directions, for the reason [[D-031]] gives: a check
  that only looks for absence passes for the wrong reason the moment the
  build tag itself breaks.
- `TestSignRejectsInteractiveRatherThanIgnoringIt` pins (a)'s second
  half.

CI's binary-inspection step is extended to name
`internal/cli.RunSignWithoutConsent` alongside the soft token, so the
absence is proved on every push rather than only by a test somebody
might remember to run.

**Rejected.**
- **A `--no-prompt`, `--unattended` or `--yes` flag.** The phase
  document's own instruction, and correct regardless of it: a flag is
  reachable in a release binary, which is the entire thing being
  prevented, and SPEC §18.2 says no flag bypasses the consent screen in
  as many words.
- **Letting `sign` mean the windowed path in a release build and the
  headless one under the tag.** Above: it would have put the regression
  test where CI cannot run it.
- **Keeping `--interactive` accepted and ignored.** It costs nothing to
  accept and it tells a person there are two behaviours when there is
  one.
- **Keeping the removed flags by routing them into the window flow.** A
  bigger build than this phase asked for, and half of them are questions
  the window exists to ask.
- **Fixing the missing audit record separately, as its own change.** It
  is the same code and the same fix: the path that recorded nothing is
  gone, and the path that replaced it has recorded every batch since F5.

---

## D-223 — Nothing stops two processes appending to one audit chain, and the result is a log that reports tampering for ever; measured, reported, not fixed

**Date:** 2026-09-09
**Phase:** F9

**The question.** [[D-182]] noted that `sign --interactive` opens its own
pairing store because it is a different process. The audit log is the
same shape of thing and a much worse one to get wrong: two processes
appending to one hash chain would destroy the chain's only property. So
— what happens today when `sign` runs while the tray agent is running in
the same session?

**Where the guard would be, and what is actually there.** `audit.Store`
has exactly one (`internal/audit/store.go`):

```go
// Store guards its own directory with a mutex: Append must read the
// last entry and write the new one as one atomic-from-this-process
// operation, or two concurrent batches finishing at once could both
// compute the same PrevHash and silently fork the chain.
type Store struct {
	dir string
	mu  sync.Mutex
}
```

A `sync.Mutex` on the `Store` value. Its own comment says what it is for
and says the scope out loud — *atomic-from-this-process*. There is no
file lock, no named mutex, no `O_EXCL`, no lock file and no cross-process
anything anywhere in `internal/audit`: `NewStore` is `os.MkdirAll`, and
`Append` reads every chain file, takes the last entry, computes
`Sequence` and `PrevHash` from it, then opens the current file
`O_APPEND|O_CREATE|O_WRONLY` and writes one line.

Nothing else in the project covers it either. The only cross-process
guard this program has is `platform.ShellBatchLeaderName`
(`internal/platform/singleinstance_windows.go`), a `Local\` named mutex
with exactly one caller — `runShellVerb`, deciding which of twenty
Explorer invocations opens a window ([[D-119]]). Nothing prevents a
second agent process from starting, and both processes resolve the same
directory: `newAuditStore` is
`filepath.Join(platform.ConfigDir("windows", platform.OSEnv), "audit")`,
which is per user, so a tray agent and a `sign` process in one session
share it by construction.

**So: nothing guards it.** Said plainly rather than inferred.

**Measured, two ways, because the mutex being per-`Store` makes the two
identical.** A throwaway harness inside the module, created and deleted
in the same session ([[D-100]]).

*Two `*audit.Store` values over one directory, in one process:*

```
entries=2 chains=1 storeOK=false brokenAt=1
  seq=0 app=store-1  prev=(none) hash=3b0995ab
  seq=0 app=store-0  prev=(none) hash=8000e566
```

*Two real OS processes, each with its own `Store`, released at the same
instant:*

```
entries=2 chains=1 storeOK=false brokenAt=1
{"sequence":0,...,"application":"tray","prevHash":"","hash":"5e0247a4…"}
{"sequence":0,...,"application":"sign","prevHash":"","hash":"22e0afec…"}
```

Both wrote `sequence: 0`. Both wrote an empty `prevHash`. `Verify`
reports `brokenAt: 1` — the second line — because its `PrevHash` does
not match the first line's `Hash`, which is exactly what an altered log
looks like.

**It is permanent, not a one-off.** Seeded with one entry, forked, then
appended to normally:

```
entries=4 chains=1 storeOK=false brokenAt=2
  seq=0 seed     prev=(none)   hash=0abd36d9
  seq=1 store-0  prev=0abd36d9 hash=8f610448
  seq=1 store-1  prev=0abd36d9 hash=d146ee2a
  seq=2 later    prev=d146ee2a hash=319c2ed1
```

The later entry chains from whichever line was written last, so the log
goes on growing and goes on verifying as tampered from the fork onward.
[[D-166]]'s recovery does not help and is not meant to: it fires when a
chain's last entry cannot be *read*, and here every line reads
perfectly. The person is told nothing at the time, and months later the
export's integrity check says the log was altered at entry 2 — about a
race, in a file whose entire value is that it cannot be altered without
saying so.

**Reproducing it takes four lines**: two `audit.NewStore` calls on one
directory, an `Append` on each from two goroutines released together,
then `Verify`. Recorded here rather than committed, so nobody has to
rediscover the shape.

**How likely, stated rather than implied.** It needs two batches
finishing inside the same few milliseconds, in two processes, in one
session. That is not the ordinary case and it is not exotic either: F6's
Explorer integration starts one process per selected file ([[D-119]]),
F7's protocol path signs from the tray while a person may be signing
from `sign`, and this phase makes `sign` write an audit entry where
before it wrote none ([[D-222]]) — so the exposure is larger today than
it was yesterday, which is exactly why the question was asked now.

**Not fixed here, and deliberately not.** The remedy is a decision with
at least three defensible shapes — a `Local\` named mutex held across
`Append`, a `LockFileEx` byte-range lock on the chain file, or one
process owning the log and the others handing entries to it — and the
third is a change to how this program is arranged, not to a function.
Choosing one silently inside a phase that was asked to establish the
fact is how a design nobody chose ships ([[D-201]]'s own reason for
leaving a layout defect to the owner). The phase asked for the fact,
said not to fix it silently, and said not to assume it is fine. It is
not fine.

**Rejected.**
- **Assuming the per-process mutex was enough because its comment reads
  like a guarantee.** It says "atomic-from-this-process" and means it.
  The measurement is what settles it, and this project has recorded five
  times what reading instead of measuring costs ([[D-087]], [[D-122]],
  [[D-161]], [[D-172]], [[D-219]]).
- **Fixing it with a named mutex while in here.** Probably the right
  answer. Not this phase's to choose, and a lock added without a story
  for what happens when the holder dies is a program that will not sign.

---

## D-224 — TSA credentials leave the command line; no flag on any command accepts a secret

**Date:** 2026-09-09
**Phase:** F9

**Decision.** `--tsa-user`, `--tsa-password` and
`--tsa-client-cert-password` are removed. The signing command's
timestamp credentials come from `config.Config` — `TSAUser`,
`TSAPassword`, `TSAClientCertPassword`, the fields [[D-091]] added —
carried in as `cli.TSACredentials` by the one place that knows what a
configuration is. `--tsa` (a URL) and `--tsa-client-cert` (a file path)
stay: neither is a credential.

**Why.** On Windows a process's full command line is readable by every
other process running as that user, and by anything running as an
administrator: Task Manager's "Command line" column, `Get-CimInstance
Win32_Process`, any process lister. A password given as an argument is
therefore published to the machine for as long as the process runs, and
it survives afterwards in the shell history and in whatever scheduled
task or batch file invoked it. [[D-091]] had already moved these values
into configuration; the flags stayed beside them, so the safe path
existed and the unsafe one was still the documented one.

**No `--pin`, ever, in any form.** Nobody has asked for one; somebody
will. SPEC §6.5 is the standing answer and it is not about command
lines: the card caches the PIN in its own state independently of which
process is talking to it, so the PIN is not an access-control boundary
between applications at all. A PIN on a command line therefore buys no
security and spends a great deal — and [[D-025]] already made it
impossible for a PIN to be a *field* anywhere in `internal/keysource`,
`internal/keysource/softtoken` or `internal/signing`. This is the same
rule one layer out.

**The test.** `TestNoFlagOnAnyCommandAcceptsASecret`
(`internal/cli/flagsecrets_test.go`) parses every `.go` file in
`internal/cli` and `cmd/liro-bridge` and collects the first argument of
every `*flag.FlagSet` registration, then fails on any name containing
`pin`, `password`, `passwd`, `passphrase`, `secret`, `credential`,
`apikey` or `api-key`. Reading the source is this project's method for a
property about what is *declared* — [[D-025]]'s "no PIN field anywhere",
[[D-158]]'s `AllCodes`, [[D-185]]'s "nothing binds anywhere but
loopback" — because a flag registered in a file no test happens to
exercise is invisible to anything else. Files are parsed individually
rather than as a package so that build constraints are ignored: the
signing path with no consent window is behind the `softtoken` tag
([[D-222]]) and its flags are still flags.

`TestTheFlagSecretRuleWouldActuallyFire` is the other half, because a
word list nothing could match passes for ever: it requires `--pin`,
`--card-pin`, `--tsa-password`, `--tsa-client-cert-password` and
`--api-key` to be caught, and `--tsa`, `--tsa-client-cert`,
`--thumbprint`, `--stamp-show-document-id`, `--resign` and `--all` not
to be. Confirmed against a deliberately reintroduced `--tsa-password`
and `--pin` on `certs`: both are named, with their file and line.

**"user" is deliberately not a secret word.** A username is not a
secret, and a list that caught it would catch
`--stamp-show-document-id` and every future flag with "use" in it.
`--tsa-user` was removed with the other two for a different reason — it
is half of a credential pair whose other half now comes from
configuration, so a flag for it alone would configure nothing — and a
second assertion pins all three by name, which is what actually keeps
them removed.

**Rejected.**
- **Keeping `--tsa-password` and warning about it.** A warning is a
  thing to read once and route around; the value is on the command line
  either way.
- **Reading the credentials from the environment instead.** Better than
  a command line and worse than configuration: an environment variable
  is inherited by every child process, and this program already has one
  place a credential belongs.
- **A rule over flag *usage strings* rather than names.** It would catch
  `--tsa-client-cert`'s own description, which names a certificate
  rather than a credential, and it would miss a flag called `--p`
  documented as "the password".
- **Removing `--tsa` and `--tsa-client-cert` too, so the whole timestamp
  configuration comes from one place.** Tidier, and beyond what was
  asked: a URL and a path are not secrets, and the phase named three
  flags.

---

## D-225 — Two promises removed from SPEC: the .NET SDK, and F9 as a CLI contract for legacy systems

**Date:** 2026-09-09
**Phase:** F9

**Decision.** Two edits to `docs/SPEC.md`, both removing something the
project had promised and no longer intends. No rule is added.

*(a) §5's repository layout.* `sdk/dotnet/` is gone, replaced by what is
actually there:

```
├── sdk/
│   ├── typescript/                # the only first-class SDK
│   └── examples/                  # one client per language, protocol only
```

*(b) §19's F9 row.* It read

| **F9** | CLI for legacy systems; .NET SDK | A Delphi program signs via `exec` |

and now reads

| **F9** | One `sign`, and it shows the consent window; no credential on any command line | A release binary contains no signing path that skips the consent screen, proved by inspecting the binary |

**Why the .NET SDK goes.** F8 §0 settled it and the owner has restated
it: TypeScript is the only first-class SDK, and other languages keep the
example-only status F8 §8 gave them. `sdk/examples/` holds seven clients
— `sign-with-sdk.mjs`, `liro-test-client.ps1`, `sign.py`, `sign.go`,
`Sign.cs`, `Sign.java`, `sign.php` — written against `docs/PROTOCOL.md`
and, for five of them, run end to end against a real agent ([[D-218]]).
A directory in the layout for a package nobody is building is a promise
the repository makes on every reading of §5; a .NET integrator's answer
is `Sign.cs` and the protocol, and that answer exists.

**Why the F9 row goes.** Both halves of it are gone. The .NET SDK by F8
§0, above. The CLI contract for legacy callers by the owner's decision
this phase: an integrator who is not on Node uses the protocol directly.
Keeping "A Delphi program signs via `exec`" as an exit condition would
be worse than stale — it is the exact shape of thing [[D-222]] has just
removed, a program obtaining a qualified signature by running a command,
with no window and nobody at the machine. The row it is replaced with is
what the phase actually was, and its exit condition is one a machine
checks.

**Why these two and nothing else.** The phase said these are the only
SPEC edits it makes, and it is right to bound it: SPEC is "the rules
that never change" (§0), and a phase that edits it freely is a phase that
can soften a constraint by rewording it. Both of these remove a promise
rather than adding a rule, which is the one direction that cannot make a
future implementation wrong.

**One thing left inaccurate, deliberately, and reported instead.** SPEC
§4.3's table of the four entry points still describes the CLI row's
caller as "Scripts, legacy systems" with the note `liro-bridge sign --in
x.pdf --out y.pdf`. After [[D-222]] a release binary's `sign` has no
`--out`, and "legacy systems" is the audience the owner has just said
there is no contract for. It is a Notes column rather than a rule,
changing it is a third SPEC edit this phase was told not to make, and
the sentence immediately above it — "All four go through the **same**
consent screen, the same session, the same audit log" — is the
load-bearing part and is now true for the first time. Recorded here for
whoever makes the next SPEC edit.

**Rejected.**
- **Leaving `sdk/dotnet/` and marking it "not built".** A layout is a
  description of the repository; a directory that does not exist is not
  a description.
- **Rewriting SPEC §4.3's CLI row while in here.** Above.
- **Replacing the F9 row with nothing, since a phase's row is history.**
  §19 is a map of what each phase is for and every other row says what
  its phase did; a blank one would read as a phase that did nothing.

---

## D-226 — Three gaps closed in `docs/PROTOCOL.md`, and a fourth found while closing them

**Date:** 2026-09-09
**Phase:** F9

**Decision.** [[D-220]] recorded three things the protocol document does
not say that an implementation needs, and deliberately left them: F8 was
forbidden from editing the specification it was implementing against.
This phase is not, so all three are now said. Nothing about the agent
changed; every sentence describes what the code already does, read out
of the code.

*(a) §5.2 now says what a failed document looks like.* §5.1 was explicit
for the digests path — one entry per digest, `null` in place, the array
never closes up — and §5.2 showed only a fully successful `documents`
array. Read out of `internal/api/jobhandlers.go`'s `signedDocumentBody`:
a failed document keeps its `name` and has neither `content` nor
`achievedLevel` (both `omitempty`), and its position in the request
appears in `failures`. The example now shows a two-document batch with
one failure, and the prose says the thing an integrator actually needs
told: `content` is not a field to read unconditionally, because
`Buffer.from(doc.content, 'base64')` over every entry crashes on exactly
the path that matters.

*(b) `minimumClientVersion` has a stated format.* It is a semantic
version (semver 2.0.0) compared by precedence, with pre-release and
build metadata not used. Also stated: the comparison is the caller's.
The agent publishes the number (`api.MinimumClientVersion`, `"0.0.0"`)
and enforces nothing with it — refusing a client at the agent would
refuse it before the person could be told anything useful.

*(c) §4 gives all three discovery paths.* It gave only the Windows one;
SPEC §14 gives three, and `platform.BridgeFile` implements them. The
Linux one is not what a guess would produce — `$XDG_RUNTIME_DIR/liro/`,
falling back to `~/.local/state/liro/`, not `~/.config` — which is the
whole reason writing it down is worth anything. Said with it: the agent
is Windows-only until phase 12, so only the first line has an agent
behind it today.

**A fourth, found while writing (a).** Both result bodies declare
`Failures []failureBody` with `json:"failures,omitempty"`, so
**`failures` is absent from an all-succeeded result, not `[]`** — and
§5.2's example showed `"failures": []`. §5.1's example happened to be
right, because the failure it shows is real. One sentence now covers
both endpoints, and the wrong example is corrected. It is a small thing
that costs an integrator an afternoon: a client written against the
example indexes into an array that is not there.

**What this does not do.** The SDK still does not compare its own
version against `minimumClientVersion`. [[D-212]] declined to, correctly
at the time, because "a comparison rule this SDK invented would be a
rule the agent had not agreed to". The agent has now stated one, so the
comparison is a small piece of work that can be done — and it is SDK
work, not a documentation fix, so it is recorded here rather than
smuggled into a phase that was asked to edit a document.

**Rejected.**
- **Changing the agent so a failed document carries an explicit
  `"content": null` instead of omitting the key.** It would make §5.2
  read more like §5.1, and it is a protocol behaviour change this phase
  is forbidden. Document what the code does.
- **Making `failures` always present as `[]`.** Same objection, and the
  same one [[D-220]] gives for not guessing a semver rule: two
  implementations disagreeing about a field is how this gets expensive.

---

## D-227 — `sign-digest` is the same bypass one command over, and this phase did not fix it

**Date:** 2026-09-09
**Phase:** F9

**Recorded because finding it was part of establishing whether
[[D-222]]'s fix is complete. It is not.**

`liro-bridge sign-digest --thumbprint <card> --digest <64 hex>` opens a
session on a real card and returns a raw RSA signature over whatever
digest it was handed. `internal/cli.RunSignDigest` has no consent
screen, no audit entry and no window, in a release build, on every
platform. It is F2's own exit condition and the README's documented
OpenSSL verification recipe.

**Why it is the same defect.** A digest is not a lesser thing than a
document: the digest a PAdES signature is computed over is a SHA-256 of
a `/ByteRange`, so a signature over an attacker-chosen digest is a
signature over an attacker-chosen document. And SPEC §6.5 forecloses the
obvious reassurance — the card's own PIN dialog is not a gate, because
the card caches the PIN in its own state independently of which process
is talking to it, so a second process signs silently for as long as the
session lives. That is the paragraph SPEC calls the most important in
the document, and it is why the consent window exists at all.

**Why it is not fixed here.** The phase named one defect and said "This
phase only." `sign-digest` is not a variant of `sign` — it is F2's whole
surface, the README's recipe, and one of the two things CI signs on
every push. What replaces it is a design question with real answers on
both sides: a consent window in front of a raw digest is a window that
can say almost nothing useful about what is being signed, since SPEC
§6.6's screen shows a document count, a batch fingerprint and file
names, and a bare digest has none of the three. Making that decision
quietly inside a phase scoped to `sign` is how a design nobody chose
ships.

**What would need deciding, so the next pass does not start from
nothing.** Either `sign-digest` goes behind the `softtoken` tag beside
[[D-222]]'s path — which costs a release binary nothing, since its only
documented use is verifying a build — or it keeps a real card and grows
a consent screen of its own, which needs an answer to what that screen
shows. The first is smaller and is what the evidence points at: F2
§6.1's recipe is a developer checking a build, and a developer's build
is exactly what the tag is for.

**Rejected.**
- **Reporting it only in this phase's report and not here.** A report is
  read once. This is the log a future phase reads in full.
- **Fixing it quietly as "obviously the same thing".** It is the same
  class and it is not the same change, and the phase's own instruction
  about the audit chain applies here with equal force: do not fix it
  silently.

---

## D-228 — `sign-digest` moves behind the `softtoken` tag; a release binary now contains no signing path that asks nobody, and the binary is read to prove it

**Date:** 2026-09-10
**Phase:** F9b

**Decision.** [[D-227]] set out two answers and said which the evidence
pointed at. This is that one, taken.

*(a) `internal/cli/signdigest.go` carries `//go:build softtoken`*, beside
`sign_noconsent.go`, and so does its test file. Nothing about what the
command does changed — the same flags, the same output, the same
`SignDeps`.

*(b) `cmd/liro-bridge` reaches it only through `runBuildOnlyCommand`*,
the softtoken-only dispatch [[D-222]] built for `sign-no-consent`.
`sign-digest` is gone from `topLevelCommands` and appears in
`buildOnlyCommands`, so a build that has it says so in `--help` and a
build that does not never mentions it.

*(c) A release binary contains neither `internal/cli.RunSignDigest` nor
`internal/cli.RunSignWithoutConsent` nor `internal/keysource/softtoken`*,
proved by reading the symbol table.
`TestTheNoConsentPathIsAbsentFromAReleaseBinary` now names all three, in
both directions, and so does CI's own binary-inspection step. Measured
against the two binaries this phase built:

```
internal/cli.RunSignDigest                 release=0 tagged=2
internal/cli.RunSignWithoutConsent         release=0 tagged=2
internal/keysource/softtoken               release=0 tagged=14
```

**The reasoning, recorded rather than assumed.** A consent screen in
front of a bare digest cannot show what SPEC §6.6 requires it to show —
a document count, a batch fingerprint and the list of file names —
because a digest has none of the three. It is not that the screen would
be inconvenient to build; there is nothing for it to say. And the
hash-only use case already has a better front door: `POST /v2/sign`,
with pairing, origin binding and a real consent screen, which is where
SPEC §4.3 puts it. SPEC §4.3 does not list a CLI digest command among
the four entry points at all. A release binary loses nothing.

What it keeps is what the command was written for: F2 §6.1's recipe, a
signature over a digest verified with OpenSSL, a tool sharing no code
with this project. That is a developer checking a build, and a
developer's build is what the tag is for (SPEC §16.6).

**The tag adds the soft token; it does not take CNG away.** Said here
because the README's recipe is the manual acceptance step against real
hardware and somebody will otherwise assume a tagged build cannot see a
card. `openCardOrSoftToken` opens the CNG store first and falls back to
the soft token only on `CERT_NOT_FOUND` ([[D-033]]), in every build that
has it. The README now says so where the recipe is, and its commands
build with the tag.

**Verified by running the binaries, not only by testing.** Both built
from this tree, with `LOCALAPPDATA` pointed at a scratch directory:

```
> f9b-release.exe sign-digest --thumbprint ABCD --digest <64 hex>
liro-bridge: sign-digest is not in a release build: it signs whatever
digest it is handed, with no consent window (SPEC §18.2).
  A local batch: liro-bridge sign --in <file>. An application:
  POST /v2/sign (see docs/PROTOCOL.md).
exit=2

> f9b-tagged.exe sign-digest --thumbprint 95AB…2882 --digest <64 hex> --out sig.bin
TEST POTPIS — napravljen softverskim tokenom, ne pravom karticom.
exit=0
> openssl dgst -sha256 -verify pub.pem -signature sig.bin input.txt
Verified OK
```

`sign-no-consent` was run from the same tagged binary against
`testdata/pdfs/blank.pdf` and produced a B-B signature, so F3's exit
condition is unaffected by the rearrangement.

**A release binary refuses `sign-digest` by name.**
`runBuildOnlyCommand` in `noconsent_release.go` prints one sentence
saying the command is not in a release build, names the two things to
use instead, and returns 2.

This is the one place this phase adds something rather than removing it,
and the reason is a trap it would otherwise have created. An unknown
subcommand in this program prints the usage text and exits **0** — so a
script that had been calling `sign-digest` would have gone on reporting
success while producing no signature. That is worse than the removal it
would be hiding. It is the same answer [[D-222]] gave `sign
--interactive`: say what happened, in a sentence, and fail.

Knowing the name is not knowing the path: the string is in the binary
and `internal/cli.RunSignDigest` is not, which is the property (c) reads
the symbol table to establish.

**Tests.**
- `TestTheCommandsWithNoConsentWindowAreListedExactlyWhenTheyExist`
  compares `--help` against a `hasNoConsentPaths` constant that each of
  `noconsent_release.go` and `noconsent_softtoken.go` defines for
  itself. One test, asserting the right thing in both build
  configurations — rather than two tests each asserting half of it in a
  build the other never sees. The Windows CI job builds this suite
  **with** the tag, so a check that only compiled without it would never
  run there ([[D-221]], [[D-222]]).
- `TestAReleaseBuildRefusesSignDigestByNameRatherThanPrintingUsage`
  pins the exit code, because 0 is what it would otherwise be.
- The symbol-table test above, extended.

**One mechanical consequence, recorded so it is not read as scope
creep.** `softTokenSource`'s only untagged, platform-independent caller
was `runSignDigest` in `main.go`. With `runSignDigest` behind the tag,
the plain `GOOS=linux` view of `cmd/liro-bridge` had a function nothing
could reach, and `golangci-lint`'s `unused` said so — the same shape of
consequence [[D-222]] recorded for `caCertificatesFromTSL`, and
`//nolint` is the wrong answer to it for the reason [[D-111]] gives.

The fix is the one [[D-222]] used: put the thing where it belongs. Three
copies of one rule — "open the CNG store, and fall back to the soft
token only when the certificate is genuinely not there" — lived in
`main.go`, `noconsent_softtoken.go` and `interactive_windows.go`. They
agreed, which is the only reason it was not already a defect; three
copies of a rule is how they stop agreeing ([[D-108]], [[D-124]],
[[D-138]]). They are now one function, `openCardOrSoftToken(hwnd)`, in
`keysources.go`, constrained to `windows || softtoken` — which is
exactly the set of builds that have a signing path at all: Windows,
where the window flow signs, and any tagged build, whose two headless
commands are what CI signs with on an Ubuntu runner. `softTokenSource`'s
`!softtoken` half moves to a `windows`-constrained file for the same
reason, since in an untagged build its only caller is the window flow.
All four views — {Windows, Linux} × {tagged, untagged} — build, vet and
lint clean.

**Rejected.**
- **Giving `sign-digest` a consent screen of its own and keeping it in a
  release binary.** [[D-227]]'s second answer. It needs an answer to
  what that screen shows, and there is none: SPEC §6.6's three items do
  not exist for a digest. A screen that says "a program wants to sign 32
  bytes" is a screen that trains people to click Approve.
- **Leaving it, on the grounds that only a developer uses it.** Who uses
  it is not a property of the binary. SPEC §6.5 is the standing answer:
  the card caches the PIN in its own state, so any process on the
  machine can sign for as long as a session lives, and the consent
  window is the only real gate.
- **Renaming the file `signdigest_softtoken.go`.** The name says what
  the file is; the tag says which builds have it. `sign_noconsent.go`
  set that convention one phase ago and it needs no second form.
- **Letting a release binary print the usage text for `sign-digest` like
  any other unknown command.** Above: exit 0 for a command that used to
  sign is a trap, and this is the one command that had callers.
- **Fixing the general "unknown subcommand exits 0" behaviour.** A real
  defect, and not this phase's. It applies to every misspelling, not
  only to the one name this phase removed, and changing it changes what
  `liro-bridge frobnicate` does — which nothing here has been asked to
  decide.

---

## D-229 — The audit chain is guarded by a named mutex over its own directory, held across the whole of Append; what it does when it cannot be had, and what it does when the last writer died

**Date:** 2026-09-10
**Phase:** F9b

**The defect.** [[D-223]] established it and deliberately left it: two
processes appending to one hash chain both read the same last entry,
both compute the same `Sequence` and `PrevHash`, and the chain forks —
after which the log reports itself as tampered with, from the fork
onwards, for ever. Nothing guarded it. `audit.Store`'s only lock was a
`sync.Mutex` on the *Store value*, which its own comment described
accurately as "atomic-from-this-process".

**Decision: a named mutex over the audit directory, held across the
whole of `Append`.**

*(a) The whole of Append, not the write.* `platform.DirLock` is taken
before `s.chains()` and released after the line is written. The read is
half of what forks — two processes that read the same last entry have
already lost, whatever they do next.

*(b) The name comes from the directory, never from the session.*
`Global\LiroBridge.Dir.<first 8 bytes of SHA-256 of the cleaned,
lower-cased path>`. `Local\` — which is what this project's only other
named mutex uses ([[D-119]]'s `ShellBatchLeaderName`) — is per logon
session, and the audit directory is per *user* (`platform.ConfigDir`).
One user with a console session and an RDP session, or an interactive
agent and a scheduled task running as the same user in session 0,
resolves one directory from two sessions and would not share a `Local\`
name. Hashed rather than embedded because a Windows object name cannot
contain a backslash and a path is mostly backslashes.

*(c) `WAIT_ABANDONED` is a signal about the data, not a kind of
acquisition.* When it comes back, `Append` checks whether the entry it
is about to chain from is *sound* — its own hash recomputes, and its
`PrevHash` is the previous entry's `Hash` — and if it is not, leaves the
chain exactly where it is and starts a new one beside it whose first
entry records `unsound`. That reaches [[D-166]]'s machinery rather than
duplicating it: the same `Discontinuity`, the same "the old file is
never touched", the same "the person is told once", one more
`BreakReason`.

*(d) The wait is bounded, and expiry starts a new chain rather than
refusing to sign.* `AppendLockTimeout` is ten seconds. On expiry the
entry goes into a chain of its own, created with `O_EXCL`, whose first
entry records `unguarded`. SPEC §6.7 already settles the direction for
the unreachable case in as many words — "blocking a bookkeeper's
afternoon over a log is worse than recording that the log moved" — and
this is the same trade with a different cause.

**Why a named mutex rather than `LockFileEx` or one owning process.**
[[D-223]] listed three shapes and did not choose. The reason for this
one is a property none of the three descriptions states: **on Windows a
mutex whose owner died returns `WAIT_ABANDONED` to the next waiter and
grants it ownership.** That is not merely "it does not deadlock" — it
*tells the acquirer that the previous writer died mid-write*, which is
[[D-166]]'s situation exactly. `LockFileEx` releases on process death
too, but silently: the next writer gets the lock and cannot tell whether
the last line on disk is whole. One process owning the log is the better
architecture and is not this phase's to build; it belongs with the batch
hand-off, when the program's arrangement is being changed anyway.

**A limit of the abandoned signal, stated rather than glossed.** A named
object exists only while some handle to it is open. If the process that
died was the only holder, the object simply ceases to exist and the next
process creates a fresh, unowned one and is told nothing. So the signal
is observable exactly when somebody else was already waiting — which is
the contended case, the one this whole change is about — and a
long-lived tray agent holding a handle is what makes it observable more
widely. The uncontended case is not left uncovered: a torn last line
does not parse, and [[D-166]]'s existing machinery catches that on every
read. `unsound` covers the remaining gap, a line that reads perfectly
and is not sound.

**Measured: `Global\` needs no privilege here, and the fallback if it
ever does.** Creating an object in the global namespace is documented as
requiring `SeCreateGlobalPrivilege`. Measured on this machine, Windows
11 Pro 26200, with a filtered administrator token — `whoami /priv` lists
exactly five privileges and that is not one of them — `CreateMutexW` on
a `Global\` name **succeeds**. That is one machine, and a policy that
denies it is possible, so `ERROR_ACCESS_DENIED` falls back to the
session-scoped `Local\` name and says so once at `Warn`. That still
guards the case [[D-223]] actually measured, an agent and a `sign` in
one session; it does not guard two sessions. Recorded here rather than
hidden because it is a real difference between two machines.

**The goroutine is pinned to its OS thread for the hold.** A Windows
mutex is owned by the *thread* that waited on it, and a goroutine may
move between threads at any function call, so `ReleaseMutex` could
otherwise run on a thread that does not own it, fail with
`ERROR_NOT_OWNER`, and leave the lock held until the process exits.
`runtime.LockOSThread` in `Lock`, `UnlockOSThread` in `Unlock` and on
every failure path.

**Non-Windows is `flock` on a `.lock` file inside the directory**, with
a bounded retry. It never reports abandoned, which is the difference
being described above, and that costs nothing today: the agent is
Windows-only until phase 13. It exists so `internal/audit` has one
behaviour on every platform its tests run on rather than two — CI's
Ubuntu job runs this package's whole suite, and a no-op lock there would
have made the reproduction test below pass for the wrong reason on the
one platform CI actually runs it on. Both suites were run on a real
Linux kernel from this tree, not only cross-compiled.

**What ten seconds costs, and where.** `recordInteractiveAudit` runs on
the flow goroutine, not on the UI thread — `internal/ui`'s own model is
that nothing a caller supplied ever runs on the UI thread ([[D-207]]) —
so a wait delays the report screen and does not freeze the window. And
the signature has already happened by then. Ten rather than one because
the cost of waiting is a report that appears late and the cost of giving
up is a permanent extra chain in the audit log, which a person sees for
the rest of the log's life.

**Measured, with real processes and built binaries.** A throwaway
harness inside the module, created and deleted in the same session
([[D-100]]), built as an `.exe` and run as eight separate processes over
one audit directory, all released by one gate file.

*Against the tree this phase started from:*

```
entries=8 chains=1 storeOK=false brokenAt=1
  seq=0 app=proc-7  prev= hash=19dfdded
  seq=0 app=proc-0  prev= hash=0bb48b1d
  seq=0 app=proc-6  prev= hash=9835c27e
  … five more, every one of them sequence 0
```

*Against this one:*

```
entries=8 chains=1 storeOK=true brokenAt=-1
  seq=0 app=proc-6  prev=          hash=14c3eba8
  seq=1 app=proc-7  prev=14c3eba8  hash=8e640882
  seq=2 app=proc-4  prev=8e640882  hash=eee372e1
  … through seq=7, one unbroken chain
```

*What it does when the lock cannot be had* — a second process holds it
and does not let go:

```
10:47:58.264
WARN audit: the log's lock could not be taken; starting a new chain
  rather than refusing to record the batch waited=10s
  lock=Global\LiroBridge.Dir.dd3a73cdcaa6613f
blocked pid=17720 seq=0 discontinuity=&{1 2026-09-001.jsonl 0 true 0 unguarded}
10:48:08.338

entries=2 chains=2 storeOK=true brokenAt=-1
  chain 1: entries=1 ok=true
  chain 2: entries=1 ok=true started because chain 1 could not be continued (unguarded)
```

Ten seconds, then a chain of its own that says why, and both chains
intact.

*What it does when the writer died* — the holder killed by exact PID
while holding the lock, over the forked log the pre-fix binary produced
above:

```
WARN audit: the previous writer died mid-append and the chain's last
  entry is not sound; starting a new chain chain=1 entries=8
after-death pid=13792 seq=0 discontinuity=&{1 2026-09-001.jsonl 0 true 0 unsound}

entries=9 chains=2 storeOK=false brokenAt=1
  chain 1: entries=8 ok=false
  chain 2: entries=1 ok=true started because chain 1 could not be continued (unsound)
```

The damaged chain still has its eight entries and is still reported as
damaged, which is what it is. Over a *sound* log the same kill produces
no new chain at all — the wait returns immediately, ownership is
granted, and the entry chains from its predecessor at sequence 1.

**Tests.**

- `TestTwoStoresOnOneDirectoryDoNotForkTheChain` is [[D-223]]'s
  four-line reproduction, committed. Two `NewStore` calls on one
  directory, an `Append` on each from two goroutines released together,
  then `Verify`. **Confirmed to fail against the tree this phase started
  from**, in a worktree at `58a6a23`, on both Windows and Linux:
  "sequences = 0, 0; want 0, 1" and "the log verifies as tampered with
  at entry 1". Two Stores in one process rather than two processes on
  purpose: the guard that was there was per-Store, so this is the shape
  that isolates the defect with no scheduling to arrange.
- `TestManyAppendsFromManyStoresStayOneUnbrokenChain` is the same at
  F6's own scale, twenty writers, because Explorer starts one process
  per selected file ([[D-119]]) and a lock that holds for two and not
  for twenty passes the test above and fails in a person's hands.
- `TestASecondProcessCannotAppendWhileThisOneHoldsTheLog` is the
  cross-process half, and it is what makes the two tests above mean
  anything: a `sync.Mutex` passes them and fails this. A second real
  process — the test binary re-executed — takes the lock, says so on
  stdout, and holds it until its stdin is closed.
- `TestALogReleasedByADeadWriterIsPickedUpAndContinued` and
  `TestADeadWriterOverAnUnsoundChainStartsANewOneRatherThanExtendingIt`
  kill that process instead, by its own handle, never by image name.
- `TestAnUnsoundChainIsExtendedWhenNobodyDied` is the control. Same
  directory, same forked chain, same append, no dead writer — and the
  entry goes onto the chain. Without it the test above would pass just
  as happily if the soundness check ran unconditionally, and the
  abandoned signal would be proving nothing.
- `internal/platform`'s own four, including that a held lock's wait
  expires rather than blocking for ever, and that two directories are
  two locks.

**Could the two-real-processes measurement be made a deterministic
test?** The collision itself, no. Two independent schedulers cannot be
made to enter one critical section on the same tick, and a test built
around arranging it would be timing how fast the machine is rather than
observing what this program does — which is exactly what [[D-201]]
forbids and why this is said here rather than a `time.Sleep` being
written. What *can* be made deterministic is the property the collision
was evidence about — that one process holding the log stops another from
reading and extending it — and that is what the three cross-process
tests above observe, with a real second process, a real kill, and
assertions on the resulting chain structure rather than on any duration.

**What is not locked, and why.** `All`, `Chains`, `Verify`,
`LatestChainFile` and `Export` take the process mutex and not the
directory lock. They are tolerant of a chain that cannot be read to the
end by construction, so the worst a concurrent append does to a reader
is hide the line being written at that instant — while taking the lock
would make the audit window wait on a signing batch, which buys the
reader nothing.

**Two new `BreakReason` values**, `unsound` and `unguarded`, with their
sentences in all three catalogues and in both places a reason becomes
words (`tray_windows.go`'s export summary and `auditlog_windows.go`'s
list).

**Rejected.**
- **`LockFileEx`.** Above: it releases on process death silently, which
  loses the one fact worth having.
- **One process owning the log, the others handing entries to it.** The
  better architecture, and a change to how this program is arranged
  rather than to a function. It belongs with the batch hand-off.
- **Verifying the whole chain on an abandoned acquisition rather than
  its last entry.** That answers `Verify`'s question, not `Append`'s. An
  entry damaged three months ago does not stop a correct successor being
  computed today, and starting a new chain over it would hide the older
  damage behind a fresh one.
- **Checking soundness on every append.** It would make the abandoned
  signal decorative, and it would turn every read of a log damaged long
  ago into a new chain. The control test above exists to keep this
  rejected.
- **Refusing to sign when the lock cannot be had.** SPEC §6.7 rejects it
  for the unreachable case in its own words, and the reasoning does not
  change with the cause.
- **Appending to the current chain anyway after the wait expires.** It
  is the defect, performed deliberately.
- **Writing the new chain's file without `O_EXCL`.** Two processes whose
  waits expire do so at the same moment by construction, so both would
  pick the same next chain number and fork the file they had just
  created to avoid forking. `O_EXCL` and the next number up.
- **A `Global\` name with no fallback.** The privilege is documented as
  required and measured as not enforced here; one machine is not
  evidence about every machine, and a lock that fails to open is worse
  than one whose scope is narrower than intended.
- **A private namespace (`CreatePrivateNamespaceW`) with a SID-carrying
  boundary descriptor.** It is the arrangement designed for exactly this
  — cross-session, per-user, no privilege — and it was measured working
  here. Rejected because `Global\` also works here, with one documented
  API call instead of four that `golang.org/x/sys/windows` does not
  bind, and the fallback covers the case that would justify the extra
  machinery. It is written down so the next person who meets an
  `ERROR_ACCESS_DENIED` on that path knows it was tried.

---

## D-230 — What `--force` forces: the output-file question, and nothing that can reach the document being signed

**Date:** 2026-09-10
**Phase:** F9b

**The question.** [[D-222]] left a release binary's `sign` taking two
flags, `--in` and `--force`, and nothing said what the second one
forced. If it touched overwriting an input file, SPEC §18.10 would
apply.

**Decision.** `--force` answers one question in advance — *"a file
already exists where this document's signature would go: replace it, or
write beside it?"* — and answers nothing else. With it,
`resolveOutputConflict` returns immediately and the person is never
shown the output-exists screen; without it, an existing output is
refused or renamed according to what they say ([[D-104]]).

**It can never reach the document being signed.** That is a property of
two things together rather than a check anywhere in the signing path:

- the output name is `base + suffix + ext` (`jobs.OutputPathFor`), and
- the suffix can never be empty, because `config.Load` replaces an empty
  one with `-signed`.

So the output's base name is strictly longer than the input's, and the
two cannot be the same file — including for a document whose own name
already ends in the suffix, which is the case a person actually meets on
a second run over a folder (`ugovor-signed.pdf` →
`ugovor-signed-signed.pdf`). **SPEC §18.10 and SPEC §12.11 are therefore
not in question**: the original is not merely not overwritten silently,
it is not reachable.

What `--force` *does* overwrite is a previous run's output, and not
silently: the person typed the flag. SPEC §12.11 forbids a silent
overwrite; it does not forbid overwriting a file the person named the
flag about.

**What it does not answer.** J-3's question — "this document's name
already ends in `-signed`; is this a counter-signature or did you mean
the original?" ([[D-164]]) — is still asked. The two are different
questions about different files, and a flag that silently answered both
would sign documents the person did not choose.

**Tests.** `TestForceCanNeverOverwriteTheDocumentBeingSigned` walks six
input shapes, including a name that is already `-signed` and one with no
extension, with the output folder unset and set to the input's own
folder. `TestAnEmptyOutputSuffixCannotReachTheSigningFlow` writes
`{"outputSuffix": ""}` to a real config file and loads it, because the
claim is not "nobody would configure an empty suffix" but "an empty
suffix is not what the flow is handed".

**Rejected.**
- **Renaming it `--overwrite`.** More accurate about what it does and
  less accurate about when it applies: it is answered once and applies
  to every document in the batch, which "overwrite" reads as being about
  one file. `--force` is also what the path with no window has always
  called it, and two names for one answer is what [[D-138]] removed.
- **Removing it, since the window can ask.** The window asks once per
  batch already; the flag is how a person who knows the answer skips
  being asked. It bypasses no consent — the approval screen is
  unaffected — and SPEC §18.2 is about the signature, not about where
  the file lands.

---

## D-231 — `sign` and `open` stay two commands, and the overlap between them is deleted

**Date:** 2026-09-10
**Phase:** F9b

**The question.** After [[D-222]], `liro-bridge sign --in x.pdf` and
`liro-bridge open x.pdf` both opened the same window. This project has
recorded its objection to one fact living in two places three times
([[D-108]], [[D-124]], [[D-138]]). Either justify the two or collapse
them.

**Decision. Two commands, and the overlap goes.** `open` now takes no
arguments at all.

**Why two.** They are two different questions, and each has an answer
the other cannot give:

- `open` is *the window, with nothing in it*. There is no `sign` for
  that: `sign` requires `--in`, and a `sign` with no documents would be
  a window opened at a step that has nothing to show.
- `sign --in <pattern>` is *this batch*. There is no `open` for that
  either: it expands a pattern, and it opens the flow one step in, at
  the certificate, with the document step skipped because the documents
  are already named. That difference is not cosmetic — `open` is a
  window the person opened themselves, `sign` is a window that comes to
  the front because a batch is waiting (`AlwaysOnTop` is set for exactly
  one of the two).

Nothing is implemented twice underneath either: both funnel into
`mainWindow.open`, and there is one consent screen, one session and one
audit log — SPEC §4.3's own sentence, which is what the "one fact in two
places" objection is actually about.

**What is deleted, and why that is the whole of the charge.** `open
<paths>` seeded the window with paths given after the command. Nothing
ever called it that way — not the tray's Open item, which reaches the
window inside this process; not the Explorer context menu, which has its
own verb (`platform.ShellMenuVerbFlag`); not `--help`, which has never
mentioned an argument. And it was the one place two commands answered
one question differently: `sign --in "C:\docs\*.pdf"` expands the
pattern, because on Windows the shell does not, and `open
"C:\docs\*.pdf"` looked for a file with an asterisk in its name. One
intent, two syntaxes, two answers — which is the disagreement [[D-138]]
removed for the stamp margin and [[D-108]] for the certificate filter.

`open` with an argument now says so and points at the command that does
take documents, and returns 2:

```
> f9b-release.exe open C:\docs\ugovor.pdf
liro-bridge: open takes no arguments; to sign named documents use:
  liro-bridge sign --in <file or pattern>
exit=2
```

**Consequence for SPEC §4.3.** The CLI stays one of the four entry
points, because `sign` is still a way in that the window row does not
describe. §4.3's row is corrected in [[D-232]]; §1 needs no change,
since its second paragraph is about the local HTTP API and says nothing
about a command line.

**Tests.** `TestOpenTakesNoArgumentsAndPointsAtSignInstead`.

**Rejected.**
- **Collapsing `open` into `sign` — `sign` with no `--in` opens the
  document step.** It would work, and it would make "which step opens"
  depend on whether a flag was given, which is a hidden mode. It also
  costs the name `open`, which is what a person types when they want the
  application rather than a signature, and what a desktop shortcut will
  want when F10 makes one.
- **Collapsing `sign` into `open` — `open` takes documents and starts at
  the certificate.** Worse: `sign` is the name SPEC §19's own F9 row is
  written around, and "open" does not mean "sign these".
- **Keeping `open <paths>` and making it expand patterns too.** It would
  end the disagreement by giving two commands the same behaviour, which
  is two ways to say one thing — the thing being objected to, one step
  further on.
- **Keeping `open <paths>` because F10 might want it.** F10 can add it
  back with a reason, which is what [[D-222]] said about `--out` and
  `--resign` and is the same answer here.

---

## D-232 — SPEC §4.3's CLI row said "Scripts, legacy systems" and showed a flag that no longer exists; corrected

**Date:** 2026-09-10
**Phase:** F9b

**Decision.** One edit to `docs/SPEC.md`, the one [[D-225]] recorded as
deliberately left for whoever made the next SPEC edit. The table of the
four entry points read

| CLI | Scripts, legacy systems | Yes | `liro-bridge sign --in x.pdf --out y.pdf` |

and now reads

| CLI | The user, from a shell | Yes | `liro-bridge sign --in x.pdf` — the same window, entered at the certificate step. There is no unattended mode. |

The window row gains one clause in its Notes for the same reason —
`liro-bridge open` is that row's command line, and naming it is what
keeps the CLI row from reading as though it were the only one with a
command.

**Why both halves of the old row were wrong.** "Scripts, legacy systems"
is an audience the owner ruled there is no contract for ([[D-225]]);
`--out` was removed from a release binary by [[D-222]]. The row now
describes what the command actually is: a person at a shell, who gets
the same window, the same consent screen and the same audit log as every
other entry point — which is the sentence immediately above the table,
and the one that is load-bearing.

**"There is no unattended mode" is in the row on purpose.** It is the
question the old row invited, it is answered elsewhere in SPEC (§2's
non-goals, §18.2) and it was answered wrongly by the code until
[[D-222]]. A Notes column that says it costs nothing and closes the
reading that produced the defect.

**Four rows, not three.** Whether the CLI is still an entry point
depended on [[D-231]], and it is: `sign` is a way in that the window row
does not describe.

**Rejected.**
- **Deleting the CLI row.** It is an entry point, it goes through the
  consent screen, and it is what F9's own exit condition is about.
- **Rewriting more of §4.3 while in here.** SPEC is "the rules that
  never change" (§0), and a phase that edits it freely is a phase that
  can soften a constraint by rewording it. The phase named this row.

---

## D-233 — `--out` and `--resign` have no replacement in a release binary, and that is the choice rather than an oversight

**Date:** 2026-09-10
**Phase:** F9b

**Decision.** Recorded as a decision because F9's report said it plainly
and a report is read once. A release binary's `sign` takes `--in` and
`--force`. Of the fifteen flags [[D-222]] removed, thirteen have a home
— the certificate is chosen in the window, the level, the timestamp
authority and the output folder are `config.json`, the stamp is the
method screen ([[D-146]], [[D-151]]) — and **two do not**:

- **`--out`**, "write this one document to exactly this path". It is
  nearly answered by the output-file question ([[D-104]]) and the chosen
  output folder ([[D-126]]), and neither is the same thing.
- **`--resign`**, "yes, sign this document whose name already ends in
  `-signed`". It is answered by J-3's own question ([[D-164]]), which a
  person answers rather than a script.

**Neither is coming back in this phase, and the reason is the same for
both: what they were for is gone.** Both are flags whose value is that
they let something run with nobody at the machine — a script that names
an exact output path, a script that says "yes" in advance. A release
binary's `sign` opens a window and waits for a person; a flag that
pre-answers a question that person is about to be asked saves them one
click and buys a script nothing at all, because the script is stopped at
the approval either way.

`--force` survives that test and these two do not, which is worth saying
explicitly since it looks inconsistent: `--force` answers a question
about *where a file lands*, which is not a question about the signature
and does not become useful only when nobody is watching. A person
signing forty documents at a command line has a real reason to say "yes,
replace them" once rather than forty times ([[D-230]]).

**What would bring one back.** A person asking for it, with what they
were doing. It would be a flag on the window flow, it would pre-answer a
question the flow already asks, and it would arrive with a reason — the
same conditions [[D-222]] set. An integrator who is not on Node uses the
protocol directly; that is the owner's standing ruling and it is not
this phase's to revisit.

**Rejected.**
- **Adding `--out` back now because it is small.** Nobody has asked for
  it. A flag added on a guess is a flag that has to be kept.
- **Leaving it recorded only in F9's report.** The report is why this
  entry exists. `docs/decisions.md` is the file a future phase reads in
  full (SPEC §17), and "this was chosen" and "this fell out" are
  indistinguishable a year later unless one of them is written down.

---

## D-234 — The audit log's guard is a file lock and the named mutex is only the signal; [[D-229]]'s Global\ measurement was taken on an administrator account and is re-measured here

**Date:** 2026-09-10
**Phase:** F9b — review

**Supersedes the mechanism half of [[D-229]].** That entry stands as
written (SPEC §17: entries are never edited) and its reasoning for
*wanting* WAIT_ABANDONED is unchanged and still the reason a mutex is
there at all. What changes is what carries the exclusion.

**The objection, and it was right.** [[D-229]] reported that a `Global\`
mutex opens here without `SeCreateGlobalPrivilege`, and paired that with
a `Local\` fallback for machines that refuse. Two things were wrong with
it:

*(a) The measurement was taken on the wrong account.* Stated plainly:
`helios\veljko` **is** a member of `BUILTIN\Administrators`. The token
it runs with is a filtered one — Administrators deny-only, five
privileges, none of them `SeCreateGlobalPrivilege` — which looks like a
standard user's and is not one. The privilege is granted by default to
Administrators and the service accounts, so measuring on an
administrator's machine is measuring the case that decides nothing.

*(b) The fallback reintroduced the defect the mutex was chosen to
avoid.* `Local\` is per logon session; the audit directory is per user.
On a machine that refuses `Global\`, one user's console session and RDP
session — or an interactive agent and a scheduled task in session 0 —
would silently have stopped sharing a lock over one directory, and the
chain would fork again. A `Warn` line goes into a log nobody reads. That
is [[D-221]]'s shape three days later: the check ran where it was
written.

**Re-measured, on a token strictly weaker than a standard user's.** A
second local account cannot be created here without elevation, so the
next best thing was measured and it is stronger evidence in the
direction that matters: `CreateRestrictedToken` with
`DISABLE_MAX_PRIVILEGE` and `BUILTIN\Administrators`,
`BUILTIN\Performance Log Users` and `Helios\docker-users` all turned
deny-only. An access check is monotone — removing SIDs and privileges
can only reduce what a token can do — so a `Global\` name that opens
under this opens for a standard user, whose enabled group set is a
subset of what is left. Measured two ways, because a privilege check
might plausibly consult the process token rather than the thread's:

```
=== this process, as launched ===
  privileges   : SeShutdownPrivilege SeChangeNotifyPrivilege SeUndockPrivilege
                 SeIncreaseWorkingSetPrivilege SeTimeZonePrivilege
  admin enabled: false     elevated: false
  Global\LiroProbeGlobal   OK      Local\LiroProbeLocal   OK      LockFileEx  OK

=== this thread, impersonating the restricted token ===
  privileges   : SeChangeNotifyPrivilege
  admin enabled: false     elevated: false
  Global\LiroProbeGlobal   OK      Local\LiroProbeLocal   OK      LockFileEx  OK

=== child process, restricted primary token ===
  privileges   : SeChangeNotifyPrivilege
  admin enabled: false     elevated: false
  Global\LiroProbeGlobal   OK      Local\LiroProbeLocal   OK      LockFileEx  OK
```

So on Windows 11 Pro 26200, `CreateMutexW` on a `Global\` name does not
require the privilege its documentation names, for an interactive user
with no privileges at all. [[D-229]]'s primary path was right; its
evidence was not, and now it is.

**Decision, all the same: the guard moves off the mutex.**

*(a) The exclusion is `LockFileEx` on a `.lock` file inside the audit
directory* (`flock` elsewhere, which is what the non-Windows side
already did). The file *is* the name: two processes that resolve the
same directory open the same file and contend, in any session, under any
account, with no privilege and no namespace to get wrong.

*(b) The named mutex carries the abandoned-writer signal and nothing
else.* It is taken first — fixed order, so nothing can hold one while
waiting for the other backwards, and mutex-first specifically because a
waiter blocked on a mutex holds a handle to it, which is what keeps the
object alive when its owner dies and what makes the waiter the thread
told about it. Failing to get it is fatal to nothing: the state is
reported as an ordinary acquisition and the file lock decides.

*(c) The `Local\` fallback survives, and now costs nothing.* On a
machine that refuses `Global\`, the signal narrows to one session and
the guard does not move at all. That is a fallback worth having rather
than one that hides a defect.

**Why this is the right answer even though (a) came out well.** The
question "is a `Global\` object available" has a different answer on
different machines, and a correctness property should not have a
different answer on different machines. The file lock has no such
question attached. It also closes a hole [[D-229]] recorded and accepted
in `dirLockID`'s own comment — two spellings of one directory (a
substituted drive, an 8.3 short name) hash differently and would not
have shared a mutex. Measured, with real processes, below.

**Measured: the cross-session case, by its mechanism.** Two logon
sessions cannot be created on this machine without administrative
rights — `query session` shows session 0 (services, disconnected) and
session 1 (console) and nothing else — so the report does not claim one
was. What two sessions do to a session-scoped mutex is make it two
different objects over one directory, and that *is* reproducible: `subst
Y: <dir>` and then reach one audit directory by both spellings. The
premise, measured rather than assumed:

```
C:\...\f9b\subst\audit   -> flock+0839113275106621
Y:\audit                 -> flock+4af52dc03fe54eca
```

Two different mutex names, one directory. Eight real processes, four by
each spelling, released together by one gate file:

```
entries=8 chains=1 storeOK=true brokenAt=-1
  seq=0 app=ypath-3  prev=          hash=c55f984f
  seq=1 app=ypath-2  prev=c55f984f  hash=613087b9
  ...
  seq=7 app=cpath-3  prev=1dd85049  hash=971058a6
```

One unbroken chain. The mutex could not have excluded any of them; the
file did. Under [[D-229]]'s arrangement this run forks.

**Measured: the fallback path itself, rather than reasoned about.**
`TestALockWithNoMutexAtAllStillExcludes` builds two locks over one
directory with no mutex at all — the state `openMutex` produces on a
machine that refuses every named object — and asserts that one still
cannot be taken while the other holds it, and that it is granted once
the other lets go. `TestTwoLocksThatCannotShareAMutexStillExcludeEachOther`
does the same with two mutexes that are provably different objects, and
proves that premise before relying on it.

Both observe order and outcome, never duration. An earlier version of
that helper released the first lock without waiting for the second
side's bounded attempt to finish, so the attempt raced the release and
was granted for a perfectly good reason — which is exactly the class of
mistake [[D-201]] is about, caught here by the test failing.

**Everything [[D-229]] measured still holds under the new arrangement**,
re-run against it with real processes: eight processes over one
directory produce one chain 0…7; a holder that never lets go produces
the ten-second wait and a chain of its own recording `unguarded`; a
holder killed over a sound log continues the same chain at sequence 1;
and a holder killed over a forked log starts a new chain recording
`unsound` — which is the abandoned signal still arriving, since nothing
else can produce that reason.

**One thing that changed for SPEC §18.3's sake.** `DirLock.Name` is
logged by `audit.Append` when a wait expires, and the obvious name for a
file lock is its path — which sits under a user profile and therefore
carries the person's name, into a log file, which SPEC §18.3 forbids.
The name is the directory's hash on both platforms (`flock+<16 hex>`),
and `TestTheNameNeverCarriesThePath` pins it.

**Rejected.**
- **Leaving the mutex as the guard, since `Global\` works here.** The
  measured account was the wrong one, and even with the right one the
  answer is a per-machine policy. A correctness property that depends
  on one is a property with a footnote.
- **Keeping the `Local\` fallback as the guard on machines that refuse
  `Global\`.** Above: it is a per-session guard on a per-user directory,
  which is the defect wearing a warning label.
- **Dropping the mutex entirely now that the file lock carries the
  exclusion.** It is the only thing that says a writer died mid-write.
  [[D-229]]'s reasoning for it stands and is untouched.
- **Taking the file lock first and the mutex second.** Simpler-looking,
  and it loses the signal: a waiter that already holds the file finds
  the mutex uncontended, and the object may not even exist any more.
- **Creating a real standard-user account to measure on.** It needs
  elevation, it leaves a profile and a SID behind, and the restricted
  token is stronger evidence: it is strictly weaker than the account it
  stands in for.

---

## D-235 — A batch never writes over one of its own documents; [[D-230]]'s claim was true of one document and not of a pattern

**Date:** 2026-09-10
**Phase:** F9b — review

**Supersedes [[D-230]]'s scope claim.** That entry stands as written
(SPEC §17) and everything it says about what `--force` *answers* is
still true. What it got wrong is the word "cannot".

**The objection, and it was right.** [[D-230]] argued that `--force`
cannot reach the document being signed, because the output name is
`base + suffix + ext` and the suffix is never empty, so an output is
always longer than its own input. True — and it is a claim about one
document. A pattern is not one document:

```
liro-bridge sign --in *.pdf
```

over a folder holding `faktura.pdf` and `faktura-signed.pdf` expands to
both, and `faktura.pdf`'s output **is** `faktura-signed.pdf`, which the
batch is at that moment signing.

**Measured, on disk, with the built binary.** The same folder, before
and after, hashes and sizes:

```
--- before the fix, --resign --force ---
  before: faktura.pdf=427B/b9749fdb   faktura-signed.pdf=427B/b9749fdb
  | Potpisano: ...\faktura-signed-signed.pdf
  | Potpisano: ...\faktura-signed.pdf
  | Potpisano 2/2 dokumenata
  after : faktura-signed-signed.pdf   66714 bytes  307a9930
  after : faktura-signed.pdf          66714 bytes  81188829   <- was 427B/b9749fdb
  after : faktura.pdf                   427 bytes  b9749fdb
```

`faktura-signed.pdf` — a document this batch had just signed — was
replaced by `faktura.pdf`'s signature. The only surviving copy of what
was signed is inside `faktura-signed-signed.pdf`, a signature over a
document that no longer exists.

**And it is not about `--force`.** Answering the output-file question
with "overwrite" reaches the same place; `--force` is that answer given
in advance. Neither the flag nor the question can tell the two cases
apart, because at that level they look identical: a file exists where a
signature is about to go.

**Order, and what decides it.** `expandInteractiveInput` sorts the
glob's matches. Comparing `faktura-signed.pdf` with `faktura.pdf`, the
first difference is the suffix's own first character against the
extension's dot: `-` is 0x2D and `.` is 0x2E, so **the already-signed
sibling sorts first and is signed first**, and is then destroyed. A
configured suffix beginning with a character above `.` — `_signed`, say
— reverses it, and the sibling is destroyed before it is read, so the
signature that survives is of a document nobody kept.

**Decision. No document's signature may be written over another document
in the same batch, or over a path an earlier document in the batch has
already been promised.**

The rule is `jobs.CollidingOutputs`, beside `OutputPathFor` and
`LooksLikeOutput` — the package that already holds the other
output-naming facts, so that both front doors ask one question rather
than two that can drift apart ([[D-108]], [[D-124]], [[D-138]]). Like
`LooksLikeOutput`, it decides nothing about what to do; it answers the
question the front doors ask.

*The window skips those documents and says so.* Skipping rather than
renaming, for [[D-164]]'s own reason: the request is genuinely ambiguous
— is `faktura-signed.pdf` a previous run's output or a document being
signed? — and guessing destroys something. It is the same mechanism J-3
already uses (`jobs.ErrSkipDocument`) and the report carries its own
sentence in all three locales, rather than folding it into a count the
person cannot interpret.

*The collision is decided before anybody is asked anything.* The
intended paths are computed first, the collisions found, and only the
remaining documents produce an output-file question. A person asked
"replace it?" about a file that is going to be skipped whatever they say
is being asked a question with no answer.

*The command line refuses them and names them.* Reachable there only
with `--resign`, since `partitionAlreadySigned` drops the siblings by
default. It counts as that document's failure — "Potpisano 1/2" — and
does not fail the run, because F3 §12.10's batch policy is
skip-and-continue and a collision is not a better reason to abandon the
other ninety-nine documents than any other per-document failure.

**After the fix, the same measurement:**

```
--- after, --resign --force ---
  | Potpisano: ...\faktura-signed-signed.pdf
  | liro-bridge: sign-no-consent: ...\faktura.pdf: njegov potpis bi zamenio
  |   drugi dokument iz ove grupe
  | Potpisano 1/2 dokumenata
  after : faktura-signed-signed.pdf   66714 bytes  81188829
  after : faktura-signed.pdf            427 bytes  b9749fdb   <- untouched
  after : faktura.pdf                   427 bytes  b9749fdb
```

**What [[D-230]] should have said**, and what is true now: `--force`
answers the output-file question about files that are not part of this
batch, which is what a previous run's output is. It never answers a
question about a document the batch is signing, and no longer can.

**Tests.** `jobs.CollidingOutputs` has its own table, including the
three-deep case, two documents promised one path, and one directory
spelled two ways. In `cmd/liro-bridge`,
`TestNoOutputInABatchIsAnotherDocumentInTheSameBatch` and
`TestABatchNeverDestroysOneOfItsOwnDocuments` — the second with real
signatures, comparing every input's SHA-256 before and after the run —
both **measured failing before the fix**, with the message quoted above.
`TestTheCollisionIsDecidedBeforeAnybodyIsAsked` passes a nil window, so
a question asked would panic and the test passing is the evidence that
none was.

**One test defect this found in itself.** The `jobs` table originally
wrote its "one directory spelled two ways" case with literal
backslashes, which are separators on Windows and ordinary characters
everywhere else. It passed on Windows and failed on a real Linux kernel
— found by running the cross-compiled test binary there rather than by
cross-compiling it and stopping.

**Rejected.**
- **Writing beside it instead of skipping** — `faktura.pdf` becoming
  `faktura-signed-2.pdf`. It signs everything the person asked for, and
  it answers an ambiguous request by inventing a name they did not
  choose. [[D-164]] rejected guessing in this exact place.
- **Refusing the whole batch.** One ambiguous pair should not cost the
  other ninety-eight documents their signatures.
- **Narrowing [[D-230]]'s wording and changing nothing.** The phase
  offered that and it is the worse half of the choice: the behaviour is
  destructive whatever the entry says about it.
- **Fixing it only in the window flow.** The command line reaches it
  with `--resign`, and a defect fixed in one front door and not the
  other is how the two stop agreeing.
- **Making the collision an existence check.** A file that exists and is
  not in the batch is a previous run's output, which is exactly what
  `--force` is for. `TestCollidingOutputsIsNotADisguisedExistenceCheck`
  keeps that distinction.

---

## D-236 — What `sign` does on a machine with no reader: the window opens first, and it says which kind of nothing this is

**Date:** 2026-09-10
**Phase:** F10 — pre-phase fix

**How this was found, and it was not by reading the code.** F9's own
consent regression test —
`TestTheSignCommandOpensAWindowAndSignsNothingUntilItIsAnswered`, the
test that `sign` shows the consent window — passed on the machine it was
written on and failed the first time it ran anywhere else:

```
--- FAIL: TestTheSignCommandOpensAWindowAndSignsNothingUntilItIsAnswered (90.02s)
    signconsent_windows_test.go:117: no new visible window titled "Liro Bridge"
    appeared within 1m30s
```

That is [[D-221]]'s finding for the third time in this repository, and
the test is the smaller half of it. Measured on the runner itself, with a
probe reporting what each step of the flow actually did:

```
PROBE readers   elapsed=2ms   n=0  err=smart card service is not running
PROBE enumerate elapsed=2ms   n=0  err=<nil>
PROBE gather    elapsed=950ms rows=0 visible=0
                err=listing smart card readers: smart card service is not running
PROBE sign      window-appeared=false after=0s
PROBE sign      returned=1.011s code=1 stdout=""
PROBE log       {"level":"ERROR","msg":"signing flow: listing certificates failed",
                 "error":"listing smart card readers: smart card service is not running"}
```

**So it does not hang. It gives up in one second and says nothing.**
`SCardEstablishContext` returns `SCARD_E_NO_SERVICE` on a machine with no
reader — windows-latest has neither the hardware nor the service running
— `cli.Gather` fails, and `runSigningFlow` enumerated *before* it opened
anything, so a failure there returned 1 with no window to put it on. Not
a line on stdout. The one sentence that knew why went to `slog`, which
`run()` has by then pointed at a JSON file under `LOCALAPPDATA` — a
temporary directory the suite deleted on its way out ([[D-237]]).

**This is not a CI problem.** It is F10's stranger: somebody who installs
the agent before the reader is plugged in, or whose reader has no driver
yet, double-clicks a PDF and gets a second of nothing. It is also every
developer's first run. A console message would help only somebody who
started this from a console, and nobody double-clicking a PDF did.

**Decision. The window opens first, and it says which state this
machine is in.**

*(a) The window is created before the enumeration is asked about, not
after.* `runSigningFlow` starts the listing on its own goroutine and
opens the window; the answer arrives into the window's own loop
(`certificateListing`, a case in `mainWindow.loop`). Whatever the machine
turns out to be, there is a window. The two overlap rather than run in
sequence — measured here at 0.36s for the listing and 0.86s for the
window — so promptness was not traded for it.

*(b) The screen says which kind of nothing.* These are different
remedies, and a person needs to know which one they are in:

| State | Code | What the screen says |
|---|---|---|
| Nothing to put a card into | `NO_READER` | "No card reader detected. Connect the reader and try again." |
| A reader, and no card in it | `CARD_NOT_PRESENT` | "Insert your card into the reader." |
| The Windows service is not running | `SMART_CARD_SERVICE_DOWN` | "The Windows Smart Card service is not running." |
| A card, and nothing on it | `CERT_NOT_FOUND` | "No signing certificate was found on this card." |
| Certificates, none of them usable | (row's own) | nothing above the list; each row already carries its own reason |

The codes are SPEC §7's, not new ones. Two of them — `NO_READER` and
`SMART_CARD_SERVICE_DOWN` — had been in `internal/errs` since F1 and were
**never once produced by anything**: `platform.ErrSmartCardServiceDown`
travelled as a bare wrapped error and every caller above it saw an
unclassified failure. `cli.Gather` classifies it now
(`readerListingError`), and `cli.Report.NothingUsableReason` answers the
rest — one function on the report, where the reader states and the
hidden-row rule already are, so that the next screen with an empty list
to explain answers this the same way rather than a second way ([[D-108]],
[[D-124]], [[D-138]]).

The other front doors get the same answer for free, because it is the
same function. The tray window's certificate step says which state the
machine is in rather than asserting a card, and its failure screen —
reached when the enumeration itself fails — now renders
`error.smart_card_service_down` in the person's own language instead of
the untranslated developer sentence `errMessage` falls back to for an
error with no code on it.

*And a list with rows on it says nothing above itself.* This is the one
thing the tests had right and the product wrong, and only a photograph of
the real window showed it: with the card out of the reader, the screen
printed *"Ubacite karticu u čitač."* twice — once as the machine's
verdict above the list and once as the certificate's own reason under its
name. The rows are more specific than any summary can be, so the summary
is not drawn when there are rows. `consent.no_usable_certificate`, the
string this began as, now has no reader at all and is deleted from all
three catalogues; the element it lived in is `cert-notice` rather than
`no-usable-cert`, because that is what it is.

*(c) `sign` still exits non-zero.* A person sees a window and a
sentence; a script sees only the exit code, and a machine with nothing to
sign with is a failed `sign` rather than a refused one — which is what
the old path returned and what would otherwise have been lost in the
change. Closing the window *before* the listing has landed is a
different thing and stays 0: that is somebody who changed their mind.

*(d) The screen does not assert a card before it has looked.* The line
that carries all this used to be a static label the page resolved for
itself: `consent.no_usable_certificate`, "None of the certificates on
this card can be used for signing" — a claim about a card that may not
exist, shown whenever the list was empty for any reason, and never once
asserted by a test. It is Go's sentence now
(`jsConsentModel.CertNoticeText`), because only Go knows which of the
rows in (b) applies; and while the listing is still running it says
`consent.looking_for_certificates` instead of delivering a verdict on a
question nobody has answered yet.

**Rejected.**
- **A console message.** It reaches the one person who did not need it.
  `sign` is what the Explorer verb runs; there is no console.
- **Enumerating first and opening the window only on success**, i.e. what
  the code did. The behaviour it produces — no window, no message, one
  second — is the worst of the available ones, because it is
  indistinguishable from the program not having been started.
- **A "Try again" button that re-enumerates.** Genuinely useful for
  somebody who plugs the reader in while the window is open, and out of
  scope here: new UI surface, in three locales, with its own layout and
  its own failure modes, for a case the person can already answer by
  closing the window and double-clicking again. Recorded as worth doing
  rather than done.
- **Watching for card arrival and re-enumerating by itself.** The same
  idea one step further, and a bigger one: a `SCardGetStatusChange`
  watcher has a lifetime, a thread, and a way of being wrong. Not in a
  fix pass.
- **Failing the window onto the failure screen (`m.fail`).** That screen
  is for a batch that went wrong, and it is a dead end. "Plug the reader
  in" is not a failure of this batch; it is a state of this machine, and
  it belongs on the step it is about.
- **Opening a window on the protocol path too.** It keeps answering with
  a code and no window: a request from a paired application is a
  program's problem, and putting a window in front of a person to explain
  it is worse than telling the program (`runProtocolFlow`, SPEC §7).
  What did change there is *which* code — it returned a hardcoded
  `errs.CodeInternal` for every listing failure, which meant
  `SMART_CARD_SERVICE_DOWN`, a code `docs/PROTOCOL.md` documents with a
  422 and a remedy of its own, could not be produced by any request this
  agent has ever served.
  (`TestAProtocolRequestIsToldWhichKindOfNothingThisIs`.)

**Tests.**
- `internal/cli`: `TestTheReportSaysWhyItHasNothingToSignWith` (seven
  states), `TestHiddenRowsAreNotSomethingToSignWith`, and
  `TestAStoppedSmartCardServiceIsItsOwnCode`, which also checks that an
  unrelated reader failure does not borrow the code.
- `cmd/liro-bridge`:
  `TestTheSignWindowIsOnScreenBeforeTheCertificateListIs` holds the
  enumeration open and observes the window while the machine's answer is
  still unknown — an ordering observed rather than a duration timed
  ([[D-201]]).
  `TestTheCertificateStepSaysWhichKindOfNothingThisIs` opens the real
  window over each of the four listings and reads the sentence off the
  page.
  `TestTheCertificateStepNeverAssertsACardBeforeItHasLooked` and
  `TestALiveListSaysNothingAboveItself` is the photograph's own
  regression test, `TestConsentWindowFitsWithEveryNoCertificateNotice`
  puts every one of the sentences through the layout assertions in all
  three locales,
  `TestAUsableCertificateStillGetsNoNotice` covers the other end, and
  `TestSignExitsNonZeroWhenThereIsNothingToSignWith` covers what a script
  sees.
- The states are reachable in a test because the enumeration is a field
  on the window with a package variable behind it (`interactiveGather`),
  the same seam `auditStore` already is. This machine has a reader and
  cannot be put into any of those states; a check that could only run
  where the hardware is absent would be [[D-221]] again with the
  platforms swapped.

**Measured on the runner after the change**, by the same probe:

```
PROBE gather    elapsed=688ms rows=0 visible=0
                err=SMART_CARD_SERVICE_DOWN: listing smart card readers: ...
PROBE sign      window-appeared=true after=5.644s
```

and the Windows job's Go step went from a ninety-second timeout and a
red build to 77 seconds and a green one.

**5.6 seconds is not "within a second or two", and it is worth being
exact about what it is.** All of it is `ui.NewWindow` — the agent's own
log puts "liro-bridge starting" at 11:16:06.008 and the window's first
drop-target registration at 11:16:11.637, with the enumeration long
since finished at 688ms and waiting. That is a cold Edge and no GPU on a
CI runner; the same window measured 0.86s here. Nothing in this change is
in that path, and nothing was made slower by it — but the claim that
holds everywhere is "the window is not waiting on the card", not "a
window inside two seconds", and a machine slow enough to make WebView2
take six seconds will take six seconds.

**Measured against the pre-fix trees.** At 58a6a23, in a worktree, the
regression test still fails — and now fails in 0.04s with the reason
attached instead of after ninety seconds without it:

```
liro-bridge: sign: --thumbprint je obavezan.
    signconsent_windows_test.go:156: sign returned 2 without ever showing
    a window; nobody was asked and nothing said so
```

At cd7b403, the certificate step with no reader, no card and no
certificate was measured saying, in full: *"Nijedan sertifikat na ovoj
kartici se ne moze koristiti za potpisivanje."* — nothing on the card
that is not there.

---

## D-237 — The agent's own log reaches CI, and a failed test keeps the directory that holds it

**Date:** 2026-09-10
**Phase:** F10 — pre-phase fix

**What [[D-236]] had to be found around.** The Windows job's failure said
what had not happened and had no way of saying why. `config.SetupLogging`
calls `slog.SetDefault` over a JSON handler pointed at
`%LOCALAPPDATA%\Liro\logs`; the suite points `LOCALAPPDATA` at a
temporary directory (`tempConfigHome`) and deleted it on the way out —
including on the way out of a failure. So the one line that explained the
failure was written, and then deleted, on every red Windows build this
project has ever had. The only thing to do with such a failure is re-run
it, which is how the one that matters gets ignored.

**Decision, in two parts, and neither of them is a debugging aid.**

*(a) `LIRO_DEBUG=1` on the Windows test step.* F0 §10 already defines
that switch as "text to stderr as well as JSON to the file"; setting it
in CI is what puts the agent's own account of itself into the same log
that says the job went red.

*(b) A failing test keeps its config home, and the job uploads it.*
`tempConfigHome`'s cleanup returns without deleting when `t.Failed()`,
and an `if: failure()` step collects every surviving `Liro` directory and
uploads it as an artifact. That carries what stderr cannot: the JSON log,
the audit chain and the config as the run left them.
`sweepStaleConfigHomes` still collects these an hour later, so this does
not reintroduce C-6's accumulation — it delays it by an hour, which is
longer than any CI job and shorter than any developer's afternoon.

**One thing measured, because it contradicts the premise this was chosen
on.** Artifacts are *not* downloadable without authentication.
`GET /actions/artifacts/{id}/zip` answers 401 to an anonymous request
against this public repository, and the browser URL 404s; job logs answer
403 ("Must have admin rights to Repository"). What *is* public is the
check-run annotations API — which is why the measurement quoted in
[[D-236]] was taken by having a probe step emit its results as
`::warning::` annotations and reading them back over that API. The
artifact is for a person signed in to GitHub, which is who reads a red
build; it is not a way around authentication.

---

## D-238 — The release key: two slots, generated by the owner and never by the agent, and what happens when the primary is lost

**Date:** 2026-09-11
**Phase:** F10

**Decision.** There are two Ed25519 keys. Both public halves are
embedded in `internal/update/trustedkeys.go` from the first release, so
that every agent ever installed trusts both.

| | Private half lives | Has signed |
|---|---|---|
| **primary** | the `LIRO_RELEASE_SIGNING_KEY` secret of the `release` deployment environment, and nowhere else | every release |
| **spare** | offline, off this machine, off CI | nothing, ever, until it has to |

**Why two.** Every installed agent trusts what it was built with and
can never be told to stop. F10 §4.1 asks what happens if the key is
lost or compromised, and with one slot the honest answer is that every
user reinstalls by hand — discovered at the worst possible moment,
which is the moment the answer is needed.

**The loss and compromise answer, in full.**

*If the primary is lost* — the secret is gone, the machine holding it
died, nobody can sign. The next release is signed with the spare.
Every agent in the field accepts it, because it has trusted both since
it was installed, and nobody reinstalls anything. That release ships a
build whose embedded set is the spare plus a newly generated
replacement spare, and from then on the arrangement is as it was.

*If the primary is compromised* — the same two steps, and one more
before them: the primary must stop being able to publish. Deleting the
environment secret is what does that, and it is the first action, not
the last, because until it happens whoever has the key can publish a
release every installed agent will accept. Agents already installed
cannot be told to distrust it, so the window closes only as they update
to a build whose trust set no longer contains it — which is why the
replacement release matters more than the revocation does.

*What neither case covers.* An agent that never updates goes on
trusting a compromised key for as long as it runs. There is no
mechanism in this design that reaches it, and inventing one — a
revocation list the agent fetches — would mean a second thing the agent
trusts over the network, which is a larger attack surface than the one
it closes. Recorded as the limit rather than papered over.

*The spare is not a backup of the primary.* It is a second key. If both
are lost the answer really is that every user reinstalls by hand, and
the arrangement does not pretend otherwise.

**Who generates it, and why that is part of the decision rather than a
detail.** The owner generates the pair on his own machine and pastes
back the two public halves. The agent has never generated, printed,
held or seen a private half.

The previous session's pair is discarded — not because anything leaked,
but because it was generated inside an agent session and its
cleanliness can no longer be established. The owner's own framing, and
it is the right test: a key that can publish an update every installed
agent accepts without argument is not a key whose provenance should
rest on a claim.

The instruction this phase was given said to print the halves once.
That was raised rather than followed: anything printed through this
harness lands in the session transcript on disk and travels to a model
API, so "printed once" persists it in two places neither party
controls. The owner's answer was to generate it himself. **Recorded
because the reasoning generalises: for a secret, the agent's own output
channel is a place of record, and "do not write it to a file" and "do
not let it through the agent" are different requirements.**

**The generator was checked by running it, not by reading it.**
`scripts/genreleasekey` is a developer tool, imported by nothing that
ships. Three properties, each measured:

| Property | How |
|---|---|
| Writes only to the path it is given | One file created; `git status` unchanged; nothing new under `%TEMP%`; both private halves present in the file and **absent from stdout and stderr**, checked by string match |
| Prints pasteable, labelled public halves | `primary` and `spare`, one per line |
| Needs no network and no configuration | `go list -deps` reports stdlib only; run with `GOPROXY=off`; reads nothing but `go.mod` while walking up for the module root |

**Two defects it had, both found in that hour.**

*The in-repository guard did not cover a dot-directory.* `inRepository`
asked whether the relative path *began with a dot*, intending to catch
`..`. Measured: `--out .github/probe.json` **wrote a real key pair into
the repository**, and `git check-ignore` confirmed nothing would have
stopped `git add .` from staging it. The test is now whether the
relative path is `..` or begins with `../` — about leaving the root
rather than about the first character of a name.

*The printed half and the stored half were never checked against each
other.* They are two encodings made at two moments, and the consequence
of their disagreeing is not a failed build but a public key embedded in
every installed agent whose private half nobody has. `roundTrips` now
decodes both back out of the exact strings about to be written and
printed, checks that the public half is the one the private half
derives, and does a sign-and-verify round trip — before anything is
written.

That check earned itself immediately, against its own author:
`ed25519.Verify` takes `(publicKey, message, sig)` and the first
version passed `(publicKey, sig, message)`. Both orders compile,
because the last two parameters are both `[]byte`, and the wrong one
rejects everything. It was diagnosed by feeding it **RFC 8032 §7.1's
own test vector**, where `ed25519.Sign` reproduced the published
signature byte-for-byte and `Verify` refused it — which is not a
property any correct implementation can have, and is what said the
fault was in the caller. `internal/update/signature.go:163` has the
order right, which is why the agent's own tests never noticed.

**Rejected.**
- **One key.** The answer to F10 §4.1 becomes "everybody reinstalls",
  and it is an answer nobody gets to prepare for.
- **Three or more slots.** Each one is another key whose private half
  has to be kept somewhere, and the second slot already buys the whole
  recovery story. A third is a place to lose a key, not a place to find
  one.
- **Keeping the spare in CI "so a release can be made either way".**
  Then there is one key with two names. Putting the spare into CI is
  the act of retiring the primary, and it is deliberately a manual act.
- **A revocation list the agent fetches.** Above: it reaches the agents
  that update, which are the ones the replacement release already
  reaches, and it adds a second network-trusted input to a program
  whose outbound requests SPEC §6.8 enumerates.
- **Letting the agent generate the pair and write it outside the
  repository**, which is what the previous session did. It is the
  option this phase was originally told to take, and the owner's
  reasoning against it is recorded above.
- **Printing the private halves once.** Above.

---

## D-239 — The release signing key is reachable only from a tag, and the job that can reach it does nothing else; `signrelease` wrote its output before checking it

**Date:** 2026-09-11
**Phase:** F10

**Decision.** `.github/workflows/release.yml` is two jobs.

```
build     tag push and workflow_dispatch.  Never holds the release key.
          Builds the three artefacts, checks the binary reports the tag,
          checks that signing refuses without a key, uploads what it made.

release   tag push only.  environment: release.  The only place the
          release signing key exists.  Takes the build's own artefacts,
          signs the manifest, verifies it as the agent will, publishes.
```

The `release` environment carries a deployment rule admitting only refs
matching `v*`. That rule is in repository settings, not in this file,
and the file says so where it matters: **this workflow cannot enforce
it and cannot show whether it is on.**

**Why an environment rather than a repository secret.** A repository
secret is readable by any job in any workflow run on any ref the
workflow triggers on — including `workflow_dispatch`, which this
workflow has and which runs from a branch. An environment secret is
readable only by a job bound to that environment, and the
environment's own rule decides which refs may bind to it. That is the
difference between "the key is used only on a tag because the file says
so" and "the key cannot be reached except from a tag", and F10 §4.1
asks for the second.

**What it cost, and why the cost is the right shape.** A dispatch can
no longer sign anything, so the pipeline can no longer be exercised end
to end without cutting a tag. That is not a regression being tolerated
— it is the property being bought. What a dispatch still proves is
everything up to the signature: both MSIs, the plain EXE, and that
`liro-bridge --version` reports the tag.

**A step the split made possible, and it found something.** The build
job is the one place the release key is *guaranteed* absent, so it is
the only place `signrelease`'s refusal can be observed. It now asserts
that signing without a key exits non-zero **and leaves no `release.json`
or `release.json.sig` behind**. Both halves were confirmed able to fail:
planting a `release.json` makes it report the file it found, and a run
that signs successfully trips the other branch.

Writing that assertion is what exposed the defect below.

**`signrelease` wrote both files and then checked them.** The order was
build the manifest, sign it, **write `release.json` and
`release.json.sig`**, then verify the signature against the trust set
the agent embeds. Measured, by signing a directory with a freshly
generated key that is not in that set: the tool exited 1 and both files
were sitting in the directory afterwards, verifying against nothing.

The release workflow happens to catch it one step later —
`verifyrelease` runs next and fails — so nothing could have been
published. It is still wrong: a tool that fails should not leave its
output behind for the next thing along to pick up, and the next thing
along is `gh release create dist/release/*`. The verification now
happens before either `WriteFile`; re-measured with the same untrusted
key, the directory afterwards holds the artefact and nothing else.

**Authenticode stays in the build job, and its protection is
deliberately weaker.** It cannot move: `build.ps1` signs the staged
executable *before* WiX embeds it in the two MSIs, so signing the
finished artefacts afterwards would produce packages whose own copy of
the program is unsigned. The certificate is therefore handed to the
build job, and only on a tag push — an expression in the workflow file
rather than a platform guarantee. The difference is written at the
point it is applied, because the two must not be read as equally
strong: an edit to that line lifts it, where an edit to this file
cannot lift a deployment rule. It is the right strength for what it
protects, since anyone who can dispatch this workflow can also push a
tag, and there is no certificate today in any case (SPEC §15.1).

**Not verified from here, and stated rather than implied.** Whether the
environment exists, whether its deployment rule is set to `v*`, and
whether the secret is in it rather than at repository level are all
repository settings this machine cannot read and this file cannot
assert. The first tag pushed after the environment is created is what
establishes it. Until then the release job will fail at the signing
step with an empty key — which is the correct failure, and better than
the alternative shape, where a missing environment silently falls back
to a repository secret.

**Rejected.**
- **One job with `environment: release`.** A dispatch would then be
  refused at job start and build nothing, which removes the dispatch
  path's whole value.
- **Rebuilding the artefacts inside the release job.** Two builds of
  one tag are two different sets of bytes with one set of digests
  claimed over them; the artefacts that get signed have to be the ones
  that were built and checked.
- **Moving Authenticode into the release job.** Above: it would ship
  MSIs containing an unsigned binary, which is worse than the exposure
  it closes.
- **Adding a flag to `verifyrelease` that trusts an extra key, so a
  dispatch could exercise signing with a throwaway one.** A flag that
  adds trust, in the one tool whose job is to refuse anything the agent
  would refuse. The sign-and-verify round trip is already covered by
  `internal/update`'s own tests with generated keys.

---

## D-240 — The resize wait observes the resize; what failed by "one pixel" was a tolerance finer than the layout's own granularity, reported rounded

**Date:** 2026-09-11
**Phase:** F10 — carried over from F9b (F10 §7)

**The question F10 §7 asks, answered first.** *Does the resize wait
observe the resize or wait a fixed time?* **It observes.**
`resizeAndSettle` calls `Window.Resize` and then reads
`window.innerWidth`/`innerHeight` back out of the page in a loop until
they are the size that was asked for. Each turn is a real round trip, so
nothing spins, and the thirty-second deadline exists only to turn a
window that never resizes at all into a failure rather than a hang. That
is [[D-201]]'s own remedy and it is unchanged by this entry. **No
timeout was raised and none needed to be.**

**So the flake is not the resize.** Two measurements say so.

*The page cannot overflow at all.* The pairing window's viewport was
walked from 365 to 371 points with the 120-character name in place. At
every height `document.body.scrollHeight` equals `clientHeight`, and the
page cannot be scrolled by any amount — `scrollTop` set to 10⁶ comes
back 0 at each one. `height: 100vh` and the one region allowed to shrink
([[D-106]], [[D-202]]) absorb the whole difference; the identity block
took it up point for point, 136.016 at 365 through 142.016 at 371. A
page that structurally cannot scroll cannot fail a scroll assertion by
one pixel.

*Every box on that page is fractional.* Measured, same window:

```
.identity 140.016   .field 87.969    .field-label 16.797
.field-value 67.172 .code-block 105.984
.pairing-code 47.594  .code-note 33.594
```

So an edge lands on a fraction and a viewport is an integer, by
construction, on every screen this project measures.

**What was actually wrong, and it is two things.**

*The tolerances were finer than the fractions in play.*
`assertButtonsVisible` and `assertNameStartsInsideTheWindow` compared a
rendered edge against the viewport with a **half**-pixel tolerance, on a
layout whose boxes carry fractions up to `.984`. A tolerance below the
granularity of the thing being compared fails on arithmetic rather than
on anything a person could see. Both now use one whole CSS pixel, named
once as `layoutEpsilon`, and `assertPageDoesNotScroll`'s page-level
comparison gets the same one-pixel tolerance its own per-element check
has had since it was written — the two sat in one function disagreeing
about how precise `scrollHeight` is.

*The failure was reported rounded, which is why it reads as one pixel.*
`assertNameStartsInsideTheWindow` formatted every number with `%.0f` and
`assertButtonsVisible` with `Math.round`. A bottom edge at 369.6 in a
369-point window prints as "at 353..370 of 369" — indistinguishable from
a real one-pixel overflow, and unactionable either way. Every one of
these now prints three decimal places. **That is the half of this fix
that matters most: it is why the F9b failure could not be diagnosed from
its own message, and it is what makes the next one diagnosable in one
reading.**

**This is not an assertion being loosened.** Nothing these tests guard
against — a button below the fold, a name scrolled off the top, a page
that scrolls when it must not — is ever wrong by one pixel. [[D-202]]
measured the real cases: `#app-name` at **−8**, `#connected-name` at
**−53**, a page needing 354 in 330. Those are tens of pixels. A
one-pixel tolerance does not reach any of them.

**Not reproduced, and said plainly.** 25 consecutive runs of
`TestALongApplicationNameStaysReadableInThePairingWindow` and
`TestAnOrdinaryNameLeavesTheIdentityBlockUnscrolled` pass on this
machine, which reports `devicePixelRatio` 1. The F9b occurrence was one
run in an unknown number on an unknown display scale, and no record of
its message survives beyond F10 §7's summary of it — which is itself the
argument for the reporting change. What is established is which
assertions in that test are structurally capable of a one-pixel failure
and which are not; the fix is applied to the ones that are.

**Rejected.**
- **Raising a timeout.** Forbidden by F10 §7 and wrong regardless: the
  wait already observes the property, so there is no duration in it to
  raise.
- **Leaving the half-pixel tolerances and re-running on failure.** That
  is the habit F10 §7 exists to stop — "a test that fails one run in ten
  becomes a release that fails one attempt in ten, and a suite people
  learn to re-run instead of read".
- **Removing the tolerance entirely and comparing fractional values
  throughout.** There is no fractional `scrollHeight` in the DOM to
  compare against; the number is rounded before any test can see it.
- **Replacing the scroll comparison with an attempt to scroll the page
  and see whether it moves.** It observes the property directly, which
  is attractive, and it is weaker: an element with `overflow: hidden`
  clips its content and cannot be scrolled, so the observation would
  pass while content was being cut off. `scrollHeight` against
  `clientHeight` catches clipping and scrolling both.
- **Widening the windows so nothing is near a boundary.** [[D-106]]'s
  standing answer: widening is not a fix, it is a delay.

---

## D-241 — The packaging build runs on every push, and what a hosted runner can and cannot say about an installer

**Date:** 2026-09-11
**Phase:** F10

**Decision.** `.github/workflows/ci.yml` gains a `packaging` job on
`windows-latest`: it runs `build/msi/build.ps1`, asserts the three
artefacts exist and that the binary reports `dev`, and opens both MSIs
through `WindowsInstaller.Installer` to confirm Windows Installer itself
can read them.

**Why.** Until now `build/msi` ran in exactly one place — the release
workflow, on a tag. So the first thing a broken WiX source, a renamed
asset or a missing file would break was a release: the one build where
being wrong costs the most and where nobody has time to read the log.
That is [[D-221]]'s finding pointed at a build rather than at a test — a
check that only runs where it was written — with the aggravation that
this one only ran when it was most expensive to be wrong.

**It builds a development version deliberately.** Omitting `-Version`
exercises the other branch of every version decision in `build.ps1` (the
binary reports `dev`, the MSIs are stamped `0.0.0`), and it means this
job can never be mistaken for a release or produce something that looks
like one.

**What a hosted runner can actually establish, stated rather than
implied.** It cannot install the per-machine package — that needs
elevation. It cannot run the per-user one usefully — there is no reader,
no card, and no smart card service ([[D-236]] measured exactly that).
So this job inspects what the build produced, and this project has
recorded four times that inspecting the build is not the same as
installing it ([[D-161]], [[D-200]], [[D-219]], [[D-221]]).

It is still worth having, and the reason is what it catches rather than
what it proves: a package that does not build, a package Windows
Installer will not open, an artefact whose name changed, a binary that
does not report its own version. Every one of those is a release that
fails at the tag today. **A real install remains a thing done by hand on
a real machine, and the phase report says so in those words rather than
letting a green tick stand in for it.**

**The Windows Installer read is not decoration.** `build.ps1` already
runs WiX's own ICE validation with three checks suppressed for stated
reasons, and an MSI can pass all of that and still be a file the
installer refuses. Opening the summary information stream through the
same COM interface the installer uses is the cheapest check that is not
WiX marking its own work, and the platform/language template
(`x64;1033`) is what a package missing it fails on.

**The WiX cache key is the archive's own digest.** `build.ps1` pins WiX
3.14.1 by SHA-256 and fails if what it fetched does not match, so a
cache hit and a fresh download are the same bytes or there is no build.
Keying the cache on that digest rather than on a version string means a
changed pin can never be served a stale toolset. The cache is a saving,
never a source of truth.

**Measured before it was added**, rather than written and pushed: the
whole job was run on this machine. `build.ps1 -Out dist/ci` took 61.9 s
including the WiX fetch and produced `liro-bridge-dev-x64.msi`
(4,575,232 bytes), `liro-bridge-dev-x64-per-machine.msi` (4,575,232) and
`liro-bridge-dev-x64.exe` (11,357,184); `--version` reported
`liro-bridge dev (commit 67d39a6, …)`; and both MSIs opened through the
COM interface reporting `x64;1033`. The two packages are the same size
and different digests, which is what two builds of one source with one
property changed should look like.

**Rejected.**
- **Adding these as steps on the existing `windows` job.** That job is
  `go vet` and the test suite, and it already runs with the `softtoken`
  tag; a red packaging build should say "packaging" rather than sending
  somebody to read a test job's log. The same reasoning `sdk-typescript`
  already has its own job for.
- **Building the versioned artefacts in CI too.** It would test one more
  branch and produce files that look like a release on every push. The
  release workflow builds those, and its own `--version` check is what
  covers that branch.
- **Installing the per-user MSI on the runner to prove more.** Tempting,
  and it would be a real install — but what it would exercise is a
  machine with no reader and no smart card service, which is the one
  configuration [[D-236]] already covers, and the artefact it installs
  is one no stranger will ever see. The install that matters is the one
  on a machine with a card in it.

---

## D-242 — The guide is Serbian and English from one set of screenshots; the Serbian is the original and the English says so on its own first screen

**Date:** 2026-09-11
**Phase:** F10

**Decision.** Two documents, `docs/guide/Uputstvo.html` (`sr-Latn`) and
`docs/guide/Guide.html` (`en`), one folder of screenshots
(`docs/guide/slike`) referenced by both, and both installed beside the
program.

**Which is authoritative: the Serbian one.** It is the original text;
the English is a translation of it. That is stated in three places —
this entry, a note at the top of the English document, and the footer of
each — because a reader who only ever opens one of them is the reader
the statement is for.

The reason is not that Serbian is the default locale (SPEC §9.1), though
it is. It is who the document is for. Every person this guide describes
holds a Serbian qualified certificate, from MUP, Pošta Srbije or Halcom,
and reads a Windows in Serbian. The English document exists for a
developer, an administrator deploying by Group Policy, or somebody whose
Windows is in English — a real audience, and the second one. When a
sentence has to be got exactly right, it gets got right in the language
of the person who is going to act on it.

**`sr-Cyrl` is not a third document, and that is a departure from SPEC
§9.1.** The interface has three locales and the guide has two. The
reason is that a screenshot set has a locale in it, and this one is
`sr-Latn`: a Cyrillic document illustrated with Latin screenshots would
be a document whose pictures contradict its text on every page, and
three documents means three that drift. A Serbian reader who runs the
interface in Cyrillic reads Latin without difficulty; the reverse
direction — pictures in the wrong script — is the one that costs
something. Recorded as a departure rather than presented as the obvious
reading.

**One screenshot set, and its locale is recorded because it is a fact
about the artefact.** The screenshots are `sr-Latn`, which is the
interface's own default, and they are referenced by both documents
rather than duplicated. The owner's instruction is the reasoning: a
dialog is a dialog in either language, and two sets are two sets to keep
in step. The English document therefore shows Serbian windows and names
the buttons in both — `Izaberi…` (Browse), `Odobri` (Approve) — which is
what a person with a Serbian interface in front of them actually needs
matched up.

**Relative references rather than images embedded in the HTML.** A
self-contained document would survive being emailed on its own, and it
would mean the same eight pictures existing twice, regenerated twice,
and the two drifting. The guide is installed beside its `slike` folder
and opened from the Start menu shortcut, which is how it is read.

**The build refuses rather than shipping a guide with holes in it.**
`build.ps1` checks both documents and each of the eight images before
WiX runs, and fails naming the missing file and the command that
regenerates it. It also fails if the folder holds a picture the WiX
source does not install, because the source names the eight explicitly
and a ninth added to the folder alone would ship in neither package —
a broken image discovered by a reader rather than by a build.

**What is still missing from the guide, said plainly.** The SmartScreen
warning is described in words, with what to click and a warning against
clicking it out of habit, and **has no screenshot**. SPEC §15.1 and F10
§6 both require one. Producing it means running an unsigned binary
carrying a Mark-of-the-Web on the owner's own desktop so that Windows
shows the dialog, which is a step outside this project's own files and
windows and is therefore his to authorise. The section is written and
the figure is the one thing outstanding.

**Two WiX consequences, recorded because both were errors first.** The
guide component now holds two files and the images component holds
eight, and WiX refuses `Guid="*"` for a component with more than one
file unless its keypath is a versioned file — an HTML document is not
one. Both carry fixed GUIDs now, like every other authored component in
the source. And the images live in their own `Directory` named `slike`,
because that is the folder name the documents' own `img src` attributes
use; the directory is removed on uninstall with the rest.

**Rejected.**
- **`sr-Latn` only.** F10 §6 leaves the choice open and the owner
  settled it: English as well. An administrator deploying this by Group
  Policy is not necessarily a Serbian speaker.
- **All three SPEC §9.1 locales.** Above: a third document needs a third
  screenshot set or it contradicts its own pictures.
- **Two screenshot sets, one per language.** The owner's instruction,
  and right: two sets drift, and the drift shows up as a picture that no
  longer matches the words beside it.
- **Embedding the images as data URIs.** Above.
- **Making the English document the authoritative one** on the grounds
  that the rest of this repository is English (SPEC §0). Rejected: SPEC
  §0's rule is about code, comments and documentation for developers.
  This is the one document in the project written for somebody who will
  never read any of that, and it is written for a Serbian signer.

