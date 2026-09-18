# F12 — handover 2

**What this is:** what `F12.md` and `f12-handover.md` do not say. F12.md is the
phase; `f12-handover.md` covers the session before this one, which built §2 to
the PIN seam's edge. This covers the session that closed §2 and measured it
against real hardware.

**Written:** 2026-09-18, end of day, at the office machine.
**Entries this session:** D-299 through D-306. Read them rather than this for
the reasoning; this says where things are and what to do next.

---

## 1. §2 is closed

Built, guarded, mutation-tested, and — this is the part that was missing until
today — **measured against a real card**:

- **The Enumerate refactor.** `enumerate`/`list`/`chainFor` take an already-open
  module; the exported one-shots are those with an open and a close around them.
  [[D-299]].
- **The child's dispatch loop.** One `LockOSThread` goroutine, no channel,
  because the PIN arrives on the same reader. [[D-300]].
- **The parent's supervisor.** Respawn bounded by a count and not a clock;
  which operations may be re-sent is an allow-list. [[D-301]].
- **The PIN seam.** Two phases, because only the child can see
  `CKF_PROTECTED_AUTHENTICATION_PATH`; an exchange identifier binds a PIN
  question to an approved operation; a question with nothing pending kills the
  worker. [[D-302]].
- **A data race CI found and inspection did not**, in the writer every test was
  reading. [[D-303]].
- **The reap backstop.** §4 below. [[D-306]].

**Measured on real hardware today**, which nothing before this had been:

- a real module held open answers exactly what a one-shot open answers, same
  bytes, same order, both NetSeT builds and SafeSign;
- D-297's claim is visible — ~250 ms once for spawn plus `C_Initialize`, then
  reads an order of magnitude cheaper than paying the open each time;
- three workers opened and closed in sequence over one module, no failures.

`internal/keysource/pkcs11/worker/realmodule_windows_test.go` is that test. It
is env-gated (`LIRO_PKCS11_MODULE`, `LIRO_PKCS11_WORKER_CARD`), read-only, and
spends no PIN attempt.

### The one §2 requirement that genuinely lives in F11 §4

F12 §2 lists **"a worker that dies is a `Failure` in a list"**. That cannot be
finished in §2, because nothing calls `Worker` yet — `main.go` imports it only
to dispatch the subcommand. The `Failure` is produced where the listing is
produced, and the listing is F11 §4's. So §2 ends with the supervisor turning a
dead worker into `ErrWorkerDied`, and §4 turns that into a row.

The other §2 requirement worth knowing is already discharged:
`cmd/liro-bridge/pkcs11reach_test.go` — the gate stopping the agent importing the
backend — was deleted in `3ae0b86` when the probe subcommand landed. F12 §2 says
deleting it is part of doing this. It is done.

---

## 2. F11 §4, in the order I would do it

**Nothing of it is started.** Deliberately: it is a wiring change across four
packages and half of one is worse than none.

1. **`Source` reaches the module through `Worker`.** Today `internal/keysource/
   pkcs11` loads modules in-process and nothing outside `main.go` imports it.
   This is the change that makes §2's whole argument true of the agent rather
   than of a test. Do this first and alone: everything below depends on its
   shape.
2. **A dead worker becomes a `Failure` in a list** — §2's own requirement,
   above. F11 §3 already defines what discovery's `Failure` looks like; this is
   giving `ErrWorkerDied` and `ErrWorkerAbandoned` somewhere to land with the
   module path attached.
3. **One certificate to one row across two backends.** `windowscng` and
   `pkcs11` will both see the same physical card. `dedupe` in
   `certificate.go` already collapses sightings *within* pkcs11 and its comment
   records that the thumbprint is byte-identical to what `windowscng` computes
   for the same bytes — which is what makes the cross-backend collapse possible
   at all. The decision that is not yet made: **which source signs** when both
   offer the same certificate.
4. **What the audit log records.** Which backend, which module path, and — new
   since this morning — the fact that two builds of one vendor's module behave
   very differently (§3 below), which is exactly the kind of thing a support
   question a year from now will need.

### The one thing in it that needs your hands

**Step 3's verification.** One card, both backends, one row — and whether the
row is the same row needs a person looking at a screen with a card in the
reader. Everything else in §4 can be built and tested without hardware; that
cannot.

