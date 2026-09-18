package worker

import (
	"context"
	"fmt"
	"io"
	"runtime"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// Subcommand is the argument that puts this program's own binary into worker
// mode: one module, held open, answering a pipe.
//
// It is the second of the two subcommands that load a vendor PKCS#11 module,
// and it is deliberately not the first. pkcs11.ProbeSubcommand spawns a
// throwaway child per candidate, because discovery is asking unknown files what
// they are and a crash there is the expected outcome. This one holds
// C_Initialize open, because a session has to survive many calls and paying
// C_Initialize per call rolls D-272's dice every time. D-297 is why they are
// two things and must not be merged: the tell that such a consolidation is
// wrong is that the merged version has to take a parameter to decide which of
// two it is.
const Subcommand = "pkcs11-worker"

// Run is the child side: hold one module open and answer the pipe until the
// parent is done.
//
// # The contract, which is the reason this comment is long
//
// F12 §2: "The subcommand must not become a way in. D-222 and D-228 spent two
// phases keeping hidden modes out of a release binary. This one is reachable,
// so it needs the same scrutiny: it signs nothing, it holds no consent, and it
// cannot be driven into signing by anything but the parent that spawned it."
//
//   - **It signs nothing.** The operation set is closed —
//     TestEveryOperationIsNamedInOneClosedSet fails when it changes — and every
//     operation in it is a read. Nothing here opens a signing session, and
//     nothing here has a digest to sign.
//   - **It holds no consent.** contract_test.go asserts over this package's
//     whole dependency closure, rather than over its import block, that it
//     cannot reach internal/consent, internal/ui, internal/pades or anything
//     else that knows what a document is or could believe a person said yes.
//   - **It takes work from one place.** One module path on the command line,
//     and requests from the standard input it inherited. It opens no socket,
//     reads no file for instructions, and takes no module path from a request
//     (TestTheProtocolCarriesNoModulePath). A person who runs it from a shell
//     has no pipe to give it: the first read ends, and so does it.
//
// # One thread, locked, for the life of the process
//
// The module is opened on this goroutine and every later call is made on it,
// and the thread is locked before the open rather than after. F12 §2 requires
// it, and D-298 is why it is the braces rather than the belt: two of the four
// modules this project has measured answered CKR_OK to CKF_OS_LOCKING_OK
// without having read the arguments structure at all, so nothing about their
// thread safety was ever agreed to. One goroutine making every call holds
// whatever a module does, because there is never a second thread.
//
// The thread is not unlocked. The module must stay on it until the process
// ends, and the process ends when this function returns.
//
// # What it does when the module will not open
//
// Nothing on standard output, the reason on standard error, and a non-zero
// exit. Standard output is the response pipe and the parent has not asked
// anything yet; an unsolicited frame there would be a frame the parent reads as
// the answer to its first request.
//
// That makes "the module would not load" and "the module killed this process"
// the same thing from the parent's side, which is honest rather than
// convenient: they genuinely are, because the way D-272's module fails is by
// dying inside the C_Initialize this function is about to call. The supervisor
// respawns either way, and the reason — when there was one to give — is on the
// standard error it inherited.
func Run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 1 || args[0] == "" {
		_, _ = fmt.Fprintln(stderr, "pkcs11 worker: takes exactly one module path")
		return 2
	}

	// Before the open, so that C_Initialize and every call after it are one
	// thread. See above.
	runtime.LockOSThread()

	live, err := pkcs11.NewSource(args[0]).Hold()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "pkcs11 worker:", err)
		return 1
	}
	// Close is safe twice, which is what makes this defer correct alongside the
	// shutdown request that also closes.
	defer func() { _ = live.Close() }()

	if err := Serve(context.Background(), stdin, stdout, heldModule{live: live}); err != nil {
		_, _ = fmt.Fprintln(stderr, "pkcs11 worker:", err)
		return 1
	}
	return 0
}

// heldModule serves requests from one open module.
//
// It is the one place a pkcs11.LiveModule is turned into answers, and it is
// deliberately thin: everything it does is read what the module said and shape
// it for the wire. A decision made here would be a decision made in the process
// that may be killed by somebody else's code.
type heldModule struct{ live *pkcs11.LiveModule }

func (h heldModule) Enumerate(ctx context.Context) ([]CertificatePayload, error) {
	found, err := h.live.Enumerate(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CertificatePayload, 0, len(found))
	for _, c := range found {
		out = append(out, CertificatePayload{DER: c.DER, Label: c.Label})
	}
	return out, nil
}

func (h heldModule) List(ctx context.Context) ([]CertificatePayload, error) {
	certs, err := h.live.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CertificatePayload, 0, len(certs))
	for _, c := range certs {
		// A deduplicated listing has no per-object label: one certificate can be
		// two objects with two labels, and picking one of them would be a
		// decision made in the wrong process.
		out = append(out, CertificatePayload{DER: c.DER})
	}
	return out, nil
}

func (h heldModule) ChainFor(ctx context.Context, thumbprint string) ([][]byte, error) {
	// The wire carries a plain string and the backend takes the named type. The
	// conversion is here rather than in the protocol so that the protocol stays
	// a description of bytes on a pipe: a named type in a JSON field is a type
	// the far end has to have, and the far end is a process.
	return h.live.ChainFor(ctx, keysource.Thumbprint(thumbprint))
}

func (h heldModule) Close() error { return h.live.Close() }
