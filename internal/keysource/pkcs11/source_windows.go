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
type Source struct {
	modulePath string
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

// Enumerate returns every X.509 certificate this module can see, across every
// slot with a token in it.
//
// It needs no login, and it cannot tell which of them has a private key behind
// it. Measured on a MUP card: a public session sees 2 certificates and 2
// public keys and **0 private keys**, because private objects are hidden from
// a session that has not logged in. So "can this certificate actually sign"
// is not a question this layer answers — internal/trust/classify answers the
// part that matters from KeyUsage (SPEC §11.4: contentCommitment, never
// digitalSignature), and the rest is answered by the card when a signature is
// attempted.
//
// A slot whose token this module does not recognise is skipped rather than
// failing the enumeration: SafeSign answers CKR_TOKEN_NOT_RECOGNIZED for a MUP
// card, and that means "not mine", not "something is wrong" (F11 §4).
func (s Source) Enumerate(ctx context.Context) ([]CertificateInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m, err := openModule(s.modulePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = m.close() }()

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
	found, err := s.Enumerate(ctx)
	if err != nil {
		return nil, err
	}
	return dedupe(found), nil
}

// Open implements keysource.Source, and stops at the wall.
//
// See ErrLoginNotBuilt. Everything above this line is read-only and cannot
// spend a PIN attempt; this is the one method that would, and it is not built.
func (s Source) Open(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, ErrLoginNotBuilt
}

// ChainFor returns the issuing chain the token itself carries for one of its
// certificates, which is what keysource.Session.Chain will return once a
// session can be opened.
//
// It is exported and takes a thumbprint so that the answer is available now,
// before the login step exists — because what it returns decides whether a
// document signed through this path can reach SPEC §12.6's default level.
// Measured on a MUP card: empty. See issuersFor for what that costs.
func (s Source) ChainFor(ctx context.Context, want keysource.Thumbprint) ([][]byte, error) {
	found, err := s.Enumerate(ctx)
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
