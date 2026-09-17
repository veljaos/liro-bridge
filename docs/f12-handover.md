# F12 handover — 2026-09-17

`docs/phases/F12.md` is the phase and `docs/decisions.md` is the record. This
file is neither. It is what those two do not say: where §2 stopped, which
decisions are load-bearing for the next piece, and what this machine will do to
you if you do not know about it.

**Entries are pointed at, not restated.** Read the entry before acting on a line
here.

---

## 1. Where §2 stopped

Master is green. §1 is closed ([[D-287]], SPEC §1.1). §4's consent audit is done
([[D-288]]). §2 is part-built, in this order, and the order was agreed:

| | state |
| --- | --- |
| Out-of-process **discovery** probe | **done** — [[D-293]], [[D-294]] |
| [[D-275]]'s gate replaced | **done** — `cmd/liro-bridge/pkcs11reach_test.go` deleted, `pkcs11probe_test.go` stands in its place |
| Worker **request/response encoding** | **done** — `internal/keysource/pkcs11/worker/protocol.go`, [[D-297]] |
| Worker **guards**, written before the code | **done** — `worker/protocol_guards_test.go`, mutation-tested |
| `C_Initialize` with `CKF_OS_LOCKING_OK` | **done** — [[D-298]] |
| Worker **child-side dispatch**, one `LockOSThread` goroutine | **next** |
| Parent-side **supervisor**: dead worker → `Failure`, respawn | after that |
| `Enumerate`, `List`, `ChainFor` across the boundary | after that |
| F11 §4 — which source signs, one certificate to one row | after §2 |

**Start with the dispatch loop.** The protocol and its guards are in and the
guards bite; the next commit is the child that serves them.

### The one refactor the dispatch loop needs

`Source.Enumerate` in `source_windows.go` does `openModule` → work → `close`,
and `List` and `ChainFor` both call it. A worker calling those would run
`C_Initialize` **per request**, which is the thing the worker exists to avoid.

Extract the body to take an already-open `*module`, and let `Source.Enumerate`
open and close around it. That is a consolidation of the [[D-108]] kind — one
rule, two callers — and it is the *right* kind. See §2 below for the kind that
is not.

`openModuleLocking` already exists for the worker's side.

---

## 2. There are two process shapes and they are not to be collapsed

Discovery spawns a **throwaway child per candidate**. The worker holds **one
child open per module** with `C_Initialize` live. They look alike — same binary,
same subcommand shape, same "a dead child is a `Failure`" rule.

**[[D-297]] is the entry. Read it before merging anything.** The short form:

- Discovery is asking unknown files what they are. A crash there is the
  *expected* outcome, costs one `Failure` and ~30ms, and paying `C_Initialize`
  per candidate is correct.
- A session must survive many calls. Paying `C_Initialize` per call rolls
  [[D-272]]'s dice every time, which turns a startup problem into a listing
  problem — and a listing that fails one time in a hundred is the kind of defect
  people learn to re-run instead of read.

This project has removed one-thing-in-two-places three times and was right every
time ([[D-108]], [[D-124]], [[D-138]]), which is exactly why this case is argued
in the record. **The tell that a consolidation is wrong: the merged version has
to take a parameter to decide which of two things it is.**

---

## 3. The worker's protocol, and the one thing that decided its shape

`worker/protocol.go`. Length-prefixed frames: four bytes of length, then exactly
that many, read with `io.ReadFull`.

**That is SPEC §6.5.1 clause 2's doing, not taste.** The clause bounds the PIN
to *"one write, read immediately, never buffered"*. Newline-delimited JSON needs
a buffered reader, and a buffered reader reads ahead — asked for a request frame
it may pull the PIN that follows into the worker's heap, where nothing
overwrites it and **no guard in `pin_test.go` can see it**, because it is not a
field, a parameter or a named result but somebody else's byte slice.

`TestTheWorkerNeverBuffersItsInput` forbids `bufio` across the whole package.
**Do not import it.** If you need buffering, you need a different design.

It also settles a platform question: `os/exec`'s `ExtraFiles` is **not supported
on Windows**, so a dedicated second pipe for the PIN would need `SysProcAttr`
handle inheritance to exist at all. One pipe that cannot read ahead is better.

The protocol carries **no module path** (F12 §10; the path comes from argv) and
**no PIN field, ever**. Both are guarded.

