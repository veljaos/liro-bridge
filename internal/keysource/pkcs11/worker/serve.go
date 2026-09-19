package worker

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// Handler is what a request is served against: one module, held open, answering
// the read-only operations many times.
//
// It is an interface so that the loop above it can be exercised without a card,
// a module or a machine that has either — which is most of what this file is
// checkable against at all. The real implementation is heldModule in child.go
// and wraps a pkcs11.LiveModule.
//
// Every method is called on the goroutine that called Serve, and never
// concurrently. That is not a convenience: F12 §2 requires one OS-locked
// goroutine to make every PKCS#11 call, and D-298 is why it is the braces
// rather than the belt — two of the four modules this project has measured
// never read the CK_C_INITIALIZE_ARGS at all, so their CKR_OK to
// CKF_OS_LOCKING_OK means nothing about what they do with threads.
type Handler interface {
	// Enumerate reads certificate objects off every token the module sees.
	Enumerate(ctx context.Context) ([]CertificatePayload, error)
	// List is the same reading, deduplicated for the agent's listing.
	List(ctx context.Context) ([]CertificatePayload, error)
	// ChainFor returns the issuer chain the token itself carries for one
	// certificate, which is empty for both Serbian cards (D-274).
	ChainFor(ctx context.Context, thumbprint string) ([][]byte, error)

	// Login opens a session on the token holding one certificate and logs in,
	// keeping the session for SignDigest.
	//
	// ask is called if and only if the token needs a PIN, at most once, and it
	// is Serve's own: the handler does not touch the pipe. On a token that
	// advertises a protected authentication path (SPEC §6.5.1 clause 1) it is
	// never called at all, and only the handler can know that.
	Login(ctx context.Context, thumbprint string, ask PINExchange) (CertificatePayload, [][]byte, error)

	// SignDigest signs one digest with the key the login found. alg is
	// keysource.DigestAlgorithm as an integer; converting it is the handler's,
	// so that this file stays a description of bytes on a pipe.
	SignDigest(ctx context.Context, alg int, digest []byte) ([]byte, error)

	// CloseSession logs out and closes the session, leaving the module loaded.
	CloseSession() error

	// Close calls C_Finalize and unloads. It must be safe to call twice, and it
	// closes any open session first.
	Close() error
}

// PINExchange tells the parent what this token needs and reads the PIN it sends
// back, straight into dst.
//
// It is the whole of SPEC §6.5.1 clause 2's child half. dst is the buffer
// C_Login will be called with — already the token's own ulMaxPinLen, already
// pinned by the function that will overwrite it — and the bytes go into it by
// one io.ReadFull and are copied nowhere on the way. There is no intermediate
// buffer, no decode step, and nothing that outlives the call: the PIN is never
// a field, a parameter or a named result, which is what pin_test.go enforces
// over this package's syntax tree.
//
// It is called at most once. Clause 5 — nothing retries a PIN, ever, for any
// reason — and there is no loop above it that could.
type PINExchange func(dst []byte, needs LoginNeeds) (int, error)

// ErrProtocolDesync is a PIN exchange that did not go as it must: a frame that
// is not OpLoginPIN, one that does not carry the exchange it was asked about,
// or a length this token cannot accept.
//
// It ends the worker rather than becoming a Response, and that is deliberate.
// The parent has already written, or is about to write, some number of raw
// bytes that are not a frame; a reader that has lost track of where the next
// frame starts cannot recover by guessing, and the bytes it would be guessing
// about are a PIN. Ending is the only answer that cannot silently read one as
// something else. The supervisor respawns, and a login is not retried.
var ErrProtocolDesync = errors.New("pkcs11 worker: the PIN exchange did not go as it must; this pipe cannot be trusted to be in step")

// maxOpInMessage bounds how much of an unrecognised operation name is quoted
// back.
//
// A frame may claim up to maxFrame, so an Op of a megabyte is representable —
// and a response quoting it whole would itself exceed maxFrame, so WriteFrame
// would refuse it and the loop would end. That is a worker killed by a string,
// which is a worse answer than a refusal, and it costs one truncation to
// remove. The pipe is inherited and only this program's own parent writes it,
// so this is not a defence against an attacker; it is a defence against a
// length nobody thought about.
const maxOpInMessage = 64

// ErrUnknownOperation is a request naming something outside the closed set.
//
// It is refused rather than ignored. A worker that silently did nothing for an
// operation it did not recognise would answer a newer parent's request with an
// empty success, and an empty success is what a listing with no certificates on
// it looks like — the failure would arrive as "this card has nothing on it"
// rather than as "these two do not agree about the protocol".
var ErrUnknownOperation = errors.New("pkcs11 worker: unknown operation")