---

## 3. The 855 ms, which becomes a person waiting the moment §4 lands

[[D-305]] has the measurement and the argument. The short form:

`C_FindObjectsInit` costs **855.8 ms** on NetSeT 1.1.3.3 (`TrustEdgeID`) and at
most 33 ms on NetSeT 1.1.0.0 (`MUP RS\Celik`) — same vendor, same card, same two
certificates. It is **fixed per search**: `C_FindObjects`, `C_FindObjectsFinal`
and all twelve `C_GetAttributeValue` calls are below the clock floor, including
two certificate bodies of 1910 and 1655 bytes.

`Source.openOn` takes **three** searches — `findCertificateObject`,
`privateKeyFor`, `allCertificateDER`. That is **~2.6 s per signature** on 1.1.3.3
against ~100 ms on 1.1.0.0, and which build a person has is an accident of what
their issuer's installer left behind ([[D-271]] found both on one machine, five
years apart).

**What I think should be done, unchanged from D-305:**

- **Collapse `findCertificateObject` and `allCertificateDER` into one search.**
  They ask the same question — every certificate object — and the first is the
  second plus a filter this layer already applies in memory. Three searches
  become two, ~2.6 s becomes ~1.7 s, no behaviour changes.
- **`privateKeyFor` stays a second search and must.** It asks a genuinely
  different question and cannot run before the login: a public session sees zero
  private keys, measured on this card ([[D-271]]).
- **Whether the remaining two can become one is unmeasured** and I would not
  guess. It depends on whether one broad `C_FindObjectsInit` is cheaper than two
  narrow ones on that module.
- **Do not simply accept 2.6 s.** Not because it is intolerable but because it
  is *invisible*: nothing on screen says which module is being read, and a
  person on the slow build has no way to know the same card through a different
  DLL is twenty-seven times faster. If it stays, the program should be able to
  say so.

**Order:** the PIN entry (§5) before this. The collapse is a performance change
and the login is a correctness one, and the home list's item 2 is blocked on the
login rather than on the speed. The owner ruled this.

---

## 4. The backstop

[[D-306]] is the entry and it was ruled by the owner against three measured
numbers. What a reader needs to know without opening it:

- **10 seconds**, on `Worker.reap`'s waiting.
- **Both of reap's waits**, not only the unbounded one. `endLocked` kills
  *before* calling `reap`, so reap's first wait is already a wait for a killed
  child; bounding only the named line would have left the other.
- **Not per request.** Every request is still bounded by its caller's context
  and by nothing this package invented. **[[D-297]]'s per-request deadline
  question stays open and this does not answer it.**
- **Logged loudly** — module, operation, elapsed, and that the child was killed
  — with its own sentinel `ErrWorkerAbandoned`, because `ErrWorkerDied` is a
  process that ended and this is one that would not.
- **The firing arm has no test and cannot have one.** Mutation B2 removes it
  outright and *survives*: the whole suite passes, card in, because nothing in
  this project can produce a child that will not be reaped. Two tests assert it
  never fires on ordinary work and are killed by setting the bound to 1 ns — so
  a green suite is evidence the backstop does not fire *wrongly*, which is a
  different claim from evidence that it works. Do not read it as the latter.
- The `C_Finalize` explanation for the unreapable processes **is recorded as
  wrong**, not dropped. A killed child does not finalise either and reaps in
  2–5 ms. The mechanism is unknown.

---

## 5. The home list

`docs/f12-home-list.md`. Two items, and **item 1 decides whether item 2 measures
anything**:

- **Item 1** asks whether the Pošta token offers a protected authentication
  path. Free, read-only, under a minute. If it reports `protectedPIN=true`, then
  SafeSign collects the PIN itself, `C_Login` is called with NULL, and the
  asking half of the PIN seam is never reached on that card — item 2 as written
  would measure nothing and needs rewriting first.
- **Item 2** is one login through the worker, and it **is not written yet**. It
  needs a PIN entry that does not exist: the real PIN screen is `internal/ui`,
  which the worker's contract test forbids the worker package from reaching, and
  `go test` is not a reliable place to type a PIN. It belongs in a
  `scripts/p11worker` beside `scripts/p11probe`, copying p11probe's two guards
  exactly — refuse to call `C_Login` unless all three user-PIN flags are clear
  beforehand, read them again immediately after ([[D-268]]). One attempt, no
  retry, no code path that could take a second.

