//go:build windows

package pkcs11

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// ckaSign is CKA_SIGN, the attribute a private key sets when it may be used
// for C_Sign.
const ckaSign = 0x00000108

// maxSignatureLen bounds the buffer C_Sign's length call is allowed to ask
// for. An RSA signature is the modulus size — 256 bytes at RSA-2048, 512 at
// RSA-4096, which SPEC §12.9 records Pošta as migrating to. 1024 leaves room
// for a key size nobody issues yet and refuses a module that answers with
// something absurd rather than allocating for it.
const maxSignatureLen = 1024

// signSession is one logged-in session over one certificate's private key. It
// implements keysource.Session.
//
// It is unexported and handed out as the interface: the concrete type has
// nothing a caller needs, and this package already has a lowercase `session`
// for the PKCS#11 session underneath it. Two exported names differing only in
// case would be a poor trade for a type nobody names.
//
// Not safe for concurrent use, like every other keysource.Session. The card is
// a single serial device whose driver serialises anyway, so parallelism here
// buys no throughput and concurrent access to smart card APIs is a known
// source of driver-level failures (D-027).
type signSession struct {
	m    *module
	sess *session

	cert  keysource.Certificate
	chain [][]byte

	// key is the private key object this session signs with, found after the
	// login and not before: a public session sees zero private keys, measured
	// on a MUP card (D-271).
	key uint32
}

// SignDigest implements keysource.Session.
//
// The mechanism is CKM_RSA_PKCS and the data is a DigestInfo this layer
// builds. See digestinfo.go for why that is not the same as handing over the
// digest, and for how those bytes are known to be right.
func (s *signSession) SignDigest(ctx context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.sess == nil {
		return nil, fmt.Errorf("pkcs11: this session is closed")
	}
	info, err := digestInfo(alg, digest)
	if err != nil {
		return nil, err
	}
	return s.sess.sign(s.key, info)
}

// Certificate implements keysource.Session.
func (s *signSession) Certificate() keysource.Certificate { return s.cert }

// Chain implements keysource.Session.
//
// It is what the token itself carries and never a guess, which for both
// Serbian cards this project has is empty — and empty for two different
// reasons, only one of which is the obvious one. See issuersFor and D-274.
// The caller completes the chain; keysource.Session.Chain's own contract says
// so, and SPEC §11.8 already requires that completion for MUP.
func (s *signSession) Chain() [][]byte { return s.chain }

// Close logs out, closes the session and unloads the module.
//
// Logging out is not merely tidiness: SPEC §6.5's measured premise is that the
// card holds its own authenticated state independently of which process is
// talking to it, so a session left logged in is a card another process can
// sign with. The consent screen is the gate that actually holds (§6.5), but
// leaving the card open for longer than the batch that opened it is a thing to
// avoid on purpose rather than by accident.
func (s *signSession) Close() error {
	if s.sess == nil {
		return nil
	}
	s.sess.logout()
	err := s.sess.close()
	s.sess = nil
	if s.m != nil {
		if cerr := s.m.close(); err == nil {
			err = cerr
		}
		s.m = nil
	}
	return err
}

