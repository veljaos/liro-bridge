package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"runtime"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// exchangeIDBytes is how much randomness an exchange identifier carries.
//
// It is random rather than a counter, and the difference is what the identifier
// is for. A counter would bind a PIN question to the login that is in flight,
// which is most of the job — but a child that has seen exchange 3 can write 4,
// and the whole point of the binding is that the question comes from the
// operation a person approved rather than from the process that happens to be
// running. Sixteen bytes is not guessable, and an identifier is never reused
// because it is never derived from anything.
const exchangeIDBytes = 16

// newExchangeID mints one identifier for one approved operation.
func newExchangeID() (string, error) {
	var b [exchangeIDBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Refused rather than falling back to something weaker. A binding that
		// silently degrades when the system's randomness is unavailable is a
		// binding that is weakest exactly when the machine is strangest.
		return "", fmt.Errorf("pkcs11 worker: minting an exchange identifier: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// Open logs in to the token holding one certificate and returns a session that
// signs with it in the worker process.
//
// # The two phases, and which clause each one is for
//
// The parent sends OpLogin and the child answers either with the finished login
// or with a question. Two phases rather than one because of SPEC §6.5.1 clause
// 1, not clause 2: the parent cannot know in advance whether a PIN is needed at
// all. Only the child can see CKF_PROTECTED_AUTHENTICATION_PATH, it is a
// property of a token through a module read per token every time (D-273,
// D-276), and a reader with a pinpad answers differently through the same DLL.
// A parent that collected a PIN first would have drawn a screen for a card that
// was never going to be asked.
//
// That the shape also gives clause 2 its most exact form — the PIN is written
// into a reader that is already blocked waiting to consume exactly that many
// bytes, so "one write, read immediately, never buffered" is true by
// construction rather than by care — is a consequence and not the reason.
//
// # It is not retried, at any level
//
// OpLogin is deliberately absent from retryable, and nothing here loops. Clause
// 5: one wrong PIN is one of three attempts, and for a national identity card
// the third means a visit to a police station. A worker that dies during a
// login is a failure reported to the caller, never a second screen.
//
// # Any failure after the question kills the worker
//
// Once the child has asked, it is blocked reading a PIN, and every way this can
// fail from there — the person cancelled, the length was refused, the pipe
// broke — leaves it blocked on bytes that are not coming. There is no "never
// mind" in the operation set and there must not be one: an operation that
// cancels a login is an operation that can be sent instead of a login, and the
// set is the argument for why this subcommand is safe to ship. Killing costs a
// respawn, which is what a respawn is for.
func (w *Worker) Open(ctx context.Context, want keysource.Thumbprint, entry pkcs11.PINEntry) (keysource.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if want == "" {
		return nil, fmt.Errorf("pkcs11 worker: no certificate was named")
	}

	exchangeID, err := newExchangeID()
	if err != nil {
		return nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.start(); err != nil {
		return nil, err
	}

	mid := func(in io.Writer, needs LoginNeeds) error {
		if needs.Exchange != exchangeID {
			return fmt.Errorf("%w: it asked about an exchange this parent did not open", ErrUnexpectedPINRequest)
		}
		return sendPIN(in, exchangeID, needs, entry, w.modulePath)
	}

	resp, err := w.exchange(ctx, Request{Op: OpLogin, Thumbprint: string(want), Exchange: exchangeID}, mid)
	if err != nil {
		// Every error out of a login exchange ends the worker, and there is no
		// case here worth telling from the others. A child that died is gone; a
		// child that asked a question this parent refused is blocked reading a
		// PIN that is not coming; a child whose PIN send failed part-way is
		// blocked on the rest of it; a context that ended has already killed it.
		// The one case where the child is fine — it answered, and the answer was
		// a refusal — is not an error here at all, it is resp.Err below.
		//
		// An earlier draft carried a flag set inside mid to tell "stranded" from
		// "merely disappointed". It was both a distinction with no different
		// outcome and a data race: mid runs on the goroutine exchange abandons
		// when the context ends, so reading the flag afterwards is a read racing
		// a write. Removing it removed the race.
		w.endLocked(ctx, OpLogin)
		return nil, err
	}
	if resp.Err != "" {
		return nil, fmt.Errorf("pkcs11 worker: %s: %s", w.modulePath, resp.Err)
	}
	if resp.Certificate == nil || len(resp.Certificate.DER) == 0 {
		// A success with no certificate is a worker that answered the wrong
		// question. Refused rather than returned as a session over nothing: a
		// session whose Certificate() is empty produces a signature that
		// verifies against nothing while every layer reports success, which is
		// the silent failure F11 §2.1 is about one level down.
		w.endLocked(ctx, OpLogin)
		return nil, fmt.Errorf("pkcs11 worker: %s: the login succeeded and named no certificate", w.modulePath)
	}

	return &remoteSession{
		worker: w,
		cert: keysource.Certificate{
			Thumbprint: want,
			DER:        resp.Certificate.DER,
			// IsTestKey is false and is not read from the wire. It marks the
			// soft token (SPEC §16.6), and a worker holds a real module by
			// construction — there is no path from here to the soft token, and
			// letting a child assert it would let a child un-mark a test
			// signature.
		},
		chain: resp.Chain,
	}, nil
}

// sendPIN collects one PIN and writes it to the child, in that order and once.
//
// # What happens in which order, and why the order is the clause
//
// The token's limits are checked before a screen is drawn, the collected length
// is checked before anything is written, and the PIN leaves in a single Write
// of exactly the bytes the frame just promised. SPEC §6.5.1 clause 7 is the
// middle one and it is not a formality: D-268 measured one authorised C_Login
// with a NULL PIN on a MUP token cost one of three attempts, because the module
// range-checked nothing and the card counted it. A length that cannot be right
// must not reach the pipe, because past the pipe is the card.
//
// # Two writes, one of which is the PIN
//
// The frame naming the length and the PIN itself are separate writes on
// purpose. Clause 2 bounds the PIN — "one write, read immediately, never
// buffered" — and that is exactly what the second one is: len(dst[:n]) bytes,
// one Write, into a child that is already blocked in io.ReadFull waiting for
// that many. Putting the PIN inside the frame would make it a field of a
// marshalled struct, which is the thing this whole protocol is shaped to avoid.
//
// # The buffer
//
// Allocated at the token's own maximum, pinned, and overwritten by a deferred
// Wipe on every path out including the ones that return an error. pkcs11.Wipe
// rather than a loop here, because the rule now has two readers and a wipe
// written twice is the one function where a second copy would be worst.
// runtime.Pinner rather than runtime.KeepAlive for D-101's measured reason: a
// stack that grows moves the frame it was on, and KeepAlive says nothing about
// a value being copied.
//
// entry is a parameter rather than a field for the same reason it is one on
// pkcs11's own openOn: the screen belongs to whoever is orchestrating the
// signature, and a Worker that held one would be a Worker that could draw it.
func sendPIN(in io.Writer, exchangeID string, needs LoginNeeds, entry pkcs11.PINEntry, modulePath string) error {
	if entry == nil {
		return pkcs11.ErrNoPINEntry
	}

	// The child read these off CK_TOKEN_INFO; this end refuses the ones it will
	// not allocate for. Both ends applying one constant is one rule with two
	// readers (pkcs11.MaxPINLength), not two rules that could disagree.
	if needs.MaxPINLength <= 0 || needs.MaxPINLength > pkcs11.MaxPINLength {
		return fmt.Errorf("pkcs11 worker: the worker reports a maximum PIN length of %d, which this layer will not allocate for", needs.MaxPINLength)
	}
	if needs.MinPINLength < 0 || needs.MinPINLength > needs.MaxPINLength {
		return fmt.Errorf("pkcs11 worker: the worker reports a minimum PIN length of %d against a maximum of %d", needs.MinPINLength, needs.MaxPINLength)
	}

	dst := make([]byte, needs.MaxPINLength)
	var p runtime.Pinner
	p.Pin(&dst[0])
	defer func() {
		pkcs11.Wipe(dst)
		p.Unpin()
	}()

	n, err := entry(dst, pkcs11.PINRequest{
		TokenLabel:       needs.TokenLabel,
		TokenSerial:      needs.TokenSerial,
		CertificateLabel: needs.CertificateLabel,
		// The path this parent spawned the worker with, which it has always had
		// and did not learn from the child. A machine can have one card visible
		// through two modules at two versions (D-271, D-272), and when something
		// is wrong this is the only thing that tells them apart.
		ModulePath: modulePath,
		MinLength:  needs.MinPINLength,
	})
	if err != nil {
		return err // ErrPINCancelled included: a cancellation is not a failure
	}
	if n < needs.MinPINLength || n > needs.MaxPINLength {
		return &pkcs11.PINLengthError{Got: n, Min: needs.MinPINLength, Max: needs.MaxPINLength}
	}

	if err := WriteFrame(in, Request{Op: OpLoginPIN, Exchange: exchangeID, PINLength: n}); err != nil {
		return fmt.Errorf("%w: writing the PIN length: %w", ErrWorkerDied, err)
	}
	if _, err := in.Write(dst[:n]); err != nil {
		return fmt.Errorf("%w: writing the PIN: %w", ErrWorkerDied, err)
	}
	return nil
}

// remoteSession is one logged-in session, in another process. It implements
// keysource.Session.
//
// The certificate and the chain are read once, at login, and held here rather
// than asked for again: they are what the child saw at the moment it logged in,
// and a second read could answer differently because the card was swapped
// between them.
//
// Not safe for concurrent use, like every other keysource.Session. The Worker's
// own mutex makes interleaved frames impossible, but the thing underneath is a
// card, which is a single serial device whose driver serialises anyway (D-027)
// — and internal/signing is what owns serialisation (SPEC §8.5).
type remoteSession struct {
	worker *Worker

	cert  keysource.Certificate
	chain [][]byte

	closed bool
}

// SignDigest implements keysource.Session.
//
// The digest's length is not checked here. It is checked in the child, by the
// same digestInfo that turns it into the bytes C_Sign is given — which is the
// only place the mapping from algorithm to DigestInfo exists, and a second copy
// of a rule like that can only ever be a copy that is wrong later.
func (s *remoteSession) SignDigest(ctx context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if s.closed {
		return nil, fmt.Errorf("pkcs11 worker: this session is closed")
	}
	resp, err := s.worker.do(ctx, Request{
		Op:              OpSignDigest,
		DigestAlgorithm: int(alg),
		Digest:          digest,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Signature) == 0 {
		return nil, fmt.Errorf("pkcs11 worker: %s: the worker reported a signature of no bytes", s.worker.ModulePath())
	}
	return resp.Signature, nil
}

// Certificate implements keysource.Session.
func (s *remoteSession) Certificate() keysource.Certificate { return s.cert }

// Chain implements keysource.Session.
//
// What the token itself carries and never a guess, which for both Serbian cards
// this project has is empty — and empty for two different reasons, only one of
// which is the obvious one (D-274). The caller completes the chain.
func (s *remoteSession) Chain() [][]byte { return s.chain }

// Close logs the card out, leaving the worker and its module alive for the next
// operation.
//
// # It uses context.Background, which is a choice and not an oversight
//
// keysource.Session.Close takes no context, and nothing here may invent a
// duration to bound it with — D-297 left the per-request deadline open on
// purpose, as a question with its own justification and its own owner, and the
// place hardest to find it later is inside a Close. What bounds this in practice
// is Worker.Close, which does take one, and which the owner of the Worker calls.
//
// A failure is returned rather than swallowed, but the session is marked closed
// either way: the card's authenticated state is the child's to hold and this end
// has said its piece.
func (s *remoteSession) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	_, err := s.worker.do(context.Background(), Request{Op: OpCloseSession})
	return err
}