---

## 4. `CKF_OS_LOCKING_OK`: two of four modules never read it

**[[D-298]].** Passed to all four real modules here, every one answers `CKR_OK`.
That reads as four acceptances and it is not:

| module | reads the args structure | its `CKR_OK` is worth |
| --- | --- | --- |
| NetSeT TrustEdgeID 1.1.3.3 | yes | a real acceptance |
| NetSeT MUP RS\Celik 1.1.0.0 | yes | a real acceptance |
| A.E.T. SafeSign 3.9.24.1 | **no** | nothing |
| Nexus Personal 5.17 | **no** | nothing |

The control is PKCS#11 v2.40 §5.4: a non-NULL `pReserved` **must** be refused
with `CKR_ARGUMENTS_BAD`. Two modules refuse it; two return `CKR_OK`, which
means they never looked at the structure.

**Two things follow, and both matter for the dispatch loop:**

1. **`runtime.LockOSThread` is the braces, not the belt.** Thread safety comes
   from one goroutine making every PKCS#11 call, full stop. It holds whether or
   not a module locks anything, because there is never a second thread. **If
   anyone relaxes that on the grounds that the modules handle locking, [[D-298]]
   is why they are wrong** — for half of them there is no evidence the request
   was even read.
2. **The 44-byte `CK_C_INITIALIZE_ARGS` layout is confirmed by two vendors, not
   four.** It was *derived* — there is no PKCS#11 header on this machine — and
   what makes it confirmation is that two independently written modules, builds
   five years apart, both refused a value placed at offset 36 of a buffer this
   code built.

`TestWhichModulesAcceptOSLocking` **reports** the vendor difference and does not
fail on it. A module ignoring `pReserved` is a fact about that vendor, not a
defect here, and a test that failed on it would fail on whichever middleware a
developer happens to have installed. **Do not "fix" it back into failing.**

---

## 5. The exit item is open, and a green run will not close it

F12 §2 asks for *"a module that kills its worker becomes a `Failure`, and the
agent survives — demonstrated with the module that does it"*.

The owner ran **2000 probes of MUP RS\Celik 1.1.0.0 with the card in** — four
batches of 500, 76ms each, stable to 0.3% — and got **zero deaths**, where
[[D-272]]'s rate would predict about twenty.

**[[D-294]] names three possibilities and picks none**: the rate is far lower
than that sample suggested, or something on this machine changed since
2026-09-15, or the probe path differs from the path that died. Distinguishing
them costs more than it is worth; guessing is worse.

It does **not** unmeasure [[D-272]]. Four crashes happened, two observed by the
owner independently, with two distinct terminations. **A fifth declining to
appear on demand is not evidence against the first four.**

> **No number of clean runs demonstrates "when it dies, the parent survives";
> only a death does.**

[[D-296]] closed the half that can be closed — the parent's handling of a real
death, against a child that fail-fasts on purpose, which the owner ruled is not
what [[D-094]] forbids. It measured: `WerFault` launches every time, writes no
dump, and the child is reaped in **341–376 ms** against a 10 s `ProbeTimeout`.
Only one of [[D-272]]'s two terminations is reachable from a Go child —
`0xE06D7363` meets Go's own handler and exits 2 — so the C++-throw case is
recorded as **untested**, not covered.

**Do not size a supervisor timeout from the ~320 ms.** What it is spent on is
not established, and pinning it needs the disable [[D-292]] measured as
undemonstrable. The supervisor detects a dead worker by **events** — the pipe
ending, the process exiting — and bounds respawns by a **count**, not an
interval. A per-request deadline is a separate question justified by the cost of
the request it bounds; bring the owner the number rather than choosing.

---

## 6. Six instrument failures in one week — read this before you trust a check

You will otherwise meet these one at a time. **[[D-296]] holds the canonical
statement**; this is the summary.

> **1. Could this check have failed?** Ask it of anything reporting a presence
> or a verdict.
> **2. Could this instrument have seen the thing whose absence it reports?** Ask
> it of anything reporting an absence.

