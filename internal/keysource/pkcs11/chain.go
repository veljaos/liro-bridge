package pkcs11

import "crypto/x509"

// maxChainDepth bounds the walk. A certificate whose issuer is itself, or a
// pair that issue each other, would otherwise loop; the tokens this project
// has measured carry two certificates and no CA at all, so any depth beyond a
// handful means the token is lying about something.
const maxChainDepth = 8

// issuersFor returns the issuing chain for signerDER, built only from
// certificates the token itself carries, ordered from the signer's immediate
// issuer upward. The signer is not included.
//
// It returns what is there and never a guess. The only thing that counts as an
// issuer is an exact byte match between a candidate's RawSubject and the
// child's RawIssuer — not a string comparison of distinguished names, which is
// parser-dependent and format-fragile (SPEC §11.6 makes that point about
// subject parsing and it holds at least as strongly here), and not a signature
// check, because a token carrying a certificate it did not issue against is a
// different problem from a chain this layer cannot complete.
//
// # What this returns for a Serbian card, and what that costs
//
// Measured on a MUP e-ID card through both NetSeT modules: the token carries
// two certificates, the signing one and its authentication twin, and **no CA
// certificate at all**. Both are issued by "MUP Gradjani CA 4", which is not
// on the card. So this returns empty, and SPEC §11.8 already says why — MUP
// embeds one certificate in the CMS of a signed document where Halcom and
// Pošta embed three.
//
// That is not a cosmetic gap. SPEC §12.6 makes B-LT the default signature
// level, B-LT is B-T plus a /DSS carrying revocation evidence, and revocation
// evidence is collected per certificate in the chain. A missing chain is
// therefore a missing signature level rather than a missing field: the chain
// has to be completed from the certificate's own AIA caIssuers or from a
// bundled trust store (SPEC §11.8 requires exactly that for MUP) before a
// document signed through this path can reach the level the specification
// defaults to.
//
// The completion already exists for the CNG path and is not this layer's to
// do: keysource.Session.Chain's own contract is "the issuing chain if the
// source can supply it. May be empty; the caller is responsible for completing
// the chain". This returns what the token has, honestly and with nothing
// invented, and the caller does the rest.
func issuersFor(signerDER []byte, available [][]byte) [][]byte {
	signer, err := x509.ParseCertificate(signerDER)
	if err != nil {
		return nil
	}

	type candidate struct {
		cert *x509.Certificate
		der  []byte
	}
	var pool []candidate
	for _, der := range available {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			continue // a token object that is not a certificate is not an issuer
		}
		if equalBytes(der, signerDER) {
			continue // the signer is not its own chain
		}
		pool = append(pool, candidate{cert, der})
	}

	var chain [][]byte
	used := make(map[int]bool, len(pool))
	child := signer
	for depth := 0; depth < maxChainDepth; depth++ {
		if isSelfIssued(child) {
			return chain // a root issues itself; there is nothing above it
		}
		next := -1
		for i, c := range pool {
			if used[i] {
				continue
			}
			if equalBytes(c.cert.RawSubject, child.RawIssuer) {
				next = i
				break
			}
		}
		if next < 0 {
			return chain // the issuer is not on this token
		}
		used[next] = true
		chain = append(chain, pool[next].der)
		child = pool[next].cert
	}
	return chain
}

func isSelfIssued(c *x509.Certificate) bool { return equalBytes(c.RawSubject, c.RawIssuer) }

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
