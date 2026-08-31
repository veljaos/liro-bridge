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
