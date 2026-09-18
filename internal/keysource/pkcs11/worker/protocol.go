package worker

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Op is one thing the worker will do. The set is closed and small, and
// protocol_guards_test.go fails when it changes, because every addition widens
// what a release binary can be asked to do (F12 §2).
type Op string

const (
	// OpEnumerate reads certificate objects off the token.
	OpEnumerate Op = "enumerate"
	// OpList is the same reading, shaped for the agent's listing.
	OpList Op = "list"
	// OpChainFor returns the issuer chain for one certificate.
	OpChainFor Op = "chainfor"
	// OpShutdown calls C_Finalize and exits. F12 §2: "C_Finalize only on
	// shutdown or a deliberate reset."
	OpShutdown Op = "shutdown"
)

// Request is one thing the parent asks for.
//
// # There is no module path here, and that is the point
//
// The worker is told which module to load by its command line, once, by the
// parent that spawned it. F12 §10: "A configured path remains the escape hatch,
// and a protocol-supplied path remains refused." If a path could arrive in a
// request, anything that reaches this pipe could choose which foreign DLL this
// process loads — and the pipe is reachable from the agent, which is reachable
// from F7's local protocol, which is reachable from a web page. That chain is
// cut here rather than checked for at the far end, and
// TestTheProtocolCarriesNoModulePath keeps it cut.
//
// There is no PIN here either, and never will be. The PIN does not travel as a
// field in a struct that something might log, marshal or keep; it is written
// once, on its own, and read by an exact-length read (SPEC §6.5.1 clause 2).
// pin_test.go enforces that over this package's whole syntax tree.
type Request struct {
	Op Op `json:"op"`

	// Thumbprint selects one certificate, for OpChainFor. Hex, lower case.
	// It is an identifier the parent already holds, not a path and not a
	// handle: the worker resolves it against what is on the token, so a
	// value that names nothing is an error rather than a reach into memory.
	Thumbprint string `json:"thumbprint,omitempty"`
}

// CertificatePayload is one certificate as it crosses the boundary: the DER,
// and the label if the token gave the object one.
//
// It is deliberately not keysource.Certificate. The worker reports what is on
// the card; whether a certificate is *usable* depends on presence, trust and
// policy, which are the agent's questions and are answered with things the
// worker cannot see. Sending the richer type would invite the worker to fill in
// fields it has no business deciding.
//
// # CKA_ID is not here, and the line is where it can be used
//
// An earlier draft carried the object's CKA_ID alongside the label, as the
// other attribute that identifies the object on the token. It is removed
// because the parent can never use it: CKA_ID identifies an object *within the
// child's own session*, which is the one place the parent has no handle on.
// Every lookup that needs it — finding the private key that goes with a
// certificate — happens in the child, from the child's own read.
//
// Label stays for the opposite reason: it names the certificate to a person,
// which is a thing the parent could put on a screen or in a log. That is the
// line, and it is worth stating because "identifies the object" was true of
// both and is not the test. The test is whether the far end can do anything
// with it.
type CertificatePayload struct {
	DER   []byte `json:"der"`
	Label string `json:"label,omitempty"`
}

// Response is one answer. Exactly one request gets exactly one of these.
type Response struct {
	// Err is empty on success. It is a string rather than a structured code
	// because the parent turns it into a Failure with the module's path
	// attached, and F11 §3 asks for a readable reason rather than a number.
	Err string `json:"error,omitempty"`

	Certificates []CertificatePayload `json:"certificates,omitempty"`
	Chain        [][]byte             `json:"chain,omitempty"`
}

// maxFrame bounds what a single frame may claim to be.
//
// Without it, a corrupt or hostile length prefix is an allocation of up to 4GB
// in whichever process reads it — and the parent reads frames from a process
// whose address space a vendor module has been running in. One megabyte is
// comfortably more than any answer this protocol has: the largest is a
// certificate chain, and a long chain of large certificates is tens of
// kilobytes.
const maxFrame = 1 << 20

var (
	// ErrFrameTooLarge is a length prefix that claims more than maxFrame.
	ErrFrameTooLarge = errors.New("pkcs11 worker: frame larger than the protocol allows")
	// ErrShortFrame is a frame that ended early — which is what a worker
	// dying mid-answer looks like from the parent's side.
	ErrShortFrame = errors.New("pkcs11 worker: the frame ended before it was complete")
)

// WriteFrame writes one length-prefixed JSON value.
//
// # Why length-prefixed rather than newline-delimited
//
// This is SPEC §6.5.1 clause 2's doing, not a preference. The clause permits
// the PIN to cross one process boundary and bounds it: "one write, read
// immediately, never buffered."
//
// Newline-delimited JSON is read with a buffered reader, and a buffered reader
// reads ahead. Asked for a request, it may pull whatever follows into its own
// buffer — and what follows a login request is the PIN. It would then sit in
// this process's heap for the life of the reader, never overwritten, invisible
// to pin_test.go because it is not a field, a parameter or a named result but
// somebody else's byte slice.
//
// Length prefixing means ReadFrame reads exactly four bytes and then exactly
// that many, never one more. A PIN written immediately after a request can be
// read by an exact-length read and has been buffered by nothing. That is what
// makes one pipe sufficient, which matters because os/exec's ExtraFiles — the
// obvious way to give a child a second, dedicated pipe — is not supported on
// Windows.
//
// The header and the payload go out in one Write so that a frame cannot be torn
// across two, which a reader on the far side would have to reassemble anyway
// but which makes a truncated write unambiguous.
func WriteFrame(w io.Writer, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("pkcs11 worker: encoding a frame: %w", err)
	}
	if len(payload) > maxFrame {
		return fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, len(payload))
	}

	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)

	if _, err := w.Write(frame); err != nil {
		return fmt.Errorf("pkcs11 worker: writing a frame: %w", err)
	}
	return nil
}

// ReadFrame reads exactly one length-prefixed JSON value into v.
//
// It reads four bytes and then exactly the number they name. It never reads
// ahead, which is the property WriteFrame's comment explains and which
// TestTheWorkerNeverBuffersItsInput keeps true by forbidding bufio in this
// package.
//
// A reader that ends before the frame is complete returns ErrShortFrame wrapping
// io.ErrUnexpectedEOF: from the parent's side that is what a worker dying
// mid-answer looks like, and the supervisor turns it into a Failure rather than
// treating it as a protocol violation.
func ReadFrame(r io.Reader, v any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return fmt.Errorf("%w: %w", ErrShortFrame, err)
		}
		return err // io.EOF between frames is an ordinary end, not an error
	}

	n := binary.BigEndian.Uint32(header[:])
	if n > maxFrame {
		return fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, n)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return fmt.Errorf("%w: %w", ErrShortFrame, err)
	}
	return json.Unmarshal(payload, v)
}
