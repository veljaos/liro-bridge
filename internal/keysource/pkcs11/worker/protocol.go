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

	// OpLogin opens a session on the token holding one certificate and logs
	// in. It is the first half of the PIN exchange and, on a token that
	// advertises a protected authentication path, the whole of it.
	OpLogin Op = "login"
	// OpLoginPIN carries the length of a PIN that follows this frame as raw
	// bytes. It is only ever sent in answer to a Response carrying LoginNeeds.
	OpLoginPIN Op = "login-pin"
	// OpSignDigest signs one DigestInfo with the key found at login. SPEC
	// §5.1's own boundary: key sources sign hashes and do not know what a PDF
	// is.
	OpSignDigest Op = "signdigest"
	// OpCloseSession logs out and closes the session, leaving the module
	// loaded. SPEC §6.5: a session left logged in is a card another process can
	// sign with.
	OpCloseSession Op = "closesession"
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

	// Thumbprint selects one certificate, for OpChainFor and OpLogin.
	// It is an identifier the parent already holds, not a path and not a
	// handle: the worker resolves it against what is on the token, so a
	// value that names nothing is an error rather than a reach into memory.
	Thumbprint string `json:"thumbprint,omitempty"`

	// Exchange binds a login to the operation a person approved.
	//
	// The parent mints one per Open — which is the call the consent screen
	// leads to — sends it with OpLogin, and refuses to act on a PIN request
	// that does not carry it back. A worker that can ask for a PIN at will is a
	// worker that can make PIN dialogs appear, and that dialog is the one window
	// in this program that deliberately looks like a system dialog (D-277,
	// SPEC §10). The binding is to the approved operation and not to the worker
	// being alive.
	//
	// The parent's own call structure already scopes it — the mutex is held and
	// a Response's LoginNeeds is only read inside the exchange — and that is not
	// enough on its own: a binding that lives only in the shape of one function
	// is a binding the next refactor loses without noticing.
	Exchange string `json:"exchange,omitempty"`

	// PINLength is how many raw bytes follow this frame, for OpLoginPIN.
	//
	// It is a length and not a PIN, which is why it can be here at all: the
	// guard in pin_test.go asks what a declaration can *hold* rather than what
	// it is called (D-270), and an int cannot hold a PIN however it is named.
	// The PIN itself is never a field, never marshalled, and never in a frame —
	// it is written once, on its own, immediately behind this one, and read by
	// an exact-length read (SPEC §6.5.1 clause 2).
	PINLength int `json:"pinLength,omitempty"`

	// DigestAlgorithm is keysource.DigestAlgorithm as an integer, for
	// OpSignDigest. It crosses as a number rather than as the named type
	// because the far end is a process: a named type in a JSON field is a type
	// the far end has to have.
	DigestAlgorithm int `json:"digestAlgorithm,omitempty"`

	// Digest is the hash to be signed. Not the document, and not a PIN: SPEC
	// §5.1 puts the document boundary above this layer, and this is what a key
	// source is for.
	Digest []byte `json:"digest,omitempty"`
}

// LoginNeeds is what a token wants before it will log in, as the child found
// it: present on a Response when — and only when — the child has an open
// session and is waiting to read a PIN.
//
// # Its presence is the question, which is why there is no NeedsPIN flag
//
// The parent cannot know in advance whether a PIN is needed at all. SPEC
// §6.5.1 clause 1 uses the protected authentication path wherever a module
// advertises one, and only the child can see CKF_PROTECTED_AUTHENTICATION_PATH
// — it is a property of a token through a module, read per token every time
// (D-273, D-276), so a reader with a pinpad answers differently through the
// same DLL. A token with a protected path never produces one of these, and the
// login simply completes.
//
// That is the reason the exchange has two phases. That it also gives clause 2's
// "one write and no more" its most exact form — the PIN is written into a
// reader that is already waiting to consume it — is a consequence rather than
// something anyone had to arrange.
type LoginNeeds struct {
	// Exchange is the value the parent sent with OpLogin, echoed back. A
	// LoginNeeds that does not carry it is not an answer to anything this
	// parent asked for.
	Exchange string `json:"exchange"`

	// MinPINLength and MaxPINLength are this token's own, read from
	// CK_TOKEN_INFO at the moment they are needed. SPEC §6.5.1 clause 7: "the
	// limits are the token's, read when they are needed: measured, a MUP token
	// declares 4 and 8 where a Pošta token declares 5 and 15, and both live in
	// one person's drawer."
	//
	// The parent enforces them before it writes, so a length that cannot be
	// right reaches neither the pipe nor the card — which is what D-268 cost
	// one of three attempts to establish is nobody else's job.
	MinPINLength int `json:"minPinLength"`
	MaxPINLength int `json:"maxPinLength"`

	// TokenLabel, TokenSerial and CertificateLabel are what the screen needs in
	// order to obey clause 6: a person must be able to tell they are giving
	// their PIN to Liro Bridge, and for which card.
	TokenLabel       string `json:"tokenLabel,omitempty"`
	TokenSerial      string `json:"tokenSerial,omitempty"`
	CertificateLabel string `json:"certificateLabel,omitempty"`
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

	// Login is present only on the mid-exchange answer to OpLogin, and its
	// presence is the child saying it has a session open and is waiting to read
	// a PIN. See LoginNeeds.
	Login *LoginNeeds `json:"login,omitempty"`

	// Certificate is the signer the login resolved, answered once the login has
	// completed. It is here rather than in Certificates because it is one
	// certificate chosen by thumbprint rather than a listing.
	Certificate *CertificatePayload `json:"certificate,omitempty"`

	// Signature is what C_Sign produced, for OpSignDigest.
	Signature []byte `json:"signature,omitempty"`
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
