package pkcs11

import (
	"encoding/json"
	"fmt"
	"io"
)

// RunProbe is the child side: it loads one module, asks it what it is, and
// reports one JSON object. It is the only code in this program that loads a
// vendor PKCS#11 module, and it is only ever reached by a process this program
// spawned for the purpose.
//
// # It is a subcommand in a release binary, which is the thing to scrutinise
//
// D-222 and D-228 spent two consecutive phases keeping paths that sign without
// asking out of a release binary, and proved their absence by reading the
// symbol table. This one cannot be proved that way, because it is meant to be
// there. So the properties are different and they are worth naming:
//
//   - **It signs nothing.** It opens no session, logs in to nothing, and never
//     reaches C_Sign. It calls exactly C_Initialize, C_GetInfo, C_Finalize.
//   - **It holds no consent and shows no window.** There is no screen it can
//     put up and nothing in it that could believe a person said yes.
//   - **It takes one argument and acts on nothing else.** No standard input —
//     the one thing that ever travels a pipe into a child of this program is a
//     PIN (SPEC §6.5.1 clause 2), and a probe must not be able to receive one.
//     It opens no file of its own and no socket, and it reads no environment
//     variable. One variable is set *on* it, which it never reads:
//     probeChildMarker, by which a process that is already a probe child
//     refuses to spawn another. It can only subtract a capability and never add
//     one, which is why it does not qualify this sentence.
//   - **It cannot be driven anywhere else.** Its whole vocabulary is one path
//     in and one JSON object out.
//
// A person who runs it from a shell gets a JSON object describing a module, or
// a refusal, and nothing has happened to their card.
//
// # Why its answer is data rather than a decision
//
// The child is the process that may be killed by somebody else's code, so it
// is trusted to report a fact about one module and nothing more. It does not
// decide whether the module is usable; Modules does, in the parent, from what
// came back.
func RunProbe(args []string, stdout io.Writer) int {
	if len(args) != 1 || args[0] == "" {
		_, _ = fmt.Fprintln(stdout, `{"ok":false,"error":"the probe takes exactly one module path"}`)
		return 2
	}
	// The load itself is per-platform (describeModule), because a platform with
	// no binding has no live branch here and a shared version would carry a
	// comparison that is always true. discover_other.go's Modules is split for
	// the same reason and says so.
	res := describeModule(args[0])

	// Marshalling cannot fail for this shape, and a probe that died writing its
	// own answer would be indistinguishable from one the module killed — so the
	// error is not swallowed, it is reported in the one form the parent parses.
	b, err := json.Marshal(res)
	if err != nil {
		_, _ = fmt.Fprintln(stdout, `{"ok":false,"error":"the probe could not encode its own result"}`)
		return 1
	}
	// This write is the whole output of the process, so its error is the one
	// thing here that cannot be ignored: a child that returned zero having
	// written nothing would be read by the parent as a module that answered
	// with something unparseable, which points at the module rather than at
	// the pipe. Exiting non-zero says "this child did not report", which is
	// what actually happened.
	if _, err := fmt.Fprintln(stdout, string(b)); err != nil {
		return 1
	}

	// Zero whether or not the module was usable: "this file is not a module" is
	// an answer, and only the parent decides what it means. A non-zero exit is
	// reserved for the child not having answered at all, which is what the
	// parent reads as the module having killed it.
	return 0
}
