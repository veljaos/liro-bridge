package worker

import (
	"context"
	"errors"
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
//   - **It signs nothing** — meaning it has no path to a signed *document*. It
//     signs a digest it is handed, which is SPEC §5.1's own boundary: key
//     sources sign hashes and do not know what a PDF is. That reading is not a
//     concession made when the PIN seam arrived; doc.go wrote it down before
//     this file could sign anything, which is the only reason the sentence did
//     not have to change under pressure. The operation set is closed and
//     TestEveryOperationIsNamedInOneClosedSet fails when it widens.
//   - **It cannot be driven into signing by anything but the parent that
//     spawned it.** This is the clause the PIN seam gives a mechanism to, and
//     it is now a chain rather than an assertion: a signature needs a session,
//     a session needs a login, and a login needs an exchange identifier the
//     parent minted for one operation a person approved. A login carrying none
//     is refused before a card is touched.
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

	if err := Serve(context.Background(), stdin, stdout, &heldModule{live: live}); err != nil {
		_, _ = fmt.Fprintln(stderr, "pkcs11 worker:", err)
		return 1
	}
	return 0
}

// errNoSession is a sign or a close with no login behind it.
//
// It is a refusal in a Response rather than an ending, because it is a parent
// that asked in the wrong order — which is this program disagreeing with itself
// and is worth reading — and because nothing has been read past it. That is the
// difference between it and ErrProtocolDesync, which is about bytes rather than
// about order.
var errNoSession = errors.New("pkcs11 worker: this worker has no open session; a login has to succeed first")

// heldModule serves requests from one open module.
//
// It is the one place a pkcs11.LiveModule is turned into answers, and it is
// deliberately thin: everything it does is read what the module said and shape
// it for the wire. A decision made here would be a decision made in the process
// that may be killed by somebody else's code.
//
// # One session at a time, and it is a pointer for that reason
//
// The value receiver this had before the PIN seam could not hold a session.
// Exactly one is held: a card is a single serial device (D-027), the module is
// single-threaded by construction (D-298), and a second login on the same
// worker would be a second PIN screen for a consent nobody gave. A login
// arriving while one is open is refused rather than replacing it.
type heldModule struct {
	live *pkcs11.LiveModule

	// sess is the logged-in session, from Login until CloseSession or Close.
	// Every method that touches it runs on the goroutine that called Serve, so
	// there is no lock here and must not be one: a lock would say that two
	// goroutines might, and if two ever did, the problem would be C_Sign on a
	// thread the module was not initialised on rather than a data race.
	sess keysource.Session
}

func (h *heldModule) Enumerate(ctx context.Context) ([]CertificatePayload, error) {
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

func (h *heldModule) List(ctx context.Context) ([]CertificatePayload, error) {
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

func (h *heldModule) ChainFor(ctx context.Context, thumbprint string) ([][]byte, error) {
	// The wire carries a plain string and the backend takes the named type. The
	// conversion is here rather than in the protocol so that the protocol stays
	// a description of bytes on a pipe: a named type in a JSON field is a type
	// the far end has to have, and the far end is a process.
	return h.live.ChainFor(ctx, keysource.Thumbprint(thumbprint))
}

// Login opens a session on the token holding one certificate and logs in.
//
// # The PINEntry it builds is the whole of the child's PIN handling
//
// pkcs11.PINEntry fills a buffer the login allocated, pinned and will overwrite,
// and returns how many bytes it wrote (SPEC §6.5.1 clause 2). ask has the same
// shape for the same reason, so this adapter is a translation of the *question*
// and touches nothing else: dst goes through untouched, and the bytes that come
// back are written by io.ReadFull inside ask, into dst, once. There is no line
// here that a PIN passes through.
//
// # What the question does and does not carry
//
// The token's own minimum and maximum, read from CK_TOKEN_INFO at the moment
// they are needed (clause 7), and the three labels a screen needs in order to
// say whose card this is (clause 6). Not req.ModulePath, which the login also
// has: the parent chose that path and passed it on this process's command line,
// so sending it back would put a module path in the protocol — the one thing
// TestTheProtocolCarriesNoModulePath exists to keep out, and it would have
// arrived as a field nobody thought of as a path because it was only ever an
// echo.
func (h *heldModule) Login(ctx context.Context, thumbprint string, ask PINExchange) (CertificatePayload, [][]byte, error) {
	if h.sess != nil {
		return CertificatePayload{}, nil, errors.New("pkcs11 worker: this worker already has a session open")
	}

	entry := pkcs11.PINEntry(func(dst []byte, req pkcs11.PINRequest) (int, error) {
		return ask(dst, LoginNeeds{
			MinPINLength: req.MinLength,
			// len(dst) rather than a second copy of the token's maximum: the
			// buffer *is* the maximum, and a number that could disagree with the
			// buffer it describes is a number worth not having. pkcs11's own
			// PINRequest makes the same choice, and says so.
			MaxPINLength:     len(dst),
			TokenLabel:       req.TokenLabel,
			TokenSerial:      req.TokenSerial,
			CertificateLabel: req.CertificateLabel,
		})
	})

	sess, err := h.live.Open(ctx, keysource.Thumbprint(thumbprint), entry)
	if err != nil {
		return CertificatePayload{}, nil, err
	}
	h.sess = sess
	return CertificatePayload{DER: sess.Certificate().DER}, sess.Chain(), nil
}

// SignDigest signs one digest with the key the login found.
//
// The algorithm arrives as an integer and is converted here rather than in the
// protocol, so that the protocol stays a description of bytes on a pipe. An
// integer naming nothing is refused by digestInfo one layer down, which is where
// the mapping from algorithm to DigestInfo bytes actually lives — checking it
// again here would be a second copy of a rule that can only be right in one
// place.
func (h *heldModule) SignDigest(ctx context.Context, alg int, digest []byte) ([]byte, error) {
	if h.sess == nil {
		return nil, errNoSession
	}
	return h.sess.SignDigest(ctx, keysource.DigestAlgorithm(alg), digest)
}

// CloseSession logs out and closes the session, leaving the module loaded.
//
// That is the whole point of the worker being a separate thing from the probe:
// the expensive part is C_Initialize and the module stays, while the card's
// authenticated state goes as soon as the batch that needed it is done. SPEC
// §6.5: a session left logged in is a card another process can sign with.
//
// Closing one that is not open is not an error. Close calls this, and a parent
// that sends OpCloseSession after a login that failed is a parent being careful
// rather than a parent being wrong.
func (h *heldModule) CloseSession() error {
	if h.sess == nil {
		return nil
	}
	sess := h.sess
	h.sess = nil
	return sess.Close()
}

// Close logs out and unloads. The session goes first: an unloaded module cannot
// log out of anything, and a card left authenticated because C_Finalize ran
// first is the failure SPEC §6.5 is about.
func (h *heldModule) Close() error {
	err := h.CloseSession()
	if cerr := h.live.Close(); err == nil {
		err = cerr
	}
	return err
}
