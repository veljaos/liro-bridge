package pkcs11

import (
	"context"
	"crypto/x509"
	"fmt"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// Source implements keysource.Source over one PKCS#11 module.
//
// One Source is one module. A machine can have several — this one has NetSeT
// at two paths in two builds five years apart, plus SafeSign and Nexus — and
// each is its own Source. Deduplicating one card seen through two of them is
// the caller's job, not this type's, and the thumbprint is what makes it
// possible (F11 §4).
//
// # One-shot and held-open are two shapes, and both are here
//
// Enumerate, List and ChainFor each open a module, do one thing, and close it.
// That is right for a caller who wants an answer now and is wrong for a
// process whose whole reason to exist is that C_Initialize must not be
// repeated: paying it per request rolls D-272's dice on every listing, which
// turns a startup problem into a listing problem, and a listing that fails one
// time in a hundred is the kind of defect people learn to re-run instead of
// read (D-297).
//
// So the work is in methods that take an already-open module, and there are
// two ways to reach them: the three above, which open and close around one
// call, and Hold, which opens once and answers many. One rule, two callers,
// which is the consolidation D-108 is about — as against the one D-297 refuses,
// where a merged version would have to take a parameter to decide which of two
// things it is.
type Source struct {
	modulePath string

	// entry collects the PIN when a token needs one and offers no protected
	// authentication path. Nil until something wires one up, and a token that
	// needs it then fails with ErrNoPINEntry rather than silently.
	//
	// It holds a way to obtain a PIN and never a PIN: SPEC §6.5.1 clause 2
	// forbids the second, and pin_test.go is what makes that structural rather
	// than a promise.
	entry PINEntry
}

// WithPINEntry returns a copy of this Source that collects PINs with entry.
//
// A copy rather than a mutation because Source is a value type and Sources
// hands out several of them; a setter would make which module got the screen
// depend on the order somebody called it in.
func (s Source) WithPINEntry(entry PINEntry) Source {
	s.entry = entry
	return s
}

// NewSource returns a Source over the module at path.
//
// The path comes from configuration or from discovery and from nowhere else. A
// calling application naming a DLL for the agent to load is arbitrary code
// execution wearing a configuration field (F11 §3), and nothing in this
// package will take one from the protocol.
func NewSource(modulePath string) Source { return Source{modulePath: modulePath} }

// ModulePath is which module this Source speaks to. Two Sources over one card
// are told apart by this, and it is what a report or an audit entry would name
// if it ever names the backend.
func (s Source) ModulePath() string { return s.modulePath }

// Name implements keysource.Source.
func (Source) Name() string { return "pkcs11" }

// LiveModule is one module held open: C_Initialize called once, and not called
// again until Close.
//
// It exists for the worker (F12 §2), and the distinction it draws is the one
// D-297 records. Discovery spawns a throwaway child per candidate and pays
// C_Initialize per candidate, which is correct — a crash there is the expected
// outcome and costs one Failure and about 30 ms. A session has to survive many
// calls, and paying C_Initialize per call rolls the same dice every time.
//
// Not safe for concurrent use, and more strongly than module is: F12 §2
// requires one goroutine pinned with runtime.LockOSThread to make every PKCS#11
// call, and D-298 is why that is the braces rather than the belt — two of the
// four modules on this project's own machines never read the
// CK_C_INITIALIZE_ARGS at all, so their CKR_OK to CKF_OS_LOCKING_OK means
// nothing. Whoever holds one of these owns the thread it was opened on.
type LiveModule struct {
	source Source
	module *module
}

// Hold opens this Source's module and keeps it open until Close.
//
// It initialises with CKF_OS_LOCKING_OK, which openModule deliberately does not:
// F12 §2 requires it for the worker, and D-298 measured that two of four
// modules answer CKR_OK without having read the request. Passing it is right
// for the two that read it and costs nothing for the two that do not; relying
// on it is what D-298 forbids.
//
// The caller must already be on the thread it intends to make every later call
// from. C_Initialize and every call after it belong to one thread, and this
// function does not lock one for you — the lock has to outlive this call, so it
// belongs to whoever owns the loop (see RunWorker).
func (s Source) Hold() (*LiveModule, error) {
	if s.modulePath == "" {
		return nil, fmt.Errorf("pkcs11: no module path")
	}
	m, err := openModuleLocking(s.modulePath)
	if err != nil {
		return nil, err
	}
	return &LiveModule{source: s, module: m}, nil
}

// ModulePath is which module this holder has open.
func (l *LiveModule) ModulePath() string { return l.source.modulePath }

// Close calls C_Finalize and unloads the module. It is safe to call twice.
//
// Twice is the ordinary case rather than an edge one: the worker closes on an
// OpShutdown request and again from the defer that covers every other way its
// loop can end. Making the second call a no-op here is cheaper than making
// every caller remember, and it means the answer does not depend on what a
// second runtime.Pinner.Unpin does — a question this code would otherwise be
// resting on without having measured it.
func (l *LiveModule) Close() error {
	m := l.module
	if m == nil {
		return nil
	}
	l.module = nil
	return m.close()
}

// Enumerate is Source.Enumerate against the module this holder has open.
func (l *LiveModule) Enumerate(ctx context.Context) ([]CertificateInfo, error) {
	if l.module == nil {
		return nil, ErrModuleClosed
	}
	return l.source.enumerate(ctx, l.module)
}

// List is Source.List against the module this holder has open.
func (l *LiveModule) List(ctx context.Context) ([]keysource.Certificate, error) {
	if l.module == nil {
		return nil, ErrModuleClosed
	}
	return l.source.list(ctx, l.module)
}

// ChainFor is Source.ChainFor against the module this holder has open.
func (l *LiveModule) ChainFor(ctx context.Context, want keysource.Thumbprint) ([][]byte, error) {
	if l.module == nil {
		return nil, ErrModuleClosed
	}
	return l.source.chainFor(ctx, l.module, want)
}

// Enumerate returns every X.509 certificate this module can see, across every
// slot with a token in it.
//
// It opens the module, reads, and closes it. A caller that wants several
// answers from one module without paying C_Initialize for each — which is the
// worker, and only the worker — uses Hold instead.
//
// It needs no login, and it cannot tell which of them has a private key behind
// it. Measured on a MUP card: a public session sees 2 certificates and 2
// public keys and **0 private keys**, because private objects are hidden from
// a session that has not logged in. So "can this certificate actually sign"
// is not a question this layer answers — internal/trust/classify answers the
// part that matters from KeyUsage (SPEC §11.4: contentCommitment, never
// digitalSignature), and the rest is answered by the card when a signature is
// attempted.
func (s Source) Enumerate(ctx context.Context) ([]CertificateInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m, err := openModule(s.modulePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = m.close() }()
	return s.enumerate(ctx, m)
}

// enumerate is Enumerate's body, against a module somebody else opened and
// will close.
//
// A slot whose token this module does not recognise is skipped rather than
// failing the enumeration: SafeSign answers CKR_TOKEN_NOT_RECOGNIZED for a MUP
// card, and that means "not mine", not "something is wrong" (F11 §4).
func (s Source) enumerate(ctx context.Context, m *module) ([]CertificateInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	slots, err := m.slots(true)
	if err != nil {
		return nil, err
	}

	var out []CertificateInfo
	for _, slot := range slots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ti, err := m.tokenInfo(slot)
		if err != nil {
			if rv, ok := asCKR(err); ok && (rv == ckrTokenNotRecognized || rv == ckrTokenNotPresent) {
				continue // not this module's card
			}
			return nil, err
		}
		found, err := certificatesInSlot(m, slot)
		if err != nil {
			return nil, err
		}
		for _, c := range found {
			c.SlotID = slot
			c.TokenLabel = ti.Label
			c.TokenSerial = ti.SerialNumber
			c.ModulePath = s.modulePath
			c.ProtectedPIN = ti.HasProtectedAuthenticationPath()
			out = append(out, c)
		}
	}
	return out, nil
}

