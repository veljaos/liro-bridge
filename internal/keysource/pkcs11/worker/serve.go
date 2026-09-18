package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	// Close calls C_Finalize and unloads. It must be safe to call twice.
	Close() error
}

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
// Serve makes no PIN call yet; the seam is described here because the shape of
// this loop is what has to be true when it does.
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

		if err := WriteFrame(w, serveOne(ctx, h, req)); err != nil {
			return fmt.Errorf("writing a response: %w", err)
		}
	}
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
			return Response{Err: err.Error()}
		}
		return Response{Chain: chain}

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
