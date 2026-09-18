// Package worker is both sides of the PKCS#11 out-of-process boundary: the
// child that holds one vendor module open and answers a pipe, and the
// supervisor that spawns it, talks to it, and respawns it when it stops
// answering.
//
// # Why both sides are here, which the first draft of this comment got wrong
//
// This said "the child side", on the assumption that the parent would live in
// internal/keysource/pkcs11 beside Source. It cannot: the child needs the
// binding, so this package imports pkcs11, and a parent in pkcs11 would need
// the protocol, which is here. That is a cycle, and it is not avoidable by
// moving the parent somewhere else — the parent has to be reachable from
// Source.List, because serving Source out of process is the entire point.
//
// So both ends of one protocol live in one package. That is also where the
// guards want to be: pin_test.go has to be where the PIN is read, the no-bufio
// rule has to cover both the end that reads a PIN off the pipe and the end that
// writes one onto it, and contract_test.go has to be over the closure that
// includes the module-loading code. Splitting the sides would have put each
// guard one package away from the thing it guards.
//
// # Why it exists at all
//
// One real module kills its host. D-272 measured NetSeT 1.1.0.0 — the build
// MUP's own middleware installs — dying inside its own C_Initialize about once
// in a hundred calls, in two different ways (a C++ throw and a CRT fail-fast),
// and established by direct measurement that **no Go process survives either**:
// recover() catches neither, and a vectored exception handler does not help. A
// fail-fast bypasses exception dispatch by design.
//
// So there is no in-process remedy, and the only way a program survives a
// module that does this is for the call not to be in that program's process.
// A module that kills this worker becomes a Failure in a list; the agent, its
// windows and any batch in flight carry on (F12 §2, F11 §3).
//
// # The contract, which is the reason this doc comment is long
//
// F12 §2: "The subcommand must not become a way in. D-222 and D-228 spent two
// phases keeping hidden modes out of a release binary. This one is reachable,
// so it needs the same scrutiny: it signs nothing, it holds no consent, and it
// cannot be driven into signing by anything but the parent that spawned it."
//
// Those three are not claims made here. Each is a property something checks:
//
//   - **It signs nothing** — meaning it has no path to a signed *document*.
//     It signs a digest it is handed, which is SPEC §5.1's own boundary ("key
//     sources sign hashes; they do not know what a PDF is"). It cannot reach
//     internal/pades, internal/signing or anything that knows what a PDF is,
//     and contract_test.go asserts that over the package's whole dependency
//     closure rather than over its import block.
//   - **It holds no consent** — it cannot reach internal/consent or
//     internal/ui, so there is no screen it can show, no approval it can
//     record, and nothing in it that could believe a person said yes. The
//     gate stays where SPEC §6.5 puts it, in the agent.
//   - **It cannot be driven into signing by anything but its parent** — its
//     only input is a pipe it inherited, and one module path on its command
//     line. It opens no socket and reads no file for instructions. A person who
//     runs the subcommand from a shell has no parent to give it a pipe: its
//     first read ends, and so does it.
//
// # What the supervisor adds, and what it deliberately does not
//
// A worker that stops answering is an error the caller can act on rather than a
// process this program has lost: the pipe ends or the process exits, and both
// are events rather than deadlines. A request that may be re-sent is re-sent to
// a fresh worker, bounded by a count — D-297's instruction, because the only
// duration this project has measured around a dying child is D-296's
// 341–376 ms reap and D-296 says plainly that what those milliseconds are spent
// on is not established.
//
// Which requests may be re-sent is an allow-list, and the reason is the one
// thing in this package that must not be got wrong later: SPEC §6.5.1 clause 5
// forbids retrying a PIN, ever, for any reason. An operation added to this
// protocol is not retried until somebody says so.
//
// # The PIN
//
// SPEC §6.5.1 clause 2, as amended: C_Sign needs a logged-in session and a
// session belongs to the process that opened it, so C_Login happens here while
// the screen that collects the PIN belongs to the agent. The PIN is written
// once, by the agent, into the pipe this process inherited — the single
// process boundary that clause permits, and the second place in the PIN's path
// that this program does not own.
//
// What that requires of this package, all of it from that clause:
//
//   - the PIN is read immediately and in full, never through a buffered
//     reader, never into anything that outlives the call;
//   - it lives in one pinned buffer, is passed to C_Login, and is overwritten
//     through the pinned address rather than through a slice header — a loop
//     that was elided and a loop that ran look identical through the header;
//   - it is never a field, never a parameter, never a named result, which
//     pin_test.go enforces over this package's syntax tree from this, its
//     first commit (D-269's ordering: a test written after a backend is a test
//     written around whatever that backend already does);
//   - nothing retries a PIN, ever;
//   - ulMinPinLen and ulMaxPinLen are the token's, and are enforced by the
//     agent before the write, so an invalid length reaches neither this pipe
//     nor the card (D-268 cost a PIN attempt establishing that no module
//     checks them).
//
// The exchange is two phases, and clause 1 rather than clause 2 is why. Only
// this side can see CKF_PROTECTED_AUTHENTICATION_PATH — it is a property of a
// token through a module, read per token every time (D-273, D-276) — so the
// agent cannot know in advance whether a PIN is needed at all, and a login
// either asks or simply completes. That the shape also puts the write into a
// reader already blocked waiting for exactly that many bytes is a consequence
// rather than something anyone had to arrange (D-302).
//
// Each login carries an exchange identifier the agent minted for one operation
// a person approved, and a PIN question that does not carry it back — or that
// arrives with no login pending at all — is a defect this package reports
// loudly and kills the worker over. The binding is to the approved operation
// and not to the worker being alive: a worker that could ask at will is a
// worker that could make PIN dialogs appear, and that dialog is the one window
// in this program that deliberately looks like a system dialog (D-277).
//
// # Error reporting is NOT disabled for this process, and that is a decision
//
// An earlier version of this comment said the opposite. D-289 required this
// process to turn its own error reporting off for its whole life, with a stop
// condition: if that could not be done reliably, the phase was to stop and say
// so rather than ship the gap. The condition fired.
//
// D-292 measured three mechanisms and none of them shipped.
// WerRegisterExcludedMemoryBlock returns S_OK and does not exclude — a
// registered block appears in a full-memory dump exactly as often as the
// control. A wholesale per-process disable returns success and cannot be shown
// to do anything, because observing the property needs a dump that needs a
// registry change declined for good reason. CryptProtectMemory works and covers
// 10.572µs of a window the length of a card operation, under 1% of it.
//
// The owner's ruling was that a mechanism which cannot be shown to work does
// not ship, and that keeping one alongside as belt and braces would make the
// refusal of the others decorative. So SPEC §6.5.1 clause 3 was narrowed
// instead, to what this program controls, and this package makes no claim about
// crash dumps.
//
// What remains true, and is the reason the process boundary still earns its
// keep: the PIN lives here for the length of one C_Login, in a process that
// holds nothing else of value, and is overwritten through its pinned address
// immediately afterwards. And the copy that matters was never this program's —
// C_Login takes the PIN by pointer, so the module may keep its own, in memory
// this program does not own and cannot wipe (D-292).
package worker