// Serve reads requests from r and writes exactly one response to w for each,
// until a shutdown request, the end of r, or ctx.
//
// # One goroutine does everything, and there is no channel
//
// F12 §2 asks for "one goroutine owns every PKCS#11 call, pinned with
// runtime.LockOSThread; everything else reaches it through a channel". The
// first half is honoured more simply than the second describes: the goroutine
// that reads the pipe is the goroutine that calls the Handler, so there is no
// "everything else" to reach it. One requester, one loop, no channel.
//
// That is not only simpler, it is required by SPEC §6.5.1 clause 2. The PIN
// arrives on this same reader, immediately behind a request, and the clause
// bounds it to "one write, read immediately, never buffered". A channel between
// the reader and the module-owner would mean the PIN travelling as a Go value
// through a channel's buffer — somebody else's byte slice, never overwritten,
// invisible to pin_test.go because it is not a field, a parameter or a named
// result. That is the same hazard D-297 rejected bufio for, one mechanism over.
//
// The login case below is where that becomes load-bearing rather than
// hypothetical: serveLogin hands the Handler a closure that writes a question
// down w and reads the answer back off r, and the Handler is called from this
// same goroutine, so the PIN travels from ReadFull straight into the buffer
// C_Login is given without passing through anything that could keep it.
//
// # What ends it, and what each ending means
//
// A shutdown request: the handler is closed, the response is written, nil.
// The end of r: the parent let go of its end of the pipe, which is an ordinary
// close rather than a fault, so nil. Anything else — a frame that will not
// read, a response that will not write — is an error, and the caller turns it
// into a non-zero exit.
//
// A handler that fails is not an ending. It is a Response carrying the reason,
// because F11 §3 asks for a readable reason rather than a number and because a
// module that refuses one request has not died; the supervisor is what notices
// one that has.
//
// ErrProtocolDesync is the exception, and it is one for the reason written on
// the sentinel: after a PIN exchange that went wrong there is no way to know
// where the next frame starts, and the bytes in question are a PIN.
func Serve(ctx context.Context, r io.Reader, w io.Writer, h Handler) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var req Request
		if err := ReadFrame(r, &req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil // the parent closed its end; nothing is wrong
			}
			return fmt.Errorf("reading a request: %w", err)
		}

		if req.Op == OpShutdown {
			// Close first, so that C_Finalize has happened before the parent is
			// told it has. F12 §2: "C_Finalize only on shutdown or a deliberate
			// reset."
			var resp Response
			if err := h.Close(); err != nil {
				resp.Err = err.Error()
			}
			if err := WriteFrame(w, resp); err != nil {
				return fmt.Errorf("writing the shutdown response: %w", err)
			}
			return nil
		}

		if req.Op == OpLoginPIN {
			// A PIN offered with no login in progress, which ends the worker
			// rather than being refused in a Response.
			//
			// It is tempting to answer it: nothing has been read past this frame
			// and the loop looks recoverable. It is not. Behind this frame are
			// PINLength raw bytes that only serveLogin's ask ever reads, and ask
			// did not run — so carrying on means the next ReadFrame takes the
			// first four bytes of a PIN as a length prefix and the rest into a
			// payload buffer, where they sit in this process's heap, never
			// overwritten, as somebody else's byte slice. That is precisely the
			// thing SPEC §6.5.1 clause 2 and D-297 rejected bufio for, arrived
			// at from the other direction.
			//
			// The parent refuses the mirror of this — a PIN question outside an
			// exchange — for the reason the owner gave: a request arriving when
			// nothing is pending means either this worker is confused or
			// something else is talking to the pipe, and both are worth knowing
			// about loudly.
			return fmt.Errorf("%w: a PIN was offered with no login in progress", ErrProtocolDesync)
		}

		if req.Op == OpLogin {
			// Not in serveOne, because this one operation needs the pipe itself:
			// it asks a question down w and reads the answer back off r, in the
			// middle of the Handler call. serveOne has neither and must not
			// acquire them — an operation that can write to w is an operation
			// that can put a frame where the parent is not expecting one.
			resp, err := serveLogin(ctx, r, w, h, req)
			if err != nil {
				return err
			}
			if err := WriteFrame(w, resp); err != nil {
				return fmt.Errorf("writing a login response: %w", err)
			}
			continue
		}

		if err := WriteFrame(w, serveOne(ctx, h, req)); err != nil {
			return fmt.Errorf("writing a response: %w", err)
		}
	}
}

