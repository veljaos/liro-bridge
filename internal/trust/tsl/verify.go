package tsl

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/veljaos/liro-bridge/internal/trust/tsl/c14n"
)

// dsigNS and exclusiveC14NAlgorithm are the two XML-DSig constants this
// package cares about. The document itself may bind any prefix to
// dsigNS (F1's real list uses "ns2" at the root and "ds" locally on the
// Signature element) — lookups are always by namespace URI, never by
// prefix text.
const (
	dsigNS                  = "http://www.w3.org/2000/09/xmldsig#"
	exclusiveC14NAlgorithm  = "http://www.w3.org/2001/10/xml-exc-c14n#"
	envelopedSigTransform   = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
	digestAlgoSHA256        = "http://www.w3.org/2001/04/xmlenc#sha256"
	digestAlgoSHA512        = "http://www.w3.org/2001/04/xmlenc#sha512"
	sigAlgoRSASHA256        = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	sigAlgoRSASHA256Classic = "http://www.w3.org/2000/09/xmldsig#rsa-sha256" // seen from some tools; treated identically to the "more#" URI
	sigAlgoRSASHA512        = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha512"
)

// ErrSignatureInvalid means the list's XML-DSig signature did not verify:
// wrong signature, wrong digest, or the signer certificate is not one of
// the two pinned Ministry certificates (F1 §4.6). It is also returned
// for a document that does not even parse as a signed Trusted List.
var ErrSignatureInvalid = errors.New("trusted list signature is invalid")

// PinnedSigners are the SHA-256 fingerprints (lowercase hex) of the two
// certificates the Ministry publishes for signing the Trusted List
// (F1 §4.6). There is no chain to validate — both are self-signed — so a
// fingerprint match is both simpler and stronger than chain validation.
var PinnedSigners = map[string]bool{
	"cfd20b5a6696621266171c7cd3969bce23bbb2910ddf73bbf54e235d26b7e4b1": true, // Serbian Trusted List Signer 1, valid to 2028-06-10
	"397e057c6d818feaa4e17154aeb88c2071ade2077520de42e092853e73336d2d": true, // Serbian Trusted List Signer 2, valid to 2028-12-31
}

func hashFor(algorithm string) (crypto.Hash, error) {
	switch algorithm {
	case digestAlgoSHA256, sigAlgoRSASHA256, sigAlgoRSASHA256Classic:
		return crypto.SHA256, nil
	case digestAlgoSHA512, sigAlgoRSASHA512:
		return crypto.SHA512, nil
	default:
		// Deliberately no SHA-1 case: SPEC §18.8 forbids producing SHA-1
		// anywhere, and accepting it here to verify a signature would be
		// a downgrade path for the one document that decides what the
		// agent trusts. An unrecognised algorithm fails closed.
		return 0, fmt.Errorf("%w: unsupported algorithm %q", ErrSignatureInvalid, algorithm)
	}
}

func digest(alg crypto.Hash, data []byte) []byte {
	switch alg {
	case crypto.SHA256:
		sum := sha256.Sum256(data)
		return sum[:]
	case crypto.SHA512:
		sum := sha512.Sum512(data)
		return sum[:]
	default:
		panic("tsl: digest called with an unvetted hash algorithm")
	}
}