| | what it was | question |
| --- | --- | --- |
| [[D-285]] | a note describing a hazard, standing in for a guard against it | 1 |
| [[D-290]] | a control that held under `-tags softtoken` but not under the release build | 1 |
| [[D-291]] | a canary that was a compile-time constant, so it was in every dump | 1 |
| [[D-293]] | `grep` for a carriage return, in a `grep` that strips them | 1 |
| [[D-295]] | two lint arms that were the same arm | 1 |
| [[D-296]] | one sampling run reporting `WerFault` "never ran" | **2** |
| [[D-298]] | four `CKR_OK`s read as four acceptances | 1 |

The practical forms: **a sampler cannot report absence**, and **a check written
in the same breath as the thing it checks tends to agree with it.** The fix in
[[D-298]] is the pattern worth copying — find a question whose answer a
specification fixes in advance, and ask that.

---

## 7. What this machine does to you

- **[[D-266]]: `go test ./cmd/liro-bridge/` writes `HKCU\…\Run\LiroBridge`**,
  reproducibly. `reg export` the Run key and the Explorer verb key
  (`HKCU\Software\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign`)
  first. **Compare the Run key value by value** — `reg export` emits values in an
  unstable order, so a hash of the export is not a comparison.
- **[[D-285]]: `internal/ui`'s window tests extract the icon into the real
  config directory.** `%LOCALAPPDATA%\Liro\icon-NNNNN.ico` appears on every
  `go test ./...`. Find it by creation time and remove it. **This note has now
  caught three separate authors after each had read it** — it is not a guard,
  and a temporary config home the window tests cannot escape is what would be.
- **[[D-295]]: the lint that matches CI is three commands, and the obvious one
  is a duplicate.** This machine is Windows, so a bare `golangci-lint run` *is*
  the `GOOS=windows` view. Run:
  ```
  GOOS=linux   golangci-lint run ./...   # the Ubuntu job — the one that fails
  GOOS=windows golangci-lint run ./...
  GOOS=darwin  golangci-lint run ./...   # not in CI; F13 will want it
  ```
  The version is pinned in the workflow, so "clean locally, red on CI" is almost
  never version drift.
- **Backslashes through a shell heredoc get eaten, and the note about it has now
  failed four times** — the fourth *inside the sentence describing it*, which put
  literal carriage returns into prose about how escapes get eaten. A **quoted**
  heredoc does not help. Use the Write/Edit tools for any content with
  backslashes or escapes. See [[D-293]].
- **Never round-trip a source file through PowerShell** (`Get-Content -Raw |
  Set-Content` mangles UTF-8; every file here is full of `§` and `—`). PowerShell
  *displaying* `§` as `Â§` in the console is harmless; check the file's bytes
  before believing it is damaged.
- **`go test` over `internal/keysource/pkcs11` used to fork-bomb this machine**
  — 254 processes to 827. It cannot now ([[D-293]]'s `probeChildMarker` bounds
  it at one generation whatever binary is spawned), but if you add a package
  whose tests reach `Modules`, watch the process count the first time.
- **Crash dumps**: `%LOCALAPPDATA%\CrashDumps` holds ten by default and is
  **full**. Anything that writes one evicts somebody's. Copy all ten before
  deliberately crashing anything — [[D-243]]: a hash proves something changed,
  only a copy can put it back.
- **The owner's tray agent is running** (`liro-bridge.exe tray`). It is the
  installed v0.9.2, not your build. Leave it alone; point any test build at a
  scratch `LOCALAPPDATA`.

---

## 8. Still open, beyond §2

- **SPEC §6.5's foreground sentence.** [[D-288]] found that no production code
  rests on the consent window being in front — only SPEC §6.5:346's sentence
  does, cited by `mainwindow_windows.go`. Wayland has no always-on-top, so the
  sentence needs drafting against what §4 built. **The owner's to make.**
- **The PIN seam.** Waits for something to be built against clause 2 as amended
  ([[D-289]]). The protocol is ready for it: exact-length reads, no `bufio`.
- **`RTLD_LOCAL`** on the Linux side (F12 §2), when there is a `dlopen` binding
  to apply it to. `module_other.go` has none yet, deliberately.
- **F11 §4** — which source signs, one certificate to one row across two
  backends, and what the audit log records. After §2.
- **Everything from §3 onward in F12.md** is untouched: GTK4/WebKitGTK, Wayland
  consent, the PIN dialog, the tray that is not there, packaging, `pcsclite`,
  p11-kit discovery. None of it can be done from here — see F12.md §0.