// privateKeyFor returns the handle of the private key paired with one
// certificate object.
//
// The pairing is CKA_ID: PKCS#11 §4.4 says a certificate and the key it
// belongs to share one, and it is how every card this project has met pairs
// them. Matching on anything else would be guessing — and a wrong key produces
// a signature that verifies against nothing while every layer reports success,
// which is the same silent failure F11 §2.1 is about one level down.
//
// A certificate with no CKA_ID is refused rather than resolved by taking the
// only private key on the token. On a Serbian card there are two key pairs, a
// signing one and an authentication one (SPEC §11.5), so "the only one" is not
// a case that arises and picking one would be picking the wrong one half the
// time.
func (s *session) privateKeyFor(certObj uint32) (uint32, error) {
	id, err := s.attributeValue(certObj, ckaID)
	if err != nil {
		return 0, err
	}
	if len(id) == 0 {
		return 0, fmt.Errorf("pkcs11: the certificate object on this token carries no CKA_ID, so the private key it belongs to cannot be identified")
	}
	// CKA_SIGN as well as the identifier: a token may carry a private key
	// that shares an identifier and is not for signing, and "the key paired
	// with this certificate" and "a key this card will sign with" are two
	// conditions rather than one. Asking for both is what makes the single
	// match below mean something.
	keys, err := s.findObjects([]attribute{
		{typ: ckaClass, value: u32Bytes(ckoPrivateKey)},
		{typ: ckaSign, value: []byte{1}},
		{typ: ckaID, value: id},
	})
	if err != nil {
		return 0, err
	}
	switch len(keys) {
	case 0:
		return 0, fmt.Errorf("pkcs11: this token has no private key with the certificate's CKA_ID; the login may not have taken effect")
	case 1:
		return keys[0], nil
	default:
		// More than one key sharing one CKA_ID is a token saying something
		// this layer does not understand. Refusing is better than choosing.
		return 0, fmt.Errorf("pkcs11: this token has %d private keys sharing the certificate's CKA_ID", len(keys))
	}
}

// sign runs C_SignInit and C_Sign over data with CKM_RSA_PKCS.
//
// Two calls for the length, as PKCS#11 requires: the first with a NULL
// pSignature to learn how many bytes the signature needs, the second to fill a
// buffer of that size.
func (s *session) sign(key uint32, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("pkcs11: nothing to sign")
	}

	var p runtime.Pinner
	defer p.Unpin()

	// CK_MECHANISM, packed: mechanism at +0 (CK_ULONG, 4), pParameter at +4
	// (8), ulParameterLen at +12 (4) — the same 16-byte shape and the same
	// reason as CK_ATTRIBUTE's. CKM_RSA_PKCS takes no parameter.
	mech := make([]byte, 16)
	putU32(mech[0:], ckmRSAPKCS)
	putU64(mech[4:], 0)
	putU32(mech[12:], 0)
	p.Pin(&mech[0])

	if rv := ckr(s.m.call(iSignInit, uintptr(s.handle),
		uintptr(unsafe.Pointer(&mech[0])), uintptr(key))); rv != ckrOK {
		return nil, &ckrError{"C_SignInit", rv}
	}

	p.Pin(&data[0])
	var n uint32
	p.Pin(&n)
	if rv := ckr(s.m.call(iSign, uintptr(s.handle),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		0, uintptr(unsafe.Pointer(&n)))); rv != ckrOK {
		return nil, &ckrError{"C_Sign(size)", rv}
	}
	if n == 0 || n > maxSignatureLen {
		return nil, fmt.Errorf("pkcs11: C_Sign wants a %d-byte signature buffer, which this layer will not allocate", n)
	}

	sig := make([]byte, n)
	p.Pin(&sig[0])
	if rv := ckr(s.m.call(iSign, uintptr(s.handle),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		uintptr(unsafe.Pointer(&sig[0])), uintptr(unsafe.Pointer(&n)))); rv != ckrOK {
		return nil, &ckrError{"C_Sign", rv}
	}
	if int(n) > len(sig) {
		return nil, fmt.Errorf("pkcs11: C_Sign reported %d bytes into a %d-byte buffer", n, len(sig))
	}
	return sig[:n], nil
}

// Open implements keysource.Source.
//
// The order is deliberate and each step is the reason the next one can happen:
// find the certificate first, because that is public and needs no login and
// tells us which slot to work in; log in; then find the private key, which was
// invisible until the login and is what a signature actually needs.
//
// A failure after the login closes the session, which logs out — a card left
// authenticated because an error path forgot is a card the next process can
// sign with (SPEC §6.5).
func (s Source) Open(ctx context.Context, want keysource.Thumbprint) (keysource.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m, err := openModule(s.modulePath)
	if err != nil {
		return nil, err
	}
	sess, err := s.openOn(ctx, m, want, s.entry)
	if err != nil {
		_ = m.close()
		return nil, err
	}
	// Ownership of the module transfers to the session, whose Close unloads it.
	// The worker's holder does not do this, which is the whole difference
	// between the two callers: there the module outlives the session and is
	// closed by whoever opened it.
	sess.m = m
	return sess, nil
}