func certificatesInSlot(m *module, slot uint32) ([]CertificateInfo, error) {
	sess, err := m.openSession(slot)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sess.close() }()

	objects, err := sess.certificateObjects()
	if err != nil {
		return nil, err
	}
	var out []CertificateInfo
	for _, obj := range objects {
		der, err := sess.attributeValue(obj, ckaValue)
		if err != nil {
			return nil, err
		}
		if len(der) == 0 {
			// A wrong CK_ATTRIBUTE layout returns CKR_OK with a zero length
			// rather than failing, so an empty value is worth refusing loudly
			// rather than skipping quietly.
			return nil, fmt.Errorf("pkcs11: object %d in slot %d returned an empty CKA_VALUE", obj, slot)
		}
		if _, err := x509.ParseCertificate(der); err != nil {
			// Not every CKO_CERTIFICATE is one this project can read; skip it
			// rather than failing the whole enumeration over somebody else's
			// object.
			continue
		}
		label, err := sess.attributeValue(obj, ckaLabel)
		if err != nil {
			return nil, err
		}
		out = append(out, CertificateInfo{
			Thumbprint: thumbprint(der),
			DER:        der,
			Label:      string(label),
		})
	}
	return out, nil
}

// List implements keysource.Source, deduplicated on thumbprint.
//
// One card in one reader can be reported more than once by one module: a
// module may present the same token in several slots, and a token may carry
// the same certificate as more than one object. The thumbprint is the
// certificate, so the first sighting wins and the rest are dropped — the same
// value the CNG backend computes for the same bytes, which is what lets the
// layer above collapse one card seen through two backends into one row
// (F11 §4).
//
// The order objects come back in is not relied on anywhere. Measured: the two
// NetSeT builds return this card's two certificates in opposite order —
// Sign-then-Auth from TrustEdgeID 1.1.3.3, Auth-then-Sign from MUP RS\Celik
// 1.1.0.0.
func (s Source) List(ctx context.Context) ([]keysource.Certificate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m, err := openModule(s.modulePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = m.close() }()
	return s.list(ctx, m)
}