func attrValue(n *c14n.Node, local string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Prefix == "" && a.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

func childrenNamed(n *c14n.Node, uri, local string) []*c14n.Node {
	var out []*c14n.Node
	for _, c := range n.Children {
		if c.Kind == c14n.ElementNode && c.URI == uri && c.Local == local {
			out = append(out, c)
		}
	}
	return out
}

func child(n *c14n.Node, uri, local string) *c14n.Node {
	if cs := childrenNamed(n, uri, local); len(cs) > 0 {
		return cs[0]
	}
	return nil
}

func decodeBase64Element(n *c14n.Node) ([]byte, error) {
	if n == nil {
		return nil, errors.New("element not found")
	}
	s := strings.Join(strings.Fields(c14n.Text(n)), "")
	return base64.StdEncoding.DecodeString(s)
}

// Verify checks the Trusted List XML's enveloped XML-DSig signature:
//
//  1. The signer certificate embedded in KeyInfo must be one of the two
//     pinned Ministry certificates (F1 §4.6) — SHA-256 fingerprint match,
//     no chain building.
//  2. The RSA signature in SignatureValue must verify against
//     Exclusive-C14N-canonicalised SignedInfo (F1 §4.5/§4.9).
//  3. SignedInfo's Reference over the whole document (URI="", with the
//     enveloped-signature transform) must have a DigestValue matching the
//     document's own content — proving what was signed is what is here.
//
// It does not verify the XAdES SignedProperties reference: F1's trust
// decision rests on the pinned certificate and the document content, and
// nothing here consumes XAdES metadata such as the claimed signing time
// (see docs/decisions.md).
func Verify(raw []byte) error {
	root, err := c14n.Parse(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}

	sig := c14n.Find(root, dsigNS, "Signature")
	if sig == nil {
		return fmt.Errorf("%w: no XML-DSig Signature element found", ErrSignatureInvalid)
	}
	signedInfo := child(sig, dsigNS, "SignedInfo")
	sigValueNode := child(sig, dsigNS, "SignatureValue")
	keyInfo := child(sig, dsigNS, "KeyInfo")
	if signedInfo == nil || sigValueNode == nil || keyInfo == nil {
		return fmt.Errorf("%w: missing SignedInfo, SignatureValue or KeyInfo", ErrSignatureInvalid)
	}

	canonMethod := child(signedInfo, dsigNS, "CanonicalizationMethod")
	if algo, ok := attrValue(canonMethod, "Algorithm"); !ok || algo != exclusiveC14NAlgorithm {
		return fmt.Errorf("%w: unsupported CanonicalizationMethod %q", ErrSignatureInvalid, algo)
	}

	sigMethod := child(signedInfo, dsigNS, "SignatureMethod")
	sigAlgoURI, _ := attrValue(sigMethod, "Algorithm")
	sigHash, err := hashFor(sigAlgoURI)
	if err != nil {
		return err
	}

	// --- Pinned certificate ---
	x509Data := child(keyInfo, dsigNS, "X509Data")
	certNode := child(x509Data, dsigNS, "X509Certificate")
	certDER, err := decodeBase64Element(certNode)
	if err != nil {
		return fmt.Errorf("%w: decoding signer certificate: %v", ErrSignatureInvalid, err)
	}
	fp := sha256.Sum256(certDER)
	fpHex := hex.EncodeToString(fp[:])
	if !PinnedSigners[fpHex] {
		return fmt.Errorf("%w: signer certificate %s is not a pinned Trusted List signer", ErrSignatureInvalid, fpHex)
	}
	signerCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("%w: parsing signer certificate: %v", ErrSignatureInvalid, err)
	}
	pub, ok := signerCert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("%w: signer certificate does not carry an RSA key", ErrSignatureInvalid)
	}

	// --- Signature over SignedInfo ---
	signedInfoC14N := c14n.Canonicalize(signedInfo, nil)
	signedInfoDigest := digest(sigHash, signedInfoC14N)

	sigValue, err := decodeBase64Element(sigValueNode)
	if err != nil {
		return fmt.Errorf("%w: decoding SignatureValue: %v", ErrSignatureInvalid, err)
	}
	if err := rsa.VerifyPKCS1v15(pub, sigHash, signedInfoDigest, sigValue); err != nil {
		return fmt.Errorf("%w: RSA signature over SignedInfo did not verify", ErrSignatureInvalid)
	}

	// --- Whole-document Reference (URI="") ---
	docRef := findReferenceByURI(signedInfo, "")
	if docRef == nil {
		return fmt.Errorf("%w: no whole-document Reference (URI=\"\") in SignedInfo", ErrSignatureInvalid)
	}
	if err := verifyEnvelopedReference(root, sig, docRef); err != nil {
		return err
	}

	return nil
}

func findReferenceByURI(signedInfo *c14n.Node, uri string) *c14n.Node {
	for _, ref := range childrenNamed(signedInfo, dsigNS, "Reference") {
		if u, ok := attrValue(ref, "URI"); ok && u == uri {
			return ref
		}
	}
	return nil
}

// verifyEnvelopedReference checks one Reference whose Transforms are
// exactly [enveloped-signature, exclusive-c14n] against the document
// root, which is what the Trusted List's whole-document Reference uses
// (F1 §4.5: "the enveloped-signature transform means: remove the
// Signature element itself, then canonicalise, then digest").
func verifyEnvelopedReference(root, sig, ref *c14n.Node) error {
	transformsNode := child(ref, dsigNS, "Transforms")
	if transformsNode == nil {
		return fmt.Errorf("%w: Reference has no Transforms", ErrSignatureInvalid)
	}
	transforms := childrenNamed(transformsNode, dsigNS, "Transform")
	sawEnveloped, sawC14N := false, false
	for _, t := range transforms {
		algo, _ := attrValue(t, "Algorithm")
		switch algo {
		case envelopedSigTransform:
			sawEnveloped = true
		case exclusiveC14NAlgorithm:
			sawC14N = true
		default:
			return fmt.Errorf("%w: unsupported Reference transform %q", ErrSignatureInvalid, algo)
		}
	}
	if !sawEnveloped || !sawC14N {
		return fmt.Errorf("%w: Reference does not use [enveloped-signature, exclusive-c14n]", ErrSignatureInvalid)
	}

	digestMethod := child(ref, dsigNS, "DigestMethod")
	algo, _ := attrValue(digestMethod, "Algorithm")
	hash, err := hashFor(algo)
	if err != nil {
		return err
	}

	digestValueNode := child(ref, dsigNS, "DigestValue")
	wantDigest, err := decodeBase64Element(digestValueNode)
	if err != nil {
		return fmt.Errorf("%w: decoding Reference DigestValue: %v", ErrSignatureInvalid, err)
	}

	excludeSignature := func(n *c14n.Node) bool { return n == sig }
	got := digest(hash, c14n.Canonicalize(root, excludeSignature))
	if !bytes.Equal(got, wantDigest) {
		return fmt.Errorf("%w: document digest does not match the signed DigestValue", ErrSignatureInvalid)
	}
	return nil
}