The step-by-step goes to the owner as one message **before** anything is typed.

---

## 6. The machine, as it stands

**Six processes that will not die**, counted at the end of the session:
`p11probe` PIDs 2916, 4868, 16956, 18924, 32656 and 37164. Reported to the owner
as four earlier in the day, which was the count at that moment; the later
`--time --objects` and slot-check runs added two.

Each loaded a NetSeT module, each reports `HasExited=true`, each sits in the
process table with one thread in `Wait`/`UserRequest`, and each survives
`TerminateProcess`. **Only a reboot clears them.** The owner knows and has
chosen when.

They are untidy rather than harmful, and that was checked rather than assumed:
they arose with the card out, and with the card back in afterwards every module
still saw the token. The mechanism is unknown — see [[D-306]], where the
plausible explanation is recorded as wrong.

**Every one of them came from `scripts/p11probe`, not from the worker.** No
worker child has ever failed to reap, in any configuration tried. Anyone running
p11probe repeatedly should expect to accumulate these.

**Verified against the snapshot at the end of the session**: `Run` key identical
value by value, Explorer verb key byte-for-byte, config tree unchanged, ten crash
dumps intact and none added.

### Traps

- **`-count=1` in anything run more than once.** `go test` caches a successful
  result and replays its output verbatim; a cached run cannot detect that it is
  cached. Three invocations of a card test printed numbers identical to the last
  decimal and were one run shown three times. `(cached)` on the package line is
  the tell. It is now in this project's example commands and in the home list.
  [[D-304]], question 4.
- **This machine cannot time anything under ~500 µs.** `time.Since` across an
  instant call reads exactly `0s`. [[D-201]] measured 512 µs and this session
  measured 506.5 µs and 718.1 µs an hour apart, so the real-module test measures
  the floor per run and prints every duration against it. A `0s` in any older
  output means "below the floor", never "instant". [[D-304]].
- **`go test ./cmd/liro-bridge/`** writes `HKCU\…\Run\LiroBridge` reproducibly
  ([[D-266]]); **`internal/ui`'s window tests** extract the icon into the real
  config directory ([[D-285]]). Run only what a change can reach.
- **Linting on Windows gives the `GOOS=windows` view twice** unless the other
  two are asked for. Three commands, [[D-295]].
- **The config directory is `%LOCALAPPDATA%\Liro`**, not `%APPDATA%\LiroBridge`.
  The second exists, is a stale older layout, and comparing against it reports
  every file missing.
- **Bash heredocs mangle backslash escapes.** It has now cost five files in this
  session alone, including twice inside Python strings containing `\n`. Use the
  file-writing tools for anything with a backslash in it.
- **`-race` does not run on these machines**: it needs cgo and there is no C
  toolchain. CI's Linux job is the only place it runs, and [[D-303]] is what it
  caught. Job logs need repo-admin auth; the `::warning::` annotation channel
  does not, and is how a probe's output is read back.

---

## 7. What the home machine needs

It has Go, Git and Node and has run this project before. Check rather than
assume:

- **Go 1.26.5** — `go.mod` names it exactly. An older toolchain will refuse.
- **golangci-lint v2.13.2** — pinned in `.github/workflows/ci.yml`, not
  `latest`, so that CI lints with exactly what a developer runs. A different
  version is the first thing to suspect if lint disagrees with CI, *after*
  checking the `GOOS` view ([[D-295]]).
- **Node 18** — only needed for the TypeScript SDK job; nothing in F12 §2 or
  F11 §4 touches it.
- **No C toolchain is needed** and none should be installed to chase `-race`;
  `CGO_ENABLED=0` is F0 §10 and the race detector belongs to CI.
- **SafeSign** at `C:\Windows\System32\aetpkss1.dll` — item 1 needs it and the
  Pošta card. Confirm with `go run ./scripts/p11probe --module "..."`, which is
  read-only and spends nothing.
- **Git push works from the office machine** via the cached credential; confirm
  the same at home with `git push --dry-run` before relying on it.
- Nothing needs installing for the PIN entry work itself: it is
  `golang.org/x/sys` (already a dependency) or plain `syscall`, and
  `scripts/p11probe` is the model.