// serveLogin runs one login, including the PIN exchange if the token needs one.
//
// Its error return is reserved for ErrProtocolDesync and for a pipe that broke:
// everything a card or a module can refuse comes back in the Response, like
// every other operation. The split matters because the two have opposite
// answers — a refusal is told to the parent and the worker keeps serving, and a
// desync ends the worker without another frame being written.
//
// # The exchange identifier is bound here rather than by the Handler
//
// The Handler is given a closure and knows nothing about exchanges. It cannot
// mint one, cannot echo the wrong one, and cannot ask a question outside one,
// because the only thing it can do is call ask — and ask stamps the identifier
// this request arrived with onto the question and requires it back on the
// answer. A worker that could ask for a PIN at will is a worker that could make
// PIN dialogs appear, and that dialog is the one window in this program that
// deliberately looks like a system dialog (D-277).
func serveLogin(ctx context.Context, r io.Reader, w io.Writer, h Handler, req Request) (Response, error) {
	if req.Thumbprint == "" {
		return Response{Err: "pkcs11 worker: login needs a thumbprint"}, nil
	}
	if req.Exchange == "" {
		// Refused rather than defaulted. An exchange identifier is what binds
		// this login to an operation a person approved, and a login with no
		// binding is the thing the binding exists to prevent.
		return Response{Err: "pkcs11 worker: login needs an exchange identifier"}, nil
	}

	// Set by ask when the pipe stopped making sense, and checked after the
	// Handler returns rather than only on the Handler's own error: a handler
	// that swallowed ask's error would otherwise leave this loop reading a pipe
	// it has lost its place in.
	var desync error

	ask := func(dst []byte, needs LoginNeeds) (int, error) {
		needs.Exchange = req.Exchange
		if err := WriteFrame(w, Response{Login: &needs}); err != nil {
			desync = fmt.Errorf("writing the PIN question: %w", err)
			return 0, desync
		}

		var answer Request
		if err := ReadFrame(r, &answer); err != nil {
			desync = fmt.Errorf("reading the PIN answer: %w", err)
			return 0, desync
		}
		if answer.Op != OpLoginPIN {
			desync = fmt.Errorf("%w: expected %q and got %q", ErrProtocolDesync, OpLoginPIN, truncate(string(answer.Op)))
			return 0, desync
		}
		if answer.Exchange != req.Exchange {
			desync = fmt.Errorf("%w: the PIN answer carries a different exchange", ErrProtocolDesync)
			return 0, desync
		}
		// The parent enforces the token's own limits before it writes, so a
		// length outside them cannot arrive from a parent that is in step. This
		// is not that check repeated: it is what stops a length that does not
		// fit dst from being read into dst, and it cannot recover, because the
		// bytes the parent is about to write are already on their way.
		if answer.PINLength < needs.MinPINLength || answer.PINLength > len(dst) {
			desync = fmt.Errorf("%w: a PIN of %d bytes against a token that accepts %d to %d",
				ErrProtocolDesync, answer.PINLength, needs.MinPINLength, len(dst))
			return 0, desync
		}

		// The one read. Straight into the buffer C_Login will be given, exactly
		// the number of bytes the frame named, never one more (SPEC §6.5.1
		// clause 2). There is no intermediate slice here on purpose: a copy
		// made on this line would be a copy nothing overwrites.
		if _, err := io.ReadFull(r, dst[:answer.PINLength]); err != nil {
			desync = fmt.Errorf("%w: reading the PIN: %w", ErrProtocolDesync, err)
			return 0, desync
		}
		return answer.PINLength, nil
	}

	cert, chain, err := h.Login(ctx, req.Thumbprint, ask)
	if desync != nil {
		return Response{}, desync
	}
	if err != nil {
		return Response{Err: err.Error(), NotFound: errors.Is(err, pkcs11.ErrCertificateNotFound)}, nil
	}
	return Response{Certificate: &cert, Chain: chain}, nil
}

// serveOne answers one request. It never returns an error: every way a request
// can fail is something the parent is told in a Response, so that a failure to
// read a card and a failure of this process are two different things from the
// outside.
func serveOne(ctx context.Context, h Handler, req Request) Response {
	switch req.Op {
	case OpEnumerate:
		certs, err := h.Enumerate(ctx)
		if err != nil {
			return Response{Err: err.Error()}
		}
		return Response{Certificates: certs}

	case OpList:
		certs, err := h.List(ctx)
		if err != nil {
			return Response{Err: err.Error()}
		}
		return Response{Certificates: certs}

	case OpChainFor:
		if req.Thumbprint == "" {
			return Response{Err: "pkcs11 worker: chainfor needs a thumbprint"}
		}
		chain, err := h.ChainFor(ctx, req.Thumbprint)
		if err != nil {
			return Response{Err: err.Error(), NotFound: errors.Is(err, pkcs11.ErrCertificateNotFound)}
		}
		return Response{Chain: chain}

	case OpSignDigest:
		sig, err := h.SignDigest(ctx, req.DigestAlgorithm, req.Digest)
		if err != nil {
			return Response{Err: err.Error()}
		}
		return Response{Signature: sig}

	case OpCloseSession:
		if err := h.CloseSession(); err != nil {
			return Response{Err: err.Error()}
		}
		return Response{}

	default:
		return Response{Err: fmt.Sprintf("%v: %q", ErrUnknownOperation, truncate(string(req.Op)))}
	}
}

// truncate bounds a caller-supplied string on its way into a message. See
// maxOpInMessage.
func truncate(s string) string {
	if len(s) <= maxOpInMessage {
		return s
	}
	return s[:maxOpInMessage] + "…"
}
