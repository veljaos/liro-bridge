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