// list is List's body, against a module somebody else opened and will close.
func (s Source) list(ctx context.Context, m *module) ([]keysource.Certificate, error) {
	found, err := s.enumerate(ctx, m)
	if err != nil {
		return nil, err
	}
	return dedupe(found), nil
}

// Open is implemented in sign_windows.go, where the login step and the
// signing session it produces live together. It is the one method in this
// package that can cost a PIN attempt; everything else here is read-only.

// ChainFor returns the issuing chain the token itself carries for one of its
// certificates, which is what keysource.Session.Chain will return once a
// session can be opened.
//
// It is exported and takes a thumbprint so that the answer is available now,
// before the login step exists — because what it returns decides whether a
// document signed through this path can reach SPEC §12.6's default level.
// Measured on a MUP card: empty. See issuersFor for what that costs.
func (s Source) ChainFor(ctx context.Context, want keysource.Thumbprint) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m, err := openModule(s.modulePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = m.close() }()
	return s.chainFor(ctx, m, want)
}

// chainFor is ChainFor's body, against a module somebody else opened and will
// close.
func (s Source) chainFor(ctx context.Context, m *module, want keysource.Thumbprint) ([][]byte, error) {
	found, err := s.enumerate(ctx, m)
	if err != nil {
		return nil, err
	}
	var signer []byte
	all := make([][]byte, 0, len(found))
	for _, c := range found {
		all = append(all, c.DER)
		if keysource.Thumbprint(c.Thumbprint) == want {
			signer = c.DER
		}
	}
	if signer == nil {
		return nil, fmt.Errorf("pkcs11: no certificate with thumbprint %s on any token this module sees", want)
	}
	return issuersFor(signer, all), nil
}