// openOn is Open's body, against a module somebody else opened and will close,
// and with the PIN collected by an entry the caller supplies rather than by the
// one on this Source.
//
// The entry is a parameter here and a field on Source for the same reason the
// module is: the worker's child has neither. Its module is held open across
// many requests (D-299) and its PIN arrives on a pipe rather than from a screen
// this process drew, so both of the things Open reads off the receiver are
// things that caller has to hand in.
//
// It never sets signSession.m. A session that closed a module it did not open
// would unload it from under the holder, and the holder is the thing the worker
// exists to keep alive.
func (s Source) openOn(ctx context.Context, m *module, want keysource.Thumbprint, entry PINEntry) (*signSession, error) {
	slots, err := m.slots(true)
	if err != nil {
		return nil, err
	}

	for _, slot := range slots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ti, err := m.tokenInfo(slot)
		if err != nil {
			if rv, cok := asCKR(err); cok && (rv == ckrTokenNotRecognized || rv == ckrTokenNotPresent) {
				continue // not this module's card
			}
			return nil, err
		}

		sess, err := m.openSession(slot)
		if err != nil {
			return nil, err
		}

		certObj, der, label, found, err := findCertificateObject(sess, want)
		if err != nil {
			_ = sess.close()
			return nil, err
		}
		if !found {
			_ = sess.close()
			continue
		}

		// Everything above this line is read-only. Everything below can cost a
		// PIN attempt, and does so at most once (SPEC §6.5.1 clause 5).
		if ti.LoginRequired() {
			req := PINRequest{
				TokenLabel:       ti.Label,
				TokenSerial:      ti.SerialNumber,
				CertificateLabel: label,
				ModulePath:       s.modulePath,
			}
			if err := sess.login(ti, entry, req); err != nil {
				_ = sess.close()
				return nil, err
			}
		}

		key, err := sess.privateKeyFor(certObj)
		if err != nil {
			sess.logout()
			_ = sess.close()
			return nil, err
		}

		all, err := allCertificateDER(sess)
		if err != nil {
			sess.logout()
			_ = sess.close()
			return nil, err
		}

		return &signSession{
			sess:  sess,
			key:   key,
			cert:  keysource.Certificate{Thumbprint: want, DER: der},
			chain: issuersFor(der, all),
		}, nil
	}
	return nil, fmt.Errorf("%w: %s, through %s", ErrCertificateNotFound, want, s.modulePath)
}

// findCertificateObject looks for one certificate by thumbprint in an open
// session, returning its object handle and its bytes.
func findCertificateObject(sess *session, want keysource.Thumbprint) (obj uint32, der []byte, label string, found bool, err error) {
	objects, err := sess.certificateObjects()
	if err != nil {
		return 0, nil, "", false, err
	}
	for _, o := range objects {
		value, err := sess.attributeValue(o, ckaValue)
		if err != nil {
			return 0, nil, "", false, err
		}
		if len(value) == 0 {
			// A wrong CK_ATTRIBUTE layout returns CKR_OK with a zero length
			// rather than failing (D-271), so an empty value is refused loudly.
			return 0, nil, "", false, fmt.Errorf("pkcs11: object %d returned an empty CKA_VALUE", o)
		}
		if keysource.Thumbprint(thumbprint(value)) != want {
			continue
		}
		name, err := sess.attributeValue(o, ckaLabel)
		if err != nil {
			return 0, nil, "", false, err
		}
		return o, value, string(name), true, nil
	}
	return 0, nil, "", false, nil
}

// allCertificateDER returns every certificate on the token, for issuersFor to
// walk. Duplicated objects are collapsed so that a token presenting one
// certificate twice cannot make the chain walk see it as two candidates.
func allCertificateDER(sess *session) ([][]byte, error) {
	objects, err := sess.certificateObjects()
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, o := range objects {
		value, err := sess.attributeValue(o, ckaValue)
		if err != nil {
			return nil, err
		}
		if len(value) == 0 {
			continue
		}
		seen := false
		for _, existing := range out {
			if bytes.Equal(existing, value) {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, value)
		}
	}
	return out, nil
}
