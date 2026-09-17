// Package worker is the child side of the PKCS#11 out-of-process boundary:
// the only code in this project that is permitted to load a vendor PKCS#11
// module and call into it.
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
//     only input is a pipe it inherited. It opens no socket, reads no file for
//     instructions, and takes no work from its command line. A person who runs
//     the subcommand from a shell has no pipe to give it and gets a refusal.
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
